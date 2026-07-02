// Package runner's clicker runner: while physical keys are held, emit
// clicks (mouse) or key taps (keyboard). Its lifecycle is driven by
// internal/lifecycle; timing uses internal/timing; the session interface
// is internal/session.InputSession.
//
// Clicker's public UpdateSettings(slots [ClickerSlotCount]ClickerSlot) is
// preserved (matches what gui/main.go calls). Under the hood we build a
// full Config from lc.Settings() and swap in the new slots.
package runner

import (
	"context"
	"fmt"
	"time"

	windows "belarus-champ-tools/runner/platform/windows"

	"belarus-champ-tools/runner/internal/lifecycle"
	"belarus-champ-tools/runner/internal/session"
	"belarus-champ-tools/runner/internal/timing"
)

const (
	ClickerSlotCount = 2
	DefaultDelayMs   = 50
)

// ClickerSlot describes one of the two clicker rows:
//   - With mouse click (slot 0)
//   - Without mouse click — keyboard only (slot 1)
type ClickerSlot struct {
	TriggerVKs []int32
	DelayMs    int
	MouseClick bool
}

// Config holds every mutable thing the clicker loop needs.
type Config struct {
	Session session.InputSession
	Log     func(string)
	Slots   [ClickerSlotCount]ClickerSlot
}

// Runner watches trigger keys and emits clicks.
type Runner struct {
	lc *lifecycle.Lifecycle[Config]
}

// New constructs a Runner backed by a Lifecycle. The Log callback is
// defaulted to a no-op so callers don't have to.
func New(cfg Config) *Runner {
	if cfg.Log == nil {
		cfg.Log = func(string) {}
	}
	r := &Runner{}
	r.lc = lifecycle.New[Config](
		cfg,
		func(c Config) error {
			if c.Session == nil {
				return fmt.Errorf("input session is required")
			}
			return nil
		},
		nil,
	)
	return r
}

// Running reports whether the clicker loop is currently active.
func (r *Runner) Running() bool { return r.lc.Running() }

// UpdateSettings merges the new slots into the live config (preserving
// Session/Log captured at Start() time) so callers can push just the part
// of the cfg they're editing.
func (r *Runner) UpdateSettings(slots [ClickerSlotCount]ClickerSlot) {
	cfg := r.lc.Settings()
	cfg.Slots = slots
	r.lc.UpdateSettings(cfg)
}

func (r *Runner) settings() Config { return r.lc.Settings() }

// Start launches the clicker loop.
func (r *Runner) Start() error {
	if err := r.lc.Start(r.run); err != nil {
		return fmt.Errorf("clicker: %w", err)
	}
	return nil
}

// Stop signals the clicker loop to exit.
func (r *Runner) Stop() { r.lc.Stop() }

// Wait blocks until the clicker goroutine has exited.
func (r *Runner) Wait() { r.lc.Wait() }

func (r *Runner) run(ctx context.Context, cfg Config) {
	for {
		if ctx.Err() != nil {
			return
		}
		anySlot := false
		for _, slot := range r.settings().Slots {
			if len(slot.TriggerVKs) == 0 {
				continue
			}
			anySlot = true
			if r.runSlot(ctx, cfg.Session, slot) {
				return
			}
			timing.Sleep(ctx, timing.PollInterval)
		}
		if !anySlot {
			timing.Sleep(ctx, timing.CaptureRetryDelay)
		}
	}
}

// runSlot: while any trigger key is physically held, loop:
//
//	key press → [mouse click] → sleep DelayMs
func (r *Runner) runSlot(ctx context.Context, sess session.InputSession, slot ClickerSlot) bool {
	if len(slot.TriggerVKs) == 0 {
		return false
	}
	if slot.DelayMs <= 0 {
		slot.DelayMs = DefaultDelayMs
	}
	delay := time.Duration(slot.DelayMs) * time.Millisecond

	for anyKeyDown(slot.TriggerVKs) {
		if ctx.Err() != nil {
			return true
		}
		// 1. Key press
		for _, vk := range slot.TriggerVKs {
			if vk == 0 {
				continue
			}
			if err := sess.TapKey(vk, timing.KeyTapHold); err != nil {
				return false
			}
		}

		// 2. Mouse click (only slot 0)
		if slot.MouseClick {
			if err := sess.MouseDown(); err != nil {
				r.settings().Log(fmt.Sprintf("clicker mouse down failed: %v", err))
				return false
			}
			time.Sleep(delay)
			_ = sess.MouseUp()
		}

		// 3. Delay before next iteration
		timing.Sleep(ctx, delay)
	}
	return false
}

func anyKeyDown(vks []int32) bool {
	for _, vk := range vks {
		if vk != 0 && windows.PhysicalKeyDown(vk) {
			return true
		}
	}
	return false
}
