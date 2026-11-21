package windows

import "github.com/nsgo-dev/nsgo/internal/driver"

// colorRefInvalid is Win32's CLR_INVALID returned by GetPixel on failure.
const colorRefInvalid uint32 = 0xffffffff

// colorFromColorRef converts a Win32 COLORREF (0x00BBGGRR) into an opaque
// sRGB driver.Color (0xRRGGBBAA). CLR_INVALID returns (0, false).
func colorFromColorRef(cr uint32) (driver.Color, bool) {
	if cr == colorRefInvalid {
		return 0, false
	}
	r := cr & 0xff
	g := (cr >> 8) & 0xff
	b := (cr >> 16) & 0xff
	return driver.Color(r<<24 | g<<16 | b<<8 | 0xff), true
}

// virtualScreenRect describes Windows' virtual desktop in physical screen
// pixels, which may have negative origins when a monitor sits left of or above
// the primary display.
type virtualScreenRect struct {
	x, y, width, height int32
}

func (r virtualScreenRect) valid() bool { return r.width > 0 && r.height > 0 }

func (r virtualScreenRect) contains(x, y int32) bool {
	if !r.valid() {
		return false
	}
	return x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}

func (r virtualScreenRect) clamp(x, y int32) (int32, int32) {
	if !r.valid() {
		return x, y
	}
	maxX := r.x + r.width - 1
	maxY := r.y + r.height - 1
	if x < r.x {
		x = r.x
	} else if x > maxX {
		x = maxX
	}
	if y < r.y {
		y = r.y
	} else if y > maxY {
		y = maxY
	}
	return x, y
}

type colorPickerEvent uint8

const (
	colorPickLeftDown colorPickerEvent = iota + 1
	colorPickRightDown
	colorPickMiddleDown
	colorPickEscape
	colorPickOtherKey
	colorPickCaptureLost
	colorPickDeactivate
	colorPickCancelMode
	colorPickDestroy
)

type colorPickerOutcome uint8

const (
	colorPickContinue colorPickerOutcome = iota
	colorPickSample
	colorPickCancel
)

// colorPickerState drives the Windows eyedropper's input transitions and
// ensures every pick completes at most once.
type colorPickerState struct {
	starting bool
	done     bool
}

func (s *colorPickerState) handle(ev colorPickerEvent) colorPickerOutcome {
	if s.done {
		return colorPickContinue
	}
	switch ev {
	case colorPickLeftDown:
		return colorPickSample
	case colorPickRightDown, colorPickMiddleDown, colorPickEscape, colorPickCancelMode, colorPickDestroy:
		return colorPickCancel
	case colorPickCaptureLost, colorPickDeactivate:
		if s.starting {
			return colorPickContinue
		}
		return colorPickCancel
	default:
		return colorPickContinue
	}
}

func (s *colorPickerState) finish(c driver.Color, ok bool) (driver.Color, bool, bool) {
	if s.done {
		return 0, false, false
	}
	s.done = true
	if !ok {
		return 0, false, true
	}
	return c, true, true
}

func colorPickerResultEvent(id uint32, c driver.Color, ok bool) driver.Event {
	if !ok {
		return driver.Event{Kind: driver.EventFiles, Count: id}
	}
	return driver.Event{Kind: driver.EventFiles, Count: id, Finished: true, Data: c}
}
