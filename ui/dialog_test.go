package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestDialog opens a dialog, centered on desktops and a sheet on phones,
// and closes it from its footer; an alert dialog keeps open on its
// backdrop and closes on an action.
func TestDialog(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var confirmed bool
		var opts []nsgotest.Option
		if e.Idiom == core.IdiomDesktop {
			opts = append(opts, nsgotest.ManualFrames())
		}
		app, p := start(t, e, func(cx *core.Scope) core.Node {
			open, alert := core.NewSignal(cx, false), core.NewSignal(cx, false)
			return core.View{Style: core.Style{AlignItems: core.AlignStart, Gap: 8}, Children: []core.Node{
				Button{Text: "Rename", TestID: "open", OnPress: func() { open.Set(true) }},
				Button{Text: "Delete", TestID: "open-alert", OnPress: func() { alert.Set(true) }},
				Dialog{TestID: "dialog", Title: "Rename file", Description: "Choose a new name.",
					Open: open.Get, OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
					Children: []core.Node{Input{Label: "Name", DefaultValue: "report.pdf"}},
					Footer: []core.Node{Button{Variant: ButtonSecondary, Text: "Cancel", TestID: "cancel", OnPress: func() { open.Set(false) }},
						Button{Variant: ButtonPrimary, Text: "Rename"}}},
				AlertDialog{TestID: "alert", Title: "Delete report.pdf?", Description: "This cannot be undone.",
					Open: alert.Get, OnOpenChange: func(v bool, _ base.Reason) { alert.Set(v) },
					Actions: []AlertAction{{Text: "Cancel", Variant: ButtonSecondary, TestID: "keep"},
						{Text: "Delete", Color: ColorsOf(cx).Danger, TestID: "confirm", OnPress: func() { confirmed = true }}}},
			}}
		}, opts...)
		app.Press("open")
		d := app.Find("dialog")
		if e.Idiom == core.IdiomDesktop {
			if d.Style.CornerRadius != RadiusDialog {
				t.Fatal("the desktop dialog is not a rounded panel")
			}
			if _, _, _, a := d.Style.BorderColor.Components(); a != 1 || len(d.Style.Effects.Shadows) == 0 {
				t.Fatalf("the desktop dialog's rim %v is not solid over a shadow", d.Style.BorderColor)
			}
		} else if d.Style.Background != p.Surface || d.Style.Corners.BottomLeft != 0 || d.Style.Corners.TopLeft == 0 {
			t.Fatal("the phone dialog is not a sheet from the bottom")
		}
		app.Find("Rename file")
		app.Press("cancel")
		if e.Idiom == core.IdiomDesktop {
			app.Find("Rename file") // it fades out
			app.Advance(200 * time.Millisecond)
		}
		app.WaitForGone("Rename file", time.Second)
		app.Press("open-alert")
		app.Press("confirm")
		app.Advance(200 * time.Millisecond)
		app.WaitForGone("Delete report.pdf?", time.Second)
		if !confirmed {
			t.Fatal("the alert's action did not run")
		}
	})
}

// TestDialogPlacement keeps a short dialog near a third of the spare
// height from the window's top, nearer the middle the taller it is, and
// centers one that fills the window within its inset.
func TestDialogPlacement(t *testing.T) {
	for _, c := range []struct {
		height core.Points
		above  float32
	}{{0, 1.0 / 3}, {360, 1.0/3 + 0.5/6}, {720, 0.5}} {
		app, _ := start(t, desktopEnv, func(*core.Scope) core.Node {
			return Dialog{Open: func() bool { return true }, Bare: true, TestID: "dialog", Style: core.Style{Width: core.Pt(300), Height: core.Pt(c.height)}}
		})
		d := app.Find("dialog")
		spare := 760 - d.Frame.Size.Height
		if got := float32(d.Frame.Origin.Y / spare); got < c.above-0.01 || got > c.above+0.01 {
			t.Errorf("a %v-point dialog has %v of the spare height above it, want %v", c.height, got, c.above)
		}
	}
}

