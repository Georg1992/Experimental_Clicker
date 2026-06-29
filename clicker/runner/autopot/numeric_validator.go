package autopot

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	win "experimental-clicker/runner/platform/windows"
	status "experimental-clicker/runner/statusui"
)

// PotionKind represents the type of potion (HP or SP).
type PotionKind int

const (
	PotionHP PotionKind = iota
	PotionSP
)

// SafetySnapshot is the immutable safety state published by the validator.
// It contains pre-computed DoNotPot flags based on current thresholds.
type SafetySnapshot struct {
	HPPercent    float64   // Current HP percentage, or 0 if unknown
	SPPercent    float64   // Current SP percentage, or 0 if unknown
	HPConfidence float64   // HP parse confidence, or 0 if unknown
	SPConfidence float64   // SP parse confidence, or 0 if unknown
	HPDoNotPot   bool      // true = HP is safe, skip HP potion
	SPDoNotPot   bool      // true = SP is safe, skip SP potion
	HPThreshold  int       // Threshold used to compute HPDoNotPot
	SPThreshold  int       // Threshold used to compute SPDoNotPot
	UpdatedAt    time.Time // When this snapshot was published
	Found        bool      // true = at least one resource was successfully parsed
	ErrorReason  string    // Error details if parse failed
}

// IsFresh returns true if the snapshot is less than maxAge old.
func (s *SafetySnapshot) IsFresh(maxAge time.Duration) bool {
	return time.Since(s.UpdatedAt) <= maxAge
}

// Age returns the age of this snapshot in milliseconds.
func (s *SafetySnapshot) Age() int64 {
	return int64(time.Since(s.UpdatedAt).Milliseconds())
}

// NumericSafetyValidator runs in a background goroutine to continuously
// capture/parse HP/SP, compute safety flags, and publish them via atomic.Value.
// It operates independently from AutoPot and never requests anything from AutoPot.
// AutoPot reads cached safety flags before pressing potions.
type NumericSafetyValidator struct {
	// Configuration (locked only for updates, not on hot path)
	mu            sync.RWMutex
	hpThreshold   int // Threshold: HPDoNotPot = (HPPercent > hpThreshold)
	spThreshold   int // Threshold: SPDoNotPot = (SPPercent > spThreshold)
	pollInterval  time.Duration
	maxStateAge   time.Duration
	minConfidence float64
	log           func(string)

	// Atomic cache: stores *SafetySnapshot
	// Parser publishes safety flags here; AutoPot reads from here
	cachedSafety atomic.Value // *SafetySnapshot
}

// NewNumericSafetyValidator creates a new numeric validator.
func NewNumericSafetyValidator() *NumericSafetyValidator {
	v := &NumericSafetyValidator{
		hpThreshold:   30,
		spThreshold:   30,
		pollInterval:  500 * time.Millisecond,
		maxStateAge:   2 * time.Second,
		minConfidence: 0.7,
		log:           func(string) {},
	}
	// Initialize with empty safety snapshot
	v.cachedSafety.Store(&SafetySnapshot{UpdatedAt: time.Now()})
	return v
}

// SetLogFunc sets the logging function.
func (v *NumericSafetyValidator) SetLogFunc(fn func(string)) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.log = fn
}

// SetThresholds sets both HP and SP thresholds for DoNotPot calculations.
// These are used on the next parse cycle.
func (v *NumericSafetyValidator) SetThresholds(hpThreshold, spThreshold int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.hpThreshold = hpThreshold
	v.spThreshold = spThreshold
}

// SetPollInterval sets how often to capture and parse numeric data.
// Locked: run() reads v.pollInterval at start (and is the only reader of
// the ticker interval), so the setter and reader must be serialized.
func (v *NumericSafetyValidator) SetPollInterval(d time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.pollInterval = d
}

// SetMinConfidence sets the minimum confidence required to publish safety flags.
// Locked: publishSafetySnapshot() reads v.minConfidence on every publish, so
// the setter and the publish-loop reader must be serialized.
func (v *NumericSafetyValidator) SetMinConfidence(conf float64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.minConfidence = conf
}

