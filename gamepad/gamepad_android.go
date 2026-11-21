//go:build android

package gamepad

import "github.com/nsgo-dev/nsgo/internal/driver"

var androidDefault = &androidSource{}

func platformSource() driver.GamepadSource {
	return androidDefault
}

type androidSource struct{}

func (*androidSource) Poll() []driver.GamepadState {
	return driver.PollPlatformGamepads()
}

func (*androidSource) Watch(onChange func()) func() {
	return driver.WatchPlatformGamepads(onChange)
}
