package autopot

import (
	"context"
	"fmt"

	win "experimental-clicker/runner/platform/windows"
	"experimental-clicker/runner/statusui"
)

// BarReadResult is the unified HP/SP reading produced by any BarReader.
// HP and SP are 0-100 percentages. HPLow/SPLow are true when the relevant
// bar is below its threshold (for the pixel-bar reader this requires
// PotConfirmReads=3 consecutive low reads via the stabiliser; for the
// statusUI reader a single low parse suffices). Err is non-nil when the
// read failed (panel not found, parse error, capture failure, etc.).
type BarReadResult struct {
	HP    float64
	SP    float64
	HPLow bool
	SPLow bool
	Err   error
}

// BarReader produces HP/SP percentage readings. Two implementations exist:
//   - pixelBarReader — colour-based bar detection (always-available fallback)
//   - statusUIReader — OCR-based status panel reading (primary, higher precision)
//
// ReadBars blocks until a reading is available or ctx is cancelled.
// Name returns a short identifier for the overlay mode label.
type BarReader interface {
	ReadBars(ctx context.Context) BarReadResult
	Name() string
}

// pixelBarReader wraps the bar stabilisers and screen capture for
// pixel-based HP/SP reading. It is stateless — the stabilisers carry
// their own tracking state (fullLatched, lowStreak).
type pixelBarReader struct {
	hpStab *BarStabilizer
	spStab *BarStabilizer
}

func (r *pixelBarReader) Name() string { return "Pixel" }

func (r *pixelBarReader) ReadBars(ctx context.Context) BarReadResult {
	if ctx.Err() != nil {
		return BarReadResult{Err: ctx.Err()}
	}
	img, roi, err := win.CapturePlayerBarSearch()
	if err != nil {
		return BarReadResult{Err: err}
	}
	mapped, pairOK := RefreshStableBarPair(img)
	if !pairOK {
		// Pair detection failed — return an error so the orchestrator
		// retries. Don't call the stabilisers on bad data: UpdatePair
		// would call readUnknown() which resets lowStreak to 0, making
		// it impossible to accumulate the 3 low reads needed for
		// BarStatusLow. By skipping UpdatePair, the stabiliser state
		// (fullLatched, lowStreak) survives transient failures.
		// Include ROI bounds so the user can verify the search region
		// matches their screen / game UI layout.
		return BarReadResult{Err: fmt.Errorf("pixel bars not found (ROI %d,%d %dx%d)", roi.X, roi.Y, roi.W, roi.H)}
	}
	hp := r.hpStab.UpdatePair(img, true, mapped, pairOK)
	sp := r.spStab.UpdatePair(img, false, mapped, pairOK)
	return BarReadResult{
		HP:    hp.Percent,
		SP:    sp.Percent,
		HPLow: hp.Status == BarStatusLow,
		SPLow: sp.Status == BarStatusLow,
	}
}

// statusUIReader wraps the StripPoller for OCR-based HP/SP reading.
// It handles panel validation, debounced logging, overlay mode transitions,
// and the OnStatusParsed overlay callback — all as side-effects of ReadBars.
// The settings function provides access to live thresholds (which can change
// via UpdateSettings mid-run) so HPLow/SPLow are computed correctly.
type statusUIReader struct {
	poller        *statusui.StripPoller
	wasPanelFound bool
	onModeChange  func(string)
	onParsed      func(hp, hpMax, sp, spMax, stripX, stripY, stripW, stripH int)
	log           func(string)
	settings      func() AutoPotConfig
}

func (r *statusUIReader) Name() string { return "OCR" }

func (r *statusUIReader) ReadBars(ctx context.Context) BarReadResult {
	if ctx.Err() != nil {
		return BarReadResult{Err: ctx.Err()}
	}
	if r.poller.NeedsValidation() {
		if err := r.validate(); err != nil {
			return BarReadResult{Err: err}
		}
	}
	status, err := r.captureAndParse()
	if err != nil {
		// Parse failed — trigger ONE instant panel re-search before
		// giving up. Invalidate forces NeedsValidation() on the next
		// attempt, and we validate immediately so the orchestrator
		// doesn't have to switch to pixel on a single transient error.
		r.poller.Invalidate()
		if valErr := r.validate(); valErr != nil {
			return BarReadResult{Err: valErr}
		}
		status, err = r.captureAndParse()
		if err != nil {
			return BarReadResult{Err: err}
		}
	}
	hpPct := 0.0
	spPct := 0.0
	if status.HPMax > 0 {
		hpPct = float64(status.HP) * 100 / float64(status.HPMax)
	}
	if status.SPMax > 0 {
		spPct = float64(status.SP) * 100 / float64(status.SPMax)
	}
	r.notifyParsed(status)

	cfg := r.settings()
	return BarReadResult{
		HP:    hpPct,
		SP:    spPct,
		HPLow: hpPct < float64(cfg.HPThreshold),
		SPLow: spPct < float64(cfg.SPThreshold),
	}
}

// validate captures a full screenshot and runs panel detection.
// Logs failures only on state transitions (panel lost / found) to
// avoid GUI spam on repeated retries. Screen capture failures
// are logged once then suppressed until a successful capture.
func (r *statusUIReader) validate() error {
	screen, err := win.CaptureFullScreen()
	if err != nil {
		if r.wasPanelFound && r.log != nil {
			r.log(fmt.Sprintf("autopot statusui: screen capture failed: %v", err))
		}
		return err
	}
	if err := r.poller.Validate(screen); err != nil {
		if r.wasPanelFound {
			if r.log != nil {
				r.log("autopot statusui: status panel lost, searching...")
			}
			r.wasPanelFound = false
			if r.onModeChange != nil {
				r.onModeChange("Searching...")
			}
		}
		return err
	}
	if !r.wasPanelFound {
		if r.log != nil {
			r.log("autopot statusui: status panel found")
		}
		r.wasPanelFound = true
		if r.onModeChange != nil {
			r.onModeChange("OCR")
		}
	}
	return nil
}

// captureAndParse captures the cached strip region and parses HP/SP values.
func (r *statusUIReader) captureAndParse() (statusui.ParsedStatus, error) {
	strip := r.poller.StripRect()
	if strip.Empty() {
		return statusui.ParsedStatus{}, fmt.Errorf("strip rect not yet validated")
	}
	img, err := win.CaptureScreenRegion(win.ScreenROI{
		X: strip.Min.X, Y: strip.Min.Y,
		W: strip.Dx(), H: strip.Dy(),
	})
	if err != nil {
		return statusui.ParsedStatus{}, err
	}
	return r.poller.Parse(img)
}

func (r *statusUIReader) notifyParsed(s statusui.ParsedStatus) {
	if r.onParsed == nil {
		return
	}
	strip := r.poller.StripRect()
	r.onParsed(s.HP, s.HPMax, s.SP, s.SPMax, strip.Min.X, strip.Min.Y, strip.Dx(), strip.Dy())
}
