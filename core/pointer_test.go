package core

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestHitTestsFollowNestedTransformsAndClips(t *testing.T) {
	for _, clipped := range []bool{false, true} {
		t.Run(map[bool]string{false: "overflow", true: "clipped"}[clipped], func(t *testing.T) {
			var got []PressEvent
			var touch, responder Point
			style := Style{Position: Absolute, Left: Pt(100), Top: Pt(100), Width: Pt(40), Height: Pt(40),
				Transform: Transform{Rotate: 90, Scale: 2}}
			if clipped {
				style.Overflow = Hidden
			}
			h := newHost(t, Size{Width: 400, Height: 400}, func(*Scope) Node {
				return View{Children: []Node{View{TestID: "parent", Style: style, Children: []Node{
					View{TestID: "button", Style: Style{Position: Absolute, Left: Pt(5), Top: Pt(5), Width: Pt(20), Height: Pt(20),
						Transform: Transform{TranslateY: -100}}, OnPress: func(e PressEvent) { got = append(got, e) },
						OnTouch:   func(e TouchEvent) { touch = e.Touches[0].Position },
						Responder: &Responder{Claims: []TouchClaim{{}}, OnGrant: func(e ResponderEvent) { responder = e.Position }}},
				}}}}
			})
			h.commit()
			// Child (10, 10) is drawn at (330, 110): first translated above its parent, then rotated and scaled about its center.
			at := Point{X: 330, Y: 110}
			id := h.tree.HitTest(at)
			if clipped {
				if id != 0 {
					t.Fatalf("clipped child hit = %d", id)
				}
				return
			}
			if id != h.find("button").id {
				t.Fatalf("drawn child hit = %d, want button", id)
			}
			h.tree.PressAt(driver.Event{Page: at})
			if len(got) != 1 || math.Abs(float64(got[0].Position.X-10)) > .001 || math.Abs(float64(got[0].Position.Y-10)) > .001 || got[0].PagePosition != at {
				t.Fatalf("press = %+v, want local (10,10) at %+v", got, at)
			}
			if id := h.tree.HitTest(Point{X: 115, Y: 115}); id != 0 {
				t.Fatalf("old layout position hit = %d", id)
			}
			h.find("parent").props.PointerEvents = PointerEventsBoxNone
			if id := h.tree.HitTest(at); id != h.find("button").id {
				t.Fatalf("box-none parent hid drawn child: hit = %d", id)
			}
			h.tree.Dispatch(finger(GestureBegan, at, 0))
			for name, p := range map[string]Point{"touch": touch, "responder": responder} {
				if math.Abs(float64(p.X-10)) > .001 || math.Abs(float64(p.Y-10)) > .001 {
					t.Errorf("%s local = %+v, want (10,10)", name, p)
				}
			}
			h.tree.Dispatch(finger(GestureEnded, at, 10))
		})
	}
}

