package scrollbar

import "github.com/nsgo-dev/nsgo/internal/driver"

// InterruptHost ends scroll animation and glide state before a new input.
// Native wheel ownership and refresh indicators stay in the callbacks.
type InterruptHost struct {
	Momentum       bool
	ClearAnimation func()
	Gliding        bool
	ClearGlide     func()
	Event          func(driver.EventKind)
	AfterGlide     func()
}

// Interrupt preserves the order of momentum reports and native cleanup.
func (host InterruptHost) Interrupt() {
	if host.Momentum {
		host.Event(driver.EventMomentumEnd)
	}
	host.ClearAnimation()
	if host.Gliding {
		host.ClearGlide()
		host.Event(driver.EventMomentumEnd)
		if host.AfterGlide != nil {
			host.AfterGlide()
		}
	}
}
