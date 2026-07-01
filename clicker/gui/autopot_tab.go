//go:build windows

package main

import (
	"fmt"
	"strconv"

	"experimental-clicker/runner"
	"github.com/lxn/walk"
)

func (a *guiApp) buildAutoPotTab(page *walk.TabPage) error {
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 4, VNear: 4, HFar: 4, VFar: 4})
	layout.SetSpacing(10)
	if err := page.SetLayout(layout); err != nil {
		return err
	}

	hintFont, err := walk.NewFont("Segoe UI", 8, 0)
	if err != nil {
		return err
	}

	hpGB, err := walk.NewGroupBox(page)
	if err != nil {
		return err
	}
	if err := hpGB.SetTitle("HP potion"); err != nil {
		return err
	}
	hpLayout := walk.NewHBoxLayout()
	hpLayout.SetSpacing(10)
	if err := hpGB.SetLayout(hpLayout); err != nil {
		return err
	}

	a.hpEnabledCB, err = walk.NewCheckBox(hpGB)
	if err != nil {
		return err
	}
	if err := a.hpEnabledCB.SetText("Enabled"); err != nil {
		return err
	}
	a.hpEnabledCB.SetChecked(true)
	a.hpEnabledCB.CheckedChanged().Attach(a.syncAutoPotSettings)

	hpThreshLabel, err := walk.NewLabel(hpGB)
	if err != nil {
		return err
	}
	if err := hpThreshLabel.SetText("Trigger below %:"); err != nil {
		return err
	}

	a.hpThresholdEdit, err = walk.NewLineEdit(hpGB)
	if err != nil {
		return err
	}
	a.hpThresholdEdit.SetMaxLength(2)
	if err := a.hpThresholdEdit.SetMinMaxSize(walk.Size{Width: 40, Height: 0}, walk.Size{Width: 40, Height: 0}); err != nil {
		return err
	}
	if err := a.hpThresholdEdit.SetText("50"); err != nil {
		return err
	}
	a.hpThreshold = 50
	a.hpThresholdEdit.EditingFinished().Attach(func() {
		a.commitHPThresholdEdit()
		a.syncAutoPotSettings()
	})

	hpKeyLabel, err := walk.NewLabel(hpGB)
	if err != nil {
		return err
	}
	if err := hpKeyLabel.SetText("Key:"); err != nil {
		return err
	}

	a.hpKeyLabel, err = walk.NewLabel(hpGB)
	if err != nil {
		return err
	}
	if err := a.hpKeyLabel.SetText("none"); err != nil {
		return err
	}

	a.hpBindBtn, err = walk.NewPushButton(hpGB)
	if err != nil {
		return err
	}
	if err := a.hpBindBtn.SetText("Set key..."); err != nil {
		return err
	}
	a.hpBindBtn.Clicked().Attach(a.onBindHPKey)

	a.hpClearBtn, err = walk.NewPushButton(hpGB)
	if err != nil {
		return err
	}
	if err := a.hpClearBtn.SetText("Clear"); err != nil {
		return err
	}
	a.hpClearBtn.Clicked().Attach(a.onClearHPKey)

	spGB, err := walk.NewGroupBox(page)
	if err != nil {
		return err
	}
	if err := spGB.SetTitle("SP potion"); err != nil {
		return err
	}
	spLayout := walk.NewHBoxLayout()
	spLayout.SetSpacing(10)
	if err := spGB.SetLayout(spLayout); err != nil {
		return err
	}

	a.spEnabledCB, err = walk.NewCheckBox(spGB)
	if err != nil {
		return err
	}
	if err := a.spEnabledCB.SetText("Enabled"); err != nil {
		return err
	}
	a.spEnabledCB.SetChecked(true)
	a.spEnabledCB.CheckedChanged().Attach(a.syncAutoPotSettings)

	spThreshLabel, err := walk.NewLabel(spGB)
	if err != nil {
		return err
	}
	if err := spThreshLabel.SetText("Trigger below %:"); err != nil {
		return err
	}

	a.spThresholdEdit, err = walk.NewLineEdit(spGB)
	if err != nil {
		return err
	}
	a.spThresholdEdit.SetMaxLength(2)
	if err := a.spThresholdEdit.SetMinMaxSize(walk.Size{Width: 40, Height: 0}, walk.Size{Width: 40, Height: 0}); err != nil {
		return err
	}
	if err := a.spThresholdEdit.SetText("30"); err != nil {
		return err
	}
	a.spThreshold = 30
	a.spThresholdEdit.EditingFinished().Attach(func() {
		a.commitSPThresholdEdit()
		a.syncAutoPotSettings()
	})

	spKeyLabel, err := walk.NewLabel(spGB)
	if err != nil {
		return err
	}
	if err := spKeyLabel.SetText("Key:"); err != nil {
		return err
	}

	a.spKeyLabel, err = walk.NewLabel(spGB)
	if err != nil {
		return err
	}
	if err := a.spKeyLabel.SetText("none"); err != nil {
		return err
	}

	a.spBindBtn, err = walk.NewPushButton(spGB)
	if err != nil {
		return err
	}
	if err := a.spBindBtn.SetText("Set key..."); err != nil {
		return err
	}
	a.spBindBtn.Clicked().Attach(a.onBindSPKey)

	a.spClearBtn, err = walk.NewPushButton(spGB)
	if err != nil {
		return err
	}
	if err := a.spClearBtn.SetText("Clear"); err != nil {
		return err
	}
	a.spClearBtn.Clicked().Attach(a.onClearSPKey)

	autopotHint, err := walk.NewLabel(page)
	if err != nil {
		return err
	}
	if err := autopotHint.SetText("When HP or SP drops below the threshold, one potion is pressed at a time; the bar is polled until it recovers before using another."); err != nil {
		return err
	}
	autopotHint.SetFont(hintFont)

	return nil
}