func TestOverflowFlagsChildrenTransformedOutOfTheirParent(t *testing.T) {
	var shift *Signal[Points]
	var clip *Signal[bool]
	pressed := 0
	h := newHost(t, Size{Width: 400, Height: 400}, func(cx *Scope) Node {
		shift, clip = NewSignal(cx, Points(0)), NewSignal(cx, false)
		return View{Children: []Node{
			View{TestID: "parent", StyleFunc: func() Style {
				s := Style{Position: Absolute, Left: Pt(100), Top: Pt(100), Width: Pt(40), Height: Pt(40)}
				if clip.Get() {
					s.Overflow = Hidden
				}
				return s
			}, Children: []Node{
				View{TestID: "child", OnPress: func(PressEvent) { pressed++ }, StyleFunc: func() Style {
					return Style{Width: Pt(20), Height: Pt(20), Transform: Transform{TranslateY: shift.Get()}}
				}},
			}},
		}}
	})
	flag := func(b *driver.MountBatch) (sent, transformed bool) {
		for _, it := range b.Items {
			if it.Kind == driver.ItemOverflow && it.ID == h.find("parent").id {
				sent, transformed = true, it.Transformed
			}
		}
		return sent, transformed
	}
	// Children laid out inside an untransformed parent are not searched outside it.
	if sent, _ := flag(h.commit()); sent || h.find("parent").sent.transformed {
		t.Fatal("untransformed children flagged")
	}
	shift.Set(-100)
	if sent, transformed := flag(h.commit()); !sent || !transformed {
		t.Fatalf("transformed child: ItemOverflow sent %v, transformed %v", sent, transformed)
	}
	if h.tree.PressAt(driver.Event{Page: Point{X: 110, Y: 10}}); pressed != 1 {
		t.Fatalf("presses where the child is drawn = %d", pressed)
	}
	// A transition back to none can still draw it elsewhere, so the flag stays.
	shift.Set(0)
	if sent, _ := flag(h.commit()); sent || !h.find("parent").sent.transformed {
		t.Fatal("the flag went with the transform")
	}
	clip.Set(true)
	if sent, transformed := flag(h.commit()); !sent || transformed {
		t.Fatalf("clipped parent: ItemOverflow sent %v, transformed %v", sent, transformed)
	}
}

func TestPointerEventsCarryPositionsAndModifiers(t *testing.T) {
	var presses []PressEvent
	var moves []MouseEvent
	var menus []ContextMenuEvent
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		return View{Children: []Node{
			View{Style: Style{Height: Pt(40)}, TestID: "v",
				OnPress:       func(e PressEvent) { presses = append(presses, e) },
				OnPressDown:   func(e PressEvent) { presses = append(presses, e) },
				OnMouseMove:   func(e MouseEvent) { moves = append(moves, e) },
				OnContextMenu: func(e ContextMenuEvent) { menus = append(menus, e) }},
			View{Style: Style{Height: Pt(40)}, TestID: "plain", OnMouseEnter: func(MouseEvent) {}},
		}}
	})
	h.commit()
	n := h.find("v")
	m := n.events()
	for _, k := range []driver.EventKind{driver.EventPress, driver.EventPressDown, driver.EventMouseMove, driver.EventContextMenu} {
		if !m.Has(k) {
			t.Errorf("events lack %v", k)
		}
	}
	if m := h.find("plain").events(); m.Has(driver.EventMouseMove) {
		t.Error("a view without move handlers reports moves")
	}
	shift := Modifiers{Shift: true}
	h.tree.Dispatch(driver.Event{Kind: driver.EventPressDown, ID: n.id, Point: Point{X: 5, Y: 6}, Page: Point{X: 5, Y: 46}, Modifiers: shift, Count: 2})
	h.tree.Dispatch(driver.Event{Kind: driver.EventPress, ID: n.id, Point: Point{X: 5, Y: 6}, Page: Point{X: 5, Y: 46}, Modifiers: shift, Count: 2})
	want := PressEvent{Position: Point{X: 5, Y: 6}, PagePosition: Point{X: 5, Y: 46}, Modifiers: shift, Clicks: 2}
	if len(presses) != 2 || presses[0] != want || presses[1] != want {
		t.Fatalf("presses = %+v, want two of %+v", presses, want)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseMove, ID: n.id, Point: Point{X: 1, Y: 2}, Page: Point{X: 1, Y: 42},
		Modifiers: Modifiers{Platform: true}, Button: MouseButtonLeft})
	if len(moves) != 1 || moves[0] != (MouseEvent{Point{X: 1, Y: 2}, Point{X: 1, Y: 42}, Modifiers{Platform: true}, MouseButtonLeft}) {
		t.Fatalf("moves = %+v", moves)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventContextMenu, ID: n.id, Point: Point{X: 7, Y: 8}, Page: Point{X: 7, Y: 48}, Modifiers: Modifiers{Control: true}})
	if len(menus) != 1 || menus[0] != (ContextMenuEvent{Point{X: 7, Y: 8}, Point{X: 7, Y: 48}, Modifiers{Control: true}}) {
		t.Fatalf("menus = %+v", menus)
	}
	if !(driver.Event{Kind: driver.EventMouseMove}).Mergeable() {
		t.Fatal("mouse moves do not merge")
	}
}

