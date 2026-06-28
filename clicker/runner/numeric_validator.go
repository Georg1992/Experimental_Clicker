package runner

import (
	"context"
	"sync"
	"time"
)

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

// NumericSafetyState holds the latest validated numeric HP and SP reads.
type NumericSafetyState struct {
	HP NumericResourceRead
	SP NumericResourceRead
}

// NumericSafetyValidator runs in a background goroutine to validate HP/SP
// by periodically parsing numeric text from the status window.
// It only blocks potting when it confidently knows resources are safe.
type NumericSafetyValidator struct {
	mu    sync.RWMutex
	state NumericSafetyState

	// Configuration
	pollInterval  time.Duration
	maxStateAge   time.Duration
	safetyMargin  int // percentage points above threshold to block
	minConfidence float64

	// Logging
	log func(string)
}

// NewNumericSafetyValidator creates a new numeric validator.
func NewNumericSafetyValidator() *NumericSafetyValidator {
	return &NumericSafetyValidator{
		pollInterval:  750 * time.Millisecond,
		maxStateAge:   2 * time.Second,
		safetyMargin:  4, // block if numeric is 4% above threshold
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

// SetSafetyMargin sets the percentage above threshold to trigger blocking.
func (v *NumericSafetyValidator) SetSafetyMargin(percent int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.safetyMargin = percent
}

// State returns a copy of the current numeric safety state.
func (v *NumericSafetyValidator) State() NumericSafetyState {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.state
}

// ShouldBlockHP returns true if numeric validator confidently knows HP is safe above threshold.
// Returns false if parse failed, is stale, or confidence is too low (fail-safe).
func (v *NumericSafetyValidator) ShouldBlockHP(threshold int) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.shouldBlock(v.state.HP, threshold)
}

// ShouldBlockSP returns true if numeric validator confidently knows SP is safe above threshold.
// Returns false if parse failed, is stale, or confidence is too low (fail-safe).
func (v *NumericSafetyValidator) ShouldBlockSP(threshold int) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.shouldBlock(v.state.SP, threshold)
}

// shouldBlock is the core blocking logic.
// Returns true ONLY if all conditions are met for safe blocking.
func (v *NumericSafetyValidator) shouldBlock(read NumericResourceRead, threshold int) bool {
	// Fail-safe: no data means don't block
	if !read.Found {
		return false
	}

	// Fail-safe: stale data means don't block
	if read.IsStale(v.maxStateAge) {
		return false
	}

	// Fail-safe: low confidence means don't block
	if read.Confidence < v.minConfidence {
		return false
	}

	// Block if numeric is at or above threshold + safety margin
	blockThreshold := threshold + v.safetyMargin
	return int(read.Percent) >= blockThreshold
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
