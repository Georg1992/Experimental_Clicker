package autopot

import (
	"sync"
	"testing"
	"time"
)

// TestBarStabilizerConcurrentUpdates stresses the BarStabilizer's internal
// mu by hammering UpdatePair (mutates fullLatched / lastValidRect / lowStreak
// / notFullStreak) concurrently with SetThreshold and Reset. The stabilizer
// is read+mutated on every poll by autopot.run and autopot.healUntil, so
// this is on the auto-pot hot path.
func TestBarStabilizerConcurrentUpdates(t *testing.T) {
	img := loadFixture(t, "jj.png")
	mapped, err := RefreshBarPair(img)
	if err != nil {
		t.Fatal(err)
	}

	stab := NewBarStabilizer(true, 50)
	const duration = 250 * time.Millisecond
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = stab.UpdatePair(img, true, mapped, true)
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
					stab.SetThreshold(1 + n%99)
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
				stab.Reset()
			}
		}
	}()

	time.Sleep(duration)
	close(stop)
	wg.Wait()
}