func TestMouseEnterAndLeaveSayWhereAndPressesCountOne(t *testing.T) {
	var got []MouseEvent
	var press PressEvent
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		return View{TestID: "v", OnPress: func(e PressEvent) { press = e },
			OnMouseEnter: func(e MouseEvent) { got = append(got, e) },
			OnMouseLeave: func(e MouseEvent) { got = append(got, e) }}
	})
	h.commit()
	n := h.find("v")
	in := MouseEvent{Position: Point{X: 1, Y: 2}, PagePosition: Point{X: 3, Y: 4}, Modifiers: Modifiers{Shift: true}}
	out := MouseEvent{Position: Point{X: -1, Y: 2}, PagePosition: Point{X: 1, Y: 4}, PressedButton: MouseButtonRight}
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseEnter, ID: n.id, Point: in.Position, Page: in.PagePosition, Modifiers: in.Modifiers})
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseLeave, ID: n.id, Point: out.Position, Page: out.PagePosition, Button: out.PressedButton})
	if len(got) != 2 || got[0] != in || got[1] != out {
		t.Fatalf("got %+v", got)
	}
	// A press from the keyboard or a screen reader knows no pointer.
	h.tree.Dispatch(driver.Event{Kind: driver.EventPress, ID: n.id})
	if press != (PressEvent{Clicks: 1}) {
		t.Fatalf("press = %+v", press)
	}
}

func TestReachFollowsPointerEventsSlopAndArea(t *testing.T) {
	frame := Bounds{Origin: Point{X: 10, Y: 10}, Size: Size{Width: 20, Height: 20}}
	area := Bounds{Origin: Point{X: 0, Y: 0}, Size: Size{Width: 20, Height: 60}} // children reach 40 points below
	for _, c := range []struct {
		name     string
		props    driver.ViewProps
		p        Point
		self, in bool
	}{
		{"inside", driver.ViewProps{}, Point{X: 15, Y: 15}, true, true},
		{"below, in the area", driver.ViewProps{}, Point{X: 15, Y: 50}, false, true},
		{"beside", driver.ViewProps{}, Point{X: 40, Y: 15}, false, false},
		{"in the slop", driver.ViewProps{HitSlop: All(12)}, Point{X: 40, Y: 15}, true, false},
		{"none", driver.ViewProps{PointerEvents: PointerEventsNone}, Point{X: 15, Y: 15}, false, false},
		{"box-none", driver.ViewProps{PointerEvents: PointerEventsBoxNone}, Point{X: 15, Y: 15}, false, true},
		{"box-only", driver.ViewProps{PointerEvents: PointerEventsBoxOnly}, Point{X: 15, Y: 50}, false, false},
	} {
		self, in := driver.Reach(frame, &c.props, area, c.p)
		if self != c.self || in != c.in {
			t.Errorf("%s: Reach = %v, %v; want %v, %v", c.name, self, in, c.self, c.in)
		}
	}
}

func TestOverflowAreasLetChildrenOutsideTheirParentTakePresses(t *testing.T) {
	var pressed []string
	press := func(name string) func(PressEvent) { return func(PressEvent) { pressed = append(pressed, name) } }
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
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
		if it.Kind == driver.ItemOverflow && it.ID == field.id {
			area = it.Frame
		}
	}
	if area != (Bounds{Size: Size{Width: 200, Height: 90}}) {
		t.Fatalf("field's area = %+v", area)
	}
	if got := h.tree.HitTest(Point{X: 50, Y: 50}); got != h.find("menu").id {
		t.Fatalf("hit at the menu = %d, want the menu", got)
	}
	h.tree.PressAt(driver.Event{Page: Point{X: 50, Y: 50}})
	if len(pressed) != 1 || pressed[0] != "menu" {
		t.Fatalf("pressed %v", pressed)
	}
	if got := h.tree.HitTest(Point{X: 150, Y: 50}); got != 0 {
		t.Fatalf("hit beside the menu = %d, want none", got)
	}
}

