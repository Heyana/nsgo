package core

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSnapTargets(t *testing.T) {
	snap := func(b ScrollBehavior, offset, projected Points, v float32, viewport, content Points) Points {
		t.Helper()
		at, ok := b.SnapTarget(offset, projected, v, viewport, content)
		if !ok {
			t.Fatalf("%+v does not snap", b)
		}
		return at
	}
	// Pages of 300 in 1000 points of content (the last page is short):
	// one page per fling, wherever the fling would glide.
	paging := ScrollBehavior{Paging: true}
	for _, c := range []struct {
		offset, projected Points
		v                 float32
		want              Points
	}{{100, 100, 0, 0}, {100, 900, 1, 300}, {650, 900, 1, 700}, {250, 0, -1, 0}} {
		if got := snap(paging, c.offset, c.projected, c.v, 300, 1000); got != c.want {
			t.Errorf("paging from %v: %v, want %v", c.offset, got, c.want)
		}
	}
	// Items of 120 centered in a 300 viewport: k * 120 - 90.
	interval := ScrollBehavior{SnapToInterval: 120, SnapToAlignment: SnapCenter}
	if got := snap(interval, 200, 200, 0, 300, 2000); got != 150 {
		t.Errorf("settled at %v, want 150", got)
	}
	// A fast fling glides past several items, rounded forward.
	if got := snap(interval, 200, 700, 2, 300, 2000); got != 750 {
		t.Errorf("fling to %v, want 750", got)
	}
	one := interval
	one.DisableIntervalMomentum = true
	if got := snap(one, 200, 700, 2, 300, 2000); got != 270 {
		t.Errorf("one step to %v, want 270", got)
	}
	offsets := ScrollBehavior{SnapToOffsets: []Points{900, 0, 500}}
	if got := snap(offsets, 520, 600, 2, 300, 2000); got != 900 {
		t.Errorf("offsets fling to %v, want 900", got)
	}
	if got := snap(offsets, 520, 520, 0, 300, 2000); got != 500 {
		t.Errorf("offsets settle at %v, want 500", got)
	}
	if _, ok := (&ScrollBehavior{}).SnapTarget(10, 10, 1, 300, 2000); ok {
		t.Error("a free scroll view snapped")
	}
	if r := (&ScrollBehavior{}).Deceleration(); r != driver.DecelerationNormal {
		t.Errorf("rate %v", r)
	}
	if r := paging.Deceleration(); r != DecelerationFast {
		t.Errorf("paging rate %v", r)
	}
	if p := driver.Projected(100, 1, driver.DecelerationNormal); p < 590 || p > 600 {
		t.Errorf("projected %v", p)
	}
}

