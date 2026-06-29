package autopot

import (
	"errors"
	"image"
	"image/color"
	"time"
)

const (
	// 55px half-width covers ±50px horizontal camera drift while keeping height tight.
	mapROIHalfW = 55
	mapROIHalfH = 30
	// Player bars sit below the screen center; bias ROI downward without enlarging it.
	mapROICenterYOffset = 15

	barRowHeight   = 3
	expectedBarGap = 4
	maxBarPairGap  = 12
	minRunWidth    = 2
	runGapMerge    = 2

	minBarWidth = 20
	maxBarWidth = 120

	minPairOverlap = 4
	minPairRunW    = 3
	barExtentGap   = 2

	barBgR, barBgG, barBgB = 10, 10, 14
	barBgTol               = 28

	hpGreenR, hpGreenG, hpGreenB = 16, 238, 33
	hpRedR, hpRedG, hpRedB       = 255, 13, 0
	spBlueR, spBlueG, spBlueB    = 25, 101, 225
	fillTol                      = 50

	// BarPairStableDrift is max HP/SP rect movement between two same-frame pair refreshes.
	BarPairStableDrift = 2
	// BarPositionMaxDrift is max rect movement allowed between timed pot confirmations.
	BarPositionMaxDrift = 8

	// HP fill detection thresholds
	hpFillMinGreen     = 35
	hpFillMinRed       = 50
	hpFillRedGreenDiff = 25

	// Bar alignment tolerance
	barXAlignmentTolerance = 6

	// Bar pair scoring weights
	centerDistXWeight = 3
	centerDistYWeight = 4
	gapPenaltyWeight  = 8
	barRunDistYWeight = 2
	barRunWidthWeight = 3

	// Bar run scoring
	barRunDistXWeight = 3

	// Bar run finding tolerance for anchoring
	barExtensionSearchLimit = 12

	// Color detection tolerances for isHPTrack
	hpTrackBgTol   = 8
	hpTrackSumMin  = 60
	hpTrackSumMax  = 210
	hpTrackDiffTol = 30

	// Color detection thresholds for bar background
	barBgNearBlackTol = 15
	barBgDarkSum      = 35

	// HP green detection thresholds
	hpGreenMinGreen    = 60
	hpGreenMaxRed      = 20
	hpGreenBlueGapTol  = 8
	hpGreenRedGapMin   = 10
	hpGreenAltMinGreen = 50
	hpGreenAltMinDiff  = 10
	hpGreenBrightMin   = 80

	// HP red detection thresholds
	hpRedMinRed    = 130
	hpRedGreenDiff = 25

	// HP yellow detection thresholds
	hpYellowMinRed        = 110
	hpYellowMinGreen      = 90
	hpYellowMaxBlue       = 90
	hpYellowRedBlueDiff   = 15
	hpYellowGreenBlueDiff = 10

	// SP blue detection thresholds
	spBlueMinBlue   = 90
	spBlueGreenDiff = 10
	spBlueRedDiff   = 18

	// SP fill detection thresholds
	spFillMinBlue  = 130
	spFillMinRed   = 12
	spFillBlueDiff = 20

	// SP cyan detection thresholds
	spCyanMinBlue   = 80
	spCyanMinGreen  = 60
	spCyanBlueDiff  = 10
	spCyanGreenDiff = 5

	// Debug visualization cross size
	debugCrossSize = 6
)

var ErrBarsNotFound = errors.New("player bars not found")

type Rect struct {
	X, Y, W, H int
}

type ColorRun struct {
	X1    int
	X2    int
	Y     int
	Width int
	Color string // "hp" or "sp"
}

type MappedBars struct {
	Block      Rect
	HP         Rect
	SP         Rect
	Valid      bool
	MapScore   int
	LastMapped time.Time
}

type BarRead struct {
	Percent     float64
	FilledWidth int
	FullWidth   int
	Found       bool
}

// Bar is used by debug visualization output.
type Bar struct {
	Left, Right int
	Y           int
	Width       int
	Height      int
	FilledWidth int
	Percent     float64
	Found       bool
}

