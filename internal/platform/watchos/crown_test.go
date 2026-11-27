package watchos

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func makeCrownNode(id driver.NodeID, kind driver.ViewKind, parent *watchNode, frame driver.Bounds, events driver.EventMask, props driver.ViewProps) *watchNode {
	n := &watchNode{
		id:     id,
		hostID: id,
		kind:   kind,
		parent: parent,
		frame:  frame,
		style:  driver.DefaultViewStyle,
		events: events,
		props:  props,
	}
	if parent != nil {
		parent.children = append(parent.children, n)
	}
	return n
}

func testCrownCenter(n *watchNode) (local, page driver.Point) {
	f := n.shownFrame()
	local = driver.Point{X: f.Size.Width / 2, Y: f.Size.Height / 2}
	page = driver.Point{X: f.Origin.X + local.X, Y: f.Origin.Y + local.Y}
	return local, page
}

func incDecActions() *[]driver.AccessibilityAction {
	a := []driver.AccessibilityAction{{Name: driver.ActionIncrement}, {Name: driver.ActionDecrement}}
	return &a
}

func TestCrownTargetResolutionAndPriority(t *testing.T) {
	full := driver.Bounds{Size: driver.Size{Width: 184, Height: 224}}
	box := driver.Bounds{Origin: driver.Point{X: 12, Y: 20}, Size: driver.Size{Width: 160, Height: 44}}
	adjProps := driver.ViewProps{
		Role:    driver.RoleAdjustable,
		Actions: incDecActions(),
	}
	spinProps := driver.ViewProps{
		Role:    driver.RoleSpinButton,
		Actions: incDecActions(),
	}

	// 1. Single adjustable value-turning view takes the crown automatically.
	root := makeCrownNode(1, driver.KindView, nil, full, 0, driver.ViewProps{})
	slider := makeCrownNode(2, driver.KindView, root, box, 1<<driver.EventAccessibilityAction, adjProps)
	if got, kind := resolveCrownTarget(root, nil, 0, nil); got != slider || kind != crownValue {
		t.Fatalf("single slider target = (%v, %v), want (%v, crownValue)", got, kind, slider)
	}

	// 2. Scrollable ScrollView keeps the Digital Crown unless a view has explicit crown focus.
	scroll := makeCrownNode(3, driver.KindScroll, root, full, 1<<driver.EventScroll, driver.ViewProps{})
	scroll.content = driver.Size{Width: 184, Height: 600}
	if got, kind := resolveCrownTarget(root, nil, 0, nil); got != nil || kind != crownNone {
		t.Fatalf("with scrollable ScrollView target = (%v, %v), want (nil, crownNone)", got, kind)
	}

	// Explicit crown focus on slider overrides ScrollView.
	if got, kind := resolveCrownTarget(root, nil, slider.id, nil); got != slider || kind != crownValue {
		t.Fatalf("focused slider over ScrollView = (%v, %v), want (%v, crownValue)", got, kind, slider)
	}

	// Focused spinbutton input takes highest priority over both crownFocus and ScrollView.
	spin := makeCrownNode(4, driver.KindInput, root, box, 1<<driver.EventAccessibilityAction, spinProps)
	if got, kind := resolveCrownTarget(root, spin, slider.id, nil); got != spin || kind != crownValue {
		t.Fatalf("focused spinbutton = (%v, %v), want (%v, crownValue)", got, kind, spin)
	}

	// Disabled spinbutton falls back to crownFocus.
	if got, kind := resolveCrownTarget(root, spin, slider.id, func(n *watchNode) bool { return n == spin }); got != slider || kind != crownValue {
		t.Fatalf("disabled spinbutton fallback = (%v, %v), want (%v, crownValue)", got, kind, slider)
	}

	// Non-overflowing ScrollView does not suppress crown-eligible views.
	scroll.content = driver.Size{Width: 184, Height: 224}
	if got, kind := resolveCrownTarget(root, nil, 0, nil); got != slider || kind != crownValue {
		t.Fatalf("non-overflowing scroll = (%v, %v), want (%v, crownValue)", got, kind, slider)
	}

	// 3. Modal layer isolates the crown from background scrollable views and background focus.
	scroll.content = driver.Size{Width: 184, Height: 600}
	modal := makeCrownNode(5, driver.KindModal, root, full, 0, driver.ViewProps{})
	modalPan := makeCrownNode(6, driver.KindView, modal, box, 1<<driver.EventPan, driver.ViewProps{
		PanActivation: driver.PanActivation{ActiveOffsetX: 4},
	})
	if got, kind := resolveCrownTarget(root, spin, slider.id, nil); got != modalPan || kind != crownPan {
		t.Fatalf("modal pan target = (%v, %v), want (%v, crownPan)", got, kind, modalPan)
	}

	// 4. Hidden, disabled, PointerEventsNone, culled, deleted or leaving ghosts are excluded.
	modalPan.retired = true
	if got, kind := resolveCrownTarget(root, nil, modalPan.id, nil); got != nil || kind != crownNone {
		t.Fatalf("retired leaving ghost took crown: (%v, %v)", got, kind)
	}
	modalPan.retired = false
	modalPan.props.Disabled = true
	if got, kind := resolveCrownTarget(root, nil, modalPan.id, nil); got != nil || kind != crownNone {
		t.Fatalf("disabled view took crown: (%v, %v)", got, kind)
	}
	modalPan.props.Disabled = false
	modalPan.props.PointerEvents = driver.PointerEventsNone
	if got, kind := resolveCrownTarget(root, nil, modalPan.id, nil); got != nil || kind != crownNone {
		t.Fatalf("PointerEventsNone view took crown: (%v, %v)", got, kind)
	}
	modalPan.props.PointerEvents = driver.PointerEventsAuto
	modalPan.style.Hidden = true
	if got, kind := resolveCrownTarget(root, nil, modalPan.id, nil); got != nil || kind != crownNone {
		t.Fatalf("hidden view took crown: (%v, %v)", got, kind)
	}
	modalPan.style.Hidden = false
	modalPan.culled = true
	if got, kind := resolveCrownTarget(root, nil, modalPan.id, nil); got != nil || kind != crownNone {
		t.Fatalf("culled view took crown: (%v, %v)", got, kind)
	}

	// 5. EventWheel takes priority over EventPan on the same screen without explicit focus.
	wheelRoot := makeCrownNode(10, driver.KindView, nil, full, 1<<driver.EventWheel, driver.ViewProps{})
	_ = makeCrownNode(11, driver.KindView, wheelRoot, box, 1<<driver.EventPan, driver.ViewProps{})
	if got, kind := resolveCrownTarget(wheelRoot, nil, 0, nil); got != wheelRoot || kind != crownWheel {
		t.Fatalf("wheel over pan = (%v, %v), want (%v, crownWheel)", got, kind, wheelRoot)
	}
}

