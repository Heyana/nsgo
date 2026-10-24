package finger

import (
	"testing"

	"github.com/oneo-me/nsgo/core"
)

func TestMouseIsFingerZeroWhileAViewListens(t *testing.T) {
	var m Mouse
	if _, ok := m.Down(false, core.Point{}); ok {
		t.Fatal("a press reported while no view listens")
	}
	if _, ok := m.Move(core.Point{X: 1}); ok {
		t.Fatal("a move reported after an unreported press")
	}
	ev, ok := m.Down(true, core.Point{X: 5, Y: 6})
	if !ok || ev.Phase != core.GestureBegan || ev.Touches[0].PagePosition != (core.Point{X: 5, Y: 6}) || !ev.Touches[0].Changed {
		t.Fatalf("down = %+v, %v", ev, ok)
	}
	if ev, ok := m.Move(core.Point{X: 7}); !ok || ev.Phase != core.GestureChanged {
		t.Fatalf("move = %+v, %v", ev, ok)
	}
	if ev, ok := m.Up(core.Point{X: 7}); !ok || ev.Phase != core.GestureEnded {
		t.Fatalf("up = %+v, %v", ev, ok)
	}
	if _, ok := m.Up(core.Point{}); ok {
		t.Fatal("a second up reported")
	}
}