func TestScrollConfigEventsAndCommands(t *testing.T) {
	var ref Ref
	var snapped *Signal[bool]
	var refreshing *Signal[bool]
	var zoom *Signal[float32]
	var events []string
	var last ScrollEvent
	record := func(name string) func(ScrollEvent) {
		return func(e ScrollEvent) { events = append(events, name); last = e }
	}
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		snapped = NewSignal(cx, false)
		refreshing = NewSignal(cx, false)
		zoom = NewSignal(cx, float32(1))
		return Scroll{
			TestID: "scroll",
			Ref:    &ref,
			BehaviorFunc: func() ScrollBehavior {
				if snapped.Get() {
					return ScrollBehavior{SnapToOffsets: []Points{0, 300}, ContentInset: Edges{Top: 20}}
				}
				return ScrollBehavior{Paging: true}
			},
			ContentOffset:     Point{Y: 120},
			Refreshing:        refreshing.Get,
			OnRefresh:         func() { events = append(events, "refresh"); refreshing.Set(true) },
			OnScroll:          record("scroll"),
			OnScrollDragStart: record("drag"),
			OnMomentumEnd:     record("rest"),
			MaxZoomScale:      4,
			ZoomScaleFunc:     zoom.Get,
			Style:             Style{Height: Pt(200)},
			Children:          []Node{View{Style: Style{Height: Pt(1000)}}},
		}
	})
	b := h.commit()
	var cfg *driver.ScrollConfig
	var mask driver.EventMask
	var scrollTo []driver.MountItem
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemScrollConfig:
			cfg = it.Scroll
		case driver.ItemEvents:
			if it.ID == h.find("scroll").id {
				mask = it.Events
			}
		case driver.ItemScrollTo:
			scrollTo = append(scrollTo, it)
		}
	}
	if cfg == nil || !cfg.Behavior.Paging || cfg.MinZoomScale != 1 || cfg.MaxZoomScale != 4 || cfg.ZoomScale != 1 ||
		!cfg.Refreshable || cfg.Refreshing || !cfg.Zooms() {
		t.Fatalf("config %+v", cfg)
	}
	if !mask.Has(driver.EventScroll) || !mask.Has(driver.EventScrollDragStart) || !mask.Has(driver.EventMomentumEnd) ||
		!mask.Has(driver.EventRefresh) || mask.Has(driver.EventScrollDragEnd) || mask.Has(driver.EventMomentumStart) {
		t.Fatalf("mask %b", mask)
	}
	if len(scrollTo) != 1 || scrollTo[0].Point != (Point{Y: 120}) || scrollTo[0].Animated {
		t.Fatalf("initial offset %+v", scrollTo)
	}
	if k := kinds(h.commit()); k[driver.ItemScrollConfig] != 0 {
		t.Fatalf("resent an unchanged config: %v", k)
	}

	snapped.Set(true)
	zoom.Set(9)
	b = h.commit()
	cfg = nil
	for _, it := range b.Items {
		if it.Kind == driver.ItemScrollConfig {
			cfg = it.Scroll
		}
	}
	if cfg == nil || cfg.Behavior.Paging || len(cfg.Behavior.SnapToOffsets) != 2 || cfg.Behavior.ContentInset.Top != 20 ||
		cfg.ZoomScale != 4 {
		t.Fatalf("changed config %+v", cfg)
	}

	id := h.find("scroll").id
	h.tree.Dispatch(driver.Event{Kind: driver.EventScrollDragStart, ID: id, Point: Point{Y: 130}})
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: id, Point: Point{X: 10, Y: 300}, Value: 2, Count: 1})
	h.tree.Dispatch(driver.Event{Kind: driver.EventMomentumEnd, ID: id, Point: Point{X: 10, Y: 300}})
	want := ScrollEvent{Offset: Point{X: 10, Y: 300}, ContentSize: Size{Width: 600, Height: 2000},
		Size: Size{Width: 300, Height: 200}, ZoomScale: 2}
	if len(events) != 3 || events[0] != "drag" || events[1] != "scroll" || events[2] != "rest" || last != want {
		t.Fatalf("events %v, last %+v, want %+v", events, last, want)
	}

	h.tree.Dispatch(driver.Event{Kind: driver.EventRefresh, ID: id, Point: Point{Y: -70}})
	b = h.commit()
	cfg = nil
	for _, it := range b.Items {
		if it.Kind == driver.ItemScrollConfig {
			cfg = it.Scroll
		}
	}
	if events[len(events)-1] != "refresh" || cfg == nil || !cfg.Refreshing {
		t.Fatalf("refresh: events %v, config %+v", events, cfg)
	}
	// A refresh the app does not take is sent back as not refreshing.
	refreshing.Set(false)
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventRefresh, ID: id})
	refreshing.Set(false)
	b = h.commit()
	if k := kinds(b); k[driver.ItemScrollConfig] != 1 {
		t.Fatalf("refresh not answered: %v", k)
	}

	ref.ZoomTo(10, &Point{X: 50, Y: 60}, true)
	ref.ZoomTo(0, nil, false)
	var zooms []*driver.ZoomRequest
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemZoomTo {
			zooms = append(zooms, it.Zoom)
			if it.Animated != (len(zooms) == 1) {
				t.Fatalf("zoom %d animated %v", len(zooms), it.Animated)
			}
		}
	}
	if len(zooms) != 2 || zooms[0].Scale != 4 || *zooms[0].Center != (Point{X: 50, Y: 60}) ||
		zooms[1].Scale != 1 || zooms[1].Center != nil {
		t.Fatalf("zooms %+v", zooms)
	}
}

