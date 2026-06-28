package runner

import (
	"errors"
	"image"
	"image/color"
	"time"
)

// NumericResourceRead holds the result of parsing numeric HP/SP from the status window.
type NumericResourceRead struct {
	Found      bool
	Current    int
	Max        int
	Percent    float64
	UpdatedAt  time.Time
	Confidence float64 // 0.0 to 1.0
}

// IsStale returns true if the read is older than the given duration.
func (r *NumericResourceRead) IsStale(maxAge time.Duration) bool {
	return time.Since(r.UpdatedAt) > maxAge
}

// Age returns the age of this read in milliseconds.
func (r *NumericResourceRead) Age() int64 {
	return int64(time.Since(r.UpdatedAt).Milliseconds())
}

// NumericRead holds parsed HP and SP values from the status window.
type NumericRead struct {
	HP NumericResourceRead
	SP NumericResourceRead
}

// ParseNumericResources parses numeric HP/SP values from a game screenshot.
// The status window is expected to be in the top-left corner with fixed layout:
//
//	HP. 751 / 1290
//	SP. 102 / 201
//
// Returns NumericRead with Found=false if parsing fails or confidence is too low.
func ParseNumericResources(img image.Image) (NumericRead, error) {
	if img == nil {
		return NumericRead{}, errors.New("image is nil")
	}

	// Step 1: Capture and validate ROI
	roi := CaptureStatusWindowROI(img)
	if roi.Empty() {
		return NumericRead{}, errors.New("invalid status window ROI")
	}

	// Step 2: Extract and preprocess the ROI
	roiImg := ExtractROI(img, roi)
	if roiImg == nil {
		return NumericRead{}, errors.New("failed to extract ROI")
	}

	// Step 2b: Upscale the ROI to make small text readable
	// The status window text is typically very small (2-3 pixels high)
	// Upscale by 4x to make it 8-12 pixels high for better recognition
	roiImg = UpscaleImage(roiImg, 4)

	// Step 3: Preprocess (grayscale, threshold)
	binary := PreprocessImage(roiImg)

	// Step 4: Segment glyphs (connected components)
	glyphs := SegmentGlyphs(binary)
	if len(glyphs) == 0 {
		return NumericRead{}, errors.New("no glyphs detected")
	}

	// Step 5: Recognize glyphs and build the text line
	recognizedLine, avgConfidence := RecognizeGlyphSequence(glyphs)
	if recognizedLine == "" || avgConfidence < minConfidenceThreshold {
		return NumericRead{}, errors.New("recognition confidence too low")
	}

	// Step 6: Parse HP and SP from recognized text
	hp, sp, ok := ParseHPSPLine(recognizedLine)
	if !ok {
		return NumericRead{}, errors.New("failed to parse HP/SP values")
	}

	result := NumericRead{
		HP: NumericResourceRead{
			Found:      hp.Found,
			Current:    hp.Current,
			Max:        hp.Max,
			Percent:    hp.Percent,
			Confidence: avgConfidence,
		},
		SP: NumericResourceRead{
			Found:      sp.Found,
			Current:    sp.Current,
			Max:        sp.Max,
			Percent:    sp.Percent,
			Confidence: avgConfidence,
		},
	}

	return result, nil
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

// CaptureStatusWindowROI returns the fixed ROI for the status window.
// Ragnarok status window is in top-left corner, typically 200x60 pixels.
// This includes both HP and SP lines:
//
//	HP. 751 / 1290
//	SP. 102 / 201
func CaptureStatusWindowROI(img image.Image) image.Rectangle {
	bounds := img.Bounds()

	// Adjusted to focus on just the text, skipping top decorations
	// The actual HP/SP text starts around row 8 in a 250x70 region
	x0, y0 := 15, 20         // Move down to skip decorations
	width, height := 200, 35 // Smaller height, focus on text area

	x1 := x0 + width
	y1 := y0 + height

	// Clamp to image bounds
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

// GlyphBitmap represents a recognized glyph with its bounds and pixel data.
type GlyphBitmap struct {
	X, Y   int      // Top-left position in the binary image
	Width  int      // Width in pixels
	Height int      // Height in pixels
	Pixels [][]bool // Binary pixel data
}

// SegmentGlyphs finds connected components in the binary image.
// Returns glyphs sorted left-to-right by X coordinate.
func SegmentGlyphs(binary [][]bool) []GlyphBitmap {
	if len(binary) == 0 {
		return nil
	}

	height := len(binary)
	width := len(binary[0])

	visited := make([][]bool, height)
	for y := 0; y < height; y++ {
		visited[y] = make([]bool, width)
	}

	var glyphs []GlyphBitmap

	// Find connected components
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if binary[y][x] && !visited[y][x] {
				// Found a new glyph, flood-fill to find its bounds
				glyph := oldFloodFill(binary, visited, x, y)
				if glyph != nil && isValidGlyph(glyph) {
					glyphs = append(glyphs, *glyph)
				}
			}
		}
	}

	// Sort glyphs left-to-right by X coordinate
	sortGlyphsByX(glyphs)

	return glyphs
}

