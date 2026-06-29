package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	windows "experimental-clicker/runner/platform/windows"
)

const KeyChainSlotCount = 7

type KeyChainConfig struct {
	Session  *ViiperSession
	Keys     [KeyChainSlotCount]int32
	DelaysMs [KeyChainSlotCount]int
	Log      func(string)
}

func (c *KeyChainConfig) applyDefaults() {
	if c.Log == nil {
		c.Log = func(string) {}
	}
}

func (c KeyChainConfig) Active() bool {
	return c.Keys[0] != 0
}

type KeyChainRunner struct {
	cfg KeyChainConfig

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	running bool

	liveMu sync.RWMutex
	live   KeyChainConfig
}

func NewKeyChain(cfg KeyChainConfig) *KeyChainRunner {
	cfg.applyDefaults()
	return &KeyChainRunner{cfg: cfg, live: cfg}
}

func (k *KeyChainRunner) Running() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.running
}

func (k *KeyChainRunner) UpdateSettings(cfg KeyChainConfig) {
	cfg.applyDefaults()
	k.liveMu.Lock()
	k.live = cfg
	k.liveMu.Unlock()
}

func (k *KeyChainRunner) settings() KeyChainConfig {
	k.liveMu.RLock()
	defer k.liveMu.RUnlock()
	return k.live
}

func (k *KeyChainRunner) Start() error {
	k.mu.Lock()
	if k.running {
		k.mu.Unlock()
		return fmt.Errorf("keychain already running")
	}
	cfg := k.settings()
	if !cfg.Active() {
		k.mu.Unlock()
		return nil
	}
	if cfg.Session == nil {
		k.mu.Unlock()
		return fmt.Errorf("input session is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	k.cancel = cancel
	k.running = true
	k.done = make(chan struct{})
	k.mu.Unlock()

	go func() {
		defer close(k.done)
		defer func() {
			k.mu.Lock()
			k.running = false
			k.cancel = nil
			k.mu.Unlock()
		}()
		k.run(ctx)
	}()

	return nil
}

func (k *KeyChainRunner) Stop() {
	k.mu.Lock()
	cancel := k.cancel
	k.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (k *KeyChainRunner) Wait() {
	k.mu.Lock()
	done := k.done
	k.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (k *KeyChainRunner) log(msg string) {
	k.cfg.Log(msg)
}

func (k *KeyChainRunner) run(ctx context.Context) {
	session := k.cfg.Session

	for {
		if ctx.Err() != nil {
			return
		}
		if session.Paused() {
			sleep(ctx, PollInterval)
			continue
		}

		cfg := k.settings()
		if !cfg.Active() {
			sleep(ctx, PollInterval)
			continue
		}

		trigger := cfg.Keys[0]
		if !windows.PhysicalKeyDown(trigger) {
			sleep(ctx, PollInterval)
			continue
		}

		for windows.PhysicalKeyDown(trigger) && ctx.Err() == nil {
			if session.Paused() {
				break
			}
			cfg = k.settings()
			if !cfg.Active() {
				break
			}
			if err := k.executeChain(ctx, session, cfg); err != nil {
				if ctx.Err() != nil {
					return
				}
				k.log(fmt.Sprintf("KeyChain failed: %v", err))
				return
			}
		}
	}
}

func (k *KeyChainRunner) executeChain(ctx context.Context, session *ViiperSession, cfg KeyChainConfig) error {
	for i := 0; i < KeyChainSlotCount; i++ {
		if cfg.Keys[i] == 0 {
			continue
		}
		if err := session.TapKey(cfg.Keys[i], KeyTapHold); err != nil {
			return err
		}
		if err := sleepDelay(ctx, cfg.DelaysMs[i]); err != nil {
			return err
		}
	}
	return nil
}

func sleepDelay(ctx context.Context, ms int) error {
	if ms <= 0 {
		return nil
	}
	sleep(ctx, time.Duration(ms)*time.Millisecond)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}
