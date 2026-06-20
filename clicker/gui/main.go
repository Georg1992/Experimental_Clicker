//go:build windows

//go:generate go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -o rsrc.syso

package main

import (
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
	keyLabel   *walk.Label
	delayEdit  *walk.LineEdit
	startBtn   *walk.PushButton
	stopBtn    *walk.PushButton
	bindBtn    *walk.PushButton
	clearBtn   *walk.PushButton

	mu              sync.Mutex
	shutdownOnce    sync.Once
	runner          *runner.Runner
	triggerVKs      []int32
	binding         bool
	lastLoggedDelay int
}

func main() {
	app := &guiApp{}
	defer app.shutdown()

	if err := app.createWindow(); err != nil {
		walk.MsgBox(nil, "BELARUS CHAMP CLICKER", err.Error(), walk.MsgBoxIconError)
	}
}

func (a *guiApp) shutdown() {
	a.shutdownOnce.Do(func() {
		a.mu.Lock()
		r := a.runner
		a.runner = nil
		a.mu.Unlock()

		if r != nil {
			r.Stop()
			r.Wait()
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
	if err := mw.SetMinMaxSize(walk.Size{Width: 560, Height: 480}, walk.Size{}); err != nil {
		return err
	}
	if err := mw.SetSize(walk.Size{Width: 560, Height: 480}); err != nil {
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

	runGB, err := walk.NewGroupBox(mw)
	if err != nil {
		return err
	}
	if err := runGB.SetTitle("1. Start VIIPER"); err != nil {
		return err
	}
	runLayout := walk.NewVBoxLayout()
	runLayout.SetSpacing(8)
	if err := runGB.SetLayout(runLayout); err != nil {
		return err
	}

	btnRow, err := walk.NewComposite(runGB)
	if err != nil {
		return err
	}
	btnHBox := walk.NewHBoxLayout()
	btnHBox.SetSpacing(10)
	if err := btnRow.SetLayout(btnHBox); err != nil {
		return err
	}

	a.startBtn, err = walk.NewPushButton(btnRow)
	if err != nil {
		return err
	}
	if err := a.startBtn.SetText("Start"); err != nil {
		return err
	}
	a.startBtn.Clicked().Attach(a.onStart)

	a.stopBtn, err = walk.NewPushButton(btnRow)
	if err != nil {
		return err
	}
	if err := a.stopBtn.SetText("Stop"); err != nil {
		return err
	}
	a.stopBtn.SetEnabled(false)
	a.stopBtn.Clicked().Attach(a.onStop)

	runHint, err := walk.NewLabel(runGB)
	if err != nil {
		return err
	}
	if err := runHint.SetText("Start before launching the game. This starts the server and creates virtual devices."); err != nil {
		return err
	}

	configGB, err := walk.NewGroupBox(mw)
	if err != nil {
		return err
	}
	if err := configGB.SetTitle("2. Configure clicker"); err != nil {
		return err
	}
	configLayout := walk.NewVBoxLayout()
	configLayout.SetSpacing(8)
	if err := configGB.SetLayout(configLayout); err != nil {
		return err
	}

	keyRow, err := walk.NewComposite(configGB)
	if err != nil {
		return err
	}
	keyHBox := walk.NewHBoxLayout()
	keyHBox.SetSpacing(10)
	if err := keyRow.SetLayout(keyHBox); err != nil {
		return err
	}

	triggerLabel, err := walk.NewLabel(keyRow)
	if err != nil {
		return err
	}
	if err := triggerLabel.SetText("Trigger keys:"); err != nil {
		return err
	}

	a.keyLabel, err = walk.NewLabel(keyRow)
	if err != nil {
		return err
	}
	if err := a.keyLabel.SetText(runner.KeysText(a.triggerVKs)); err != nil {
		return err
	}

	a.bindBtn, err = walk.NewPushButton(keyRow)
	if err != nil {
		return err
	}
	if err := a.bindBtn.SetText("Add key..."); err != nil {
		return err
	}
	a.bindBtn.Clicked().Attach(a.onBindKey)

	a.clearBtn, err = walk.NewPushButton(keyRow)
	if err != nil {
		return err
	}
	if err := a.clearBtn.SetText("Clear keys"); err != nil {
		return err
	}
	a.clearBtn.Clicked().Attach(a.onClearKeys)

	delayRow, err := walk.NewComposite(configGB)
	if err != nil {
		return err
	}
	delayHBox := walk.NewHBoxLayout()
	delayHBox.SetSpacing(10)
	if err := delayRow.SetLayout(delayHBox); err != nil {
		return err
	}

	delayLabel, err := walk.NewLabel(delayRow)
	if err != nil {
		return err
	}
	if err := delayLabel.SetText("Delay (ms):"); err != nil {
		return err
	}

	a.delayEdit, err = walk.NewLineEdit(delayRow)
	if err != nil {
		return err
	}
	a.delayEdit.SetMaxLength(6)
	if err := a.delayEdit.SetMinMaxSize(walk.Size{Width: 80, Height: 0}, walk.Size{Width: 80, Height: 0}); err != nil {
		return err
	}
	if err := a.delayEdit.SetText(strconv.Itoa(runner.DefaultDelayMs)); err != nil {
		return err
	}
	a.lastLoggedDelay = runner.DefaultDelayMs
	a.delayEdit.TextChanged().Attach(func() {
		a.syncRunnerSettings()
	})
	a.delayEdit.EditingFinished().Attach(func() {
		a.logDelayIfChanged()
	})

	configHint, err := walk.NewLabel(configGB)
	if err != nil {
		return err
	}
	if err := configHint.SetText("Available after Start. Hold mapped keys to run the loop. ESC stops."); err != nil {
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
	if err := a.logList.SetMinMaxSize(walk.Size{Width: 0, Height: 200}, walk.Size{}); err != nil {
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
	r := a.runner
	a.mu.Unlock()
	return r != nil && r.Running()
}

func (a *guiApp) setStarted(started bool) {
	a.startBtn.SetEnabled(!started)
	a.stopBtn.SetEnabled(started)
	a.setConfigEnabled(started)
}

func (a *guiApp) setConfigEnabled(enabled bool) {
	if a.binding {
		enabled = false
	}
	a.bindBtn.SetEnabled(enabled)
	a.clearBtn.SetEnabled(enabled)
	a.delayEdit.SetEnabled(enabled)
}

func (a *guiApp) syncRunnerSettings() {
	a.mu.Lock()
	r := a.runner
	vks := append([]int32(nil), a.triggerVKs...)
	delay := a.delayMs()
	a.mu.Unlock()

	if r != nil && r.Running() {
		r.UpdateSettings(vks, delay)
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
	defer a.mu.Unlock()

	if a.runner != nil && a.runner.Running() {
		return
	}

	a.appendLog("Starting VIIPER server...")

	started, err := ensureViiperServer()
	if err != nil {
		a.appendLog(fmt.Sprintf("VIIPER server failed: %v", err))
		return
	}
	if started {
		a.appendLog("VIIPER server ready")
	} else {
		a.appendLog("Using existing VIIPER server")
	}

	cfg := runner.Config{
		APIAddr:    runner.DefaultAPIAddr,
		TriggerVKs: append([]int32(nil), a.triggerVKs...),
		DelayMs:    a.delayMs(),
		Log:        a.appendLog,
	}

	a.runner = runner.New(cfg)
	if err := a.runner.Start(); err != nil {
		a.appendLog(fmt.Sprintf("Start failed: %v", err))
		return
	}

	a.setStarted(true)
}

func (a *guiApp) onStop() {
	a.mu.Lock()
	r := a.runner
	a.mu.Unlock()

	if r != nil {
		r.Stop()
	}

	go func() {
		if r != nil {
			r.Wait()
		}
		stopViiperServerIfStarted()
		a.mainWindow.Synchronize(func() {
			a.mu.Lock()
			a.runner = nil
			a.mu.Unlock()
			a.setStarted(false)
			a.appendLog("Clicker stopped — click Start before launching the game")
		})
	}()
}