func (a *guiApp) autopotConfig() runner.AutoPotConfig {
	return runner.AutoPotConfig{
		HPThreshold:    a.hpThreshold,
		SPThreshold:    a.spThreshold,
		HPKeyVK:        a.hpKeyVK,
		SPKeyVK:        a.spKeyVK,
		HPEnabled:      a.hpEnabledCB.Checked(),
		SPEnabled:      a.spEnabledCB.Checked(),
		Log:            a.appendLog,
		OnStatusParsed: a.onStatusParsed,
	}
}

// onStatusParsed is called from the autopot goroutine whenever new HP/SP
// values are parsed. It forwards the values to the overlay window via the
// walk message queue (Synchronize is thread-safe).
func (a *guiApp) onStatusParsed(hp, hpMax, sp, spMax, stripX, stripY, stripW, stripH int) {
	if a.overlay == nil {
		return
	}
	ovl := a.overlay
	ovl.Update(hp, hpMax, sp, spMax, stripX, stripY, stripW, stripH)
}

func (a *guiApp) commitHPThresholdEdit() {
	v, ok := a.parseThreshold(a.hpThresholdEdit)
	if !ok {
		a.hpThresholdEdit.SetText(strconv.Itoa(a.hpThreshold))
		return
	}
	if v == a.hpThreshold {
		return
	}
	a.hpThreshold = v
	a.appendLog(fmt.Sprintf("AutoPot HP threshold: %d%%", v))
}

func (a *guiApp) commitSPThresholdEdit() {
	v, ok := a.parseThreshold(a.spThresholdEdit)
	if !ok {
		a.spThresholdEdit.SetText(strconv.Itoa(a.spThreshold))
		return
	}
	if v == a.spThreshold {
		return
	}
	a.spThreshold = v
	a.appendLog(fmt.Sprintf("AutoPot SP threshold: %d%%", v))
}

func (a *guiApp) syncAutoPotSettings() {
	cfg := a.autopotWanted()
	a.mu.Lock()
	cfg.Session = a.inputSession
	cfg.Log = a.appendLog
	r := a.autopotRunner
	a.mu.Unlock()

	if cfg.Session == nil || cfg.Log == nil {
		return
	}

	if r != nil && r.Running() {
		r.UpdateSettings(cfg)
		return
	}

	if !a.isStarted() {
		return
	}

	a.startAutoPotRunner(cfg, a.appendLog)
}