func TestCrownRouterWheelValueAndPan(t *testing.T) {
	full := driver.Bounds{Size: driver.Size{Width: 184, Height: 224}}
	box := driver.Bounds{Origin: driver.Point{X: 12, Y: 20}, Size: driver.Size{Width: 160, Height: 44}}

	t.Run("wheel", func(t *testing.T) {
		root := makeCrownNode(1, driver.KindView, nil, full, 1<<driver.EventWheel, driver.ViewProps{})
		var r crownRouter
		evs := r.handle(crownBegan, 0.5, 2.0, root, nil, nil, testCrownCenter)
		if len(evs) != 2 || evs[0].Kind != driver.EventWheel || evs[0].Phase != driver.GestureBegan || !evs[0].Precise {
			t.Fatalf("wheel began = %+v", evs)
		}
		if evs[1].Phase != driver.GestureChanged || evs[1].Delta.Y != driver.Points(0.5*crownWheelScale) || evs[1].Velocity.Y != driver.Points(2.0*crownWheelScale) {
			t.Fatalf("wheel first change = %+v", evs[1])
		}
		evs = r.handle(crownChanged, -0.25, -1.0, root, nil, nil, testCrownCenter)
		if len(evs) != 1 || evs[0].Phase != driver.GestureChanged || evs[0].Delta.Y != driver.Points(-0.25*crownWheelScale) {
			t.Fatalf("wheel second change = %+v", evs)
		}
		evs = r.handle(crownEnded, 0, 0, root, nil, nil, testCrownCenter)
		if len(evs) != 1 || evs[0].Kind != driver.EventWheel || evs[0].Phase != driver.GestureEnded || evs[0].ID != root.id {
			t.Fatalf("wheel end = %+v", evs)
		}
		// Extra idle after end emits nothing.
		if extra := r.handle(crownEnded, 0, 0, root, nil, nil, testCrownCenter); len(extra) != 0 {
			t.Fatalf("extra idle emitted %+v", extra)
		}
	})

	t.Run("value_detents", func(t *testing.T) {
		root := makeCrownNode(1, driver.KindView, nil, full, 0, driver.ViewProps{})
		slider := makeCrownNode(2, driver.KindView, root, box, 1<<driver.EventAccessibilityAction, driver.ViewProps{
			Role:    driver.RoleAdjustable,
			Actions: incDecActions(),
		})
		var r crownRouter
		// Sub-detent turns accumulate without firing prematurely.
		if evs := r.handle(crownBegan, 0.4, 1.0, root, nil, nil, testCrownCenter); len(evs) != 0 {
			t.Fatalf("0.4 turn emitted %+v, want none", evs)
		}
		if evs := r.handle(crownChanged, 0.4, 1.0, root, nil, nil, testCrownCenter); len(evs) != 0 {
			t.Fatalf("0.8 total emitted %+v, want none", evs)
		}
		// Crossing 1.0 fires one increment and keeps remainder 0.2.
		evs := r.handle(crownChanged, 0.4, 1.0, root, nil, nil, testCrownCenter)
		if len(evs) != 1 || evs[0].Kind != driver.EventAccessibilityAction || evs[0].ID != slider.id || evs[0].Text != driver.ActionIncrement {
			t.Fatalf("1.2 total = %+v, want single increment", evs)
		}
		// Large negative turn emits multiple decrements.
		evs = r.handle(crownChanged, -2.5, -5.0, root, nil, nil, testCrownCenter)
		if len(evs) != 2 || evs[0].Text != driver.ActionDecrement || evs[1].Text != driver.ActionDecrement {
			t.Fatalf("-2.3 total = %+v, want two decrements", evs)
		}
		// Idle clears residual accumulation.
		if evs := r.handle(crownEnded, 0, 0, root, nil, nil, testCrownCenter); len(evs) != 0 {
			t.Fatalf("value idle emitted %+v", evs)
		}
		if r.accum != 0 {
			t.Fatalf("accum after idle = %v, want 0", r.accum)
		}
		// NaN/Inf inputs are ignored safely.
		if evs := r.handle(crownBegan, math.NaN(), math.Inf(1), root, nil, nil, testCrownCenter); len(evs) != 0 {
			t.Fatalf("NaN turn emitted %+v", evs)
		}
	})

	t.Run("pan_axes_and_switch", func(t *testing.T) {
		root := makeCrownNode(1, driver.KindView, nil, full, 0, driver.ViewProps{})
		horiz := makeCrownNode(2, driver.KindView, root, box, 1<<driver.EventPan, driver.ViewProps{
			PanActivation: driver.PanActivation{ActiveOffsetX: 4},
		})
		vert := makeCrownNode(3, driver.KindView, root, box, 1<<driver.EventPan, driver.ViewProps{})
		var r crownRouter
		r.setFocus(horiz.id)

		evs := r.handle(crownBegan, 1.0, 2.0, root, nil, nil, testCrownCenter)
		if len(evs) != 2 || evs[0].Kind != driver.EventPan || evs[0].Phase != driver.GestureBegan {
			t.Fatalf("horiz pan began = %+v", evs)
		}
		if evs[1].Phase != driver.GestureChanged || evs[1].Point.X != driver.Points(crownPanScale) || evs[1].Point.Y != 0 {
			t.Fatalf("horiz pan first change = %+v, want X=%v Y=0", evs[1], crownPanScale)
		}
		evs = r.handle(crownChanged, 0.5, 1.0, root, nil, nil, testCrownCenter)
		if len(evs) != 1 || evs[0].Point.X != driver.Points(1.5*crownPanScale) {
			t.Fatalf("horiz pan cumulative = %+v, want X=%v", evs, 1.5*crownPanScale)
		}

		// Switching crown focus mid-gesture cleanly ends the old pan and begins the new vertical pan.
		r.setFocus(vert.id)
		evs = r.handle(crownChanged, 1.0, 3.0, root, nil, nil, testCrownCenter)
		if len(evs) != 3 {
			t.Fatalf("switch mid-gesture = %+v, want 3 events (end old, begin new, change new)", evs)
		}
		if evs[0].ID != horiz.id || evs[0].Phase != driver.GestureEnded {
			t.Fatalf("first event = %+v, want horiz GestureEnded", evs[0])
		}
		if evs[1].ID != vert.id || evs[1].Phase != driver.GestureBegan {
			t.Fatalf("second event = %+v, want vert GestureBegan", evs[1])
		}
		if evs[2].ID != vert.id || evs[2].Phase != driver.GestureChanged || evs[2].Point.X != 0 || evs[2].Point.Y != driver.Points(crownPanScale) {
			t.Fatalf("third event = %+v, want vert Y=%v", evs[2], crownPanScale)
		}

		// Forgetting the active node clears state so idle does not emit on a dead ID.
		r.forgetNode(vert.id)
		if evs := r.handle(crownEnded, 0, 0, root, nil, nil, testCrownCenter); len(evs) != 0 {
			t.Fatalf("idle after forgetNode emitted %+v", evs)
		}
	})
}

