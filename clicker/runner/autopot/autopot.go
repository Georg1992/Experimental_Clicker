// Package autopot is the HP/SP auto-potion runner.
//
// Lifecycle bookkeeping (Start/Stop/Wait/Running/UpdateSettings) lives in
// internal/lifecycle; timing constants in internal/timing; the
// InputSession interface in internal/session. autopot does not import the
// parent runner package (to keep the import graph cycle-free) so it
// composes Lifecycle, session.InputSession, and timing.* from internal/.
package autopot

import (
	"context"
	"fmt"

	win "experimental-clicker/runner/platform/windows"

	"experimental-clicker/runner/internal/lifecycle"
	"experimental-clicker/runner/internal/session"
	"experimental-clicker/runner/internal/timing"
)

// AutoPotConfig is what gui/main.go passes to NewAutoPot.
type AutoPotConfig struct {
	Session     session.InputSession
	HPThreshold int
	SPThreshold int
	HPKeyVK     int32
	SPKeyVK     int32
	HPEnabled   bool
	SPEnabled   bool
	Log         func(string)
	// OnStatusParsed is called from the statusui loop after each successful
	// HP/SP parse. stripX/Y/W/H are the screen-space coordinates of the text
	// strip used to read the values. May be nil.
	OnStatusParsed func(hp, hpMax, sp, spMax, stripX, stripY, stripW, stripH int)
}

// AutoPotRunner heals HP/SP based on bar-fill reading. Embeds a Lifecycle so
// the goroutine bookkeeping isn't reimplemented.
type AutoPotRunner struct {
	lc *lifecycle.Lifecycle[AutoPotConfig]

	hpStabilizer *BarStabilizer
	spStabilizer *BarStabilizer

	// wasPanelFound tracks whether the status panel was successfully
	// located at least once. Used by validateWithLog to debounce log
	// messages: failures are only logged on a state transition
	// (found→lost, lost→found), not on every retry.
	wasPanelFound bool
}

// NewAutoPot constructs an AutoPotRunner with the given initial config.
func NewAutoPot(cfg AutoPotConfig) *AutoPotRunner {
	return &AutoPotRunner{
		lc: lifecycle.New(
			cfg,
			func(c AutoPotConfig) error {
				if c.Session == nil {
					return fmt.Errorf("input session is required")
				}
				if c.Log == nil {
					return fmt.Errorf("log callback is required")
				}
				if c.HPEnabled && c.HPKeyVK == 0 {
					return fmt.Errorf("HP potion key is not set")
				}
				if c.SPEnabled && c.SPKeyVK == 0 {
					return fmt.Errorf("SP potion key is not set")
				}
				return nil
			},
			func(c AutoPotConfig) {
				// On stop, reset stabilizers so a future Start begins clean.
				_ = c // stabilizer.Reset is on the runner; called in Stop hook below
			},
		),
		hpStabilizer: NewBarStabilizer(true, cfg.HPThreshold),
		spStabilizer: NewBarStabilizer(false, cfg.SPThreshold),
	}
}

// Running reports whether the heal loop is currently active.
func (a *AutoPotRunner) Running() bool { return a.lc.Running() }

// UpdateSettings propagates new settings to the stabilizers.
// Settings applied after Start() take effect on the next poll.
func (a *AutoPotRunner) UpdateSettings(cfg AutoPotConfig) {
	a.lc.UpdateSettings(cfg)
	a.hpStabilizer.SetThreshold(cfg.HPThreshold)
	a.spStabilizer.SetThreshold(cfg.SPThreshold)
}

// Start launches the healer. Returns an error if validation fails or the
// runner is already active.
func (a *AutoPotRunner) Start() error {
	if err := a.lc.Start(a.run); err != nil {
		return fmt.Errorf("autopot: %w", err)
	}
	return nil
}

// Stop signals the healer to exit.
func (a *AutoPotRunner) Stop() { a.lc.Stop() }

