//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMounterTabBarOpsSizesAndSelection(t *testing.T) {
	var dispatched []driver.Event
	var batches [][][]any

	bridge := js.Global().Get("Object").New()
	submit := js.FuncOf(func(_ js.Value, args []js.Value) any {
		var ops [][]any
		if err := json.Unmarshal([]byte(args[0].String()), &ops); err != nil {
			t.Errorf("unmarshal ops: %v", err)
			return nil
		}
		batches = append(batches, ops)
		return nil
	})
	defer submit.Release()
	bridge.Set("submit", submit)

	s := &surface{
		bridge: bridge,
		size0:  driver.Size{Width: 390, Height: 844},
		env: driver.Environment{
			SafeArea: driver.Edges{Top: 38, Bottom: 28},
		},
		dispatchFn: func(ev driver.Event) {
			dispatched = append(dispatched, ev)
		},
	}
	m := newMounter(s)
	s.m = m

	icon := &driver.Drawing{
		Ops: []driver.DrawOp{
			{Kind: driver.DrawBeginPath},
			{Kind: driver.DrawRect, X: 4, Y: 4, W: 16, H: 16},
			{Kind: driver.DrawStrokeColor, Color: driver.RGB(0xffffff)},
			{Kind: driver.DrawStroke},
		},
	}

	// Mount root and native TabBar without a frame yet: should emit "c" with KindNative,
	// "tb" with serialized items/badges/colors, and dispatch initial EventNativeSize.
	m.mount(&driver.MountBatch{
		Root: 1,
		Items: []driver.MountItem{
			{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
			{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 390, Height: 844}}},
			{Kind: driver.ItemCreate, ID: 2, View: driver.KindNative},
			{Kind: driver.ItemInsert, ID: 2, Parent: 1},
			{Kind: driver.ItemNative, ID: 2, Native: &driver.NativeConfig{
				Component: driver.TabBarComponent,
				Props: map[string]any{
					"items": []any{
						map[string]any{"title": "Components", "drawing": icon},
						map[string]any{"title": "Basics", "drawing": icon},
						map[string]any{"title": "Settings"},
					},
					"selectedIndex": float64(0),
					"edge":          "bottom",
					"tint":          float64(driver.RGB(0x0a59f7)),
					"badges": driver.TabBadges{
						Items:    []driver.TabBadge{{}, {Text: "5"}, {Dot: true}},
						Language: "en",
					},
				},
			}},
		},
	})

	if len(batches) == 0 {
		t.Fatal("expected mount batch to be submitted")
	}
	var foundCreate, foundTB bool
	var spec tabBarSpec
	for _, op := range batches[0] {
		if op[0] == "c" && op[1] == float64(2) && op[2] == float64(driver.KindNative) {
			foundCreate = true
		}
		if op[0] == "tb" && op[1] == float64(2) {
			foundTB = true
			if err := json.Unmarshal([]byte(op[2].(string)), &spec); err != nil {
				t.Fatalf("unmarshal tb spec: %v", err)
			}
		}
	}
	if !foundCreate || !foundTB {
		t.Fatalf("missing create (%v) or tb (%v) op in %v", foundCreate, foundTB, batches[0])
	}
	if len(spec.Items) != 3 || spec.Items[0].Title != "Components" || spec.Items[0].Drawing == "" {
		t.Fatalf("unexpected tb items: %+v", spec.Items)
	}
	if len(spec.Badges) != 3 || spec.Badges[1].Text != "5" || !spec.Badges[2].Dot {
		t.Fatalf("unexpected tb badges: %+v", spec.Badges)
	}
	if len(dispatched) != 1 || dispatched[0].Kind != driver.EventNativeSize || dispatched[0].ID != 2 || dispatched[0].Size.Height != tabBarHeight {
		t.Fatalf("initial dispatched = %+v, want EventNativeSize{Height:%v}", dispatched, tabBarHeight)
	}

	// Place the tab bar at the bottom of the screen so it overlaps SafeArea.Bottom (28).
	dispatched = nil
	batches = nil
	m.mount(&driver.MountBatch{
		Root: 1,
		Items: []driver.MountItem{
			{Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{
				Origin: driver.Point{X: 0, Y: 844 - tabBarHeight},
				Size:   driver.Size{Width: 390, Height: tabBarHeight},
			}},
		},
	})
	if len(dispatched) != 1 || dispatched[0].Kind != driver.EventNativeSize || dispatched[0].Size.Height != tabBarHeight+28 {
		t.Fatalf("bottom-placed dispatched = %+v, want EventNativeSize{Height:%v}", dispatched, tabBarHeight+28)
	}
	var updatedSpec tabBarSpec
	for _, batch := range batches {
		for _, op := range batch {
			if op[0] == "tb" && op[1] == float64(2) {
				if err := json.Unmarshal([]byte(op[2].(string)), &updatedSpec); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if updatedSpec.InsetBottom != 28 {
		t.Fatalf("updated tb InsetBottom = %v, want 28", updatedSpec.InsetBottom)
	}

	// Tap tab 2 from ArkTS -> nsgoEvent("tabselect", 2, 2, 0, "") routes EventNative{"change", 2}.
	dispatched = nil
	s.event([]js.Value{js.ValueOf("tabselect"), js.ValueOf(2), js.ValueOf(2), js.ValueOf(0), js.ValueOf("")})
	if len(dispatched) != 1 {
		t.Fatalf("tabselect dispatched %d events, want 1", len(dispatched))
	}
	ev := dispatched[0]
	if ev.Kind != driver.EventNative || ev.ID != 2 || ev.Text != "change" || ev.Data != float64(2) {
		t.Fatalf("tabselect event = %+v, want EventNative change=2", ev)
	}

	// Out-of-range tab index is ignored.
	dispatched = nil
	s.event([]js.Value{js.ValueOf("tabselect"), js.ValueOf(2), js.ValueOf(99), js.ValueOf(0), js.ValueOf("")})
	if len(dispatched) != 0 {
		t.Fatalf("out-of-range tabselect dispatched %+v", dispatched)
	}
}

func TestPressTargetRespectsClipAncestor(t *testing.T) {
	m := &mounter{views: map[driver.NodeID]*view{}}
	add := func(id, parent driver.NodeID, x, y, w, h driver.Points, clip, pressable bool) {
		m.order++
		m.views[id] = &view{
			id:        id,
			parent:    parent,
			kind:      driver.KindView,
			order:     m.order,
			pressable: pressable,
			style:     driver.ViewStyle{Opacity: 1, Clip: clip},
			frame:     driver.Bounds{Origin: driver.Point{X: x, Y: y}, Size: driver.Size{Width: w, Height: h}},
		}
	}
	// Root -> clipped container (0,0 300x100) -> child row spilling past container (0,120 300x40),
	// and a sibling below the container (0,120 300x40).
	add(1, 0, 0, 0, 400, 400, false, false)
	add(2, 1, 0, 0, 300, 100, true, false)
	add(3, 2, 0, 20, 300, 40, false, true)
	add(4, 2, 0, 120, 300, 40, false, true)
	add(5, 1, 0, 120, 300, 40, false, true)

	if got := m.pressTarget(driver.Point{X: 50, Y: 40}); got != 3 {
		t.Fatalf("inside clipped container went to %d, want 3", got)
	}
	if got := m.pressTarget(driver.Point{X: 50, Y: 140}); got != 5 {
		t.Fatalf("outside clipped container went to %d, want sibling 5 (not clipped child 4)", got)
	}
}