// GetCachedSafety reads the latest published safety snapshot.
// This is the main entry point called by AutoPot before each potion keypress.
// O(1), non-blocking, fail-open: returns safety flags computed independently.
func (v *NumericSafetyValidator) GetCachedSafety() *SafetySnapshot {
	// Read cached safety (atomic, non-blocking, O(1))
	snapshot := v.cachedSafety.Load().(*SafetySnapshot)
	return snapshot
}

// State returns the raw numeric reads for diagnostics.
// Note: This is for testing/debugging; AutoPot uses GetCachedSafety.
func (v *NumericSafetyValidator) State() status.NumericRead {
	// For backward compatibility with tests; returns empty
	return status.NumericRead{}
}

// Start launches the background numeric parsing goroutine.
// The goroutine runs independently, capturing and parsing as fast as possible.
func (v *NumericSafetyValidator) Start(ctx context.Context) {
	go v.run(ctx)
}

// run is the main loop that periodically captures and parses numeric HP/SP,
// then publishes safety flags.
func (v *NumericSafetyValidator) run(ctx context.Context) {
	// Snapshot the poll interval under the lock so SetPollInterval callers
	// are serialized with the read.
	v.mu.RLock()
	pollInterval := v.pollInterval
	v.mu.RUnlock()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			v.captureAndPublishSafety()
		}
	}
}

// captureAndPublishSafety captures screen, parses HP/SP, computes safety flags, and publishes.
func (v *NumericSafetyValidator) captureAndPublishSafety() {
	// Capture screen for status window ROI
	img, _, err := win.CapturePlayerBarSearch()
	if err != nil {
		// Capture failed: publish failure state (fail-open)
		v.publishSafetySnapshot(status.NumericRead{}, false, "capture_failed")
		return
	}

	// Parse numeric HP/SP from the status window using deterministic bitmap matching
	numRead, err := status.ParseNumericResources(img)
	if err != nil {
		// Parsing failed: publish failure state (fail-open)
		v.publishSafetySnapshot(numRead, false, "parse_error")
		return
	}

	// Update timestamps
	if numRead.HP.Found {
		numRead.HP.UpdatedAt = time.Now()
	} else {
		numRead.HP.UpdatedAt = time.Now()
	}

	if numRead.SP.Found {
		numRead.SP.UpdatedAt = time.Now()
	} else {
		numRead.SP.UpdatedAt = time.Now()
	}

	// Publish safety snapshot with computed flags
	v.publishSafetySnapshot(numRead, true, "")
}

// publishSafetySnapshot computes safety flags and publishes immutable snapshot.
func (v *NumericSafetyValidator) publishSafetySnapshot(state status.NumericRead, found bool, errorReason string) {
	// Read all config fields under one RLock so the locked setters
	// (SetThresholds, SetLogFunc, SetPollInterval, SetMinConfidence)
	// are serialized with these reads.
	v.mu.RLock()
	hpThreshold := v.hpThreshold
	spThreshold := v.spThreshold
	maxStateAge := v.maxStateAge
	minConfidence := v.minConfidence
	v.mu.RUnlock()

	// Build immutable snapshot locally
	snapshot := &SafetySnapshot{
		HPPercent:    state.HP.Percent,
		SPPercent:    state.SP.Percent,
		HPConfidence: state.HP.Confidence,
		SPConfidence: state.SP.Confidence,
		HPThreshold:  hpThreshold,
		SPThreshold:  spThreshold,
		UpdatedAt:    time.Now(),
		Found:        found,
		ErrorReason:  errorReason,
	}

	// Compute HPDoNotPot: true only if HP found, fresh, confident, and above threshold
	if state.HP.Found && !state.HP.IsStale(maxStateAge) && state.HP.Confidence >= minConfidence && state.HP.Max > 0 {
		snapshot.HPDoNotPot = state.HP.Percent > float64(hpThreshold)
	} else {
		snapshot.HPDoNotPot = false
	}

	// Compute SPDoNotPot: true only if SP found, fresh, confident, and above threshold
	if state.SP.Found && !state.SP.IsStale(maxStateAge) && state.SP.Confidence >= minConfidence && state.SP.Max > 0 {
		snapshot.SPDoNotPot = state.SP.Percent > float64(spThreshold)
	} else {
		snapshot.SPDoNotPot = false
	}

	// Atomically publish the new snapshot
	// AutoPot reads this without waiting
	v.cachedSafety.Store(snapshot)
}
