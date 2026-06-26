//go:build windows

package main

import (
	"strconv"

	"experimental-clicker/runner"
	"github.com/lxn/walk"
)

func (a *guiApp) buildClickerTab(page *walk.TabPage) error {
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 4, VNear: 4, HFar: 4, VFar: 4})
	layout.SetSpacing(10)
	if err := page.SetLayout(layout); err != nil {
		return err
	}

	configGB, err := walk.NewGroupBox(page)
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

	a.mouseClickCB, err = walk.NewCheckBox(delayRow)
	if err != nil {
		return err
	}
	if err := a.mouseClickCB.SetText("Mouse click"); err != nil {
		return err
	}
	a.mouseClickCB.SetChecked(true)
	a.mouseClickCB.CheckedChanged().Attach(func() {
		a.syncRunnerSettings()
		if a.mouseClickCB.Checked() {
			a.appendLog("Mouse click: on")
		} else {
			a.appendLog("Mouse click: off")
		}
	})

	configHint, err := walk.NewLabel(configGB)
	if err != nil {
		return err
	}
	if err := configHint.SetText("After Start, add keys anytime — no restart needed. Hold mapped keys to click. End pauses everything. Stop turns off."); err != nil {
		return err
	}

	return a.buildTimerKeySection(page)
}
