package runner

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestParseNumericV2GroundTruth tests the new V2 parser against known screenshot values.
func TestParseNumericV2GroundTruth(t *testing.T) {
	// Ground truth values for all test screenshots
	groundTruth := map[string]struct {
		hpCur, hpMax, spCur, spMax int
	}{
		"drift1.png":            {1290, 1290, 201, 201},
		"drift2.png":            {1290, 1290, 201, 201},
		"drift3.png":            {1290, 1290, 201, 201},
		"drift4.png":            {1290, 1290, 201, 201},
		"Drift8.png":            {1290, 1290, 201, 201},
		"assasincrossskill.png": {1290, 1290, 201, 201},
		"drift1.2.png":          {1290, 1290, 201, 201},
		"aa.png":                {751, 1290, 102, 201},
		"drift5.png":            {639, 1290, 33, 201},
		"drift6.png":            {651, 1290, 57, 201},
		"Drift7.png":            {663, 1290, 93, 201},
		"gg.png":                {411, 1254, 117, 195},
		"ii.png":                {1254, 1254, 195, 195},
		"jj.png":                {120, 1280, 6, 201},
		"pp.png":                {1045, 1230, 66, 201},
		"tt.png":                {674, 1290, 18, 201},
		"zoomed1.png":           {675, 1290, 117, 201},
	}

	testdataDir := "testdata"
	passCount := 0
	totalCount := 0

	for filename, gt := range groundTruth {
		totalCount++
		filePath := filepath.Join(testdataDir, filename)

		file, err := os.Open(filePath)
		if err != nil {
			t.Logf("%-30s SKIP (open failed)", filename)
			continue
		}
		defer file.Close()

		img, err := png.Decode(file)
		if err != nil {
			t.Logf("%-30s SKIP (decode failed)", filename)
			continue
		}

		// Parse using V2 parser
		result, err := ParseNumericResourcesV2(img)
		if err != nil {
			t.Logf("%-30s FAIL (parse error: %v)", filename, err)
			continue
		}

		// Check results
		hpMatch := result.HP.Found && result.HP.Current == gt.hpCur && result.HP.Max == gt.hpMax
		spMatch := result.SP.Found && result.SP.Current == gt.spCur && result.SP.Max == gt.spMax

		if hpMatch && spMatch {
			t.Logf("%-30s PASS HP: %3d/%4d  SP: %3d/%3d  conf=%.2f/%.2f",
				filename, result.HP.Current, result.HP.Max, result.SP.Current, result.SP.Max,
				result.HP.Confidence, result.SP.Confidence)
			passCount++
		} else {
			hpStr := ""
			if result.HP.Found {
				hpStr = "OK"
			} else {
				hpStr = "FAIL"
			}
			spStr := ""
			if result.SP.Found {
				spStr = "OK"
			} else {
				spStr = "FAIL"
			}

			t.Logf("%-30s %s HP: got %3d/%4d (want %3d/%4d) SP: got %3d/%4d (want %3d/%4d) conf=%.2f/%.2f",
				filename, hpStr+" "+spStr,
				result.HP.Current, result.HP.Max, gt.hpCur, gt.hpMax,
				result.SP.Current, result.SP.Max, gt.spCur, gt.spMax,
				result.HP.Confidence, result.SP.Confidence)
		}
	}

	t.Logf("\n=== V2 PARSER RESULTS ===")
	t.Logf("Passed: %d/%d (%.1f%%)", passCount, totalCount, float64(passCount)/float64(totalCount)*100)
}

// TestParseNumericV2Debug tests the V2 parser with debug output for a single screenshot.
func TestParseNumericV2Debug(t *testing.T) {
	filename := "aa.png"
	filePath := filepath.Join("testdata", filename)

	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", filename, err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("Failed to decode %s: %v", filename, err)
	}

	t.Logf("Input image: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())

	// Extract status ROI
	statusROI := CaptureStatusWindowROI(img)
	t.Logf("Status ROI: (%d,%d) %dx%d", statusROI.Min.X, statusROI.Min.Y, statusROI.Dx(), statusROI.Dy())

	statusImg := ExtractROI(img, statusROI)
	if statusImg == nil {
		t.Fatalf("Failed to extract status ROI")
	}

	// Upscale
	statusImg = UpscaleImage(statusImg, 4)
	t.Logf("Upscaled: %dx%d", statusImg.Bounds().Dx(), statusImg.Bounds().Dy())

	// Preprocess
	binary := PreprocessImage(statusImg)
	if len(binary) == 0 {
		t.Fatalf("Preprocessing failed")
	}
	t.Logf("Binary image: %dx%d", len(binary[0]), len(binary))

	// Parse with V2
	result, err := ParseNumericResourcesV2(img)
	if err != nil {
		t.Logf("Parse error: %v", err)
		t.Logf("HP: found=%v  %d/%d (%.0f%%)  conf=%.2f",
			result.HP.Found, result.HP.Current, result.HP.Max, result.HP.Percent, result.HP.Confidence)
		t.Logf("SP: found=%v  %d/%d (%.0f%%)  conf=%.2f",
			result.SP.Found, result.SP.Current, result.SP.Max, result.SP.Percent, result.SP.Confidence)
		return
	}

	t.Logf("SUCCESS!")
	t.Logf("HP: %d/%d (%.0f%%)", result.HP.Current, result.HP.Max, result.HP.Percent)
	t.Logf("SP: %d/%d (%.0f%%)", result.SP.Current, result.SP.Max, result.SP.Percent)
}

// TestColumnSegmentation tests the column-based glyph segmentation.
func TestColumnSegmentation(t *testing.T) {
	filename := "aa.png"
	filePath := filepath.Join("testdata", filename)

	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", filename, err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("Failed to decode %s: %v", filename, err)
	}

	// Extract and preprocess
	statusROI := CaptureStatusWindowROI(img)
	statusImg := ExtractROI(img, statusROI)
	statusImg = UpscaleImage(statusImg, 4)
	binary := PreprocessImage(statusImg)

	height := len(binary)
	width := len(binary[0])
	midLine := height / 2

	hpLineROI := image.Rect(0, 0, width, midLine)
	spLineROI := image.Rect(0, midLine, width, height)

	// Test HP line
	hpText := parseLineV2(binary, hpLineROI)
	t.Logf("HP text: %q", hpText)

	// Test SP line
	spText := parseLineV2(binary, spLineROI)
	t.Logf("SP text: %q", spText)

	// Parse values
	hp, sp, ok := ParseHPSPFromText(hpText, spText)
	if ok {
		t.Logf("Parsed HP: %d/%d", hp.Current, hp.Max)
		t.Logf("Parsed SP: %d/%d", sp.Current, sp.Max)
	} else {
		t.Logf("Failed to parse values from HP=%q SP=%q", hpText, spText)
	}
}
