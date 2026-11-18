package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestCursorPointReadsThePointerPositionAndAnswersIt guards the request and
// answer round trip: a mismatched Count, an EventCursorPoint that drops
// Point, or a different answer kind leaves done empty.
func TestCursorPointReadsThePointerPositionAndAnswersIt(t *testing.T) {
	var cx *Scope
	var got Point
	w := newWindows(t, func(c *Scope) {
		cx = c
		cx.CursorPoint(func(p Point) { got = p })
	})
	w.commit()
	asks := w.take(0, driver.ItemCursorPoint)
	if len(asks) != 1 {
		t.Fatalf("cursor asks %+v, want one ItemCursorPoint", asks)
	}
	w.dispatch(0, driver.Event{Kind: driver.EventCursorPoint, Count: asks[0].Count, Point: Point{X: 12, Y: 34}})
	if got != (Point{X: 12, Y: 34}) {
		t.Fatalf("cursor = %+v, want (12, 34)", got)
	}
}

// TestCursorPointAnswersZeroWhereThePlatformOpensNoWindow guards the gate:
// a host without windows must not send an ItemCursorPoint, and done hears
// the zero Point.
func TestCursorPointAnswersZeroWhereThePlatformOpensNoWindow(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	var got Point
	cx.CursorPoint(func(p Point) { got = p })
	if b := h.commit(); command(b, driver.ItemCursorPoint) != nil {
		t.Fatal("a windowless platform was sent an ItemCursorPoint")
	}
	if got != (Point{}) {
		t.Fatalf("cursor = %+v, want the zero Point", got)
	}
}
