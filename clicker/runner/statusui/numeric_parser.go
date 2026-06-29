package statusui

import (
	"errors"
	"image"
	"image/color"
	"regexp"
	"strconv"
)

// parseLineV2 parses a single HP or SP line using column-based segmentation.
// Returns recognized text like "751/1290" or "102/201".
func parseLineV2(binary [][]bool, lineROI image.Rectangle) string {
	if lineROI.Empty() || len(binary) == 0 {
		return ""
	}

	// Step 1: Find connected components of foreground pixels
	components := glyph.FindConnectedComponents(binary, lineROI)

	// Step 2: Convert components to glyph bounding boxes
	// Merge components that are within 10 pixels of each other
	glyphs := glyph.BoundingBoxesToGlyphs(components, 10)

	// Step 3: Recognize each glyph using exemplar-based matching
	recognized := ""
	lib := glyph.NewGlyphExemplarLibrary()

	for _, glyphROI := range glyphs {
		char := recognizeGlyphV2Exemplar(binary, glyphROI, lib)
		if char != 0 {
			recognized += string(char)
		}
	}

	return recognized
}

// recognizeGlyphV2Exemplar recognizes a single glyph using exemplar-based matching.
// Uses unified normalization pipeline: extract ROI → trim → normalize → compare.
func recognizeGlyphV2Exemplar(binary [][]bool, glyphROI image.Rectangle, lib *glyph.GlyphExemplarLibrary) rune {
	height := glyphROI.Dy()
	width := glyphROI.Dx()

	if height < 3 || width < 2 {
		return 0 // Too small
	}

	// Step 1: Extract binary glyph from ROI
	glyphBinary := extractBinaryROI(binary, glyphROI)
	if len(glyphBinary) == 0 {
		return 0
	}

	// Step 2: Preprocess using UNIFIED FUNCTION
	// This does: trim to foreground bounds, then normalize to canonical size
	runtimeNormalized := glyph.PreprocessGlyph(glyphBinary)
	if len(runtimeNormalized.Pattern) != glyph.CanonicalBits {
		return 0
	}

	// Step 3: Match against exemplars
	char, distance, _, _ := lib.MatchGlyph(runtimeNormalized)

	// Accept if distance is reasonable (0 = perfect match, 1 = opposite)
	// Distance < 0.5 means at least 50% of bits match
	if distance > 0.5 {
		return 0
	}

	return char
}

// extractBinaryROI extracts a binary region from the larger binary image.
func extractBinaryROI(binary [][]bool, roi image.Rectangle) [][]bool {
	height := len(binary)
	width := 0
	if height > 0 {
		width = len(binary[0])
	}

	// Clip ROI to image bounds
	minX := roi.Min.X
	minY := roi.Min.Y
	maxX := roi.Max.X
	maxY := roi.Max.Y

	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > width {
		maxX = width
	}
	if maxY > height {
		maxY = height
	}

	if minX >= maxX || minY >= maxY {
		return nil
	}

	// Extract region
	extracted := make([][]bool, maxY-minY)
	for y := minY; y < maxY; y++ {
		row := make([]bool, maxX-minX)
		for x := minX; x < maxX; x++ {
			if y < len(binary) && x < len(binary[y]) {
				row[x-minX] = binary[y][x]
			}
		}
		extracted[y-minY] = row
	}

	return extracted
}

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

