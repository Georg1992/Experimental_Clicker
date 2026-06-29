package glyph

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// GlyphExemplarLibrary stores normalized exemplars for matching.
type GlyphExemplarLibrary struct {
	// exemplars maps symbol (rune) to normalized exemplar
	exemplars map[rune]NormalizedGlyph
	// debugDir for saving intermediate processing steps
	debugDir string
}

// NewGlyphExemplarLibrary creates a library by loading templates from disk.
func NewGlyphExemplarLibrary() *GlyphExemplarLibrary {
	lib := &GlyphExemplarLibrary{
		exemplars: make(map[rune]NormalizedGlyph),
	}

	// Load from testdata/Digits folder
	lib.loadFromDisk()

	return lib
}

// SetDebugDir enables debug output saving.
func (lib *GlyphExemplarLibrary) SetDebugDir(debugDir string) {
	lib.debugDir = debugDir
	if debugDir != "" {
		os.MkdirAll(debugDir, 0755)
	}
}

// loadFromDisk loads templates from testdata/Digits/*.png files.
func (lib *GlyphExemplarLibrary) loadFromDisk() bool {
	trainingDir := filepath.Join("testdata", "Digits")

	// Check if directory exists
	_, err := os.Stat(trainingDir)
	if err != nil {
		return false
	}

	digitNames := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "slash", "HP", "SP"}
	loadedCount := 0

	for _, name := range digitNames {
		filePath := filepath.Join(trainingDir, name+".png")

		// Load PNG file
		file, err := os.Open(filePath)
		if err != nil {
			continue // File not found, skip
		}

		img, err := png.Decode(file)
		file.Close()

		if err != nil {
			continue
		}

		// Convert PNG to binary image (gray < 150 = foreground/black)
		binary := imageToGrayscaleBinary(img)
		if len(binary) == 0 {
			continue
		}

		// Save debug: original binary
		if lib.debugDir != "" {
			lib.saveBinaryDebugImage(filepath.Join(lib.debugDir, name+"_01_binary.png"), binary)
		}

		// Trim to foreground bounds (UNIFIED FUNCTION)
		trimmed := trimToForegroundBounds(binary)
		if len(trimmed) == 0 {
			continue
		}

		// Save debug: trimmed
		if lib.debugDir != "" {
			lib.saveBinaryDebugImage(filepath.Join(lib.debugDir, name+"_02_trimmed.png"), trimmed)
		}

		// Normalize using UNIFIED FUNCTION
		normalized := PreprocessGlyph(trimmed)
		if len(normalized.Pattern) != CanonicalBits {
			continue
		}

		// Save debug: normalized
		if lib.debugDir != "" {
			lib.saveNormalizedDebugImage(filepath.Join(lib.debugDir, name+"_03_normalized.png"), normalized)
		}

		// Register exemplar with special character codes for labels
		var char rune
		switch name {
		case "slash":
			char = '/'
		case "HP":
			char = rune(0xF001) // Private use area code for HP label
		case "SP":
			char = rune(0xF002) // Private use area code for SP label
		default:
			char = rune(name[0])
		}

		lib.exemplars[char] = normalized
		loadedCount++
	}

	return loadedCount > 0
}

// imageToGrayscaleBinary converts image to binary, gray < 150 = foreground.
func imageToGrayscaleBinary(img image.Image) [][]bool {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	binary := make([][]bool, height)
	for y := 0; y < height; y++ {
		binary[y] = make([]bool, width)
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			// Convert to 8-bit grayscale using SAME formula as PreprocessImage
			gray := uint8((299*r + 587*g + 114*b) / 1000 >> 8)
			// Foreground (black) if gray < 150
			binary[y][x] = gray < 150
		}
	}

	return binary
}

