package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func foldBounds(x, y, width, height core.Points) core.Bounds {
	return core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: width, Height: height}}
}

func TestFoldGallery(t *testing.T) {
	headless(t)
	app := startDesktop(t)
	open(app, "Foldable display")
	app.Find("id:fold-pane-0")
	if app.Exists("id:fold-pane-1") {
		t.Fatal("an ordinary display split into two panes")
	}
	app.Press("id:fold-add-0")
	if got := app.Find("id:fold-count-0").Text; got != "Count: 1" {
		t.Fatalf("count = %q", got)
	}
}

func TestFoldPageChangesPosture(t *testing.T) {
	headless(t)
	size := core.Size{Width: 800, Height: 600}
	var geometry *core.Signal[core.Geometry]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		geometry = core.NewSignalFunc(cx, driver.Divide(size, nil), core.Geometry.Equal)
		return foldPage(cx, geometry.Get)
	}, nsgotest.Size(size.Width, size.Height), nsgotest.Environment(core.Environment{Locale: "en-US"}))
	app.Press("id:fold-add-0")
	set := func(obstruction core.Obstruction) {
		app.Post(func() { geometry.Set(driver.Divide(size, []core.Obstruction{obstruction})) })
		app.Settle()
	}
	set(core.Obstruction{Frame: foldBounds(390, 0, 20, 600), Separating: true, Occluding: true})
	for i, want := range []core.Bounds{foldBounds(0, 0, 390, 600), foldBounds(410, 0, 390, 600)} {
		if got := app.Find(fmt.Sprintf("id:fold-pane-%d", i)).Frame; got != want {
			t.Fatalf("pane %d = %+v, want %+v", i, got, want)
		}
	}
	app.Press("id:fold-add-1")
	app.Press("id:fold-add-1")
	before := app.Find("id:fold-pane-1").ID
	app.Post(func() {
		g := geometry.GetUntracked()
		g.State, g.Angle, g.HasAngle = core.HingeHalfOpen, 1.5, true
		geometry.Set(g)
	})
	app.Settle()
	if app.Find("id:fold-pane-1").ID != before || app.Find("id:fold-state-1").Text != "Half open" {
		t.Fatal("a hinge reading without a layout change rebuilt the pane or missed its state")
	}
	set(core.Obstruction{Frame: foldBounds(0, 290, 800, 20), Horizontal: true, Separating: true})
	for i, want := range []core.Bounds{foldBounds(0, 0, 800, 290), foldBounds(0, 310, 800, 290)} {
		if got := app.Find(fmt.Sprintf("id:fold-pane-%d", i)).Frame; got != want {
			t.Fatalf("rotated pane %d = %+v, want %+v", i, got, want)
		}
	}
	app.Post(func() { geometry.Set(driver.Divide(size, nil)) })
	app.Settle()
	if app.Exists("id:fold-pane-1") || app.Find("id:fold-count-0").Text != "Count: 1" {
		t.Fatal("unfolding did not restore one pane with its original count")
	}
	set(core.Obstruction{Frame: foldBounds(400, 0, 0, 600), Separating: true})
	if got := app.Find("id:fold-count-1").Text; got != "Count: 2" {
		t.Fatalf("the second pane lost its count: %q", got)
	}
}

func TestFoldPanesRespectPageAndReservedArea(t *testing.T) {
	g := core.Geometry{Regions: []core.Region{
		{Frame: foldBounds(0, 0, 390, 600), SafeArea: core.Edges{Top: 30, Bottom: 20, Left: 10}},
		{Frame: foldBounds(410, 0, 390, 600), SafeArea: core.Edges{Right: 25}, Reserved: foldBounds(720, 0, 80, 600)},
	}}
	got := foldPaneFrames(g, foldBounds(220, 50, 580, 500))
	want := []core.Bounds{foldBounds(0, 0, 170, 500), foldBounds(190, 0, 310, 500)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("panes = %+v, want %+v", got, want)
	}
	if got := foldPaneFrames(g, foldBounds(0, 700, 800, 200)); len(got) != 0 {
		t.Fatalf("regions outside the page left panes: %+v", got)
	}
}