func TestDialogTitleActions(t *testing.T) {
	for _, e := range []core.Environment{desktopEnv, phoneEnv} {
		t.Run(string(e.OS), func(t *testing.T) {
			app, _ := start(t, e, func(*core.Scope) core.Node {
				return Dialog{Open: func() bool { return true }, Title: "Edit", TestID: "dialog",
					Leading:  []core.Node{Button{Text: "Cancel", TestID: "cancel"}},
					Trailing: []core.Node{Button{Text: "Done", TestID: "done"}},
					Children: []core.Node{core.Text{Content: "Form"}}}
			})
			title := app.Find("role:header")
			leading, trailing := app.Find("dialog-title-leading"), app.Find("dialog-title-trailing")
			center := func(v *nsgotest.View) core.Points { return v.Frame.Origin.X + v.Frame.Size.Width/2 }
			if got, want := center(title), center(title.Parent); got != want {
				t.Fatalf("title center = %v, title row center = %v", got, want)
			}
			if leading.Frame.Origin.X >= trailing.Frame.Origin.X {
				t.Fatalf("title actions are reversed: leading %v, trailing %v", leading.Frame, trailing.Frame)
			}
			if title.Props.Role != core.RoleHeader {
				t.Fatalf("title role = %v, want header", title.Props.Role)
			}
		})
	}
	// A long title wraps between the actions instead of running under them.
	app, _ := start(t, desktopEnv, func(*core.Scope) core.Node {
		return Dialog{Open: func() bool { return true }, Title: "A title long enough to need more than the room between the actions of this dialog", TestID: "dialog",
			Leading:  []core.Node{Button{Text: "Cancel", TestID: "cancel"}},
			Trailing: []core.Node{Button{Text: "Done", TestID: "done"}},
			Children: []core.Node{core.Text{Content: "Form"}}}
	})
	title, leading, trailing := app.Find("role:header").Frame, app.Find("dialog-title-leading").Frame, app.Find("dialog-title-trailing").Frame
	cancel, done := app.Find("cancel").Frame, app.Find("done").Frame
	if title.Origin.X < leading.Origin.X+leading.Size.Width || title.Origin.X+title.Size.Width > trailing.Origin.X {
		t.Fatalf("title %v overlaps the actions' sides %v and %v", title, leading, trailing)
	}
	if leading.Size.Width < cancel.Size.Width || trailing.Size.Width < done.Size.Width || cancel.Origin.X < 0 || done.Origin.X < 0 {
		t.Fatalf("the actions %v and %v do not fit their sides %v and %v", cancel, done, leading, trailing)
	}
}

