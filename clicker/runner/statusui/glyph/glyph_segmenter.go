package glyph

import "image"

// SegmentGlyphs finds connected components in a binary image and converts them to glyph bounding boxes.
// This is the ONLY module responsible for glyph segmentation.
func SegmentGlyphs(binary [][]bool, roi image.Rectangle, mergeThreshold int) []image.Rectangle {
	// Step 1: Find connected components
	components := FindConnectedComponents(binary, roi)

	// Step 2: Convert to glyph bounding boxes with merging
	return BoundingBoxesToGlyphs(components, mergeThreshold)
}

// FindConnectedComponents detects connected foreground pixel clusters within ROI (8-connectivity).
// Returns the bounding box (image.Rectangle) of each cluster.
func FindConnectedComponents(binary [][]bool, roi image.Rectangle) []image.Rectangle {
	height := len(binary)
	if height == 0 {
		return nil
	}
	width := len(binary[0])
	roi = roi.Intersect(image.Rect(0, 0, width, height))
	if roi.Empty() {
		return nil
	}

	visited := make([][]bool, height)
	for y := range visited {
		visited[y] = make([]bool, width)
	}

	var boxes []image.Rectangle
	dirs := [8][2]int{{-1, -1}, {-1, 0}, {-1, 1}, {0, -1}, {0, 1}, {1, -1}, {1, 0}, {1, 1}}

	for y := roi.Min.Y; y < roi.Max.Y; y++ {
		for x := roi.Min.X; x < roi.Max.X; x++ {
			if !binary[y][x] || visited[y][x] {
				continue
			}
			minX, maxX := x, x
			minY, maxY := y, y
			stack := [][2]int{{x, y}}
			visited[y][x] = true
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				px, py := p[0], p[1]
				for _, d := range dirs {
					nx, ny := px+d[0], py+d[1]
					if nx >= roi.Min.X && nx < roi.Max.X && ny >= roi.Min.Y && ny < roi.Max.Y &&
						!visited[ny][nx] && binary[ny][nx] {
						visited[ny][nx] = true
						stack = append(stack, [2]int{nx, ny})
						if nx < minX {
							minX = nx
						}
						if nx > maxX {
							maxX = nx
						}
						if ny < minY {
							minY = ny
						}
						if ny > maxY {
							maxY = ny
						}
					}
				}
			}
			boxes = append(boxes, image.Rect(minX, minY, maxX+1, maxY+1))
		}
	}
	return boxes
}

// BoundingBoxesToGlyphs merges nearby bounding boxes (within threshold pixels).
func BoundingBoxesToGlyphs(boxes []image.Rectangle, mergeThreshold int) []image.Rectangle {
	if len(boxes) == 0 {
		return nil
	}

	merged := make([]image.Rectangle, 0, len(boxes))
	used := make([]bool, len(boxes))

	for i := 0; i < len(boxes); i++ {
		if used[i] {
			continue
		}
		curr := boxes[i]
		for j := i + 1; j < len(boxes); j++ {
			if used[j] {
				continue
			}
			b := boxes[j]
			if abs(curr.Min.X-b.Max.X) <= mergeThreshold ||
				abs(curr.Max.X-b.Min.X) <= mergeThreshold {
				curr = curr.Union(b)
				used[j] = true
			}
		}
		merged = append(merged, curr)
	}

	return merged
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ExtractBinaryROI extracts a binary region from a larger binary image.
// Used by glyph recognition to isolate individual glyphs.
func ExtractBinaryROI(binary [][]bool, roi image.Rectangle) [][]bool {
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
