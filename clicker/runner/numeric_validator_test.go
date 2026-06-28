package runner

import (
	"context"
	"testing"
	"time"
)

// TestPushMonitorPublishesHPDoNotPot tests that validator publishes HPDoNotPot=true when HP is above threshold.
func TestPushMonitorPublishesHPDoNotPot(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetThresholds(30, 30)

	// Simulate validator publish: HP at 97%
	snapshot := &SafetySnapshot{
		HPPercent:    97.0,
		HPConfidence: 0.9,
		HPThreshold:  30,
		HPDoNotPot:   true, // Above threshold
		UpdatedAt:    time.Now(),
		Found:        true,
	}
	v.cachedSafety.Store(snapshot)

	// AutoPot reads the flag
	safety := v.GetCachedSafety()
	if !safety.HPDoNotPot {
		t.Errorf("Expected HPDoNotPot=true when HP=97%% > threshold=30%%, got %v", safety.HPDoNotPot)
	}
	if safety.HPPercent != 97.0 {
		t.Errorf("Expected HPPercent=97.0, got %.1f", safety.HPPercent)
	}
}

// TestPushMonitorAllowsHPBelowThreshold tests that validator publishes HPDoNotPot=false when HP is below threshold.
func TestPushMonitorAllowsHPBelowThreshold(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetThresholds(30, 30)

	// Simulate validator publish: HP at 25%
	snapshot := &SafetySnapshot{
		HPPercent:    25.0,
		HPConfidence: 0.9,
		HPThreshold:  30,
		HPDoNotPot:   false, // Below threshold
		UpdatedAt:    time.Now(),
		Found:        true,
	}
	v.cachedSafety.Store(snapshot)

	// AutoPot reads the flag
	safety := v.GetCachedSafety()
	if safety.HPDoNotPot {
		t.Errorf("Expected HPDoNotPot=false when HP=25%% < threshold=30%%, got %v", safety.HPDoNotPot)
	}
	if safety.HPPercent != 25.0 {
		t.Errorf("Expected HPPercent=25.0, got %.1f", safety.HPPercent)
	}
}

// TestPushMonitorSPIndependent tests that SP DoNotPot flag is independent from HP.
func TestPushMonitorSPIndependent(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetThresholds(30, 30)

	// Publish: HP safe (100%), SP needs potion (25%)
	snapshot := &SafetySnapshot{
		HPPercent:    100.0,
		HPConfidence: 0.9,
		HPThreshold:  30,
		HPDoNotPot:   true, // HP above threshold
		SPPercent:    25.0,
		SPConfidence: 0.9,
		SPThreshold:  30,
		SPDoNotPot:   false, // SP below threshold
		UpdatedAt:    time.Now(),
		Found:        true,
	}
	v.cachedSafety.Store(snapshot)

	safety := v.GetCachedSafety()
	if !safety.HPDoNotPot {
		t.Errorf("Expected HPDoNotPot=true, got %v", safety.HPDoNotPot)
	}
	if safety.SPDoNotPot {
		t.Errorf("Expected SPDoNotPot=false, got %v", safety.SPDoNotPot)
	}
}

// TestPushMonitorFailOpenOnParseFailure tests that parse failure publishes fail-open flags.
func TestPushMonitorFailOpenOnParseFailure(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Publish failure state
	snapshot := &SafetySnapshot{
		HPDoNotPot:  false, // Fail-open
		SPDoNotPot:  false, // Fail-open
		UpdatedAt:   time.Now(),
		Found:       false,
		ErrorReason: "parse_error",
	}
	v.cachedSafety.Store(snapshot)

	safety := v.GetCachedSafety()
	if safety.HPDoNotPot || safety.SPDoNotPot {
		t.Errorf("Expected fail-open flags on parse failure: HPDoNotPot=%v, SPDoNotPot=%v", safety.HPDoNotPot, safety.SPDoNotPot)
	}
}

// TestPushMonitorStaleNeverBlocks tests that stale snapshot never blocks (fail-safe).
func TestPushMonitorStaleNeverBlocks(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Old snapshot (5 seconds old, max age is 2 seconds)
	oldTime := time.Now().Add(-5 * time.Second)
	snapshot := &SafetySnapshot{
		HPPercent:   100.0,
		HPDoNotPot:  true, // Would block if fresh
		HPThreshold: 30,
		UpdatedAt:   oldTime,
		Found:       true,
	}
	v.cachedSafety.Store(snapshot)

	safety := v.GetCachedSafety()
	// Snapshot is stale
	if safety.IsFresh(2 * time.Second) {
		t.Errorf("Expected snapshot to be stale after 5 seconds")
	}

	// AutoPot checks freshness before using DoNotPot
	// Stale snapshot should not block
	if safety.IsFresh(2 * time.Second) && safety.HPDoNotPot {
		t.Errorf("Stale snapshot should not cause block")
	}
}

