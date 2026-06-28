package runner

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// ExtractRealGameFontTemplates learns digit templates from known screenshots.
// This should be run once to create templates, then templates are hardcoded in code.
func TestExtractRealGameFontTemplates(t *testing.T) {
	// Ground truth: position index in value -> digit character
	// Format: "current/max"
	// For "751/1290": positions are ['7','5','1','/',  '1','2','9','0']
	// For "102/201": positions are ['1','0','2','/',  '2','0','1']

	groundTruth := map[string]struct {
		hpCur, hpMax, spCur, spMax int
	}{
		"drift1.png":  {1290, 1290, 201, 201},
		"aa.png":      {751, 1290, 102, 201},
		"drift5.png":  {639, 1290, 33, 201},
		"drift6.png":  {651, 1290, 57, 201},
		"Drift7.png":  {663, 1290, 93, 201},
		"gg.png":      {411, 1254, 117, 195},
		"ii.png":      {1254, 1254, 195, 195},
		"jj.png":      {120, 1280, 6, 201},
		"pp.png":      {1045, 1230, 66, 201},
		"tt.png":      {674, 1290, 18, 201},
		"zoomed1.png": {675, 1290, 117, 201},
	}

	// Map: digit -> list of extracted glyph patterns
	digitSamples := make(map[rune][]string)

	testdataDir := "testdata"

	for filename, gt := range groundTruth {
		filePath := filepath.Join(testdataDir, filename)
		file, err := os.Open(filePath)
		if err != nil {
			t.Logf("Skip %s: open failed", filename)
			continue
		}
		defer file.Close()

		img, err := png.Decode(file)
		if err != nil {
			t.Logf("Skip %s: decode failed", filename)
			continue
		}

		// Extract and preprocess
		statusROI := CaptureStatusWindowROI(img)
		statusImg := ExtractROI(img, statusROI)
		statusImg = UpscaleImage(statusImg, 4)
		binary := PreprocessImage(statusImg)

		height := len(binary)
		width := len(binary[0])
		midLine := height / 2

		// Extract HP digits
		hpLineROI := image.Rect(0, 0, width, midLine)
		extractDigitSamples(t, binary, hpLineROI, gt.hpCur, gt.hpMax, false, digitSamples)

		// Extract SP digits
		spLineROI := image.Rect(0, midLine, width, height)
		extractDigitSamples(t, binary, spLineROI, gt.spCur, gt.spMax, true, digitSamples)
	}

	// Print template code
	t.Logf("\n=== TEMPLATE CODE ===")
	t.Logf("// Add this to game_font_templates.go")
	t.Logf("")
	t.Logf("func buildGameFontTemplatesFromRealGame() map[rune]string {")
	t.Logf("\ttemplates := make(map[rune]string)")
	t.Logf("")

	for ch := '0'; ch <= '9'; ch++ {
		samples := digitSamples[ch]
		if len(samples) > 0 {
			// Use first sample as template
			pattern := samples[0]
			t.Logf("\t// Digit '%c' (%d samples)", ch, len(samples))
			t.Logf("\ttemplates['%c'] = %q", ch, pattern)
		}
	}

	// Add slash
	samples := digitSamples['/']
	if len(samples) > 0 {
		t.Logf("\t// Slash '/' (%d samples)", len(samples))
		t.Logf("\ttemplates['/'] = %q", samples[0])
	}

	t.Logf("\treturn templates")
	t.Logf("}")

	t.Logf("\n=== SAMPLE STATISTICS ===")
	for ch := '0'; ch <= '9'; ch++ {
		samples := digitSamples[ch]
		t.Logf("Digit '%c': %d samples", ch, len(samples))
	}
	t.Logf("Slash '/': %d samples", len(digitSamples['/']))
}

// extractDigitSamples extracts digit glyphs from a line and adds them to digitSamples.
func extractDigitSamples(t *testing.T, binary [][]bool, lineROI image.Rectangle,
	cur, max int, isSP bool, digitSamples map[rune][]string) {

	// Build expected digit sequence
	curStr := fmt.Sprintf("%d", cur)
	maxStr := fmt.Sprintf("%d", max)
	expectedDigits := curStr + "/" + maxStr

	// Column-based segmentation
	height := lineROI.Dy()
	width := lineROI.Dx()
	x0 := lineROI.Min.X
	y0 := lineROI.Min.Y

	columnHasPixel := make([]bool, width)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			absY := y0 + y
			absX := x0 + x
			if absY < len(binary) && absX < len(binary[absY]) {
				if binary[absY][absX] {
					columnHasPixel[x] = true
					break
				}
			}
		}
	}

	// Group columns into glyphs
	glyphs := []image.Rectangle{}
	inGlyph := false
	glyphStart := 0

	for x := 0; x < width; x++ {
		if columnHasPixel[x] {
			if !inGlyph {
				glyphStart = x
				inGlyph = true
			}
		} else {
			if inGlyph {
				glyphEnd := x
				glyphs = append(glyphs, image.Rect(glyphStart+x0, y0, glyphEnd+x0, y0+height))
				inGlyph = false
			}
		}
	}
	if inGlyph {
		glyphs = append(glyphs, image.Rect(glyphStart+x0, y0, width+x0, y0+height))
	}

	// Label glyphs with expected digits
	if len(glyphs) >= len(expectedDigits) {
		for i, char := range expectedDigits {
			if i < len(glyphs) {
				glyphROI := glyphs[i]
				pattern := buildGlyphPattern(binary, glyphROI)
				digitSamples[char] = append(digitSamples[char], pattern)
			}
		}
	}
}

// TestTemplateMatchingOnRealPatterns tests template matching with extracted patterns.
func TestTemplateMatchingOnRealPatterns(t *testing.T) {
	// This would use extracted patterns to test recognition
	// For now, it's a placeholder for comprehensive testing
	t.Log("Template matching on real patterns - to be implemented")
}
