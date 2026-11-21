//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// Where a window takes the pointer (driver.WindowPointerEvents). A window that takes none ignores mouse events, which AppKit then sends to the window below, the app's or another's. One that takes it where it draws ignores them while the pointer is over nothing painted: AppKit sends an ignoring window no moves, so a timer on the main run loop follows the pointer at the display's pace and hit-tests the views under it each time it moved, a batch came or an animation runs, as the views under a still pointer may change.

// passInterval is how often a painted window looks where the pointer is: a frame at 60 Hz, so a press just after the pointer comes over a view lands in the window.
const passInterval = 1.0 / 60

// windowPointer holds what a surface's window does with the pointer.
type windowPointer struct {
	mode driver.WindowPointerEvents
	// timer is the painted window's NSTimer, 0 for the other modes; ignoring is what the window was last told, and at and batch where the pointer was and the batch applied at the last hit test.
	timer    objc.ID
	ignoring bool
	at       apple.CGPoint
	batch    uint64
	tested   bool
	// An animated image changes the alpha under a still pointer.
	animatedImage bool
}

// setPointerEvents makes the window take the pointer as mode says, from now.
func (s *surface) setPointerEvents(mode driver.WindowPointerEvents) {
	if s.window == 0 || s.embedded || s.closed {
		return
	}
	s.pointer.mode = mode
	switch mode {
	case driver.WindowPointerNone:
		s.stopPassTimer()
		s.ignoreMouse(true)
	case driver.WindowPointerPainted:
		s.startPassTimer()
		s.pointer.tested = false
		s.passUnpainted()
	default:
		s.stopPassTimer()
		s.ignoreMouse(false)
	}
}

// ignoreMouse tells the window whether to let mouse events through.
func (s *surface) ignoreMouse(on bool) {
	if s.pointer.ignoring == on {
		return
	}
	s.pointer.ignoring = on
	s.window.Send(sel("setIgnoresMouseEvents:"), on)
}

// startPassTimer starts the painted window's timer, on the main run loop's common modes so that it runs through a drag and a menu's tracking too.
func (s *surface) startPassTimer() {
	if s.pointer.timer != 0 {
		return
	}
	tick := objcblock.New(func(_ objc.Block, _ objc.ID) {
		if !s.closed {
			s.passUnpainted()
		}
	})
	t := objc.ID(apple.Class("NSTimer")).Send(sel("timerWithTimeInterval:repeats:block:"), passInterval, true, tick)
	tick.Release()
	s.pointer.timer = t.Send(sel("retain"))
	mainLoop().Send(sel("addTimer:forMode:"), t, apple.CommonModes())
}

// stopPassTimer stops the painted window's timer.
func (s *surface) stopPassTimer() {
	if t := s.pointer.timer; t != 0 {
		s.pointer.timer = 0
		t.Send(sel("invalidate"))
		t.Send(selRelease)
	}
}

// passUnpainted lets the mouse through while the pointer is over nothing painted, by the hit test of the views under it. A button held keeps what the window does, so a drag that began on a view goes on to its end and one from another app passes over the window.
func (s *surface) passUnpainted() {
	loc := objc.Send[apple.CGPoint](s.window, sel("mouseLocationOutsideOfEventStream"))
	if s.pointer.tested && loc == s.pointer.at && s.batch == s.pointer.batch && !s.ticking && !s.pointer.animatedImage {
		return
	}
	if !objc.Send[bool](s.window, sel("isVisible")) ||
		objc.Send[uint](objc.ID(apple.Class("NSEvent")), sel("pressedMouseButtons")) != 0 {
		return
	}
	s.pointer.at, s.pointer.batch, s.pointer.tested = loc, s.batch, true
	s.pointer.animatedImage = false
	s.ignoreMouse(!s.paintedAt(loc))
}

// paintedAt reports whether a view under loc, a point of the window, takes the pointer in a painted window: the view the hit test finds, or one of the views it is in (driver.Painted).
func (s *surface) paintedAt(loc apple.CGPoint) bool {
	hit := s.hitAt(loc)
	stop := s.native.Send(selSuperview)
	for n := hit; n != 0 && n != stop; n = n.Send(selSuperview) {
		if v := views[n]; v != nil && driver.Painted(v.kind, &v.style, v.events, v.windowDrag) {
			return true
		}
	}
	return false
}
