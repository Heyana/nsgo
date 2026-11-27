package driver

import (
	"slices"
	"testing"
)

func TestCanvasNodePropertiesAndActions(t *testing.T) {
	slider := CanvasNode{
		ID:        "vol",
		TestID:    "vol-test",
		Role:      RoleAdjustable,
		Label:     "Volume",
		Hint:      "Adjusts output volume",
		Value:     50,
		Min:       0,
		Max:       100,
		Step:      5,
		ValueText: "50%",
		Live:      LiveRegionPolite,
	}
	if slider.NodeID() != "vol" {
		t.Fatalf("NodeID = %q, want vol", slider.NodeID())
	}
	if !slider.HasRange() || slider.RangeStep() != 5 {
		t.Fatalf("range=%v step=%v", slider.HasRange(), slider.RangeStep())
	}
	if got := slider.AccessibilityValue(); got != (AccessibilityValue{Min: 0, Max: 100, Now: 50, Text: "50%"}) {
		t.Fatalf("AccessibilityValue = %+v", got)
	}
	if !slider.HasAction(ActionIncrement) || !slider.HasAction(ActionDecrement) || slider.HasAction(ActionActivate) {
		t.Fatalf("slider actions = %b", slider.EffectiveActions())
	}
	vp := CanvasNodeViewProps(slider)
	if vp.Label != "Volume" || vp.Hint != "Adjusts output volume" || vp.TestID != "vol-test" ||
		vp.LiveRegion != LiveRegionPolite || vp.Value.Now != 50 || !vp.HasAction(ActionIncrement) {
		t.Fatalf("ViewProps = %+v", vp)
	}

	// Default step is a tenth of the range when Step is 0.
	slider.Step = 0
	if got := slider.RangeStep(); got != 10 {
		t.Fatalf("default RangeStep = %v, want 10", got)
	}

	// NodeID falls back to TestID when ID is empty.
	fallback := CanvasNode{TestID: "bar-1", Role: RoleButton, Label: "Bar 1"}
	if fallback.NodeID() != "bar-1" {
		t.Fatalf("fallback NodeID = %q, want bar-1", fallback.NodeID())
	}
	if !fallback.HasAction(ActionActivate) {
		t.Fatal("RoleButton should infer activate action")
	}

	// Disabled clears all effective actions even when explicitly listed.
	disabled := CanvasNode{
		ID:       "off",
		Role:     RoleAdjustable,
		Disabled: true,
		Actions:  CanvasActionActivate | CanvasActionIncrement | CanvasActionDecrement,
	}
	if disabled.EffectiveActions() != 0 || disabled.HasAction(ActionIncrement) {
		t.Fatalf("disabled node actions = %b, want 0", disabled.EffectiveActions())
	}

	// Tri-state checked and expanded mapping.
	cb := CanvasNode{
		ID:       "all",
		Role:     RoleCheckbox,
		Checked:  CheckedMixed,
		Expanded: ExpandedFalse,
		Busy:     true,
	}
	if !cb.IsCheckable() {
		t.Fatal("checkbox should be checkable")
	}
	cvp := CanvasNodeViewProps(cb)
	if cvp.Checked || !cvp.Indeterminate || !cvp.Expandable || cvp.Expanded || !cvp.Busy {
		t.Fatalf("checkbox ViewProps = %+v", cvp)
	}
	cb.Checked = CheckedTrue
	cb.Expanded = ExpandedTrue
	cvp = CanvasNodeViewProps(cb)
	if !cvp.Checked || cvp.Indeterminate || !cvp.Expandable || !cvp.Expanded {
		t.Fatalf("checked/expanded ViewProps = %+v", cvp)
	}

	// Explicit actions override role defaults.
	custom := CanvasNode{
		ID:      "pane",
		Role:    RoleGroup,
		Actions: CanvasActionScroll | CanvasActionDismiss,
	}
	if !custom.HasAction(ActionScroll) || !custom.HasAction(ActionDismiss) || custom.HasAction(ActionActivate) {
		t.Fatalf("custom actions = %b", custom.EffectiveActions())
	}
	if !IsStandardAction(ActionScroll) || !IsStandardAction(ActionDismiss) {
		t.Fatal("ActionScroll and ActionDismiss must be standard actions")
	}
}

func TestCanvasLiveChangesAndLookup(t *testing.T) {
	prev := []CanvasNode{
		{ID: "score", Label: "Score", ValueText: "10", Live: LiveRegionPolite},
		{ID: "timer", Label: "Timer", ValueText: "00:30", Live: LiveRegionAssertive},
		{ID: "static", Label: "Health", ValueText: "100"},
	}
	next := []CanvasNode{
		{ID: "score", Label: "Score", ValueText: "20", Live: LiveRegionPolite},
		{ID: "timer", Label: "Timer", ValueText: "00:30", Live: LiveRegionAssertive},
		{ID: "static", Label: "Health", ValueText: "80"},
		{ID: "toast", Label: "Level up", Live: LiveRegionAssertive},
	}
	got := CanvasLiveChanges(prev, next)
	want := []CanvasLiveAnnouncement{{Index: 0, Text: "Score, 20", Live: LiveRegionPolite}}
	if !slices.Equal(got, want) {
		t.Fatalf("CanvasLiveChanges = %+v, want %+v", got, want)
	}
	if idx := FindCanvasNode(next, "toast"); idx != 3 {
		t.Fatalf("FindCanvasNode(toast) = %d, want 3", idx)
	}
	if idx := FindCanvasNode(next, "missing"); idx != -1 {
		t.Fatalf("FindCanvasNode(missing) = %d, want -1", idx)
	}
}
