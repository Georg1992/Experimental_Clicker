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
	a.lastAppliedHPThreshold = 50
	a.hpThresholdEdit.TextChanged().Attach(a.syncAutoPotSettings)
	a.hpThresholdEdit.EditingFinished().Attach(a.syncAutoPotSettings)

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
	a.lastAppliedSPThreshold = 30
	a.spThresholdEdit.TextChanged().Attach(a.syncAutoPotSettings)
	a.spThresholdEdit.EditingFinished().Attach(a.syncAutoPotSettings)

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
		HPThreshold: a.thresholdPercent(a.hpThresholdEdit, 50),
		SPThreshold: a.thresholdPercent(a.spThresholdEdit, 30),
		HPKeyVK:     a.hpKeyVK,
		SPKeyVK:     a.spKeyVK,
		HPEnabled:   a.hpEnabledCB.Checked(),
		SPEnabled:   a.spEnabledCB.Checked(),
		Log:         a.appendLog,
	}
}

func (a *guiApp) syncAutoPotSettings() {
	cfg := a.autopotWanted()
	a.mu.Lock()
	r := a.autopotRunner
	a.mu.Unlock()

	if r != nil && r.Running() {
		r.UpdateSettings(cfg)
		a.logAppliedThresholds(cfg)
		return
	}

	if !a.isStarted() {
		return
	}

	a.startAutoPotRunner(cfg)
	a.mu.Lock()
	r = a.autopotRunner
	a.mu.Unlock()
	if r != nil && r.Running() {
		a.logAppliedThresholds(cfg)
	}
}

func (a *guiApp) seedAppliedThresholds(cfg runner.AutoPotConfig) {
	a.lastAppliedHPThreshold = cfg.HPThreshold
	a.lastAppliedSPThreshold = cfg.SPThreshold
}

func (a *guiApp) logAppliedThresholds(cfg runner.AutoPotConfig) {
	if cfg.HPThreshold != a.lastAppliedHPThreshold {
		a.lastAppliedHPThreshold = cfg.HPThreshold
		a.appendLog(fmt.Sprintf("AutoPot HP threshold: %d%%", cfg.HPThreshold))
	}
	if cfg.SPThreshold != a.lastAppliedSPThreshold {
		a.lastAppliedSPThreshold = cfg.SPThreshold
		a.appendLog(fmt.Sprintf("AutoPot SP threshold: %d%%", cfg.SPThreshold))
	}
}

func (a *guiApp) setAutoPotConfigEnabled(enabled bool) {
	if a.autopotBinding {
		enabled = false
	}
	a.hpEnabledCB.SetEnabled(enabled)
	a.spEnabledCB.SetEnabled(enabled)
	a.hpThresholdEdit.SetEnabled(enabled)
	a.spThresholdEdit.SetEnabled(enabled)
	a.hpBindBtn.SetEnabled(enabled)
	a.hpClearBtn.SetEnabled(enabled)
	a.spBindBtn.SetEnabled(enabled)
	a.spClearBtn.SetEnabled(enabled)
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

func (a *guiApp) normalizeThresholdEdit(edit *walk.LineEdit, lastApplied int) {
	if edit == nil {
		return
	}
	if _, ok := a.parseThreshold(edit); !ok {
		edit.SetText(strconv.Itoa(lastApplied))
	}
}

func (a *guiApp) blurThresholdEdits() {
	if a.mainWindow != nil {
		_ = a.mainWindow.SetFocus()
	}
}

func (a *guiApp) finishThresholdInput() {
	a.normalizeThresholdEdit(a.hpThresholdEdit, a.lastAppliedHPThreshold)
	a.normalizeThresholdEdit(a.spThresholdEdit, a.lastAppliedSPThreshold)
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

func (a *guiApp) thresholdPercent(edit *walk.LineEdit, fallback int) int {
	if v, ok := a.parseThreshold(edit); ok {
		return v
	}
	return fallback
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
	if a.autopotBinding {
		return
	}
	a.autopotBinding = true
	a.setAutoPotConfigEnabled(false)
	a.appendLog("Press a potion hotkey to assign (5s timeout)...")

	go func() {
		defer func() {
			a.autopotBinding = false
			a.mainWindow.Synchronize(func() {
				a.setAutoPotConfigEnabled(a.isStarted())
			})
		}()

		vk, ok := runner.WaitForKeyPress(runner.KeyBindTimeout)
		a.mainWindow.Synchronize(func() {
			if !ok {
				a.appendLog("Key bind timed out")
				return
			}
			if _, hidOK := runner.VKToHID(vk); !hidOK {
				a.appendLog(fmt.Sprintf("Key %s is not supported", runner.KeyName(vk)))
				return
			}
			if hp {
				a.hpKeyVK = vk
				a.hpKeyLabel.SetText(runner.KeyName(vk))
				a.appendLog(fmt.Sprintf("HP potion key: %s", runner.KeyName(vk)))
			} else {
				a.spKeyVK = vk
				a.spKeyLabel.SetText(runner.KeyName(vk))
				a.appendLog(fmt.Sprintf("SP potion key: %s", runner.KeyName(vk)))
			}
			a.syncAutoPotSettings()
		})
	}()
}

