//go:build windows

//go:generate go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -o rsrc.syso

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"experimental-clicker/runner"
	"github.com/lxn/walk"
)

type guiApp struct {
	mainWindow *walk.MainWindow
	logList    *walk.ListBox
	logItems   []string

	// Clicker tab
	clickerSlots           [runner.ClickerSlotCount]clickerSlotWidgets
	clickerTriggerVKs      [runner.ClickerSlotCount][]int32
	clickerBindingSlot     int
	clickerLastLoggedDelay [runner.ClickerSlotCount]int
	startBtn                *walk.PushButton
	stopBtn                 *walk.PushButton
	statusBadge             *statusBadge

	// Timer keys (clicker tab)
	timerSlots         [runner.TimerKeySlotCount]timerSlotWidgets
	timerKeyVKs        [runner.TimerKeySlotCount]int32
	timerVisibleCount  int
	timerAddBtn        *walk.PushButton
	timerBindingSlot   int

	// AutoPot tab
	hpEnabledCB     *walk.CheckBox
	spEnabledCB        *walk.CheckBox
	hpThresholdEdit    *walk.LineEdit
	spThresholdEdit    *walk.LineEdit
	hpKeyLabel         *walk.Label
	spKeyLabel         *walk.Label
	hpBindBtn          *walk.PushButton
	hpClearBtn         *walk.PushButton
	spBindBtn          *walk.PushButton
	spClearBtn         *walk.PushButton

	// KeyChain tab
	keyChainSlots        [runner.KeyChainSlotCount]keyChainSlotWidgets
	keyChainKeyVKs       [runner.KeyChainSlotCount]int32
	keyChainClearBtn     *walk.PushButton
	keyChainBindingSlot  int

	mu              sync.Mutex
	shutdownOnce    sync.Once
	runner          *runner.Runner
	autopotRunner   *runner.AutoPotRunner
	timerKeyRunner  *runner.TimerKeyRunner
	keyChainRunner  *runner.KeyChainRunner
	inputSession    *runner.ViiperSession
	hpKeyVK         int32
	spKeyVK         int32
	autopotBinding  bool
	lastAppliedHPThreshold  int
	lastAppliedSPThreshold  int
}

func main() {
	app := &guiApp{timerBindingSlot: -1, keyChainBindingSlot: -1, clickerBindingSlot: -1}
	defer app.shutdown()

	if err := app.createWindow(); err != nil {
		walk.MsgBox(nil, "BELARUS CHAMP CLICKER", err.Error(), walk.MsgBoxIconError)
	}
}

func (a *guiApp) shutdown() {
	a.shutdownOnce.Do(func() {
		a.mu.Lock()
		r := a.runner
		ap := a.autopotRunner
		tk := a.timerKeyRunner
		kc := a.keyChainRunner
		session := a.inputSession
		a.runner = nil
		a.autopotRunner = nil
		a.timerKeyRunner = nil
		a.keyChainRunner = nil
		a.inputSession = nil
		a.mu.Unlock()

		if r != nil {
			r.Stop()
			r.Wait()
		}
		if ap != nil {
			ap.Stop()
			ap.Wait()
		}
		if tk != nil {
			tk.Stop()
			tk.Wait()
		}
		if kc != nil {
			kc.Stop()
			kc.Wait()
		}
		if session != nil {
			session.Close()
		}

		stopViiperServerIfStarted()
	})
}