type BarROI struct {
	X, Y, W, H int
}

// RefreshBarPair locates the player HP/SP colored-run pair near screen center.
// This is a periodic pair refresh, not a full-screen rectangle search.
func RefreshBarPair(img image.Image) (MappedBars, error) {
	b := img.Bounds()
	roi := driftROI(b)
	roiCX := roi.X + roi.W/2
	roiCY := roi.Y + roi.H/2

	hpRuns := consolidateRuns(scanColorRuns(img, roi, IsHPPixel, "hp"))
	spRuns := consolidateRuns(scanColorRuns(img, roi, IsSPPixel, "sp"))

	hpRun, spRun, score, ok := findPlayerBarPair(hpRuns, spRuns, roi, roiCX, roiCY)
	if !ok {
		return MappedBars{}, ErrBarsNotFound
	}

	hpRect, spRect := deriveBarRects(img, hpRun, spRun)
	block := unionRect(hpRect, spRect)

	return MappedBars{
		Block:      block,
		HP:         hpRect,
		SP:         spRect,
		Valid:      true,
		MapScore:   score,
		LastMapped: time.Now(),
	}, nil
}

func driftROI(bounds image.Rectangle) Rect {
	cx := bounds.Min.X + bounds.Dx()/2
	cy := bounds.Min.Y + bounds.Dy()/2 + mapROICenterYOffset
	return clampROI(bounds, Rect{
		X: cx - mapROIHalfW,
		Y: cy - mapROIHalfH,
		W: mapROIHalfW * 2,
		H: mapROIHalfH * 2,
	})
}

// PlayerBarSearchROI returns the region where the application should search for player HP/SP bars.
// This accounts for screen size and expected bar position below center.
func PlayerBarSearchROI(screenW, screenH int) BarROI {
	cx := screenW / 2
	cy := screenH/2 + mapROICenterYOffset
	return BarROI{
		X: cx - mapROIHalfW,
		Y: cy - mapROIHalfH,
		W: mapROIHalfW * 2,
		H: mapROIHalfH * 2,
	}
}

// ReadMappedBars reads the fill percentage of HP and SP bars from the given image.
// Uses cached bar rectangles from the last successful pair detection.
func ReadMappedBars(img image.Image, bars MappedBars) (hp BarRead, sp BarRead) {
	if !bars.Valid {
		return BarRead{}, BarRead{}
	}
	if bars.HP.W < 1 || bars.SP.W < 1 {
		return BarRead{FullWidth: bars.HP.W}, BarRead{FullWidth: bars.SP.W}
	}
	hp = ReadHPFill(img, bars.HP)
	sp = ReadSPFill(img, bars.SP)
	return hp, sp
}

func normalizeBarRead(img image.Image, r Rect, hpBar bool, read BarRead) BarRead {
	if !read.Found || r.W < 2 {
		return read
	}
	if !barLooksFull(img, r, hpBar) {
		return read
	}
	if read.FullWidth < 1 {
		read.FullWidth = r.W
	}
	read.FilledWidth = read.FullWidth
	read.Percent = 100
	read.Found = true
	return read
}

// ReadHPFill reads the fill percentage of an HP bar from the image.
// Returns a BarRead with the fill percentage and pixel counts.
func ReadHPFill(img image.Image, hp Rect) BarRead {
	if hp.W < 1 || hp.H < 1 {
		return BarRead{Found: false}
	}
	hp = trimBarEdges(img, hp, true)
	if hp.W < 1 {
		return BarRead{Found: false}
	}
	best := BarRead{Found: true, FullWidth: hp.W}
	for row := 0; row < hp.H; row++ {
		br := readBarFillSingleRow(img, hp.X, hp.Y+row, hp.W, isHPFillRead)
		if br.FilledWidth > best.FilledWidth {
			best = br
		}
	}
	if best.FilledWidth == 0 {
		return normalizeBarRead(img, hp, true, readBarFillSingleRow(img, hp.X, hp.Y, hp.W, isHPFillRead))
	}
	return normalizeBarRead(img, hp, true, best)
}

