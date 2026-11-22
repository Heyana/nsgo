package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func geometryNode(node devtools.Node, id string) (devtools.Node, bool) {
	if node.TestID == id {
		return node, true
	}
	for _, child := range node.Children {
		if found, ok := geometryNode(child, id); ok {
			return found, true
		}
	}
	return devtools.Node{}, false
}

func TestNativeDevToolsFollowPresentationDuringTransition(t *testing.T) {
	testMovingGeometry(t, false, false)
}

func TestNativeDevToolsFollowPresentationDuringLayoutAnimation(t *testing.T) {
	testMovingGeometry(t, true, false)
}

func TestNativeDevToolsLateObservationDuringTransition(t *testing.T) {
	testMovingGeometry(t, false, true)
}

func testMovingGeometry(t *testing.T, layout, late bool) {
	t.Helper()
	if nsgotest.Target() == "" || strings.HasPrefix(nsgotest.Target(), "harmonyos") || strings.HasPrefix(nsgotest.Target(), "miniprogram") {
		t.Skip("requires a native presentation source")
	}
	devtools.Enabled = true
	t.Cleanup(func() { devtools.Enabled = false })
	var moved *core.Signal[bool]
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		moved = core.NewSignal(cx, false)
		nodes := core.Group{
			core.View{TestID: "moving", StyleFunc: func() core.Style {
				x, left := core.Points(10), core.Points(20)
				if moved.Get() {
					if layout {
						left = 60
					} else {
						x = 50
					}
				}
				return core.Style{Position: core.Absolute, Left: core.Pt(left), Top: core.Pt(20), Width: core.Pt(40), Height: core.Pt(20),
					Transform: core.Transform{TranslateX: x}, Transition: core.Transition{Duration: 800 * time.Millisecond, Properties: core.TransitionTransform}}
			}},
			core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true}},
		}
		if late {
			return nodes[:1]
		}
		return nodes
	}, nsgotest.Size(300, 200), nsgotest.Environment(core.Environment{ReduceMotion: false}))
	surfaces := devtools.Surfaces()
	if len(surfaces) == 0 {
		t.Fatal("native app did not register its DevTools surface")
	}
	surface := surfaces[len(surfaces)-1]
	read := func() devtools.Node { node, _ := geometryNode(surface.Elements(), "moving"); return node }
	initial := make(chan devtools.Node, 1)
	surface.Post(func() { initial <- read() })
	first := <-initial
	app.Settle() // apply a diagnostic barrier when observation started late
	if math.Abs(float64(first.Bounds[0]-30)) > 1 {
		t.Fatalf("initial transformed native bounds = %v, want x=30", first.Bounds)
	}
	// Sample until the native bounds are between the two ends. One sample at a fixed delay can land before the platform starts the animation, which on a busy runner takes a while after the change, and the ease-in curve barely moves at first.
	middle := make(chan devtools.Node, 1)
	var sampled devtools.Node
	probe := func() {
		for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(15 * time.Millisecond) {
			got := make(chan devtools.Node, 1)
			surface.Post(func() { got <- read() })
			node := <-got
			sampled = node
			if node.Bounds[0] > 31 && node.Bounds[0] < 69 {
				middle <- node
				return
			}
		}
		close(middle)
	}
	app.Post(func() {
		if layout {
			scope.AnimateLayout(core.LayoutAnimation{Update: &core.LayoutTransition{Curve: core.LayoutEaseInEaseOut, Duration: 800 * time.Millisecond}})
		}
		moved.Set(true)
		go probe()
	})
	app.Settle()
	select {
	case _, ok := <-middle:
		if !ok {
			t.Fatalf("native bounds never showed the transition's middle: last %v, want x between 30 and 70", sampled.Bounds)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("native transition probe did not reach the graph")
	}
	final := make(chan devtools.Node, 1)
	surface.Post(func() { final <- read() })
	last := <-final
	if math.Abs(float64(last.Bounds[0]-70)) > 1 {
		t.Fatalf("final native bounds = %v, want x=70", last.Bounds)
	}
}

func TestNativeDevToolsFollowStickyScroll(t *testing.T) {
	if nsgotest.Target() == "" || strings.HasPrefix(nsgotest.Target(), "harmonyos") || strings.HasPrefix(nsgotest.Target(), "miniprogram") || strings.HasPrefix(nsgotest.Target(), "watchos") {
		t.Skip("requires a native sticky presentation source")
	}
	devtools.Enabled = true
	t.Cleanup(func() { devtools.Enabled = false })
	var scroll core.Ref
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Group{core.Scroll{TestID: "scroll", Ref: &scroll, Style: core.Style{Height: core.Pt(200)}, Children: []core.Node{
			core.View{Style: core.Style{Height: core.Pt(100)}},
			core.View{Style: core.Style{Height: core.Pt(500)}, Children: []core.Node{
				core.View{Style: core.Style{Height: core.Pt(30)}},
				core.View{TestID: "header", Style: core.Style{Height: core.Pt(20), Position: core.Sticky, Top: core.Pt(10)}},
			}},
		}}, core.UpdateHighlights{NoUpdates: true, Layout: core.LayoutAudit{Bounds: true}}}
	}, nsgotest.Size(300, 200), nsgotest.Environment(core.Environment{ReduceMotion: false}))
	surfaces := devtools.Surfaces()
	if len(surfaces) == 0 {
		t.Fatal("native app did not register its DevTools surface")
	}
	surface := surfaces[len(surfaces)-1]
	for _, step := range []struct{ offset, top core.Points }{{0, 130}, {200, 10}, {390, 10}} {
		app.Post(func() { scroll.ScrollTo(core.Point{Y: step.offset}, false) })
		app.Settle()
		result := make(chan devtools.Node, 1)
		surface.Post(func() { node, _ := geometryNode(surface.Elements(), "header"); result <- node })
		node := <-result
		if math.Abs(float64(node.Bounds[1]-float32(step.top))) > 1 {
			t.Fatalf("scroll offset %v: native sticky bounds = %v, want y=%v", step.offset, node.Bounds, step.top)
		}
	}
}