// TestBareDialogClipFollowsTheSurface checks that a bare dialog clips its body to the inside of the surface's rim, whatever corners and rim the app's Style and StyleFunc give it, and follows them while it is open.
func TestBareDialogClipFollowsTheSurface(t *testing.T) {
	type corners = core.Corners
	for _, c := range []struct {
		name          string
		style         core.Style
		surface, clip corners
	}{
		{"default", core.Style{}, uniform(RadiusDialog), uniform(RadiusDialog - 1)},
		{"radius", core.Style{CornerRadius: 20}, uniform(20), uniform(19)},
		{"rim", core.Style{CornerRadius: 20, BorderWidth: 3}, uniform(20), uniform(17)},
		{"no rim", core.Style{CornerRadius: 12, BorderStyle: core.BorderNone}, uniform(12), uniform(12)},
		{"sides", core.Style{CornerRadius: 12, Borders: core.Edges{Top: 4, Right: 1, Bottom: 1, Left: 1}}, uniform(12),
			corners{TopLeft: 8, TopRight: 8, BottomRight: 11, BottomLeft: 11}},
		{"corners", core.Style{CornerRadii: corners{TopLeft: 20, TopRight: 20}}, corners{TopLeft: 20, TopRight: 20},
			corners{TopLeft: 19, TopRight: 19}},
		{"circular", core.Style{BorderCurve: core.BorderCurveCircular}, uniform(RadiusDialog), uniform(RadiusDialog - 1)},
	} {
		t.Run(c.name, func(t *testing.T) {
			app, _ := start(t, desktopEnv, func(*core.Scope) core.Node {
				return Dialog{Open: func() bool { return true }, Bare: true, Style: c.style, TestID: "dialog",
					Children: []core.Node{core.Text{Content: "Body"}}}
			}, nsgotest.ManualFrames())
			dialog := app.Find("dialog")
			if got := shownCorners(dialog.Style); got != c.surface {
				t.Fatalf("surface corners = %+v, want %+v", got, c.surface)
			}
			clip := bareClipView(t, dialog).Style
			if got := shownCorners(clip); got != c.clip {
				t.Fatalf("clip corners = %+v, want %+v", got, c.clip)
			}
			if clip.ContinuousCorners != dialog.Style.ContinuousCorners {
				t.Fatalf("clip continuous corners = %v, surface's = %v", clip.ContinuousCorners, dialog.Style.ContinuousCorners)
			}
		})
	}
	t.Run("style func", func(t *testing.T) {
		var radius *core.Signal[core.Points]
		app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
			radius = core.NewSignal(cx, core.Points(20))
			return Dialog{Open: func() bool { return true }, Bare: true, TestID: "dialog",
				StyleFunc: func() core.Style { return core.Style{CornerRadius: radius.Get()} },
				Children:  []core.Node{core.Text{Content: "Body"}}}
		}, nsgotest.ManualFrames())
		if got := shownCorners(bareClipView(t, app.Find("dialog")).Style); got != uniform(19) {
			t.Fatalf("clip corners = %+v, want 19", got)
		}
		app.Post(func() { radius.Set(8) })
		app.Settle()
		if got := shownCorners(bareClipView(t, app.Find("dialog")).Style); got != uniform(7) {
			t.Fatalf("clip corners after the radius changed = %+v, want 7", got)
		}
	})
}

// uniform is r at every corner.
func uniform(r core.Points) core.Corners {
	return core.Corners{TopLeft: r, TopRight: r, BottomRight: r, BottomLeft: r}
}

// shownCorners is the radius each corner of a view is drawn with.
func shownCorners(s nsgotest.ViewStyle) core.Corners {
	if s.Corners != (core.Corners{}) {
		return s.Corners
	}
	return uniform(s.CornerRadius)
}

// bareClipView is the view a bare dialog clips its body with.
func bareClipView(t *testing.T, dialog *nsgotest.View) *nsgotest.View {
	t.Helper()
	for _, v := range views(dialog)[1:] {
		if v.Style.Clip {
			return v
		}
	}
	t.Fatal("the bare dialog has no clipping view")
	return nil
}

// TestDialogTextFuncsUpdateInPlace changes a dialog's and an alert dialog's title and description while they are open: the texts shown and what screen readers read follow on the same views, and the field inside keeps its focus and text.
func TestDialogTextFuncsUpdateInPlace(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var title, description, alert *core.Signal[string]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			title, description = core.NewSignal(cx, "First title"), core.NewSignal(cx, "First description")
			alert = core.NewSignal(cx, "First alert")
			return core.View{Children: []core.Node{
				Dialog{Open: func() bool { return true }, TestID: "dialog", TitleFunc: title.Get, DescriptionFunc: description.Get,
					Children: []core.Node{Input{Label: "Name", TestID: "name"}}},
				AlertDialog{Open: func() bool { return true }, TestID: "alert", TitleFunc: alert.Get, Description: "Static"},
			}}
		})
		dialog, alert0 := app.Find("dialog"), app.Find("alert")
		if p := dialog.Props; p.Label != "First title" || p.Description != "First description" {
			t.Fatalf("dialog reads %q, %q", p.Label, p.Description)
		}
		if p := alert0.Props; p.Label != "First alert" || p.Description != "Static" {
			t.Fatalf("alert dialog reads %q, %q", p.Label, p.Description)
		}
		app.Find("First title")
		app.Type("name", "Ada")
		field := app.Find("name")
		app.Post(func() {
			title.Set("Second title")
			description.Set("Second description")
			alert.Set("Second alert")
		})
		app.Settle()
		app.Find("Second title")
		app.Find("Second description")
		app.Find("Second alert")
		if got := app.Find("dialog"); got.ID != dialog.ID || got.Props.Label != "Second title" || got.Props.Description != "Second description" {
			t.Fatalf("dialog %v reads %q, %q after the change, was %v", got.ID, got.Props.Label, got.Props.Description, dialog.ID)
		}
		if got := app.Find("alert"); got.ID != alert0.ID || got.Props.Label != "Second alert" {
			t.Fatalf("alert dialog %v reads %q after the change, was %v", got.ID, got.Props.Label, alert0.ID)
		}
		if got := app.Find("name"); got.ID != field.ID || got.Text != "Ada" || got.Focused != field.Focused {
			t.Fatalf("the field became %v %q focused %v, was %v %q focused %v", got.ID, got.Text, got.Focused, field.ID, field.Text, field.Focused)
		}
	})
}

