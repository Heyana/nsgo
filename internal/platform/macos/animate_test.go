//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Closing a surface whose frame clock ran lets go of its view: the display link, which the run loop holds, holds the view as its target.
func TestClosingASurfaceReleasesItsDisplayLink(t *testing.T) {
	onMain(func() {
		loadAppKit()
		drained := func(fn func()) {
			pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
			fn()
			pool.Send(sel("drain"))
		}
		var window objc.ID
		var s *surface
		drained(func() {
			window = objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
				apple.Rect(0, 0, 200, 100), uint(0), uint(2), false)
			window.Send(sel("setReleasedWhenClosed:"), false)
			s = newSurface(Options{Width: 200, Height: 100}, false)
			s.window = window
			window.Send(sel("setContentView:"), s.native)
		})
		defer window.Send(selRelease)
		native := s.native
		defer func() {
			delete(views, native)
			native.Send(selRelease)
		}()
		retained := func() uint { return uint(native.Send(sel("retainCount"))) }
		start := retained()
		drained(s.startTicking)
		if s.link == 0 {
			t.Skip("no display link before macOS 14")
		}
		if retained() <= start {
			t.Fatal("the display link does not hold its target")
		}
		drained(s.close)
		if s.link != 0 {
			t.Error("a closed surface keeps its display link")
		}
		if got := retained(); got != start {
			t.Errorf("a closed surface's view keeps retain count %d, want %d", got, start)
		}
	})
}
