//go:build android || nsgoandroidhost

package gamepad

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var androidDefault = &androidSource{}

func platformSource() driver.GamepadSource {
	return androidDefault
}

type androidSource struct{}

func (*androidSource) Poll() []driver.GamepadState {
	pads := driver.PollPlatformGamepads()
	for i := range pads {
		if pads[i].Family == driver.GamepadFamilyGeneric {
			pads[i].Family = driver.GamepadFamily(DetectFamily(pads[i].VendorID, pads[i].ProductID, pads[i].Name))
		}
	}
	return pads
}

func (*androidSource) Watch(onChange func()) func() {
	return driver.WatchPlatformGamepads(onChange)
}

func (*androidSource) Rumble(id int, lowFreq, highFreq float32, duration time.Duration) {
	driver.SendGamepadRumble(id, lowFreq, highFreq, duration)
}

func (*androidSource) StopRumble(id int) {
	driver.SendGamepadRumble(id, 0, 0, 0)
}
