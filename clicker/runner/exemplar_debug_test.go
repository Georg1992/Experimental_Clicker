package runner

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestExemplarMatchingDebug tests the exemplar matching in detail.
func TestExemplarMatchingDebug(t *testing.T) {
	testdataDir := "testdata"
	filename := "aa.png"
	filePath := filepath.Join(testdataDir, filename)

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

	upscaledHeight := len(binary)
	upscaledWidth := len(binary[0])

	midLine := upscaledHeight / 2
	hpLineROI := image.Rect(0, 0, upscaledWidth, midLine)

	// Extract HP line glyphs
	t.Logf("HP Line ROI: %v (size %d x %d)", hpLineROI, hpLineROI.Dx(), hpLineROI.Dy())

	// Find connected components
	components := FindConnectedComponents(binary, hpLineROI)
	t.Logf("Found %d connected components", len(components))

	// Convert to glyphs with merge threshold 10
	glyphs := BoundingBoxesToGlyphs(components, 10)

	// Merge small fragments with neighbors
	const minGlyphWidth = 6
	mergedGlyphs := []image.Rectangle{}
	for i, g := range glyphs {
		if g.Dx() < minGlyphWidth && i > 0 && len(mergedGlyphs) > 0 {
			// Merge with previous glyph
			last := mergedGlyphs[len(mergedGlyphs)-1]
			mergedGlyphs[len(mergedGlyphs)-1] = image.Rect(last.Min.X, last.Min.Y, g.Max.X, g.Max.Y)
		} else {
			mergedGlyphs = append(mergedGlyphs, g)
		}
	}
	glyphs = mergedGlyphs

	t.Logf("Found %d glyphs in HP line", len(glyphs))

	lib := NewGlyphExemplarLibrary()
	recognized := ""

	for i, glyphROI := range glyphs {
		glyphPattern := buildGlyphPattern(binary, glyphROI)
		if glyphPattern == "" {
			t.Logf("Glyph %d: Empty pattern", i)
			continue
		}

		gw := glyphROI.Dx()
		gh := glyphROI.Dy()

		t.Logf("\nGlyph %d: ROI %v (size %d x %d), pattern len=%d", i, glyphROI, gw, gh, len(glyphPattern))

		// Count foreground bits
		foregroundCount := 0
		for _, bit := range glyphPattern {
			if bit == '1' {
				foregroundCount++
			}
		}
		t.Logf("  Foreground pixels: %d / %d (%.1f%%)", foregroundCount, len(glyphPattern),
			float64(foregroundCount)/float64(len(glyphPattern))*100)

		char, bestScore, secondBestScore, confidence := lib.MatchGlyph(glyphPattern, gw, gh)

		t.Logf("  Match: char='%c' (%.3f), second (%.3f), conf=%.3f",
			char, bestScore, secondBestScore, confidence)

		if char != 0 {
			recognized += string(char)
		}
	}

	t.Logf("\nRecognized HP text: '%s' (expected: '751/1290')", recognized)
}

// TestNormalizeGlyphFunction tests the normalization pipeline.
func TestNormalizeGlyphFunction(t *testing.T) {
	// Create a simple test pattern: a digit '1' shape
	// Narrow vertical line in middle
	pattern := ""
	for row := 0; row < 10; row++ {
		for col := 0; col < 5; col++ {
			if col == 2 {
				pattern += "1"
			} else {
				pattern += "0"
			}
		}
	}

	t.Logf("Source pattern: 5x10, %d bits", len(pattern))

	// Normalize to 20x28
	normalized := NormalizeGlyph(pattern, 5, 10, 20, 28)

	t.Logf("Normalized: %dx%d, %d bits", normalized.Width, normalized.Height, len(normalized.Pattern))

	if len(normalized.Pattern) != 20*28 {
		t.Fatalf("Expected 560 bits, got %d", len(normalized.Pattern))
	}

	// Count foreground bits
	foreground := 0
	for _, bit := range normalized.Pattern {
		if bit == '1' {
			foreground++
		}
	}

	t.Logf("Foreground ratio after normalization: %d / %d (%.1f%%)", foreground,
		len(normalized.Pattern), float64(foreground)/float64(len(normalized.Pattern))*100)

	// Visualize middle column
	t.Logf("\nMiddle column (x=10) of normalized glyph:")
	for y := 0; y < 28; y++ {
		idx := y*20 + 10
		if normalized.Pattern[idx] == '1' {
			t.Logf("  Row %d: X", y)
		} else {
			t.Logf("  Row %d: .", y)
		}
	}
}

// TestExemplarLibraryLoading tests if exemplars load correctly.
func TestExemplarLibraryLoading(t *testing.T) {
	lib := NewGlyphExemplarLibrary()

	digits := []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '/'}

	for _, digit := range digits {
		exemplars, ok := lib.exemplars[digit]
		if !ok {
			t.Fatalf("Digit '%c' not found in library", digit)
		}

		t.Logf("Digit '%c': %d exemplars", digit, len(exemplars))

		for i, ex := range exemplars {
			if len(ex.Pattern) != 560 {
				t.Errorf("  Exemplar %d: Invalid size %d (expected 560)", i, len(ex.Pattern))
			}

			// Count foreground
			fg := 0
			for _, bit := range ex.Pattern {
				if bit == '1' {
					fg++
				}
			}

			if i == 0 {
				t.Logf("  Size: %dx%d, foreground: %d / %d", ex.Width, ex.Height, fg, len(ex.Pattern))
			}
		}
	}
}

// TestHammingDistanceFormula tests the matching score computation.
func TestHammingDistanceFormula(t *testing.T) {
	// Identical patterns
	p1 := "111100001111"
	p2 := "111100001111"
	score1 := hammingDistanceNormalized(p1, p2)
	if score1 != 1.0 {
		t.Fatalf("Identical patterns should have score 1.0, got %.3f", score1)
	}
	t.Logf("Identical patterns: %.3f ✓", score1)

	// Opposite patterns
	p3 := "111100001111"
	p4 := "000011110000"
	score2 := hammingDistanceNormalized(p3, p4)
	if score2 != 0.0 {
		t.Fatalf("Opposite patterns should have score 0.0, got %.3f", score2)
	}
	t.Logf("Opposite patterns: %.3f ✓", score2)

	// 50% match
	p5 := "111100001111"
	p6 := "111111110000"
	score3 := hammingDistanceNormalized(p5, p6)
	t.Logf("50%% match: %.3f", score3)
	if score3 <= 0.5 || score3 >= 0.8 {
		t.Logf("  (Expected ~0.67)")
	}
}
