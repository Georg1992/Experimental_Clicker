package runner

import (
	"image"
)

// ConnectedComponent represents a group of connected foreground pixels.
type ConnectedComponent struct {
	Pixels []image.Point
	Bounds image.Rectangle
}

// FindConnectedComponents finds all connected components of foreground pixels.
// Returns list of components sorted by X position of leftmost pixel.
func FindConnectedComponents(binary [][]bool, scanROI image.Rectangle) []ConnectedComponent {
	if len(binary) == 0 {
		return nil
	}

	height := len(binary)
	width := len(binary[0])
	visited := make([][]bool, height)
	for i := range visited {
		visited[i] = make([]bool, width)
	}

	var components []ConnectedComponent

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Skip if out of scan ROI
			if x < scanROI.Min.X || x >= scanROI.Max.X || y < scanROI.Min.Y || y >= scanROI.Max.Y {
				continue
			}

			if binary[y][x] && !visited[y][x] {
				// Start a new component
				comp := floodFill(binary, visited, x, y, scanROI)
				if len(comp.Pixels) > 2 { // Only accept components with >2 pixels
					components = append(components, comp)
				}
			}
		}
	}

	return components
}

// floodFill performs 4-connected flood fill to find all pixels in a component.
func floodFill(binary [][]bool, visited [][]bool, startX, startY int, roi image.Rectangle) ConnectedComponent {
	var stack []image.Point
	var pixels []image.Point
	minX, maxX := startX, startX
	minY, maxY := startY, startY

	stack = append(stack, image.Pt(startX, startY))
	visited[startY][startX] = true

	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		pixels = append(pixels, p)
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}

		// Check 4 neighbors
		neighbors := []image.Point{
			{p.X + 1, p.Y},
			{p.X - 1, p.Y},
			{p.X, p.Y + 1},
			{p.X, p.Y - 1},
		}

		for _, n := range neighbors {
			if n.X >= roi.Min.X && n.X < roi.Max.X && n.Y >= roi.Min.Y && n.Y < roi.Max.Y {
				if n.X < len(binary[0]) && n.Y < len(binary) && binary[n.Y][n.X] && !visited[n.Y][n.X] {
					visited[n.Y][n.X] = true
					stack = append(stack, n)
				}
			}
		}
	}

	return ConnectedComponent{
		Pixels: pixels,
		Bounds: image.Rect(minX, minY, maxX+1, maxY+1),
	}
}

// BoundingBoxesToGlyphs converts connected components to glyph ROIs.
// Merges components that are very close (within threshold pixels).
func BoundingBoxesToGlyphs(components []ConnectedComponent, mergeThreshold int) []image.Rectangle {
	if len(components) == 0 {
		return nil
	}

	// Sort by left edge (X coordinate)
	sorted := make([]ConnectedComponent, len(components))
	copy(sorted, components)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Bounds.Min.X < sorted[i].Bounds.Min.X {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Merge components that are close together
	var glyphs []image.Rectangle
	current := sorted[0].Bounds

	for i := 1; i < len(sorted); i++ {
		gap := sorted[i].Bounds.Min.X - current.Max.X
		if gap <= mergeThreshold {
			// Merge with current
			current.Max.X = sorted[i].Bounds.Max.X
			if sorted[i].Bounds.Min.Y < current.Min.Y {
				current.Min.Y = sorted[i].Bounds.Min.Y
			}
			if sorted[i].Bounds.Max.Y > current.Max.Y {
				current.Max.Y = sorted[i].Bounds.Max.Y
			}
		} else {
			// Save current and start new
			glyphs = append(glyphs, current)
			current = sorted[i].Bounds
		}
	}
	glyphs = append(glyphs, current)

	return glyphs
}
