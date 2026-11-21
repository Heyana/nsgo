package viewtree

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type testNode struct {
	origin, offset driver.Point
	hidden         bool
	transparent    bool
	events         driver.PointerEvents
	bar            driver.Bounds
	children       []*testNode
}

func (v *testNode) PointerVisibility() (bool, bool) { return v.hidden, v.transparent }

func (v *testNode) PointerReach(p driver.Point) (driver.Point, bool, bool) {
	local := driver.Point{X: p.X - v.origin.X, Y: p.Y - v.origin.Y}
	size := driver.Size{Width: 100, Height: 100}
	self, children := driver.ReachView(size, &driver.ViewProps{PointerEvents: v.events}, driver.Bounds{}, false, local)
	return local, self, children
}

func (v *testNode) PointerChildren(local driver.Point) (driver.Point, []*testNode) {
	return driver.Point{X: local.X + v.offset.X, Y: local.Y + v.offset.Y}, v.children
}

func (v *testNode) ScrollbarContains(local driver.Point) bool { return v.bar.Contains(local) }

func TestHitSkipsUnmatchedViewsAndCanReachTransparentViews(t *testing.T) {
	back, front := &testNode{}, &testNode{}
	root := &testNode{children: []*testNode{back, front}}
	p := driver.Point{X: 10, Y: 10}
	always := func(*testNode) bool { return true }

	if got := Hit(root, p, func(v *testNode) bool { return v == back }); got != back {
		t.Fatal("unmatched front view blocked the press")
	}
	front.transparent = true
	if got := Hit(root, p, always); got != front {
		t.Fatal("transparent front view could not take the press")
	}
	front.hidden = true
	if got := Hit(root, p, always); got != back {
		t.Fatal("hidden front view blocked the press")
	}
}

func TestViewsAtUsesFrontmostPointerPath(t *testing.T) {
	back, front := &testNode{}, &testNode{}
	root := &testNode{children: []*testNode{back, front}}
	p := driver.Point{X: 10, Y: 10}
	match := func(v *testNode) bool { return v != front }

	if got, took := ViewsAt(root, p, match, nil); !took || !slices.Equal(got, []*testNode{root}) {
		t.Fatal("unmatched front view did not occlude the back view")
	}
	front.transparent = true
	if got, took := ViewsAt(root, p, match, nil); !took || !slices.Equal(got, []*testNode{root, back}) {
		t.Fatal("transparent front view occluded the back view")
	}
	front.transparent = false
	front.events = driver.PointerEventsBoxNone
	child := &testNode{origin: driver.Point{X: 30, Y: 20}}
	front.children = []*testNode{child}
	if got, took := ViewsAt(root, p, match, nil); !took || !slices.Equal(got, []*testNode{root, back}) {
		t.Fatal("a child outside the point blocked the back view")
	}
	front.offset = driver.Point{X: 30, Y: 20}
	if got, took := ViewsAt(root, p, match, nil); !took || !slices.Equal(got, []*testNode{root, child}) {
		t.Fatal("the path did not follow the scrolled child")
	}
}

func TestBarAtUsesLocalCoordinatesAndRespectsOcclusion(t *testing.T) {
	back := &testNode{
		origin: driver.Point{X: 20, Y: 40},
		bar: driver.Bounds{
			Origin: driver.Point{X: 6},
			Size:   driver.Size{Width: 4, Height: 100},
		},
	}
	front := &testNode{}
	root := &testNode{
		origin:   driver.Point{X: 12, Y: 21},
		offset:   driver.Point{X: 20, Y: 40},
		children: []*testNode{back, front},
	}
	p := driver.Point{X: 20, Y: 41}
	local := driver.Point{X: 8, Y: 20}

	if got, at := BarAt(root, p); got != nil || at != p {
		t.Fatalf("occluded scrollbar hit = (%p, %v), want no hit at %v", got, at, p)
	}
	root.bar = back.bar
	if got, at := BarAt(root, p); got != root || at != local {
		t.Fatalf("outer scrollbar hit = (%p, %v), want root at %v", got, at, local)
	}
	front.events = driver.PointerEventsNone
	if got, at := BarAt(root, p); got != back || at != local {
		t.Fatalf("inner scrollbar hit = (%p, %v), want back at %v", got, at, local)
	}
}
