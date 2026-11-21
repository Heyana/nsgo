package nsgotest_test

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func phoneEnvs() []struct {
	name string
	env  core.Environment
} {
	return []struct {
		name string
		env  core.Environment
	}{
		{"ios", core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}},
		{"android", core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone}},
	}
}

func TestPhoneScrollHandleVertical(t *testing.T) {
	for _, pe := range phoneEnvs() {
		t.Run(pe.name, func(t *testing.T) {
			var dragStarts, dragEnds, scrollEnds, rowPresses int
			var lastY core.Points
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				return core.Scroll{
					TestID:            "vscroll",
					Style:             core.Style{Width: core.Pt(390), Height: core.Pt(600)},
					OnScroll:          func(e core.ScrollEvent) { lastY = e.Offset.Y },
					OnScrollDragStart: func(core.ScrollEvent) { dragStarts++ },
					OnScrollDragEnd:   func(core.ScrollEvent) { dragEnds++ },
					OnScrollEnd:       func(core.ScrollEvent) { scrollEnds++ },
					Children: []core.Node{
						core.View{
							Role:    core.RoleButton,
							Label:   "Row",
							OnPress: func(core.PressEvent) { rowPresses++ },
							Style:   core.Style{Width: core.Pt(390), Height: core.Pt(3000)},
						},
					},
				}
			}, nsgotest.Environment(pe.env), nsgotest.Size(390, 600))

			handle, ok := app.ScrollHandle("vscroll")
			if !ok {
				t.Fatal("expected vertical scroll handle on overflowing scroll view")
			}
			if handle.Origin.X < 370 || handle.Size.Height < 44 {
				t.Fatalf("unexpected vertical handle bounds: %+v", handle)
			}

			// Drag the handle halfway down its track.
			start := core.Point{X: handle.Origin.X + handle.Size.Width/2, Y: handle.Origin.Y + handle.Size.Height/2}
			travel := 600 - 2*handle.Origin.Y - handle.Size.Height
			app.DragAt("vscroll", start, core.Point{Y: travel / 2})
			if got := app.Find("vscroll").Offset.Y; math.Abs(float64(got-1200)) > 20 {
				t.Fatalf("vertical handle half drag offset = %v, want ~1200", got)
			}
			if lastY != app.Find("vscroll").Offset.Y {
				t.Fatalf("OnScroll reported %v, view offset is %v", lastY, app.Find("vscroll").Offset.Y)
			}
			if rowPresses != 0 {
				t.Fatalf("dragging handle pressed content underneath %d times", rowPresses)
			}
			if dragStarts != 1 || dragEnds != 1 || scrollEnds != 1 {
				t.Fatalf("events: starts=%d ends=%d scrollEnds=%d, want 1 each", dragStarts, dragEnds, scrollEnds)
			}

			// Handle moved down with the scroll offset; drag it to the bottom end.
			midHandle, ok := app.ScrollHandle("vscroll")
			if !ok || midHandle.Origin.Y <= handle.Origin.Y {
				t.Fatalf("handle did not advance with scroll: before=%+v after=%+v", handle, midHandle)
			}
			midStart := core.Point{X: midHandle.Origin.X + midHandle.Size.Width/2, Y: midHandle.Origin.Y + midHandle.Size.Height/2}
			app.DragAt("vscroll", midStart, core.Point{Y: travel})
			if got := app.Find("vscroll").Offset.Y; math.Abs(float64(got-2400)) > 1 {
				t.Fatalf("vertical handle end drag offset = %v, want 2400", got)
			}

			// Drag the handle back to the top.
			endHandle, ok := app.ScrollHandle("vscroll")
			if !ok {
				t.Fatal("handle missing at end of scroll")
			}
			endStart := core.Point{X: endHandle.Origin.X + endHandle.Size.Width/2, Y: endHandle.Origin.Y + endHandle.Size.Height/2}
			app.DragAt("vscroll", endStart, core.Point{Y: -travel - 40})
			if got := app.Find("vscroll").Offset.Y; got != 0 {
				t.Fatalf("vertical handle drag back to top = %v, want 0", got)
			}

			// Content drag in the middle still scrolls naturally (dragging finger up increases offset).
			app.DragAt("vscroll", core.Point{X: 150, Y: 300}, core.Point{Y: -120})
			if got := app.Find("vscroll").Offset.Y; got <= 0 {
				t.Fatalf("content drag did not scroll downward: offset=%v", got)
			}
		})
	}
}

