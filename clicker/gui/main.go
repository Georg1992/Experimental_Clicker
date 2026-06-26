//go:build windows

//go:generate go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -o rsrc.syso

package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
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
	keyLabel      *walk.Label
	delayEdit     *walk.LineEdit
	mouseClickCB  *walk.CheckBox
	startBtn      *walk.PushButton
	stopBtn       *walk.PushButton
	statusBadge   *statusBadge
	bindBtn       *walk.PushButton
	clearBtn      *walk.PushButton

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

	mu              sync.Mutex
	shutdownOnce    sync.Once
	runner          *runner.Runner
	autopotRunner   *runner.AutoPotRunner
	timerKeyRunner  *runner.TimerKeyRunner
	inputSession    *runner.ViiperSession
	triggerVKs      []int32
	hpKeyVK         int32
	spKeyVK         int32
	binding         bool
	autopotBinding  bool
	lastLoggedDelay int
}

func main() {
	app := &guiApp{timerBindingSlot: -1}
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
		session := a.inputSession
		a.runner = nil
		a.autopotRunner = nil
		a.timerKeyRunner = nil
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
	if err := mw.SetMinMaxSize(walk.Size{Width: 580, Height: 520}, walk.Size{}); err != nil {
		return err
	}
	if err := mw.SetSize(walk.Size{Width: 580, Height: 520}); err != nil {
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
	a.logList.Synchronize(func() {
		a.logItems = append(a.logItems, stamped)
		_ = a.logList.SetModel(a.logItems)
		if len(a.logItems) > 0 {
			_ = a.logList.SetCurrentIndex(len(a.logItems) - 1)
		}
	})
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
	a.setConfigEnabled(started)
	a.setAutoPotConfigEnabled(started)
	a.setTimerKeyConfigEnabled(started)
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

func (a *guiApp) setConfigEnabled(enabled bool) {
	if a.binding {
		enabled = false
	}
	a.bindBtn.SetEnabled(enabled)
	a.clearBtn.SetEnabled(enabled)
	a.delayEdit.SetEnabled(enabled)
	a.mouseClickCB.SetEnabled(enabled)
}

func (a *guiApp) syncRunnerSettings() {
	a.mu.Lock()
	r := a.runner
	vks := append([]int32(nil), a.triggerVKs...)
	delay := a.delayMs()
	a.mu.Unlock()

	if r != nil && r.Running() {
		r.UpdateSettings(vks, delay, a.mouseClickCB.Checked())
	}
}

func (a *guiApp) updateKeyLabel() {
	a.keyLabel.SetText(runner.KeysText(a.triggerVKs))
}

func (a *guiApp) delayMs() int {
	text := a.delayEdit.Text()
	delay, err := strconv.Atoi(text)
	if err != nil || delay <= 0 {
		return runner.DefaultDelayMs
	}
	return delay
}

func (a *guiApp) logDelayIfChanged() {
	delay := a.delayMs()
	if delay == a.lastLoggedDelay {
		return
	}
	a.lastLoggedDelay = delay
	a.appendLog(fmt.Sprintf("Delay: %d ms", delay))
}

func (a *guiApp) onClearKeys() {
	if !a.isStarted() {
		return
	}
	a.triggerVKs = nil
	a.updateKeyLabel()
	a.syncRunnerSettings()
	a.appendLog("Trigger keys cleared")
}

func (a *guiApp) onBindKey() {
	if !a.isStarted() || a.binding {
		return
	}
	a.binding = true
	a.setConfigEnabled(false)
	a.appendLog("Press a key to add (5s timeout)...")

	go func() {
		defer func() {
			a.binding = false
			a.mainWindow.Synchronize(func() {
				a.setConfigEnabled(a.isStarted())
			})
		}()

		vk, ok := runner.WaitForKeyPress(5 * time.Second)
		a.mainWindow.Synchronize(func() {
			if !ok {
				a.appendLog("Key bind timed out")
				return
			}
			if _, hidOK := runner.VKToHID(vk); !hidOK {
				a.appendLog(fmt.Sprintf("Key %s is not supported", runner.KeyName(vk)))
				return
			}
			if slices.Contains(a.triggerVKs, vk) {
				a.appendLog(fmt.Sprintf("Key %s is already bound", runner.KeyName(vk)))
				return
			}
			a.triggerVKs = append(a.triggerVKs, vk)
			a.updateKeyLabel()
			a.syncRunnerSettings()
			a.appendLog(fmt.Sprintf("Added trigger key %s", runner.KeyName(vk)))
		})
	}()
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

	cfg := runner.Config{
		Session:    session,
		TriggerVKs: append([]int32(nil), a.triggerVKs...),
		DelayMs:    a.delayMs(),
		MouseClick: a.mouseClickCB.Checked(),
		Log:        a.appendLog,
	}
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
	a.mu.Unlock()

	a.startAutoPotRunner(autopotCfg)
	a.startTimerKeyRunner(timerCfg)
	a.setStarted(true)
	a.appendLog("Started")
}

func (a *guiApp) onStop() {
	a.mu.Lock()
	r := a.runner
	ap := a.autopotRunner
	tk := a.timerKeyRunner
	session := a.inputSession
	a.runner = nil
	a.autopotRunner = nil
	a.timerKeyRunner = nil
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
		if session != nil {
			session.Close()
		}
		stopViiperServerIfStarted()
		a.mainWindow.Synchronize(func() {
			a.appendLog("Clicker stopped — click Start before launching the game")
		})
	}()
}
