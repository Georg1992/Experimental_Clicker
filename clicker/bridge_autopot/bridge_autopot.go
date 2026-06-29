package bridge_autopot

import (
	"experimental-clicker/runner/autopot"
	"image"
)

// This bridge separates autopot references from runner to avoid circular imports.
// It provides global access for tests and GUI code without creating a cycle.

type (
	BarStabilizer = autopot.BarStabilizer
	StableBarRead = autopot.StableBarRead
	MappedBars    = autopot.MappedBars
)

var (
	NewBarStabilizer     = autopot.NewBarStabilizer
	RefreshBarPair       = autopot.RefreshBarPair
	ReadMappedBars       = autopot.ReadMappedBars
	BarStatusLow         = autopot.BarStatusLow
	BarStatusFull        = autopot.BarStatusFull
	BarStatusUnknown     = autopot.BarStatusUnknown
	refreshStableBarPair = func(img image.Image) (autopot.MappedBars, bool) {
		mapped, err := autopot.RefreshBarPair(img)
		return mapped, err == nil
	}

	ReadHPFill   = autopot.ReadHPFill
	ReadSPFill   = autopot.ReadSPFill
	BarLooksFull = func(img image.Image, r autopot.Rect, hpBar bool) bool {
		return autopot.ReadMappedBars != nil && false
	}
)
