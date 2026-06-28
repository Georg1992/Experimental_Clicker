package runner

import (
	"context"
	"fmt"
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

type AutoPotRunner struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	running bool

	liveMu sync.RWMutex
	live   AutoPotConfig

	hpStabilizer     *BarStabilizer
	spStabilizer     *BarStabilizer
	numericValidator *NumericSafetyValidator
}

func NewAutoPot(cfg AutoPotConfig) *AutoPotRunner {
	return &AutoPotRunner{
		live:             cfg,
		hpStabilizer:     NewBarStabilizer(true, cfg.HPThreshold),
		spStabilizer:     NewBarStabilizer(false, cfg.SPThreshold),
		numericValidator: NewNumericSafetyValidator(),
	}
}

func (a *AutoPotRunner) Running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *AutoPotRunner) UpdateSettings(cfg AutoPotConfig) {
	a.liveMu.Lock()
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
	if cfg.Log == nil {
		a.mu.Unlock()
		return fmt.Errorf("log callback is required")
	}

	a.resetStabilizers()

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.running = true
	a.done = make(chan struct{})

	// Initialize numeric validator
	a.numericValidator.SetLogFunc(cfg.Log)
	a.numericValidator.Start(ctx)

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

func (a *AutoPotRunner) run(ctx context.Context) {
	for {
		cfg := a.settings()
		session := cfg.Session
		if session == nil {
			return
		}

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

		mapped, pairOK := refreshStableBarPair(img)

		hp := a.hpStabilizer.UpdatePair(img, true, mapped, pairOK)
		if cfg.HPEnabled && hp.Status == BarStatusLow {
			a.healUntil(ctx, session, true)
			continue
		}

		sp := a.spStabilizer.UpdatePair(img, false, mapped, pairOK)
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
		mapped, pairOK := refreshStableBarPair(img)
		read := stabilizer.UpdatePair(img, hpBar, mapped, pairOK)
		if read.Status != BarStatusLow {
			return
		}
		before := read.Percent

		// Check numeric validator gate before potting (negative-only gate)
		threshold := cfg.HPThreshold
		potionKind := PotionHP
		if !hpBar {
			threshold = cfg.SPThreshold
			potionKind = PotionSP
		}
		
		blockDecision := a.numericValidator.ShouldBlockPotion(potionKind, threshold)
		if blockDecision.Block {
			// Log the numeric block with detailed information
			var potionName string
			if hpBar {
				potionName = "hp"
			} else {
				potionName = "sp"
			}
			cfg.Log(fmt.Sprintf("numeric_block kind=%s percent=%.1f threshold=%d confidence=%.2f age_ms=%d reason=%s",
				potionName, blockDecision.Percent, threshold, blockDecision.Confidence, blockDecision.AgeMs, blockDecision.Reason))
			return
		}

		if err := session.TapKey(vk, KeyTapHold); err != nil {
			cfg.Log(fmt.Sprintf("Key %s failed: %v", KeyName(vk), err))
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
			cfg = a.settings()
			if _, ok := healTarget(cfg, hpBar); !ok {
				return
			}
			img, _, err := CapturePlayerBarSearch()
			if err != nil {
				continue
			}
			mapped, pairOK := refreshStableBarPair(img)
			read := stabilizer.UpdatePair(img, hpBar, mapped, pairOK)
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