func isHPFillRead(r, g, b uint8) bool {
	if IsHPPixel(r, g, b) {
		return true
	}
	if isHPTrack(r, g, b) {
		return false
	}
	ri, gi, _ := int(r), int(g), int(b)
	return gi >= hpFillMinGreen && ri >= hpFillMinRed && absInt(ri-gi) < hpFillRedGreenDiff
}

// ReadSPFill reads the fill percentage of an SP bar from the image.
// Similar to ReadHPFill but uses SP color detection.
func ReadSPFill(img image.Image, sp Rect) BarRead {
	if sp.W < 1 || sp.H < 1 {
		return BarRead{Found: false}
	}
	sp = trimBarEdges(img, sp, false)
	if sp.W < 1 {
		return BarRead{Found: false}
	}
	if sp.H >= 3 {
		return normalizeBarRead(img, sp, false, readBarFillSingleRow(img, sp.X, sp.Y+1, sp.W, isSPFill))
	}
	best := BarRead{Found: true, FullWidth: sp.W}
	for row := 0; row < sp.H; row++ {
		br := readBarFillSingleRow(img, sp.X, sp.Y+row, sp.W, isSPFill)
		if br.FilledWidth > best.FilledWidth {
			best = br
		}
	}
	return normalizeBarRead(img, sp, false, best)
}

func trimBarEdges(img image.Image, r Rect, hpBar bool) Rect {
	y := r.Y + r.H/2
	for r.W > 0 {
		rp, gp, bp := pixelAt(img, r.X, y)
		if barEdgePixel(rp, gp, bp, hpBar) {
			break
		}
		r.X++
		r.W--
	}
	for r.W > 0 {
		rp, gp, bp := pixelAt(img, r.X+r.W-1, y)
		if barEdgePixel(rp, gp, bp, hpBar) {
			break
		}
		r.W--
	}
	return r
}

func barEdgePixel(r, g, b uint8, hpBar bool) bool {
	if hpBar {
		return IsHPPixel(r, g, b) || isHPTrack(r, g, b)
	}
	return IsSPPixel(r, g, b) || isSPFill(r, g, b) || isHPTrack(r, g, b)
}

func barsMisaligned(a, b MappedBars) bool {
	if !a.Valid || !b.Valid {
		return true
	}
	return rectDrifted(a.HP, b.HP, BarPairStableDrift) ||
		rectDrifted(a.SP, b.SP, BarPairStableDrift)
}

func rectDrifted(a, b Rect, max int) bool {
	if a.W < 1 || b.W < 1 {
		return true
	}
	return absInt(a.X-b.X) > max ||
		absInt(a.Y-b.Y) > max ||
		absInt(a.W-b.W) > max
}

func refreshStableBarPair(img image.Image) (MappedBars, bool) {
	first, err := RefreshBarPair(img)
	if err != nil {
		return MappedBars{}, false
	}
	second, err := RefreshBarPair(img)
	if err != nil {
		return MappedBars{}, false
	}
	if barsMisaligned(first, second) {
		return MappedBars{}, false
	}
	return second, true
}

func isBarEmptyPixel(r, g, b uint8, hpBar bool) bool {
	if hpBar {
		if IsHPPixel(r, g, b) || isHPFillRead(r, g, b) {
			return false
		}
	} else if IsSPPixel(r, g, b) || isSPFill(r, g, b) {
		return false
	}
	return isHPTrack(r, g, b) || isBarBackground(r, g, b)
}

func barRightHasEmptyTrack(img image.Image, r Rect, hpBar bool) bool {
	if r.W < 4 || r.H < 1 {
		return false
	}
	checkCols := r.W / 5
	if checkCols < 3 {
		checkCols = 3
	}
	if checkCols > 8 {
		checkCols = 8
	}
	for row := 0; row < r.H; row++ {
		y := r.Y + row
		empty := 0
		for col := r.W - checkCols; col < r.W; col++ {
			rp, gp, bp := pixelAt(img, r.X+col, y)
			if isBarEmptyPixel(rp, gp, bp, hpBar) {
				empty++
			}
		}
		if empty >= 2 {
			return true
		}
	}
	return false
}

