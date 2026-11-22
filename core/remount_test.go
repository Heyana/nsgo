package core

import (
	"slices"
	"strconv"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRemountKeepsTheGraphAndRecreatesCurrentNativeState(t *testing.T) {
	renders := 0
	var count *Signal[int]
	h := newHost(t, Size{Width: 240, Height: 120}, func(cx *Scope) Node {
		renders++
		count = NewSignal(cx, 0)
		return View{Style: Style{Padding: All(8), Background: RGB(0x112233)}, Children: []Node{
			Text{ContentFunc: func() string { return strconv.Itoa(count.Get()) }},
			Input{Value: func() string { return "value " + strconv.Itoa(count.Get()) }},
		}}
	})
	defer h.root.Dispose()
	first := h.tree.Commit()
	created := map[driver.NodeID]driver.ViewKind{}
	for _, it := range first.Items {
		if it.Kind == driver.ItemCreate {
			created[it.ID] = it.View
		}
	}
	count.Set(2)
	h.tree.Commit()
	h.tree.Remount()
	b := h.tree.Commit()
	if b == nil || b.Root != first.Root || renders != 1 || count.Get() != 2 {
		t.Fatalf("remount changed app state: batch=%v renders=%d count=%d", b, renders, count.Get())
	}
	var text, input bool
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemCreate:
			if kind, ok := created[it.ID]; !ok || kind != it.View {
				t.Fatalf("remount changed node identity: %+v", it)
			}
			delete(created, it.ID)
		case driver.ItemText:
			text = it.Text.Text == "2"
		case driver.ItemInputValue:
			input = it.Value == "value 2"
		}
	}
	if len(created) != 0 || !text || !input {
		t.Fatalf("remount omitted current native state: nodes=%v text=%v input=%v", created, text, input)
	}
	if b := h.tree.Commit(); b != nil {
		t.Fatalf("remount kept emitting unchanged state: %+v", b)
	}
}

func TestRemountRestoresScrollAndCurrentFocus(t *testing.T) {
	h := newHost(t, Size{Width: 240, Height: 200}, func(cx *Scope) Node {
		return View{Children: []Node{
			Input{TestID: "first", AutoFocus: true},
			Input{TestID: "second"},
			Scroll{TestID: "scroll", MaxZoomScale: 4, Style: Style{Height: Pt(80)}, Children: []Node{View{Style: Style{Height: Pt(400)}}}},
		}}
	})
	defer h.root.Dispose()
	h.commit()
	focused := h.find("second").id
	scroll := h.find("scroll").id
	h.tree.Dispatch(driver.Event{Kind: driver.EventFocus, ID: focused})
	h.tree.Dispatch(driver.Event{Kind: driver.EventScroll, ID: scroll, Point: Point{Y: 600}, Value: 2})
	h.commit()
	h.tree.Remount()
	h.tree.Remount() // Coalesced attachment requests must not duplicate commands.
	b := h.commit()
	var focus []driver.NodeID
	var offset Point
	var zoom float32
	for _, it := range b.Items {
		if it.Kind == driver.ItemFocus {
			focus = append(focus, it.ID)
		}
		if it.Kind == driver.ItemScrollTo && it.ID == scroll {
			offset = it.Point
		}
		if it.Kind == driver.ItemZoomTo && it.ID == scroll {
			zoom = it.Zoom.Scale
		}
	}
	if len(focus) != 1 || focus[0] != focused || offset.Y != 600 || zoom != 2 {
		t.Fatalf("restored focus=%v scroll=%v zoom=%v, want focus %d, offset 600 and zoom 2", focus, offset, zoom, focused)
	}
}

func TestRemountRestoresDrawingsComponentsAndBindings(t *testing.T) {
	program := MustCompileShader(testShader)
	h := newHost(t, Size{Width: 240, Height: 200}, func(cx *Scope) Node {
		x := NewAnimatedValue(cx, 0.5)
		return View{Style: Style{Animated: &AnimatedStyle{Opacity: x}}, Children: []Node{
			Canvas{Style: Style{Width: Pt(20), Height: Pt(20)}, Draw: func(d *Drawing, size Size) { d.FillRect(0, 0, float32(size.Width), float32(size.Height)) }},
			Shader{Program: program, Style: Style{Width: Pt(20), Height: Pt(20)}},
			Native{Component: "badge", Props: map[string]any{"title": "retained", "progress": x}},
			Scroll{Style: Style{Height: Pt(40)}, OffsetValues: AnimatedXY{Y: x}, Children: []Node{View{Style: Style{Height: Pt(200)}}}},
		}}
	})
	defer h.root.Dispose()
	first := h.commit()
	h.tree.Remount()
	b := h.commit()
	for _, kind := range []driver.ItemKind{driver.ItemCanvas, driver.ItemShader, driver.ItemNative} {
		if !slices.ContainsFunc(b.Items, func(it driver.MountItem) bool { return it.Kind == kind }) {
			t.Errorf("remount omitted kind %v", kind)
		}
	}
	var before, after []driver.AnimationOp
	for _, it := range first.Items {
		if it.Kind == driver.ItemAnimation {
			for _, op := range it.Animation {
				if op.Kind == driver.AnimBind || op.Kind == driver.AnimBindScroll {
					before = append(before, op)
				}
			}
		}
	}
	for _, it := range b.Items {
		if it.Kind == driver.ItemNative && (it.Native.Component != "badge" || it.Native.Props["title"] != "retained" || len(it.Native.Bindings) != 1) {
			t.Errorf("native config not restored: %+v", it.Native)
		}
		if it.Kind == driver.ItemAnimation {
			after = append(after, it.Animation...)
		}
	}
	if len(before) != 2 || len(after) != 2 {
		t.Fatalf("binding count before=%d after=%d", len(before), len(after))
	}
	for _, op := range before {
		if !slices.ContainsFunc(after, func(got driver.AnimationOp) bool {
			return got.Kind == op.Kind && got.Node == op.Node && got.View == op.View && got.X == op.X && got.Y == op.Y && got.Prop == op.Prop
		}) {
			t.Errorf("binding not restored: %+v", op)
		}
	}
}

func TestRemountOmitsDeletesForTheDiscardedNativeTree(t *testing.T) {
	var visible *Signal[bool]
	h := newHost(t, Size{Width: 240, Height: 120}, func(cx *Scope) Node {
		visible = NewSignal(cx, true)
		return Show{When: visible.Get, Then: func(*Scope) Node { return Text{Content: "old"} }}
	})
	defer h.root.Dispose()
	h.commit()
	visible.Set(false)
	h.tree.Remount()
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemDelete || it.Kind == driver.ItemRemove {
			t.Fatalf("remount touched a node from the discarded native tree: %+v", it)
		}
	}
}
