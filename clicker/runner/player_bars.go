package runner

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
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

	// BarPairRecalInterval is how often cached HP/SP rects are refreshed while walking.
	BarPairRecalDefaultRecal = 200 * time.Millisecond
	BarPairRecalMin          = 100 * time.Millisecond
	BarPairRecalMax          = 500 * time.Millisecond
)

var BarPairRecalInterval = BarPairRecalDefaultRecal

// SetBarPairRecalInterval sets the periodic colored-run pair refresh interval (100–500ms).
func SetBarPairRecalInterval(d time.Duration) {
	if d < BarPairRecalMin {
		d = BarPairRecalMin
	}
	if d > BarPairRecalMax {
		d = BarPairRecalMax
	}
	BarPairRecalInterval = d
}

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

// Bar is kept for debug output and legacy callers.
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

// MapPlayerBars is an alias for RefreshBarPair.
func MapPlayerBars(img image.Image) (MappedBars, error) {
	return RefreshBarPair(img)
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

func ReadHPFill(img image.Image, hp Rect) BarRead {
	if hp.W < 1 || hp.H < 1 {
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
		return readBarFillSingleRow(img, hp.X, hp.Y, hp.W, isHPFillRead)
	}
	return best
}

func isHPFillRead(r, g, b uint8) bool {
	if IsHPPixel(r, g, b) {
		return true
	}
	if isHPTrack(r, g, b) {
		return false
	}
	ri, gi, _ := int(r), int(g), int(b)
	return gi >= 35 && ri >= 50 && absInt(ri-gi) < 25
}

func ReadSPFill(img image.Image, sp Rect) BarRead {
	if sp.W < 1 || sp.H < 1 {
		return BarRead{Found: false}
	}
	row := sp.Y + sp.H - 1
	return readBarFillSingleRow(img, sp.X, row, sp.W, isSPFill)
}

func FindHPBar(img image.Image) Bar {
	mapped, err := MapPlayerBars(img)
	if err != nil {
		return Bar{}
	}
	return barFromRead(mapped.HP, ReadHPFill(img, mapped.HP))
}

func FindSPBar(img image.Image) Bar {
	mapped, err := MapPlayerBars(img)
	if err != nil {
		return Bar{}
	}
	return barFromRead(mapped.SP, ReadSPFill(img, mapped.SP))
}

func NeedsRemap(bars MappedBars, hp, sp BarRead) bool {
	if !bars.Valid {
		return true
	}
	if time.Since(bars.LastMapped) > BarPairRecalInterval {
		return true
	}
	if !hp.Found || !sp.Found {
		return true
	}
	return false
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
			if absInt(hpRun.X1-spRun.X1) > 6 {
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

func scoreBarPair(hp, sp ColorRun, cx, cy int) int {
	midX := (hp.X1 + hp.X2 + sp.X1 + sp.X2) / 4
	midY := (hp.Y + sp.Y) / 2
	centerDist := absInt(midX-cx)*3 + absInt(midY-cy)*4

	abovePenalty := 0
	if midY < cy {
		abovePenalty = (cy - midY) * 4
	}

	gapPenalty := absInt((sp.Y-hp.Y)-expectedBarGap) * 8
	leftPenalty := absInt(hp.X1-sp.X1) * 4

	qualityBonus := (hp.Width + sp.Width) * 3
	if sp.Width >= 40 {
		qualityBonus += sp.Width * 2
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
	return r.Width*3 - absInt(mx-cx)*3 - absInt(r.Y-cy)*2
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
		minLeft := left - 12
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

func extendBarRightRow(img image.Image, y, fromX int, isRowPixel func(r, g, b uint8) bool) int {
	right := fromX
	b := img.Bounds()
	gap := 0
	for x := fromX + 1; x < b.Max.X; x++ {
		rp, gp, bp := pixelAt(img, x, y)
		if isRowPixel(rp, gp, bp) {
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

func isHPBarRowPixel(r, g, b uint8) bool {
	return IsHPPixel(r, g, b) || isHPTrack(r, g, b)
}

func isSPBarRowPixel(r, g, b uint8) bool {
	return IsSPPixel(r, g, b)
}

func isBarRowPixel(img image.Image, x, y int) bool {
	rp, gp, bp := pixelAt(img, x, y)
	return isHPBarRowPixel(rp, gp, bp) || isSPBarRowPixel(rp, gp, bp)
}

func readBarFillColumns(img image.Image, r Rect, isPixel func(r, g, b uint8) bool) BarRead {
	return readBarFillColumnsMinHits(img, r, isPixel, 0)
}

func readBarFillColumnsMinHits(img image.Image, r Rect, isPixel func(r, g, b uint8) bool, minHits int) BarRead {
	if r.W < 1 || r.H < 1 {
		return BarRead{Found: false}
	}
	if minHits < 1 {
		minHits = r.H / 2
		if minHits < 1 {
			minHits = 1
		}
	}

	filled := 0
	for col := 0; col < r.W; col++ {
		hits := 0
		x := r.X + col
		for row := 0; row < r.H; row++ {
			rp, gp, bp := pixelAt(img, x, r.Y+row)
			if isPixel(rp, gp, bp) {
				hits++
			}
		}
		if hits >= minHits {
			filled++
			continue
		}
		if filled > 0 {
			break
		}
	}

	return barReadFromFill(filled, r.W)
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

func IsHPPixel(r, g, b uint8) bool {
	return isHPGreen(r, g, b) || isHPRed(r, g, b) || isHPYellow(r, g, b)
}

func IsSPPixel(r, g, b uint8) bool {
	return isSPBlue(r, g, b) || isSPCyan(r, g, b)
}

func isBarTrack(r, g, b uint8) bool {
	return isHPTrack(r, g, b)
}

func isHPTrack(r, g, b uint8) bool {
	if IsHPPixel(r, g, b) || IsSPPixel(r, g, b) {
		return false
	}
	if colorNear(r, g, b, barBgR, barBgG, barBgB, 8) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	sum := ri + gi + bi
	if sum < 60 || sum > 210 {
		return false
	}
	return bi <= gi && absInt(ri-gi) < 30
}

func isSPTrack(r, g, b uint8) bool {
	if IsHPPixel(r, g, b) || IsSPPixel(r, g, b) {
		return false
	}
	if isBarBackground(r, g, b) {
		return false
	}
	ri, gi, bi := int(r), int(g), int(b)
	return bi >= 35 && bi > gi+6 && bi > ri+10
}

func isBarBackground(r, g, b uint8) bool {
	if colorNear(r, g, b, barBgR, barBgG, barBgB, barBgTol) {
		return true
	}
	if colorNear(r, g, b, 0, 0, 5, 15) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	return ri+gi+bi < 35
}

func isHPGreen(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	if colorNear(r, g, b, hpGreenR, hpGreenG, hpGreenB, fillTol) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	if gi >= 60 && ri <= 20 && bi <= gi+8 && gi > ri+10 {
		return true
	}
	if gi >= 50 && gi > ri+10 && gi > bi {
		return true
	}
	return gi > 80 && gi > ri && gi+ri > bi+20
}

func isHPRed(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	if colorNear(r, g, b, hpRedR, hpRedG, hpRedB, fillTol) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	return ri > 130 && ri > gi+25 && ri > bi+25
}

func isHPYellow(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	ri, gi, bi := int(r), int(g), int(b)
	return ri > 110 && gi > 90 && bi < 90 && ri > bi+15 && gi > bi+10
}

func isSPBlue(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	if colorNear(r, g, b, spBlueR, spBlueG, spBlueB, fillTol) {
		return true
	}
	ri, gi, bi := int(r), int(g), int(b)
	return bi >= 90 && bi > gi+10 && bi > ri+18
}

func isSPFill(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	ri, gi, bi := int(r), int(g), int(b)
	return bi >= 130 && ri >= 12 && bi > gi+20 && bi > ri+20
}

func isSPCyan(r, g, b uint8) bool {
	if isBarBackground(r, g, b) {
		return false
	}
	ri, gi, bi := int(r), int(g), int(b)
	return bi >= 80 && gi >= 60 && bi > ri+10 && gi > ri+5
}

func isBarFill(r, g, b uint8) bool {
	return IsHPPixel(r, g, b) || IsSPPixel(r, g, b)
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

func FormatBarReadLog(name string, r Rect, br BarRead) string {
	if !br.Found {
		return name + ": not found"
	}
	return name + ":\n" +
		"x=" + itoa(r.X) + "\n" +
		"y=" + itoa(r.Y) + "\n" +
		"w=" + itoa(r.W) + "\n" +
		"h=" + itoa(r.H) + "\n" +
		"fillPx=" + itoa(br.FilledWidth) + "\n" +
		"fullPx=" + itoa(br.FullWidth) + "\n" +
		"percent=" + ftoa(br.Percent)
}

func FormatMappedBarsLog(img image.Image, bars MappedBars, hp, sp BarRead, refreshed bool) string {
	b := img.Bounds()
	cx := b.Min.X + b.Dx()/2
	cy := b.Min.Y + b.Dy()/2
	roi := driftROI(b)
	roiCX := roi.X + roi.W/2
	roiCY := roi.Y + roi.H/2
	hpRuns := consolidateRuns(scanColorRuns(img, roi, IsHPPixel, "hp"))
	spRuns := consolidateRuns(scanColorRuns(img, roi, IsSPPixel, "sp"))
	hpRun, spRun, _, _ := findPlayerBarPair(hpRuns, spRuns, roi, roiCX, roiCY)
	pcx, pcy := pairCenter(hpRun, spRun)
	mode := "reused"
	if refreshed {
		mode = "refreshed"
	}
	age := time.Since(bars.LastMapped)
	return "driftROI x=" + itoa(roi.X) + " y=" + itoa(roi.Y) +
		" w=" + itoa(roi.W) + " h=" + itoa(roi.H) + "\n" +
		"screenCenter x=" + itoa(cx) + " y=" + itoa(cy) + "\n" +
		"pairCenter x=" + itoa(pcx) + " y=" + itoa(pcy) + "\n" +
		"barPair=" + mode + " remapAge=" + formatDuration(age) +
		" recalInterval=" + formatDuration(BarPairRecalInterval) + "\n" +
		"hpRuns=" + itoa(len(hpRuns)) + " spRuns=" + itoa(len(spRuns)) + "\n" +
		"selectedHP x1=" + itoa(hpRun.X1) + " x2=" + itoa(hpRun.X2) +
		" y=" + itoa(hpRun.Y) + " w=" + itoa(hpRun.Width) + "\n" +
		"selectedSP x1=" + itoa(spRun.X1) + " x2=" + itoa(spRun.X2) +
		" y=" + itoa(spRun.Y) + " w=" + itoa(spRun.Width) + "\n" +
		"mapScore=" + itoa(bars.MapScore) + "\n" +
		"mapped HP rect x=" + itoa(bars.HP.X) + " y=" + itoa(bars.HP.Y) +
		" w=" + itoa(bars.HP.W) + " h=" + itoa(bars.HP.H) + "\n" +
		"mapped SP rect x=" + itoa(bars.SP.X) + " y=" + itoa(bars.SP.Y) +
		" w=" + itoa(bars.SP.W) + " h=" + itoa(bars.SP.H) + "\n" +
		FormatBarReadLog("HP", bars.HP, hp) + "\n" +
		FormatBarReadLog("SP", bars.SP, sp)
}

func formatDuration(d time.Duration) string {
	ms := d.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return itoa(int(ms)) + "ms"
}

func SaveBarSearchDebug(img image.Image, hp, sp Bar, path string) error {
	mapped, err := MapPlayerBars(img)
	if err != nil {
		return err
	}
	return SaveMappedBarsDebug(img, mapped, path)
}

func SaveMappedBarsDebug(img image.Image, bars MappedBars, path string) error {
	if path == "" {
		return nil
	}
	b := img.Bounds()
	cx := b.Min.X + b.Dx()/2
	cy := b.Min.Y + b.Dy()/2
	roi := driftROI(b)
	roiCX := roi.X + roi.W/2
	roiCY := roi.Y + roi.H/2
	hpRuns := consolidateRuns(scanColorRuns(img, roi, IsHPPixel, "hp"))
	spRuns := consolidateRuns(scanColorRuns(img, roi, IsSPPixel, "sp"))
	hpRun, spRun, _, _ := findPlayerBarPair(hpRuns, spRuns, roi, roiCX, roiCY)
	pcx, pcy := pairCenter(hpRun, spRun)

	out := imageToRGBA(img)
	drawRectOutline(out, roi, color.RGBA{R: 255, G: 255, B: 0, A: 255})
	drawCross(out, cx, cy, color.RGBA{R: 255, G: 0, B: 255, A: 255})
	drawCross(out, pcx, pcy, color.RGBA{R: 255, G: 128, B: 0, A: 255})
	for _, r := range hpRuns {
		drawRunOutline(out, r, color.RGBA{R: 0, G: 180, B: 0, A: 255})
	}
	for _, r := range spRuns {
		drawRunOutline(out, r, color.RGBA{R: 80, G: 160, B: 255, A: 255})
	}
	drawRunOutline(out, hpRun, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	drawRunOutline(out, spRun, color.RGBA{R: 0, G: 200, B: 255, A: 255})

	hp := barFromRead(bars.HP, ReadHPFill(img, bars.HP))
	sp := barFromRead(bars.SP, ReadSPFill(img, bars.SP))
	drawBarRectDebug(out, hp, color.RGBA{R: 0, G: 255, B: 0, A: 255}, color.RGBA{R: 0, G: 200, B: 0, A: 255})
	drawBarRectDebug(out, sp, color.RGBA{R: 0, G: 128, B: 255, A: 255}, color.RGBA{R: 0, G: 200, B: 255, A: 255})

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}

func drawCross(img *image.RGBA, x, y int, c color.RGBA) {
	b := img.Bounds()
	for dx := -6; dx <= 6; dx++ {
		px := x + dx
		if px >= b.Min.X && px < b.Max.X && y >= b.Min.Y && y < b.Max.Y {
			img.Set(px, y, c)
		}
	}
	for dy := -6; dy <= 6; dy++ {
		py := y + dy
		if x >= b.Min.X && x < b.Max.X && py >= b.Min.Y && py < b.Max.Y {
			img.Set(x, py, c)
		}
	}
}

func drawRunOutline(img *image.RGBA, r ColorRun, c color.RGBA) {
	for x := r.X1; x <= r.X2; x++ {
		img.Set(x, r.Y, c)
	}
}

func drawRectOutline(img *image.RGBA, r Rect, c color.RGBA) {
	for x := r.X; x < r.X+r.W; x++ {
		img.Set(x, r.Y, c)
		img.Set(x, r.Y+r.H-1, c)
	}
	for y := r.Y; y < r.Y+r.H; y++ {
		img.Set(r.X, y, c)
		img.Set(r.X+r.W-1, y, c)
	}
}

func drawBarRectDebug(img *image.RGBA, bar Bar, outline, fill color.RGBA) {
	if !bar.Found {
		return
	}
	h := bar.Height
	if h < 1 {
		h = barRowHeight
	}
	for dy := 0; dy < h; dy++ {
		y := bar.Y + dy
		img.Set(bar.Left, y, outline)
		img.Set(bar.Right, y, outline)
	}
	for dx := 0; dx < bar.Width; dx++ {
		x := bar.Left + dx
		img.Set(x, bar.Y, outline)
		img.Set(x, bar.Y+h-1, outline)
	}
	for dx := 0; dx < bar.FilledWidth; dx++ {
		x := bar.Left + dx
		for dy := 0; dy < h; dy++ {
			img.Set(x, bar.Y+dy, fill)
		}
	}
}

func FormatBarLog(name string, bar Bar) string {
	if !bar.Found {
		return name + ": not found"
	}
	h := bar.Height
	if h < 1 {
		h = barRowHeight
	}
	return name + ":\n" +
		"x=" + itoa(bar.Left) + "\n" +
		"y=" + itoa(bar.Y) + "\n" +
		"w=" + itoa(bar.Width) + "\n" +
		"h=" + itoa(h) + "\n" +
		"fillPx=" + itoa(bar.FilledWidth) + "\n" +
		"percent=" + ftoa(bar.Percent)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func ftoa(v float64) string {
	return itoa(int(v+0.5)) + "%"
}

func imageToRGBA(img image.Image) *image.RGBA {
	bounds := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := bounds.Min.Y; y < bounds.Min.Y+bounds.Dy(); y++ {
		for x := bounds.Min.X; x < bounds.Min.X+bounds.Dx(); x++ {
			out.Set(x-bounds.Min.X, y-bounds.Min.Y, img.At(x, y))
		}
	}
	return out
}
