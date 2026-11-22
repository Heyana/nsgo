package harmonyos

import (
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func nearPoint(a, b driver.Point) bool {
	return math.Abs(float64(a.X-b.X)) < 0.05 && math.Abs(float64(a.Y-b.Y)) < 0.05
}

func TestTouchSceneNestedBoundTransforms(t *testing.T) {
	anim := driver.NewAnimator(func(driver.Event) {})
	scene := newTouchScene(anim)

	// Mirrors TestTouchMovesFollowNestedBoundTransforms: parent (2) at (0,0) 160x120 with origin top-left, bound TranslateX=100, TranslateY=40, Scale=1.5; child (3) at (20,10) 80x60 with origin top-left, bound Rotate=90 deg CW.
	scene.applyBatch(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 2},
		{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 400, Height: 300}}},
		{Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{Size: driver.Size{Width: 160, Height: 120}}},
		{Kind: driver.ItemFrame, ID: 3, Frame: driver.Bounds{Origin: driver.Point{X: 20, Y: 10}, Size: driver.Size{Width: 80, Height: 60}}},
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

	// Surface (100, 85) maps to child local (20, 20).
	start := driver.Event{
		Kind:  driver.EventTouch,
		Phase: driver.GestureBegan,
		Touches: []driver.Touch{
			{ID: 0, PagePosition: driver.Point{X: 100, Y: 85}, Changed: true},
		},
	}
	scene.captureTouches(&start)
	if start.TouchGeometry == nil {
		t.Fatal("start.TouchGeometry = nil")
	}
	if got := start.TouchGeometry.Targets[0]; got != 3 {
		t.Fatalf("target = %d, want 3", got)
	}
	if got := start.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 20, Y: 20}) {
		t.Fatalf("child start local = %v, want (20, 20)", got)
	}
	if got := start.TouchGeometry.Positions[1]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 100, Y: 85}) {
		t.Fatalf("root start local = %v, want (100, 85)", got)
	}

	// Move to surface (85, 130) -> child local (50, 30) (outside parent's unrotated 0..160 bounds).
	move := driver.Event{
		Kind:  driver.EventTouch,
		Phase: driver.GestureChanged,
		Touches: []driver.Touch{
			{ID: 0, PagePosition: driver.Point{X: 85, Y: 130}, Changed: true},
		},
	}
	scene.captureTouches(&move)
	if got := move.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 50, Y: 30}) {
		t.Fatalf("child move local = %v, want (50, 30)", got)
	}

	// Animate TranslateX from 100 -> 130 mid-gesture: snapshot follows the live bound value.
	anim.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 10, Value: 130}})
	end := driver.Event{
		Kind:  driver.EventTouch,
		Phase: driver.GestureEnded,
		Touches: []driver.Touch{
			{ID: 0, PagePosition: driver.Point{X: 115, Y: 130}, Changed: true},
		},
	}
	scene.captureTouches(&end)
	if got := end.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 50, Y: 30}) {
		t.Fatalf("child end local after bound update = %v, want (50, 30)", got)
	}
	// Earlier snapshot remains unchanged.
	if got := start.TouchGeometry.Positions[3][0]; !nearPoint(got, driver.Point{X: 20, Y: 20}) {
		t.Fatalf("start snapshot mutated to %v", got)
	}
}

