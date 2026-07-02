// Package autopot is the HP/SP auto-potion runner.
//
// Architecture:
//
//   - BarReader interface — HP/SP detectors produce percentage readings.
//     Two implementations: pixelBarReader (colour-based, always-available)
//     and statusUIReader (OCR-based, higher precision).
//
//   - AutoPotRunner.run() — orchestrator that picks the active reader,
//     reads HP/SP, and calls healUntil when values drop below thresholds.
//
//   - healUntil() — unified heal loop that presses a potion key and
//     spin-reads via the active BarReader until the stat rises above
//     threshold. Replaces two duplicate ~140-line heal functions.
//
// Lifecycle bookkeeping is in internal/lifecycle; timing constants in
// internal/timing; InputSession interface in internal/session.
package autopot

import (
	"context"
	"fmt"
	"time"

	"experimental-clicker/runner/statusui"

	"experimental-clicker/runner/internal/lifecycle"
	"experimental-clicker/runner/internal/session"
	"experimental-clicker/runner/internal/timing"
)

// AutoPotConfig is what gui/main.go passes to NewAutoPot.
type AutoPotConfig struct {
	Session        session.InputSession
	HPThreshold    int
	SPThreshold    int
	HPKeyVK        int32
	SPKeyVK        int32
	HPEnabled      bool
	SPEnabled      bool
	Log            func(string)
	OnStatusParsed func(hp, hpMax, sp, spMax, stripX, stripY, stripW, stripH int)
	OnStatusUIMode func(mode string)
}

// AutoPotRunner heals HP/SP based on readings from the active BarReader.
type AutoPotRunner struct {
	lc *lifecycle.Lifecycle[AutoPotConfig]

	hpStabilizer *BarStabilizer
	spStabilizer *BarStabilizer
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
			nil, // cleanup is handled by defer resetStabilizers() inside run()
		),
		hpStabilizer: NewBarStabilizer(true, cfg.HPThreshold),
		spStabilizer: NewBarStabilizer(false, cfg.SPThreshold),
	}
}

// Running reports whether the heal loop is currently active.
func (a *AutoPotRunner) Running() bool { return a.lc.Running() }

// UpdateSettings propagates new settings to the stabilisers.
func (a *AutoPotRunner) UpdateSettings(cfg AutoPotConfig) {
	if cfg.OnStatusUIMode == nil {
		cfg.OnStatusUIMode = a.settings().OnStatusUIMode
	}
	if cfg.OnStatusParsed == nil {
		cfg.OnStatusParsed = a.settings().OnStatusParsed
	}
	a.lc.UpdateSettings(cfg)
	a.hpStabilizer.SetThreshold(cfg.HPThreshold)
	a.spStabilizer.SetThreshold(cfg.SPThreshold)
}

// Start launches the healer.
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

func (a *AutoPotRunner) settings() AutoPotConfig { return a.lc.Settings() }

func (a *AutoPotRunner) resetStabilizers() {
	a.hpStabilizer.Reset()
	a.spStabilizer.Reset()
}

// statusUIRetryInterval is how often the pixel-bar loop probes whether
// the status UI has recovered.
const statusUIRetryInterval = 5 * time.Second

