//go:build windows

package main

import (
	"fmt"
	"slices"
	"strconv"

	"belarus-champ-tools/runner"
	"github.com/lxn/walk"
)

const (
	clickerWithMouse    = 0
	clickerWithoutMouse = 1
)

var clickerSlotTitles = [runner.ClickerSlotCount]string{
	"With mouse click",
	"Without mouse click (keyboard only)",
}

type clickerSlotWidgets struct {
	keyLabel  *walk.Label
	bindBtn   *walk.PushButton
	clearBtn  *walk.PushButton
	delayEdit *walk.LineEdit
}

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
	if err := configGB.SetTitle("2. Configure clickers"); err != nil {
		return err
	}
	configLayout := walk.NewVBoxLayout()
	configLayout.SetSpacing(10)
	if err := configGB.SetLayout(configLayout); err != nil {
		return err
	}

	for i := 0; i < runner.ClickerSlotCount; i++ {
		if err := a.buildClickerSlot(configGB, i); err != nil {
			return err
		}
	}

	configHint, err := walk.NewLabel(configGB)
	if err != nil {
		return err
	}
	if err := configHint.SetText("After Start, add keys anytime — no restart needed. Hold any mapped key in each group to run that clicker. End toggles start/stop."); err != nil {
		return err
	}

	return a.buildTimerKeySection(page)
}

func (a *guiApp) buildClickerSlot(parent walk.Container, index int) error {
	slotGB, err := walk.NewGroupBox(parent)
	if err != nil {
		return err
	}
	if err := slotGB.SetTitle(clickerSlotTitles[index]); err != nil {
		return err
	}
	rowLayout := walk.NewHBoxLayout()
	rowLayout.SetSpacing(10)
	if err := slotGB.SetLayout(rowLayout); err != nil {
		return err
	}

	w := &a.clickerSlots[index]

	keyText, err := walk.NewLabel(slotGB)
	if err != nil {
		return err
	}
	if err := keyText.SetText("Trigger keys:"); err != nil {
		return err
	}

	w.keyLabel, err = walk.NewLabel(slotGB)
	if err != nil {
		return err
	}
	if err := w.keyLabel.SetText(runner.KeysText(a.clickerTriggerVKs[index])); err != nil {
		return err
	}

	w.bindBtn, err = walk.NewPushButton(slotGB)
	if err != nil {
		return err
	}
	if err := w.bindBtn.SetText("Add key..."); err != nil {
		return err
	}
	slot := index
	w.bindBtn.Clicked().Attach(func() {
		a.bindClickerKey(slot)
	})

	w.clearBtn, err = walk.NewPushButton(slotGB)
	if err != nil {
		return err
	}
	if err := w.clearBtn.SetText("Clear keys"); err != nil {
		return err
	}
	w.clearBtn.Clicked().Attach(func() {
		a.clearClickerKey(slot)
	})

	delayLabel, err := walk.NewLabel(slotGB)
	if err != nil {
		return err
	}
	if err := delayLabel.SetText("Delay (ms):"); err != nil {
		return err
	}

	w.delayEdit, err = walk.NewLineEdit(slotGB)
	if err != nil {
		return err
	}
	w.delayEdit.SetMaxLength(6)
	if err := w.delayEdit.SetMinMaxSize(walk.Size{Width: 80, Height: 0}, walk.Size{Width: 80, Height: 0}); err != nil {
		return err
	}
	if err := w.delayEdit.SetText(strconv.Itoa(runner.DefaultDelayMs)); err != nil {
		return err
	}
	a.clickerLastLoggedDelay[index] = runner.DefaultDelayMs
	w.delayEdit.TextChanged().Attach(a.syncRunnerSettings)
	w.delayEdit.EditingFinished().Attach(func() {
		a.logClickerDelayIfChanged(slot)
	})

	return nil
}

func (a *guiApp) clickerConfig() runner.Config {
	cfg := runner.Config{
		Log: a.appendLog,
	}
	for i := 0; i < runner.ClickerSlotCount; i++ {
		cfg.Slots[i] = runner.ClickerSlot{
			TriggerVKs: append([]int32(nil), a.clickerTriggerVKs[i]...),
			DelayMs:    a.clickerDelayMs(i),
			MouseClick: i == clickerWithMouse,
		}
	}
	return cfg
}

func (a *guiApp) clickerDelayMs(index int) int {
	if index < 0 || index >= runner.ClickerSlotCount {
		return runner.DefaultDelayMs
	}
	v, err := strconv.Atoi(a.clickerSlots[index].delayEdit.Text())
	if err != nil || v <= 0 {
		return runner.DefaultDelayMs
	}
	return v
}

func (a *guiApp) logClickerDelayIfChanged(index int) {
	delay := a.clickerDelayMs(index)
	if delay == a.clickerLastLoggedDelay[index] {
		return
	}
	a.clickerLastLoggedDelay[index] = delay
	a.appendLog(fmt.Sprintf("%s delay: %d ms", clickerSlotTitles[index], delay))
}

func (a *guiApp) setClickerConfigEnabled(enabled bool) {
	for i := 0; i < runner.ClickerSlotCount; i++ {
		a.clickerSlots[i].delayEdit.SetEnabled(enabled)
		a.clickerSlots[i].bindBtn.SetEnabled(true)
		a.clickerSlots[i].clearBtn.SetEnabled(true)
	}
}

func (a *guiApp) updateClickerKeyLabel(index int) {
	a.clickerSlots[index].keyLabel.SetText(runner.KeysText(a.clickerTriggerVKs[index]))
}

func (a *guiApp) clearClickerKey(index int) {
	if index < 0 || index >= runner.ClickerSlotCount {
		return
	}
	a.clickerTriggerVKs[index] = nil
	a.updateClickerKeyLabel(index)
	a.appendLog(fmt.Sprintf("%s keys cleared", clickerSlotTitles[index]))
	a.syncRunnerSettings()
}

func (a *guiApp) bindClickerKey(index int) {
	a.bindKeyFlow(
		func() bool {
			if !a.isStarted() || a.clickerBindingSlot >= 0 || index < 0 || index >= runner.ClickerSlotCount {
				return false
			}
			a.clickerBindingSlot = index
			return true
		},
		fmt.Sprintf("Press a key to add for %s (%s timeout)...", clickerSlotTitles[index], runner.KeyBindTimeout),
		func() { a.clickerBindingSlot = -1 },
		func() { a.setClickerConfigEnabled(a.isStarted()) },
		func(vk int32) {
			for i := 0; i < runner.ClickerSlotCount; i++ {
				if slices.Contains(a.clickerTriggerVKs[i], vk) {
					a.appendLog(fmt.Sprintf("Key %s is already bound to %s", runner.KeyName(vk), clickerSlotTitles[i]))
					return
				}
			}
			a.clickerTriggerVKs[index] = append(a.clickerTriggerVKs[index], vk)
			a.updateClickerKeyLabel(index)
			a.appendLog(fmt.Sprintf("%s added key %s", clickerSlotTitles[index], runner.KeyName(vk)))
			a.syncRunnerSettings()
		},
	)
}
