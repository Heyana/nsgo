//go:build darwin && !ios

package apple

import (
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestLayoutGhostFadesAndLeavesItsSuperview(t *testing.T) {
	onMain(func() {
		makeView := func() objc.ID { return objc.ID(Class("NSView")).Send(Sel("new")) }
		parent, deleted, descendant := makeView(), makeView(), makeView()
		defer parent.Send(layoutRelease)
		defer deleted.Send(layoutRelease)
		defer descendant.Send(layoutRelease)
		parent.Send(Sel("addSubview:"), deleted)
		deleted.Send(Sel("addSubview:"), descendant)
		deleted.Send(Sel("setWantsLayer:"), true)
		layer := deleted.Send(layoutLayer)
		layer.Send(layoutSetOpacity, float32(0.6))

		transition := &driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: 100 * time.Millisecond}
		var ghosts []*LayoutGhost
		disabled, ticks := 0, 0
		host := LayoutHost[objc.ID]{
			Ghosts:  &ghosts,
			Native:  func(v objc.ID) objc.ID { return v },
			Framed:  func(objc.ID) bool { return true },
			Now:     func() time.Duration { return 0 },
			Disable: func(objc.ID) { disabled++ },
			Tick:    func() { ticks++ },
		}
		batch := &LayoutBatch{Animation: &driver.LayoutAnimation{Delete: transition}, Deleted: map[objc.ID]bool{}}
		if !host.Vanish(deleted, batch) || host.Vanish(descendant, batch) || disabled != 1 || ticks != 1 {
			t.Fatal("the exit did not retain only the deleted subtree root")
		}
		if !InLayoutGhost(ghosts, descendant) {
			t.Error("the retained exit did not keep its descendants")
		}
		ghosts = StepLayoutMotions[int](50*time.Millisecond, nil, ghosts, nil, nil, nil)
		if got := objc.Send[float32](layer, layoutOpacity); len(ghosts) != 1 || got < 0.299 || got > 0.301 {
			t.Errorf("midpoint has %d exits and opacity %v, want one exit at 0.3", len(ghosts), got)
		}
		ghosts = StepLayoutMotions[int](100*time.Millisecond, nil, ghosts, nil, nil, nil)
		if len(ghosts) != 0 || deleted.Send(layoutSuperview) != 0 {
			t.Error("the completed exit is still attached")
		}
	})
}

func TestLayoutMatchUsesTheNewParentAndKeepsTheDestination(t *testing.T) {
	onMain(func() {
		makeView := func() objc.ID { return objc.ID(Class("NSView")).Send(Sel("new")) }
		parent, oldParent, old, next := makeView(), makeView(), makeView(), makeView()
		defer parent.Send(layoutRelease)
		defer oldParent.Send(layoutRelease)
		defer old.Send(layoutRelease)
		defer next.Send(layoutRelease)
		parent.Send(Sel("setFrame:"), Rect(0, 0, 300, 300))
		oldParent.Send(Sel("setFrame:"), Rect(40, 30, 100, 100))
		parent.Send(Sel("addSubview:"), oldParent)
		oldParent.Send(Sel("addSubview:"), old)
		parent.Send(Sel("addSubview:"), next)
		old.Send(Sel("setFrame:"), Rect(10, 20, 20, 25))
		next.Send(Sel("setFrame:"), Rect(100, 80, 40, 50))
		moved := driver.Bounds{}
		destination := driver.Bounds{Origin: driver.Point{X: 98, Y: 77}, Size: driver.Size{Width: 40, Height: 50}}
		motions := map[objc.ID]*LayoutMotion{next: {Appear: &driver.LayoutTransition{}}}
		restyled, ticks := 0, 0
		host := LayoutHost[objc.ID]{
			Motions: motions,
			Native:  func(v objc.ID) objc.ID { return v },
			Framed:  func(objc.ID) bool { return true },
			Frame:   func(objc.ID) driver.Bounds { return destination },
			Shift:   func(objc.ID) driver.Point { return driver.Point{X: 2, Y: 3} },
			Place:   func(_ objc.ID, frame driver.Bounds) { moved = frame },
			Restyle: func(objc.ID) { restyled++ },
			Start:   func(*driver.LayoutTransition) time.Duration { return 7 * time.Millisecond },
			Tick:    func() { ticks++ },
		}
		transition := &driver.LayoutTransition{Curve: driver.LayoutLinear, Duration: time.Second}
		host.Match(next, old, &LayoutBatch{Animation: &driver.LayoutAnimation{Update: transition}})
		m := motions[next]
		want := driver.Bounds{Origin: driver.Point{X: 48, Y: 47}, Size: driver.Size{Width: 20, Height: 25}}
		if m.Frame == nil || m.Frame.From() != want || moved != want || m.Frame.Target() != destination || m.Appear != nil || m.Frame.Transition() != *transition {
			t.Fatalf("match = %+v, moved %+v; want from %+v to %+v", m, moved, want, destination)
		}
		if shown, _ := m.Frame.Step(507 * time.Millisecond); shown.Origin.X != 73 {
			t.Errorf("halfway shows x %v; the match did not start on the host's transition clock", shown.Origin.X)
		}
		if !objc.Send[bool](old, Sel("isHidden")) || restyled != 1 || ticks != 1 {
			t.Error("the matched predecessor remained visible or the replacement did not start")
		}
	})
}
