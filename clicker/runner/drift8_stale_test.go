package runner

import (
	"testing"
)

func TestDrift8StaleAt80(t *testing.T) {
	img := loadFixture(t, "Drift8.png")
	mapped, err := RefreshBarPair(img)
	if err != nil {
		t.Fatal(err)
	}
	for _, dx := range []int{-40, -30, -20, -15, -10, -8, -5, 0, 5, 8, 10, 15, 20, 30, 40} {
		stale := mapped
		stale.HP.X += dx
		stale.SP.X += dx
		hp := ReadHPFill(img, stale.HP)
		sp := ReadSPFill(img, stale.SP)
		below := ""
		if hp.Found && hp.Percent < 80 {
			below = " *** TRIGGERS 80%"
		}
		t.Logf("dx=%+3d hp=%.1f%% found=%v sp=%.1f%%%s", dx, hp.Percent, hp.Found, sp.Percent, below)
	}
	for _, dy := range []int{-5, -3, -2, -1, 0, 1, 2, 3, 5} {
		stale := mapped
		stale.HP.Y += dy
		stale.SP.Y += dy
		hp := ReadHPFill(img, stale.HP)
		remap := NeedsRemap(img, stale, hp, ReadSPFill(img, stale.SP))
		below := ""
		if hp.Found && hp.Percent < 80 {
			below = " *** TRIGGERS 80%"
		}
		t.Logf("dy=%+2d hp=%.1f%% remap=%v%s", dy, hp.Percent, remap, below)
	}
	for _, dx := range []int{-40, 30, 40} {
		stale := mapped
		stale.HP.X += dx
		stale.SP.X += dx
		hp := ReadHPFill(img, stale.HP)
		remap := NeedsRemap(img, stale, hp, ReadSPFill(img, stale.SP))
		if !remap {
			t.Errorf("dx=%+d hp=%.1f%% should need remap", dx, hp.Percent)
		}
	}
}