func (a *guiApp) setAutoPotConfigEnabled(enabled bool) {
	a.hpEnabledCB.SetEnabled(enabled)
	a.spEnabledCB.SetEnabled(enabled)
	a.hpThresholdEdit.SetEnabled(enabled)
	a.spThresholdEdit.SetEnabled(enabled)
	a.hpBindBtn.SetEnabled(true)
	a.hpClearBtn.SetEnabled(true)
	a.spBindBtn.SetEnabled(true)
	a.spClearBtn.SetEnabled(true)
}

func (a *guiApp) onClearHPKey() {
	a.hpKeyVK = 0
	a.hpKeyLabel.SetText("none")
	a.appendLog("HP potion key cleared")
	a.syncAutoPotSettings()
}

func (a *guiApp) onClearSPKey() {
	a.spKeyVK = 0
	a.spKeyLabel.SetText("none")
	a.appendLog("SP potion key cleared")
	a.syncAutoPotSettings()
}

func (a *guiApp) finishThresholdInput() {
	a.commitHPThresholdEdit()
	a.commitSPThresholdEdit()
	a.syncAutoPotSettings()
	a.blurThresholdEdits()
}

func (a *guiApp) wireThresholdBlurOnClick(container walk.Container) {
	if container == nil {
		return
	}
	children := container.Children()
	if children == nil {
		return
	}
	for i := 0; i < children.Len(); i++ {
		child := children.At(i)
		if child == a.hpThresholdEdit || child == a.spThresholdEdit || child == a.logList {
			continue
		}
		if win, ok := child.(walk.Window); ok {
			win.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
				a.finishThresholdInput()
			})
		}
		if c, ok := child.(walk.Container); ok {
			a.wireThresholdBlurOnClick(c)
		}
	}
}

func (a *guiApp) blurThresholdEdits() {
	if a.mainWindow != nil {
		_ = a.mainWindow.SetFocus()
	}
}

func (a *guiApp) parseThreshold(edit *walk.LineEdit) (int, bool) {
	if edit == nil {
		return 0, false
	}
	v, err := strconv.Atoi(edit.Text())
	if err != nil || v < 1 || v > 99 {
		return 0, false
	}
	return v, true
}

func (a *guiApp) onBindHPKey() {
	a.bindAutoPotKey(true)
}

func (a *guiApp) onBindSPKey() {
	a.bindAutoPotKey(false)
}

func (a *guiApp) bindAutoPotKey(hp bool) {
	a.bindKeyFlow(
		func() bool {
			if !a.isStarted() || a.autopotBinding {
				return false
			}
			a.autopotBinding = true
			return true
		},
		fmt.Sprintf("Press a potion hotkey to assign (%s timeout)...", runner.KeyBindTimeout),
		func() { a.autopotBinding = false },
		func() { a.setAutoPotConfigEnabled(a.isStarted()) },
		func(vk int32) {
			if hp {
				if a.spKeyVK != 0 && a.spKeyVK == vk {
					a.appendLog(fmt.Sprintf("Key %s is already assigned to SP potion", runner.KeyName(vk)))
					return
				}
				a.hpKeyVK = vk
				a.hpKeyLabel.SetText(runner.KeyName(vk))
				a.appendLog(fmt.Sprintf("HP potion key: %s", runner.KeyName(vk)))
			} else {
				if a.hpKeyVK != 0 && a.hpKeyVK == vk {
					a.appendLog(fmt.Sprintf("Key %s is already assigned to HP potion", runner.KeyName(vk)))
					return
				}
				a.spKeyVK = vk
				a.spKeyLabel.SetText(runner.KeyName(vk))
				a.appendLog(fmt.Sprintf("SP potion key: %s", runner.KeyName(vk)))
			}
			a.syncAutoPotSettings()
		},
	)
}

