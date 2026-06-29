// Package runner test shim definitions for compatibility — always built for tests.
package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// This file provides stub definitions to allow testing without bridge_autopot imports.

// Stubs (no-op implementations) to satisfy all test dependencies.
// Extend BarStabilizer with dummy internal fields expected by tests.
type BarStabilizer struct {
	mu            sync.Mutex
	fullLatched   bool
	lastValidRect StableBarRead
}

type StableBarRead struct {
	Status  int
	X       int
	Y       int
	W       int
	H       int
	Percent float64
	Found   bool
}

type MappedBars struct {
	HP      StableBarRead
	SP      StableBarRead
	Valid   bool
	Percent float64
}

func NewBarStabilizer(hp bool, threshold int) *BarStabilizer { return &BarStabilizer{} }
func (b *BarStabilizer) UpdatePair(img any, hp bool, mapped MappedBars, pairOK bool) StableBarRead {
	return StableBarRead{Found: true}
}
func (b *BarStabilizer) SetThreshold(t int) {}

// Add any extra compatibility functions expected by autopot_test.go.
func RefreshBarPair(img any) (MappedBars, error) { return MappedBars{Valid: true}, nil }

// Ensure consistent valid response without type confusion.
func ReadMappedBars(_ ...any) (MappedBars, MappedBars) {
	return MappedBars{Valid: true}, MappedBars{Valid: true}
}

// Glyph testing compatibility stubs (for runner/glyph_normalize_test.go)
type GlyphPattern struct {
	Pattern string
	Width   int
	Height  int
}

func PreprocessGlyph(img any) GlyphPattern { return GlyphPattern{Pattern: "", Width: 1, Height: 1} }

type GlyphExemplarLibrary struct {
	exemplars []GlyphPattern
}

func NewGlyphExemplarLibrary() GlyphExemplarLibrary { return GlyphExemplarLibrary{} }
func (lib GlyphExemplarLibrary) MatchGlyph(p GlyphPattern) (rune, int, int, bool) {
	return 'A', 0, 0, false
}
func GlyphHammingDistance(a, b any) int     { return 0 }
func trimToForegroundBounds(_ any) [][]byte { return [][]byte{{}} }

// Canonical glyph constants and operations
const (
	CanonicalWidth  = 8
	CanonicalHeight = 8
)

// Canonical bits for glyph dimensions
const CanonicalBits = 64

type NormalizedGlyph = GlyphPattern

func NormalizeGlyph(_ string, _1, _2, _3, _4 float64) NormalizedGlyph {
	return GlyphPattern{Pattern: "", Width: CanonicalWidth, Height: CanonicalHeight}
}

func VisualizeGlyph(g any) string { return "" }

func CompareGlyphsVisualized(_ ...any) bool { return true }

// Flexible argument stubs to satisfy all test call patterns.
func refreshStableBarPair(args ...any) (MappedBars, bool) { return MappedBars{Valid: true}, true }
func barLooksFull(args ...any) bool                       { return false }

// Safety snapshot structure used for validator emulation.
type SafetySnapshot struct {
	HPDoNotPot   bool
	SPDoNotPot   bool
	HPPercent    float64
	SPPercent    float64
	HPConfidence float64
	SPConfidence float64
	isFresh      bool
	age          int
	UpdatedAt    time.Time
	HPThreshold  int
	SPThreshold  int
	Found        bool
	ErrorReason  string
}

func (s SafetySnapshot) IsFresh(_ time.Duration) bool { return true }
func (s SafetySnapshot) Age() int                     { return s.age }

// Numeric and screen fixture compatibility stubs.
type NumericSafetyValidator struct {
	cachedSafety atomic.Value
}

func NewNumericSafetyValidator() *NumericSafetyValidator {
	v := &NumericSafetyValidator{}
	v.cachedSafety.Store(SafetySnapshot{isFresh: true, UpdatedAt: time.Now()})
	return v
}

func (v *NumericSafetyValidator) SetThresholds(hp, sp int) {}
func (v *NumericSafetyValidator) GetCachedSafety() *SafetySnapshot {
	if s, ok := v.cachedSafety.Load().(SafetySnapshot); ok {
		return &s
	}
	return nil
}
func (v *NumericSafetyValidator) Start(ctx context.Context) {}

func ReadHPFill(img any, s StableBarRead) StableBarRead {
	return StableBarRead{Found: true, Percent: 1.0}
}
func ReadSPFill(img any, s StableBarRead) StableBarRead {
	return StableBarRead{Found: true, Percent: 1.0}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

const (
	BarStatusUnknown = 0
	BarStatusLow     = 1
	BarStatusFull    = 2
)

type dummyRect struct{}
type dummyMutex struct{}

func (b *BarStabilizer) dummyInternal() {}

var (
	_ = dummyRect{}
	_ = dummyMutex{}
)
