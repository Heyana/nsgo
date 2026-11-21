//go:build (ios && nsgowatch) || (js && wasm && (nsgominiprogram || nsgoharmony)) || (!darwin && !windows && !linux && !android && !(js && wasm))

package gamepad

import "github.com/nsgo-dev/nsgo/internal/driver"

var unsupportedDefault = &unsupportedSource{}

func platformSource() driver.GamepadSource {
	return unsupportedDefault
}

type unsupportedSource struct{}

func (*unsupportedSource) Poll() []driver.GamepadState { return nil }
func (*unsupportedSource) Watch(func()) func()         { return func() {} }
