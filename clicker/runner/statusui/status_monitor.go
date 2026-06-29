package statusui

import (
	"context"
	"sync/atomic"
	"time"

	win "experimental-clicker/runner/platform/windows"
)

// StatusMonitor periodically reads HP/SP values from the UI without affecting AutoPot.
// It captures, parses, and caches results independently.
type StatusMonitor struct {
	cached atomic.Value // *StatusValues
}

// NewStatusMonitor creates a new monitor with an empty initial snapshot.
func NewStatusMonitor() *StatusMonitor {
	m := &StatusMonitor{}
	m.cached.Store(&StatusValues{UpdatedAt: time.Now(), Valid: false})
	return m
}

// Start launches a background goroutine that periodically reads status values.
func (m *StatusMonitor) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.captureAndUpdate()
			}
		}
	}()
}

// Latest returns the most recently captured status values.
func (m *StatusMonitor) Latest() StatusValues {
	val := m.cached.Load().(*StatusValues)
	return *val
}

// captureAndUpdate captures the screen, parses, and stores StatusValues.
// It must not decide when to pot—only read and record.
func (m *StatusMonitor) captureAndUpdate() {
	img, _, err := win.CapturePlayerBarSearch()
	if err != nil || img == nil {
		m.cached.Store(&StatusValues{
			Valid:     false,
			UpdatedAt: time.Now(),
		})
		return
	}

	result, err := ParseNumericResources(img)
	if err != nil {
		m.cached.Store(&StatusValues{
			Valid:     false,
			UpdatedAt: time.Now(),
		})
		return
	}

	status := StatusValues{
		HP:         ResourceValue(result.HP),
		SP:         ResourceValue(result.SP),
		UpdatedAt:  time.Now(),
		Valid:      result.HP.Found || result.SP.Found,
		Confidence: (result.HP.Confidence + result.SP.Confidence) / 2,
	}
	m.cached.Store(&status)
}
