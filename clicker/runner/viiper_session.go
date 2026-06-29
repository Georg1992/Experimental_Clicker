package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	windows "experimental-clicker/runner/platform/windows"

	"github.com/Alia5/VIIPER/device/keyboard"
	"github.com/Alia5/VIIPER/device/mouse"
	"github.com/Alia5/VIIPER/viiperclient"
)

// ViiperSession owns one VIIPER bus with a shared keyboard and mouse.
// All runners use the same devices; writes are serialized.
type ViiperSession struct {
	api        *viiperclient.Client
	busID      uint32
	createdBus bool

	writeMu     sync.Mutex
	keyStream   *viiperclient.DeviceStream
	mouseStream *viiperclient.DeviceStream

	pauseMu        sync.RWMutex
	paused         bool
	onPauseChanged func(bool)
	pauseCancel    context.CancelFunc
	pauseDone      chan struct{}

	closeOnce sync.Once
}

func OpenViiperSession(ctx context.Context, apiAddr string) (*ViiperSession, error) {
	if apiAddr == "" {
		apiAddr = DefaultAPIAddr
	}

	api := viiperclient.New(apiAddr)
	if _, err := api.PingCtx(ctx); err != nil {
		return nil, fmt.Errorf("viiper ping: %w", err)
	}

	busID, createdBus, err := ensureBus(ctx, api, noopLog)
	if err != nil {
		return nil, err
	}

	keyStream, _, err := api.AddDeviceAndConnect(ctx, busID, "keyboard", nil)
	if err != nil {
		if createdBus {
			cleanupBus(ctx, api, busID, true, noopLog)
		}
		return nil, fmt.Errorf("keyboard: %w", err)
	}

	mouseStream, _, err := api.AddDeviceAndConnect(ctx, busID, "mouse", nil)
	if err != nil {
		_ = keyStream.Close()
		cleanupDevice(ctx, api, keyStream.BusID, keyStream.DevID, noopLog)
		if createdBus {
			cleanupBus(ctx, api, busID, true, noopLog)
		}
		return nil, fmt.Errorf("mouse: %w", err)
	}

	return &ViiperSession{
		api:         api,
		busID:       busID,
		createdBus:  createdBus,
		keyStream:   keyStream,
		mouseStream: mouseStream,
	}, nil
}

func (s *ViiperSession) SetOnPauseChanged(fn func(bool)) {
	s.pauseMu.Lock()
	s.onPauseChanged = fn
	s.pauseMu.Unlock()
}

func (s *ViiperSession) Paused() bool {
	s.pauseMu.RLock()
	defer s.pauseMu.RUnlock()
	return s.paused
}

func (s *ViiperSession) StartPauseWatcher(ctx context.Context, log func(string)) {
	if log == nil {
		log = noopLog
	}
	watchCtx, cancel := context.WithCancel(ctx)
	s.pauseCancel = cancel
	s.pauseDone = make(chan struct{})

	go func() {
		defer close(s.pauseDone)
		pauseKeyDown := false
		for watchCtx.Err() == nil {
			if windows.PollKeyToggle(&pauseKeyDown, PauseVK) {
				s.togglePaused(log)
			}
			time.Sleep(PollInterval)
		}
	}()
}

func (s *ViiperSession) togglePaused(log func(string)) {
	s.pauseMu.Lock()
	s.paused = !s.paused
	paused := s.paused
	onChange := s.onPauseChanged
	s.pauseMu.Unlock()

	s.ReleaseAll()
	if onChange != nil {
		onChange(paused)
	}
	if paused {
		log("Paused (End to resume)")
	} else {
		log("Resumed")
	}
}

func (s *ViiperSession) Close() {
	s.closeOnce.Do(func() {
		if s.pauseCancel != nil {
			s.pauseCancel()
			<-s.pauseDone
		}
		ctx, cancel := context.WithTimeout(context.Background(), SessionCloseWait)
		defer cancel()

		s.writeMu.Lock()
		_ = keyUpLocked(s.keyStream)
		_ = mouseUpLocked(s.mouseStream)
		s.writeMu.Unlock()

		_ = s.keyStream.Close()
		_ = s.mouseStream.Close()
		cleanupDevice(ctx, s.api, s.keyStream.BusID, s.keyStream.DevID, noopLog)
		cleanupDevice(ctx, s.api, s.mouseStream.BusID, s.mouseStream.DevID, noopLog)
		cleanupBus(ctx, s.api, s.busID, s.createdBus, noopLog)
	})
}

func (s *ViiperSession) KeyDown(vk int32) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return keyDownLocked(s.keyStream, vk)
}

func (s *ViiperSession) KeyUp() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return keyUpLocked(s.keyStream)
}

func (s *ViiperSession) TapKey(vk int32, hold time.Duration) error {
	if err := s.KeyDown(vk); err != nil {
		return err
	}
	time.Sleep(hold)
	return s.KeyUp()
}

func (s *ViiperSession) MouseDown() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return mouseDownLocked(s.mouseStream)
}

func (s *ViiperSession) MouseUp() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return mouseUpLocked(s.mouseStream)
}

func (s *ViiperSession) ReleaseAll() {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = keyUpLocked(s.keyStream)
	_ = mouseUpLocked(s.mouseStream)
}

func keyDownLocked(stream *viiperclient.DeviceStream, vk int32) error {
	hid, ok := VKToHID(vk)
	if !ok {
		return fmt.Errorf("unsupported trigger key %s", KeyName(vk))
	}
	press := keyboard.PressKey(hid)
	return stream.WriteBinary(&press)
}

func keyUpLocked(stream *viiperclient.DeviceStream) error {
	release := keyboard.Release()
	return stream.WriteBinary(&release)
}

func mouseDownLocked(stream *viiperclient.DeviceStream) error {
	return stream.WriteBinary(&mouse.InputState{Buttons: mouse.BtnLeft})
}

func mouseUpLocked(stream *viiperclient.DeviceStream) error {
	return stream.WriteBinary(&mouse.InputState{})
}

var noopLog = func(string) {}

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
		return busID, false, nil
	}

	resp, err := api.BusCreateCtx(ctx, 0)
	if err != nil {
		return 0, false, err
	}
	log(fmt.Sprintf("created VIIPER bus %d", resp.BusID))
	return resp.BusID, true, nil
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
