package autopot

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"experimental-clicker/runner/internal/session"
)

// mockSession is a session.InputSession for the autopot stress test.
type mockSession struct {
	mu       sync.Mutex
	paused   bool
	tapCount atomic.Int64
}

func (m *mockSession) Paused() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paused
}

func (m *mockSession) SetPaused(p bool) {
	m.mu.Lock()
	m.paused = p
	m.mu.Unlock()
}

func (m *mockSession) TapKey(vk int32, hold time.Duration) error {
	m.tapCount.Add(1)
	time.Sleep(hold)
	return nil
}

func (m *mockSession) MouseDown() error { return nil }
func (m *mockSession) MouseUp() error   { return nil }

// TestAutoPotRunnerStress starts a real AutoPotRunner. The run() loop
// calls win.CapturePlayerBarSearch(), which fails in a non-game test env
// and triggers the `continue` branch. That branch still exercises:
//   - a.settings()        (lifecycle.Settings, RLock on liveMu)
//   - session.Paused()    (InputSession.RLock in real ViiperSession)
//   - timing.Sleep        (ctx-aware sleep — Stop works)
// and the spawned numericValidator goroutine also loops, calling
// SetLogFunc / SetThresholds on the validator and atomic Store of
// cachedSafety. Hammering UpdateSettings from outside covers the same
// surface the healUntil() hot path reads.
func TestAutoPotRunnerStress(t *testing.T) {
	sess := &mockSession{}
	cfg := AutoPotConfig{
		Session:     sess,
		HPThreshold: 50,
		SPThreshold: 50,
		HPKeyVK:     'Q',
		SPKeyVK:     'W',
		HPEnabled:   true,
		SPEnabled:   true,
		Log:         func(string) {},
	}
	ap := NewAutoPot(cfg)
	if err := ap.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
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
					ap.UpdateSettings(AutoPotConfig{
						Session:     sess,
						HPThreshold: 40 + n%40,
						SPThreshold: 40 + n%40,
						HPKeyVK:     'Q',
						SPKeyVK:     'W',
						HPEnabled:   n%2 == 0,
						SPEnabled:   true,
						Log:         func(string) {},
					})
					n++
				}
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		on := false
		for {
			select {
			case <-stop:
				return
			case <-time.After(3 * time.Millisecond):
				on = !on
				sess.SetPaused(on)
			}
		}
	}()
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = ap.Running()
				}
			}
		}()
	}

	time.Sleep(250 * time.Millisecond)
	close(stop)
	wg.Wait()
	ap.Stop()
	ap.Wait()
	if ap.Running() {
		t.Fatal("still running after Stop+Wait")
	}
}

var _ session.InputSession = (*mockSession)(nil)