// run is the main autopot loop.
//
//  1. Try to build the OCR reader (statusUIReader). If it succeeds,
//     start in OCR mode; otherwise start in pixel-bar mode.
//  2. Each tick: read HP/SP from the active reader. If HP or SP drops
//     below its threshold, call healUntil to press the potion key and
//     wait for the value to rise.
//  3. If the OCR reader fails (panel lost, parse error), switch
//     immediately to pixel-bar. Every 30 s, probe the OCR reader and
//     switch back if it recovers.
func (a *AutoPotRunner) run(ctx context.Context, cfg AutoPotConfig) {
	defer a.resetStabilizers()

	pixel := &pixelBarReader{
		hpStab: a.hpStabilizer,
		spStab: a.spStabilizer,
		log:    cfg.Log,
	}

	pipeline, err := statusui.NewDefaultPipeline()
	hasOCR := err == nil
	var ocr *statusUIReader
	if hasOCR {
		ocr = &statusUIReader{
			poller:       statusui.NewStripPoller(pipeline),
			onModeChange: cfg.OnStatusUIMode,
			onParsed:     cfg.OnStatusParsed,
			log:          cfg.Log,
			settings:     a.settings,
		}
	}

	var reader BarReader
	if hasOCR {
		reader = ocr
		setMode(cfg.OnStatusUIMode, "Searching...")
	} else {
		reader = pixel
		// Mode label hidden — the sentinel text already shows the error.
		if cfg.OnStatusParsed != nil {
			cfg.OnStatusParsed(-1, 0, -1, 0, 0, 0, 0, 0)
		}
	}

	nextOCRRetry := time.Time{}
	loggedPixelFail := false

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cfg = a.settings()
		if cfg.Session == nil || cfg.Session.Paused() {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}

		result := reader.ReadBars(ctx)
		if result.Err != nil {
			if reader == ocr {
				cfg.Log(fmt.Sprintf("autopot: statusui issue, switching to pixel-bar: %v", result.Err))
				reader = pixel
				// -1 sentinel → overlay shows "error: Pixelsearch is used".
				if cfg.OnStatusParsed != nil {
					cfg.OnStatusParsed(-1, 0, -1, 0, 0, 0, 0, 0)
				}
				nextOCRRetry = time.Now().Add(statusUIRetryInterval)
				continue
			}
			// Pixel reader failed — retry. Log once so the user sees
			// the error but the GUI isn't flooded on repeat failures.
			if !loggedPixelFail {
				cfg.Log(fmt.Sprintf("autopot: pixel read failed: %v", result.Err))
				loggedPixelFail = true
			}
			// Even when pixel is failing, keep probing OCR recovery every 5s.
			if reader == pixel && hasOCR && time.Now().After(nextOCRRetry) {
				nextOCRRetry = time.Now().Add(statusUIRetryInterval)
				probe := ocr.ReadBars(ctx)
				if probe.Err == nil {
					cfg.Log("autopot: statusui recovered, switching back")
					reader = ocr
					setMode(cfg.OnStatusUIMode, "OCR")
					loggedPixelFail = false
					continue
				}
			}
			timing.Sleep(ctx, timing.CaptureRetryDelay)
			continue
		}

		loggedPixelFail = false

		// Periodic OCR recovery probe when on pixel reader.
		if reader == pixel && hasOCR && time.Now().After(nextOCRRetry) {
			nextOCRRetry = time.Now().Add(statusUIRetryInterval)
			probe := ocr.ReadBars(ctx)
			if probe.Err == nil {
				cfg.Log("autopot: statusui recovered, switching back")
				reader = ocr
				setMode(cfg.OnStatusUIMode, "OCR")
				result = probe
			}
		}

		if cfg.HPEnabled && result.HPLow {
			a.healUntil(ctx, reader, true)
			continue
		}
		if cfg.SPEnabled && result.SPLow {
			a.healUntil(ctx, reader, false)
			continue
		}

		timing.Sleep(ctx, timing.KeyTapHold)
	}
}

// healUntil presses the potion key and keeps pressing with minimal delay
// until the relevant stat rises above its threshold or ctx is cancelled.
//
// There is no "wait for rise" phase — the key is spammed as fast as
// the reader can keep up, since the game animation is the bottleneck.
func (a *AutoPotRunner) healUntil(ctx context.Context, reader BarReader, hpBar bool) {
	for {
		if ctx.Err() != nil {
			return
		}

		cfg := a.settings()
		if cfg.Session == nil || cfg.Session.Paused() {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}

		vk, ok := healTarget(cfg, hpBar)
		if !ok {
			return
		}

		result := reader.ReadBars(ctx)
		if result.Err != nil {
			return
		}

		pct := result.HP
		threshold := float64(cfg.HPThreshold)
		if !hpBar {
			pct = result.SP
			threshold = float64(cfg.SPThreshold)
		}
		if pct >= threshold {
			return
		}

		if err := cfg.Session.TapKey(vk, timing.KeyTapHold); err != nil {
			cfg.Log(fmt.Sprintf("Key VK_0x%02X failed: %v", vk, err))
			return
		}

		// Minimal delay — the game animation is the real bottleneck.
		timing.Sleep(ctx, timing.PollInterval)
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

func setMode(fn func(string), mode string) {
	if fn != nil {
		fn(mode)
	}
}
