//go:build darwin && !ios

package macos

import (
	"slices"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestCanvasNodeAccessibility(t *testing.T) { onMain(func() { testCanvasNodeAccessibility(t) }) }

func testCanvasNodeAccessibility(t *testing.T) {
	loadAppKit()
	var reported []driver.Event
	eng := engine.Start(actionEngine{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{Event: func(_ uint32, e driver.Event) {
		if e.Kind == driver.EventAccessibilityAction {
			reported = append(reported, e)
		}
	}})
	defer eng.Close()

	s := &surface{engine: eng, animator: driver.NewAnimator(func(driver.Event) {})}
	v := &view{id: 11, kind: driver.KindCanvas, surface: s, native: objc.ID(classView).Send(selNew)}
	s.views = make([]*view, 12)
	s.views[11] = v
	v.events = 1 << driver.EventAccessibilityAction
	defer func() {
		v.releaseCanvasNodes()
		v.native.Send(selRelease)
	}()

	nodes := []driver.CanvasNode{
		{
			ID:        "vol",
			TestID:    "volume-slider",
			Bounds:    driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 100, Height: 24}},
			Role:      driver.RoleAdjustable,
			Label:     "Volume",
			Hint:      "Adjusts level",
			Value:     40,
			Min:       0,
			Max:       100,
			Step:      10,
			ValueText: "40%",
		},
		{
			ID:       "cb",
			Bounds:   driver.Bounds{Origin: driver.Point{X: 10, Y: 40}, Size: driver.Size{Width: 80, Height: 24}},
			Role:     driver.RoleCheckbox,
			Label:    "All",
			Checked:  driver.CheckedMixed,
			Expanded: driver.ExpandedTrue,
			Busy:     true,
		},
		{
			ID:      "feed",
			Bounds:  driver.Bounds{Origin: driver.Point{X: 10, Y: 70}, Size: driver.Size{Width: 100, Height: 24}},
			Role:    driver.RoleDialog,
			Label:   "Log",
			Actions: driver.CanvasActionScroll | driver.CanvasActionDismiss,
		},
		{
			ID:       "off",
			Bounds:   driver.Bounds{Origin: driver.Point{X: 10, Y: 100}, Size: driver.Size{Width: 80, Height: 24}},
			Role:     driver.RoleButton,
			Label:    "Locked",
			Disabled: true,
		},
	}
	v.setCanvasNodes(nodes)
	if len(v.axNodes) != 4 {
		t.Fatalf("got %d axNodes, want 4", len(v.axNodes))
	}

	vol := v.axNodes[0]
	if got := apple.GoString(vol.Send(sel("accessibilityRole"))); got != "AXSlider" {
		t.Errorf("slider role = %q, want AXSlider", got)
	}
	if got := apple.GoString(vol.Send(sel("accessibilityLabel"))); got != "Volume" {
		t.Errorf("slider label = %q, want Volume", got)
	}
	if got := apple.GoString(vol.Send(sel("accessibilityHelp"))); got != "Adjusts level" {
		t.Errorf("slider help = %q, want Adjusts level", got)
	}
	if got := apple.GoString(vol.Send(sel("accessibilityIdentifier"))); got != "volume-slider" {
		t.Errorf("slider id = %q, want volume-slider", got)
	}
	if got := apple.GoString(vol.Send(sel("accessibilityValueDescription"))); got != "40%" {
		t.Errorf("slider value description = %q, want 40%%", got)
	}
	if got := objc.Send[float64](vol.Send(sel("accessibilityValue")), sel("doubleValue")); got != 40 {
		t.Errorf("slider value = %v, want 40", got)
	}
	if got := objc.Send[float64](vol.Send(sel("accessibilityMinValue")), sel("doubleValue")); got != 0 {
		t.Errorf("slider min = %v, want 0", got)
	}
	if got := objc.Send[float64](vol.Send(sel("accessibilityMaxValue")), sel("doubleValue")); got != 100 {
		t.Errorf("slider max = %v, want 100", got)
	}
	if !objc.Send[bool](vol, sel("isAccessibilitySelectorAllowed:"), sel("accessibilityPerformIncrement")) ||
		!objc.Send[bool](vol, sel("isAccessibilitySelectorAllowed:"), sel("accessibilityPerformDecrement")) ||
		objc.Send[bool](vol, sel("isAccessibilitySelectorAllowed:"), sel("accessibilityPerformCancel")) {
		t.Error("slider allowed selectors mismatch")
	}

	cb := v.axNodes[1]
	if got := objc.Send[int](cb.Send(sel("accessibilityValue")), sel("intValue")); got != 2 {
		t.Errorf("mixed checkbox value = %d, want 2", got)
	}
	if !objc.Send[bool](cb, sel("isAccessibilityExpanded")) {
		t.Error("checkbox expected expanded")
	}
	if busy := cb.Send(sel("accessibilityAttributeValue:"), apple.NSString("AXElementBusy")); busy == 0 || !objc.Send[bool](busy, sel("boolValue")) {
		t.Error("busy node did not report AXElementBusy=true")
	}

	off := v.axNodes[3]
	if objc.Send[bool](off, sel("isAccessibilityEnabled")) {
		t.Error("disabled node reported enabled")
	}
	if objc.Send[bool](off, sel("accessibilityPerformPress")) {
		t.Error("disabled node accepted press")
	}

	// Trigger increment, decrement, activate, cancel and scroll actions.
	if !objc.Send[bool](vol, sel("accessibilityPerformIncrement")) {
		t.Error("slider increment returned false")
	}
	if !objc.Send[bool](vol, sel("accessibilityPerformDecrement")) {
		t.Error("slider decrement returned false")
	}
	if !objc.Send[bool](cb, sel("accessibilityPerformPress")) {
		t.Error("checkbox press returned false")
	}
	feed := v.axNodes[2]
	if !objc.Send[bool](feed, sel("accessibilityPerformCancel")) {
		t.Error("feed cancel returned false")
	}
	customActions := feed.Send(sel("accessibilityCustomActions"))
	if count := objc.Send[uint](customActions, sel("count")); count != 1 {
		t.Fatalf("feed custom actions count = %d, want 1", count)
	}
	scrollAct := customActions.Send(sel("objectAtIndex:"), uint(0))
	if !objc.Send[bool](feed, selCanvasCustomAction, scrollAct) {
		t.Error("feed scroll custom action returned false")
	}

	// Exercise focusAccessibilityNode (must not crash or fail).
	v.focusAccessibilityNode("vol")

	// Wait for dispatched events to flush through engine.
	eng.Close()
	want := []string{"vol:" + driver.ActionIncrement, "vol:" + driver.ActionDecrement, "cb:" + driver.ActionActivate, "feed:" + driver.ActionDismiss, "feed:" + driver.ActionScroll}
	var got []string
	for _, ev := range reported {
		got = append(got, ev.CanvasNode+":"+ev.Text)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("reported actions = %v, want %v", got, want)
	}
}
