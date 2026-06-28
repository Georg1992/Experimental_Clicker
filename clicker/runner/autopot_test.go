package runner

import "testing"

func newTestStabilizers(threshold int) (*BarPairCache, *BarStabilizer, *BarStabilizer) {
	pair := &BarPairCache{}
	return pair, NewBarStabilizer(pair, true, threshold), NewBarStabilizer(pair, false, threshold)
}

func TestAutoPotUpdateSettings(t *testing.T) {
	ap := NewAutoPot(AutoPotConfig{
		HPEnabled:   true,
		HPKeyVK:     'W',
		HPThreshold: 50,
		SPThreshold: 30,
	})

	ap.UpdateSettings(AutoPotConfig{
		HPEnabled:   true,
		HPKeyVK:     'W',
		HPThreshold: 60,
		SPThreshold: 30,
	})
	cfg := ap.settings()
	if cfg.HPThreshold != 60 || cfg.SPThreshold != 30 {
		t.Fatalf("after HP edit cfg=%d/%d want 60/30", cfg.HPThreshold, cfg.SPThreshold)
	}

	ap.UpdateSettings(AutoPotConfig{
		HPEnabled:   true,
		HPKeyVK:     'W',
		HPThreshold: 75,
		SPEnabled:   true,
		SPKeyVK:     'E',
		SPThreshold: 50,
	})
	cfg = ap.settings()
	if cfg.HPThreshold != 75 || cfg.SPThreshold != 50 {
		t.Fatalf("after SP edit cfg=%d/%d want 75/50", cfg.HPThreshold, cfg.SPThreshold)
	}
}

func TestStabilizerRejectsStaleOffset(t *testing.T) {
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

	_, hpStab, _ := newTestStabilizers(50)
	read := hpStab.Update(img, true)
	if !read.Found || read.Percent < fresh.Percent-3 {
		t.Fatalf("Update=%.1f%% want ~%.1f%% (stale was %.1f%%)", read.Percent, fresh.Percent, staleHP.Percent)
	}
	if read.Status == BarStatusLow {
		t.Fatalf("Update=%.1f%% should not be low at 50%% threshold", read.Percent)
	}
}

func TestStabilizerDetectsLowHP(t *testing.T) {
	img := loadFixture(t, "jj.png")
	_, hpStab, _ := newTestStabilizers(50)

	var read StableBarRead
	for i := 0; i < PotConfirmReads; i++ {
		read = hpStab.Update(img, true)
	}
	if !read.Found || read.Percent > 15 {
		t.Fatalf("low HP read %.1f%%", read.Percent)
	}
	if read.Status != BarStatusLow {
		t.Fatalf("low HP %.1f%% want Status=Low after %d reads, got %v", read.Percent, PotConfirmReads, read.Status)
	}
}

func TestStabilizerFullAfterStablePair(t *testing.T) {
	img := loadFixture(t, "drift1.2.png")
	_, hpStab, spStab := newTestStabilizers(50)

	hp := hpStab.Update(img, true)
	sp := spStab.Update(img, false)
	if hp.Status != BarStatusFull || hp.Percent < 99.9 {
		t.Fatalf("full HP hp=%.1f%% status=%v", hp.Percent, hp.Status)
	}
	if sp.Status != BarStatusFull || sp.Percent < 99.9 {
		t.Fatalf("full SP sp=%.1f%% status=%v", sp.Percent, sp.Status)
	}
}

func TestFullLatchHoldsThroughGlitchRead(t *testing.T) {
	img := loadFixture(t, "Drift8.png")
	_, hpStab, _ := newTestStabilizers(80)

	read := hpStab.Update(img, true)
	if read.Status != BarStatusFull {
		t.Fatal("full read should latch")
	}

	hpStab.mu.Lock()
	glitch := BarRead{Found: true, Percent: 32, FilledWidth: 20, FullWidth: read.Rect.W}
	effective := hpStab.applyTrustedReadLocked(img, read.Rect, glitch)
	hpStab.mu.Unlock()

	if effective.Status != BarStatusFull || effective.Percent < 99.9 {
		t.Fatalf("latched read %.1f%% status=%v should stay full on glitch", effective.Percent, effective.Status)
	}
}