func TestScrollEventsFollowTheThrottle(t *testing.T) {
	var got []Points
	h := newHost(t, Size{Width: 300, Height: 200}, func(cx *Scope) Node {
		return Scroll{TestID: "scroll", ScrollEventThrottle: 100 * time.Millisecond,
			OnScroll: func(e ScrollEvent) { got = append(got, e.Offset.Y) },
			Children: []Node{View{Style: Style{Height: Pt(2000)}}}}
	})
	h.commit()
	id := h.find("scroll").id
	scroll := func(y Points) { h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: id, Point: Point{Y: y}}) }
	scroll(10)
	h.clock.Advance(30 * time.Millisecond)
	scroll(20)
	scroll(30)
	if !slices.Equal(got, []Points{10}) {
		t.Fatalf("within the throttle: reported %v, want [10]", got)
	}
	h.clock.Advance(70 * time.Millisecond)
	h.drain()
	if !slices.Equal(got, []Points{10, 30}) {
		t.Fatalf("after the throttle: reported %v, want [10 30]", got)
	}
	h.clock.Advance(150 * time.Millisecond)
	scroll(40)
	if !slices.Equal(got, []Points{10, 30, 40}) {
		t.Fatalf("a scroll after a quiet interval: reported %v, want [10 30 40]", got)
	}
}

// A report from before the platform applied the scrolls the app asked for
// is stale; one after them is not.
func TestScrollEventsMarkStaleReports(t *testing.T) {
	type report struct {
		y     Points
		stale bool
	}
	var got []report
	ref := &Ref{}
	h := newHost(t, Size{Width: 300, Height: 200}, func(cx *Scope) Node {
		return Scroll{Ref: ref, OnScroll: func(e ScrollEvent) { got = append(got, report{e.Offset.Y, e.Stale}) },
			Children: []Node{View{Style: Style{Height: Pt(2000)}}}}
	})
	h.commit()
	scroll := func(y Points, applied uint32) {
		h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: ref.n.id, Point: Point{Y: y}, Count: applied})
	}
	scroll(5, 0)
	ref.ScrollTo(Point{Y: 100}, false)
	h.commit()
	ref.ScrollTo(Point{Y: 200}, false)
	h.commit()
	scroll(5, 0)
	scroll(100, 1)
	scroll(200, 2)
	scroll(150, 2)
	want := []report{{5, false}, {5, true}, {100, true}, {200, false}, {150, false}}
	if !slices.Equal(got, want) {
		t.Fatalf("reports %v, want %v", got, want)
	}
}

type inputTestBar struct {
	watched *[]ScrollEvent
}

func (b inputTestBar) Scrollbar(cx *Scope, target *Ref, horizontal bool) Node {
	cx.WatchScroll(func() *Ref { return target }, func(e ScrollEvent) {
		*b.watched = append(*b.watched, e)
	})
	return View{TestID: "input-bar", Style: Style{Position: Absolute, Right: Pt(0), Top: Pt(0), Bottom: Pt(0), Width: Pt(8)}}
}

