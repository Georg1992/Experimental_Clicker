// KeyChainRunner plays a sequence of keys when its trigger key is held down.
// Lifecycle driven by internal/lifecycle.
package runner

import (
	"context"
	"fmt"
	"time"

	windows "belarus-champ-tools/runner/platform/windows"

	"belarus-champ-tools/runner/internal/lifecycle"
	"belarus-champ-tools/runner/internal/session"
	"belarus-champ-tools/runner/internal/timing"
)

const KeyChainSlotCount = 7

// KeyChainConfig is what NewKeyChain takes. Session is the canonical
// session.InputSession — same interface other runners use.
type KeyChainConfig struct {
	Session  session.InputSession
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

// KeyChainRunner runs the macro.
type KeyChainRunner struct {
	lc *lifecycle.Lifecycle[KeyChainConfig]
}

// NewKeyChain constructs a KeyChainRunner. Defaults the Log callback.
func NewKeyChain(cfg KeyChainConfig) *KeyChainRunner {
	cfg.applyDefaults()
	return &KeyChainRunner{
		lc: lifecycle.New[KeyChainConfig](
			cfg,
			func(c KeyChainConfig) error {
				if !c.Active() {
					return nil
				}
				if c.Session == nil {
					return fmt.Errorf("input session is required")
				}
				return nil
			},
			nil,
		),
	}
}

func (k *KeyChainRunner) Running() bool { return k.lc.Running() }

func (k *KeyChainRunner) UpdateSettings(cfg KeyChainConfig) {
	cfg.applyDefaults()
	k.lc.UpdateSettings(cfg)
}

func (k *KeyChainRunner) settings() KeyChainConfig { return k.lc.Settings() }

func (k *KeyChainRunner) Start() error {
	if err := k.lc.Start(k.run); err != nil {
		return fmt.Errorf("keychain: %w", err)
	}
	return nil
}

func (k *KeyChainRunner) Stop() { k.lc.Stop() }

func (k *KeyChainRunner) Wait() { k.lc.Wait() }

func (k *KeyChainRunner) run(ctx context.Context, cfg KeyChainConfig) {
	for {
		if ctx.Err() != nil {
			return
		}
		current := k.settings()
		if !current.Active() {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}
		trigger := current.Keys[0]

		if !windows.PhysicalKeyDown(trigger) {
			timing.Sleep(ctx, timing.PollInterval)
			continue
		}

		for windows.PhysicalKeyDown(trigger) && ctx.Err() == nil {
			current = k.settings()
			if !current.Active() {
				break
			}
			if err := k.executeChain(ctx, current); err != nil {
				if ctx.Err() != nil {
					return
				}
				current.Log(fmt.Sprintf("KeyChain failed: %v", err))
				return
			}
		}
	}
}

func (k *KeyChainRunner) executeChain(ctx context.Context, cfg KeyChainConfig) error {
	sess := cfg.Session
	for i := 0; i < KeyChainSlotCount; i++ {
		if cfg.Keys[i] == 0 {
			continue
		}
		if err := sess.TapKey(cfg.Keys[i], timing.KeyTapHold); err != nil {
			return err
		}
		delay := time.Duration(cfg.DelaysMs[i]) * time.Millisecond
		timing.Sleep(ctx, delay)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}
