package runner

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestGlyphRecognitionDebug tests glyph recognition with detailed output.
func TestGlyphRecognitionDebug(t *testing.T) {
	filename := "aa.png"
	filePath := filepath.Join("testdata", filename)

	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open %s", filename)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("Failed to decode %s", filename)
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

	// Get columns with foreground
	x0 := hpLineROI.Min.X
	y0 := hpLineROI.Min.Y
	lineHeight := hpLineROI.Dy()
	lineWidth := hpLineROI.Dx()

	columnHasPixel := make([]bool, lineWidth)
	for x := 0; x < lineWidth; x++ {
		for y := 0; y < lineHeight; y++ {
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

	for x := 0; x < lineWidth; x++ {
		if columnHasPixel[x] {
			if !inGlyph {
				glyphStart = x
				inGlyph = true
			}
		} else {
			if inGlyph {
				glyphEnd := x
				glyphs = append(glyphs, image.Rect(glyphStart+x0, y0, glyphEnd+x0, y0+lineHeight))
				inGlyph = false
			}
		}
	}
	if inGlyph {
		glyphs = append(glyphs, image.Rect(glyphStart+x0, y0, lineWidth+x0, y0+lineHeight))
	}

	t.Logf("Found %d glyphs in HP line", len(glyphs))

	// Test recognition on each glyph
	lib := NewGameFontTemplateLibrary()

	for i, glyphROI := range glyphs {
		if i > 10 {
			break // Limit output
		}

		// Try matching
		char, confidence := lib.MatchGlyphV2(binary, glyphROI)

		t.Logf("  Glyph %d: ROI=(%d,%d) %dx%d  char='%c'  confidence=%.3f",
			i, glyphROI.Min.X, glyphROI.Min.Y, glyphROI.Dx(), glyphROI.Dy(), char, confidence)
	}
}

// TestTemplateScoring tests the template scoring algorithm.
func TestTemplateScoring(t *testing.T) {
	// Create a simple test pattern
	// Expected digit '1': narrow column of pixels

	testPattern1 := ""
	for i := 0; i < 16*24; i++ {
		if (i%16) >= 7 && (i%16) <= 8 { // Narrow column in middle
			testPattern1 += "1"
		} else {
			testPattern1 += "0"
		}
	}

	lib := NewGameFontTemplateLibrary()

	// Score against all digits
	for ch := '0'; ch <= '9'; ch++ {
		tmpl := lib.templates[ch]
		if tmpl == nil {
			continue
		}

		score := hammingScore(testPattern1, tmpl.Pattern)
		t.Logf("Digit '%c': score=%.3f", ch, score)
	}

	// Also test slash
	score := hammingScore(testPattern1, lib.templates['/'].Pattern)
	t.Logf("Slash '/': score=%.3f", score)
}
