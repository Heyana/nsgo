package nsgotest_test

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestEditMenuModelAndActions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		os        core.OS
		selectAll string
	}{
		{"iOS", core.OSIOS, "Select All"},
		{"Android", core.OSAndroid, "Select all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ref core.Ref
			var got []core.EditAction
			var pasted []core.PasteEvent
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				return core.Input{
					Ref:           &ref,
					TestID:        "sink",
					PasteBehavior: core.PasteReport,
					OnEditAction:  func(a core.EditAction) { got = append(got, a) },
					OnPaste:       func(p core.PasteEvent) { pasted = append(pasted, p) },
				}
			}, nsgotest.Environment(core.Environment{OS: tc.os, Idiom: core.IdiomPhone}))

			bounds := core.Bounds{Origin: core.Point{X: 20, Y: 40}, Size: core.Size{Width: 80, Height: 22}}
			app.Post(func() {
				ref.Focus()
				ref.OpenEditMenu(bounds, core.EditCut|core.EditCopy|core.EditPaste|core.EditSelectAll)
			})
			app.Settle()

			if app.EditMenu == nil || app.EditMenu.Bounds != bounds {
				t.Fatalf("EditMenu = %+v", app.EditMenu)
			}
			// Empty clipboard omits Paste.
			if items := app.EditMenuItems(); !slices.Equal(items, []string{"Cut", "Copy", tc.selectAll}) {
				t.Fatalf("items without clipboard = %v", items)
			}
			app.Clipboard = "hello"
			if items := app.EditMenuItems(); !slices.Equal(items, []string{"Cut", "Copy", "Paste", tc.selectAll}) {
				t.Fatalf("items with clipboard = %v", items)
			}

			app.ChooseEditAction(core.EditCopy)
			if app.EditMenu != nil {
				t.Fatal("menu stayed open after choosing Copy")
			}
			if !slices.Equal(got, []core.EditAction{core.EditCopy}) {
				t.Fatalf("got actions = %v", got)
			}

			// Re-open and choose Paste -> runs OnPaste.
			app.Post(func() { ref.OpenEditMenu(bounds, core.EditPaste) })
			app.Settle()
			app.ChooseEditAction(core.EditPaste)
			if len(pasted) != 1 || pasted[0].Text != "hello" {
				t.Fatalf("pasted = %+v", pasted)
			}

			// Blur clears open edit menu.
			app.Post(func() { ref.OpenEditMenu(bounds, core.EditSelectAll) })
			app.Settle()
			if app.EditMenu == nil {
				t.Fatal("menu did not reopen")
			}
			app.Post(func() { ref.Blur() })
			app.Settle()
			if app.EditMenu != nil {
				t.Fatal("blur did not clear EditMenu")
			}
		})
	}
}

func TestLongPressAtAndFingerHold(t *testing.T) {
	var events []string
	var lastAt core.Point
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{
			TestID:      "pad",
			Style:       core.Style{Width: core.Pt(200), Height: core.Pt(100)},
			OnPressDown: func(core.PressEvent) { events = append(events, "down") },
			OnLongPress: func(e core.PressEvent) { events = append(events, "long"); lastAt = e.Position },
			OnPressUp:   func(core.PressEvent) { events = append(events, "up") },
			OnPress:     func(core.PressEvent) { events = append(events, "press") },
		}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))

	app.LongPressAt("id:pad", core.Point{X: 45, Y: 30})
	if !slices.Equal(events, []string{"down", "long", "up"}) || lastAt != (core.Point{X: 45, Y: 30}) {
		t.Fatalf("LongPressAt events=%v at=%v", events, lastAt)
	}

	events = nil
	f := app.Touch("id:pad", core.Point{X: 80, Y: 20})
	f.Hold(500 * time.Millisecond)
	if !slices.Equal(events, []string{"down", "long"}) {
		t.Fatalf("Finger.Hold events=%v, want [down long]", events)
	}
	f.Lift()
	if !slices.Equal(events, []string{"down", "long", "up"}) {
		t.Fatalf("after Lift events=%v, want [down long up] without duplicate long or press", events)
	}
}