func TestPhoneScrollHandleHorizontal(t *testing.T) {
	for _, pe := range phoneEnvs() {
		t.Run(pe.name, func(t *testing.T) {
			var dragStarts, dragEnds, scrollEnds int
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				return core.Scroll{
					TestID:            "hscroll",
					Horizontal:        true,
					Style:             core.Style{Width: core.Pt(390), Height: core.Pt(200)},
					OnScrollDragStart: func(core.ScrollEvent) { dragStarts++ },
					OnScrollDragEnd:   func(core.ScrollEvent) { dragEnds++ },
					OnScrollEnd:       func(core.ScrollEvent) { scrollEnds++ },
					Children: []core.Node{
						core.View{Style: core.Style{Width: core.Pt(2390), Height: core.Pt(200)}},
					},
				}
			}, nsgotest.Environment(pe.env), nsgotest.Size(390, 200))

			handle, ok := app.ScrollHandle("hscroll")
			if !ok {
				t.Fatal("expected horizontal scroll handle on overflowing scroll view")
			}
			if handle.Origin.Y < 180 || handle.Size.Width < 44 {
				t.Fatalf("unexpected horizontal handle bounds: %+v", handle)
			}

			start := core.Point{X: handle.Origin.X + handle.Size.Width/2, Y: handle.Origin.Y + handle.Size.Height/2}
			travel := 390 - 2*handle.Origin.X - handle.Size.Width

			// Scrub halfway right.
			app.DragAt("hscroll", start, core.Point{X: travel / 2})
			if got := app.Find("hscroll").Offset.X; math.Abs(float64(got-1000)) > 20 {
				t.Fatalf("horizontal handle half drag offset = %v, want ~1000", got)
			}
			if dragStarts != 1 || dragEnds != 1 || scrollEnds != 1 {
				t.Fatalf("horizontal events: starts=%d ends=%d scrollEnds=%d, want 1 each", dragStarts, dragEnds, scrollEnds)
			}

			// Scrub to full right end.
			midHandle, ok := app.ScrollHandle("hscroll")
			if !ok || midHandle.Origin.X <= handle.Origin.X {
				t.Fatalf("horizontal handle did not move right: before=%+v after=%+v", handle, midHandle)
			}
			midStart := core.Point{X: midHandle.Origin.X + midHandle.Size.Width/2, Y: midHandle.Origin.Y + midHandle.Size.Height/2}
			app.DragAt("hscroll", midStart, core.Point{X: travel})
			if got := app.Find("hscroll").Offset.X; math.Abs(float64(got-2000)) > 1 {
				t.Fatalf("horizontal handle end drag offset = %v, want 2000", got)
			}

			// Scrub back to left start.
			endHandle, _ := app.ScrollHandle("hscroll")
			endStart := core.Point{X: endHandle.Origin.X + endHandle.Size.Width/2, Y: endHandle.Origin.Y + endHandle.Size.Height/2}
			app.DragAt("hscroll", endStart, core.Point{X: -travel - 40})
			if got := app.Find("hscroll").Offset.X; got != 0 {
				t.Fatalf("horizontal handle drag back to start = %v, want 0", got)
			}
		})
	}
}

func TestPhoneScrollHandleTouchStepsAndCancel(t *testing.T) {
	var starts, ends, rests int
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Scroll{
			TestID:            "scroll",
			Style:             core.Style{Width: core.Pt(390), Height: core.Pt(400)},
			OnScrollDragStart: func(core.ScrollEvent) { starts++ },
			OnScrollDragEnd:   func(core.ScrollEvent) { ends++ },
			OnScrollEnd:       func(core.ScrollEvent) { rests++ },
			Children:          []core.Node{core.View{Style: core.Style{Height: core.Pt(2400)}}},
		}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}), nsgotest.Size(390, 400))

	handle, ok := app.ScrollHandle("scroll")
	if !ok {
		t.Fatal("expected handle")
	}
	at := core.Point{X: handle.Origin.X + handle.Size.Width/2, Y: handle.Origin.Y + handle.Size.Height/2}
	finger := app.Touch("scroll", at)
	if starts != 1 {
		t.Fatalf("touch down on handle did not start drag: starts=%d", starts)
	}
	finger.Move(core.Point{X: at.X, Y: at.Y + 60})
	first := app.Find("scroll").Offset.Y
	if first <= 0 {
		t.Fatalf("first touch move did not scroll: offset=%v", first)
	}
	finger.Move(core.Point{X: at.X, Y: at.Y + 120})
	second := app.Find("scroll").Offset.Y
	if second <= first {
		t.Fatalf("second touch move did not advance: first=%v second=%v", first, second)
	}
	finger.Cancel()
	if ends != 1 || rests != 1 {
		t.Fatalf("cancel did not finish drag cleanly: ends=%d rests=%d", ends, rests)
	}
}