func barLooksFull(img image.Image, r Rect, hpBar bool) bool {
	if r.W < 2 {
		return false
	}
	if barRightHasEmptyTrack(img, r, hpBar) {
		return false
	}
	return bestFillWidth(img, r, hpBar) >= r.W-2
}

func barConfirmedNotFull(img image.Image, r Rect, hpBar bool, read BarRead) bool {
	if !read.Found || !barReadConsistent(img, r, hpBar, read) {
		return false
	}
	if barLooksFull(img, r, hpBar) || read.Percent >= 99 {
		return false
	}
	return barRightHasEmptyTrack(img, r, hpBar)
}

func barReadConsistent(img image.Image, r Rect, hpBar bool, read BarRead) bool {
	if !read.Found || r.W < 2 {
		return false
	}
	if barLooksFull(img, r, hpBar) {
		return true
	}
	fillW := bestFillWidth(img, r, hpBar)
	if fillW == 0 {
		return read.FilledWidth == 0
	}
	if read.FilledWidth == 0 {
		return false
	}
	if read.FilledWidth < fillW-2 {
		return false
	}
	return read.FilledWidth <= fillW+1
}

func bestFillWidth(img image.Image, r Rect, hpBar bool) int {
	if !hpBar && r.H >= 3 {
		return readBarFillSingleRow(img, r.X, r.Y+1, r.W, isSPFill).FilledWidth
	}
	isPixel := isHPFillRead
	if !hpBar {
		isPixel = isSPFill
	}
	best := 0
	for row := 0; row < r.H; row++ {
		br := readBarFillSingleRow(img, r.X, r.Y+row, r.W, isPixel)
		if br.FilledWidth > best {
			best = br.FilledWidth
		}
	}
	return best
}

func clampROI(bounds image.Rectangle, roi Rect) Rect {
	maxX := bounds.Max.X
	maxY := bounds.Max.Y
	if roi.X < bounds.Min.X {
		roi.W -= bounds.Min.X - roi.X
		roi.X = bounds.Min.X
	}
	if roi.Y < bounds.Min.Y {
		roi.H -= bounds.Min.Y - roi.Y
		roi.Y = bounds.Min.Y
	}
	if roi.X+roi.W > maxX {
		roi.W = maxX - roi.X
	}
	if roi.Y+roi.H > maxY {
		roi.H = maxY - roi.Y
	}
	if roi.W < 1 {
		roi.W = 1
	}
	if roi.H < 1 {
		roi.H = 1
	}
	return roi
}

// scanColorRuns finds horizontal runs of matching pixels in the ROI.
// A "run" is a contiguous horizontal sequence of pixels matching the provided color test.
// Returns all runs >= minRunWidth, with small gaps (<=runGapMerge pixels) automatically merged.
func scanColorRuns(img image.Image, roi Rect, isPixel func(r, g, b uint8) bool, colorKind string) []ColorRun {
	var runs []ColorRun
	for y := roi.Y; y < roi.Y+roi.H; y++ {
		runs = append(runs, extractRowRuns(img, y, roi.X, roi.X+roi.W, isPixel, colorKind)...)
	}
	return runs
}

func extractRowRuns(img image.Image, y, x0, x1 int, isPixel func(r, g, b uint8) bool, colorKind string) []ColorRun {
	var runs []ColorRun
	runStart := -1
	runEnd := -1
	gap := 0

	flush := func() {
		if runStart < 0 {
			return
		}
		w := runEnd - runStart + 1
		if w >= minRunWidth {
			runs = append(runs, ColorRun{
				X1: runStart, X2: runEnd, Y: y, Width: w, Color: colorKind,
			})
		}
		runStart = -1
		runEnd = -1
		gap = 0
	}

	for x := x0; x < x1; x++ {
		rp, gp, bp := pixelAt(img, x, y)
		if isPixel(rp, gp, bp) {
			if runStart < 0 {
				runStart = x
			}
			runEnd = x
			gap = 0
			continue
		}
		if runStart >= 0 {
			gap++
			if gap > runGapMerge {
				flush()
			}
		}
	}
	flush()
	return runs
}

