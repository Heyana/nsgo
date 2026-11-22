package watchos

import (
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func nearPoint(a, b driver.Point) bool {
	return math.Abs(float64(a.X-b.X)) < 0.05 && math.Abs(float64(a.Y-b.Y)) < 0.05
}

func TestResponderTouchesLifecycleAndTransforms(t *testing.T) {
	type node struct {
		id     driver.NodeID
		parent *node
		frame  driver.Bounds
		style  driver.ViewStyle
		events driver.EventMask
		gone   bool
	}
	root := &node{
		id:     1,
		frame:  driver.Bounds{Size: driver.Size{Width: 400, Height: 300}},
		style:  driver.DefaultViewStyle,
		events: 1 << driver.EventTouch,
	}
	parent := &node{
		id:     2,
		parent: root,
		frame:  driver.Bounds{Size: driver.Size{Width: 160, Height: 120}},
		style: driver.ViewStyle{
			Opacity:   1,
			Origin:    driver.Point{X: -80, Y: -60},
			Transform: driver.Transform{TranslateX: 100, TranslateY: 40, Scale: 1.5},
		},
	}
	child := &node{
		id:     3,
		parent: parent,
		frame:  driver.Bounds{Origin: driver.Point{X: 20, Y: 10}, Size: driver.Size{Width: 80, Height: 60}},
		style: driver.ViewStyle{
			Opacity:   1,
			Origin:    driver.Point{X: -40, Y: -30},
			Transform: driver.Transform{Rotate: 90},
		},
		events: 1 << driver.EventTouch,
	}
	nodes := map[driver.NodeID]*node{1: root, 2: parent, 3: child}

	var localOf func(n *node, p driver.Point) driver.Point
	localOf = func(n *node, p driver.Point) driver.Point {
		if n.parent != nil {
			p = localOf(n.parent, p)
		}
		m := affinePointerMatrix(n.style, n.frame, driver.Keyframe{Opacity: 1, Scale: 1}, 1)
		local, _ := driver.IntoView(n.frame, m, p)
		return local
	}
	hitFn := func(p driver.Point) driver.NodeID {
		for _, n := range []*node{child, parent, root} {
			if n.gone {
				continue
			}
			at := localOf(n, p)
			if at.X >= 0 && at.Y >= 0 && at.X < n.frame.Size.Width && at.Y < n.frame.Size.Height {
				return n.id
			}
		}
		return 0
	}
	parentFn := func(id driver.NodeID) (driver.NodeID, driver.EventMask) {
		n := nodes[id]
		if n == nil || n.gone {
			return 0, 0
		}
		var up driver.NodeID
		if n.parent != nil {
			up = n.parent.id
		}
		return up, n.events
	}
	localFn := func(id driver.NodeID, p driver.Point) (driver.Point, bool) {
		n := nodes[id]
		if n == nil || n.gone {
			return driver.Point{}, false
		}
		return localOf(n, p), true
	}

	var r responderTouches

	// Nothing fires when no view listens to EventTouch.
	if _, ok := r.down(false, driver.Point{X: 100, Y: 85}, 1.0, hitFn, parentFn, localFn); ok {
		t.Fatal("down emitted EventTouch with listening=false")
	}
	if _, ok := r.move(driver.Point{X: 90, Y: 100}, 1.016, hitFn, parentFn, localFn); ok {
		t.Fatal("move emitted EventTouch without an active touch")
	}

	// Touch down at surface (100, 85) -> child local (20, 20).
	evStart, ok := r.down(true, driver.Point{X: 100, Y: 85}, 2.0, hitFn, parentFn, localFn)
	if !ok || evStart.Kind != driver.EventTouch || evStart.Phase != driver.GestureBegan || evStart.Time != 2*time.Second {
		t.Fatalf("start = %+v, ok=%v", evStart, ok)
	}
	if evStart.TouchGeometry == nil || evStart.TouchGeometry.Targets[0] != 3 {
		t.Fatalf("start target = %+v, want 3", evStart.TouchGeometry)
	}
	if got := evStart.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 20, Y: 20}) {
		t.Fatalf("start child local = %v, want (20, 20)", got)
	}
	if got := evStart.TouchGeometry.Positions[1]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 100, Y: 85}) {
		t.Fatalf("start root local = %v, want (100, 85)", got)
	}

	// Move to surface (85, 130) -> child local (50, 30).
	evMove, ok := r.move(driver.Point{X: 85, Y: 130}, 2.016, hitFn, parentFn, localFn)
	if !ok || evMove.Phase != driver.GestureChanged {
		t.Fatalf("move = %+v, ok=%v", evMove, ok)
	}
	if got := evMove.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 50, Y: 30}) {
		t.Fatalf("move child local = %v, want (50, 30)", got)
	}

	// Update bound translateX mid-gesture from 100 -> 130 before lifting.
	parent.style.Transform.TranslateX = 130
	evEnd, ok := r.up(driver.Point{X: 115, Y: 130}, 2.032, false, hitFn, parentFn, localFn)
	if !ok || evEnd.Phase != driver.GestureEnded {
		t.Fatalf("end = %+v, ok=%v", evEnd, ok)
	}
	if got := evEnd.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 50, Y: 30}) {
		t.Fatalf("end child local = %v, want (50, 30)", got)
	}
	// Earlier snapshot stays untouched.
	if got := evStart.TouchGeometry.Positions[3][0]; !nearPoint(got, driver.Point{X: 20, Y: 20}) {
		t.Fatalf("start snapshot mutated to %v", got)
	}

	// Scroll view taking the drag cancels the active touch and silences later moves/ups.
	parent.style.Transform.TranslateX = 100
	if _, ok := r.down(true, driver.Point{X: 100, Y: 85}, 3.0, hitFn, parentFn, localFn); !ok {
		t.Fatal("second down did not fire")
	}
	evCancel, ok := r.cancel(hitFn, parentFn, localFn)
	if !ok || evCancel.Phase != driver.GestureCancelled || len(evCancel.Touches) != 1 || !evCancel.Touches[0].Changed {
		t.Fatalf("cancel = %+v, ok=%v", evCancel, ok)
	}
	if _, ok := r.move(driver.Point{X: 95, Y: 90}, 3.016, hitFn, parentFn, localFn); ok {
		t.Fatal("move fired after cancel")
	}
	if _, ok := r.up(driver.Point{X: 95, Y: 90}, 3.032, false, hitFn, parentFn, localFn); ok {
		t.Fatal("up fired after cancel")
	}

	// System cancellation (cancelled=true) reports GestureCancelled and omits a deleted view cleanly.
	if _, ok := r.down(true, driver.Point{X: 100, Y: 85}, 4.0, hitFn, parentFn, localFn); !ok {
		t.Fatal("third down did not fire")
	}
	child.gone = true
	evSysCancel, ok := r.up(driver.Point{X: 100, Y: 85}, 4.02, true, hitFn, parentFn, localFn)
	if !ok || evSysCancel.Phase != driver.GestureCancelled {
		t.Fatalf("system cancel = %+v, ok=%v", evSysCancel, ok)
	}
	if _, hasChild := evSysCancel.TouchGeometry.Positions[3]; hasChild {
		t.Fatalf("deleted child remained in positions: %+v", evSysCancel.TouchGeometry.Positions)
	}
	if _, hasRoot := evSysCancel.TouchGeometry.Positions[1]; !hasRoot {
		t.Fatalf("root missing from positions: %+v", evSysCancel.TouchGeometry.Positions)
	}
}

func TestAffinePointerMatrixLayoutScaleAndNonAffineFallback(t *testing.T) {
	frame := driver.Bounds{Size: driver.Size{Width: 100, Height: 60}}
	m := affinePointerMatrix(driver.DefaultViewStyle, frame, driver.Keyframe{Opacity: 1, Scale: 1}, 0.5)
	// Point at center (50, 30) stays fixed; (75, 45) unprojects to (100, 60) under 0.5x around center.
	got, ok := driver.IntoView(frame, m, driver.Point{X: 75, Y: 45})
	if !ok || !nearPoint(got, driver.Point{X: 100, Y: 60}) {
		t.Fatalf("scaled unproject = %v (ok=%v), want (100, 60)", got, ok)
	}

	// Non-affine 3D perspective falls back to Identity3D, matching SwiftUI's transformEffect.
	persp := driver.ViewStyle{
		Opacity:   1,
		Transform: driver.Transform{Perspective: 300, RotateY: 45},
	}
	if got := affinePointerMatrix(persp, frame, driver.Keyframe{Opacity: 1, Scale: 1}, 1); got != driver.Identity3D {
		t.Fatalf("non-affine matrix = %v, want Identity3D", got)
	}
}