// Wait blocks until the healer goroutine has exited.
func (a *AutoPotRunner) Wait() { a.lc.Wait() }

// settings returns a snapshot of the live config.
func (a *AutoPotRunner) settings() AutoPotConfig { return a.lc.Settings() }

// resetStabilizers is called after a Stop completes (or on Start).
func (a *AutoPotRunner) resetStabilizers() {
	a.hpStabilizer.Reset()
	a.spStabilizer.Reset()
	a.wasPanelFound = false
}

func (a *AutoPotRunner) run(ctx context.Context, cfg AutoPotConfig) {
	defer a.resetStabilizers()

	// Try the statusui OCR-based reader first. If the pipeline fails to
	// initialise (e.g. missing glyphs, screen resolution issues), fall
	// back to the pixel-bar reader transparently.
	if err := a.runStatusUI(ctx, cfg); err != nil {
		cfg.Log(fmt.Sprintf("autopot: statusui unavailable, falling back to pixel-bar: %v", err))
	} else {
		return // normal Stop via ctx cancel
	}

	// Pixel-bar fallback path.
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		cfg := a.settings()
		session := cfg.Session
		if session == nil || session.Paused() {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}

		img, _, err := win.CapturePlayerBarSearch()
		if err != nil {
			timing.Sleep(ctx, timing.CaptureRetryDelay)
			continue
		}

		mapped, pairOK := RefreshStableBarPair(img)

		hp := a.hpStabilizer.UpdatePair(img, true, mapped, pairOK)
		if cfg.HPEnabled && hp.Status == BarStatusLow {
			a.healUntil(ctx, session, true)
			continue
		}

		sp := a.spStabilizer.UpdatePair(img, false, mapped, pairOK)
		if cfg.SPEnabled && sp.Status == BarStatusLow {
			a.healUntil(ctx, session, false)
			continue
		}

		timing.Sleep(ctx, timing.KeyTapHold)
	}
}

func (a *AutoPotRunner) healUntil(ctx context.Context, session session.InputSession, hpBar bool) {
	stabilizer := a.spStabilizer
	if hpBar {
		stabilizer = a.hpStabilizer
	}

	for {
		if ctx.Err() != nil {
			return
		}
		if session.Paused() {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}
		cfg := a.settings()
		vk, ok := healTarget(cfg, hpBar)
		if !ok {
			return
		}

		img, _, err := win.CapturePlayerBarSearch()
		if err != nil {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}
		mapped, pairOK := RefreshStableBarPair(img)
		read := stabilizer.UpdatePair(img, hpBar, mapped, pairOK)
		if read.Status != BarStatusLow {
			return
		}
		before := read.Percent

		if err := session.TapKey(vk, timing.KeyTapHold); err != nil {
			cfg.Log(fmt.Sprintf("Key VK_0x%02X failed: %v", vk, err))
			return
		}
		for {
			if ctx.Err() != nil {
				return
			}
			if session.Paused() {
				timing.Sleep(ctx, timing.PollInterval)
				continue
			}
			cfg = a.settings()
			if _, ok := healTarget(cfg, hpBar); !ok {
				return
			}
			img, _, err := win.CapturePlayerBarSearch()
			if err != nil {
				continue
			}
			mapped, pairOK := RefreshStableBarPair(img)
			read := stabilizer.UpdatePair(img, hpBar, mapped, pairOK)
			if read.Status != BarStatusLow {
				return
			}
			if read.Percent > before {
				break
			}
		}
	}
}

func healTarget(cfg AutoPotConfig, hpBar bool) (vk int32, ok bool) {
	if hpBar {
		if !cfg.HPEnabled || cfg.HPKeyVK == 0 {
			return 0, false
		}
		return cfg.HPKeyVK, true
	}
	if !cfg.SPEnabled || cfg.SPKeyVK == 0 {
		return 0, false
	}
	return cfg.SPKeyVK, true
}
