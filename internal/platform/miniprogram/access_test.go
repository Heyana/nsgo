package miniprogram

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAccessibilityOpsExposeLabelsStatesAndUpdates(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	ids := []driver.NodeID{2}
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindText},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindInput},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 1},
		// Visible label with LabelPress, resolved name and projected disabled/invalid.
		{Kind: driver.ItemViewProps, ID: 2, ViewProps: &driver.ViewProps{
			Role:       driver.RoleText,
			Label:      "Email",
			LabelFor:   3,
			LabelPress: true,
			Disabled:   true,
			Invalid:    true,
		}},
		{Kind: driver.ItemEvents, ID: 2, Events: 1 << driver.EventPress},
		// Control with resolved LabelledBy name, hint, disabled and invalid.
		{Kind: driver.ItemViewProps, ID: 3, ViewProps: &driver.ViewProps{
			Role:        driver.RoleTextField,
			Label:       "Email",
			LabelledBy:  &ids,
			Description: "Work email",
			Hint:        "Required",
			Disabled:    true,
			Invalid:     true,
		}},
	}})

	axOps := h.ops("ax")
	if len(axOps) != 2 {
		t.Fatalf("ax ops = %v, want 2", axOps)
	}
	var labelOp []any
	if err := json.Unmarshal([]byte(axOps[0]), &labelOp); err != nil {
		t.Fatal(err)
	}
	labelProps := labelOp[1].(map[string]any)
	if labelProps["l"] != "Email" || labelProps["d"] != true || labelProps["inv"] != true {
		t.Fatalf("label ax = %v", labelProps)
	}
	if _, hasRole := labelProps["r"]; hasRole {
		t.Fatalf("visible label must not infer button role: %v", labelProps)
	}

	var fieldOp []any
	if err := json.Unmarshal([]byte(axOps[1]), &fieldOp); err != nil {
		t.Fatal(err)
	}
	fieldProps := fieldOp[1].(map[string]any)
	if fieldProps["l"] != "Email" || fieldProps["h"] != "Work email. Required" || fieldProps["d"] != true || fieldProps["inv"] != true {
		t.Fatalf("field ax = %v", fieldProps)
	}

	// Reactive label update via ItemAccessibilityLabel emits an updated ax op.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemAccessibilityLabel, ID: 3, Value: "Personal email"},
	}})
	axOps = h.ops("ax")
	if len(axOps) != 3 {
		t.Fatalf("after ItemAccessibilityLabel, ax ops = %v, want 3", axOps)
	}
	var updated []any
	if err := json.Unmarshal([]byte(axOps[2]), &updated); err != nil {
		t.Fatal(err)
	}
	if updated[1].(map[string]any)["l"] != "Personal email" {
		t.Fatalf("updated ax = %v", updated)
	}

	// Clearing all accessibility props sends null so attributes are removed.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewProps, ID: 2, ViewProps: &driver.ViewProps{}},
	}})
	axOps = h.ops("ax")
	if len(axOps) != 4 || axOps[3] != "[2,null]" {
		t.Fatalf("cleared ax ops = %v, want [2,null]", axOps)
	}
}

func TestAccessibilityHiddenAndNonAccessibleNodes(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindView},
		{Kind: driver.ItemViewProps, ID: 1, ViewProps: &driver.ViewProps{}},
		{Kind: driver.ItemViewProps, ID: 2, ViewProps: &driver.ViewProps{
			Accessible:     driver.AccessibleNo,
			ElementsHidden: true,
		}},
		{Kind: driver.ItemViewProps, ID: 3, ViewProps: &driver.ViewProps{
			Accessible: driver.AccessibleNo,
		}},
		{Kind: driver.ItemViewProps, ID: 4, ViewProps: &driver.ViewProps{
			Accessible: driver.AccessibleYes,
			Label:      "Grouped card",
		}},
	}})

	// Node 1 has no accessibility props so emits no ax op.
	axOps := h.ops("ax")
	if len(axOps) != 3 {
		t.Fatalf("ax ops = %v, want 3 (nodes 2, 3, 4)", axOps)
	}
	var op2, op3, op4 []any
	_ = json.Unmarshal([]byte(axOps[0]), &op2)
	_ = json.Unmarshal([]byte(axOps[1]), &op3)
	_ = json.Unmarshal([]byte(axOps[2]), &op4)

	p2 := op2[1].(map[string]any)
	if p2["hd"] != true || p2["r"] != "presentation" {
		t.Fatalf("hidden node ax = %v", p2)
	}
	p3 := op3[1].(map[string]any)
	if p3["r"] != "presentation" || p3["hd"] != nil {
		t.Fatalf("non-accessible node ax = %v", p3)
	}
	p4 := op4[1].(map[string]any)
	if p4["r"] != "group" || p4["l"] != "Grouped card" {
		t.Fatalf("grouped node ax = %v", p4)
	}
}
