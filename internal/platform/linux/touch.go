//go:build linux && !android

package linux

import (
	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A touchscreen's touches are the views' fingers, one per GDK event sequence, on X11 and Wayland alike: a legacy controller in the surface's capture phase sees every sequence before GTK's gestures do and leaves them to those gestures, so presses, pans, pinches, rotations and touch scrolling go on as before. The press a touch begins reports no mouse finger (pointerDown), so a finger is never reported twice.

// GDK's touch event types.
const (
	gdkTouchUpdate = 18
	gdkTouchEnd    = 19
	gdkTouchCancel = 20
)

var gdkEventGetEventSequence func(uintptr) uintptr

func bindTouches() {
	purego.RegisterLibFunc(&gdkEventGetEventSequence, gtkLib, "gdk_event_get_event_sequence")
}

func touchCallback(controller, event, data uintptr) int32 {
	w := windowOf(data)
	if w == nil || w.closing {
		return 0
	}
	if w.ignoreTestInput() {
		return 1
	}
	w.touchEvent(gdkEventGetEventType(event), gdkEventGetEventSequence(event), event)
	return 0
}

// touchEvent reports the touch event of type typ, of sequence, as a finger.
func (w *window) touchEvent(typ int32, sequence, event uintptr) {
	if typ < gdkTouchBegin || typ > gdkTouchCancel || sequence == 0 {
		return
	}
	at, ok := w.eventPoint(event)
	switch typ {
	case gdkTouchBegin:
		// Touches on a field, a page or a video's controls are theirs, as their presses are.
		if !ok || w.nativeAt(gtkWidgetPick(w.surface, float64(at.X), float64(at.Y), gtkPickDefault)) {
			return
		}
		w.touch(w.fingers.TouchDown(sequence, w.listensTouches(), at))
	case gdkTouchUpdate:
		if ok {
			w.touch(w.fingers.TouchMove(sequence, at))
		}
	case gdkTouchEnd:
		if !ok {
			at = w.lastTouch(sequence)
		}
		w.touch(w.fingers.TouchUp(sequence, at))
	case gdkTouchCancel:
		w.touch(w.fingers.TouchCancel(sequence, w.lastTouch(sequence)))
	}
}

// lastTouch is where sequence was last reported.
func (w *window) lastTouch(sequence uintptr) driver.Point {
	p, _ := w.fingers.TouchAt(sequence)
	return p
}

// cancelTouches reports the touchscreen's fingers taken away, as a scroll view takes them.
func (w *window) cancelTouches() {
	w.touch(w.fingers.CancelTouches())
}
