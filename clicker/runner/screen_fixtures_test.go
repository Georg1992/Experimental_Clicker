package runner

import (
	_ "image/png"
	"image"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type screenBarCase struct {
	file  string
	hpUI  string
	spUI  string
	hpPct float64
	spPct float64
}

func screenBarCases() []screenBarCase {
	return []screenBarCase{
		{file: "aa.png", hpUI: "751/1290", spUI: "102/201", hpPct: 751.0 / 1290.0 * 100, spPct: 102.0 / 201.0 * 100},
		{file: "gg.png", hpUI: "411/1254", spUI: "117/195", hpPct: 411.0 / 1254.0 * 100, spPct: 117.0 / 195.0 * 100},
		{file: "ii.png", hpUI: "1254/1254", spUI: "195/195", hpPct: 100, spPct: 100},
		{file: "jj.png", hpUI: "120/1290", spUI: "6/201", hpPct: 120.0 / 1290.0 * 100, spPct: 6.0 / 201.0 * 100},
		{file: "pp.png", hpUI: "1045/1290", spUI: "66/201", hpPct: 1045.0 / 1290.0 * 100, spPct: 66.0 / 201.0 * 100},
		{file: "tt.png", hpUI: "674/1290", spUI: "18/201", hpPct: 674.0 / 1290.0 * 100, spPct: 18.0 / 201.0 * 100},
	}
}

func TestFindHPBarScreens(t *testing.T) {
	for _, tc := range screenBarCases() {
		t.Run(tc.file, func(t *testing.T) {
			img := loadFixture(t, tc.file)
			mapped, err := MapPlayerBars(img)
			if err != nil {
				t.Fatalf("MapPlayerBars: %v", err)
			}
			hp := ReadHPFill(img, mapped.HP)
			sp := ReadSPFill(img, mapped.SP)
			if !hp.Found || !sp.Found {
				t.Fatalf("bars not found hp=%v sp=%v", hp.Found, sp.Found)
			}

			debugName := "debug_" + tc.file
			_ = SaveMappedBarsDebug(img, mapped, filepath.Join(testdataDir(t), debugName))

			hpDelta := hp.Percent - tc.hpPct
			spDelta := sp.Percent - tc.spPct
			t.Logf("HP game %s = %.1f%%  detected %.1f%%  delta %+.1f", tc.hpUI, tc.hpPct, hp.Percent, hpDelta)
			t.Logf("SP game %s = %.1f%%  detected %.1f%%  delta %+.1f", tc.spUI, tc.spPct, sp.Percent, spDelta)
			t.Logf("HPRect=%+v SPRect=%+v score=%d", mapped.HP, mapped.SP, mapped.MapScore)

			if !barDeltaOK(hp.Percent, tc.hpPct, mapped.HP.W) {
				t.Fatalf("HP delta %+.1f exceeds tolerance (got %.1f%% want %.1f%%)", hpDelta, hp.Percent, tc.hpPct)
			}
			if !barDeltaOK(sp.Percent, tc.spPct, mapped.SP.W) {
				t.Fatalf("SP delta %+.1f exceeds tolerance (got %.1f%% want %.1f%%)", spDelta, sp.Percent, tc.spPct)
			}
		})
	}
}

func barDeltaOK(got, want float64, barW int) bool {
	if want < 10 {
		pxDelta := absInt(int(got/100*float64(barW)+0.5) - int(want/100*float64(barW)+0.5))
		if pxDelta <= 3 {
			return true
		}
		return math.Abs(got-want) <= 5
	}
	return math.Abs(got-want) <= 4
}

func loadFixture(t *testing.T, name string) image.Image {
	t.Helper()
	path := filepath.Join(testdataDir(t), name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("fixture missing %s: %v", name, err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return img
}

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "testdata")
}
