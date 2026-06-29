package autopot

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestNumericSafetyValidatorConfigRace exercises the config setters
// (SetPollInterval, SetMinConfidence) that share state with the
// validator's run() goroutine (read of pollInterval at start) and the
// publish-loop (read of minConfidence / maxStateAge on every publish).
//
// Without the locking fix on the setters and reads, this test fails
// with the race detector flagging unsynchronized access to v.pollInterval
// and v.minConfidence.
func TestNumericSafetyValidatorConfigRace(t *testing.T) {
	v := NewNumericSafetyValidator()
	// Tight poll interval so publishSafetySnapshot fires many times
	// during the test window — that's the path that reads
	// minConfidence / maxStateAge and is the one that races with the
	// unlocked setters.
	v.SetPollInterval(2 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v.Start(ctx)

	const duration = 200 * time.Millisecond
	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Hammer the previously-unlocked config setters
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			n := seed
			for {
				select {
				case <-stop:
					return
				default:
					v.SetMinConfidence(0.1 + float64(n%9)/10.0)
					v.SetPollInterval(time.Duration(1+n%5) * time.Millisecond)
					n++
				}
			}
		}(i)
	}
	// Hammer the locked threshold setter — should be race-clean too,
	// and gives the detector more signal that the locked path works.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			n := seed
			for {
				select {
				case <-stop:
					return
				default:
					v.SetThresholds(1+n%90, 1+n%90)
					n++
				}
			}
		}(i)
	}
	// Concurrent reader on the atomic snapshot
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = v.GetCachedSafety()
				}
			}
		}()
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()
	cancel()
	// Give the run() goroutine a moment to observe ctx.Done and exit.
	time.Sleep(50 * time.Millisecond)
}