// An alert reports its role and message, focuses its default action, and runs one action however it is answered: Return on the focused default, Return on another action Tab reached, Escape, back or a press. Keys pressed as it goes run nothing more, and it focuses the default again as it opens again.
func TestAlertDialogRoleAndActions(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		ways := []string{"escape", "back", "press"}
		// nsgotest presses the focused view on Return in a desktop or web environment, as those platforms do; on a phone Return is the default's through base.Sheet (TestReturnRunsTheDefaultActionOnce).
		if e.Idiom == core.IdiomDesktop {
			ways = append(ways, "return", "tab-return")
		}
		for _, how := range ways {
			t.Run(how, func(t *testing.T) {
				var calls []string
				var reason base.Reason
				var open *core.Signal[bool]
				closed := 0
				app, _ := start(t, e, func(cx *core.Scope) core.Node {
					open = core.NewSignal(cx, true)
					return AlertDialog{TestID: "alert", Title: "Delete the note?", Description: "This cannot be undone.",
						Open: open.Get, OnClose: func() { closed++ },
						OnOpenChange: func(v bool, r base.Reason) { calls = append(calls, "change"); reason = r; open.Set(v) },
						Actions: []AlertAction{
							{Text: "Cancel", Role: core.AlertCancel, TestID: "cancel", OnPress: func() { calls = append(calls, "cancel") }},
							{Text: "Delete", Role: core.AlertDestructive, Default: true, TestID: "delete", OnPress: func() { calls = append(calls, "delete") }},
						}}
				})
				alert := app.Find("role:alertdialog")
				if alert.Props.Label != "Delete the note?" || alert.Props.Description != "This cannot be undone." {
					t.Fatalf("alert accessibility = %+v", alert.Props)
				}
				// The default's AutoFocus focuses it on a phone too, for a hardware keyboard, as base.Button's does there.
				if !app.Find("id:delete").Focused {
					t.Fatal("the default action is not focused as the alert opens")
				}
				wantAction, wantReason := "delete", base.ReasonSelect
				switch how {
				case "return":
					app.Key("Return")
				case "tab-return":
					app.Key("Tab")
					if !app.Find("id:cancel").Focused {
						t.Fatal("Tab did not reach the cancel action")
					}
					app.Key("Return")
					wantAction = "cancel"
				case "escape":
					app.Key("Escape")
					wantAction, wantReason = "cancel", base.ReasonCancel
				case "back":
					app.Back()
					wantAction, wantReason = "cancel", base.ReasonCancel
				case "press":
					app.Press("id:delete")
				}
				// Keys pressed while it goes cannot run an action again, even on a focused action.
				app.Key("Return")
				app.Key("Escape")
				app.Key("Return")
				if !slices.Equal(calls, []string{wantAction, "change"}) || reason != wantReason {
					t.Fatalf("calls %v, reason %v; want [%s change], %v", calls, reason, wantAction, wantReason)
				}
				app.Advance(time.Second)
				app.WaitForGone("id:alert", time.Second)
				if closed != 1 {
					t.Fatalf("OnClose ran %d times", closed)
				}
				app.Post(func() { open.Set(true) })
				app.WaitFor("id:delete", time.Second)
				if !app.Find("id:delete").Focused {
					t.Fatal("the default action is not focused as the alert opens again")
				}
			})
		}
	})
}