func TestPhoneScrollHandleHidesForShortAndDynamicContent(t *testing.T) {
	for _, pe := range phoneEnvs() {
		t.Run(pe.name, func(t *testing.T) {
			var vHeight, hWidth *core.Signal[core.Points]
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				vHeight = core.NewSignal(cx, core.Points(200))
				hWidth = core.NewSignal(cx, core.Points(300))
				return core.View{Children: []core.Node{
					core.Scroll{
						TestID: "vshort",
						Style:  core.Style{Width: core.Pt(390), Height: core.Pt(300)},
						Children: []core.Node{
							core.View{StyleFunc: func() core.Style { return core.Style{Height: core.Pt(vHeight.Get())} }},
						},
					},
					core.Scroll{
						TestID:     "hshort",
						Horizontal: true,
						Style:      core.Style{Width: core.Pt(390), Height: core.Pt(120)},
						Children: []core.Node{
							core.View{StyleFunc: func() core.Style { return core.Style{Width: core.Pt(hWidth.Get()), Height: core.Pt(120)} }},
						},
					},
				}}
			}, nsgotest.Environment(pe.env), nsgotest.Size(390, 500))

			if _, ok := app.ScrollHandle("vshort"); ok {
				t.Fatal("short vertical scroll view should hide handle")
			}
			if _, ok := app.ScrollHandle("hshort"); ok {
				t.Fatal("short horizontal scroll view should hide handle")
			}
			// Dragging along the edge of a short scroll view leaves offset at 0.
			app.DragAt("vshort", core.Point{X: 384, Y: 20}, core.Point{Y: 100})
			if app.Find("vshort").Offset.Y != 0 {
				t.Fatalf("short vertical scroll view moved to %v", app.Find("vshort").Offset.Y)
			}

			// Grow both beyond viewport: handles appear and become draggable.
			vHeight.Set(1500)
			hWidth.Set(1500)
			app.Settle()

			vHandle, ok := app.ScrollHandle("vshort")
			if !ok {
				t.Fatal("handle did not appear when vertical content grew past viewport")
			}
			hHandle, ok := app.ScrollHandle("hshort")
			if !ok {
				t.Fatal("handle did not appear when horizontal content grew past viewport")
			}
			app.DragAt("vshort", core.Point{X: vHandle.Origin.X + 2, Y: vHandle.Origin.Y + 10}, core.Point{Y: 80})
			if app.Find("vshort").Offset.Y <= 0 {
				t.Fatal("grown vertical handle did not scrub")
			}
			app.DragAt("hshort", core.Point{X: hHandle.Origin.X + 10, Y: hHandle.Origin.Y + 2}, core.Point{X: 80})
			if app.Find("hshort").Offset.X <= 0 {
				t.Fatal("grown horizontal handle did not scrub")
			}

			// Shrink back to fit: handles hide again.
			vHeight.Set(250)
			hWidth.Set(350)
			app.Settle()
			if _, ok := app.ScrollHandle("vshort"); ok {
				t.Fatal("vertical handle stayed shown after content shrank to fit")
			}
			if _, ok := app.ScrollHandle("hshort"); ok {
				t.Fatal("horizontal handle stayed shown after content shrank to fit")
			}
		})
	}
}

