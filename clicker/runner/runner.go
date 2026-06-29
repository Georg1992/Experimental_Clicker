package runner

import (
	"experimental-clicker/bridge_autopot"
)

/*
Temporary alias to bridge_autopot until direct imports are refactored.
*/
type (
	BarStabilizer = bridge_autopot.BarStabilizer
	StableBarRead = bridge_autopot.StableBarRead
	MappedBars    = bridge_autopot.MappedBars
)

var (
	NewBarStabilizer = bridge_autopot.NewBarStabilizer
	RefreshBarPair   = bridge_autopot.RefreshBarPair
	ReadMappedBars   = bridge_autopot.ReadMappedBars
	BarStatusLow     = bridge_autopot.BarStatusLow
	BarStatusFull    = bridge_autopot.BarStatusFull
)