func consolidateRuns(runs []ColorRun) []ColorRun {
	if len(runs) == 0 {
		return nil
	}
	bestByY := map[int]ColorRun{}
	for _, r := range runs {
		prev, ok := bestByY[r.Y]
		if !ok || r.Width > prev.Width {
			bestByY[r.Y] = r
		}
	}
	out := make([]ColorRun, 0, len(bestByY))
	for _, r := range bestByY {
		if r.Width >= minPairRunW {
			out = append(out, r)
		}
	}
	return out
}

// findPlayerBarPair finds the best HP/SP bar pair from detected color runs.
// First tries to find pairs that satisfy all geometric constraints (gap, overlap, alignment).
// If no valid pair is found, anchors from the nearest single run.
// Returns empty runs if no bars are detected at all.
func findPlayerBarPair(hpRuns, spRuns []ColorRun, roi Rect, cx, cy int) (hp, sp ColorRun, score int, ok bool) {
	bestScore := -1
	var bestHP, bestSP ColorRun
	hasPair := false

	for _, spRun := range spRuns {
		for _, hpRun := range hpRuns {
			if hpRun.Color != "hp" || spRun.Color != "sp" {
				continue
			}
			gap := spRun.Y - hpRun.Y
			if gap < 1 || gap > maxBarPairGap {
				continue
			}
			if runOverlap(hpRun, spRun) < minPairOverlap {
				continue
			}
			if absInt(hpRun.X1-spRun.X1) > barXAlignmentTolerance {
				continue
			}
			if !pairCenterInROI(hpRun, spRun, roi) {
				continue
			}
			pairScore := scoreBarPair(hpRun, spRun, cx, cy)
			if pairScore > bestScore {
				bestScore = pairScore
				bestHP = hpRun
				bestSP = spRun
				hasPair = true
			}
		}
	}

	if hasPair {
		return bestHP, bestSP, bestScore, true
	}

	if len(spRuns) > 0 {
		spRun := nearestBarRun(spRuns, roi, cx, cy)
		hpRun := anchorHPFromSP(spRun)
		return hpRun, spRun, scoreBarPair(hpRun, spRun, cx, cy), true
	}
	if len(hpRuns) > 0 {
		hpRun := nearestBarRun(hpRuns, roi, cx, cy)
		spRun := anchorSPFromHP(hpRun)
		return hpRun, spRun, scoreBarPair(hpRun, spRun, cx, cy), true
	}
	return ColorRun{}, ColorRun{}, 0, false
}

func pairCenterInROI(hp, sp ColorRun, roi Rect) bool {
	midX := (hp.X1 + hp.X2 + sp.X1 + sp.X2) / 4
	midY := (hp.Y + sp.Y) / 2
	return midX >= roi.X && midX < roi.X+roi.W &&
		midY >= roi.Y && midY < roi.Y+roi.H
}

func pairCenter(hp, sp ColorRun) (int, int) {
	midX := (hp.X1 + hp.X2 + sp.X1 + sp.X2) / 4
	midY := (hp.Y + sp.Y) / 2
	return midX, midY
}

func runOverlap(a, b ColorRun) int {
	lo := a.X1
	if b.X1 > lo {
		lo = b.X1
	}
	hi := a.X2
	if b.X2 < hi {
		hi = b.X2
	}
	if hi < lo {
		return 0
	}
	return hi - lo + 1
}

