package main

import (
	"fmt"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// scrollingPage shows how scroll views scroll: paging, snapping, insets,
// an initial offset, pull to refresh, the scroll events and zooming.
func scrollingPage(cx *core.Scope, t *theme) core.Node {
	colors := []core.Color{t.accent, t.success, t.warning, t.danger}
	card := func(i int, width core.Points) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(width), Height: core.Pct(100), CornerRadius: radius,
			Background: colors[i%len(colors)], AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			Children: []core.Node{core.Text{Content: fmt.Sprintf("%d", i+1),
				Style: core.Style{Color: core.RGB(0xffffff), FontSize: 28, FontWeight: core.FontWeightBold}}}}
	}

	// Paging: a page per fling, starting on the second page.
	const pageWidth = 280
	page := core.NewSignal(cx, 1)
	var pages []core.Node
	for i := range 5 {
		pages = append(pages, core.View{Style: core.Style{Width: core.Pt(pageWidth), Height: core.Pct(100), Padding: core.All(6)},
			Children: []core.Node{card(i, pageWidth-12)}})
	}
	settled := func(e core.ScrollEvent) { page.Set(int(e.Offset.X/pageWidth + 0.5)) }

	// Snapping: cards of 140 with a gap of 12 centered in the viewport.
	oneStep := core.NewSignal(cx, false)
	var cards []core.Node
	for i := range 12 {
		cards = append(cards, core.View{Style: core.Style{Width: core.Pt(152), Height: core.Pct(100), Padding: core.Edges{Left: 6, Right: 6}},
			Children: []core.Node{card(i, 140)}})
	}

	// The scroll events, pull to refresh and the behavior's switches.
	events := core.NewSignal(cx, "Drag the list.")
	refreshing := core.NewSignal(cx, false)
	refreshes := core.NewSignal(cx, 0)
	noScroll := core.NewSignal(cx, false)
	noBounces := core.NewSignal(cx, false)
	report := func(name string) func(core.ScrollEvent) {
		return func(e core.ScrollEvent) {
			events.Set(fmt.Sprintf("%s at %.0f · content %.0f × %.0f in %.0f × %.0f", name, e.Offset.Y,
				e.ContentSize.Width, e.ContentSize.Height, e.Size.Width, e.Size.Height))
		}
	}
	refresh := func() {
		refreshing.Set(true)
		cx.AfterFunc(1500*time.Millisecond, func() {
			refreshes.Set(refreshes.Get() + 1)
			refreshing.Set(false)
		})
	}
	var rows []core.Node
	for i := range 30 {
		rows = append(rows, core.View{Style: core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 14), JustifyContent: core.JustifyCenter,
			Background: []core.Color{t.surface, t.sunken}[i%2]},
			Children: []core.Node{core.Text{ContentFunc: func() string {
				return fmt.Sprintf("Row %d, refreshed %d times", i+1, refreshes.Get())
			}}}})
	}

	// Zooming.
	zoomRef := &core.Ref{}
	zoom := core.NewSignal(cx, float32(1))
	var tiles []core.Node
	for i := range 16 {
		tiles = append(tiles, core.View{Style: core.Style{Width: core.Pt(60), Height: core.Pt(60), CornerRadius: 8,
			Background: colors[(i+i/4)%len(colors)], AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			Children: []core.Node{core.Text{Content: fmt.Sprintf("%c", 'A'+i), Style: core.Style{Color: core.RGB(0xffffff)}}}})
	}

	// The keyboard.
	query := core.NewSignal(cx, "")

	strip := core.Style{Direction: core.Row, Height: core.Pct(100)}
	frame := core.Style{BorderWidth: 1, BorderColor: t.line, CornerRadius: radius, Overflow: core.Hidden}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Paging", "Behavior.Paging stops at each page; ContentOffset starts on the second.",
			core.Scroll{Horizontal: true, HideIndicator: true, TestID: "pages", ContentOffset: core.Point{X: pageWidth},
				Behavior:        core.ScrollBehavior{Paging: true},
				OnMomentumEnd:   settled,
				OnScrollDragEnd: settled,
				Style:           core.Style{Width: core.Pt(pageWidth), Height: core.Pt(150)},
				Children:        []core.Node{core.View{Style: strip, Children: pages}}},
			t.live(func() string { return fmt.Sprintf("Page %d of 5", page.Get()+1) }),
		),
		t.demo("Snapping", "SnapToInterval with SnapCenter centers a card; DisableIntervalMomentum moves one at a time.",
			flow(t.plain("One card per fling", func() { oneStep.Set(!oneStep.Get()) })),
			core.Scroll{Horizontal: true, TestID: "cards",
				BehaviorFunc: func() core.ScrollBehavior {
					return core.ScrollBehavior{SnapToInterval: 152, SnapToAlignment: core.SnapCenter,
						DisableIntervalMomentum: oneStep.Get(), ContentInset: core.Edges{Left: 60, Right: 60}}
				},
				Style:    core.Style{Height: core.Pt(120)},
				Children: []core.Node{core.View{Style: strip, Children: cards}}},
			t.live(func() string {
				if oneStep.Get() {
					return "Each fling moves one card"
				}
				return "A fling glides past several cards"
			}),
		),
		t.demo("Refresh and events", "Pull down to refresh (touch screens and macOS trackpads); the events report the content and viewport sizes.",
			flow(
				t.plain("Refresh", refresh),
				t.plain("Scrolling on or off", func() { noScroll.Set(!noScroll.Get()) }),
				t.plain("Bounces on or off", func() { noBounces.Set(!noBounces.Get()) }),
			),
			core.Scroll{TestID: "feed",
				BehaviorFunc: func() core.ScrollBehavior {
					return core.ScrollBehavior{NoScroll: noScroll.Get(), NoBounces: noBounces.Get(),
						ContentInset: core.Edges{Top: 8, Bottom: 8}, DecelerationRate: core.DecelerationFast}
				},
				Refreshing:        refreshing.Get,
				OnRefresh:         refresh,
				OnScrollDragStart: report("Drag start"),
				OnScrollDragEnd:   report("Drag end"),
				OnMomentumStart:   report("Momentum start"),
				OnMomentumEnd:     report("Momentum end"),
				Style:             frame.Merge(core.Style{Height: core.Pt(220)}),
				Children:          rows},
			t.live(events.Get),
			t.live(func() string {
				switch {
				case refreshing.Get():
					return "Refreshing…"
				case noScroll.Get():
					return "Scrolling is off"
				}
				return fmt.Sprintf("Refreshed %d times", refreshes.Get())
			}),
		),
		t.demo("Zooming", "MaxZoomScale lets pinches zoom; CenterContent centers it, and Ref.ZoomTo zooms from the app.",
			flow(
				t.plain("Zoom in on C", func() { zoomRef.ZoomTo(3, &core.Point{X: 168, Y: 30}, true) }),
				t.plain("Reset", func() { zoomRef.ZoomTo(1, nil, true) }),
			),
			core.Scroll{TestID: "zoom", Ref: zoomRef, MinZoomScale: 0.5, MaxZoomScale: 4, CenterContent: true,
				OnScroll: func(e core.ScrollEvent) { zoom.Set(e.ZoomScale) },
				Style:    frame.Merge(core.Style{Height: core.Pt(220), Background: t.sunken}),
				Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(264), Padding: core.All(4),
					Direction: core.Row, Wrap: core.Wrap, Gap: 4}, Children: tiles}}},
			t.live(func() string { return fmt.Sprintf("Zoom %.1f×", zoom.Get()) }),
		),
		t.demo("Keyboard", "Dragging dismisses the keyboard (KeyboardDismissOnDrag); taps on buttons keep it (KeyboardTapsHandled).",
			core.Scroll{TestID: "keyboard",
				Behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissOnDrag, KeyboardShouldPersistTaps: core.KeyboardTapsHandled},
				Style:    frame.Merge(core.Style{Height: core.Pt(160)}),
				Children: []core.Node{core.View{Style: core.Style{Padding: core.All(12), Gap: 10}, Children: []core.Node{
					core.Input{Placeholder: "Type, then drag the area", Value: query.Get, OnChange: query.Set, Style: t.field()},
					t.plain("Clear", func() { query.Set("") }),
					core.View{Style: core.Style{Height: core.Pt(240)}},
				}}}},
		),
	}}
}
