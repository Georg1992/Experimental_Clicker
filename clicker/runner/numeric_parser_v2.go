package runner

import (
	"errors"
	"image"
	"regexp"
	"strconv"
)

// ParseNumericResourcesV2 is the new reliable numeric parser.
// Uses fixed line ROIs, column-based glyph segmentation, and exemplar-based matching.
func ParseNumericResourcesV2(img image.Image) (NumericRead, error) {
	if img == nil {
		return NumericRead{}, errors.New("image is nil")
	}

	// Step 1: Extract status window ROI
	statusROI := CaptureStatusWindowROI(img)
	if statusROI.Empty() {
		return NumericRead{}, errors.New("invalid status window ROI")
	}

	statusImg := ExtractROI(img, statusROI)
	if statusImg == nil {
		return NumericRead{}, errors.New("failed to extract status ROI")
	}

	// Step 2: Upscale for better text quality
	statusImg = UpscaleImage(statusImg, 4)

	// Step 3: Preprocess to binary
	binary := PreprocessImage(statusImg)
	if len(binary) == 0 {
		return NumericRead{}, errors.New("preprocessing failed")
	}

	// Step 4: Define fixed line ROIs within the status image
	// Original ROI is 200x35 after upscaling becomes 800x140
	// HP line is typically in the first half
	// SP line is typically in the second half
	upscaledHeight := len(binary)
	upscaledWidth := len(binary[0])

	// Split into two lines at middle
	midLine := upscaledHeight / 2

	hpLineROI := image.Rect(0, 0, upscaledWidth, midLine)
	spLineROI := image.Rect(0, midLine, upscaledWidth, upscaledHeight)

	// Step 5: Parse each line
	hpText := parseLineV2(binary, hpLineROI)
	spText := parseLineV2(binary, spLineROI)

	// Step 6: Validate and extract values from parsed text
	hp, sp, ok := ParseHPSPFromText(hpText, spText)
	if !ok {
		return NumericRead{}, errors.New("failed to parse HP/SP values")
	}

	// Calculate confidence based on recognition
	confidence := 0.8 // Fixed confidence if successfully parsed
	if hpText != "" && spText != "" {
		confidence = 0.9
	}

	result := NumericRead{
		HP: NumericResourceRead{
			Found:      hp.Found,
			Current:    hp.Current,
			Max:        hp.Max,
			Percent:    hp.Percent,
			Confidence: confidence,
		},
		SP: NumericResourceRead{
			Found:      sp.Found,
			Current:    sp.Current,
			Max:        sp.Max,
			Percent:    sp.Percent,
			Confidence: confidence,
		},
	}

	return result, nil
}

// parseLineV2 parses a single HP or SP line using column-based segmentation.
// Returns recognized text like "751/1290" or "102/201".
func parseLineV2(binary [][]bool, lineROI image.Rectangle) string {
	if lineROI.Empty() || len(binary) == 0 {
		return ""
	}

	// Step 1: Find connected components of foreground pixels
	components := FindConnectedComponents(binary, lineROI)

	// Step 2: Convert components to glyph bounding boxes
	// Merge components that are within 10 pixels of each other
	glyphs := BoundingBoxesToGlyphs(components, 10)

	// Step 3: Recognize each glyph using exemplar-based matching
	recognized := ""
	lib := NewGlyphExemplarLibrary()

	for _, glyphROI := range glyphs {
		char := recognizeGlyphV2Exemplar(binary, glyphROI, lib)
		if char != 0 {
			recognized += string(char)
		}
	}

	return recognized
}

// recognizeGlyphV2Exemplar recognizes a single glyph using exemplar-based matching.
func recognizeGlyphV2Exemplar(binary [][]bool, glyphROI image.Rectangle, lib *GlyphExemplarLibrary) rune {
	height := glyphROI.Dy()
	width := glyphROI.Dx()

	if height < 3 || width < 2 {
		return 0 // Too small
	}

	// Extract glyph pattern
	glyphPattern := buildGlyphPattern(binary, glyphROI)
	if glyphPattern == "" {
		return 0
	}

	// Match against exemplars
	char, bestScore, _, confidence := lib.MatchGlyph(glyphPattern, width, height)

	// Confidence threshold
	// We need good separation between best and second best
	if confidence < 0.1 {
		return 0 // Too uncertain
	}

	// Also check absolute score (should be reasonably high)
	if bestScore < 0.6 {
		return 0
	}

	return char
}

// buildGlyphPattern creates a binary pattern from a glyph ROI.
func buildGlyphPattern(binary [][]bool, glyphROI image.Rectangle) string {
	pattern := ""
	for y := glyphROI.Min.Y; y < glyphROI.Max.Y; y++ {
		for x := glyphROI.Min.X; x < glyphROI.Max.X; x++ {
			if y < len(binary) && x < len(binary[y]) {
				if binary[y][x] {
					pattern += "1"
				} else {
					pattern += "0"
				}
			} else {
				pattern += "0"
			}
		}
	}

	return pattern
}

// Note: buildGlyphPattern is also defined in glyph_exemplars.go
// ParseHPSPFromText extracts HP and SP values from recognized text.
// Expected format: hpText="751/1290" spText="102/201"
func ParseHPSPFromText(hpText, spText string) (hp, sp NumericResourceRead, ok bool) {
	// Parse HP
	if hpText == "" {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	hpCur, hpMax, err1 := parseValuePair(hpText)
	if err1 != nil {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	// Parse SP
	spCur, spMax, err2 := parseValuePair(spText)
	if err2 != nil {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	// Validate ranges
	if hpMax <= 0 || hpCur < 0 || hpCur > hpMax {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}
	if spMax <= 0 || spCur < 0 || spCur > spMax {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	hp = NumericResourceRead{
		Found:   true,
		Current: hpCur,
		Max:     hpMax,
		Percent: float64(hpCur) / float64(hpMax) * 100.0,
	}

	sp = NumericResourceRead{
		Found:   true,
		Current: spCur,
		Max:     spMax,
		Percent: float64(spCur) / float64(spMax) * 100.0,
	}

	return hp, sp, true
}

// parseValuePair parses "current/max" format.
func parseValuePair(text string) (current, max int, err error) {
	// Remove spaces
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, "")

	// Find slash
	parts := regexp.MustCompile(`/`).Split(text, -1)
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid format: missing slash")
	}

	current, err1 := strconv.Atoi(parts[0])
	max, err2 := strconv.Atoi(parts[1])

	if err1 != nil || err2 != nil {
		return 0, 0, errors.New("parse error: invalid numbers")
	}

	return current, max, nil
}
