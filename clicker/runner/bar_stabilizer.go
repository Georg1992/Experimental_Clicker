package runner

import (
	"image"
	"sync"
)

type BarStatus int

const (
	BarStatusUnknown BarStatus = iota
	BarStatusFull
	BarStatusOK
	BarStatusLow
)

type StableBarRead struct {
	Found      bool
	Percent    float64
	Status     BarStatus
	Rect       Rect
	Confidence float64
}

type BarPairCache struct{}

func (c *BarPairCache) Reset() {}

func (c *BarPairCache) trustedPair(img image.Image) (MappedBars, bool) {
	return refreshStableBarPair(img)
}

type BarStabilizer struct {
	pair      *BarPairCache
	hpBar     bool
	threshold int

	mu                 sync.Mutex
	lastValidRect      Rect
	fullLatched        bool
	notFullStreak int
	lowStreak     int
}

func NewBarStabilizer(pair *BarPairCache, hpBar bool, threshold int) *BarStabilizer {
	return &BarStabilizer{pair: pair, hpBar: hpBar, threshold: threshold}
}

func (s *BarStabilizer) SetThreshold(threshold int) {
	s.mu.Lock()
	s.threshold = threshold
	s.mu.Unlock()
}

func (s *BarStabilizer) Reset() {
	s.mu.Lock()
	s.lastValidRect = Rect{}
	s.fullLatched = false
	s.notFullStreak = 0
	s.lowStreak = 0
	s.mu.Unlock()
}

func (s *BarStabilizer) Update(img image.Image, hpBar bool) StableBarRead {
	if hpBar != s.hpBar || img == nil {
		return s.latchedFullOrUnknown()
	}

	mapped, ok := s.pair.trustedPair(img)
	if !ok {
		return s.latchedFullOrUnknown()
	}

	hp, sp := ReadMappedBars(img, mapped)
	if !hp.Found || !sp.Found {
		return s.latchedFullOrUnknown()
	}

	var read BarRead
	var rect Rect
	if s.hpBar {
		read, rect = hp, mapped.HP
	} else {
		read, rect = sp, mapped.SP
	}
	if !read.Found || !barReadConsistent(img, rect, s.hpBar, read) {
		return s.latchedFullOrUnknown()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastValidRect.W >= 1 && rectDrifted(s.lastValidRect, rect, BarPositionMaxDrift) {
		s.lowStreak = 0
		s.fullLatched = false
		s.notFullStreak = 0
	}

	s.lastValidRect = rect
	return s.applyTrustedReadLocked(img, rect, read)
}

func (s *BarStabilizer) latchedFullOrUnknown() StableBarRead {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fullLatched && s.lastValidRect.W >= 1 {
		return s.fullReadLocked()
	}
	return StableBarRead{Status: BarStatusUnknown, Confidence: 0}
}

func (s *BarStabilizer) fullReadLocked() StableBarRead {
	return StableBarRead{
		Found:      true,
		Percent:    100,
		Status:     BarStatusFull,
		Rect:       s.lastValidRect,
		Confidence: 1,
	}
}

func (s *BarStabilizer) applyTrustedReadLocked(img image.Image, rect Rect, read BarRead) StableBarRead {
	if barLooksFull(img, rect, s.hpBar) || read.Percent >= 99.9 {
		s.fullLatched = true
		s.notFullStreak = 0
		s.lowStreak = 0
		return StableBarRead{
			Found:      true,
			Percent:    100,
			Status:     BarStatusFull,
			Rect:       rect,
			Confidence: 1,
		}
	}

	if s.fullLatched {
		if barConfirmedNotFull(img, rect, s.hpBar, read) {
			s.notFullStreak++
			if s.notFullStreak >= PotUnlatchReads {
				s.fullLatched = false
				s.notFullStreak = 0
			}
		} else {
			s.notFullStreak = 0
		}
		if s.fullLatched {
			return s.fullReadLocked()
		}
	}

	if read.Percent < float64(s.threshold) {
		s.lowStreak++
	} else {
		s.lowStreak = 0
	}

	if s.lowStreak >= PotConfirmReads {
		return StableBarRead{
			Found:      true,
			Percent:    read.Percent,
			Status:     BarStatusLow,
			Rect:       rect,
			Confidence: 1,
		}
	}

	confidence := 0.9
	if read.Percent < float64(s.threshold) && s.lowStreak > 0 {
		confidence = 0.5 + 0.4*float64(s.lowStreak)/float64(PotConfirmReads)
	}

	return StableBarRead{
		Found:      true,
		Percent:    read.Percent,
		Status:     BarStatusOK,
		Rect:       rect,
		Confidence: confidence,
	}
}
