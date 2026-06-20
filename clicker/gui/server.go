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

	"github.com/Alia5/VIIPER/viiperclient"
	"golang.org/x/sys/windows"
)

//go:embed embed/viiper.exe
var viiperBin []byte

const (
	serverAPIAddr    = "localhost:3242"
	serverWaitTime   = 30 * time.Second
	serverPollPeriod = 200 * time.Millisecond
)

var (
	serverMu    sync.Mutex
	serverCmd   *exec.Cmd
	serverStarted bool
	serverPID   int
	viiperTempDir string
)

func ensureViiperServer() (started bool, err error) {
	serverMu.Lock()
	defer serverMu.Unlock()

	api := viiperclient.New(serverAPIAddr)
	if _, err := api.PingCtx(context.Background()); err == nil {
		return false, nil
	}

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

	if err := waitForServer(serverAPIAddr, serverWaitTime); err != nil {
		killProcessTree(serverPID)
		_, _ = cmd.Process.Wait()
		serverPID = 0
		removeViiperTempDir()
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
		_, _ = cmd.Process.Wait()
	}
	removeViiperTempDirPath(dir)
}

func killProcessTree(pid int) {
	if pid <= 0 {
		return
	}
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

func removeViiperTempDir() {
	serverMu.Lock()
	dir := viiperTempDir
	viiperTempDir = ""
	serverMu.Unlock()
	removeViiperTempDirPath(dir)
}

func removeViiperTempDirPath(dir string) {
	if dir == "" {
		return
	}
	_ = os.RemoveAll(dir)
}

func extractViiper() (string, string, error) {
	dir, err := os.MkdirTemp("", "viiper-clicker-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}
	path := filepath.Join(dir, "viiper.exe")
	if err := os.WriteFile(path, viiperBin, 0o755); err != nil {
		_ = os.RemoveAll(dir)
		return "", "", fmt.Errorf("write viiper.exe: %w", err)
	}
	return path, dir, nil
}

func waitForServer(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	api := viiperclient.New(addr)

	for time.Now().Before(deadline) {
		if _, err := api.PingCtx(context.Background()); err == nil {
			return nil
		}
		time.Sleep(serverPollPeriod)
	}
	return fmt.Errorf("server ping timed out after %s", timeout)
}