func (a *guiApp) createWindow() error {
	mw, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	a.mainWindow = mw

	if err := mw.SetTitle("BELARUS CHAMP CLICKER"); err != nil {
		return err
	}
	if err := mw.SetMinMaxSize(walk.Size{Width: 780, Height: 600}, walk.Size{}); err != nil {
		return err
	}
	if err := mw.SetSize(walk.Size{Width: 780, Height: 600}); err != nil {
		return err
	}

	root := walk.NewVBoxLayout()
	root.SetMargins(walk.Margins{HNear: 10, VNear: 10, HFar: 10, VFar: 10})
	root.SetSpacing(10)
	if err := mw.SetLayout(root); err != nil {
		return err
	}

	icon, err := walk.NewIconFromImage(belarusFlagImage())
	if err != nil {
		return err
	}
	if err := mw.SetIcon(icon); err != nil {
		return err
	}

	if err := addBelarusHeader(mw); err != nil {
		return err
	}

	if err := a.buildControlPanel(mw); err != nil {
		return err
	}

	tabs, err := walk.NewTabWidget(mw)
	if err != nil {
		return err
	}

	clickerPage, err := walk.NewTabPage()
	if err != nil {
		return err
	}
	if err := clickerPage.SetTitle("Clicker"); err != nil {
		return err
	}
	if err := a.buildClickerTab(clickerPage); err != nil {
		return err
	}
	if err := tabs.Pages().Add(clickerPage); err != nil {
		return err
	}

	autopotPage, err := walk.NewTabPage()
	if err != nil {
		return err
	}
	if err := autopotPage.SetTitle("AutoPot"); err != nil {
		return err
	}
	if err := a.buildAutoPotTab(autopotPage); err != nil {
		return err
	}
	if err := tabs.Pages().Add(autopotPage); err != nil {
		return err
	}

	keyChainPage, err := walk.NewTabPage()
	if err != nil {
		return err
	}
	if err := keyChainPage.SetTitle("KeyChain"); err != nil {
		return err
	}
	if err := a.buildKeyChainTab(keyChainPage); err != nil {
		return err
	}
	if err := tabs.Pages().Add(keyChainPage); err != nil {
		return err
	}

	tabs.CurrentIndexChanged().Attach(a.finishThresholdInput)

	mw.Deactivating().Attach(a.finishThresholdInput)

	logLabel, err := walk.NewLabel(mw)
	if err != nil {
		return err
	}
	if err := logLabel.SetText("Logs"); err != nil {
		return err
	}

	a.logList, err = walk.NewListBox(mw)
	if err != nil {
		return err
	}
	if err := a.logList.SetMinMaxSize(walk.Size{Width: 0, Height: 140}, walk.Size{}); err != nil {
		return err
	}
	a.logItems = []string{}
	if err := a.logList.SetModel(a.logItems); err != nil {
		return err
	}
	a.wireThresholdBlurOnClick(mw)
	a.setStarted(false)

	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		a.shutdown()
	})

	mw.Show()
	mw.Run()
	return nil
}

func (a *guiApp) appendLog(line string) {
	if a.logList == nil {
		return
	}
	stamped := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), line)
	a.logItems = append(a.logItems, stamped)
	_ = a.logList.SetModel(a.logItems)
	if len(a.logItems) > 0 {
		_ = a.logList.SetCurrentIndex(len(a.logItems) - 1)
	}
}

func (a *guiApp) isStarted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.runner != nil && a.runner.Running() {
		return true
	}
	if a.autopotRunner != nil && a.autopotRunner.Running() {
		return true
	}
	return false
}

func (a *guiApp) autopotWanted() runner.AutoPotConfig {
	cfg := a.autopotConfig()
	cfg.HPEnabled = cfg.HPEnabled && cfg.HPKeyVK != 0
	cfg.SPEnabled = cfg.SPEnabled && cfg.SPKeyVK != 0
	return cfg
}

func (a *guiApp) startAutoPotRunner(cfg runner.AutoPotConfig) {
	if !cfg.HPEnabled && !cfg.SPEnabled {
		return
	}
	a.mu.Lock()
	if a.autopotRunner != nil && a.autopotRunner.Running() {
		a.mu.Unlock()
		return
	}
	old := a.autopotRunner
	a.autopotRunner = nil
	a.mu.Unlock()

	if old != nil {
		old.Stop()
		old.Wait()
	}

	cfg.Log = a.appendLog
	a.mu.Lock()
	cfg.Session = a.inputSession
	a.mu.Unlock()
	if cfg.Session == nil {
		return
	}
	ap := runner.NewAutoPot(cfg)
	if err := ap.Start(); err != nil {
		a.appendLog(fmt.Sprintf("AutoPot start failed: %v", err))
		return
	}
	a.mu.Lock()
	a.autopotRunner = ap
	a.mu.Unlock()
	a.appendLog("AutoPot started")
}