func TestMultilineInputScrollAndScrollbar(t *testing.T) {
	var ref Ref
	var events []ScrollEvent
	var watched []ScrollEvent
	var multiline *Signal[bool]
	text := strings.Repeat("line of text\n", 20)

	h := newHost(t, Size{Width: 240, Height: 200}, func(cx *Scope) Node {
		multiline = NewSignal(cx, true)
		return Show{
			When: multiline.Get,
			Then: func(*Scope) Node {
				return Input{
					TestID:       "notes",
					Ref:          &ref,
					Multiline:    true,
					DefaultValue: text,
					Bar:          inputTestBar{watched: &watched},
					OnScroll:     func(e ScrollEvent) { events = append(events, e) },
					Style:        Style{Width: Pt(200), Height: Pt(80), Padding: All(6)},
				}
			},
			Else: func(*Scope) Node {
				return Input{
					TestID:       "single",
					DefaultValue: "one line",
					Bar:          inputTestBar{watched: &watched},
					Style:        Style{Width: Pt(200), Height: Pt(32)},
				}
			},
		}
	})
	b := h.commit()
	in := h.find("notes")
	if in == nil {
		t.Fatal("missing multiline input")
	}
	if in.ChildCount() != 0 {
		t.Fatalf("multiline input must stay a layout leaf, got ChildCount=%d", in.ChildCount())
	}
	if len(in.nativeChildren()) != 1 || in.input.barHost == nil {
		t.Fatalf("expected barHost as native child of multiline input, got %d", len(in.nativeChildren()))
	}
	var cfg *driver.InputConfig
	var mask driver.EventMask
	for _, it := range b.Items {
		if it.Kind == driver.ItemInput && it.ID == in.id {
			cfg = it.Input
		}
		if it.Kind == driver.ItemEvents && it.ID == in.id {
			mask = it.Events
		}
	}
	if cfg == nil || !cfg.HideIndicator {
		t.Fatalf("expected ItemInput HideIndicator=true when Bar is set, got %+v", cfg)
	}
	if !mask.Has(driver.EventScroll) {
		t.Fatalf("expected EventScroll on multiline input, mask=%b", mask)
	}
	if len(watched) == 0 || watched[len(watched)-1].ContentSize.Height <= watched[len(watched)-1].Size.Height {
		t.Fatalf("expected initial WatchScroll with overflow, got %+v", watched)
	}

	// Programmatic ScrollTo emits ItemScrollTo and reconciles stale/applied reports without loops.
	ref.ScrollTo(Point{Y: 50}, false)
	b = h.commit()
	var scrollItems []driver.MountItem
	for _, it := range b.Items {
		if it.Kind == driver.ItemScrollTo && it.ID == in.id {
			scrollItems = append(scrollItems, it)
		}
	}
	if len(scrollItems) != 1 || scrollItems[0].Point.Y != 50 || scrollItems[0].Animated {
		t.Fatalf("ItemScrollTo = %+v, want Y=50 non-animated", scrollItems)
	}
	// Stale report (Count 0) is marked stale; applied report (Count 1) settles and updates platformScroll.
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: in.id, Point: Point{Y: 10}, Count: 0})
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: in.id, Point: Point{Y: 50}, Size: Size{Width: 200, Height: 340}, Count: 1})
	h.commit()
	if len(events) != 2 || !events[0].Stale || events[1].Stale || events[1].Offset.Y != 50 || events[1].ContentSize.Height != 340 {
		t.Fatalf("OnScroll events = %+v", events)
	}
	if last := watched[len(watched)-1]; last.Offset.Y != 50 || last.ContentSize.Height != 340 {
		t.Fatalf("WatchScroll last = %+v, want Offset.Y=50 ContentSize.Height=340", last)
	}

	// ScrollToEnd scrolls to the bottom of the scrollable height.
	ref.ScrollToEnd(true)
	b = h.commit()
	scrollItems = nil
	for _, it := range b.Items {
		if it.Kind == driver.ItemScrollTo && it.ID == in.id {
			scrollItems = append(scrollItems, it)
		}
	}
	if len(scrollItems) != 1 || scrollItems[0].Point.Y != 260 || !scrollItems[0].Animated {
		t.Fatalf("ScrollToEnd ItemScrollTo = %+v, want Y=260 animated", scrollItems)
	}

	// Single-line inputs never mount a barHost or listen for EventScroll.
	multiline.Set(false)
	b = h.commit()
	single := h.find("single")
	if single == nil || single.input.barHost != nil {
		t.Fatalf("single-line input should not mount a barHost")
	}
	for _, it := range b.Items {
		if it.Kind == driver.ItemEvents && it.ID == single.id && it.Events.Has(driver.EventScroll) {
			t.Fatalf("single-line input should not request EventScroll")
		}
	}
}

func TestSetScrollbarsAppliesToMultilineInputs(t *testing.T) {
	var watched []ScrollEvent
	h := newHost(t, Size{Width: 240, Height: 200}, func(cx *Scope) Node {
		return View{Children: []Node{
			Input{TestID: "a", Multiline: true, DefaultValue: strings.Repeat("row\n", 15), Style: Style{Width: Pt(200), Height: Pt(60)}},
			Input{TestID: "hidden", Multiline: true, HideIndicator: true, DefaultValue: strings.Repeat("row\n", 15), Style: Style{Width: Pt(200), Height: Pt(60)}},
		}}
	})
	h.commit()
	if h.find("a").input.barHost != nil {
		t.Fatal("barHost mounted before SetScrollbars")
	}
	h.root.SetScrollbars(inputTestBar{watched: &watched})
	h.commit()
	if h.find("a").input.barHost == nil || !h.find("a").input.config.HideIndicator {
		t.Fatal("SetScrollbars did not mount barHost or hide indicator on multiline input")
	}
	if h.find("hidden").input.barHost != nil {
		t.Fatal("SetScrollbars mounted barHost on HideIndicator multiline input")
	}
	h.root.SetScrollbars(nil)
	h.commit()
	if h.find("a").input.bar.Get() != nil || h.find("a").input.config.HideIndicator {
		t.Fatal("SetScrollbars(nil) did not clear bar or restore indicator")
	}
}
