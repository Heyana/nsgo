//go:build android || nsgoandroidhost

package android

import "github.com/nsgo-dev/nsgo/internal/driver"

// Gamepad message sub-operations, kept in sync with Protocol.java.
const (
	gamepadConnect    = 0
	gamepadDisconnect = 1
	gamepadState      = 2
)

// readGamepad decodes a msgGamepad payload sent by Gamepads.java and forwards it
// to package driver's platform gamepad bridge for package gamepad.
func readGamepad(r *reader) {
	op := r.byte()
	id := int(int32(r.u32()))
	switch op {
	case gamepadConnect:
		vendorID := uint16(r.u32())
		productID := uint16(r.u32())
		canRumble := r.byte() != 0
		name := r.str()
		driver.ReportGamepadConnectInfo(id, name, vendorID, productID, canRumble)
	case gamepadDisconnect:
		driver.ReportGamepadDisconnect(id)
	case gamepadState:
		var buttons [driver.GamepadButtonCount]float32
		for i := range buttons {
			buttons[i] = r.float()
		}
		var axes [driver.GamepadAxisCount]float32
		for i := range axes {
			axes[i] = r.float()
		}
		driver.ReportGamepadState(id, buttons, axes)
	}
}