// scoreBarPair computes a quality score for an HP/SP pair.
// Favors pairs centered near (cx,cy), with correct bar gap, proper alignment, and good width.
// Higher scores indicate better quality matches.
// Penalties: distance from center, gap deviation, horizontal misalignment, bars above center.
// Bonus: bar width (especially wider SP bars).
func scoreBarPair(hp, sp ColorRun, cx, cy int) int {
	midX := (hp.X1 + hp.X2 + sp.X1 + sp.X2) / 4
	midY := (hp.Y + sp.Y) / 2
	centerDist := absInt(midX-cx)*centerDistXWeight + absInt(midY-cy)*centerDistYWeight

	abovePenalty := 0
	if midY < cy {
		abovePenalty = (cy - midY) * centerDistYWeight
	}

	gapPenalty := absInt((sp.Y-hp.Y)-expectedBarGap) * gapPenaltyWeight
	leftPenalty := absInt(hp.X1-sp.X1) * centerDistXWeight

	qualityBonus := (hp.Width + sp.Width) * barRunWidthWeight
	if sp.Width >= 40 {
		qualityBonus += sp.Width * 2 // Extra bonus for wider SP bar
	}

	return 1000 - centerDist - gapPenalty - leftPenalty - abovePenalty + qualityBonus
}

func nearestBarRun(runs []ColorRun, roi Rect, cx, cy int) ColorRun {
	best := runs[0]
	bestScore := -1
	for _, r := range runs {
		if !runCenterInROI(r, roi) {
			continue
		}
		s := scoreBarRun(r, cx, cy)
		if s > bestScore {
			bestScore = s
			best = r
		}
	}
	if bestScore >= 0 {
		return best
	}
	return nearestRunToCenter(runs, cx, cy)
}

func runCenterInROI(r ColorRun, roi Rect) bool {
	mx := (r.X1 + r.X2) / 2
	return mx >= roi.X && mx < roi.X+roi.W &&
		r.Y >= roi.Y && r.Y < roi.Y+roi.H
}

func scoreBarRun(r ColorRun, cx, cy int) int {
	mx := (r.X1 + r.X2) / 2
	return r.Width*barRunWidthWeight - absInt(mx-cx)*barRunDistXWeight - absInt(r.Y-cy)*barRunDistYWeight
}

func nearestRunToCenter(runs []ColorRun, cx, cy int) ColorRun {
	best := runs[0]
	bestDist := runCenterDist(best, cx, cy)
	for _, r := range runs[1:] {
		d := runCenterDist(r, cx, cy)
		if d < bestDist {
			bestDist = d
			best = r
		}
	}
	return best
}

// runCenterDist returns the Manhattan distance from run center to point (cx, cy)
func runCenterDist(r ColorRun, cx, cy int) int {
	mx := (r.X1 + r.X2) / 2
	return absInt(mx-cx) + absInt(r.Y-cy)
}

func anchorHPFromSP(sp ColorRun) ColorRun {
	y := sp.Y - expectedBarGap
	if y < 0 {
		y = 0
	}
	return ColorRun{X1: sp.X1, X2: sp.X2, Y: y, Width: sp.Width, Color: "hp"}
}

func anchorSPFromHP(hp ColorRun) ColorRun {
	return ColorRun{X1: hp.X1, X2: hp.X2, Y: hp.Y + expectedBarGap, Width: hp.Width, Color: "sp"}
}

