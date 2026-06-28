package runner

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultAPIAddr   = "localhost:3242"
	DefaultDelayMs   = 50
	StepHoldMs       = 20   // minimum gap so virtual HID events register
	PauseVK          = 0x23 // End
	ClickerSlotCount = 2
)

type ClickerSlot struct {
	TriggerVKs []int32
	DelayMs    int
	MouseClick bool
}

type Config struct {
	Session *ViiperSession
	Slots   [ClickerSlotCount]ClickerSlot
	Log     func(string)
}

func (c *Config) applyDefaults() {
	for i := range c.Slots {
		if c.Slots[i].DelayMs <= 0 {
			c.Slots[i].DelayMs = DefaultDelayMs
		}
	}
	if c.Log == nil {
		c.Log = func(string) {}
	}
}

func copyClickerSlots(slots [ClickerSlotCount]ClickerSlot) [ClickerSlotCount]ClickerSlot {
	var out [ClickerSlotCount]ClickerSlot
	for i, slot := range slots {
		out[i] = ClickerSlot{
			TriggerVKs: append([]int32(nil), slot.TriggerVKs...),
			DelayMs:    slot.DelayMs,
			MouseClick: slot.MouseClick,
		}
	}
	return out
}

func ActiveClickerSlotIndex(slots [ClickerSlotCount]ClickerSlot) (int, int32, bool) {
	for i, slot := range slots {
		if vk, ok := ActiveTrigger(slot.TriggerVKs); ok {
			return i, vk, true
		}
	}
	return -1, 0, false
}

type Runner struct {
	cfg Config

	mu             sync.Mutex
	cancel         context.CancelFunc
	done           chan struct{}
	running        bool
	liveMu    sync.RWMutex
	liveSlots [ClickerSlotCount]ClickerSlot
}

func New(cfg Config) *Runner {
	cfg.applyDefaults()
	return &Runner{cfg: cfg}
}

func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *Runner) Start() error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("clicker already running")
	}
	if r.cfg.Session == nil {
		r.mu.Unlock()
		return fmt.Errorf("input session is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.running = true
	r.liveSlots = copyClickerSlots(r.cfg.Slots)
	r.done = make(chan struct{})
	r.mu.Unlock()

	go func() {
		defer close(r.done)
		defer func() {
			r.mu.Lock()
			r.running = false
			r.cancel = nil
			r.mu.Unlock()
		}()
		r.run(ctx)
	}()

	return nil
}

func (r *Runner) Stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *Runner) Wait() {
	r.mu.Lock()
	done := r.done
	r.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (r *Runner) UpdateSettings(slots [ClickerSlotCount]ClickerSlot) {
	cfg := Config{Slots: slots}
	cfg.applyDefaults()
	r.liveMu.Lock()
	r.liveSlots = copyClickerSlots(cfg.Slots)
	r.liveMu.Unlock()
}

func (r *Runner) settings() [ClickerSlotCount]ClickerSlot {
	r.liveMu.RLock()
	slots := copyClickerSlots(r.liveSlots)
	r.liveMu.RUnlock()
	return slots
}

func (r *Runner) log(msg string) {
	r.cfg.Log(msg)
}

func (r *Runner) run(ctx context.Context) {
	session := r.cfg.Session

	for {
		if ctx.Err() != nil {
			return
		}

		if session.Paused() {
			time.Sleep(PollInterval)
			continue
		}

		slots := r.settings()
		slotIdx, triggerVK, active := ActiveClickerSlotIndex(slots)
		if !active {
			time.Sleep(PollInterval)
			continue
		}

		slot := slots[slotIdx]
		triggerVKs := append([]int32(nil), slot.TriggerVKs...)
		delay := time.Duration(slot.DelayMs) * time.Millisecond
		mouseClick := slot.MouseClick

		for TriggerHeld(triggerVKs) && !session.Paused() {
			if ctx.Err() != nil {
				return
			}

			slots = r.settings()
			newIdx, newVK, active := ActiveClickerSlotIndex(slots)
			if !active || newIdx != slotIdx {
				break
			}
			triggerVK = newVK
			slot = slots[slotIdx]
			triggerVKs = append([]int32(nil), slot.TriggerVKs...)
			delay = time.Duration(slot.DelayMs) * time.Millisecond
			mouseClick = slot.MouseClick
			if err := runCycle(ctx, session, triggerVK, triggerVKs, delay, mouseClick); err != nil {
				if ctx.Err() != nil {
					return
				}
				r.log(fmt.Sprintf("Clicker loop failed: %v", err))
				return
			}
			if !TriggerHeld(triggerVKs) || session.Paused() {
				break
			}
		}
	}
}

func runCycle(ctx context.Context, session *ViiperSession, vk int32, triggerVKs []int32, delay time.Duration, mouseClick bool) error {
	defer session.ReleaseAll()

	step := time.Duration(StepHoldMs) * time.Millisecond

	if err := session.KeyDown(vk); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !waitDelay(ctx, triggerVKs, delay, session.Paused) {
		return ctx.Err()
	}

	if mouseClick {
		if err := session.MouseDown(); err != nil {
			return err
		}
		sleep(ctx, step)
	}

	if err := session.KeyUp(); err != nil {
		return err
	}

	if mouseClick {
		sleep(ctx, step)
		if err := session.MouseUp(); err != nil {
			return err
		}
	}
	return nil
}

// waitDelay sleeps for delay. Trigger release ends the wait early but the cycle continues.
// Returns false only when the clicker is stopped (context cancelled).
func waitDelay(ctx context.Context, triggerVKs []int32, d time.Duration, paused func() bool) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if paused != nil && paused() {
			return true
		}
		if len(triggerVKs) > 0 && !TriggerHeld(triggerVKs) {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return ctx.Err() == nil
}

func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
