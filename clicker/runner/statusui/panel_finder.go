package statusui

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"
	"sync"
)

//go:embed assets/StatusPanel.png
var statusPanelTemplatePNG []byte

var (
	statusPanelTemplateOnce sync.Once
	statusPanelTemplate     image.Image
)

// DefaultStatusPanelTemplate returns the embedded StatusPanel.png decoded as
// an image.Image. Loaded once on first call; safe for concurrent use.
//
// The template is embedded into the binary so the recognition pipeline
// doesn't depend on a runtime file path. Callers that want to ship their
// own template can still pass any image.Image to FindStatusPanel directly.
func DefaultStatusPanelTemplate() image.Image {
	statusPanelTemplateOnce.Do(func() {
		if len(statusPanelTemplatePNG) == 0 {
			return
		}
		img, err := png.Decode(bytes.NewReader(statusPanelTemplatePNG))
		if err == nil {
			statusPanelTemplate = img
		}
	})
	return statusPanelTemplate
}

// FindStatusPanelOptions configures the panel search.
type FindStatusPanelOptions struct {
	// TopLeftRegion is the first region to scan, in screen coordinates.
	// Defaults to image.Rect(0, 0, 400, 200) when empty — the
	// typical Ragnarok status window location in the upper-left.
	TopLeftRegion image.Rectangle
	// MaxScore is the maximum acceptable SAD score (0 = perfect,
	// 1 = worst). Defaults to 0.30 when zero. 0.30 sits above the
	// worst top-left-panel SAD observed in the regression set
	// (≈0.25 on captures with content drift) and below
	// false-positive SAD observed on flat unrelated regions
	// (≈0.99) — so VerifyPanel's content signals remain the
	// discriminator.
	MaxScore float64
}

// FindStatusPanel searches for the status panel inside img using
// grayscale sum-of-absolute-differences (SAD) against the template.
//
// The search runs in two passes:
//
//  1. The TopLeftRegion (defaults to image.Rect(0, 0, 400, 200)).
//     The Ragnarok status panel normally lives in this region on
//     real captures; if a match below MaxScore exists here, this
//     returns immediately.
//
//  2. The rest of the image (img.Bounds()). If the first pass did
//     not find anything, this passes continues looking for a
//     panel-shaped match anywhere else on screen. It is the
//     continuation of the search, not a fallback.
//
// Both passes use the same single full-accuracy scan algorithm
// (findPanelInRegion); only the search region differs. This is the
// "same behaviour" — a single uniform algorithm applied to two
// candidate regions in a defined order — instead of two different
// implementations trying to do the same thing.
//
// For *image.RGBA (the format produced by the screen-capture path)
// the hot loop accesses the underlying Pix slice directly rather
// than going through image.At(); for any other format it falls
// back to At().
//
// Returns the best match location, the normalized 0..1 score (0 =
// perfect, 1 = worst), and ok=true iff the best score is at or
// below MaxScore. If no match is found in either region, rect is
// the zero Rectangle and ok=false.
func FindStatusPanel(img, template image.Image, opts FindStatusPanelOptions) (image.Rectangle, float64, bool) {
	region := opts.TopLeftRegion
	if region.Empty() {
		region = image.Rect(0, 0, 400, 200)
	}
	maxScore := opts.MaxScore
	if maxScore == 0 {
		maxScore = 0.30
	}

	// Pass 1: top-left region.
	if rect, score, ok := findPanelInRegion(img, template, region, maxScore); ok {
		return rect, score, true
	}
	// Pass 2: continuation across the rest of the screen.
	return findPanelInRegion(img, template, img.Bounds(), maxScore)
}

// panelBoundaryOverflowPx is the maximum number of pixels the template may
// extend beyond the top or left image boundary during the panel search.
// This allows panels partially clipped at the screen top-left corner to be
// found at the correct position. The value is kept small (20 px) to prevent
// false-positive matches from tiny visible slivers far outside the image.
const panelBoundaryOverflowPx = 20

// panelMinVisibleFraction is the minimum fraction of the template area that
// must be inside the image for a candidate position to be evaluated.
const panelMinVisibleFraction = 0.75