func TestPhoneScrollHandleRespectsConfigAndPlatform(t *testing.T) {
	phone := core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.Scroll{
				TestID:        "hidden-ind",
				HideIndicator: true,
				Style:         core.Style{Width: core.Pt(390), Height: core.Pt(120)},
				Children:      []core.Node{core.View{Style: core.Style{Height: core.Pt(1200)}}},
			},
			core.Scroll{
				TestID:   "no-scroll",
				Behavior: core.ScrollBehavior{NoScroll: true},
				Style:    core.Style{Width: core.Pt(390), Height: core.Pt(120)},
				Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(1200)}}},
			},
			core.Scroll{
				TestID:       "zoomable",
				MinZoomScale: 1,
				MaxZoomScale: 3,
				Style:        core.Style{Width: core.Pt(390), Height: core.Pt(120)},
				Children:     []core.Node{core.View{Style: core.Style{Height: core.Pt(1200)}}},
			},
		}}
	}, nsgotest.Environment(phone), nsgotest.Size(390, 400))

	for _, id := range []string{"hidden-ind", "no-scroll", "zoomable"} {
		if _, ok := app.ScrollHandle(id); ok {
			t.Errorf("%s should not show phone scroll handle", id)
		}
	}

	for _, tc := range []struct {
		name string
		env  core.Environment
	}{
		{"macos desktop", core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}},
		{"tvos", core.Environment{OS: core.OSIOS, Idiom: core.IdiomTV}},
		{"watchos", core.Environment{OS: core.OSIOS, Idiom: core.IdiomWatch}},
		{"simulated phone on web", core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone, Simulated: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				return core.Scroll{
					TestID:   "s",
					Style:    core.Style{Width: core.Pt(390), Height: core.Pt(200)},
					Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(2000)}}},
				}
			}, nsgotest.Environment(tc.env), nsgotest.Size(390, 200))
			if _, ok := a.ScrollHandle("s"); ok {
				t.Fatalf("unexpected phone scroll handle on %s", tc.name)
			}
		})
	}
}

func TestPhoneScrollHandleWithInsetsAndSnapping(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Scroll{
			TestID:        "snapping",
			ContentOffset: core.Point{Y: -40},
			Behavior: core.ScrollBehavior{
				ContentInset:   core.Edges{Top: 40, Bottom: 20},
				SnapToInterval: 200,
			},
			Style:    core.Style{Width: core.Pt(390), Height: core.Pt(200)},
			Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(1200)}}},
		}
	}, nsgotest.Environment(core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone}), nsgotest.Size(390, 200))

	handle, ok := app.ScrollHandle("snapping")
	if !ok {
		t.Fatal("expected handle on inset snapping scroll view")
	}
	start := core.Point{X: handle.Origin.X + handle.Size.Width/2, Y: handle.Origin.Y + handle.Size.Height/2}
	app.DragAt("snapping", start, core.Point{Y: 45})
	got := app.Find("snapping").Offset.Y
	// Should have moved and settled onto a 200pt snap interval.
	if got <= 0 || math.Mod(float64(got), 200) > 1 {
		t.Fatalf("snapping handle release settled at %v, want positive multiple of 200", got)
	}
}

func TestPhoneScrollHandleOnVirtualizedList(t *testing.T) {
	for _, pe := range phoneEnvs() {
		t.Run(pe.name, func(t *testing.T) {
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				return core.View{Children: []core.Node{
					core.List{
						TestID:        "vlist",
						Count:         func() int { return 100 },
						EstimatedSize: 40,
						Style:         core.Style{Width: core.Pt(390), Height: core.Pt(200)},
						Row: func(cx *core.Scope, r core.ListRow) core.Node {
							return core.View{Style: core.Style{Height: core.Pt(40)}}
						},
					},
					core.List{
						TestID:        "hlist",
						Horizontal:    true,
						Count:         func() int { return 50 },
						EstimatedSize: 80,
						Style:         core.Style{Width: core.Pt(390), Height: core.Pt(100)},
						Row: func(cx *core.Scope, r core.ListRow) core.Node {
							return core.View{Style: core.Style{Width: core.Pt(80), Height: core.Pt(100)}}
						},
					},
				}}
			}, nsgotest.Environment(pe.env), nsgotest.Size(390, 320))

			vh, ok := app.ScrollHandle("vlist")
			if !ok {
				t.Fatal("expected handle on long vertical core.List")
			}
			app.DragAt("vlist", core.Point{X: vh.Origin.X + 2, Y: vh.Origin.Y + 10}, core.Point{Y: 80})
			if app.Find("vlist").Offset.Y <= 0 {
				t.Fatal("dragging vertical list handle did not scroll")
			}

			hh, ok := app.ScrollHandle("hlist")
			if !ok {
				t.Fatal("expected handle on long horizontal core.List")
			}
			app.DragAt("hlist", core.Point{X: hh.Origin.X + 10, Y: hh.Origin.Y + 2}, core.Point{X: 120})
			if app.Find("hlist").Offset.X <= 0 {
				t.Fatal("dragging horizontal list handle did not scroll")
			}
		})
	}
}
