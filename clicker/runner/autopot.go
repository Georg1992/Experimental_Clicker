package runner

import (
	"context"
	"fmt"
	"os"
	"sync"
)

type AutoPotConfig struct {
	Session     *ViiperSession
	HPThreshold int
	SPThreshold int
	HPKeyVK     int32
	SPKeyVK     int32
	HPEnabled   bool
	SPEnabled   bool
	Log         func(string)
}

func (c *AutoPotConfig) applyDefaults() {
	if c.Log == nil {
		c.Log = func(string) {}
	}
}

type AutoPotRunner struct {
	cfg AutoPotConfig

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	running bool

	liveMu sync.RWMutex
	live   AutoPotConfig

	pair         *BarPairCache
	hpStabilizer *BarStabilizer
	spStabilizer *BarStabilizer
}

func NewAutoPot(cfg AutoPotConfig) *AutoPotRunner {
	cfg.applyDefaults()
	pair := &BarPairCache{}
	return &AutoPotRunner{
		cfg:          cfg,
		live:         cfg,
		pair:         pair,
		hpStabilizer: NewBarStabilizer(pair, true, cfg.HPThreshold),
		spStabilizer: NewBarStabilizer(pair, false, cfg.SPThreshold),
	}
}

func (a *AutoPotRunner) Running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *AutoPotRunner) UpdateSettings(cfg AutoPotConfig) {
	cfg.applyDefaults()
	a.liveMu.Lock()
	if cfg.Session == nil {
		cfg.Session = a.live.Session
	}
	if cfg.Log == nil {
		cfg.Log = a.live.Log
	}
	a.live = cfg
	a.liveMu.Unlock()
	a.hpStabilizer.SetThreshold(cfg.HPThreshold)
	a.spStabilizer.SetThreshold(cfg.SPThreshold)
}

func (a *AutoPotRunner) settings() AutoPotConfig {
	a.liveMu.RLock()
	defer a.liveMu.RUnlock()
	return a.live
}

func (a *AutoPotRunner) resetStabilizers() {
	a.pair.Reset()
	a.hpStabilizer.Reset()
	a.spStabilizer.Reset()
}

func (a *AutoPotRunner) Start() error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("autopot already running")
	}

	cfg := a.settings()
	if cfg.HPEnabled && cfg.HPKeyVK == 0 {
		a.mu.Unlock()
		return fmt.Errorf("HP potion key is not set")
	}
	if cfg.SPEnabled && cfg.SPKeyVK == 0 {
		a.mu.Unlock()
		return fmt.Errorf("SP potion key is not set")
	}
	if cfg.Session == nil {
		a.mu.Unlock()
		return fmt.Errorf("input session is required")
	}

	a.resetStabilizers()

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.running = true
	a.done = make(chan struct{})
	a.mu.Unlock()

	go func() {
		defer close(a.done)
		defer func() {
			a.mu.Lock()
			a.running = false
			a.cancel = nil
			a.mu.Unlock()
			a.resetStabilizers()
		}()
		a.run(ctx)
	}()

	return nil
}

func (a *AutoPotRunner) Stop() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *AutoPotRunner) Wait() {
	a.mu.Lock()
	done := a.done
	a.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (a *AutoPotRunner) log(msg string) {
	a.settings().Log(msg)
}

func (a *AutoPotRunner) run(ctx context.Context) {
	session := a.cfg.Session
	debugSave := os.Getenv("BAR_SEARCH_DEBUG") != ""

	for {
		if ctx.Err() != nil {
			return
		}
		if session.Paused() {
			sleep(ctx, PollInterval)
			continue
		}

		img, _, err := CapturePlayerBarSearch()
		if err != nil {
			sleep(ctx, CaptureRetryDelay)
			continue
		}

		if debugSave {
			bars, err := RefreshBarPair(img)
			if err == nil {
				hp, sp := ReadMappedBars(img, bars)
				_ = SaveMappedBarsDebug(img, bars, "bar_search_debug.png")
				a.log(FormatMappedBarsLog(img, bars, hp, sp, true))
			}
		}

		cfg := a.settings()

		hp := a.hpStabilizer.Update(img, true)
		if cfg.HPEnabled && hp.Status == BarStatusLow {
			a.healUntil(ctx, session, true)
			continue
		}

		sp := a.spStabilizer.Update(img, false)
		if cfg.SPEnabled && sp.Status == BarStatusLow {
			a.healUntil(ctx, session, false)
			continue
		}

		sleep(ctx, KeyTapHold)
	}
}

func (a *AutoPotRunner) healUntil(ctx context.Context, session *ViiperSession, hpBar bool) {
	stabilizer := a.spStabilizer
	if hpBar {
		stabilizer = a.hpStabilizer
	}

	for {
		if ctx.Err() != nil {
			return
		}
		if session.Paused() {
			sleep(ctx, PollInterval)
			continue
		}
		cfg := a.settings()
		vk, ok := healTarget(cfg, hpBar)
		if !ok {
			return
		}

		img, _, err := CapturePlayerBarSearch()
		if err != nil {
			sleep(ctx, PollInterval)
			continue
		}
		read := stabilizer.Update(img, hpBar)
		if read.Status != BarStatusLow {
			return
		}
		before := read.Percent
		if err := session.TapKey(vk, KeyTapHold); err != nil {
			a.log(fmt.Sprintf("Key %s failed: %v", KeyName(vk), err))
			return
		}
		for {
			if ctx.Err() != nil {
				return
			}
			if session.Paused() {
				sleep(ctx, PollInterval)
				continue
			}
			if _, ok := healTarget(a.settings(), hpBar); !ok {
				return
			}
			img, _, err := CapturePlayerBarSearch()
			if err != nil {
				continue
			}
			read := stabilizer.Update(img, hpBar)
			if read.Status != BarStatusLow {
				return
			}
			if read.Percent > before {
				break
			}
		}
	}
}

func healTarget(cfg AutoPotConfig, hpBar bool) (vk int32, ok bool) {
	if hpBar {
		if !cfg.HPEnabled || cfg.HPKeyVK == 0 {
			return 0, false
		}
		return cfg.HPKeyVK, true
	}
	if !cfg.SPEnabled || cfg.SPKeyVK == 0 {
		return 0, false
	}
	return cfg.SPKeyVK, true
}