func TestPointerEventsNoneLetsPressesThrough(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		return View{Children: []Node{
			View{TestID: "under", OnPress: func(PressEvent) {}, Style: Style{Position: Absolute, Width: Pt(100), Height: Pt(100)}},
			View{TestID: "over", OnPress: func(PressEvent) {}, PointerEvents: PointerEventsNone,
				Style: Style{Position: Absolute, Width: Pt(100), Height: Pt(100)}},
		}}
	})
	h.commit()
	if got := h.tree.HitTest(Point{X: 50, Y: 50}); got != h.find("under").id {
		t.Fatalf("hit = %d, want the view under the one that takes no pointer", got)
	}
}

func TestMouseButtonDownAndUpDeliverAllFiveButtons(t *testing.T) {
	var downs, ups []MouseButtonEvent
	var alerted int
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		return View{Children: []Node{
			View{TestID: "pad",
				OnMouseDown: func(e MouseButtonEvent) { downs = append(downs, e) },
				OnMouseUp:   func(e MouseButtonEvent) { ups = append(ups, e) }},
			View{TestID: "upOnly", OnMouseUp: func(e MouseButtonEvent) { ups = append(ups, e) }},
			View{TestID: "plain"},
		}}
	})
	b := h.commit()
	pad, upOnly, plain := h.find("pad"), h.find("upOnly"), h.find("plain")
	masks := map[driver.NodeID]driver.EventMask{}
	for _, it := range b.Items {
		if it.Kind == driver.ItemEvents {
			masks[it.ID] = it.Events
		}
	}
	if !masks[pad.id].Has(driver.EventMouseDown) || !masks[upOnly.id].Has(driver.EventMouseDown) || masks[plain.id].Has(driver.EventMouseDown) {
		t.Fatalf("masks: pad=%v upOnly=%v plain=%v", masks[pad.id].Has(driver.EventMouseDown), masks[upOnly.id].Has(driver.EventMouseDown), masks[plain.id].Has(driver.EventMouseDown))
	}
	// OnMouseDown / OnMouseUp must not turn a plain view into a pressable Tab stop.
	if pad.takesPresses() || upOnly.takesPresses() {
		t.Fatal("OnMouseDown/OnMouseUp turned a view into a pressable")
	}

	buttons := []MouseButton{MouseButtonLeft, MouseButtonRight, MouseButtonMiddle, MouseButtonBack, MouseButtonForward}
	for i, btn := range buttons {
		mods := Modifiers{Shift: i%2 == 1}
		h.tree.Dispatch(driver.Event{Kind: driver.EventMouseDown, ID: pad.id, Point: Point{X: 10, Y: 20}, Page: Point{X: 30, Y: 40}, Modifiers: mods, Button: btn, Count: uint32(i + 1)})
		h.tree.Dispatch(driver.Event{Kind: driver.EventMouseUp, ID: pad.id, Point: Point{X: 11, Y: 21}, Page: Point{X: 31, Y: 41}, Modifiers: mods, Button: btn, Count: uint32(i + 1)})
	}
	if len(downs) != 5 || len(ups) != 5 {
		t.Fatalf("downs=%d ups=%d, want 5 each", len(downs), len(ups))
	}
	for i, btn := range buttons {
		mods := Modifiers{Shift: i%2 == 1}
		wantDown := MouseButtonEvent{Position: Point{X: 10, Y: 20}, PagePosition: Point{X: 30, Y: 40}, Modifiers: mods, Button: btn, Clicks: i + 1}
		wantUp := MouseButtonEvent{Position: Point{X: 11, Y: 21}, PagePosition: Point{X: 31, Y: 41}, Modifiers: mods, Button: btn, Clicks: i + 1}
		if downs[i] != wantDown || ups[i] != wantUp {
			t.Fatalf("button %s: down=%+v up=%+v", btn, downs[i], ups[i])
		}
	}
	// A view with only OnMouseUp hears ups and ignores downs.
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseDown, ID: upOnly.id, Button: MouseButtonRight})
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseUp, ID: upOnly.id, Button: MouseButtonRight, Count: 1})
	if len(ups) != 6 || ups[5].Button != MouseButtonRight || ups[5].Clicks != 1 {
		t.Fatalf("upOnly: ups=%+v", ups)
	}
	// Focus-loss release carries Clicks == 0.
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseDown, ID: pad.id, Button: MouseButtonMiddle})
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseUp, ID: pad.id, Button: MouseButtonMiddle})
	if len(ups) != 7 || ups[6].Clicks != 0 || ups[6].Button != MouseButtonMiddle {
		t.Fatalf("focus-loss release: %+v", ups[len(ups)-1])
	}
	// EventAlert shares EventMouseDown's kind, distinguished by Button == MouseButtonNone.
	h.tree.root.ShowAlert(AlertConfig{Title: "Hi", Actions: []AlertAction{{Label: "OK"}}}, func(i int) { alerted = i + 1 })
	b = h.commit()
	var alertID uint32
	for _, it := range b.Items {
		if it.Kind == driver.ItemAlert && it.Alert != nil {
			alertID = it.Alert.ID
		}
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventAlert, Count: alertID, Index: 0})
	if alerted != 1 {
		t.Fatalf("EventAlert callback ran %d, want 1", alerted)
	}
}

