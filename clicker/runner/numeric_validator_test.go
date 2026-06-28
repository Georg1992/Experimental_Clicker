package runner

import (
	"context"
	"testing"
	"time"
)

func TestNumericBlocksWhenAboveThreshold(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetSafetyMargin(4)

	// Simulate numeric read: HP at 74%
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    74,
		Max:        100,
		Percent:    74.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// Threshold is 70, numeric is 74, margin is 4 -> block at 74 (70+4)
	if !v.ShouldBlockHP(70) {
		t.Errorf("Expected ShouldBlockHP(70) to return true when numeric is 74%%")
	}
}

func TestNumericDoesNotBlockWhenBelowThreshold(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetSafetyMargin(4)

	// Simulate numeric read: HP at 69%
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    69,
		Max:        100,
		Percent:    69.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// Threshold is 70, numeric is 69, margin is 4 -> don't block
	if v.ShouldBlockHP(70) {
		t.Errorf("Expected ShouldBlockHP(70) to return false when numeric is 69%%")
	}
}

func TestNumericDoesNotBlockWhenNotFound(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetSafetyMargin(4)

	// No numeric data
	if v.ShouldBlockHP(70) {
		t.Errorf("Expected ShouldBlockHP to return false when numeric data not found")
	}
}

func TestNumericDoesNotBlockWhenStale(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetSafetyMargin(4)

	// Simulate old numeric read
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    90,
		Max:        100,
		Percent:    90.0,
		UpdatedAt:  time.Now().Add(-5 * time.Second), // 5 seconds old
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// Even though numeric says 90%, it's stale -> don't block
	if v.ShouldBlockHP(70) {
		t.Errorf("Expected ShouldBlockHP to return false when data is stale")
	}
}

func TestNumericDoesNotBlockWhenLowConfidence(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetSafetyMargin(4)

	// Simulate low-confidence numeric read
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    90,
		Max:        100,
		Percent:    90.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.3, // Low confidence
	}
	v.mu.Unlock()

	// Even though numeric says 90%, confidence is low -> don't block
	if v.ShouldBlockHP(70) {
		t.Errorf("Expected ShouldBlockHP to return false when confidence is low")
	}
}

func TestNumericValidatorNeverTriggersPotion(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetSafetyMargin(4)

	// Simulate any numeric state
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    50,
		Max:        100,
		Percent:    50.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// NumericSafetyValidator should NEVER say "use potion"
	// It only says "don't use" (true) or "I can't decide" (false)
	// It never returns true to trigger potting

	// Test: When threshold is 80 and numeric is 50, should not block
	// (can't decide because numeric is below threshold)
	if v.ShouldBlockHP(80) {
		t.Errorf("Expected ShouldBlockHP(80) false when numeric is 50%%")
	}

	// Test: When threshold is 30 and numeric is 50, should block
	// (numeric is above threshold + margin)
	if !v.ShouldBlockHP(30) {
		t.Errorf("Expected ShouldBlockHP(30) true when numeric is 50%%")
	}

	// But this is still not triggering, just blocking false triggers
}

func TestNumericBlocksSPIndependently(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetSafetyMargin(4)

	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    50,
		Max:        100,
		Percent:    50.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.state.SP = NumericResourceRead{
		Found:      true,
		Current:    90,
		Max:        100,
		Percent:    90.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// HP at 50%, SP at 90%
	// With threshold 70:
	// - HP should not block (50 < 70+4)
	// - SP should block (90 >= 70+4)

	if v.ShouldBlockHP(70) {
		t.Errorf("Expected ShouldBlockHP(70) false when HP is 50%%")
	}

	if !v.ShouldBlockSP(70) {
		t.Errorf("Expected ShouldBlockSP(70) true when SP is 90%%")
	}
}

func TestNumericValidatorCopiesStateThreadSafely(t *testing.T) {
	v := NewNumericSafetyValidator()

	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    80,
		Max:        100,
		Percent:    80.0,
		UpdatedAt:  time.Now(),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	// State() should return a copy without holding the lock
	state := v.State()
	if !state.HP.Found {
		t.Errorf("Expected State() to return HP data")
	}
	if state.HP.Percent != 80.0 {
		t.Errorf("Expected HP percent 80.0, got %v", state.HP.Percent)
	}
}

func TestNumericValidatorEdgeCases(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetSafetyMargin(5)

	tests := []struct {
		name          string
		current       int
		max           int
		confidence    float64
		threshold     int
		expectedBlock bool
		description   string
	}{
		{
			name:          "exactly at threshold + margin",
			current:       75,
			max:           100,
			confidence:    0.9,
			threshold:     70,
			expectedBlock: true,
			description:   "75% == 70% + 5% margin",
		},
		{
			name:          "just below threshold + margin",
			current:       74,
			max:           100,
			confidence:    0.9,
			threshold:     70,
			expectedBlock: false,
			description:   "74% < 70% + 5% margin",
		},
		{
			name:          "confidence at minimum threshold",
			current:       100,
			max:           100,
			confidence:    0.7,
			threshold:     50,
			expectedBlock: true,
			description:   "100% with confidence 0.7 should block",
		},
		{
			name:          "confidence just below minimum",
			current:       100,
			max:           100,
			confidence:    0.69,
			threshold:     50,
			expectedBlock: false,
			description:   "confidence 0.69 is below 0.7 minimum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v.mu.Lock()
			v.state.HP = NumericResourceRead{
				Found:      true,
				Current:    tt.current,
				Max:        tt.max,
				Percent:    float64(tt.current) / float64(tt.max) * 100.0,
				UpdatedAt:  time.Now(),
				Confidence: tt.confidence,
			}
			v.mu.Unlock()

			result := v.ShouldBlockHP(tt.threshold)
			if result != tt.expectedBlock {
				t.Errorf("%s: expected %v, got %v (%s)", tt.name, tt.expectedBlock, result, tt.description)
			}
		})
	}
}

func TestNumericValidatorStalenessBoundary(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetSafetyMargin(4)

	// Test just before maxStateAge
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    90,
		Max:        100,
		Percent:    90.0,
		UpdatedAt:  time.Now().Add(-1999 * time.Millisecond),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	if !v.ShouldBlockHP(70) {
		t.Errorf("Expected fresh data (1.999s old, maxAge 2s) to block")
	}

	// Test just after maxStateAge
	v.mu.Lock()
	v.state.HP = NumericResourceRead{
		Found:      true,
		Current:    90,
		Max:        100,
		Percent:    90.0,
		UpdatedAt:  time.Now().Add(-2001 * time.Millisecond),
		Confidence: 0.9,
	}
	v.mu.Unlock()

	if v.ShouldBlockHP(70) {
		t.Errorf("Expected stale data (2.001s old, maxAge 2s) to not block")
	}
}

func TestNumericValidatorBackgroundStart(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Just verify that Start doesn't panic and the validator can be stopped
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the validator
	v.Start(ctx)

	// Small delay to ensure goroutine starts
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop validator
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Should be able to call State without blocking
	_ = v.State()
}
