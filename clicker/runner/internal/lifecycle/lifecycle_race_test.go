package lifecycle

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLifecycleSettingsRace stresses the liveMu RWMutex by hammering
// UpdateSettings (writers), Settings (readers), and Running (start/stop mu)
// from many goroutines while a run() goroutine loops reading Settings().
// The race detector watches for any unsynchronized access to live or running.
func TestLifecycleSettingsRace(t *testing.T) {
	const (
		writers     = 4
		readers     = 8
		runners     = 4
		runDuration = 250 * time.Millisecond
	)

	lc := New[int](0, nil, nil)

	var iterations atomic.Int64
	if err := lc.Start(func(ctx context.Context, cfg int) {
		for ctx.Err() == nil {
			_ = lc.Settings()
			iterations.Add(1)
			time.Sleep(20 * time.Microsecond)
		}
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			n := seed
			for {
				select {
				case <-stop:
					return
				default:
					lc.UpdateSettings(n)
					n++
				}
			}
		}(i * 100_000)
	}
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = lc.Settings()
				}
			}
		}()
	}
	for i := 0; i < runners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = lc.Running()
				}
			}
		}()
	}

	time.Sleep(runDuration)
	close(stop)
	wg.Wait()
	lc.Stop()
	lc.Wait()
	if got := iterations.Load(); got == 0 {
		t.Fatalf("run() did 0 iterations in %s — the goroutine never started", runDuration)
	}
}

// TestLifecycleStartStopRace stresses the start/stop mu by rapidly
// starting/stopping while many goroutines hammer Running. Surfaces any
// race in the cancel/done/running bookkeeping.
func TestLifecycleStartStopRace(t *testing.T) {
	const iterations = 50
	// Re-use the same lc across iterations to stress the start/stop
	// re-initialisation path (a fresh lc each time would be a different
	// memory address and miss the race surface).
	lc := New[int](42, nil, nil)
	run := func(ctx context.Context, cfg int) {
		for ctx.Err() == nil {
			time.Sleep(100 * time.Microsecond)
		}
	}
	for i := 0; i < iterations; i++ {
		if err := lc.Start(run); err != nil {
			t.Fatalf("iter %d: Start: %v", i, err)
		}
		var wg sync.WaitGroup
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for k := 0; k < 1000; k++ {
					_ = lc.Running()
				}
			}()
		}
		wg.Wait()
		lc.Stop()
		lc.Wait()
		if lc.Running() {
			t.Fatalf("iter %d: still running after Wait", i)
		}
	}
}
