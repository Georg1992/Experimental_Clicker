package runner

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	TimerKeySlotCount         = 5
	DefaultTimerKeyIntervalMs = 1000
)

type TimerSlot struct {
	Enabled    bool
	KeyVK      int32
	IntervalMs int
}

type TimerKeyConfig struct {
	Session *ViiperSession
	Slots   [TimerKeySlotCount]TimerSlot
	Log     func(string)
}

func (c *TimerKeyConfig) applyDefaults() {
	for i := range c.Slots {
		if c.Slots[i].IntervalMs <= 0 {
			c.Slots[i].IntervalMs = DefaultTimerKeyIntervalMs
		}
	}
	if c.Log == nil {
		c.Log = func(string) {}
	}
}

func (c TimerKeyConfig) AnyActive() bool {
	for _, slot := range c.Slots {
		if slot.Enabled && slot.KeyVK != 0 {
			return true
		}
	}
	return false
}

type TimerKeyRunner struct {
	cfg TimerKeyConfig

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	running bool

	liveMu sync.RWMutex
	live   TimerKeyConfig
}

func NewTimerKey(cfg TimerKeyConfig) *TimerKeyRunner {
	cfg.applyDefaults()
	return &TimerKeyRunner{cfg: cfg, live: cfg}
}

func (t *TimerKeyRunner) Running() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *TimerKeyRunner) UpdateSettings(cfg TimerKeyConfig) {
	cfg.applyDefaults()
	t.liveMu.Lock()
	t.live = cfg
	t.liveMu.Unlock()
}

func (t *TimerKeyRunner) settings() TimerKeyConfig {
	t.liveMu.RLock()
	defer t.liveMu.RUnlock()
	return t.live
}

func (t *TimerKeyRunner) Start() error {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return fmt.Errorf("timer key already running")
	}

	cfg := t.settings()
	if !cfg.AnyActive() {
		t.mu.Unlock()
		return nil
	}
	if cfg.Session == nil {
		t.mu.Unlock()
		return fmt.Errorf("input session is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.running = true
	t.done = make(chan struct{})
	t.mu.Unlock()

	go func() {
		defer close(t.done)
		defer func() {
			t.mu.Lock()
			t.running = false
			t.cancel = nil
			t.mu.Unlock()
		}()
		t.run(ctx)
	}()

	return nil
}

func (t *TimerKeyRunner) Stop() {
	t.mu.Lock()
	cancel := t.cancel
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (t *TimerKeyRunner) Wait() {
	t.mu.Lock()
	done := t.done
	t.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (t *TimerKeyRunner) log(msg string) {
	t.cfg.Log(msg)
}

func (t *TimerKeyRunner) run(ctx context.Context) {
	session := t.cfg.Session

	var lastSlots [TimerKeySlotCount]TimerSlot
	var nextDue [TimerKeySlotCount]time.Time

	for {
		if ctx.Err() != nil {
			return
		}
		if session.Paused() {
			sleep(ctx, 10*time.Millisecond)
			continue
		}

		cfg := t.settings()
		now := time.Now()
		var earliest time.Time
		anyActive := false

		for i := range cfg.Slots {
			slot := cfg.Slots[i]
			if !slot.Enabled || slot.KeyVK == 0 {
				lastSlots[i] = TimerSlot{}
				nextDue[i] = time.Time{}
				continue
			}
			anyActive = true

			if slot != lastSlots[i] {
				lastSlots[i] = slot
				nextDue[i] = time.Time{}
			}

			interval := time.Duration(slot.IntervalMs) * time.Millisecond
			if nextDue[i].IsZero() {
				nextDue[i] = now
			}
			if !now.Before(nextDue[i]) {
				if err := session.TapKey(slot.KeyVK, autoPotKeyHold); err != nil {
					t.log(fmt.Sprintf("Timer %d key %s failed: %v", i+1, KeyName(slot.KeyVK), err))
					return
				}
				nextDue[i] = now.Add(interval)
			}
			if earliest.IsZero() || nextDue[i].Before(earliest) {
				earliest = nextDue[i]
			}
		}

		if !anyActive {
			sleep(ctx, 50*time.Millisecond)
			continue
		}

		wait := time.Until(earliest)
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		if wait > 10*time.Millisecond {
			wait = 10 * time.Millisecond
		}
		sleep(ctx, wait)
	}
}