func (a *guiApp) setStarted(started bool) {
	a.startBtn.SetEnabled(!started)
	a.stopBtn.SetEnabled(started)
	a.setClickerConfigEnabled(started)
	a.setAutoPotConfigEnabled(started)
	a.setTimerKeyConfigEnabled(started)
	a.setKeyChainConfigEnabled(started)
	if started {
		a.setClickerStatus(clickerStatusRunning)
	} else {
		a.setClickerStatus(clickerStatusStopped)
	}
}

func (a *guiApp) setClickerStatus(status clickerStatus) {
	update := func() {
		if a.statusBadge != nil {
			a.statusBadge.SetStatus(status)
		}
	}

	if a.mainWindow != nil {
		a.mainWindow.Synchronize(update)
	} else {
		update()
	}
}

func (a *guiApp) syncRunnerSettings() {
	cfg := a.clickerConfig()
	a.mu.Lock()
	r := a.runner
	a.mu.Unlock()

	if r != nil && r.Running() {
		r.UpdateSettings(cfg.Slots)
	}
}

func (a *guiApp) onStart() {
	a.mu.Lock()

	if (a.runner != nil && a.runner.Running()) || (a.autopotRunner != nil && a.autopotRunner.Running()) {
		a.mu.Unlock()
		return
	}

	if ready, msg := inputDriverReady(); !ready {
		a.mu.Unlock()
		a.appendLog("Input driver not ready — see Setup required dialog.")
		walk.MsgBox(a.mainWindow, "Setup required", msg, walk.MsgBoxIconWarning)
		return
	}

	a.appendLog("Starting...")

	started, err := ensureViiperServer()
	if err != nil {
		a.mu.Unlock()
		a.appendLog(fmt.Sprintf("Start failed: %v", err))
		return
	}
	if started {
		a.appendLog("Ready")
	}

	session, err := runner.OpenViiperSession(context.Background(), runner.DefaultAPIAddr)
	if err != nil {
		a.mu.Unlock()
		a.appendLog(fmt.Sprintf("Start failed: %v", err))
		stopViiperServerIfStarted()
		return
	}
	a.inputSession = session
	session.SetOnPauseChanged(func(paused bool) {
		if paused {
			a.setClickerStatus(clickerStatusPaused)
		} else {
			a.setClickerStatus(clickerStatusRunning)
		}
	})
	session.StartPauseWatcher(context.Background(), a.appendLog)

	cfg := a.clickerConfig()
	cfg.Session = session
	cfg.Log = a.appendLog
	a.runner = runner.New(cfg)
	if err := a.runner.Start(); err != nil {
		a.appendLog(fmt.Sprintf("Start failed: %v", err))
		a.runner = nil
		a.inputSession.Close()
		a.inputSession = nil
		a.mu.Unlock()
		stopViiperServerIfStarted()
		return
	}

	autopotCfg := a.autopotWanted()
	timerCfg := a.timerKeyWanted()
	keyChainCfg := a.keyChainConfig()
	a.mu.Unlock()

	a.startAutoPotRunner(autopotCfg)
	a.seedAppliedThresholds(autopotCfg)
	a.startTimerKeyRunner(timerCfg)
	a.startKeyChainRunner(keyChainCfg)
	a.setStarted(true)
	a.appendLog("Started")
}

func (a *guiApp) onStop() {
	a.mu.Lock()
	r := a.runner
	ap := a.autopotRunner
	tk := a.timerKeyRunner
	kc := a.keyChainRunner
	session := a.inputSession
	a.runner = nil
	a.autopotRunner = nil
	a.timerKeyRunner = nil
	a.keyChainRunner = nil
	a.inputSession = nil
	a.mu.Unlock()

	a.setStarted(false)
	a.appendLog("Stopping...")

	go func() {
		if r != nil {
			r.Stop()
			r.Wait()
		}
		if ap != nil {
			ap.Stop()
			ap.Wait()
		}
		if tk != nil {
			tk.Stop()
			tk.Wait()
		}
		if kc != nil {
			kc.Stop()
			kc.Wait()
		}
		if session != nil {
			session.Close()
		}
		stopViiperServerIfStarted()
		a.mainWindow.Synchronize(func() {
			a.appendLog("Clicker stopped — click Start before launching the game")
		})
	}()
}
