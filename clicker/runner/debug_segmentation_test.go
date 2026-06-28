package runner

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestDebugSegmentation shows what's happening in the segmentation pipeline.
func TestDebugSegmentation(t *testing.T) {
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

	// Step 1: Extract status ROI
	statusROI := CaptureStatusWindowROI(img)
	statusImg := ExtractROI(img, statusROI)
	t.Logf("Status ROI: %dx%d", statusImg.Bounds().Dx(), statusImg.Bounds().Dy())

	// Step 2: Upscale
	statusImg = UpscaleImage(statusImg, 4)
	t.Logf("After upscale: %dx%d", statusImg.Bounds().Dx(), statusImg.Bounds().Dy())

	// Step 3: Preprocess
	binary := PreprocessImage(statusImg)
	t.Logf("Binary size: %dx%d", len(binary[0]), len(binary))

	// Step 4: Check for foreground pixels
	totalPixels := 0
	foregroundPixels := 0
	for y := 0; y < len(binary); y++ {
		for x := 0; x < len(binary[y]); x++ {
			totalPixels++
			if binary[y][x] {
				foregroundPixels++
			}
		}
	}
	t.Logf("Foreground ratio: %d / %d = %.2f%%", foregroundPixels, totalPixels, float64(foregroundPixels)/float64(totalPixels)*100)

	// Step 5: Find columns with foreground
	width := len(binary[0])
	height := len(binary)

	columnsWithForeground := 0
	for x := 0; x < width; x++ {
		hasPixel := false
		for y := 0; y < height; y++ {
			if binary[y][x] {
				hasPixel = true
				break
			}
		}
		if hasPixel {
			columnsWithForeground++
		}
	}
	t.Logf("Columns with foreground: %d / %d", columnsWithForeground, width)

	// Step 6: Visualize first few rows with text
	t.Logf("\nBinary visualization (first 20 rows, first 100 columns):")
	for y := 0; y < 20 && y < len(binary); y++ {
		line := ""
		for x := 0; x < 100 && x < len(binary[y]); x++ {
			if binary[y][x] {
				line += "█"
			} else {
				line += " "
			}
		}
		t.Logf("  %2d: %s", y, line)
	}

	// Step 7: Check middle line (HP/SP boundary)
	midLine := height / 2
	t.Logf("\nMiddle line (HP/SP boundary): %d", midLine)

	// Step 8: Show columns at middle
	t.Logf("Columns with foreground near middle line:")
	for x := 0; x < 50; x++ {
		hasAbove := false
		hasBelow := false
		for y := midLine - 20; y < midLine; y++ {
			if y >= 0 && y < len(binary) && binary[y][x] {
				hasAbove = true
			}
		}
		for y := midLine; y < midLine+20; y++ {
			if y < len(binary) && binary[y][x] {
				hasBelow = true
			}
		}

		if hasAbove || hasBelow {
			t.Logf("  X=%d: above=%v below=%v", x, hasAbove, hasBelow)
		}
	}
}

// TestRawThreshold shows what the threshold value is doing.
func TestRawThreshold(t *testing.T) {
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

	statusROI := CaptureStatusWindowROI(img)
	statusImg := ExtractROI(img, statusROI)
	statusImg = UpscaleImage(statusImg, 4)

	// Check actual pixel values before thresholding
	bounds := statusImg.Bounds()
	t.Logf("Image bounds: (%d,%d) to (%d,%d)", bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y)

	// Sample some pixels
	for y := 10; y < 20; y++ {
		line := ""
		for x := 0; x < 50; x++ {
			r, g, b, _ := statusImg.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			gray := uint8((299*r + 587*g + 114*b) / 1000 >> 8)
			if gray > 200 {
				line += "█"
			} else if gray > 100 {
				line += "▒"
			} else {
				line += " "
			}
		}
		t.Logf("  %2d: %s", y, line)
	}

	// Show gray values
	t.Logf("\nGray values (first 50 pixels of row 12):")
	for x := 0; x < 50; x++ {
		r, g, b, _ := statusImg.At(bounds.Min.X+x, bounds.Min.Y+12).RGBA()
		gray := uint8((299*r + 587*g + 114*b) / 1000 >> 8)
		if x%10 == 0 {
			t.Logf("  X=%d: %d", x, gray)
		}
	}
}
