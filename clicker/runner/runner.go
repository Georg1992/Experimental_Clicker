package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Alia5/VIIPER/device/keyboard"
	"github.com/Alia5/VIIPER/device/mouse"
	"github.com/Alia5/VIIPER/viiperclient"
)

const (
	DefaultAPIAddr = "localhost:3242"
	DefaultDelayMs = 50
	StepHoldMs     = 20 // minimum gap so virtual HID events register
	EscapeVK       = 0x1B
)

type Config struct {
	APIAddr    string
	TriggerVKs []int32
	DelayMs    int
	Log        func(string)
}

func (c *Config) applyDefaults() {
	if c.APIAddr == "" {
		c.APIAddr = DefaultAPIAddr
	}
	if c.DelayMs <= 0 {
		c.DelayMs = DefaultDelayMs
	}
	if c.Log == nil {
		c.Log = func(string) {}
	}
}

type Runner struct {
	cfg Config

	mu             sync.Mutex
	cancel         context.CancelFunc
	done           chan struct{}
	running        bool
	liveMu         sync.RWMutex
	liveTriggerVKs []int32
	liveDelayMs    int
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

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.running = true
	r.liveTriggerVKs = append([]int32(nil), r.cfg.TriggerVKs...)
	r.liveDelayMs = r.cfg.DelayMs
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

func (r *Runner) UpdateSettings(triggerVKs []int32, delayMs int) {
	r.liveMu.Lock()
	r.liveTriggerVKs = append([]int32(nil), triggerVKs...)
	if delayMs > 0 {
		r.liveDelayMs = delayMs
	}
	r.liveMu.Unlock()
}

func (r *Runner) settings() ([]int32, time.Duration) {
	r.liveMu.RLock()
	delayMs := r.liveDelayMs
	triggerVKs := append([]int32(nil), r.liveTriggerVKs...)
	r.liveMu.RUnlock()
	return triggerVKs, time.Duration(delayMs) * time.Millisecond
}

var noopLog = func(string) {}

func (r *Runner) log(msg string) {
	r.cfg.Log(msg)
}

func (r *Runner) run(ctx context.Context) {
	r.log("Setting up virtual keyboard and mouse...")

	api := viiperclient.New(r.cfg.APIAddr)

	ping, err := api.PingCtx(ctx)
	if err != nil {
		r.log(fmt.Sprintf("VIIPER connection failed: %v", err))
		return
	}
	r.log(fmt.Sprintf("Connected to VIIPER %s", ping.Version))

	busID, createdBus, err := ensureBus(ctx, api, noopLog)
	if err != nil {
		r.log(fmt.Sprintf("Device bus setup failed: %v", err))
		return
	}

	keyStream, keyDev, err := api.AddDeviceAndConnect(ctx, busID, "keyboard", nil)
	if err != nil {
		r.log(fmt.Sprintf("Virtual keyboard setup failed: %v", err))
		cleanupBus(ctx, api, busID, createdBus, noopLog)
		return
	}
	defer keyStream.Close() //nolint:errcheck
	_ = keyDev
	r.log("Virtual keyboard ready")

	mouseStream, mouseDev, err := api.AddDeviceAndConnect(ctx, busID, "mouse", nil)
	if err != nil {
		r.log(fmt.Sprintf("Virtual mouse setup failed: %v", err))
		cleanupDevice(ctx, api, keyStream.BusID, keyStream.DevID, noopLog)
		cleanupBus(ctx, api, busID, createdBus, noopLog)
		return
	}
	defer mouseStream.Close() //nolint:errcheck
	_ = mouseDev
	r.log("Virtual mouse ready")

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupDevice(cleanupCtx, api, keyStream.BusID, keyStream.DevID, noopLog)
		cleanupDevice(cleanupCtx, api, mouseStream.BusID, mouseStream.DevID, noopLog)
		cleanupBus(cleanupCtx, api, busID, createdBus, noopLog)
	}()

	triggerVKs, delay := r.settings()
	r.log(fmt.Sprintf("Trigger keys: %s", KeysText(triggerVKs)))
	r.log(fmt.Sprintf("Delay: %d ms", delay.Milliseconds()))
	if len(triggerVKs) == 0 {
		r.log("Add trigger keys in the GUI — you can do this before or after launching the game")
	}
	r.log("Hold a trigger key to run the loop. ESC stops the clicker.")

	for {
		if ctx.Err() != nil {
			r.log("Clicker stopped")
			return
		}
		if PhysicalKeyDown(EscapeVK) {
			r.log("ESC pressed — clicker stopped")
			return
		}

		triggerVKs, delay := r.settings()
		if !TriggerHeld(triggerVKs) {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		triggerVK, _ := ActiveTrigger(triggerVKs)
		r.log(fmt.Sprintf("Loop running (%s)", KeyName(triggerVK)))

		for TriggerHeld(triggerVKs) {
			if ctx.Err() != nil {
				return
			}
			if PhysicalKeyDown(EscapeVK) {
				r.log("ESC pressed — clicker stopped")
				return
			}

			triggerVKs, delay = r.settings()
			triggerVK, _ = ActiveTrigger(triggerVKs)
			if err := runCycle(ctx, keyStream, mouseStream, triggerVK, delay); err != nil {
				if ctx.Err() != nil {
					return
				}
				r.log(fmt.Sprintf("Loop step failed: %v", err))
				return
			}
		}

		r.log("Loop paused")
	}
}

func ensureBus(ctx context.Context, api *viiperclient.Client, log func(string)) (uint32, bool, error) {
	busesResp, err := api.BusListCtx(ctx)
	if err != nil {
		return 0, false, err
	}

	if len(busesResp.Buses) > 0 {
		busID := busesResp.Buses[0]
		for _, b := range busesResp.Buses[1:] {
			if b < busID {
				busID = b
			}
		}

		devices, err := api.DevicesListCtx(ctx, busID)
		if err == nil {
			for _, dev := range devices.Devices {
				if _, err := api.DeviceRemoveCtx(ctx, busID, dev.DevID); err != nil {
					log(fmt.Sprintf("failed to remove stale device %s: %v", dev.DevID, err))
				} else {
					log(fmt.Sprintf("removed stale device %d-%s", busID, dev.DevID))
				}
			}
		}

		log(fmt.Sprintf("using existing VIIPER bus %d", busID))
		return busID, false, nil
	}

	resp, err := api.BusCreateCtx(ctx, 0)
	if err != nil {
		return 0, false, err
	}
	log(fmt.Sprintf("created VIIPER bus %d", resp.BusID))
	return resp.BusID, true, nil
}

func runCycle(ctx context.Context, keyStream, mouseStream *viiperclient.DeviceStream, vk int32, delay time.Duration) error {
	step := time.Duration(StepHoldMs) * time.Millisecond

	if err := keyDown(keyStream, vk); err != nil {
		return err
	}
	sleep(ctx, delay)

	if err := mouseDown(mouseStream); err != nil {
		return err
	}
	sleep(ctx, step)

	if err := keyUp(keyStream); err != nil {
		return err
	}
	sleep(ctx, step)

	return mouseUp(mouseStream)
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

func keyDown(stream *viiperclient.DeviceStream, vk int32) error {
	hid, ok := VKToHID(vk)
	if !ok {
		return fmt.Errorf("unsupported trigger key %s", KeyName(vk))
	}
	press := keyboard.PressKey(hid)
	return stream.WriteBinary(&press)
}

func keyUp(stream *viiperclient.DeviceStream) error {
	release := keyboard.Release()
	return stream.WriteBinary(&release)
}

func mouseDown(stream *viiperclient.DeviceStream) error {
	return stream.WriteBinary(&mouse.InputState{Buttons: mouse.BtnLeft})
}

func mouseUp(stream *viiperclient.DeviceStream) error {
	return stream.WriteBinary(&mouse.InputState{})
}

func cleanupDevice(ctx context.Context, api *viiperclient.Client, busID uint32, devID string, log func(string)) {
	if _, err := api.DeviceRemoveCtx(ctx, busID, devID); err != nil {
		log(fmt.Sprintf("device remove %d-%s failed: %v", busID, devID, err))
		return
	}
	log(fmt.Sprintf("removed device %d-%s", busID, devID))
}

func cleanupBus(ctx context.Context, api *viiperclient.Client, busID uint32, created bool, log func(string)) {
	if !created {
		return
	}
	if _, err := api.BusRemoveCtx(ctx, busID); err != nil {
		log(fmt.Sprintf("bus remove %d failed: %v", busID, err))
		return
	}
	log(fmt.Sprintf("removed bus %d", busID))
}
