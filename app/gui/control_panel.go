//go:build windows

package main

import (
	"github.com/lxn/walk"
)

func (a *guiApp) buildControlPanel(parent walk.Container) error {
	hintFont, err := walk.NewFont("Segoe UI", 8, 0)
	if err != nil {
		return err
	}

	runGB, err := walk.NewGroupBox(parent)
	if err != nil {
		return err
	}
	if err := runGB.SetTitle("1. Clicker control"); err != nil {
		return err
	}
	runLayout := walk.NewVBoxLayout()
	runLayout.SetSpacing(8)
	if err := runGB.SetLayout(runLayout); err != nil {
		return err
	}

	controlRow, err := walk.NewComposite(runGB)
	if err != nil {
		return err
	}
	controlHBox := walk.NewHBoxLayout()
	controlHBox.SetSpacing(16)
	if err := controlRow.SetLayout(controlHBox); err != nil {
		return err
	}

	leftPanel, err := walk.NewComposite(controlRow)
	if err != nil {
		return err
	}
	leftVBox := walk.NewVBoxLayout()
	leftVBox.SetSpacing(4)
	if err := leftPanel.SetLayout(leftVBox); err != nil {
		return err
	}

	btnRow, err := walk.NewComposite(leftPanel)
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

	startHint, err := walk.NewLabel(leftPanel)
	if err != nil {
		return err
	}
	if err := startHint.SetText("Start before launching the game."); err != nil {
		return err
	}
	startHint.SetFont(hintFont)

	if _, err := walk.NewHSpacer(controlRow); err != nil {
		return err
	}

	rightPanel, err := walk.NewComposite(controlRow)
	if err != nil {
		return err
	}
	rightVBox := walk.NewVBoxLayout()
	rightVBox.SetSpacing(4)
	if err := rightPanel.SetLayout(rightVBox); err != nil {
		return err
	}

	badgeRow, err := walk.NewComposite(rightPanel)
	if err != nil {
		return err
	}
	badgeHBox := walk.NewHBoxLayout()
	badgeHBox.SetSpacing(8)
	if err := badgeRow.SetLayout(badgeHBox); err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(badgeRow); err != nil {
		return err
	}
	a.viiperBadge, err = newViiperBadge(badgeRow)
	if err != nil {
		return err
	}
	a.statusBadge, err = newStatusBadge(badgeRow)
	if err != nil {
		return err
	}
	a.setClickerStatus(clickerStatusStopped)

	toggleRow, err := walk.NewComposite(rightPanel)
	if err != nil {
		return err
	}
	toggleHBox := walk.NewHBoxLayout()
	toggleHBox.SetSpacing(6)
	if err := toggleRow.SetLayout(toggleHBox); err != nil {
		return err
	}
	if _, err := walk.NewHSpacer(toggleRow); err != nil {
		return err
	}

	toggleCaption, err := walk.NewLabel(toggleRow)
	if err != nil {
		return err
	}
	if err := toggleCaption.SetText("Toggle start / stop: End"); err != nil {
		return err
	}
	toggleCaption.SetFont(hintFont)

	return nil
}
