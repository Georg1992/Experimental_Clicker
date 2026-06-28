package runner

import (
	"context"
	"testing"
	"time"
)

// TestShouldBlockPotion_BlocksWhenAboveThreshold tests that block occurs when resource is above threshold.
func TestShouldBlockPotion_BlocksWhenAboveThreshold(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Simulate numeric read: HP at 97%
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    97,
		Max:        100,
		Percent:    97.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// Threshold is 30%, numeric is 97% -> BLOCK (resource is safe, don't need potion)
	decision := v.ShouldBlockPotion(PotionHP, 30)
	if !decision.Block {
		t.Errorf("Expected Block=true when HP=97%% and threshold=30%%, got Block=%v", decision.Block)
	}
	if decision.Reason != "numeric_above_threshold" {
		t.Errorf("Expected Reason=numeric_above_threshold, got %q", decision.Reason)
	}
}

// TestShouldBlockPotion_AllowsWhenBelowThreshold tests that potion is allowed when resource is at/below threshold.
func TestShouldBlockPotion_AllowsWhenBelowThreshold(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Simulate numeric read: HP at 25%
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    25,
		Max:        100,
		Percent:    25.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// Threshold is 30%, numeric is 25% -> ALLOW (resource needs potion)
	decision := v.ShouldBlockPotion(PotionHP, 30)
	if decision.Block {
		t.Errorf("Expected Block=false when HP=25%% and threshold=30%%, got Block=%v", decision.Block)
	}
	if decision.Reason != "numeric_below_threshold" {
		t.Errorf("Expected Reason=numeric_below_threshold, got %q", decision.Reason)
	}
}

// TestShouldBlockPotion_AllowsWhenParserFailed tests fail-safe: allow when parse not found.
func TestShouldBlockPotion_AllowsWhenParserFailed(t *testing.T) {
	v := NewNumericSafetyValidator()
	// No numeric data set

	decision := v.ShouldBlockPotion(PotionHP, 30)
	if decision.Block {
		t.Errorf("Expected Block=false when parser failed, got Block=%v", decision.Block)
	}
	if decision.Reason != "numeric_parse_not_found" {
		t.Errorf("Expected Reason=numeric_parse_not_found, got %q", decision.Reason)
	}
}

// TestShouldBlockPotion_AllowsWhenStale tests fail-safe: allow when numeric data is stale.
func TestShouldBlockPotion_AllowsWhenStale(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Simulate old numeric read (5 seconds old, max age is 2 seconds)
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    95,
		Max:        100,
		Percent:    95.0,
		UpdatedAt:  time.Now().Add(-5 * time.Second),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// Even though numeric says 95%, it's stale -> ALLOW (don't rely on stale data)
	decision := v.ShouldBlockPotion(PotionHP, 30)
	if decision.Block {
		t.Errorf("Expected Block=false when data is stale, got Block=%v", decision.Block)
	}
	if decision.Reason != "numeric_parse_stale" {
		t.Errorf("Expected Reason=numeric_parse_stale, got %q", decision.Reason)
	}
}

// TestShouldBlockPotion_AllowsWhenLowConfidence tests fail-safe: allow when confidence is low.
func TestShouldBlockPotion_AllowsWhenLowConfidence(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Simulate low-confidence numeric read
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    95,
		Max:        100,
		Percent:    95.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.3, // Low confidence (min is 0.7)
	}
	v.mu.Unlock()

	// Even though numeric says 95%, confidence is low -> ALLOW (don't trust unreliable parse)
	decision := v.ShouldBlockPotion(PotionHP, 30)
	if decision.Block {
		t.Errorf("Expected Block=false when confidence is low, got Block=%v", decision.Block)
	}
	if decision.Reason != "numeric_confidence_low" {
		t.Errorf("Expected Reason=numeric_confidence_low, got %q", decision.Reason)
	}
}

// TestShouldBlockPotion_AllowsWhenInvalidMax tests fail-safe: allow when max is invalid.
func TestShouldBlockPotion_AllowsWhenInvalidMax(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Simulate invalid max (zero or negative)
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    95,
		Max:        0, // Invalid: max should be > 0
		Percent:    0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	decision := v.ShouldBlockPotion(PotionHP, 30)
	if decision.Block {
		t.Errorf("Expected Block=false when max is invalid, got Block=%v", decision.Block)
	}
	if decision.Reason != "numeric_invalid_max" {
		t.Errorf("Expected Reason=numeric_invalid_max, got %q", decision.Reason)
	}
}

