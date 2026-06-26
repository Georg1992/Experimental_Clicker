package runner

import (
	"context"
	"fmt"
	"image"
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

	barsMu sync.RWMutex
	bars   MappedBars
}

func NewAutoPot(cfg AutoPotConfig) *AutoPotRunner {
	cfg.applyDefaults()
	return &AutoPotRunner{cfg: cfg, live: cfg}
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
}

func (a *AutoPotRunner) settings() AutoPotConfig {
	a.liveMu.RLock()
	defer a.liveMu.RUnlock()
	return a.live
}

func (a *AutoPotRunner) mappedBars() MappedBars {
	a.barsMu.RLock()
	defer a.barsMu.RUnlock()
	return a.bars
}

func (a *AutoPotRunner) setMappedBars(bars MappedBars) {
	a.barsMu.Lock()
	a.bars = bars
	a.barsMu.Unlock()
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
	a.cfg.Log(msg)
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

		bars, hp, sp, refreshed := a.readBars(img)
		if debugSave {
			_ = SaveMappedBarsDebug(img, bars, "bar_search_debug.png")
			a.log(FormatMappedBarsLog(img, bars, hp, sp, refreshed))
		}

		cfg := a.settings()

		if cfg.HPEnabled && hp.Found && hp.Percent < float64(cfg.HPThreshold) {
			a.healUntil(ctx, session, cfg.HPKeyVK, cfg.HPThreshold, true)
			continue
		}

		if cfg.SPEnabled && sp.Found && sp.Percent < float64(cfg.SPThreshold) {
			a.healUntil(ctx, session, cfg.SPKeyVK, cfg.SPThreshold, false)
			continue
		}

		sleep(ctx, KeyTapHold)
	}
}

func (a *AutoPotRunner) readBars(img image.Image) (bars MappedBars, hp, sp BarRead, refreshed bool) {
	bars = a.mappedBars()
	hp, sp = ReadMappedBars(img, bars)
	if NeedsRemap(img, bars, hp, sp) {
		mapped, err := RefreshBarPair(img)
		if err == nil {
			a.setMappedBars(mapped)
			bars = mapped
			hp, sp = ReadMappedBars(img, bars)
			refreshed = true
		} else {
			a.setMappedBars(MappedBars{})
			return MappedBars{}, BarRead{}, BarRead{}, false
		}
	}
	return bars, hp, sp, refreshed
}

func (a *AutoPotRunner) healUntil(ctx context.Context, session *ViiperSession, vk int32, threshold int, hpBar bool) {
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
			sleep(ctx, PollInterval)
			continue
		}
		_, read := a.readOneBar(img, hpBar)
		if !read.Found || read.Percent >= float64(threshold) {
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
			img, _, err := CapturePlayerBarSearch()
			if err != nil {
				continue
			}
			_, read := a.readOneBar(img, hpBar)
			if !read.Found || read.Percent >= float64(threshold) {
				return
			}
			if read.Percent > before {
				break
			}
		}
	}
}

func (a *AutoPotRunner) readOneBar(img image.Image, hpBar bool) (MappedBars, BarRead) {
	bars, hp, sp, _ := a.readBars(img)
	if hpBar {
		return bars, hp
	}
	return bars, sp
}