// findPanelInRegion scans the given region of img at full pixel
// accuracy (stride=1) against the template and returns the
// lowest-SAD match below maxScore, or ok=false if no match qualifies.
//
// The search min boundary is extended by panelBoundaryOverflowPx so a panel
// partially clipped at the screen top-left is found at its real position.
// Only the visible intersection is compared; the SAD is normalized by that
// area so a partly-clipped match competes fairly with fully-visible candidates.
func findPanelInRegion(img, template image.Image, region image.Rectangle, maxScore float64) (image.Rectangle, float64, bool) {
	ib := img.Bounds()
	tb := template.Bounds()
	tw, th := tb.Dx(), tb.Dy()
	if tw <= 0 || th <= 0 {
		return image.Rectangle{}, 0, false
	}

	region = region.Intersect(ib)
	// Extend the search start upward by up to panelBoundaryOverflowPx so a
	// panel partially clipped above the screen top is found at its real
	// position. minX is NOT extended: allowing the template to start before
	// the image left edge creates false positives where a small sliver of
	// the left screen content happens to match the template well enough to
	// beat a genuine right-side panel. The typical clipped-panel case
	// (window flush with display top) only needs a top overflow.
	minX := region.Min.X
	minY := region.Min.Y - panelBoundaryOverflowPx
	if minY < region.Min.Y-(th-1) {
		minY = region.Min.Y - (th - 1)
	}
	maxX := region.Max.X - tw
	maxY := region.Max.Y - th
	if maxX < minX || maxY < minY {
		return image.Rectangle{}, 0, false
	}
	// Minimum visible pixels: 75% of the full template. Positions with less
	// visible area are skipped — tiny slivers can't produce reliable scores.
	minVisPx := int(float64(tw*th) * 0.75)

	tplGray := precomputeGrayscale(template, tb)

	imgGray, fastOK := precomputeImageGrayscale(img, ib)

	// sadPartial computes the SAD over only the rows and columns where the
	// template overlaps the image. Normalizes by the pixel count of that
	// visible intersection so scores remain in the same 0..1 range as a
	// full-template match.
	sadPartial := func(x0, y0 int, earlyExit float64) float64 {
		// Visible template rows: rows where img row (y0+dy) is in ib.
		dyStart := ib.Min.Y - y0
		if dyStart < 0 {
			dyStart = 0
		}
		dyEnd := ib.Max.Y - y0
		if dyEnd > th {
			dyEnd = th
		}
		// Visible template cols: cols where img col (x0+dx) is in ib.
		dxStart := ib.Min.X - x0
		if dxStart < 0 {
			dxStart = 0
		}
		dxEnd := ib.Max.X - x0
		if dxEnd > tw {
			dxEnd = tw
		}
		visH := dyEnd - dyStart
		visW := dxEnd - dxStart
		if visH <= 0 || visW <= 0 || visH*visW < minVisPx {
			return earlyExit + 1 // skip: not enough visible pixels
		}
		visPixels := float64(visH * visW)
		// Scale earlyExit down to the visible-area budget so the inner-loop
		// early-exit comparison remains valid.
		scaledExit := earlyExit * (visPixels / float64(tw*th))

		var rawSum float64
		if fastOK {
			rawSum = sadOnGrayscalePartial(imgGray, tplGray, x0, y0, dxStart, dyStart, dxEnd, dyEnd, scaledExit)
		} else {
			rawSum = sadWithEarlyExitPartial(img, tplGray, x0, y0, dxStart, dyStart, dxEnd, dyEnd, scaledExit)
		}
		// Normalize rawSum to full-template scale so maxScore applies uniformly.
		return rawSum / visPixels * float64(tw*th)
	}

	maxPossible := float64(tw*th) * 255.0
	bestScore := maxPossible + 1
	bestX, bestY := minX, minY
scan:
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			score := sadPartial(x, y, bestScore)
			if score < bestScore {
				bestScore = score
				bestX, bestY = x, y
				if bestScore == 0 {
					break scan
				}
			}
		}
	}

	normalized := bestScore / maxPossible
	if normalized > maxScore {
		return image.Rectangle{}, normalized, false
	}
	return image.Rect(bestX, bestY, bestX+tw, bestY+th), normalized, true
}

// precomputeGrayscale returns a 2-D slice of 8-bit luminance values
// for the pixels in the given image bounds.
func precomputeGrayscale(img image.Image, b image.Rectangle) [][]uint8 {
	w, h := b.Dx(), b.Dy()
	out := make([][]uint8, h)
	for y := 0; y < h; y++ {
		row := make([]uint8, w)
		for x := 0; x < w; x++ {
			row[x] = luma8(img.At(b.Min.X+x, b.Min.Y+y))
		}
		out[y] = row
	}
	return out
}

