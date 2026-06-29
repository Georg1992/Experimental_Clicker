package autopot

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNumericSafetyValidatorConcurrent stresses the parts of
// NumericSafetyValidator that autopot reads on the hot path:
//   - GetCachedSafety() reads cachedSafety via atomic.Value.Load
//   - SetThresholds() and SetLogFunc() mutate cfg under mu (RWMutex)
//
// Note: the SetPollInterval / SetMaxStateAge / SetMinConfidence setters are
// intentionally NOT exercised here — those write unlocked fields that the
// run() goroutine also reads without locking. They are a separate fix.
func TestNumericSafetyValidatorConcurrent(t *testing.T) {
	v := NewNumericSafetyValidator()
	const duration = 250 * time.Millisecond
	stop := make(chan struct{})
	var wg sync.WaitGroup

	var reads atomic.Int64
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					snap := v.GetCachedSafety()
					if snap == nil {
						t.Error("GetCachedSafety returned nil")
						return
					}
					reads.Add(1)
				}
			}
		}()
	}
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
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				v.SetLogFunc(func(string) {})
			}
		}
	}()

	time.Sleep(duration)
	close(stop)
	wg.Wait()
	if reads.Load() == 0 {
		t.Fatalf("GetCachedSafety was never called")
	}
}
