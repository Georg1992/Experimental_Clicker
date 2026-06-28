package runner

import "time"

const (
	PollInterval       = 10 * time.Millisecond
	CaptureRetryDelay  = 50 * time.Millisecond
	KeyTapHold         = 1 * time.Millisecond
	KeyBindTimeout     = 5 * time.Second
	KeyReleaseSettle   = 500 * time.Millisecond
	SessionCloseWait   = 10 * time.Second
	PotConfirmReads    = 3
	PotUnlatchReads    = 3
)
