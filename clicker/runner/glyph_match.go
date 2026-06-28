package runner

import (
	"math"
)

// glyphMatcher handles template-based glyph recognition.
type glyphMatcher struct {
	library *TemplateLibrary
}

// NewGlyphMatcher creates a new glyph matcher.
func NewGlyphMatcher() *glyphMatcher {
	return &glyphMatcher{
		library: NewTemplateLibrary(),
	}
}

// MatchGlyph attempts to match a glyph bitmap to a digit or separator character.
// Returns (character, confidence) where confidence is 0.0 to 1.0.
// Confidence is based on normalized Hamming distance between normalized bitmaps.
func (m *glyphMatcher) MatchGlyph(glyph *GlyphBitmap) (rune, float64) {
	if glyph == nil || glyph.Width == 0 || glyph.Height == 0 {
		return 0, 0.0
	}

	bestChar := rune(0)
	bestScore := 0.0

	// Try matching against all templates
	for ch, template := range m.library.AllTemplates() {
		score := m.scoreMatch(glyph.Pixels, template.Pixels)
		if score > bestScore {
			bestScore = score
			bestChar = ch
		}
	}

	return bestChar, bestScore
}

// scoreMatch computes a similarity score between two binary bitmaps.
// Uses normalized Hamming distance:
//  1. Normalize both bitmaps to a common size
//  2. Count matching pixels
//  3. Return matching_pixels / total_pixels as confidence
func (m *glyphMatcher) scoreMatch(bitmap1, bitmap2 [][]bool) float64 {
	if len(bitmap1) == 0 || len(bitmap2) == 0 {
		return 0.0
	}

	// Resize both to a common size for comparison
	const normalizedSize = 16
	norm1 := resizeBitmap(bitmap1, normalizedSize, normalizedSize)
	norm2 := resizeBitmap(bitmap2, normalizedSize, normalizedSize)

	// Count matching pixels
	matching := 0
	total := normalizedSize * normalizedSize

	for y := 0; y < normalizedSize; y++ {
		for x := 0; x < normalizedSize; x++ {
			if norm1[y][x] == norm2[y][x] {
				matching++
			}
		}
	}

	// Confidence = matching pixels / total pixels
	confidence := float64(matching) / float64(total)

	return confidence
}

// resizeBitmap resizes a bitmap to a target size using nearest-neighbor interpolation.
func resizeBitmap(bitmap [][]bool, targetWidth, targetHeight int) [][]bool {
	if len(bitmap) == 0 || len(bitmap[0]) == 0 {
		return make([][]bool, targetHeight)
	}

	srcHeight := len(bitmap)
	srcWidth := len(bitmap[0])

	result := make([][]bool, targetHeight)
	for y := 0; y < targetHeight; y++ {
		result[y] = make([]bool, targetWidth)

		// Map target Y to source Y
		srcY := y * srcHeight / targetHeight
		if srcY >= srcHeight {
			srcY = srcHeight - 1
		}

		for x := 0; x < targetWidth; x++ {
			// Map target X to source X
			srcX := x * srcWidth / targetWidth
			if srcX >= srcWidth {
				srcX = srcWidth - 1
			}

			result[y][x] = bitmap[srcY][srcX]
		}
	}

	return result
}

// RecognizeGlyphSequence recognizes a sequence of glyphs and returns the text line.
// Also returns the average confidence score.
func RecognizeGlyphSequence(glyphs []GlyphBitmap) (string, float64) {
	if len(glyphs) == 0 {
		return "", 0.0
	}

	matcher := NewGlyphMatcher()
	result := ""
	totalConfidence := 0.0

	for _, glyph := range glyphs {
		ch, confidence := matcher.MatchGlyph(&glyph)
		if ch == 0 {
			// Failed to recognize - might be noise or unknown character
			// For now, skip unknown glyphs
			continue
		}

		result += string(ch)
		totalConfidence += confidence
	}

	if len(result) == 0 {
		return "", 0.0
	}

	avgConfidence := totalConfidence / float64(len(result))
	return result, avgConfidence
}

// ConfidenceThreshold is the minimum confidence to accept a single glyph match.
const ConfidenceThreshold = 0.60

// FilterGlyphsByConfidence returns only glyphs that match with high confidence.
func FilterGlyphsByConfidence(glyphs []GlyphBitmap, minConfidence float64) []GlyphBitmap {
	if minConfidence < 0 {
		minConfidence = ConfidenceThreshold
	}

	matcher := NewGlyphMatcher()
	var filtered []GlyphBitmap

	for i, glyph := range glyphs {
		_, confidence := matcher.MatchGlyph(&glyph)
		if confidence >= minConfidence {
			filtered = append(filtered, glyphs[i])
		}
	}

	return filtered
}

// ComputeConfidenceScore computes a combined confidence for the recognized line.
// Considers:
//   - Average per-glyph confidence
//   - Presence of required separators (/)
//   - Value validity (current <= max, etc.)
func ComputeConfidenceScore(recognizedLine string, avgGlyphConfidence float64) float64 {
	if recognizedLine == "" {
		return 0.0
	}

	confidence := avgGlyphConfidence

	// Require at least one separator
	if !contains(recognizedLine, "/") {
		confidence *= 0.5
	}

	// Require digits (at least 3 digits per number)
	digitCount := 0
	for _, ch := range recognizedLine {
		if isDigit(ch) {
			digitCount++
		}
	}

	if digitCount < 3 {
		confidence *= 0.5
	}

	// Clamp to valid range
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return confidence
}

// contains checks if a string contains a rune.
func contains(s string, ch string) bool {
	for _, c := range s {
		if string(c) == ch {
			return true
		}
	}
	return false
}

// HammingDistance computes the Hamming distance between two binary bitmaps of the same size.
func HammingDistance(bitmap1, bitmap2 [][]bool) int {
	if len(bitmap1) != len(bitmap2) || len(bitmap1[0]) != len(bitmap2[0]) {
		return math.MaxInt32
	}

	distance := 0
	for y := 0; y < len(bitmap1); y++ {
		for x := 0; x < len(bitmap1[0]); x++ {
			if bitmap1[y][x] != bitmap2[y][x] {
				distance++
			}
		}
	}

	return distance
}

// NormalizedHammingDistance computes normalized Hamming distance (0 to 1).
// First normalizes bitmaps to the same size, then computes distance.
// Returns 1.0 - (normalized distance), so 1.0 is perfect match.
func NormalizedHammingDistance(bitmap1, bitmap2 [][]bool, normalizedSize int) float64 {
	norm1 := resizeBitmap(bitmap1, normalizedSize, normalizedSize)
	norm2 := resizeBitmap(bitmap2, normalizedSize, normalizedSize)

	distance := HammingDistance(norm1, norm2)
	maxDistance := normalizedSize * normalizedSize

	if maxDistance == 0 {
		return 0.0
	}

	// Return similarity (1 = perfect match, 0 = completely different)
	return 1.0 - float64(distance)/float64(maxDistance)
}
