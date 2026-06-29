package glyph

// RecognizeGlyph recognizes a single glyph using exemplar-based matching.
// This is the ONLY module responsible for glyph recognition.
//
// Input: binary glyph image, glyph library
// Output: recognized character (rune)
//
// Uses unified normalization pipeline: extract → trim → normalize → compare.
func RecognizeGlyph(glyphBinary [][]bool, lib *GlyphExemplarLibrary) rune {
	if len(glyphBinary) == 0 {
		return 0
	}

	// Step 1: Normalize the glyph to canonical size using PreprocessGlyph
	normalized := PreprocessGlyph(glyphBinary)
	if len(normalized.Pattern) != CanonicalBits {
		return 0
	}

	// Step 2: Match against library
	char, distance, _, _ := lib.MatchGlyph(normalized)

	// Step 3: Accept if distance is reasonable
	// Distance < 0.5 means at least 50% of bits match
	if distance > 0.5 {
		return 0
	}

	return char
}
