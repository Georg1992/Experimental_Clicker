package autopot

import (
	"context"
	"fmt"
	"time"

	"experimental-clicker/runner/internal/timing"
	win "experimental-clicker/runner/platform/windows"
	"experimental-clicker/runner/statusui"
)

// statusUIPollInterval is how often the strip is captured and parsed.
const statusUIPollInterval = 100 * time.Millisecond

// runStatusUI is the statusui OCR-based heal loop. It returns nil when
// ctx is cancelled (normal Stop). It returns a non-nil error when the
// pipeline cannot be initialised, or when OCR fails too many times in a
// row at runtime — the caller should fall back to the pixel-bar reader.
func (a *AutoPotRunner) runStatusUI(ctx context.Context, _ AutoPotConfig) error {
	// Respect cancellation immediately so a Stop click during pipeline
	// init returns nil (normal Stop) rather than a fallback error.
	if ctx.Err() != nil {
		return nil
	}

	pipeline, err := statusui.NewDefaultPipeline()
	if err != nil {
		return fmt.Errorf("cannot init pipeline: %v", err)
	}
	poller := statusui.NewStripPoller(pipeline)

	// If OCR validation or parsing fails this many times in a row,
	// give up and let the caller fall back to the pixel-bar reader.
	const maxConsecutiveFails = 10
	consecutiveFails := 0

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		cfg := a.settings()
		sess := cfg.Session
		if sess == nil || sess.Paused() {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}

		if poller.NeedsValidation() {
			if err := a.validateWithLog(poller, cfg.Log); err != nil {
				consecutiveFails++
				if consecutiveFails >= maxConsecutiveFails {
					return fmt.Errorf("status panel detection failed %d times", consecutiveFails)
				}
				timing.Sleep(ctx, timing.CaptureRetryDelay)
				continue
			}
		}

		status, parseErr := captureAndParse(poller)
		if parseErr != nil {
			consecutiveFails++
			if consecutiveFails >= maxConsecutiveFails {
				return fmt.Errorf("status OCR parse failed %d times", consecutiveFails)
			}
			timing.Sleep(ctx, statusUIPollInterval)
			continue
		}
		consecutiveFails = 0 // reset on success
		notifyStatus(cfg, poller, status)

		if cfg.HPEnabled && status.HPMax > 0 && status.HP*100/status.HPMax < cfg.HPThreshold {
			a.healUntilStatusUI(ctx, poller, true)
			continue
		}
		if cfg.SPEnabled && status.SPMax > 0 && status.SP*100/status.SPMax < cfg.SPThreshold {
			a.healUntilStatusUI(ctx, poller, false)
			continue
		}

		timing.Sleep(ctx, statusUIPollInterval)
	}
}

// healUntilStatusUI presses the potion key and waits until the relevant
// stat rises above the configured threshold, mirroring the behaviour of
// the pixel-bar healUntil.
func (a *AutoPotRunner) healUntilStatusUI(ctx context.Context, poller *statusui.StripPoller, hpBar bool) {
	for {
		if ctx.Err() != nil {
			return
		}

		cfg := a.settings()
		sess := cfg.Session
		if sess == nil || sess.Paused() {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}

		vk, ok := healTarget(cfg, hpBar)
		if !ok {
			return
		}

		if poller.NeedsValidation() {
			if err := a.validateWithLog(poller, cfg.Log); err != nil {
				timing.Sleep(ctx, statusUIPollInterval)
				continue
			}
		}
		status, err := captureAndParse(poller)
		if err != nil {
			timing.Sleep(ctx, statusUIPollInterval)
			continue
		}

		pct, threshold := statPercent(status, cfg, hpBar)
		if pct >= threshold {
			return
		}
		before := currentVal(status, hpBar)

		if tapErr := sess.TapKey(vk, timing.KeyTapHold); tapErr != nil {
			cfg.Log(fmt.Sprintf("Key VK_0x%02X failed: %v", vk, tapErr))
			return
		}

		// Wait for value to rise above the pre-press reading.
		for {
			if ctx.Err() != nil {
				return
			}
			cfg = a.settings()
			if _, ok := healTarget(cfg, hpBar); !ok {
				return
			}
			if poller.NeedsValidation() {
				_ = a.validateWithLog(poller, cfg.Log)
			}
			status, err = captureAndParse(poller)
			if err != nil {
				continue
			}
			notifyStatus(cfg, poller, status)
			pct, threshold = statPercent(status, cfg, hpBar)
			if pct >= threshold {
				return
			}
			if currentVal(status, hpBar) > before {
				break
			}
		}
	}
}

// validateWithLog captures a full screenshot, runs panel validation, and
// logs either a detection success or failure message via log.
func (a *AutoPotRunner) validateWithLog(poller *statusui.StripPoller, log func(string)) error {
	screen, err := win.CaptureFullScreen()
	if err != nil {
		log(fmt.Sprintf("autopot statusui: screen capture failed: %v", err))
		return err
	}
	if err := poller.Validate(screen); err != nil {
		log(fmt.Sprintf("autopot statusui: failed to detect status panel: %v", err))
		return err
	}
	r := poller.StripRect()
	log(fmt.Sprintf("autopot statusui: status panel detected, strip at (%d,%d)-(%d,%d)",
		r.Min.X, r.Min.Y, r.Max.X, r.Max.Y))
	return nil
}

// captureAndParse captures the cached strip region and parses HP/SP values.
func captureAndParse(poller *statusui.StripPoller) (statusui.ParsedStatus, error) {
	r := poller.StripRect()
	strip, err := win.CaptureScreenRegion(win.ScreenROI{X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy()})
	if err != nil {
		return statusui.ParsedStatus{}, err
	}
	return poller.Parse(strip)
}

// statPercent returns (current%, threshold) for the relevant stat.
func statPercent(s statusui.ParsedStatus, cfg AutoPotConfig, hpBar bool) (pct, threshold int) {
	if hpBar {
		if s.HPMax > 0 {
			pct = s.HP * 100 / s.HPMax
		}
		return pct, cfg.HPThreshold
	}
	if s.SPMax > 0 {
		pct = s.SP * 100 / s.SPMax
	}
	return pct, cfg.SPThreshold
}

// currentVal returns HP or SP depending on hpBar.
func currentVal(s statusui.ParsedStatus, hpBar bool) int {
	if hpBar {
		return s.HP
	}
	return s.SP
}

// notifyStatus fires cfg.OnStatusParsed if set, passing parsed values and
// the strip's screen-space rectangle so the caller can position an overlay.
func notifyStatus(cfg AutoPotConfig, poller *statusui.StripPoller, s statusui.ParsedStatus) {
	if cfg.OnStatusParsed == nil {
		return
	}
	r := poller.StripRect()
	cfg.OnStatusParsed(s.HP, s.HPMax, s.SP, s.SPMax, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}