func TestAlertDialogWithoutActionMarkers(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		pressed, changed := 0, 0
		var reason base.Reason
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			open := core.NewSignal(cx, true)
			return AlertDialog{TestID: "alert", Open: open.Get, OnOpenChange: func(v bool, r base.Reason) {
				changed++
				reason = r
				open.Set(v)
			}, Actions: []AlertAction{{Text: "Action", TestID: "action", OnPress: func() { pressed++ }}}}
		})
		if !app.Exists("role:alertdialog") {
			t.Fatal("unmarked alert lost its alert dialog role")
		}
		app.Key("Return")
		if pressed != 0 || changed != 0 {
			t.Fatalf("unmarked action ran on Return: pressed %d, changed %d", pressed, changed)
		}
		app.Key("Escape")
		if pressed != 0 || changed != 1 || reason != base.ReasonCancel {
			t.Fatalf("unmarked cancel: pressed %d, changed %d, reason %v", pressed, changed, reason)
		}
	})
}

// On a touch screen a Dialog is a base.Sheet, which keeps its role and description, runs OnConfirm on a hardware keyboard's Return and reports OnClose once it has gone.
func TestDialogPreservesAlertRoleOnTouch(t *testing.T) {
	var log []string
	app, _ := start(t, phoneEnv, func(cx *core.Scope) core.Node {
		open := core.NewSignal(cx, true)
		return Dialog{Open: open.Get, OnOpenChange: func(v bool, r base.Reason) { log = append(log, r.String()); open.Set(v) },
			OnConfirm: func() { log = append(log, "confirm") }, OnClose: func() { log = append(log, "closed") },
			Role: core.RoleAlertDialog, Title: "Confirm", Description: "Choose an action", TestID: "dialog"}
	})
	if got := app.Find("role:alertdialog").Props.Description; got != "Choose an action" {
		t.Fatalf("sheet description %q", got)
	}
	app.Key("Return")
	app.Advance(time.Second)
	app.WaitForGone("id:dialog", time.Second)
	if want := []string{"confirm", base.ReasonSelect.String(), "closed"}; !slices.Equal(log, want) {
		t.Fatalf("log = %q, want %q", log, want)
	}
}

func TestDialogTouchOptions(t *testing.T) {
	app, _ := start(t, phoneEnv, func(*core.Scope) core.Node {
		return Dialog{Open: func() bool { return true }, Bare: true, NoGrabber: true,
			ContentStyle: core.Style{Padding: core.All(24), Gap: 3}, Title: "Title", TestID: "dialog",
			Children: []core.Node{core.Text{Content: "Body", TestID: "body"}}}
	})
	dialog := app.Find("dialog")
	for _, v := range views(dialog) {
		if v.Frame.Size.Width == 36 && v.Frame.Size.Height == 5 {
			t.Fatal("bare touch dialog kept the default grabber")
		}
	}
	body := app.Find("body")
	if body.Parent == nil || body.Frame.Origin.X != 0 {
		t.Fatalf("bare touch dialog unexpectedly padded content: frame=%+v parent=%+v", body.Frame, body.Parent)
	}

	app, _ = start(t, phoneEnv, func(*core.Scope) core.Node {
		return Dialog{Open: func() bool { return true }, NoGrabber: true,
			ContentStyle: core.Style{Padding: core.All(24), Gap: 3}, Title: "Title", TestID: "dialog",
			Children: []core.Node{core.Text{Content: "Body", TestID: "body"}}}
	})
	body = app.Find("body")
	if body.Parent == nil || body.Frame.Origin.X != 24 || body.Frame.Origin.Y < 24 {
		t.Fatalf("touch content style = body frame %+v parent=%+v, want 24pt inset", body.Frame, body.Parent)
	}
}

