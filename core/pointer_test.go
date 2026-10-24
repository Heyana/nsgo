package core

import "testing"

func TestPointerEventsCarryPositionsAndModifiers(t *testing.T) {
	var presses []PressEvent
	var hovers []HoverEvent
	var moves []Point
	var menus []ContextMenuEvent
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		return View{Children: []Node{
			View{Style: Style{Height: Pt(40)}, TestID: "v",
				OnPress:       func(e PressEvent) { presses = append(presses, e) },
				OnPressDown:   func(e PressEvent) { presses = append(presses, e) },
				OnHoverMove:   func(e HoverEvent) { hovers = append(hovers, e) },
				OnMouseMove:   func(p Point) { moves = append(moves, p) },
				OnContextMenu: func(e ContextMenuEvent) { menus = append(menus, e) }},
			View{Style: Style{Height: Pt(40)}, TestID: "plain", OnHoverIn: func() {}},
		}}
	})
	h.commit()
	n := h.find("v")
	m := n.events()
	for _, k := range []EventKind{EventPress, EventPressDown, EventHoverMove, EventMouseMove, EventContextMenu} {
		if !m.Has(k) {
			t.Errorf("events lack %v", k)
		}
	}
	if m := h.find("plain").events(); m.Has(EventHoverMove) || m.Has(EventMouseMove) {
		t.Error("a view without move handlers reports moves")
	}
	shift := Modifiers{Shift: true}
	h.tree.Dispatch(Event{Kind: EventPressDown, ID: n.id, Point: Point{5, 6}, Page: Point{5, 46}, Modifiers: shift, Count: 2})
	h.tree.Dispatch(Event{Kind: EventPress, ID: n.id, Point: Point{5, 6}, Page: Point{5, 46}, Modifiers: shift, Count: 2})
	want := PressEvent{Position: Point{5, 6}, PagePosition: Point{5, 46}, Modifiers: shift, Clicks: 2}
	if len(presses) != 2 || presses[0] != want || presses[1] != want {
		t.Fatalf("presses = %+v, want two of %+v", presses, want)
	}
	h.tree.Dispatch(Event{Kind: EventHoverMove, ID: n.id, Point: Point{1, 2}, Page: Point{1, 42}, Modifiers: Modifiers{Platform: true}})
	if len(hovers) != 1 || hovers[0] != (HoverEvent{Point{1, 2}, Point{1, 42}, Modifiers{Platform: true}}) {
		t.Fatalf("hovers = %+v", hovers)
	}
	h.tree.Dispatch(Event{Kind: EventMouseMove, ID: n.id, Point: Point{3, 4}})
	if len(moves) != 1 || moves[0] != (Point{3, 4}) {
		t.Fatalf("moves = %v", moves)
	}
	h.tree.Dispatch(Event{Kind: EventContextMenu, ID: n.id, Point: Point{7, 8}, Page: Point{7, 48}, Modifiers: Modifiers{Control: true}})
	if len(menus) != 1 || menus[0] != (ContextMenuEvent{Point{7, 8}, Point{7, 48}, Modifiers{Control: true}}) {
		t.Fatalf("menus = %+v", menus)
	}
	if !(Event{Kind: EventHoverMove}).Mergeable() || !(Event{Kind: EventMouseMove}).Mergeable() {
		t.Fatal("mouse moves do not merge")
	}
}

func TestReachFollowsPointerEventsSlopAndArea(t *testing.T) {
	frame := Bounds{Origin: Point{10, 10}, Size: Size{20, 20}}
	area := Bounds{Origin: Point{0, 0}, Size: Size{20, 60}} // children reach 40 points below
	for _, c := range []struct {
		name     string
		props    ViewProps
		p        Point
		self, in bool
	}{
		{"inside", ViewProps{}, Point{15, 15}, true, true},
		{"below, in the area", ViewProps{}, Point{15, 50}, false, true},
		{"beside", ViewProps{}, Point{40, 15}, false, false},
		{"in the slop", ViewProps{HitSlop: All(12)}, Point{40, 15}, true, false},
		{"none", ViewProps{PointerEvents: PointerEventsNone}, Point{15, 15}, false, false},
		{"box-none", ViewProps{PointerEvents: PointerEventsBoxNone}, Point{15, 15}, false, true},
		{"box-only", ViewProps{PointerEvents: PointerEventsBoxOnly}, Point{15, 50}, false, false},
	} {
		self, in := Reach(frame, &c.props, area, c.p)
		if self != c.self || in != c.in {
			t.Errorf("%s: Reach = %v, %v; want %v, %v", c.name, self, in, c.self, c.in)
		}
	}
}

func TestOverflowAreasLetChildrenOutsideTheirParentTakePresses(t *testing.T) {
	var pressed []string
	press := func(name string) func(PressEvent) { return func(PressEvent) { pressed = append(pressed, name) } }
	h := newHost(t, Size{200, 200}, func(cx *Scope) Node {
		return View{Children: []Node{
			View{TestID: "field", Style: Style{Height: Pt(30)}, Children: []Node{
				View{TestID: "menu", OnPress: press("menu"),
					Style: Style{Position: Absolute, Top: Pt(30), Width: Pt(100), Height: Pt(60)}},
			}},
			View{TestID: "below", OnPress: press("below"),
				Style: Style{Position: Absolute, Top: Pt(120), Width: Pt(200), Height: Pt(50)}},
		}}
	})
	b := h.commit()
	field := h.find("field")
	var area Bounds
	for _, it := range b.Items {
		if it.Kind == ItemOverflow && it.ID == field.id {
			area = it.Frame
		}
	}
	if area != (Bounds{Size: Size{200, 90}}) {
		t.Fatalf("field's area = %+v", area)
	}
	if got := h.tree.HitTest(Point{50, 50}); got != h.find("menu").id {
		t.Fatalf("hit at the menu = %d, want the menu", got)
	}
	h.tree.PressAt(Event{Page: Point{50, 50}})
	if len(pressed) != 1 || pressed[0] != "menu" {
		t.Fatalf("pressed %v", pressed)
	}
	if got := h.tree.HitTest(Point{150, 50}); got != 0 {
		t.Fatalf("hit beside the menu = %d, want none", got)
	}
}

func TestPointerEventsNoneLetsPressesThrough(t *testing.T) {
	h := newHost(t, Size{100, 100}, func(cx *Scope) Node {
		return View{Children: []Node{
			View{TestID: "under", OnPress: func(PressEvent) {}, Style: Style{Position: Absolute, Width: Pt(100), Height: Pt(100)}},
			View{TestID: "over", OnPress: func(PressEvent) {}, PointerEvents: PointerEventsNone,
				Style: Style{Position: Absolute, Width: Pt(100), Height: Pt(100)}},
		}}
	})
	h.commit()
	if got := h.tree.HitTest(Point{50, 50}); got != h.find("under").id {
		t.Fatalf("hit = %d, want the view under the one that takes no pointer", got)
	}
}