func TestMouseButtonsDownTracksHeldButtons(t *testing.T) {
	var renders int
	var root *Scope
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		root = cx
		return Text{TestID: "held", ContentFunc: func() string {
			renders++
			return fmt.Sprintf("%v left=%v right=%v none=%v",
				MouseButtonsDown(cx),
				MouseButtonHeld(cx, MouseButtonLeft),
				MouseButtonHeld(cx, MouseButtonRight),
				MouseButtonHeld(cx, MouseButtonNone))
		}}
	})
	b := h.commit()
	asked := false
	for _, it := range b.Items {
		if it.Kind == driver.ItemKeymap && it.HeldMouseButtons {
			asked = true
		}
	}
	if !asked {
		t.Fatal("first commit did not set HeldMouseButtons on ItemKeymap")
	}
	shows := func(want string) {
		t.Helper()
		h.commit()
		if got := h.find("held").text.content; got != want {
			t.Fatalf("shows %q, want %q", got, want)
		}
	}
	shows("[] left=false right=false none=false")
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseDown, Button: MouseButtonRight})
	shows("[right] left=false right=true none=false")
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseDown, Button: MouseButtonLeft})
	// Returned in canonical button order (left before right).
	shows("[left right] left=true right=true none=false")
	before := renders
	// Duplicate down for an already-held button does not rerender.
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseDown, Button: MouseButtonLeft})
	h.commit()
	if renders != before {
		t.Fatalf("duplicate down rendered %d more times", renders-before)
	}
	var framed []MouseButton
	root.RequestAnimationFrame(func(time.Time) { framed = MouseButtonsDown(root) })
	h.frame()
	if !slices.Equal(framed, []MouseButton{MouseButtonLeft, MouseButtonRight}) {
		t.Fatalf("frame callback read %v", framed)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseUp, Button: MouseButtonLeft})
	shows("[right] left=false right=true none=false")
	h.tree.Dispatch(driver.Event{Kind: driver.EventMouseUp, Button: MouseButtonRight})
	shows("[] left=false right=false none=false")
}

func TestMouseButtonsDownFromFrameAsksPlatform(t *testing.T) {
	var root *Scope
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		root = cx
		return View{}
	})
	h.commit()
	root.RequestAnimationFrame(func(time.Time) { MouseButtonsDown(root) })
	h.frame()
	asked := false
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemKeymap && it.HeldMouseButtons {
			asked = true
		}
	}
	if !asked {
		t.Fatal("frame read did not commit HeldMouseButtons on ItemKeymap")
	}
}
