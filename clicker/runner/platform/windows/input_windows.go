//go:build windows

package runner

import (
	"time"

	"golang.org/x/sys/windows"
)

const (
	KeyReleaseSettle = 500 * time.Millisecond
	PollInterval     = 10 * time.Millisecond
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

const escapeVK = 0x1B

func PollKeyToggle(wasDown *bool, vk int32) bool {
	down := PhysicalKeyDown(vk)
	toggled := down && !*wasDown
	*wasDown = down
	return toggled
}

func PhysicalKeyDown(vk int32) bool {
	ret, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return ret&0x8000 != 0
}

func ActiveTrigger(vks []int32) (int32, bool) {
	for _, vk := range vks {
		if PhysicalKeyDown(vk) {
			return vk, true
		}
	}
	return 0, false
}

func TriggerHeld(triggerVKs []int32) bool {
	_, held := ActiveTrigger(triggerVKs)
	return held
}

// WaitForKeyPress waits for keys to be released, then returns the next key pressed.
func WaitForKeyPress(timeout time.Duration) (int32, bool) {
	deadline := time.Now().Add(timeout)
	releaseBy := time.Now().Add(KeyReleaseSettle)
	for time.Now().Before(releaseBy) {
		if !anyPhysicalKeyDown() {
			break
		}
		time.Sleep(PollInterval)
	}

	for time.Now().Before(deadline) {
		for vk := int32(0x08); vk <= 0xFE; vk++ {
			if vk == escapeVK {
				continue
			}
			if PhysicalKeyDown(vk) {
				for PhysicalKeyDown(vk) && time.Now().Before(deadline) {
					time.Sleep(PollInterval)
				}
				return vk, true
			}
		}
		time.Sleep(PollInterval)
	}
	return 0, false
}

func anyPhysicalKeyDown() bool {
	for vk := int32(0x08); vk <= 0xFE; vk++ {
		if vk == escapeVK {
			continue
		}
		if PhysicalKeyDown(vk) {
			return true
		}
	}
	return false
}