// A default action that closes the alert itself still reports the close, once.
func TestAlertDefaultMayUpdateOpenItself(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		if e.Idiom != core.IdiomDesktop {
			return // Return presses the focused default only where nsgotest presses focused views (TestAlertDialogRoleAndActions).
		}
		var calls []string
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			open := core.NewSignal(cx, true)
			return AlertDialog{Open: open.Get, OnOpenChange: func(v bool, r base.Reason) {
				if r != base.ReasonSelect {
					t.Errorf("close reason %v", r)
				}
				calls = append(calls, "change")
				open.Set(v)
			}, Actions: []AlertAction{{Text: "Done", Default: true, OnPress: func() {
				calls = append(calls, "action")
				open.Set(false)
			}}}}
		})
		app.Key("Return")
		app.Key("Return")
		if !slices.Equal(calls, []string{"action", "change"}) {
			t.Fatalf("calls %v", calls)
		}
	})
}

// An alert whose app keeps Open true still closes once an action ran, and runs no other action as it goes.
func TestAlertClosesItselfOnceAnswered(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var calls []string
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			return AlertDialog{TestID: "alert", Open: func() bool { return true }, Actions: []AlertAction{
				{Text: "Cancel", Role: core.AlertCancel, TestID: "cancel", OnPress: func() { calls = append(calls, "cancel") }},
				{Text: "Delete", Default: true, TestID: "delete", OnPress: func() { calls = append(calls, "delete") }},
			}}
		})
		app.Press("id:delete")
		app.Key("Escape")
		app.Key("Return")
		app.Advance(time.Second)
		app.WaitForGone("id:alert", time.Second)
		if !slices.Equal(calls, []string{"delete"}) {
			t.Fatalf("calls %v, want [delete]", calls)
		}
	})
}

// TestDialogStyleFunc follows appearance and app signals on both panels, keeping the open dialog and its content instead of rebuilding either.
func TestDialogStyleFunc(t *testing.T) {
	for _, form := range []string{"dialog", "bare", "alert"} {
		t.Run(form, func(t *testing.T) {
			each(t, func(t *testing.T, e core.Environment) {
				e.ReduceMotion = true
				var open, custom *core.Signal[bool]
				builds := 0
				static := core.Style{Background: core.RGB(0x123456), BorderWidth: 2}
				app, _ := start(t, e, func(cx *core.Scope) core.Node {
					open, custom = core.NewSignal(cx, true), core.NewSignal(cx, true)
					colors := ColorsOf(cx)
					style := func() core.Style {
						if !custom.Get() {
							return core.Style{}
						}
						return core.Style{Background: colors.Surface(), BorderColor: colors.Text(), BorderWidth: 3}
					}
					content := core.Func(func(*core.Scope) core.Node {
						builds++
						return core.Text{Content: "Kept content", TestID: "content"}
					})
					if form == "alert" {
						return AlertDialog{TestID: "panel", Open: open.Get, Title: "Confirm", Style: static, StyleFunc: style}
					}
					return Dialog{TestID: "panel", Open: open.Get, Bare: form == "bare", Style: static, StyleFunc: style,
						Children: []core.Node{content}}
				})
				check := func(dynamic bool) {
					t.Helper()
					s := app.Find("panel").Style
					background, width := static.Background, static.BorderWidth
					if dynamic {
						palette := core.DefaultTheme().Light
						if e.Dark {
							palette = core.DefaultTheme().Dark
						}
						background, width = palette.Surface, 3
						if s.BorderColor != palette.Text {
							t.Fatalf("border = %v, want %v", s.BorderColor, palette.Text)
						}
					}
					if s.Background != background || s.BorderWidth != width {
						t.Fatalf("panel = background %v, border %v; want %v, %v", s.Background, s.BorderWidth, background, width)
					}
				}
				check(true)
				id := app.Find("panel").ID
				e.Dark = !e.Dark
				app.SetEnvironment(e)
				check(true)
				if app.Find("panel").ID != id || form != "alert" && builds != 1 {
					t.Fatal("restyling rebuilt the dialog or its content")
				}
				app.Post(func() { custom.Set(false) })
				app.Settle()
				check(false)
				app.Post(func() { open.Set(false) })
				app.WaitForGone("panel", time.Second)
				app.Post(func() { custom.Set(true); open.Set(true) })
				app.Settle()
				check(true)
			})
		})
	}
}
