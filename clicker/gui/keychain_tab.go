//go:build windows

package main

import (
	"fmt"

	"experimental-clicker/runner"
	"github.com/lxn/walk"
)

type keyChainSlotWidgets struct {
	keyEdit   *walk.LineEdit
	delayEdit *walk.NumberEdit
}

func (a *guiApp) buildKeyChainTab(page *walk.TabPage) error {
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 4, VNear: 4, HFar: 4, VFar: 4})
	layout.SetSpacing(10)
	if err := page.SetLayout(layout); err != nil {
		return err
	}

	chainGB, err := walk.NewGroupBox(page)
	if err != nil {
		return err
	}
	if err := chainGB.SetTitle("Switch 1"); err != nil {
		return err
	}
	chainLayout := walk.NewVBoxLayout()
	chainLayout.SetSpacing(10)
	if err := chainGB.SetLayout(chainLayout); err != nil {
		return err
	}

	chainRow, err := walk.NewComposite(chainGB)
	if err != nil {
		return err
	}
	chainHBox := walk.NewHBoxLayout()
	chainHBox.SetSpacing(0)
	if err := chainRow.SetLayout(chainHBox); err != nil {
		return err
	}
	applyKeyChainSurface(chainRow)

	labelsCol, err := walk.NewComposite(chainRow)
	if err != nil {
		return err
	}
	labelsLayout := walk.NewVBoxLayout()
	labelsLayout.SetSpacing(0)
	if err := labelsCol.SetLayout(labelsLayout); err != nil {
		return err
	}
	if err := labelsCol.SetMinMaxSize(walk.Size{Width: 70, Height: 0}, walk.Size{Width: 70, Height: 0}); err != nil {
		return err
	}
	applyKeyChainSurface(labelsCol)

	keysLabel, err := walk.NewLabel(labelsCol)
	if err != nil {
		return err
	}
	if err := keysLabel.SetText("Keys:"); err != nil {
		return err
	}
	if err := keysLabel.SetMinMaxSize(walk.Size{Width: 70, Height: keyChainFieldHeight}, walk.Size{Width: 70, Height: keyChainFieldHeight}); err != nil {
		return err
	}

	downSpacer, err := walk.NewComposite(labelsCol)
	if err != nil {
		return err
	}
	if err := downSpacer.SetMinMaxSize(walk.Size{Width: 70, Height: keyChainDownHeight}, walk.Size{Width: 70, Height: keyChainDownHeight}); err != nil {
		return err
	}
	applyKeyChainSurface(downSpacer)

	delaysLabel, err := walk.NewLabel(labelsCol)
	if err != nil {
		return err
	}
	if err := delaysLabel.SetText("Delay(ms):"); err != nil {
		return err
	}
	if err := delaysLabel.SetMinMaxSize(walk.Size{Width: 70, Height: keyChainFieldHeight}, walk.Size{Width: 70, Height: keyChainFieldHeight}); err != nil {
		return err
	}

	stepsRow, err := walk.NewComposite(chainRow)
	if err != nil {
		return err
	}
	stepsHBox := walk.NewHBoxLayout()
	stepsHBox.SetSpacing(0)
	if err := stepsRow.SetLayout(stepsHBox); err != nil {
		return err
	}
	applyKeyChainSurface(stepsRow)

	stepHeight := keyChainStepHeight()
	for i := 0; i < runner.KeyChainSlotCount; i++ {
		if err := a.buildKeyChainStep(stepsRow, i, stepHeight); err != nil {
			return err
		}
		if i < runner.KeyChainSlotCount-1 {
			if err := a.buildKeyChainStepLink(stepsRow, stepHeight); err != nil {
				return err
			}
		}
	}

	btnRow, err := walk.NewComposite(chainGB)
	if err != nil {
		return err
	}
	btnLayout := walk.NewHBoxLayout()
	btnLayout.SetSpacing(10)
	if err := btnRow.SetLayout(btnLayout); err != nil {
		return err
	}
	applyKeyChainSurface(btnRow)

	a.keyChainClearBtn, err = walk.NewPushButton(btnRow)
	if err != nil {
		return err
	}
	if err := a.keyChainClearBtn.SetText("Clear"); err != nil {
		return err
	}
	a.keyChainClearBtn.Clicked().Attach(a.clearKeyChain)

	hint, err := walk.NewLabel(page)
	if err != nil {
		return err
	}
	if err := hint.SetText("Key 1 is the trigger. Tap it to run the chain once; hold it to loop."); err != nil {
		return err
	}

	return nil
}

