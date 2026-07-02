// Package timing holds the canonical timing constants and the cancellable
// Sleep helper used by every runner. Centralizing here avoids the
// duplication that existed across runner/timing.go and runner/autopot/.
package timing

import (
	"context"
	"time"
)

const (
	PollInterval      = 10 * time.Millisecond
	CaptureRetryDelay = 50 * time.Millisecond
	KeyTapHold        = 1 * time.Millisecond
	KeyBindTimeout    = 5 * time.Second
	SessionCloseWait  = 10 * time.Second
)

// ToggleVK is the virtual-key code for the stop/start toggle watcher.
// "End" key (VK_END == 0x23) toggles the app between running and stopped.
const ToggleVK int32 = 0x23

// DefaultAPIAddr is the default address of the embedded VIIPER API server.
// Port 3242 verified at runtime (2026-07-02): "API listening addr=[::]:3242".
// Format is host:port — viiperclient passes this directly to net.Dial("tcp", addr).
const DefaultAPIAddr = "127.0.0.1:3242"

// Sleep sleeps for d, returning early if ctx is canceled.
func Sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		select {
		case <-ctx.Done():
		default:
		}
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
