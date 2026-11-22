package windows

import "github.com/nsgo-dev/nsgo/internal/driver"

// wheelNotch is how far the content under a view taking the wheel moves
// for a notch: three lines of 33 points, as browsers scroll about 100.
const wheelNotch driver.Points = 99

// Win32 MK_* key state bits carried in WM_MOUSEWHEEL / WM_MOUSEHWHEEL wParam.
const (
	wheelMKShift   = 0x0004
	wheelMKControl = 0x0008
)

// wheelEventValues translates a Win32 WM_MOUSEWHEEL or WM_MOUSEHWHEEL delta
// and wParam into the content delta, precision-touchpad flag, and merged
// modifiers for EventWheel.
func wheelEventValues(delta int16, tilt bool, wParam uintptr, mods driver.Modifiers) (evDelta driver.Point, precise bool, outMods driver.Modifiers) {
	d := driver.Points(delta) / 120 * wheelNotch
	// A turn away from the user shows what is above; a tilt right, what is right.
	if tilt {
		evDelta.X = -d
	} else {
		evDelta.Y = d
	}
	precise = delta%120 != 0
	outMods = mods
	if wParam&wheelMKControl != 0 {
		outMods.Control = true
	}
	if wParam&wheelMKShift != 0 {
		outMods.Shift = true
	}
	return evDelta, precise, outMods
}

// zoomScale computes the pinch scale factor from WM_GESTURE GID_ZOOM
// arguments, keeping fallback when startDist is zero or a GF_END packet
// reports a zero argument.
func zoomScale(arguments uint32, startDist float64, fallback float32) float32 {
	if startDist > 0 && arguments > 0 {
		return float32(float64(arguments) / startDist)
	}
	return fallback
}
