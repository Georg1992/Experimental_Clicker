package runner

import (
	"context"
	"sync"
	"time"
)

// PotionKind represents the type of potion (HP or SP).
type PotionKind int

const (
	PotionHP PotionKind = iota
	PotionSP
)

// BlockDecision is returned by ShouldBlockPotion to indicate whether to block a potion press.
type BlockDecision struct {
	Block      bool    // true = skip potion press, false = allow potion press
	Reason     string  // explanation of the decision
	Percent    float64 // numeric resource percentage (0-100), or 0 if not available
	Confidence float64 // parse confidence (0.0-1.0), or 0 if not available
	AgeMs      int64   // age of the numeric read in milliseconds
}

// NumericResourceRead holds the result of parsing numeric HP/SP from the status window.
type NumericResourceRead struct {
	Found      bool
	Current    int
	Max        int
	Percent    float64
	UpdatedAt  time.Time
	Confidence float64 // 0.0 to 1.0
}

// IsStale returns true if the read is older than the given duration.
func (r *NumericResourceRead) IsStale(maxAge time.Duration) bool {
	return time.Since(r.UpdatedAt) > maxAge
}

// Age returns the age of this read in milliseconds.
func (r *NumericResourceRead) Age() int64 {
	return int64(time.Since(r.UpdatedAt).Milliseconds())
}

// NumericSafetyState holds the latest validated numeric HP and SP reads.
type NumericSafetyState struct {
	HP NumericResourceRead
	SP NumericResourceRead
}

// NumericSafetyValidator runs in a background goroutine to validate HP/SP
// by periodically parsing numeric text from the status window.
// It acts as a NEGATIVE-ONLY gate: only blocks potion presses when confident
// that the resource is safe (above threshold), never triggers potting.
type NumericSafetyValidator struct {
	mu    sync.RWMutex
	state NumericSafetyState

	// Configuration
	pollInterval  time.Duration
	maxStateAge   time.Duration
	minConfidence float64

	// Logging
	log func(string)
}

// NewNumericSafetyValidator creates a new numeric validator.
func NewNumericSafetyValidator() *NumericSafetyValidator {
	return &NumericSafetyValidator{
		pollInterval:  750 * time.Millisecond,
		maxStateAge:   2 * time.Second,
		minConfidence: 0.7,
		log:           func(string) {},
	}
}

// SetLogFunc sets the logging function.
func (v *NumericSafetyValidator) SetLogFunc(fn func(string)) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.log = fn
}

// SetPollInterval sets how often to capture and parse numeric data.
func (v *NumericSafetyValidator) SetPollInterval(d time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.pollInterval = d
}

// SetMinConfidence sets the minimum confidence required to block a potion.
func (v *NumericSafetyValidator) SetMinConfidence(conf float64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.minConfidence = conf
}

// State returns a copy of the current numeric safety state.
func (v *NumericSafetyValidator) State() NumericSafetyState {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.state
}

// ShouldBlockPotion determines whether a potion press should be blocked.
// This is the main entry point called by autopot before each potion keypress.
// Returns a BlockDecision with Block=true ONLY if numeric validator is confident
// that the resource is above the threshold. Otherwise returns Block=false (fail-safe).
func (v *NumericSafetyValidator) ShouldBlockPotion(kind PotionKind, thresholdPercent int) BlockDecision {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var read NumericResourceRead
	if kind == PotionHP {
		read = v.state.HP
	} else {
		read = v.state.SP
	}

	// Fail-safe: no parse data means allow potting
	if !read.Found {
		return BlockDecision{
			Block:  false,
			Reason: "numeric_parse_not_found",
			AgeMs:  read.Age(),
		}
	}

	// Fail-safe: stale data means allow potting
	if read.IsStale(v.maxStateAge) {
		return BlockDecision{
			Block:  false,
			Reason: "numeric_parse_stale",
			Percent: read.Percent,
			Confidence: read.Confidence,
			AgeMs:  read.Age(),
		}
	}

	// Fail-safe: low confidence means allow potting
	if read.Confidence < v.minConfidence {
		return BlockDecision{
			Block:  false,
			Reason: "numeric_confidence_low",
			Percent: read.Percent,
			Confidence: read.Confidence,
			AgeMs:  read.Age(),
		}
	}

	// Fail-safe: invalid max value means allow potting
	if read.Max <= 0 {
		return BlockDecision{
			Block:  false,
			Reason: "numeric_invalid_max",
			Percent: read.Percent,
			Confidence: read.Confidence,
			AgeMs:  read.Age(),
		}
	}

	// Core blocking decision:
	// Block ONLY if numeric percent is ABOVE threshold (resource is safe, don't need potion)
	if read.Percent > float64(thresholdPercent) {
		return BlockDecision{
			Block:      true,
			Reason:     "numeric_above_threshold",
			Percent:    read.Percent,
			Confidence: read.Confidence,
			AgeMs:      read.Age(),
		}
	}

	// Resource is at or below threshold, allow autopot to pot normally
	return BlockDecision{
		Block:      false,
		Reason:     "numeric_below_threshold",
		Percent:    read.Percent,
		Confidence: read.Confidence,
		AgeMs:      read.Age(),
	}
}

// Start launches the background numeric parsing goroutine.
func (v *NumericSafetyValidator) Start(ctx context.Context) {
	go v.run(ctx)
}

// run is the main loop that periodically captures and parses numeric HP/SP.
func (v *NumericSafetyValidator) run(ctx context.Context) {
	ticker := time.NewTicker(v.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			v.captureAndParse()
		}
	}
}

// captureAndParse captures the status window and parses numeric HP/SP using the deterministic parser.
func (v *NumericSafetyValidator) captureAndParse() {
	// Capture screen for status window ROI
	img, _, err := CapturePlayerBarSearch()
	if err != nil {
		return
	}

	// Parse numeric HP/SP from the status window using deterministic bitmap matching
	numRead, err := ParseNumericResources(img)
	if err != nil {
		// Parsing failed - update state with empty reads
		v.mu.Lock()
		v.state = NumericSafetyState{}
		v.mu.Unlock()
		return
	}

	// Update HP with timestamp and validator settings
	if numRead.HP.Found {
		numRead.HP.UpdatedAt = time.Now()
		// Confidence is already set by parser, but ensure it meets validator's threshold
		if numRead.HP.Confidence < v.minConfidence {
			numRead.HP.Found = false
		}
	} else {
		numRead.HP.UpdatedAt = time.Now()
	}

	// Update SP with timestamp and validator settings
	if numRead.SP.Found {
		numRead.SP.UpdatedAt = time.Now()
		// Confidence is already set by parser
		if numRead.SP.Confidence < v.minConfidence {
			numRead.SP.Found = false
		}
	} else {
		numRead.SP.UpdatedAt = time.Now()
	}

	v.mu.Lock()
	v.state = NumericSafetyState{HP: numRead.HP, SP: numRead.SP}
	v.mu.Unlock()
}

