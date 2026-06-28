package runner

import (
	"errors"
	"image"
	"regexp"
	"strconv"
)

// ParseNumericResourcesV2 is the new reliable numeric parser.
// Uses fixed line ROIs and column-based glyph segmentation.
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

	// Step 6: Extract values from parsed text
	hp, spParsed, ok := ParseHPSPFromText(hpText, spText)
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
			Found:      spParsed.Found,
			Current:    spParsed.Current,
			Max:        spParsed.Max,
			Percent:    spParsed.Percent,
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

	height := lineROI.Dy()
	width := lineROI.Dx()
	y0 := lineROI.Min.Y
	x0 := lineROI.Min.X

	// Step 1: Find columns with foreground pixels
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

	// Step 2: Group adjacent columns into glyphs
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
				// End of glyph
				glyphEnd := x
				glyphs = append(glyphs, image.Rect(glyphStart, y0, glyphEnd, y0+height))
				inGlyph = false
			}
		}
	}
	if inGlyph {
		glyphs = append(glyphs, image.Rect(glyphStart, y0, width+x0, y0+height))
	}

	// Step 3: Recognize each glyph
	recognized := ""
	for _, glyphROI := range glyphs {
		char := recognizeGlyphV2(binary, glyphROI)
		if char != 0 {
			recognized += string(char)
		}
	}

	return recognized
}

// recognizeGlyphV2 recognizes a single glyph using template matching.
func recognizeGlyphV2(binary [][]bool, glyphROI image.Rectangle) rune {
	height := glyphROI.Dy()
	width := glyphROI.Dx()

	if height < 3 || width < 2 {
		return 0 // Too small
	}

	// Use game font template library
	lib := NewGameFontTemplateLibrary()
	char, confidence := lib.MatchGlyphV2(binary, glyphROI)

	// Accept if confidence is reasonable (lowered threshold to 0.3)
	if confidence < 0.3 {
	}

	// Parse SP
	spCur, spMax, err2 := parseValuePair(spText)
	if err2 != nil {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	// Validate
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
		return 0, 0, errors.New("invalid format")
	}

	current, err1 := strconv.Atoi(parts[0])
	max, err2 := strconv.Atoi(parts[1])

	if err1 != nil || err2 != nil {
		return 0, 0, errors.New("parse error")
	}

	return current, max, nil
}
