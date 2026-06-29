package statusui

import "time"

// ResourceValue represents a single resource (HP or SP) parsed from the status panel.
type ResourceValue struct {
	Current    int
	Max        int
	Percent    float64
	Found      bool
	UpdatedAt  time.Time
	Confidence float64
}

// StatusValues represents a complete status snapshot from the UI.
type StatusValues struct {
	HP         ResourceValue
	SP         ResourceValue
	UpdatedAt  time.Time
	Valid      bool
	Confidence float64
}
