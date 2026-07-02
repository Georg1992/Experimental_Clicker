// Package session defines the InputSession interface — the minimum surface
// needed by any runner that emits keys or mouse clicks.
// *runner.ViiperSession implements these methods; tests can use stubs.
package session

import "time"

// InputSession is the single, canonical interface used by every runner
// (clicker, autopot, keychain, timerkey). Each runner pulls from cfg.Session
// without a concrete type binding. ViiperSession satisfies it.
type InputSession interface {
	TapKey(vk int32, hold time.Duration) error
	MouseDown() error
	MouseUp() error
}
