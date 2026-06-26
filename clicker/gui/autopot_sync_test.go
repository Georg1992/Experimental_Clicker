//go:build windows

package main

import (
	"testing"

	"experimental-clicker/runner"
)

type mockAutoPotRunner struct {
	running bool
	updates []runner.AutoPotConfig
}

func (m *mockAutoPotRunner) Running() bool { return m.running }

func (m *mockAutoPotRunner) UpdateSettings(cfg runner.AutoPotConfig) {
	m.updates = append(m.updates, cfg)
}

func TestLogAppliedThresholdsTracksRunnerValues(t *testing.T) {
	app := &guiApp{lastAppliedHPThreshold: 50, lastAppliedSPThreshold: 30}

	app.logAppliedThresholds(runner.AutoPotConfig{HPThreshold: 60, SPThreshold: 30})
	if app.lastAppliedHPThreshold != 60 || app.lastAppliedSPThreshold != 30 {
		t.Fatalf("lastApplied=%d/%d want 60/30", app.lastAppliedHPThreshold, app.lastAppliedSPThreshold)
	}

	app.logAppliedThresholds(runner.AutoPotConfig{HPThreshold: 60, SPThreshold: 50})
	if app.lastAppliedSPThreshold != 50 {
		t.Fatalf("lastAppliedSP=%d want 50", app.lastAppliedSPThreshold)
	}
}

func TestRunningRunnerReceivesThresholdUpdate(t *testing.T) {
	mock := &mockAutoPotRunner{running: true}
	cfg := runner.AutoPotConfig{
		HPEnabled:   true,
		HPKeyVK:     'W',
		HPThreshold: 60,
		SPThreshold: 50,
	}
	mock.UpdateSettings(cfg)
	if len(mock.updates) != 1 {
		t.Fatalf("updates=%d want 1", len(mock.updates))
	}
	if mock.updates[0].HPThreshold != 60 || mock.updates[0].SPThreshold != 50 {
		t.Fatalf("update=%+v want 60/50", mock.updates[0])
	}
}