// ParseHPSPFromFullLine extracts HP and SP values from a full line text.
// Expected format may include labels like: "HP.XXX/XXXX|SP.AAA/BBB"
// It extracts numeric sequences (digits/slashes) and parses them as HP/SP values.
func ParseHPSPFromFullLine(fullLine string) (hp, sp NumericResourceRead, ok bool) {
	if fullLine == "" {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	// Extract all digit/slash sequences from the line
	// Remove labels like "HP", "SP", dots, pipes, spaces
	cleanedLine := ""
	for _, ch := range fullLine {
		if (ch >= '0' && ch <= '9') || ch == '/' {
			cleanedLine += string(ch)
		} else if ch == '|' {
			// Pipe separates HP and SP sections
			cleanedLine += "|"
		}
		// Skip everything else (letters, dots, spaces, etc.)
	}

	if cleanedLine == "" {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	// Now cleanedLine should be something like "751/1290|102/201"
	var hpText, spText string

	// Try splitting by pipe separator
	parts := regexp.MustCompile(`\|`).Split(cleanedLine, -1)
	if len(parts) >= 2 {
		hpText = parts[0]
		spText = parts[1]
	} else if len(parts) == 1 {
		// No pipe separator, entire line might be HP or might include both
		// Try to detect if there are two slash patterns
		slashCount := regexp.MustCompile(`/`).FindAllString(cleanedLine, -1)
		if len(slashCount) >= 2 {
			// Multiple slash patterns - try to split in middle
			matches := regexp.MustCompile(`(\d+)/(\d+)`).FindAllStringIndex(cleanedLine, -1)
			if len(matches) >= 2 {
				// Use first pattern as HP, second as SP
				hpText = cleanedLine[matches[0][0]:matches[0][1]]
				spText = cleanedLine[matches[1][0]:matches[1][1]]
			} else {
				hpText = cleanedLine
			}
		} else {
			hpText = cleanedLine
		}
	}

	// Extract HP value
	if hpText == "" {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	hpMatch := regexp.MustCompile(`^(\d+)/(\d+)`).FindStringSubmatch(hpText)
	if len(hpMatch) < 3 {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	hpCur, _ := strconv.Atoi(hpMatch[1])
	hpMax, _ := strconv.Atoi(hpMatch[2])

	if hpMax <= 0 || hpCur < 0 || hpCur > hpMax {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	hp = NumericResourceRead{
		Found:   true,
		Current: hpCur,
		Max:     hpMax,
		Percent: float64(hpCur) / float64(hpMax) * 100.0,
	}

	// Extract SP value if available
	if spText != "" {
		spMatch := regexp.MustCompile(`^(\d+)/(\d+)`).FindStringSubmatch(spText)
		if len(spMatch) >= 3 {
			spCur, _ := strconv.Atoi(spMatch[1])
			spMax, _ := strconv.Atoi(spMatch[2])

			if spMax > 0 && spCur >= 0 && spCur <= spMax {
				sp = NumericResourceRead{
					Found:   true,
					Current: spCur,
					Max:     spMax,
					Percent: float64(spCur) / float64(spMax) * 100.0,
				}
				return hp, sp, true
			}
		}
	}

	// Return HP even if SP parsing failed
	return hp, NumericResourceRead{}, true
}

// CaptureStatusWindowROI returns the fixed ROI for the status window.
// Ragnarok status window is in top-left corner, typically 200x60 pixels.
// This includes both HP and SP lines:
//
//	HP. 751 / 1290
//	SP. 102 / 201

// CaptureStatusWindowROI returns the fixed ROI for the status window (HP/SP line only).
func CaptureStatusWindowROI(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	x0, y0 := 10, 37
	width, height := 200, 18
	x1 := x0 + width
	y1 := y0 + height
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}
	return image.Rect(x0, y0, x1, y1)
}

// ExtractROI extracts a rectangular region from the image.
func ExtractROI(img image.Image, roi image.Rectangle) image.Image {
	bounds := img.Bounds()

	// Clamp ROI to bounds
	roi = roi.Intersect(bounds)
	if roi.Empty() {
		return nil
	}

	// Create a new image for this ROI
	roiImg := image.NewRGBA(image.Rect(0, 0, roi.Dx(), roi.Dy()))
	for y := 0; y < roi.Dy(); y++ {
		for x := 0; x < roi.Dx(); x++ {
			roiImg.Set(x, y, img.At(roi.Min.X+x, roi.Min.Y+y))
		}
	}

	return roiImg
}

// UpscaleImage enlarges an image using nearest-neighbor interpolation
func UpscaleImage(img image.Image, factor int) image.Image {
	if factor <= 1 {
		return img
	}

	bounds := img.Bounds()
	newWidth := bounds.Dx() * factor
	newHeight := bounds.Dy() * factor
	upscaled := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			// Fill a factor×factor block with the same color
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					upscaled.SetRGBA(x*factor+dx, y*factor+dy, color.RGBA{
						R: uint8(r >> 8),
						G: uint8(g >> 8),
						B: uint8(b >> 8),
						A: uint8(a >> 8),
					})
				}
			}
		}
	}

	return upscaled
}

// PreprocessImage converts the image to binary (black/white) for glyph recognition.
// Steps:
//  1. Convert to grayscale
//  2. Apply threshold (text is BLACK digits on WHITE background)
//  3. Black pixels become TRUE (foreground), white pixels become FALSE
func PreprocessImage(img image.Image) [][]bool {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Convert to grayscale and threshold
	binary := make([][]bool, height)
	for y := 0; y < height; y++ {
		binary[y] = make([]bool, width)
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()

			// Convert to 8-bit grayscale
			gray := uint8((299*r + 587*g + 114*b) / 1000 >> 8)

			// Apply threshold: Black/dark pixels are foreground (text)
			// Threshold at 150 - pixels darker than this are text
			// Game font is typically pure black (0) on white (255)
			binary[y][x] = gray < 150
		}
	}

	return binary
}