func TestTouchSceneScrollZoomStickyAndMultiTouch(t *testing.T) {
	scene := newTouchScene(nil)
	scene.applyBatch(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindScroll},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 2},
		{Kind: driver.ItemInsert, ID: 4, Parent: 1},
		{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 400, Height: 400}}},
		{Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{Origin: driver.Point{X: 0, Y: 0}, Size: driver.Size{Width: 300, Height: 200}}},
		{Kind: driver.ItemFrame, ID: 3, Frame: driver.Bounds{Origin: driver.Point{X: 50, Y: 100}, Size: driver.Size{Width: 100, Height: 60}}},
		{Kind: driver.ItemFrame, ID: 4, Frame: driver.Bounds{Origin: driver.Point{X: 0, Y: 250}, Size: driver.Size{Width: 300, Height: 100}}},
		{Kind: driver.ItemScrollConfig, ID: 2, Scroll: &driver.ScrollConfig{MinZoomScale: 1, MaxZoomScale: 4, ZoomScale: 2}},
		{Kind: driver.ItemEvents, ID: 3, Events: 1 << driver.EventTouch},
		{Kind: driver.ItemEvents, ID: 4, Events: 1 << driver.EventTouch},
	}})

	// Scroll offset (20, 40) at zoom 2x: child 3 origin (50, 100) appears at (50*2 - 20, 100*2 - 40) = (80, 160). Surface (100, 180) maps to child 3 local ((100+20)/2 - 50, (180+40)/2 - 100) = (10, 10).
	offset := driver.Point{X: 20, Y: 40}
	scene.setZoom(2, 2, &offset)

	ev1 := driver.Event{
		Kind:  driver.EventTouch,
		Phase: driver.GestureBegan,
		Time:  10 * time.Millisecond,
		Touches: []driver.Touch{
			{ID: 1, PagePosition: driver.Point{X: 100, Y: 180}, Changed: true},
		},
	}
	scene.captureTouches(&ev1)
	if got := ev1.TouchGeometry.Targets[1]; got != 3 {
		t.Fatalf("finger 1 target = %d, want 3", got)
	}
	if got := ev1.TouchGeometry.Positions[3]; len(got) != 1 || !nearPoint(got[0], driver.Point{X: 10, Y: 10}) {
		t.Fatalf("child 3 local = %v, want (10, 10)", got)
	}

	// Second finger goes down on view 4 while finger 1 stays down.
	ev2 := driver.Event{
		Kind:  driver.EventTouch,
		Phase: driver.GestureBegan,
		Time:  20 * time.Millisecond,
		Touches: []driver.Touch{
			{ID: 1, PagePosition: driver.Point{X: 100, Y: 180}},
			{ID: 2, PagePosition: driver.Point{X: 30, Y: 270}, Changed: true},
		},
	}
	scene.captureTouches(&ev2)
	if got := ev2.TouchGeometry.Targets[2]; got != 4 {
		t.Fatalf("finger 2 target = %d, want 4", got)
	}
	if len(ev2.TouchGeometry.Positions[3]) != 2 || len(ev2.TouchGeometry.Positions[4]) != 2 {
		t.Fatalf("multi-touch positions = %+v", ev2.TouchGeometry.Positions)
	}

	// Finger 1 lifts while finger 2 stays down; view 3 stays tracked for any responder on its path.
	ev3 := driver.Event{
		Kind:  driver.EventTouch,
		Phase: driver.GestureEnded,
		Time:  30 * time.Millisecond,
		Touches: []driver.Touch{
			{ID: 1, PagePosition: driver.Point{X: 100, Y: 180}, Changed: true},
			{ID: 2, PagePosition: driver.Point{X: 30, Y: 270}},
		},
	}
	scene.captureTouches(&ev3)
	ev4 := driver.Event{
		Kind:  driver.EventTouch,
		Phase: driver.GestureChanged,
		Time:  40 * time.Millisecond,
		Touches: []driver.Touch{
			{ID: 2, PagePosition: driver.Point{X: 40, Y: 280}, Changed: true},
		},
	}
	scene.captureTouches(&ev4)
	if len(ev4.TouchGeometry.Positions[3]) != 1 || len(ev4.TouchGeometry.Positions[4]) != 1 {
		t.Fatalf("responder view 3 dropped while finger 2 remained: %+v", ev4.TouchGeometry.Positions)
	}

	// Surface point clipped outside scroll viewport (y=210 is below scroll height 200) must not hit child 3.
	if h := scene.hit(scene.nodes[1], driver.Point{X: 100, Y: 210}); h == nil || h.id != 1 {
		t.Fatalf("clipped point hit %v, want root 1", h)
	}
}

func TestTouchScenePointerEventsAndHidden(t *testing.T) {
	scene := newTouchScene(nil)
	scene.applyBatch(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 2},
		{Kind: driver.ItemInsert, ID: 4, Parent: 1},
		{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 200, Height: 200}}},
		{Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 100, Height: 100}}},
		{Kind: driver.ItemFrame, ID: 3, Frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 40, Height: 40}}},
		{Kind: driver.ItemFrame, ID: 4, Frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 100, Height: 100}}},
		{Kind: driver.ItemViewProps, ID: 2, ViewProps: &driver.ViewProps{PointerEvents: driver.PointerEventsBoxNone}},
		{Kind: driver.ItemViewProps, ID: 4, ViewProps: &driver.ViewProps{PointerEvents: driver.PointerEventsNone}},
	}})

	// View 4 (on top) has PointerEventsNone, view 2 has BoxNone, so (30, 30) hits child 3 and (80, 80) falls through to root 1.
	if h := scene.hit(scene.nodes[1], driver.Point{X: 30, Y: 30}); h == nil || h.id != 3 {
		t.Fatalf("hit (30,30) = %v, want 3", h)
	}
	if h := scene.hit(scene.nodes[1], driver.Point{X: 80, Y: 80}); h == nil || h.id != 1 {
		t.Fatalf("hit (80,80) = %v, want 1", h)
	}
}