func deriveBarRects(img image.Image, hpRun, spRun ColorRun) (hp, sp Rect) {
	roi := driftROI(img.Bounds())

	left := hpRun.X1
	if spRun.X1 < left {
		left = spRun.X1
	}
	if left <= roi.X+2 {
		leftHP := extendHPBarLeft(img, hpRun.Y, left)
		leftSP := extendSPBarLeft(img, spRun.Y, left)
		minLeft := left - barExtensionSearchLimit
		if leftHP < minLeft {
			leftHP = minLeft
		}
		if leftSP < minLeft {
			leftSP = minLeft
		}
		if leftHP < left {
			left = leftHP
		}
		if leftSP < left {
			left = leftSP
		}
	}

	right := hpRun.X2
	if spRun.X2 > right {
		right = spRun.X2
	}
	rightHP := extendHPBarRight(img, hpRun.Y, hpRun.X2)
	rightSP := extendSPBarRight(img, spRun.Y, spRun.X2)
	right = rightHP
	if rightSP > right {
		right = rightSP
	}
	coloredRight := hpRun.X2
	if spRun.X2 > coloredRight {
		coloredRight = spRun.X2
	}
	if hpRun.Width >= 50 && spRun.Width >= 45 {
		right = coloredRight
	} else {
		maxRight := coloredRight + 30
		if right > maxRight {
			right = maxRight
		}
	}

	w := right - left + 1
	if w < minBarWidth {
		w = hpRun.Width
		if spRun.Width > w {
			w = spRun.Width
		}
		right = left + w - 1
	}
	if w > maxBarWidth {
		right = left + maxBarWidth - 1
		w = maxBarWidth
	}

	hpY := hpRun.Y - 1
	if hpY < 0 {
		hpY = hpRun.Y
	}
	spY := hpRun.Y + expectedBarGap - 1

	hp = Rect{X: left, Y: hpY, W: w, H: barRowHeight}
	sp = Rect{X: left, Y: spY, W: w, H: barRowHeight}
	return hp, sp
}

func extendHPBarRight(img image.Image, y, fromX int) int {
	right := fromX
	b := img.Bounds()
	gap := 0
	for x := fromX + 1; x < b.Max.X; x++ {
		rp, gp, bp := pixelAt(img, x, y)
		if IsHPPixel(rp, gp, bp) || isHPTrack(rp, gp, bp) {
			right = x
			gap = 0
			continue
		}
		gap++
		if gap >= barExtentGap {
			break
		}
	}
	return right
}

func extendSPBarRight(img image.Image, y, fromX int) int {
	right := fromX
	b := img.Bounds()
	gap := 0
	for x := fromX + 1; x < b.Max.X; x++ {
		rp, gp, bp := pixelAt(img, x, y)
		if IsSPPixel(rp, gp, bp) {
			right = x
			gap = 0
			continue
		}
		gap++
		if gap >= barExtentGap {
			break
		}
	}
	return right
}

func extendHPBarLeft(img image.Image, y, fromX int) int {
	left := fromX
	b := img.Bounds()
	gap := 0
	for x := fromX - 1; x >= b.Min.X; x-- {
		rp, gp, bp := pixelAt(img, x, y)
		if IsHPPixel(rp, gp, bp) || isHPTrack(rp, gp, bp) {
			left = x
			gap = 0
			continue
		}
		gap++
		if gap >= barExtentGap {
			break
		}
	}
	return left
}

func extendSPBarLeft(img image.Image, y, fromX int) int {
	left := fromX
	b := img.Bounds()
	gap := 0
	for x := fromX - 1; x >= b.Min.X; x-- {
		rp, gp, bp := pixelAt(img, x, y)
		if IsSPPixel(rp, gp, bp) {
			left = x
			gap = 0
			continue
		}
		gap++
		if gap >= barExtentGap {
			break
		}
	}
	return left
}

func readBarFillSingleRow(img image.Image, x0, y, w int, isPixel func(r, g, b uint8) bool) BarRead {
	filled := 0
	for col := 0; col < w; col++ {
		rp, gp, bp := pixelAt(img, x0+col, y)
		if isPixel(rp, gp, bp) {
			filled++
			continue
		}
		if filled > 0 {
			break
		}
	}
	return barReadFromFill(filled, w)
}

func barReadFromFill(filled, full int) BarRead {
	pct := float64(filled) * 100 / float64(full)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return BarRead{
		Percent:     pct,
		FilledWidth: filled,
		FullWidth:   full,
		Found:       true,
	}
}

// IsHPPixel returns true if the pixel color is part of the HP bar (green, red, or yellow).
func IsHPPixel(r, g, b uint8) bool {
	return isHPGreen(r, g, b) || isHPRed(r, g, b) || isHPYellow(r, g, b)
}