// saveBinaryDebugImage saves a binary image as PNG for debugging.
func (lib *GlyphExemplarLibrary) saveBinaryDebugImage(filePath string, binary [][]bool) error {
	height := len(binary)
	if height == 0 {
		return nil
	}
	width := len(binary[0])

	img := image.NewGray(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if binary[y][x] {
				img.SetGray(x, y, color.Gray{0}) // Black foreground
			} else {
				img.SetGray(x, y, color.Gray{255}) // White background
			}
		}
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

// saveNormalizedDebugImage saves a normalized glyph as PNG.
func (lib *GlyphExemplarLibrary) saveNormalizedDebugImage(filePath string, ng NormalizedGlyph) error {
	img := image.NewGray(image.Rect(0, 0, ng.Width, ng.Height))

	for y := 0; y < ng.Height; y++ {
		for x := 0; x < ng.Width; x++ {
			idx := y*ng.Width + x
			if idx < len(ng.Pattern) {
				if ng.Pattern[idx] == '1' {
					img.SetGray(x, y, color.Gray{0}) // Black
				} else {
					img.SetGray(x, y, color.Gray{255}) // White
				}
			}
		}
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

// MatchGlyph finds best matching exemplar for a runtime glyph.
// Returns (bestChar, distance, secondDistance, confidence).
// distance = 1.0 - HammingScore, so smaller is better (0 = perfect match).
func (lib *GlyphExemplarLibrary) MatchGlyph(runtimeNormalized NormalizedGlyph) (rune, float64, float64, float64) {
	if len(lib.exemplars) == 0 {
		return 0, 1.0, 1.0, 0.0
	}

	type matchInfo struct {
		char     rune
		distance float64
	}

	var matches []matchInfo

	// Score against all exemplars
	for char, exemplar := range lib.exemplars {
		hammingScore := GlyphHammingDistance(runtimeNormalized, exemplar)
		distance := 1.0 - hammingScore // Convert to distance (0 = identical)
		matches = append(matches, matchInfo{char, distance})
	}

	if len(matches) == 0 {
		return 0, 1.0, 1.0, 0.0
	}

	// Find best and second-best (lowest distance)
	var best, secondBest matchInfo
	best.distance = 2.0
	secondBest.distance = 2.0

	for _, m := range matches {
		if m.distance < best.distance {
			secondBest = best
			best = m
		} else if m.distance < secondBest.distance {
			secondBest = m
		}
	}

	// Confidence = gap between best and second-best
	confidence := secondBest.distance - best.distance
	if confidence < 0 {
		confidence = 0
	}

	return best.char, best.distance, secondBest.distance, confidence
}

// DebugMatching logs detailed match information.
func (lib *GlyphExemplarLibrary) DebugMatching(runtimeLabel string, runtime NormalizedGlyph, bestChar rune, bestDist, secondDist, conf float64) string {
	lines := []string{
		fmt.Sprintf("=== Matching %s ===", runtimeLabel),
		fmt.Sprintf("Best: '%c' distance=%.3f", bestChar, bestDist),
		fmt.Sprintf("Second: distance=%.3f", secondDist),
		fmt.Sprintf("Confidence: %.3f", conf),
	}

	if bestChar != 0 {
		exemplar := lib.exemplars[bestChar]
		comparison := CompareGlyphsVisualized(runtimeLabel, runtime, string(bestChar), exemplar)
		lines = append(lines, comparison)
	}

	return fmt.Sprintf("%v", lines)
}

// GetHPTemplate returns the normalized HP label template
func (lib *GlyphExemplarLibrary) GetHPTemplate() (NormalizedGlyph, bool) {
	template, ok := lib.exemplars[rune(0xF001)]
	return template, ok
}

// GetSPTemplate returns the normalized SP label template
func (lib *GlyphExemplarLibrary) GetSPTemplate() (NormalizedGlyph, bool) {
	template, ok := lib.exemplars[rune(0xF002)]
	return template, ok
}

// GetSlashTemplate returns the normalized slash separator template
func (lib *GlyphExemplarLibrary) GetSlashTemplate() (NormalizedGlyph, bool) {
	template, ok := lib.exemplars['/']
	return template, ok
}

// GetTemplate returns a template by label name ("HP", "SP", "slash", or digit string)
func (lib *GlyphExemplarLibrary) GetTemplate(label string) (NormalizedGlyph, bool) {
	var char rune
	switch label {
	case "HP":
		char = rune(0xF001)
	case "SP":
		char = rune(0xF002)
	case "slash":
		char = '/'
	default:
		if len(label) == 1 {
			char = rune(label[0])
		} else {
			return NormalizedGlyph{}, false
		}
	}

	template, ok := lib.exemplars[char]
	return template, ok
}
