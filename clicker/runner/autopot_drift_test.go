package runner

import "testing"

// Stale cache during walk can read ~49% when HP is ~58% without NeedsRemap firing.
func TestReadBarForPotIgnoresStaleCache(t *testing.T) {
	img := loadFixture(t, "aa.png")
	mapped, err := RefreshBarPair(img)
	if err != nil {
		t.Fatal(err)
	}
	fresh, _ := ReadMappedBars(img, mapped)

	stale := mapped
	stale.HP.X += 10
	stale.HP.Y += 2
	stale.SP.X += 10
	stale.SP.Y += 2
	staleHP, _ := ReadMappedBars(img, stale)
	if staleHP.Percent >= 50 {
		t.Fatalf("setup: stale read %.1f%% should be below 50%%", staleHP.Percent)
	}

	ap := NewAutoPot(AutoPotConfig{})
	ap.setMappedBars(stale)
	_, read := ap.readBarForPot(img, true)
	if read.Percent < fresh.Percent-3 {
		t.Fatalf("readBarForPot=%.1f%% want ~%.1f%% (stale was %.1f%%)", read.Percent, fresh.Percent, staleHP.Percent)
	}
	if read.Percent < 50 {
		t.Fatalf("readBarForPot=%.1f%% should not trigger 50%% threshold after remap", read.Percent)
	}
}

func TestReadBarForPotKeepsLegitLow(t *testing.T) {
	img := loadFixture(t, "jj.png")
	ap := NewAutoPot(AutoPotConfig{})
	_, read := ap.readBarForPot(img, true)
	if !read.Found || read.Percent > 15 {
		t.Fatalf("low HP fixture read %.1f%% want well below threshold", read.Percent)
	}
}