func TestFullLatchClearsAfterDamage(t *testing.T) {
	fullImg := loadFixture(t, "Drift8.png")
	lowImg := loadFixture(t, "jj.png")
	_, hpStab, _ := newTestStabilizers(50)

	read := hpStab.Update(fullImg, true)
	if read.Status != BarStatusFull {
		t.Fatal("full read should latch")
	}

	var effective StableBarRead
	for i := 0; i < PotUnlatchReads; i++ {
		effective = hpStab.Update(lowImg, true)
	}
	if effective.Status == BarStatusFull || effective.Percent >= 99.9 {
		t.Fatalf("latch should clear after damage, got %.1f%% status=%v", effective.Percent, effective.Status)
	}

	for i := 0; i < PotConfirmReads; i++ {
		effective = hpStab.Update(lowImg, true)
	}
	if effective.Status != BarStatusLow {
		t.Fatalf("low HP %.1f%% should be low after latch cleared", effective.Percent)
	}
}

func TestFullLatchNoReadWithoutStoredRect(t *testing.T) {
	stab := NewBarStabilizer(&BarPairCache{}, true, 50)
	stab.mu.Lock()
	stab.fullLatched = true
	stab.mu.Unlock()

	read := stab.latchedFullOrUnknown()
	if read.Status == BarStatusFull {
		t.Fatal("latched without stored rect must not return full")
	}
}

func TestPartialBarsNotDetectedAsFull(t *testing.T) {
	partials := []string{"aa.png", "jj.png", "pp.png", "drift5.png", "Drift7.png"}
	for _, name := range partials {
		t.Run(name, func(t *testing.T) {
			img := loadFixture(t, name)
			mapped, err := RefreshBarPair(img)
			if err != nil {
				t.Fatal(err)
			}
			hp, sp := ReadMappedBars(img, mapped)
			tc := knownBarCases()[name]
			if tc.hpPct < 99.9 && barLooksFull(img, mapped.HP, true) {
				t.Fatalf("HP %.1f%% (game %.1f%%) falsely detected full", hp.Percent, tc.hpPct)
			}
			if tc.spPct < 99.9 && barLooksFull(img, mapped.SP, false) {
				t.Fatalf("SP %.1f%% (game %.1f%%) falsely detected full", sp.Percent, tc.spPct)
			}
		})
	}
}

func TestFullBarNeverReportsLow(t *testing.T) {
	img := loadFixture(t, "Drift8.png")
	_, hpStab, _ := newTestStabilizers(80)

	for i := 0; i < PotConfirmReads; i++ {
		read := hpStab.Update(img, true)
		if read.Status == BarStatusLow {
			t.Fatalf("full bar must not report low on read %d", i+1)
		}
	}
}

func TestFullBarStableAcrossShiftedRects(t *testing.T) {
	img := loadFixture(t, "Drift8.png")
	for dx := -50; dx <= 50; dx += 5 {
		if dx == 0 {
			continue
		}
		mapped, err := RefreshBarPair(img)
		if err != nil {
			t.Fatal(err)
		}
		mapped.HP.X += dx
		mapped.SP.X += dx
		if barLooksFull(img, mapped.HP, true) {
			t.Fatalf("dx=%+d shifted rect must not look full", dx)
		}
	}
}

func TestDriftFullBarsNeverNeedPotion(t *testing.T) {
	for _, file := range []string{"drift1.2.png", "Drift8.png", "ii.png"} {
		t.Run(file, func(t *testing.T) {
			img := loadFixture(t, file)
			_, hpStab, spStab := newTestStabilizers(99)

			for _, threshold := range []int{1, 50, 80, 99} {
				hpStab.SetThreshold(threshold)
				spStab.SetThreshold(threshold)

				for i := 0; i < PotConfirmReads; i++ {
					hp := hpStab.Update(img, true)
					if hp.Status == BarStatusLow {
						t.Fatalf("threshold %d: HP %.1f%% must not be low", threshold, hp.Percent)
					}
					sp := spStab.Update(img, false)
					if sp.Status == BarStatusLow {
						t.Fatalf("threshold %d: SP %.1f%% must not be low", threshold, sp.Percent)
					}
				}
			}
		})
	}
}

func TestStabilizerUnknownNeverLow(t *testing.T) {
	stab := NewBarStabilizer(&BarPairCache{}, true, 50)
	read := stab.Update(nil, true)
	if read.Status == BarStatusLow {
		t.Fatal("invalid input must not return low")
	}
	if read.Status != BarStatusUnknown {
		t.Fatalf("invalid input want unknown, got %v", read.Status)
	}
}
