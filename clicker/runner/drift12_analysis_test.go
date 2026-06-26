package runner

import (
	"image"
	"testing"
	"time"
)

func TestDrift12FixtureAccuracy(t *testing.T) {
	img := loadFixture(t, "drift1.2.png")
	mapped, err := RefreshBarPair(img)
	if err != nil {
		t.Fatal(err)
	}
	hp, sp := ReadMappedBars(img, mapped)
	t.Logf("HP=%+v SP=%+v hp=%.1f%% sp=%.1f%% score=%d", mapped.HP, mapped.SP, hp.Percent, sp.Percent, mapped.MapScore)
	if hp.Percent < 95 || sp.Percent < 95 {
		t.Fatalf("full bars read hp=%.1f%% sp=%.1f%%", hp.Percent, sp.Percent)
	}
}

func TestDrift12FreshMapStable(t *testing.T) {
	img := loadFixture(t, "drift1.2.png")
	mapped, err := RefreshBarPair(img)
	if err != nil {
		t.Fatal(err)
	}
	hp, sp := ReadMappedBars(img, mapped)
	if !barAnchorValid(img, mapped.HP, true) || !barAnchorValid(img, mapped.SP, false) {
		t.Fatal("fresh map should pass anchor validation")
	}
	if NeedsRemap(img, mapped, hp, sp) {
		t.Fatalf("fresh map should not need remap (age=%v)", time.Since(mapped.LastMapped))
	}
}

func TestDrift12NoStaleBypassAt50(t *testing.T) {
	img := loadFixture(t, "drift1.2.png")
	mapped, err := RefreshBarPair(img)
	if err != nil {
		t.Fatal(err)
	}
	bypass := scanStaleBypass(t, img, mapped, 50)
	if bypass > 0 {
		t.Fatalf("%d stale offsets trigger below 50%% without NeedsRemap", bypass)
	}
}

func TestDrift12ReadBarsAfterRemap(t *testing.T) {
	img := loadFixture(t, "drift1.2.png")
	mapped, err := RefreshBarPair(img)
	if err != nil {
		t.Fatal(err)
	}
	ap := NewAutoPot(AutoPotConfig{})
	stale := mapped
	stale.HP.X -= 50
	stale.SP.X -= 50
	ap.setMappedBars(stale)

	_, hp, sp, refreshed := ap.readBars(img)
	t.Logf("refreshed=%v hp=%.1f%% sp=%.1f%%", refreshed, hp.Percent, sp.Percent)
	if !refreshed || hp.Percent < 50 || sp.Percent < 50 {
		t.Fatalf("readBars after stale shift hp=%.1f%% sp=%.1f%% refreshed=%v", hp.Percent, sp.Percent, refreshed)
	}
}

func TestDrift12AutopotNoTriggerAt50(t *testing.T) {
	img := loadFixture(t, "drift1.2.png")
	mapped, err := RefreshBarPair(img)
	if err != nil {
		t.Fatal(err)
	}
	hp, sp := ReadMappedBars(img, mapped)
	const th = 50.0
	if (hp.Found && hp.Percent < th) || (sp.Found && sp.Percent < th) {
		t.Fatalf("full HP/SP should not trigger at %.0f%%: hp=%.1f sp=%.1f", th, hp.Percent, sp.Percent)
	}
}

func scanStaleBypass(t *testing.T, img image.Image, mapped MappedBars, threshold float64) int {
	t.Helper()
	bypass := 0
	for dx := -55; dx <= 55; dx++ {
		for dy := -8; dy <= 8; dy++ {
			stale := mapped
			stale.HP.X += dx
			stale.SP.X += dx
			stale.HP.Y += dy
			stale.SP.Y += dy
			hp, sp := ReadMappedBars(img, stale)
			if !NeedsRemap(img, stale, hp, sp) {
				if (hp.Found && hp.Percent < threshold) || (sp.Found && sp.Percent < threshold) {
					bypass++
					t.Logf("bypass dx=%+d dy=%+d hp=%.1f%% sp=%.1f%%", dx, dy, hp.Percent, sp.Percent)
				}
			}
		}
	}
	return bypass
}