// precomputeImageGrayscale converts an *image.RGBA (the format produced
// by the screen-capture path) to a 2-D slice of 8-bit luminance values
// by reading the Pix slice directly. For other image formats it returns
// ok=false so the caller can fall back to per-pixel At().
//
// Accessing Pix directly is ~10× faster than calling image.At() for
// every pixel, which is the difference between a sub-second full-screen
// scan and a 5-minute timeout.
//
// Note on alpha pre-multiplication: image.At().RGBA() returns
// pre-multiplied 16-bit values, while reading Pix directly gives the
// un-pre-multiplied 8-bit values. For fully-opaque pixels (which is
// true for screen captures from win.CapturePlayerBarSearch) the two
// are identical; if a non-opaque image is ever passed in, the fast
// path and the At() path will disagree.
func precomputeImageGrayscale(img image.Image, b image.Rectangle) ([][]uint8, bool) {
	rgba, ok := img.(*image.RGBA)
	if !ok {
		return nil, false
	}
	w, h := b.Dx(), b.Dy()
	out := make([][]uint8, h)
	stride := rgba.Stride
	pix := rgba.Pix
	for y := 0; y < h; y++ {
		row := make([]uint8, w)
		srcY := b.Min.Y + y
		for x := 0; x < w; x++ {
			srcX := b.Min.X + x
			off := srcY*stride + srcX*4
			row[x] = luma8FromRGBA(pix[off], pix[off+1], pix[off+2])
		}
		out[y] = row
	}
	return out, true
}

// sadOnGrayscalePartial computes the SAD over only the visible sub-rectangle
// [dxStart,dxEnd) × [dyStart,dyEnd) of the template against the grayscale image.
func sadOnGrayscalePartial(imgGray, tpl [][]uint8, x0, y0, dxStart, dyStart, dxEnd, dyEnd int, earlyExit float64) float64 {
	sum := 0.0
	for dy := dyStart; dy < dyEnd; dy++ {
		irow := imgGray[y0+dy]
		trow := tpl[dy]
		for dx := dxStart; dx < dxEnd; dx++ {
			diff := float64(trow[dx]) - float64(irow[x0+dx])
			if diff < 0 {
				diff = -diff
			}
			sum += diff
			if sum >= earlyExit {
				return sum
			}
		}
	}
	return sum
}

// sadWithEarlyExitPartial is the At()-based fallback for non-RGBA images,
// matching only the visible sub-rectangle of the template.
func sadWithEarlyExitPartial(img image.Image, tpl [][]uint8, x0, y0, dxStart, dyStart, dxEnd, dyEnd int, earlyExit float64) float64 {
	sum := 0.0
	for dy := dyStart; dy < dyEnd; dy++ {
		row := tpl[dy]
		yy := y0 + dy
		for dx := dxStart; dx < dxEnd; dx++ {
			diff := float64(row[dx]) - float64(luma8(img.At(x0+dx, yy)))
			if diff < 0 {
				diff = -diff
			}
			sum += diff
			if sum >= earlyExit {
				return sum
			}
		}
	}
	return sum
}

// luma8 returns an 8-bit grayscale value (Rec.601-style weights) for
// any color. This is the canonical formula used across statusui
// (template load, runtime capture, and the slow path all go through
// here). Matches the formula in statusui's PreprocessImage so every
// grayscale conversion in the package agrees to ±1.
func luma8(c color.Color) uint8 {
	r, g, b, _ := c.RGBA()
	return uint8((r*299 + g*587 + b*114) / 1000 >> 8)
}

// luma8FromRGBA computes the same 8-bit grayscale for raw 8-bit RGB
// inputs read directly from an *image.RGBA Pix slice. Used by the
// fast path (precomputeImageGrayscale) to avoid the At()→RGBA() round
// trip. Assumes fully-opaque pixels — for non-opaque inputs the At()
// path (which goes through pre-multiplied 16-bit RGBA()) will differ
// from this by the alpha factor, so call the color.Color version
// instead.
func luma8FromRGBA(r, g, b uint8) uint8 {
	return uint8((uint32(r)*299 + uint32(g)*587 + uint32(b)*114) / 1000)
}
