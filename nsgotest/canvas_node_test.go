package nsgotest_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestCanvasNodeAccessibilityPropertiesActionsAndFocus(t *testing.T) {
	var canvasRef core.Ref
	var volume *core.Signal[float32]
	var checked *core.Signal[core.CheckedState]
	var expanded *core.Signal[core.ExpandedState]
	var score *core.Signal[int]
	var actions []core.CanvasActionEvent

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		volume = core.NewSignal(cx, float32(50))
		checked = core.NewSignal(cx, core.CheckedMixed)
		expanded = core.NewSignal(cx, core.ExpandedFalse)
		score = core.NewSignal(cx, 100)
		return core.Canvas{
			TestID: "hud",
			Ref:    &canvasRef,
			Style:  core.Style{Width: core.Pt(300), Height: core.Pt(240)},
			Draw: func(d *core.Drawing, _ core.Size) {
				v := volume.Get()
				d.Node(core.CanvasNode{
					ID:        "volume",
					Bounds:    core.Bounds{Origin: core.Point{X: 10, Y: 10}, Size: core.Size{Width: 200, Height: 28}},
					Role:      core.RoleAdjustable,
					Label:     "Master volume",
					Hint:      "Swipe up or down to change volume",
					Value:     v,
					Min:       0,
					Max:       100,
					Step:      10,
					ValueText: fmt.Sprintf("%.0f%%", v),
				})
				d.Node(core.CanvasNode{
					ID:      "mute",
					Bounds:  core.Bounds{Origin: core.Point{X: 10, Y: 44}, Size: core.Size{Width: 120, Height: 28}},
					Role:    core.RoleCheckbox,
					Label:   "Audio channels",
					Checked: checked.Get(),
				})
				d.Node(core.CanvasNode{
					ID:       "menu",
					Bounds:   core.Bounds{Origin: core.Point{X: 10, Y: 78}, Size: core.Size{Width: 120, Height: 28}},
					Role:     core.RoleButton,
					Label:    "Graphics",
					Expanded: expanded.Get(),
				})
				d.Node(core.CanvasNode{
					ID:       "locked",
					Bounds:   core.Bounds{Origin: core.Point{X: 10, Y: 112}, Size: core.Size{Width: 120, Height: 28}},
					Role:     core.RoleButton,
					Label:    "Continue",
					Disabled: true,
				})
				d.Node(core.CanvasNode{
					ID:        "score",
					Bounds:    core.Bounds{Origin: core.Point{X: 10, Y: 146}, Size: core.Size{Width: 120, Height: 28}},
					Role:      core.RoleStatus,
					Label:     "Score",
					ValueText: fmt.Sprintf("%d", score.Get()),
					Live:      core.LiveRegionPolite,
					Busy:      true,
				})
				d.Node(core.CanvasNode{
					ID:      "feed",
					Bounds:  core.Bounds{Origin: core.Point{X: 10, Y: 180}, Size: core.Size{Width: 120, Height: 28}},
					Role:    core.RoleGroup,
					Label:   "Combat log",
					Actions: core.CanvasActionScroll | core.CanvasActionDismiss,
				})
			},
			OnAccessibilityAction: func(ev core.CanvasActionEvent) {
				actions = append(actions, ev)
				switch ev.NodeID {
				case "volume":
					if ev.Action == core.ActionIncrement {
						volume.Set(min(volume.Get()+10, 100))
					} else if ev.Action == core.ActionDecrement {
						volume.Set(max(volume.Get()-10, 0))
					}
				case "mute":
					if ev.Action == core.ActionActivate {
						if checked.Get() == core.CheckedTrue {
							checked.Set(core.CheckedFalse)
						} else {
							checked.Set(core.CheckedTrue)
						}
					}
				case "menu":
					if ev.Action == core.ActionActivate {
						if expanded.Get() == core.ExpandedTrue {
							expanded.Set(core.ExpandedFalse)
						} else {
							expanded.Set(core.ExpandedTrue)
						}
					}
				}
			},
		}
	})

	// Find nodes by id, role, hint, value, live region and states.
	vol := app.Find("role:adjustable Master volume")
	if vol.Node == nil || vol.Node.ID != "volume" || vol.Props.Value.Now != 50 || vol.Props.Value.Min != 0 ||
		vol.Props.Value.Max != 100 || vol.Node.Step != 10 || vol.Props.Value.Text != "50%" ||
		vol.Props.Hint != "Swipe up or down to change volume" {
		t.Fatalf("volume node = %+v props = %+v", vol.Node, vol.Props)
	}
	if app.Find("hint:Swipe up or down to change volume").Node.ID != "volume" {
		t.Fatal("hint: selector did not match canvas node")
	}
	if app.Find("value:50%").Node.ID != "volume" {
		t.Fatal("value: selector did not match ValueText")
	}
	if app.Find("state:mixed").Node.ID != "mute" {
		t.Fatal("state:mixed selector did not match checkbox node")
	}
	if app.Find("state:collapsed").Node.ID != "menu" {
		t.Fatal("state:collapsed selector did not match menu node")
	}
	if app.Find("state:disabled").Node.ID != "locked" {
		t.Fatal("state:disabled selector did not match disabled node")
	}
	if app.Find("live:polite").Node.ID != "score" || app.Find("state:busy").Node.ID != "score" {
		t.Fatal("live:polite / state:busy selector did not match live status node")
	}

	// Move accessibility focus to the volume node and verify it persists across value updates.
	app.Post(func() { canvasRef.SetAccessibilityFocusNode("volume") })
	app.Settle()
	if app.AccessibilityFocus == nil || app.AccessibilityFocus.Node == nil || app.AccessibilityFocus.Node.ID != "volume" {
		t.Fatalf("AccessibilityFocus = %+v, want volume node", app.AccessibilityFocus)
	}

	// Perform increment, decrement, activate, scroll and dismiss actions.
	app.AccessibilityAction("id:volume", core.ActionIncrement)
	if app.Find("value:60%").Node.ID != "volume" {
		t.Fatalf("volume after increment = %+v", app.Find("id:volume").Props.Value)
	}
	if app.AccessibilityFocus == nil || app.AccessibilityFocus.Node == nil || app.AccessibilityFocus.Node.ID != "volume" {
		t.Fatalf("AccessibilityFocus lost after redraw: %+v", app.AccessibilityFocus)
	}
	app.AccessibilityAction("id:volume", core.ActionDecrement)
	if app.Find("id:volume").Props.Value.Now != 50 {
		t.Fatalf("volume after decrement = %v, want 50", app.Find("id:volume").Props.Value.Now)
	}
	app.AccessibilityAction("id:mute", core.ActionActivate)
	if app.Find("state:checked").Node.ID != "mute" {
		t.Fatal("mute node did not transition to checked")
	}
	app.AccessibilityAction("id:mute", core.ActionActivate)
	if app.Find("state:unchecked").Node.ID != "mute" {
		t.Fatal("mute node did not transition to unchecked")
	}
	app.AccessibilityAction("id:menu", core.ActionActivate)
	if app.Find("state:expanded").Node.ID != "menu" {
		t.Fatal("menu node did not transition to expanded")
	}
	app.AccessibilityAction("id:feed", core.ActionScroll)
	app.AccessibilityAction("id:feed", core.ActionDismiss)

	wantActions := []core.CanvasActionEvent{
		{NodeID: "volume", Action: core.ActionIncrement},
		{NodeID: "volume", Action: core.ActionDecrement},
		{NodeID: "mute", Action: core.ActionActivate},
		{NodeID: "mute", Action: core.ActionActivate},
		{NodeID: "menu", Action: core.ActionActivate},
		{NodeID: "feed", Action: core.ActionScroll},
		{NodeID: "feed", Action: core.ActionDismiss},
	}
	if !slices.Equal(actions, wantActions) {
		t.Fatalf("actions = %+v, want %+v", actions, wantActions)
	}
}
