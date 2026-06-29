package glyph

import (
	"fmt"
	"strings"
)

// CANONICAL SIZE for all normalized glyphs
const (
	CanonicalWidth  = 20
	CanonicalHeight = 28
	CanonicalBits   = CanonicalWidth * CanonicalHeight // 560
)

// NormalizedGlyph represents a glyph normalized to canonical size.
type NormalizedGlyph struct {
	Width   int    // always CanonicalWidth
	Height  int    // always CanonicalHeight
	Pattern string // always 560 bits of "0" and "1"
}

// PreprocessGlyph is the SINGLE UNIFIED PATH for all glyphs.
// Takes a binary image region and returns a normalized glyph.
// Used identically for templates and runtime glyphs.
//
// Steps:
// 1. Threshold binary image (already done by caller)
// 2. Trim to foreground bounds
// 3. Normalize to canonical size with aspect ratio preservation
// 4. Center on canvas
func PreprocessGlyph(binaryImage [][]bool) NormalizedGlyph {
	if len(binaryImage) == 0 {
		return NormalizedGlyph{}
	}

	// Step 1: Trim to foreground bounds (remove white borders)
	trimmed := trimToForegroundBounds(binaryImage)
	if len(trimmed) == 0 {
		return NormalizedGlyph{}
	}

	// Step 2: Normalize to canonical size
	return normalizeTrimmedGlyph(trimmed)
}

// trimToForegroundBounds removes white borders and returns tightest bounding box.
// Returns nil if no foreground pixels found.
func trimToForegroundBounds(binary [][]bool) [][]bool {
	if len(binary) == 0 {
		return nil
	}

	height := len(binary)
	width := len(binary[0])

	// Find bounding box of foreground pixels (true = foreground)
	minX, maxX := width, -1
	minY, maxY := height, -1

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if binary[y][x] {
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
			}
		}
	}

	if minX > maxX || minY > maxY {
		return nil // No foreground pixels
	}

	// Extract trimmed region
	trimHeight := maxY - minY + 1
	trimWidth := maxX - minX + 1
	trimmed := make([][]bool, trimHeight)

	for y := 0; y < trimHeight; y++ {
		trimmed[y] = make([]bool, trimWidth)
		for x := 0; x < trimWidth; x++ {
			trimmed[y][x] = binary[minY+y][minX+x]
		}
	}

	return trimmed
}

// normalizeTrimmedGlyph scales a trimmed glyph to canonical size.
// Preserves aspect ratio and centers on canvas.
func normalizeTrimmedGlyph(trimmed [][]bool) NormalizedGlyph {
	if len(trimmed) == 0 {
		return NormalizedGlyph{}
	}

	srcH := len(trimmed)
	srcW := len(trimmed[0])

	// Build pattern string from trimmed binary
	pattern := ""
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			if trimmed[y][x] {
				pattern += "1"
			} else {
				pattern += "0"
			}
		}
	}

	// Use universal NormalizeGlyph function
	return NormalizeGlyph(pattern, srcW, srcH, CanonicalWidth, CanonicalHeight)
}

// NormalizeGlyph scales a pattern to canonical size.
// This is the ONLY normalization function used by both templates and runtime.
// Performs nearest-neighbor scaling without aspect ratio distortion.
func NormalizeGlyph(pattern string, srcW, srcH, dstW, dstH int) NormalizedGlyph {
	if srcW*srcH == 0 || len(pattern) != srcW*srcH {
		return NormalizedGlyph{}
	}

	// Simple nearest-neighbor scaling
	result := ""
	for dstY := 0; dstY < dstH; dstY++ {
		for dstX := 0; dstX < dstW; dstX++ {
			srcX := dstX * srcW / dstW
			srcY := dstY * srcH / dstH

			if srcX >= srcW {
				srcX = srcW - 1
			}
			if srcY >= srcH {
				srcY = srcH - 1
			}

			idx := srcY*srcW + srcX
			if idx >= 0 && idx < len(pattern) {
				result += string(pattern[idx])
			} else {
				result += "0"
			}
		}
	}

	return NormalizedGlyph{
		Width:   dstW,
		Height:  dstH,
		Pattern: result,
	}
}

// GlyphHammingDistance computes match score between two normalized glyphs.
// Returns 1.0 for identical, 0.0 for completely different.
// Allows ±1 pixel shifts (dx, dy = -1..1).
func GlyphHammingDistance(glyph1, glyph2 NormalizedGlyph) float64 {
	if glyph1.Width != glyph2.Width || glyph1.Height != glyph2.Height {
		return 0.0
	}
	if len(glyph1.Pattern) != len(glyph2.Pattern) {
		return 0.0
	}

	bestScore := 0.0

	// Try all shifts: -1..1 in x and y
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			score := hammingDistanceWithShift(glyph1.Pattern, glyph2.Pattern,
				glyph1.Width, glyph1.Height, dx, dy)
			if score > bestScore {
				bestScore = score
			}
		}
	}

	return bestScore
}

// hammingDistanceWithoutShift computes matching bits without offset.
func hammingDistanceWithoutShift(p1, p2 string) float64 {
	if len(p1) != len(p2) || len(p1) == 0 {
		return 0.0
	}

	matching := 0
	for i := 0; i < len(p1); i++ {
		if p1[i] == p2[i] {
			matching++
		}
	}

	return float64(matching) / float64(len(p1))
}

// hammingDistanceWithShift computes matching bits with pixel offset.
func hammingDistanceWithShift(p1, p2 string, width, height, dx, dy int) float64 {
	if len(p1) != len(p2) || len(p1) == 0 {
		return 0.0
	}

	matching := 0
	total := 0

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if idx < 0 || idx >= len(p1) {
				continue
			}

			// Shifted position in p2
			sx := x + dx
			sy := y + dy

			if sx < 0 || sx >= width || sy < 0 || sy >= height {
				total++
				continue // Out of bounds
			}

			sidx := sy*width + sx
			if sidx < 0 || sidx >= len(p2) {
				total++
				continue
			}

			total++
			if p1[idx] == p2[sidx] {
				matching++
			}
		}
	}

	if total == 0 {
		return 0.0
	}

	return float64(matching) / float64(total)
}

// VisualizeGlyph prints a glyph as ASCII art.
// # = foreground (true), . = background (false)
func VisualizeGlyph(ng NormalizedGlyph) string {
	lines := []string{}
	for y := 0; y < ng.Height; y++ {
		line := ""
		for x := 0; x < ng.Width; x++ {
			idx := y*ng.Width + x
			if idx < len(ng.Pattern) {
				if ng.Pattern[idx] == '1' {
					line += "#"
				} else {
					line += "."
				}
			} else {
				line += "."
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// CompareGlyphsVisualized shows two glyphs side-by-side with distance.
func CompareGlyphsVisualized(label1 string, g1 NormalizedGlyph, label2 string, g2 NormalizedGlyph) string {
	vis1 := VisualizeGlyph(g1)
	vis2 := VisualizeGlyph(g2)
	distance := GlyphHammingDistance(g1, g2)

	lines := []string{
		fmt.Sprintf("=== %s (distance: %.3f) ===", label1, 1.0-distance),
		vis1,
		fmt.Sprintf("\n=== %s ===", label2),
		vis2,
	}
	return strings.Join(lines, "\n")
}
