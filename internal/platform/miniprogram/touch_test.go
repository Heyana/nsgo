package miniprogram

import (
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func nearPoint(a, b driver.Point) bool {
	return math.Abs(float64(a.X-b.X)) < 0.05 && math.Abs(float64(a.Y-b.Y)) < 0.05
}

func TestTouchGeometryFollowsNestedBoundTransforms(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	// Mirrors TestTouchMovesFollowNestedBoundTransforms: view 2 at (0,0) 160x120 origin top-left, bound TranslateX=100, TranslateY=40, Scale=1.5; view 3 at (20,10) 80x60 origin top-left, bound Rotate=90 deg CW.
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 2},
		frame(1, 0, 0, 400, 300),
		frame(2, 0, 0, 160, 120),
		frame(3, 20, 10, 80, 60),
		{Kind: driver.ItemViewStyle, ID: 2, ViewStyle: &driver.ViewStyle{Opacity: 1, Origin: driver.Point{X: -80, Y: -60}}},
		{Kind: driver.ItemViewStyle, ID: 3, ViewStyle: &driver.ViewStyle{Opacity: 1, Origin: driver.Point{X: -40, Y: -30}}},
		{Kind: driver.ItemOverflow, ID: 1, Transformed: true},
		{Kind: driver.ItemOverflow, ID: 2, Transformed: true},
		{Kind: driver.ItemEvents, ID: 1, Events: 1 << driver.EventTouch},
		{Kind: driver.ItemEvents, ID: 3, Events: 1 << driver.EventTouch},
		{Kind: driver.ItemAnimation, Animation: []driver.AnimationOp{
			{Kind: driver.AnimCreateValue, Node: 10, Value: 100},
			{Kind: driver.AnimCreateValue, Node: 11, Value: 40},
			{Kind: driver.AnimCreateValue, Node: 12, Value: 1.5},
			{Kind: driver.AnimCreateValue, Node: 13, Value: 90},
			{Kind: driver.AnimBind, View: 2, Prop: driver.PropTranslateX, Node: 10},
			{Kind: driver.AnimBind, View: 2, Prop: driver.PropTranslateY, Node: 11},
			{Kind: driver.AnimBind, View: 2, Prop: driver.PropScale, Node: 12},
			{Kind: driver.AnimBind, View: 3, Prop: driver.PropRotate, Node: 13},
		}},
	}})

	f := func(id int, x, y driver.Points) []finger {
		return []finger{{id: id, page: driver.Point{X: x, Y: y}}}
	}
	// Surface (100, 85) -> child 3 local (20, 20).
	m.touch(1, 1, driver.GestureBegan, f(0, 100, 85), f(0, 100, 85), 10*time.Millisecond)
	// Surface (85, 130) -> child 3 local (50, 30) (outside parent's layout bounds).
	m.touch(1, 2, driver.GestureChanged, f(0, 85, 130), f(0, 85, 130), 26*time.Millisecond)

	// Move bound TranslateX from 100 -> 130 mid-gesture.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemAnimation, Animation: []driver.AnimationOp{
			{Kind: driver.AnimSet, Node: 10, Value: 130},
		}},
	}})
	m.touch(1, 3, driver.GestureEnded, f(0, 115, 130), nil, 42*time.Millisecond)

	if len(h.events) != 3 {
		t.Fatalf("events = %d, want 3: %+v", len(h.events), h.events)
	}
	start, move, end := h.events[0], h.events[1], h.events[2]
	if start.TouchGeometry == nil || start.TouchGeometry.Targets[0] != 3 {
		t.Fatalf("start target = %+v, want 3", start.TouchGeometry)
	}
	if got := start.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 20, Y: 20}) {
		t.Fatalf("start child local = %v, want (20, 20)", got)
	}
	if got := start.TouchGeometry.Positions[1]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 100, Y: 85}) {
		t.Fatalf("start root local = %v, want (100, 85)", got)
	}
	if got := move.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 50, Y: 30}) {
		t.Fatalf("move child local = %v, want (50, 30)", got)
	}
	if got := end.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 50, Y: 30}) {
		t.Fatalf("end child local after bound update = %v, want (50, 30)", got)
	}
}

func TestTouchGeometryMultiTouchScrollStickyAndBlockNative(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindScroll},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 2},
		{Kind: driver.ItemInsert, ID: 4, Parent: 1},
		frame(1, 0, 0, 375, 600),
		frame(2, 0, 0, 375, 300),
		frame(3, 20, 100, 100, 50),
		frame(4, 20, 350, 100, 50),
		{Kind: driver.ItemStickyCells, ID: 2, Sticky: []driver.StickyCell{{Cell: 3, Start: 50, End: 250}}},
		{Kind: driver.ItemEvents, ID: 3, Events: 1 << driver.EventTouch},
		{Kind: driver.ItemEvents, ID: 4, Events: 1 << driver.EventTouch},
	}})

	// Scroll to y=80: sticky cell 3 shifts down by 30 (80-50), so its top in viewport is 100+30-80 = 50.
	m.scrolled(2, driver.Point{Y: 80})
	h.events = nil

	f1 := finger{id: 1, page: driver.Point{X: 40, Y: 65}}
	f2 := finger{id: 2, page: driver.Point{X: 50, Y: 370}}
	m.touch(3, 1, driver.GestureBegan, []finger{f1}, []finger{f1}, 10*time.Millisecond)
	m.touch(4, 2, driver.GestureBegan, []finger{f2}, []finger{f1, f2}, 20*time.Millisecond)
	m.touch(3, 3, driver.GestureEnded, []finger{f1}, []finger{f2}, 30*time.Millisecond)
	m.touch(4, 4, driver.GestureChanged, []finger{{id: 2, page: driver.Point{X: 60, Y: 380}}}, []finger{{id: 2, page: driver.Point{X: 60, Y: 380}}}, 40*time.Millisecond)

	if len(h.events) != 4 {
		t.Fatalf("events = %d, want 4", len(h.events))
	}
	if got := h.events[0].TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 20, Y: 15}) {
		t.Fatalf("sticky cell 3 local = %v, want (20, 15)", got)
	}
	if got := h.events[1].TouchGeometry.Targets[2]; got != 4 {
		t.Fatalf("second finger target = %d, want 4", got)
	}
	// After finger 1 lifts while finger 2 stays down, view 3 remains in Positions for a responder.
	if len(h.events[3].TouchGeometry.Positions[3]) != 1 || len(h.events[3].TouchGeometry.Positions[4]) != 1 {
		t.Fatalf("after first finger lift, positions = %+v", h.events[3].TouchGeometry.Positions)
	}

	// ItemBlockNative toggles touchCapture (16) alongside touchFollow (8) -> 24, then back to 8.
	h.batches = nil
	m.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemBlockNative, ID: 3}}})
	m.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemBlockNative, ID: 0}}})
	if got := h.ops("e"); len(got) != 2 || got[0] != "[3,24]" || got[1] != "[3,8]" {
		t.Fatalf("block native ops = %v, want [[3,24] [3,8]]", got)
	}
}