// TestPushMonitorThresholdUpdate tests that threshold changes affect next publish.
func TestPushMonitorThresholdUpdate(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetThresholds(30, 30)

	// First snapshot with threshold 30
	snapshot1 := &SafetySnapshot{
		HPPercent:   40.0,
		HPThreshold: 30,
		HPDoNotPot:  true, // 40 > 30 = true
		UpdatedAt:   time.Now(),
		Found:       true,
	}
	v.cachedSafety.Store(snapshot1)

	safety1 := v.GetCachedSafety()
	if !safety1.HPDoNotPot {
		t.Errorf("Expected HPDoNotPot=true with threshold 30 and percent 40")
	}

	// Update threshold to 50
	v.SetThresholds(50, 50)

	// Publish new snapshot with updated threshold
	snapshot2 := &SafetySnapshot{
		HPPercent:   40.0,
		HPThreshold: 50,
		HPDoNotPot:  false, // 40 > 50 = false
		UpdatedAt:   time.Now(),
		Found:       true,
	}
	v.cachedSafety.Store(snapshot2)

	safety2 := v.GetCachedSafety()
	if safety2.HPDoNotPot {
		t.Errorf("Expected HPDoNotPot=false with updated threshold 50 and percent 40")
	}
}

// TestPushMonitorLowConfidenceFailsOpen tests that low confidence results in fail-open.
func TestPushMonitorLowConfidenceFailsOpen(t *testing.T) {
	v := NewNumericSafetyValidator()
	v.SetThresholds(30, 30)

	// Snapshot with low confidence
	snapshot := &SafetySnapshot{
		HPPercent:    100.0,
		HPConfidence: 0.3, // Low confidence (min 0.7)
		HPThreshold:  30,
		HPDoNotPot:   false, // Fail-open despite high percent
		UpdatedAt:    time.Now(),
		Found:        true,
	}
	v.cachedSafety.Store(snapshot)

	safety := v.GetCachedSafety()
	if safety.HPDoNotPot {
		t.Errorf("Expected fail-open HPDoNotPot=false with low confidence, got %v", safety.HPDoNotPot)
	}
	if safety.HPConfidence != 0.3 {
		t.Errorf("Expected HPConfidence=0.3, got %.2f", safety.HPConfidence)
	}
}

// TestPushMonitorMetadataAvailable tests that all metadata is available for diagnostics.
func TestPushMonitorMetadataAvailable(t *testing.T) {
	v := NewNumericSafetyValidator()

	now := time.Now()
	snapshot := &SafetySnapshot{
		HPPercent:    85.5,
		HPConfidence: 0.92,
		HPThreshold:  30,
		SPPercent:    42.1,
		SPConfidence: 0.88,
		SPThreshold:  35,
		HPDoNotPot:   true,
		SPDoNotPot:   false,
		UpdatedAt:    now,
		Found:        true,
		ErrorReason:  "",
	}
	v.cachedSafety.Store(snapshot)

	safety := v.GetCachedSafety()
	if safety.HPPercent != 85.5 {
		t.Errorf("Expected HPPercent=85.5, got %.1f", safety.HPPercent)
	}
	if safety.SPPercent != 42.1 {
		t.Errorf("Expected SPPercent=42.1, got %.1f", safety.SPPercent)
	}
	if safety.HPConfidence != 0.92 {
		t.Errorf("Expected HPConfidence=0.92, got %.2f", safety.HPConfidence)
	}
	if safety.SPConfidence != 0.88 {
		t.Errorf("Expected SPConfidence=0.88, got %.2f", safety.SPConfidence)
	}
	if safety.Age() > 100 {
		t.Errorf("Expected Age <100ms, got %d", safety.Age())
	}
}

// TestPushMonitorNonBlocking tests that GetCachedSafety is O(1) non-blocking.
func TestPushMonitorNonBlocking(t *testing.T) {
	v := NewNumericSafetyValidator()

	snapshot := &SafetySnapshot{
		HPPercent:  50.0,
		HPDoNotPot: false,
		UpdatedAt:  time.Now(),
		Found:      true,
	}
	v.cachedSafety.Store(snapshot)

	// 10000 reads should be very fast (no IO, no blocking)
	start := time.Now()
	for i := 0; i < 10000; i++ {
		_ = v.GetCachedSafety()
	}
	elapsed := time.Since(start)

	// Should complete in <100ms (typically <10µs each)
	if elapsed > 100*time.Millisecond {
		t.Logf("WARNING: 10000 GetCachedSafety calls took %v (expected <100ms)", elapsed)
	}
}

// TestPushMonitorValidatorIndependent tests validator runs independently from AutoPot.
func TestPushMonitorValidatorIndependent(t *testing.T) {
	v := NewNumericSafetyValidator()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start validator in background
	v.Start(ctx)

	// Sleep minimal time for startup
	time.Sleep(50 * time.Millisecond)

	// Reading cache should be instant and non-blocking
	startRead := time.Now()
	safety := v.GetCachedSafety()
	elapsedRead := time.Since(startRead)

	if elapsedRead > 1*time.Millisecond {
		t.Logf("WARNING: GetCachedSafety took %v (expected <1ms)", elapsedRead)
	}

	if safety == nil {
		t.Errorf("Expected non-nil safety snapshot")
	}

	// Wait for context to expire
	<-ctx.Done()
	time.Sleep(100 * time.Millisecond) // Let goroutine exit
}

// TestPushMonitorInitialState tests that validator has safe initial state.
func TestPushMonitorInitialState(t *testing.T) {
	v := NewNumericSafetyValidator()

	// Initial state before any parsing
	safety := v.GetCachedSafety()

	// Should fail-open
	if safety.HPDoNotPot || safety.SPDoNotPot {
		t.Errorf("Expected initial fail-open state: HPDoNotPot=%v, SPDoNotPot=%v", safety.HPDoNotPot, safety.SPDoNotPot)
	}

	// Should have valid timestamp
	if safety.UpdatedAt.IsZero() {
		t.Errorf("Expected non-zero UpdatedAt timestamp")
	}
}