func (a *guiApp) buildKeyChainStep(parent walk.Container, index, height int) error {
	step, err := walk.NewComposite(parent)
	if err != nil {
		return err
	}
	stepLayout := walk.NewVBoxLayout()
	stepLayout.SetSpacing(0)
	if err := step.SetLayout(stepLayout); err != nil {
		return err
	}
	if err := step.SetMinMaxSize(walk.Size{Width: keyChainStepWidth, Height: height}, walk.Size{Width: keyChainStepWidth, Height: height}); err != nil {
		return err
	}
	applyKeyChainSurface(step)

	w := &a.keyChainSlots[index]
	w.keyEdit, err = walk.NewLineEdit(step)
	if err != nil {
		return err
	}
	if err := w.keyEdit.SetReadOnly(true); err != nil {
		return err
	}
	if err := w.keyEdit.SetMinMaxSize(walk.Size{Width: keyChainKeyFieldWidth, Height: keyChainFieldHeight}, walk.Size{Width: keyChainKeyFieldWidth, Height: keyChainFieldHeight}); err != nil {
		return err
	}
	a.setKeyChainKeyText(index, 0)
	slot := index
	w.keyEdit.MouseDown().Attach(func(_ int, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			a.bindKeyChainKey(slot)
		}
	})

	downArrow, err := newKeyChainDownArrow(step)
	if err != nil {
		return err
	}
	if err := downArrow.SetMinMaxSize(walk.Size{Width: keyChainStepWidth, Height: keyChainDownHeight}, walk.Size{Width: keyChainStepWidth, Height: keyChainDownHeight}); err != nil {
		return err
	}

	w.delayEdit, err = walk.NewNumberEdit(step)
	if err != nil {
		return err
	}
	if err := w.delayEdit.SetRange(0, 999999); err != nil {
		return err
	}
	if err := w.delayEdit.SetDecimals(0); err != nil {
		return err
	}
	if err := w.delayEdit.SetIncrement(1); err != nil {
		return err
	}
	if err := w.delayEdit.SetSpinButtonsVisible(true); err != nil {
		return err
	}
	if err := w.delayEdit.SetValue(0); err != nil {
		return err
	}
	if err := w.delayEdit.SetMinMaxSize(walk.Size{Width: keyChainDelayFieldWidth, Height: keyChainFieldHeight}, walk.Size{Width: keyChainDelayFieldWidth, Height: keyChainFieldHeight}); err != nil {
		return err
	}
	w.delayEdit.ValueChanged().Attach(a.syncKeyChainSettings)

	return nil
}

func (a *guiApp) buildKeyChainStepLink(parent walk.Container, height int) error {
	link, err := newKeyChainStepLink(parent)
	if err != nil {
		return err
	}
	return link.SetMinMaxSize(
		walk.Size{Width: keyChainLinkWidth, Height: height},
		walk.Size{Width: keyChainLinkWidth, Height: height},
	)
}

func (a *guiApp) setKeyChainKeyText(index int, vk int32) {
	text := "None"
	if vk != 0 {
		text = runner.KeyName(vk)
	}
	a.keyChainSlots[index].keyEdit.SetText(text)
}

func (a *guiApp) keyChainConfig() runner.KeyChainConfig {
	cfg := runner.KeyChainConfig{Log: a.appendLog}
	for i := 0; i < runner.KeyChainSlotCount; i++ {
		cfg.Keys[i] = a.keyChainKeyVKs[i]
		cfg.DelaysMs[i] = int(a.keyChainSlots[i].delayEdit.Value())
	}
	return cfg
}

func (a *guiApp) syncKeyChainSettings() {
	if !a.isStarted() {
		return
	}

	cfg := a.keyChainConfig()
	a.mu.Lock()
	kc := a.keyChainRunner
	a.mu.Unlock()

	if !cfg.Active() {
		a.stopKeyChainRunner()
		return
	}

	a.mu.Lock()
	cfg.Session = a.inputSession
	a.mu.Unlock()

	if kc != nil && kc.Running() {
		kc.UpdateSettings(cfg)
		return
	}

	a.startKeyChainRunner(cfg, a.appendLog)
}

func (a *guiApp) setKeyChainConfigEnabled(enabled bool) {
	for i := 0; i < runner.KeyChainSlotCount; i++ {
		a.keyChainSlots[i].keyEdit.SetEnabled(true)
		a.keyChainSlots[i].delayEdit.SetEnabled(enabled)
	}
	if a.keyChainClearBtn != nil {
		a.keyChainClearBtn.SetEnabled(true)
	}
}

func (a *guiApp) startKeyChainRunner(cfg runner.KeyChainConfig, log func(string)) {
	take, store := makeLifecycleSlot[*runner.KeyChainRunner](&a.mu, &a.keyChainRunner)
	startLifecycle(
		take, store,
		"KeyChain",
		log,
		func() runner.InputSession {
			a.mu.Lock()
			defer a.mu.Unlock()
			return a.inputSession
		},
		func() bool { return cfg.Active() },
		func(sess runner.InputSession) *runner.KeyChainRunner {
			cfg.Session = sess
			cfg.Log = log
			return runner.NewKeyChain(cfg)
		},
	)
}

func (a *guiApp) stopKeyChainRunner() {
	a.mu.Lock()
	kc := a.keyChainRunner
	a.keyChainRunner = nil
	a.mu.Unlock()
	if kc != nil {
		kc.Stop()
		kc.Wait()
	}
}

func (a *guiApp) clearKeyChain() {
	for i := 0; i < runner.KeyChainSlotCount; i++ {
		a.keyChainKeyVKs[i] = 0
		a.setKeyChainKeyText(i, 0)
		a.keyChainSlots[i].delayEdit.SetValue(0)
	}
	a.syncKeyChainSettings()
	a.appendLog("KeyChain cleared")
}

func (a *guiApp) bindKeyChainKey(index int) {
	a.bindKeyFlow(
		func() bool {
			if !a.isStarted() || a.keyChainBindingSlot >= 0 || index < 0 || index >= runner.KeyChainSlotCount {
				return false
			}
			a.keyChainBindingSlot = index
			return true
		},
		fmt.Sprintf("Press a key for chain slot %d (%s timeout)...", index+1, runner.KeyBindTimeout),
		func() { a.keyChainBindingSlot = -1 },
		func() { a.setKeyChainConfigEnabled(a.isStarted()) },
		func(vk int32) {
			a.keyChainKeyVKs[index] = vk
			a.setKeyChainKeyText(index, vk)
			a.appendLog(fmt.Sprintf("Chain key %d: %s", index+1, runner.KeyName(vk)))
			a.syncKeyChainSettings()
		},
	)
}