// TestShouldBlockPotion_SPIndependent tests that SP and HP use independent thresholds.
func TestShouldBlockPotion_SPIndependent(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Set HP to high, SP to low
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    95,
		Max:        100,
		Percent:    95.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.state.SP = NumericResourceRead{
		Found:      true,
		Current:    25,
		Max:        100,
		Percent:    25.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// HP should block at 95% with 30% threshold
	hpDecision := v.ShouldBlockPotion(PotionHP, 30)
	if !hpDecision.Block {
		t.Errorf("Expected Block=true for HP, got Block=%v", hpDecision.Block)
	}

	// SP should allow at 25% with 30% threshold
	spDecision := v.ShouldBlockPotion(PotionSP, 30)
	if spDecision.Block {
		t.Errorf("Expected Block=false for SP, got Block=%v", spDecision.Block)
	}
}

// TestShouldBlockPotion_NeverTriggersPotion tests that validator never returns a trigger signal.
// The validator is NEGATIVE-ONLY: it can only block, never trigger potting.
func TestShouldBlockPotion_NeverTriggersPotion(t *testing.T) {
	v := NewNumericSafetyValidator()

	testCases := []struct {
		percent    float64
		threshold  int
		name       string
	}{
		{10.0, 30, "low_hp"},
		{30.0, 30, "at_threshold"},
		{50.0, 50, "at_threshold_mid"},
		{0.0, 100, "critically_low"},
	}

	for _, tc := range testCases {
		v.mu.Lock()
		v.state.HP = NumericResourceRead{
			Found:      true,
			Current:    int(tc.percent),
			Max:        100,
			Percent:    tc.percent,
			UpdatedAt:  time.Now(),
			Confidence: 0.9,
		}
		v.mu.Unlock()

		decision := v.ShouldBlockPotion(PotionHP, tc.threshold)

		// When below or at threshold, block should be false (allow potion)
		if tc.percent <= float64(tc.threshold) {
			if decision.Block {
				t.Errorf("Case %q: Expected Block=false when below/at threshold, got Block=%v with reason=%q",
					tc.name, decision.Block, decision.Reason)
			}
		}
	}
}

// TestShouldBlockPotion_EdgeCases tests boundary conditions.
func TestShouldBlockPotion_EdgeCases(t *testing.T) {
	v := NewNumericSafetyValidator()

	testCases := []struct {
		percent   float64
		threshold int
		wantBlock bool
		name      string
	}{
		{30.1, 30, true, "slightly_above_threshold"},
		{30.0, 30, false, "exactly_at_threshold"},
		{29.9, 30, false, "slightly_below_threshold"},
		{100.0, 0, true, "max_resource_min_threshold"},
		{0.0, 0, false, "zero_resource_zero_threshold"},
	}

	for _, tc := range testCases {
		v.mu.Lock()
		v.state.HP = NumericResourceRead{
			Found:      true,
			Current:    int(tc.percent),
			Max:        100,
			Percent:    tc.percent,
			UpdatedAt:  time.Now(),
			Confidence: 0.9,
		}
		v.mu.Unlock()

		decision := v.ShouldBlockPotion(PotionHP, tc.threshold)
		if decision.Block != tc.wantBlock {
			t.Errorf("Case %q: Expected Block=%v at percent=%.1f threshold=%d, got Block=%v",
				tc.name, tc.wantBlock, tc.percent, tc.threshold, decision.Block)
		}
	}
}

// TestShouldBlockPotion_ReturnsMetadata tests that BlockDecision contains useful metadata.
func TestShouldBlockPotion_ReturnsMetadata(t *testing.T) {
	v := NewNumericSafetyValidator()

	now := time.Now()
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    95,
		Max:        100,
		Percent:    95.0,
		UpdatedAt:  now,
		Confidence: 0.85,
	}
	v.mu.Unlock()

	decision := v.ShouldBlockPotion(PotionHP, 30)

	if decision.Percent != 95.0 {
		t.Errorf("Expected Percent=95.0, got %.1f", decision.Percent)
	}
	if decision.Confidence != 0.85 {
		t.Errorf("Expected Confidence=0.85, got %.2f", decision.Confidence)
	}
	if decision.AgeMs < 0 || decision.AgeMs > 100 {
		t.Errorf("Expected AgeMs to be reasonable (0-100), got %d", decision.AgeMs)
	}
}

// TestNumericValidatorNeverBlocksOnError ensures fail-safe behavior for various error conditions.
func TestNumericValidatorNeverBlocksOnError(t *testing.T) {
	v := NewNumericSafetyValidator()

	errorCases := []struct {
		setup func()
		name  string
	}{
		{
			setup: func() {
				v.mu.Lock()
				v.state = NumericSafetyState{} // Empty state
				v.mu.Unlock()
			},
			name: "no_data",
		},
		{
			setup: func() {
				v.mu.Lock()
				v.state.HP = NumericResourceRead{
					Found: false,
				}
				v.mu.Unlock()
			},
			name: "found_false",
		},
		{
			setup: func() {
				v.mu.Lock()
				v.state.HP = NumericResourceRead{
					Found:     true,
					Max:       0,
					Percent:   0,
					UpdatedAt: time.Now(),
					Confidence: 0.9,
				}
				v.mu.Unlock()
			},
			name: "invalid_max",
		},
		{
			setup: func() {
				v.mu.Lock()
				v.state.HP = NumericResourceRead{
					Found:      true,
					Max:        100,
					Percent:    50.0,
					UpdatedAt:  time.Now().Add(-10 * time.Second), // Very stale
					Confidence: 0.9,
				}
				v.mu.Unlock()
			},
			name: "stale_data",
		},
	}

	for _, tc := range errorCases {
		tc.setup()
		decision := v.ShouldBlockPotion(PotionHP, 30)
		if decision.Block {
			t.Errorf("Case %q: Expected Block=false on error, got Block=%v with reason=%q",
				tc.name, decision.Block, decision.Reason)
		}
	}
}

// TestNumericValidatorAsyncStart tests that validator starts and runs asynchronously.
func TestNumericValidatorAsyncStart(t *testing.T) {
	v := NewNumericSafetyValidator()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start the validator in a goroutine
	// In real use, it will capture and parse images
	// In test, we just verify it doesn't panic during initialization
	go v.Start(ctx)

	// Give goroutine minimal time to start
	time.Sleep(50 * time.Millisecond)

	// Should be able to call State without blocking
	_ = v.State()

	// Wait for context to expire
	<-ctx.Done()
	time.Sleep(100 * time.Millisecond) // Let goroutine exit
}
