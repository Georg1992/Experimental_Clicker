package runner

import (
	_ "image/png"
	"image"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type screenBarCase struct {
	file  string
	hpUI  string
	spUI  string
	hpPct float64
	spPct float64
}

func knownBarCases() map[string]screenBarCase {
	return map[string]screenBarCase{
		"aa.png":      {file: "aa.png", hpUI: "751/1290", spUI: "102/201", hpPct: 751.0 / 1290.0 * 100, spPct: 102.0 / 201.0 * 100},
		"gg.png":      {file: "gg.png", hpUI: "411/1254", spUI: "117/195", hpPct: 411.0 / 1254.0 * 100, spPct: 117.0 / 195.0 * 100},
		"ii.png":      {file: "ii.png", hpUI: "1254/1254", spUI: "195/195", hpPct: 100, spPct: 100},
		"jj.png":      {file: "jj.png", hpUI: "120/1290", spUI: "6/201", hpPct: 120.0 / 1290.0 * 100, spPct: 6.0 / 201.0 * 100},
		"pp.png":      {file: "pp.png", hpUI: "1045/1290", spUI: "66/201", hpPct: 1045.0 / 1290.0 * 100, spPct: 66.0 / 201.0 * 100},
		"tt.png":      {file: "tt.png", hpUI: "674/1290", spUI: "18/201", hpPct: 674.0 / 1290.0 * 100, spPct: 18.0 / 201.0 * 100},
		"drift1.png":  {file: "drift1.png", hpUI: "1290/1290", spUI: "201/201", hpPct: 100, spPct: 100},
		"drift1.2.png": {file: "drift1.2.png", hpUI: "1290/1290", spUI: "201/201", hpPct: 100, spPct: 100},
		"drift2.png":  {file: "drift2.png", hpUI: "1290/1290", spUI: "201/201", hpPct: 100, spPct: 100},
		"drift3.png":  {file: "drift3.png", hpUI: "1290/1290", spUI: "201/201", hpPct: 100, spPct: 100},
		"drift4.png":  {file: "drift4.png", hpUI: "1290/1290", spUI: "201/201", hpPct: 100, spPct: 100},
		"drift5.png":  {file: "drift5.png", hpUI: "639/1290", spUI: "33/201", hpPct: 639.0 / 1290.0 * 100, spPct: 33.0 / 201.0 * 100},
		"drift6.png":  {file: "drift6.png", hpUI: "651/1290", spUI: "57/201", hpPct: 651.0 / 1290.0 * 100, spPct: 57.0 / 201.0 * 100},
		"Drift7.png":  {file: "Drift7.png", hpUI: "683/1290", spUI: "93/201", hpPct: 683.0 / 1290.0 * 100, spPct: 93.0 / 201.0 * 100},
		"Drift8.png":  {file: "Drift8.png", hpUI: "1290/1290", spUI: "201/201", hpPct: 100, spPct: 100},
		"zoomed1.png": {file: "zoomed1.png", hpUI: "675/1290", spUI: "117/201", hpPct: 675.0 / 1290.0 * 100, spPct: 117.0 / 201.0 * 100},
	}
}

func fixturePNGFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(testdataDir(t))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".png") {
			continue
		}
		if strings.HasPrefix(name, "debug_") {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no fixture png files in testdata")
	}
	return files
}

func TestRefreshBarPairFixtures(t *testing.T) {
	known := knownBarCases()
	for _, file := range fixturePNGFiles(t) {
		t.Run(file, func(t *testing.T) {
			img := loadFixture(t, file)
			mapped, err := RefreshBarPair(img)
			if err != nil {
				t.Fatalf("RefreshBarPair: %v", err)
			}
			hp := ReadHPFill(img, mapped.HP)
			sp := ReadSPFill(img, mapped.SP)
			if !hp.Found || !sp.Found {
				t.Fatalf("bars not found hp=%v sp=%v", hp.Found, sp.Found)
			}

			assertBarPairGeometry(t, mapped)

			debugName := "debug_" + file
			_ = SaveMappedBarsDebug(img, mapped, filepath.Join(t.TempDir(), debugName))

			t.Logf("HPRect=%+v SPRect=%+v score=%d hp=%.1f%% sp=%.1f%%",
				mapped.HP, mapped.SP, mapped.MapScore, hp.Percent, sp.Percent)

			tc, ok := known[file]
			if !ok {
				t.Fatalf("missing UI validation for %s", file)
			}
			hpDelta := hp.Percent - tc.hpPct
			spDelta := sp.Percent - tc.spPct
			t.Logf("HP game %s = %.1f%%  detected %.1f%%  delta %+.1f", tc.hpUI, tc.hpPct, hp.Percent, hpDelta)
			t.Logf("SP game %s = %.1f%%  detected %.1f%%  delta %+.1f", tc.spUI, tc.spPct, sp.Percent, spDelta)
			if !barDeltaOK(hp.Percent, tc.hpPct, mapped.HP.W) {
				t.Fatalf("HP delta %+.1f exceeds tolerance (got %.1f%% want %.1f%%)", hpDelta, hp.Percent, tc.hpPct)
			}
			if !barDeltaOK(sp.Percent, tc.spPct, mapped.SP.W) {
				t.Fatalf("SP delta %+.1f exceeds tolerance (got %.1f%% want %.1f%%)", spDelta, sp.Percent, tc.spPct)
			}
		})
	}
}

func assertBarPairGeometry(t *testing.T, mapped MappedBars) {
	t.Helper()
	if !mapped.Valid {
		t.Fatal("mapped bars not valid")
	}
	if mapped.HP.Y >= mapped.SP.Y {
		t.Fatalf("HP must be above SP: HP=%+v SP=%+v", mapped.HP, mapped.SP)
	}
	if mapped.HP.X != mapped.SP.X {
		t.Fatalf("HP/SP X mismatch: HP=%+v SP=%+v", mapped.HP, mapped.SP)
	}
	if mapped.HP.W != mapped.SP.W {
		t.Fatalf("HP/SP W mismatch: HP=%+v SP=%+v", mapped.HP, mapped.SP)
	}
	if mapped.HP.W < 1 || mapped.SP.W < 1 {
		t.Fatalf("invalid bar width: HP=%+v SP=%+v", mapped.HP, mapped.SP)
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
