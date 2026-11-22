package main

import (
	"fmt"
	"maps"
	"slices"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/ui"
)

// foldDemo chooses its own layout: one pane per usable region, clipped to this page's part of the surface. Geometry never changes other demos.
func foldDemo(cx *core.Scope) core.Node {
	return foldPage(cx, cx.Env().Geometry)
}

func foldPage(cx *core.Scope, geometry func() core.Geometry) core.Node {
	tr := locales.For(cx)
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	viewport := core.NewSignal(cx, core.Bounds{})
	counts := core.NewSignalFunc(cx, map[int]int{}, maps.Equal)
	frames := core.NewMemoFunc(cx, func() []core.Bounds {
		return foldPaneFrames(geometry(), viewport.Get())
	}, slices.Equal)
	state := func() string {
		switch geometry().State {
		case core.HingeClosed:
			return tr.FoldClosed()
		case core.HingeHalfOpen:
			return tr.FoldHalfOpen()
		case core.HingeOpen:
			return tr.FoldOpen()
		default:
			return tr.FoldUnknown()
		}
	}
	measure := func() {
		ref.Measure(func(_, page core.Bounds, ok bool) {
			if ok {
				viewport.Set(page)
			}
		})
	}
	cx.Effect(func() {
		_ = geometry()
		_ = cx.Env().Window()
		_ = cx.Env().SafeArea()
		measure()
	})
	return core.View{Ref: ref, TestID: "fold-page", Style: fill,
		OnLayout: func(core.Bounds) { measure() },
		Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
			var panes []core.Node
			for i, frame := range frames.Get() {
				panes = append(panes, core.Scroll{TestID: fmt.Sprintf("fold-pane-%d", i),
					Style: core.Style{Position: core.Absolute, Left: core.Pt(frame.Origin.X), Top: core.Pt(frame.Origin.Y),
						Width: core.Pt(frame.Size.Width), Height: core.Pt(frame.Size.Height)},
					StyleFunc: ui.Filled(colors.Surface), Children: []core.Node{core.View{
						Style: core.Style{Padding: core.All(16), Gap: 12}, Children: []core.Node{
							ui.Text(tr.FoldPane(i+1), 22, ui.Semibold, colors.Text),
							ui.Text(tr.DemoFoldDetail(), 14, ui.Regular, colors.Muted),
							core.Text{TestID: fmt.Sprintf("fold-state-%d", i), ContentFunc: state,
								Style: core.Style{FontSize: 14, FontWeight: ui.Medium}, StyleFunc: ui.Colored(colors.Text)},
							ui.Button{Text: tr.FoldAdd(), TestID: fmt.Sprintf("fold-add-%d", i), OnPress: func() {
								counts.Update(func(previous map[int]int) map[int]int {
									next := maps.Clone(previous)
									next[i]++
									return next
								})
							}},
							core.Text{TestID: fmt.Sprintf("fold-count-%d", i), ContentFunc: func() string { return tr.FoldCount(counts.Get()[i]) },
								Style: core.Style{FontSize: 14}, StyleFunc: ui.Colored(colors.Text)},
						}}}})
			}
			return core.Group(panes)
		})}}
}

// foldPaneFrames maps surface regions into the page, keeping its content clear of each region's safe area and reserved rectangle. A reserved rectangle that does not split the surface leaves the largest clear part.
func foldPaneFrames(g core.Geometry, viewport core.Bounds) []core.Bounds {
	intersect := func(a, b core.Bounds) core.Bounds {
		x, y := max(a.Origin.X, b.Origin.X), max(a.Origin.Y, b.Origin.Y)
		return core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{
			Width:  max(0, min(a.Origin.X+a.Size.Width, b.Origin.X+b.Size.Width)-x),
			Height: max(0, min(a.Origin.Y+a.Size.Height, b.Origin.Y+b.Size.Height)-y)}}
	}
	var frames []core.Bounds
	for _, region := range g.Regions {
		f, safe := region.Frame, region.SafeArea
		f.Origin.X, f.Origin.Y = f.Origin.X+safe.Left, f.Origin.Y+safe.Top
		f.Size.Width, f.Size.Height = max(0, f.Size.Width-safe.Left-safe.Right), max(0, f.Size.Height-safe.Top-safe.Bottom)
		f = intersect(f, viewport)
		if reserved := intersect(f, region.Reserved); reserved.Size.Width > 0 && reserved.Size.Height > 0 {
			x, y, w, h := f.Origin.X, f.Origin.Y, f.Size.Width, f.Size.Height
			rx, ry := reserved.Origin.X, reserved.Origin.Y
			right, bottom := rx+reserved.Size.Width, ry+reserved.Size.Height
			var largest core.Bounds
			for _, candidate := range []core.Bounds{
				{Origin: f.Origin, Size: core.Size{Width: rx - x, Height: h}},
				{Origin: core.Point{X: right, Y: y}, Size: core.Size{Width: x + w - right, Height: h}},
				{Origin: f.Origin, Size: core.Size{Width: w, Height: ry - y}},
				{Origin: core.Point{X: x, Y: bottom}, Size: core.Size{Width: w, Height: y + h - bottom}},
			} {
				if candidate.Size.Width*candidate.Size.Height > largest.Size.Width*largest.Size.Height {
					largest = candidate
				}
			}
			f = largest
		}
		if f.Size.Width > 0 && f.Size.Height > 0 {
			f.Origin.X, f.Origin.Y = f.Origin.X-viewport.Origin.X, f.Origin.Y-viewport.Origin.Y
			frames = append(frames, f)
		}
	}
	return frames
}
