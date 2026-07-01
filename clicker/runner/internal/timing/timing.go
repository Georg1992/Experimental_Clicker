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
	KeyReleaseSettle  = 500 * time.Millisecond
	SessionCloseWait  = 10 * time.Second
	PotConfirmReads   = 3
	PotUnlatchReads   = 3
)

// PauseVK is the virtual-key code the pause watcher polls.
// "End" key (VK_END == 0x23) is used to toggle pause/resume.
const PauseVK int32 = 0x23

// DefaultAPIAddr is the default address of the embedded VIIPER API server.
const DefaultAPIAddr = "tcp://127.0.0.1:3240"

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