// oldFloodFill finds a connected component starting from (x, y).
// Deprecated: Use connected_components.go instead.
func oldFloodFill(binary [][]bool, visited [][]bool, startX, startY int) *GlyphBitmap {
	height := len(binary)
	width := len(binary[0])

	// Find bounds of connected component
	minX, maxX := startX, startX
	minY, maxY := startY, startY

	queue := [][2]int{{startX, startY}}
	visited[startY][startX] = true

	for len(queue) > 0 {
		x, y := queue[0][0], queue[0][1]
		queue = queue[1:]

		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}

		// Check 4-connected neighbors
		for _, d := range [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}} {
			nx, ny := x+d[0], y+d[1]
			if nx >= 0 && nx < width && ny >= 0 && ny < height &&
				!visited[ny][nx] && binary[ny][nx] {
				visited[ny][nx] = true
				queue = append(queue, [2]int{nx, ny})
			}
		}
	}

	// Extract glyph bitmap
	glyphWidth := maxX - minX + 1
	glyphHeight := maxY - minY + 1
	glyphPixels := make([][]bool, glyphHeight)

	for y := 0; y < glyphHeight; y++ {
		glyphPixels[y] = make([]bool, glyphWidth)
		for x := 0; x < glyphWidth; x++ {
			glyphPixels[y][x] = binary[minY+y][minX+x]
		}
	}

	return &GlyphBitmap{
		X:      minX,
		Y:      minY,
		Width:  glyphWidth,
		Height: glyphHeight,
		Pixels: glyphPixels,
	}
}

// isValidGlyph checks if the glyph is large enough to be a digit (not noise).
func isValidGlyph(g *GlyphBitmap) bool {
	// Minimum size for a digit (adjust based on actual rendering)
	minSize := 3
	return g.Width >= minSize && g.Height >= minSize &&
		g.Width <= 50 && g.Height <= 50 // Maximum size to reject large noise
}

// sortGlyphsByX sorts glyphs left-to-right by X coordinate.
func sortGlyphsByX(glyphs []GlyphBitmap) {
	for i := 0; i < len(glyphs); i++ {
		for j := i + 1; j < len(glyphs); j++ {
			if glyphs[j].X < glyphs[i].X {
				glyphs[i], glyphs[j] = glyphs[j], glyphs[i]
			}
		}
	}
}

// ParseHPSPLine parses the recognized text line to extract HP and SP values.
// Expected format: "HP. XXX / XXX SP. YYY / YYY" or similar.
// Returns (hp, sp, ok).
func ParseHPSPLine(line string) (hp, sp NumericResourceRead, ok bool) {
	// Find the '/' separators
	slashIndices := findAllIndices(line, "/")
	if len(slashIndices) < 2 {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	// Extract HP current/max
	hpCurrentStr, hpMaxStr := extractNumberPair(line, slashIndices[0])
	hpCurrent, hpErr1 := parseNumber(hpCurrentStr)
	hpMax, hpErr2 := parseNumber(hpMaxStr)

	if hpErr1 != nil || hpErr2 != nil || hpMax <= 0 || hpCurrent > hpMax || hpCurrent < 0 {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	// Extract SP current/max
	spCurrentStr, spMaxStr := extractNumberPair(line, slashIndices[1])
	spCurrent, spErr1 := parseNumber(spCurrentStr)
	spMax, spErr2 := parseNumber(spMaxStr)

	if spErr1 != nil || spErr2 != nil || spMax <= 0 || spCurrent > spMax || spCurrent < 0 {
		return NumericResourceRead{}, NumericResourceRead{}, false
	}

	hpPercent := float64(hpCurrent) / float64(hpMax) * 100.0
	spPercent := float64(spCurrent) / float64(spMax) * 100.0

	hp = NumericResourceRead{
		Found:   true,
		Current: hpCurrent,
		Max:     hpMax,
		Percent: hpPercent,
	}

	sp = NumericResourceRead{
		Found:   true,
		Current: spCurrent,
		Max:     spMax,
		Percent: spPercent,
	}

	return hp, sp, true
}

// findAllIndices finds all indices of a substring in a string.
func findAllIndices(s, substr string) []int {
	var indices []int
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			indices = append(indices, i)
		}
	}
	return indices
}

// extractNumberPair extracts the number before and after a separator index.
func extractNumberPair(line string, separatorIdx int) (before, after string) {
	// Extract number before separator
	beforeIdx := separatorIdx - 1
	for beforeIdx >= 0 && isDigit(rune(line[beforeIdx])) {
		beforeIdx--
	}
	beforeIdx++
	before = line[beforeIdx:separatorIdx]

	// Extract number after separator
	afterIdx := separatorIdx + 1
	for afterIdx < len(line) && isDigit(rune(line[afterIdx])) {
		afterIdx++
	}
	after = line[separatorIdx+1 : afterIdx]

	return before, after
}

// isDigit checks if a rune is a digit.
func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// parseNumber parses a number string to an integer.
func parseNumber(s string) (int, error) {
	result := 0
	for _, ch := range s {
		if !isDigit(ch) {
			return 0, errors.New("invalid digit")
		}
		result = result*10 + int(ch-'0')
	}
	return result, nil
}

// minConfidenceThreshold is the minimum confidence to accept a parse.
const minConfidenceThreshold = 0.3
