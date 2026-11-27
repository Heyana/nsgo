package core

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestCanvasNodeAccessibilityActionsAndFocus(t *testing.T) {
	var ref Ref
	var events []CanvasActionEvent
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		return Canvas{
			TestID: "hud",
			Ref:    &ref,
			Style:  Style{Width: Pt(200), Height: Pt(100)},
			Draw: func(d *Drawing, _ Size) {
				d.Node(CanvasNode{
					ID:        "vol",
					Bounds:    Bounds{Origin: Point{X: 10, Y: 10}, Size: Size{Width: 100, Height: 24}},
					Role:      RoleAdjustable,
					Label:     "Volume",
					Hint:      "Swipe up or down",
					Value:     50,
					Min:       0,
					Max:       100,
					Step:      10,
					ValueText: "50%",
				})
				d.Node(CanvasNode{
					ID:       "muted",
					Bounds:   Bounds{Origin: Point{X: 10, Y: 40}, Size: Size{Width: 80, Height: 24}},
					Role:     RoleButton,
					Label:    "Disabled action",
					Disabled: true,
				})
				d.Node(CanvasNode{
					ID:      "list",
					Bounds:  Bounds{Origin: Point{X: 10, Y: 70}, Size: Size{Width: 120, Height: 24}},
					Role:    RoleGroup,
					Label:   "Inventory",
					Actions: CanvasActionScroll | CanvasActionDismiss,
				})
			},
			OnAccessibilityAction: func(ev CanvasActionEvent) {
				events = append(events, ev)
			},
		}
	})
	b := h.commit()
	id := h.find("hud").id
	var hasEvent bool
	for _, it := range b.Items {
		if it.Kind == driver.ItemEvents && it.ID == id && it.Events.Has(driver.EventAccessibilityAction) {
			hasEvent = true
		}
	}
	if !hasEvent {
		t.Fatal("canvas with OnAccessibilityAction did not subscribe to EventAccessibilityAction")
	}

	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, CanvasNode: "vol", Text: ActionIncrement, Value: 60, HasValue: true})
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, CanvasNode: "vol", Text: ActionDecrement})
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, CanvasNode: "list", Text: ActionScroll})
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, CanvasNode: "list", Text: ActionDismiss})
	// Disabled node, unsupported action, and unknown node must be dropped.
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, CanvasNode: "muted", Text: ActionActivate})
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, CanvasNode: "vol", Text: ActionDismiss})
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, CanvasNode: "missing", Text: ActionActivate})

	want := []CanvasActionEvent{
		{NodeID: "vol", Action: ActionIncrement, Value: 60, HasValue: true},
		{NodeID: "vol", Action: ActionDecrement},
		{NodeID: "list", Action: ActionScroll},
		{NodeID: "list", Action: ActionDismiss},
	}
	if !slices.Equal(events, want) {
		t.Fatalf("events = %+v, want %+v", events, want)
	}

	// Move accessibility focus to a specific canvas node by ID.
	ref.SetAccessibilityFocusNode("vol")
	b = h.commit()
	var focusItem *driver.MountItem
	for i := range b.Items {
		if b.Items[i].Kind == driver.ItemAccessibilityFocus {
			focusItem = &b.Items[i]
		}
	}
	if focusItem == nil || focusItem.ID != id || focusItem.Value != "vol" {
		t.Fatalf("focus item = %+v, want ID=%d Value=vol", focusItem, id)
	}

	// Empty node ID is a no-op.
	ref.SetAccessibilityFocusNode("")
	if k := kinds(h.commit()); k[driver.ItemAccessibilityFocus] != 0 {
		t.Fatalf("empty SetAccessibilityFocusNode emitted %v", k)
	}
}
