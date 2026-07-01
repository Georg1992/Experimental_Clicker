//go:build windows

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	_ "embed"

	"experimental-clicker/runner"

	"github.com/Alia5/VIIPER/viiperclient"
	"golang.org/x/sys/windows"
)

//go:embed embed/viiper.exe
var viiperBin []byte

const (
	serverWaitTime   = 30 * time.Second
	serverPollPeriod = 200 * time.Millisecond
)

var (
	serverMu      sync.Mutex
	serverCmd     *exec.Cmd
	serverStarted bool
	serverPID     int
	viiperTempDir string
)

func ensureViiperServer(ctx context.Context, log func(string)) (started bool, err error) {
	serverMu.Lock()
	defer serverMu.Unlock()

	// Quick ping with a short timeout — don't hold serverMu while a
	// stale listener forces TCP to retransmit for minutes.
	pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pingCancel()
	api := viiperclient.New(runner.DefaultAPIAddr)
	if _, err := api.PingCtx(pingCtx); err == nil {
		return false, nil
	}

	log("Starting VIIPER server...")

	path, dir, err := extractViiper()
	if err != nil {
		return false, err
	}
	viiperTempDir = dir

	cmd := exec.Command(path, "server")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return false, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("start server: %w", err)
	}
	serverPID = cmd.Process.Pid

	go discardViiperOutput(stdout)
	go discardViiperOutput(stderr)

	log("Waiting for VIIPER server...")
	if err := waitForServer(ctx, runner.DefaultAPIAddr, serverWaitTime); err != nil {
		killProcessTree(serverPID)
		_, _ = cmd.Process.Wait()
		serverPID = 0
		removeViiperTempDirPath(viiperTempDir)
		viiperTempDir = ""
		return false, err
	}

	serverCmd = cmd
	serverStarted = true
	return true, nil
}

func discardViiperOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
	}
}

func stopViiperServerIfStarted() {
	serverMu.Lock()
	pid := serverPID
	started := serverStarted
	cmd := serverCmd
	serverPID = 0
	serverStarted = false
	serverCmd = nil
	dir := viiperTempDir
	viiperTempDir = ""
	serverMu.Unlock()

	if !started || pid <= 0 {
		return
	}

	killProcessTree(pid)
	if cmd != nil && cmd.Process != nil {
		// Best-effort wait; process may have already terminated
		_, _ = cmd.Process.Wait()
	}
	removeViiperTempDirPath(dir)
}

func killProcessTree(pid int) {
	if pid <= 0 {
		return
	}
	// Best-effort kill; process may have already exited or be unkillable
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

func removeViiperTempDirPath(dir string) {
	if dir == "" {
		return
	}
	// Best-effort cleanup; files may still be in use by the process
	_ = os.RemoveAll(dir)
}

func extractViiper() (string, string, error) {
	dir, err := os.MkdirTemp("", "viiper-clicker-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}
	path := filepath.Join(dir, "viiper.exe")
	if err := os.WriteFile(path, viiperBin, 0o755); err != nil {
		// Best-effort cleanup on write failure
		_ = os.RemoveAll(dir)
		return "", "", fmt.Errorf("write viiper.exe: %w", err)
	}
	return path, dir, nil
}

func waitForServer(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	api := viiperclient.New(addr)

	for time.Now().Before(deadline) {
		// Honour cancellation so onStop can release serverMu promptly.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		pingCtx, pingCancel := context.WithTimeout(ctx, 1*time.Second)
		_, err := api.PingCtx(pingCtx)
		pingCancel()

		if err == nil {
			return nil
		}

		time.Sleep(serverPollPeriod)
	}
	return fmt.Errorf("server ping timed out after %s", timeout)
}
