//go:build (ios && nsgowatch) || (js && wasm && (nsgominiprogram || nsgoharmony)) || (!darwin && !windows && !linux && !android && !(js && wasm))

package gamepad

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var unsupportedDefault = &unsupportedSource{}

func platformSource() driver.GamepadSource {
	return unsupportedDefault
}

type unsupportedSource struct{}

func (*unsupportedSource) Poll() []driver.GamepadState                 { return nil }
func (*unsupportedSource) Watch(func()) func()                         { return func() {} }
func (*unsupportedSource) Rumble(int, float32, float32, time.Duration) {}
func (*unsupportedSource) StopRumble(int)                              {}
