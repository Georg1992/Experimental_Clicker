package runner

import "testing"

func TestUpdateSettingsAppliesThresholdChanges(t *testing.T) {
	ap := NewAutoPot(AutoPotConfig{
		HPEnabled:     true,
		HPKeyVK:       'W',
		HPThreshold:   50,
		SPThreshold:   30,
	})

	// GUI always sends full wanted config, not partial patches.
	ap.UpdateSettings(AutoPotConfig{
		HPEnabled:   true,
		HPKeyVK:     'W',
		HPThreshold: 60,
		SPThreshold: 30,
	})
	if cfg := ap.settings(); cfg.HPThreshold != 60 || cfg.SPThreshold != 30 {
		t.Fatalf("after HP edit cfg=%d/%d want 60/30", cfg.HPThreshold, cfg.SPThreshold)
	}

	ap.UpdateSettings(AutoPotConfig{
		HPEnabled:   true,
		HPKeyVK:     'W',
		HPThreshold: 60,
		SPEnabled:   true,
		SPKeyVK:     'E',
		SPThreshold: 50,
	})
	cfg := ap.settings()
	if cfg.HPThreshold != 60 || cfg.SPThreshold != 50 {
		t.Fatalf("after SP edit cfg=%d/%d want 60/50", cfg.HPThreshold, cfg.SPThreshold)
	}
}

func TestMainLoopReadsUpdatedThreshold(t *testing.T) {
	ap := NewAutoPot(AutoPotConfig{
		HPEnabled:   true,
		HPKeyVK:     'W',
		HPThreshold: 50,
	})
	ap.UpdateSettings(AutoPotConfig{
		HPEnabled:   true,
		HPKeyVK:     'W',
		HPThreshold: 75,
	})
	if cfg := ap.settings(); cfg.HPThreshold != 75 {
		t.Fatalf("settings HPThreshold=%d want 75", cfg.HPThreshold)
	}
}