// IsSPPixel returns true if the pixel color is part of the SP bar (blue or cyan).
func IsSPPixel(r, g, b uint8) bool {
	return isSPBlue(r, g, b) || isSPCyan(r, g, b)
}

func isHPTrack(r, g, b uint8) bool {
	if IsHPPixel(r, g, b) || IsSPPixel(r, g, b) {
		return false
	}
	if colorNear(r, g, b, barBgR, barBgG, barBgB, hpTrackBgTol) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	sum := ri + gi + bi
	if sum < hpTrackSumMin || sum > hpTrackSumMax {
		return false
	}
	return bi <= gi && absInt(ri-gi) < hpTrackDiffTol
}

func isBarBackground(r, g, b uint8) bool {
	if colorNear(r, g, b, barBgR, barBgG, barBgB, barBgTol) {
		return true
	}
	if colorNear(r, g, b, 0, 0, 5, barBgNearBlackTol) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	return ri+gi+bi < barBgDarkSum
}

func isHPGreen(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	if colorNear(r, g, b, hpGreenR, hpGreenG, hpGreenB, fillTol) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	if gi >= hpGreenMinGreen && ri <= hpGreenMaxRed && bi <= gi+hpGreenBlueGapTol && gi > ri+hpGreenRedGapMin {
		return true
	}
	if gi >= hpGreenAltMinGreen && gi > ri+hpGreenAltMinDiff && gi > bi {
		return true
	}
	return gi > hpGreenBrightMin && gi > ri && gi+ri > bi+hpGreenAltMinDiff*2
}

func isHPRed(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	if colorNear(r, g, b, hpRedR, hpRedG, hpRedB, fillTol) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	return ri > hpRedMinRed && ri > gi+hpRedGreenDiff && ri > bi+hpRedGreenDiff
}

func isHPYellow(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	ri, gi, bi := int(r), int(g), int(b)
	return ri > hpYellowMinRed && gi > hpYellowMinGreen && bi < hpYellowMaxBlue && ri > bi+hpYellowRedBlueDiff && gi > bi+hpYellowGreenBlueDiff
}

func isSPBlue(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	if colorNear(r, g, b, spBlueR, spBlueG, spBlueB, fillTol) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	return bi >= spBlueMinBlue && bi > gi+spBlueGreenDiff && bi > ri+spBlueRedDiff
}

func isSPFill(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	ri, gi, bi := int(r), int(g), int(b)
	return bi >= spFillMinBlue && ri >= spFillMinRed && bi > gi+spFillBlueDiff && bi > ri+spFillBlueDiff
}

func isSPCyan(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	ri, gi, bi := int(r), int(g), int(b)
	return bi >= spCyanMinBlue && gi >= spCyanMinGreen && bi > ri+spCyanBlueDiff && gi > ri+spCyanGreenDiff
}

func colorNear(r, g, b, refR, refG, refB uint8, tol int) bool {
	return absInt(int(r)-int(refR)) <= tol &&
		absInt(int(g)-int(refG)) <= tol &&
		absInt(int(b)-int(refB)) <= tol
}

func pixelAt(img image.Image, x, y int) (r, g, b uint8) {
	c := img.At(x, y)
	rgba := color.RGBAModel.Convert(c).(color.RGBA)
	return rgba.R, rgba.G, rgba.B
}

func unionRect(a, b Rect) Rect {
	x1 := a.X
	if b.X < x1 {
		x1 = b.X
	}
	y1 := a.Y
	if b.Y < y1 {
		y1 = b.Y
	}
	x2 := a.X + a.W
	if b.X+b.W > x2 {
		x2 = b.X + b.W
	}
	y2 := a.Y + a.H
	if b.Y+b.H > y2 {
		y2 = b.Y + b.H
	}
	return Rect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1}
}

func barFromRead(r Rect, br BarRead) Bar {
	return Bar{
		Left:        r.X,
		Right:       r.X + r.W - 1,
		Y:           r.Y,
		Width:       r.W,
		Height:      r.H,
		FilledWidth: br.FilledWidth,
		Percent:     br.Percent,
		Found:       br.Found,
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