// TestFoldGeometryOnDevice reads the real backend without injecting geometry. Set NSGO_TEST_FOLD to open, half-open or closed after putting the simulator or device in that posture; missing hardware is a skip, an unexpected report is a failure. The same gallery page and input checks run on both platforms.
func TestFoldGeometryOnDevice(t *testing.T) {
	posture := os.Getenv("NSGO_TEST_FOLD")
	if posture == "" {
		t.Skip("needs foldable hardware in a known posture and NSGO_TEST_FOLD=open|half-open|closed")
	}
	if target := nsgotest.Target(); target != "android" && target != "ios_simulator" && target != "ios" {
		t.Fatalf("fold acceptance needs an Android or iOS target, got %q", target)
	}
	want, ok := map[string]core.HingeState{"open": core.HingeOpen, "half-open": core.HingeHalfOpen, "closed": core.HingeClosed}[posture]
	if !ok {
		t.Fatalf("unknown NSGO_TEST_FOLD posture %q", posture)
	}
	// WindowLayoutComponent reports no feature on the cover display, so Android has no hinge state there; UIKit reports Closed independently.
	if nsgotest.Target() == "android" && posture == "closed" {
		want = core.HingeUnknown
	}
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return foldDemo(cx)
	}, nsgotest.Environment(core.Environment{Locale: "en-US"}))
	var g core.Geometry
	var safe core.Edges
	var bar core.VerticalBarEdge
	app.Post(func() { g, safe, bar = scope.Env().Geometry(), scope.Env().SafeArea(), scope.Env().VerticalBarEdge() })
	app.Settle()
	data, err := json.Marshal(struct {
		Geometry core.Geometry
		SafeArea core.Edges
		Bar      core.VerticalBarEdge
	}{g, safe, bar})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("native fold observation: %s", data)
	if g.State != want {
		t.Fatalf("hinge state = %v, want %v", g.State, want)
	}
	if posture == "half-open" && len(g.Regions) < 2 {
		t.Fatalf("a half-open display did not split: %+v", g)
	}
	if axis := os.Getenv("NSGO_TEST_FOLD_AXIS"); axis != "" {
		if axis != "horizontal" && axis != "vertical" {
			t.Fatalf("unknown NSGO_TEST_FOLD_AXIS %q", axis)
		}
		if len(g.Obstructions) == 0 || g.Obstructions[0].Horizontal != (axis == "horizontal") {
			t.Fatalf("no %s fold reported: %+v", axis, g)
		}
	}
	if nsgotest.Target() != "android" && !g.HasAngle {
		t.Fatal("the iOS hinge did not report its angle")
	}
	for i, region := range g.Regions {
		pane := app.Find(fmt.Sprintf("id:fold-pane-%d", i)).Frame
		within := func(a, b core.Bounds) bool {
			return a.Origin.X+0.5 >= b.Origin.X && a.Origin.Y+0.5 >= b.Origin.Y &&
				a.Origin.X+a.Size.Width <= b.Origin.X+b.Size.Width+0.5 && a.Origin.Y+a.Size.Height <= b.Origin.Y+b.Size.Height+0.5
		}
		safe := region.Frame
		safe.Origin.X += region.SafeArea.Left
		safe.Origin.Y += region.SafeArea.Top
		safe.Size.Width -= region.SafeArea.Left + region.SafeArea.Right
		safe.Size.Height -= region.SafeArea.Top + region.SafeArea.Bottom
		if !within(pane, safe) {
			t.Fatalf("pane %d crosses its native region: %+v, region %+v", i, pane, region)
		}
		app.Press(fmt.Sprintf("id:fold-add-%d", i))
		if got := app.Find(fmt.Sprintf("id:fold-count-%d", i)).Text; got != "Count: 1" {
			t.Fatalf("pane %d did not take its press: %q", i, got)
		}
	}
	if app.Exists(fmt.Sprintf("id:fold-pane-%d", len(g.Regions))) {
		t.Fatal("the page has a pane outside the native regions")
	}
	app.Screenshot("fold-" + posture)
}
