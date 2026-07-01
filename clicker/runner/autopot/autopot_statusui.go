package autopot

import (
	"context"
	"fmt"
	"time"

	win "experimental-clicker/runner/platform/windows"
	"experimental-clicker/runner/internal/timing"
	"experimental-clicker/runner/statusui"
)

// useStatusUIPot switches the autopot heal loop to the statusui OCR-based
// reader instead of the pixel-bar path. Flip to false to restore the
// original bar reader.
const useStatusUIPot = true

// statusUIPollInterval is how often the strip is captured and parsed.
const statusUIPollInterval = 100 * time.Millisecond

// runStatusUI is the statusui-backed heal loop. It is called from run()
// when useStatusUIPot is true and returns when ctx is cancelled.
func (a *AutoPotRunner) runStatusUI(ctx context.Context, startCfg AutoPotConfig) {
	pipeline, err := statusui.NewDefaultPipeline()
	if err != nil {
		startCfg.Log(fmt.Sprintf("autopot statusui: cannot init pipeline: %v", err))
		return
	}
	poller := statusui.NewStripPoller(pipeline)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cfg := a.settings()
		sess := cfg.Session
		if sess == nil || sess.Paused() {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}

		if poller.NeedsValidation() {
			screen, capErr := win.CaptureFullScreen()
			if capErr != nil {
				timing.Sleep(ctx, timing.CaptureRetryDelay)
				continue
			}
			if valErr := poller.Validate(screen); valErr != nil {
				cfg.Log(fmt.Sprintf("autopot statusui: panel not found: %v", valErr))
				timing.Sleep(ctx, timing.CaptureRetryDelay)
				continue
			}
		}

		status, parseErr := captureAndParse(poller)
		if parseErr != nil {
			timing.Sleep(ctx, statusUIPollInterval)
			continue
		}

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

		status, err := revalidateAndParse(ctx, poller)
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
			status, err = revalidateAndParse(ctx, poller)
			if err != nil {
				continue
			}
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

// revalidateAndParse runs a full screen re-acquisition if the poller needs
// it, then captures the strip and parses it.
func revalidateAndParse(ctx context.Context, poller *statusui.StripPoller) (statusui.ParsedStatus, error) {
	if poller.NeedsValidation() {
		screen, err := win.CaptureFullScreen()
		if err != nil {
			return statusui.ParsedStatus{}, err
		}
		if err := poller.Validate(screen); err != nil {
			return statusui.ParsedStatus{}, err
		}
		_ = ctx // keep signature consistent with callers that may add ctx use later
	}
	return captureAndParse(poller)
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
