package nsgotest

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/mobile"
)

func TestSystemGestureRequestsCombineAndRelease(t *testing.T) {
	base := core.Bounds{Origin: core.Point{Y: 100}, Size: core.Size{Width: 12, Height: 80}}
	extra := core.Bounds{Origin: core.Point{Y: 290}, Size: core.Size{Width: 24, Height: 60}}
	rects := []core.Bounds{base, {Size: core.Size{Width: -1, Height: 20}}}
	var show *core.Signal[bool]
	var release func()
	a := Start(t, func(cx *core.Scope) core.Node {
		release = mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
			IOSEdges: mobile.GestureEdgeBottom, AndroidRects: rects,
		})
		rects[0] = core.Bounds{}
		show = core.NewSignal(cx, true)
		return core.View{Children: []core.Node{core.Show{
			When: show.Get,
			Then: func(child *core.Scope) core.Node {
				mobile.DeferSystemGestures(child, mobile.SystemGestureOptions{
					IOSEdges:     mobile.GestureEdgeBottom | mobile.GestureEdgeLeft,
					AndroidRects: []core.Bounds{extra},
				})
				return core.View{}
			},
		}}}
	})
	check := func(edges mobile.SystemGestureEdge, rects ...core.Bounds) {
		t.Helper()
		got := a.SystemGestures
		if got.IOSEdges != edges || !slices.Equal(got.AndroidRects, rects) {
			t.Fatalf("system gestures = %+v, want edges %d and rects %v", got, edges, rects)
		}
	}
	check(mobile.GestureEdgeBottom|mobile.GestureEdgeLeft, base, extra)
	show.Set(false)
	a.Settle()
	check(mobile.GestureEdgeBottom, base)
	release()
	release()
	a.Settle()
	check(0)
}

// Check the per-surface model with two virtual windows, including on machines
// where the native phone backend supports only one.
func TestSystemGestureRequestsStayOnTheirSurface(t *testing.T) {
	if Target() != "" {
		t.Skip("uses two headless windows")
	}
	a := StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node {
			mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{IOSEdges: mobile.GestureEdgeBottom})
			return core.View{}
		})
		desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Second", Width: 180, Height: 120},
			func(cx *core.Scope) core.Node {
				mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{IOSEdges: mobile.GestureEdgeLeft})
				return core.View{}
			})
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	second := a.Surface("title:Second")
	if got := second.SystemGestures().IOSEdges; got != mobile.GestureEdgeLeft {
		t.Fatalf("second surface edges = %v, want left", got)
	}
	if got := a.SystemGestures.IOSEdges; got != mobile.GestureEdgeBottom {
		t.Fatalf("main surface edges = %v, want bottom", got)
	}
}
