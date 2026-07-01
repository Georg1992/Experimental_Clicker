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
	"strings"
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

	// maxCapturedLines is how many stdout/stderr lines we keep for
	// diagnostics when the server fails to respond.
	maxCapturedLines = 24
)

var (
	serverMu      sync.Mutex
	serverCmd     *exec.Cmd
	serverStarted bool
	serverPID     int
	viiperTempDir string
)

// ---------------------------------------------------------------------------
// ring buffer for captured viiper output
// ---------------------------------------------------------------------------

type outputRing struct {
	mu    sync.Mutex
	lines []string
	pos   int
	full  bool
}

func newOutputRing(n int) *outputRing { return &outputRing{lines: make([]string, n)} }

func (r *outputRing) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines[r.pos] = line
	r.pos++
	if r.pos >= len(r.lines) {
		r.pos = 0
		r.full = true
	}
}

// tail returns the last lines in order. If fewer than maxCapturedLines were
// captured it returns all of them.
func (r *outputRing) tail() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return append([]string(nil), r.lines[:r.pos]...)
	}
	out := make([]string, len(r.lines))
	n := copy(out, r.lines[r.pos:])
	copy(out[n:], r.lines[:r.pos])
	return out
}

// ---------------------------------------------------------------------------
// main entry point
// ---------------------------------------------------------------------------

func ensureViiperServer(ctx context.Context, log func(string)) (started bool, err error) {
	serverMu.Lock()
	defer serverMu.Unlock()

	addr := runner.DefaultAPIAddr
	log(fmt.Sprintf("VIIPER addr: %s", addr))

	// Quick ping with a short timeout — don't hold serverMu while a
	// stale listener forces TCP to retransmit for minutes.
	pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
	defer pingCancel()
	api := viiperclient.New(addr)
	if _, err := api.PingCtx(pingCtx); err == nil {
		log("VIIPER server already running on " + addr)
		return false, nil
	}
	log("No existing VIIPER server found")

	path, dir, err := extractViiper()
	if err != nil {
		return false, err
	}
	viiperTempDir = dir

	log(fmt.Sprintf("Launching: %s server", path))
	log(fmt.Sprintf("Working dir: inherited from parent process"))
	log("Env: inherited from parent (set VIIPER_API_ADDR to override port)")

	cmd := exec.Command(path, "server")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}

	ring := newOutputRing(maxCapturedLines)

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
	log(fmt.Sprintf("VIIPER PID: %d", serverPID))

	go forwardOutput(stdout, ring, log, "viiper")
	go forwardOutput(stderr, ring, log, "viiper")

	log(fmt.Sprintf("Waiting for ping on %s (up to %s)...", addr, serverWaitTime))
	if err := waitForServer(ctx, addr, serverWaitTime, log); err != nil {
		killProcessTree(serverPID)
		_, _ = cmd.Process.Wait() // populate cmd.ProcessState for diagnostics
		dumpViiperDiagnostics(cmd, ring, addr, log)
		serverPID = 0
		removeViiperTempDirPath(viiperTempDir)
		viiperTempDir = ""
		return false, err
	}

	serverCmd = cmd
	serverStarted = true
	log("VIIPER ping OK")
	return true, nil
}

// ---------------------------------------------------------------------------
// output capture
// ---------------------------------------------------------------------------

// forwardOutput reads lines from r line-by-line, appends each to ring, and
// forwards every line to log with the given prefix (e.g. "[viiper] line").
func forwardOutput(r io.Reader, ring *outputRing, log func(string), prefix string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		ring.add(line)
		log(fmt.Sprintf("[%s] %s", prefix, line))
	}
}

// ---------------------------------------------------------------------------
// ping loop
// ---------------------------------------------------------------------------

func waitForServer(ctx context.Context, addr string, timeout time.Duration, log func(string)) error {
	deadline := time.Now().Add(timeout)
	api := viiperclient.New(addr)
	attempt := 0

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		attempt++
		pingCtx, pingCancel := context.WithTimeout(ctx, 1*time.Second)
		_, err := api.PingCtx(pingCtx)
		pingCancel()

		if err == nil {
			return nil
		}

		// Log every 10th failed attempt so the user sees activity.
		if attempt%10 == 0 {
			log(fmt.Sprintf("ping attempt %d: %v", attempt, err))
		}

		time.Sleep(serverPollPeriod)
	}
	return fmt.Errorf("server ping timed out after %s (%d attempts)", timeout, attempt)
}

// ---------------------------------------------------------------------------
// timeout diagnostics
// ---------------------------------------------------------------------------

func dumpViiperDiagnostics(cmd *exec.Cmd, ring *outputRing, addr string, log func(string)) {
	log("--- VIIPER startup diagnostics ---")

	// 1. Process status.
	if cmd.ProcessState != nil {
		log(fmt.Sprintf("Process exited with code %d", cmd.ProcessState.ExitCode()))
	} else if cmd.Process != nil {
		// On Windows we can check the exit code without Wait by
		// polling GetExitCodeProcess, but a simpler approach is to
		// call Wait in a non-blocking way. Since we already know
		// pings failed, the process is either running or dead.
		log("Process still alive (no exit code)")
	} else {
		log("No process handle")
	}

	// 2. Tested address.
	log(fmt.Sprintf("Tested address: %s", addr))

	// 3. Last captured output.
	lines := ring.tail()
	if len(lines) == 0 {
		log("No stdout/stderr captured from viiper.exe")
	} else {
		log(fmt.Sprintf("Last %d viiper lines:", len(lines)))
		for _, l := range lines {
			log(fmt.Sprintf("  | %s", l))
		}
	}

	// 4. Suggested actions.
	var suggestions []string
	if cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		suggestions = append(suggestions, fmt.Sprintf("viiper.exe exited with code %d — check [viiper] logs above for crash details", code))
	}
	if len(lines) == 0 {
		suggestions = append(suggestions, "No output captured — viiper.exe may have failed to start (missing DLL, permissions, or antivirus block)")
	}
	suggestions = append(suggestions, "Run '"+filepath.Join(viiperTempDir, "viiper.exe")+" server' in a terminal to see startup output directly")
	suggestions = append(suggestions, "Check Windows Firewall — port "+stripPort(addr)+" must be allowed for localhost TCP")
	suggestions = append(suggestions, "If port is wrong, update DefaultAPIAddr in runner/internal/timing/timing.go")

	log("Suggested actions:")
	for _, s := range suggestions {
		log("  → " + s)
	}
	log("--- end diagnostics ---")
}

// stripPort extracts the port number from an address like "tcp://127.0.0.1:3240".
func stripPort(addr string) string {
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[idx+1:]
	}
	return addr
}

// ---------------------------------------------------------------------------
// process lifecycle
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

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