func TestCrownSyncModeEncoding(t *testing.T) {
	full := driver.Bounds{Size: driver.Size{Width: 184, Height: 224}}
	box := driver.Bounds{Size: driver.Size{Width: 100, Height: 40}}
	root := makeCrownNode(1, driver.KindView, nil, full, 0, driver.ViewProps{})
	scroll := makeCrownNode(2, driver.KindScroll, root, full, 1<<driver.EventScroll, driver.ViewProps{})
	scroll.content = driver.Size{Width: 184, Height: 500}
	slider := makeCrownNode(3, driver.KindView, root, box, 1<<driver.EventAccessibilityAction, driver.ViewProps{
		Role:    driver.RoleAdjustable,
		Actions: incDecActions(),
	})
	pan := makeCrownNode(4, driver.KindView, root, box, 1<<driver.EventPan, driver.ViewProps{})

	var r crownRouter
	var w writer

	// Initially scrollable ScrollView owns crown -> mode stays 0, no op emitted.
	if m := r.syncMode(&w, root, nil, nil); m != crownModeNone || len(w.buf) != 0 {
		t.Fatalf("initial syncMode = %d, buf=%v, want 0 and empty", m, w.buf)
	}

	// Focusing slider switches mode 0 -> 2 (detent).
	r.setFocus(slider.id)
	if m := r.syncMode(&w, root, nil, nil); m != crownModeDetent {
		t.Fatalf("slider syncMode = %d, want %d", m, crownModeDetent)
	}
	if len(w.buf) != 2 || w.buf[0] != opCrown || w.buf[1] != crownModeDetent {
		t.Fatalf("opCrown detent bytes = %v, want [%d %d]", w.buf, opCrown, crownModeDetent)
	}

	// Repeated sync with same mode writes nothing.
	w.buf = w.buf[:0]
	if m := r.syncMode(&w, root, nil, nil); m != crownModeDetent || len(w.buf) != 0 {
		t.Fatalf("unchanged syncMode wrote %v", w.buf)
	}

	// Focusing pan view switches mode 2 -> 1 (continuous).
	r.setFocus(pan.id)
	if m := r.syncMode(&w, root, nil, nil); m != crownModeContinuous || len(w.buf) != 2 || w.buf[1] != crownModeContinuous {
		t.Fatalf("pan syncMode = %d, buf=%v", m, w.buf)
	}

	// Clearing focus returns crown to ScrollView (mode 1 -> 0).
	w.buf = w.buf[:0]
	r.clearAllFocus()
	if m := r.syncMode(&w, root, nil, nil); m != crownModeNone || len(w.buf) != 2 || w.buf[1] != crownModeNone {
		t.Fatalf("clear focus syncMode = %d, buf=%v", m, w.buf)
	}
}
