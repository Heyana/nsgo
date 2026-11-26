package ui

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// tabsContent shows a list of each variant, named after it, of tabs of
// different widths with the third disabled.
func tabsContent(cx *core.Scope) core.Node {
	var lists []core.Node
	for v, name := range []string{"segmented", "underline"} {
		selected := core.NewSignal(cx, 0)
		lists = append(lists, Tabs[int]{Variant: TabsVariant(v), Label: name, TestID: name, Value: selected.Get, OnChange: selected.Set,
			Options: []Option[int]{{Value: 0, Label: "One", TestID: name + "-0"}, {Value: 1, Label: "A longer tab", TestID: name + "-1"},
				{Value: 2, Label: "Three", TestID: name + "-2", Disabled: func() bool { return true }}}})
	}
	return core.View{Style: core.Style{Gap: 16, AlignItems: core.AlignStart}, Children: lists}
}

// checkIndicator fails unless name's indicator lies on tab, at its look.
func checkIndicator(t *testing.T, app *nsgotest.App, name, tab string) {
	t.Helper()
	at, f := app.Find(tab).Frame, app.Find(name).Children[0].Frame
	want := at
	if name == "underline" {
		want.Origin.X, want.Size.Width = at.Origin.X+6, at.Size.Width-12
		want.Origin.Y, want.Size.Height = at.Origin.Y+at.Size.Height-2, 2
	}
	if f != want {
		t.Fatalf("%s: indicator at %+v, want %+v over %s", name, f, want, tab)
	}
}

// TestTabs selects tabs of both variants, skips a disabled one, and moves
// the one indicator onto the selected tab.
func TestTabs(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		app, p := start(t, e, tabsContent)
		for _, name := range []string{"segmented", "underline"} {
			checkIndicator(t, app, name, name+"-0")
			app.Press(name + "-1")
			if !app.Find(name+"-1").Props.Selected || app.Find(name+"-0").Props.Selected {
				t.Fatalf("%s: pressing the second tab did not select it alone", name)
			}
			if !app.Find(name + "-2").Props.Disabled {
				t.Fatalf("%s: the third tab is not disabled", name)
			}
			checkIndicator(t, app, name, name+"-1")
		}
		if got := app.Find("underline").Children[0].Style.Background; got != p.Accent {
			t.Fatalf("underline = %x, want the accent", got)
		}
		if got := app.Find("segmented").Style.Background; got == 0 {
			t.Fatal("the segmented list has no track")
		}
	})
}

// TestTabsEqual shares the width between the tabs.
func TestTabsEqual(t *testing.T) {
	headless(t)
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		value := core.NewSignal(cx, "a")
		return Tabs[string]{TestID: "tabs", Equal: true, Value: value.Get, OnChange: value.Set,
			Options: []Option[string]{{Value: "a", Label: "A", TestID: "a"}, {Value: "b", Label: "A much longer tab", TestID: "b"}}}
	})
	list, a, b := app.Find("tabs"), app.Find("a"), app.Find("b")
	if a.Frame.Size.Width != b.Frame.Size.Width {
		t.Fatalf("tabs %v and %v wide", a.Frame.Size.Width, b.Frame.Size.Width)
	}
	if want := 2*a.Frame.Size.Width + 2 + 2*2; list.Frame.Size.Width != want {
		t.Fatalf("list %v wide, want %v", list.Frame.Size.Width, want)
	}
}

// TestTabsContent shows an option's content in place of its label, which
// still names the tab.
func TestTabsContent(t *testing.T) {
	headless(t)
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		return Tabs[int]{Options: []Option[int]{{Label: "Inbox", TestID: "inbox", Content: core.View{TestID: "slot", Style: core.Style{Width: core.Pt(8), Height: core.Pt(8)}}}}}
	})
	tab := app.Find("inbox")
	if len(tab.Children) != 1 || tab.Children[0] != app.Find("slot") || tab.Props.Label != "Inbox" {
		t.Fatalf("tab %+v", tab.Props)
	}
}

// TestTabsSlide slides the indicator on a change of tab alone.
func TestTabsSlide(t *testing.T) {
	headless(t)
	app, _ := start(t, desktopEnv, tabsContent, nsgotest.ManualFrames())
	thumb := func() *nsgotest.View { return app.Find("segmented").Children[0] }
	if d := thumb().Style.Transition.Duration; d != 0 {
		t.Fatalf("the first placement slides for %v", d)
	}
	app.Press("segmented-1")
	if d := thumb().Style.Transition.Duration; d != tabsSlide.Duration {
		t.Fatalf("a change of tab slides for %v", d)
	}
	app.Press("segmented-0")
	checkIndicator(t, app, "segmented", "segmented-0")
}

// TestTabsStyles lets an app restyle the tabs, the selected one and the indicator of either variant without changing the layout or selection behavior.
func TestTabsStyles(t *testing.T) {
	headless(t)
	for _, variant := range []TabsVariant{TabsSegmented, TabsUnderline} {
		app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
			value := core.NewSignal(cx, 0)
			return Tabs[int]{Variant: variant, TestID: "tabs", Value: value.Get, OnChange: value.Set,
				TabStyle:       core.Style{Color: core.RGB(0x654321), Selected: &core.Style{FontWeight: core.FontWeightBold}},
				IndicatorStyle: core.Style{CornerRadius: 1, Background: core.RGB(0x123456)},
				Options:        []Option[int]{{Value: 0, Label: "One", TestID: "one"}, {Value: 1, Label: "Two", TestID: "two"}}}
		})
		indicator := app.Find("tabs").Children[0].Style
		if indicator.CornerRadius != 1 || indicator.Background != core.RGB(0x123456) {
			t.Fatalf("variant %v: indicator radius %v, background %x; want 1, 123456", variant, indicator.CornerRadius, indicator.Background)
		}
		if got := app.Find("two").Children[0].TextStyle.Color; got != core.RGB(0x654321) {
			t.Fatalf("variant %v: unselected tab color = %x, want 654321", variant, got)
		}
		if got := app.Find("one").Children[0].TextStyle.Font.Weight; got != core.FontWeightBold {
			t.Fatalf("variant %v: selected tab weight = %v, want bold", variant, got)
		}
		if got := app.Find("two").Children[0].TextStyle.Font.Weight; got == core.FontWeightBold {
			t.Fatalf("variant %v: unselected tab is bold", variant)
		}
		app.Press("two")
		if got := app.Find("two").Children[0].TextStyle.Font.Weight; got != core.FontWeightBold {
			t.Fatalf("variant %v: newly selected tab weight = %v, want bold", variant, got)
		}
	}
}

// TestTabsUnderlineIndicatorOptions colors an underline from IndicatorColor, following its changes, and fits it to the selected tab's content left to right and right to left, sliding between tabs.
func TestTabsUnderlineIndicatorOptions(t *testing.T) {
	headless(t)
	for _, rtl := range []bool{false, true} {
		e := desktopEnv
		e.RTL = rtl
		var color *core.Signal[core.Color]
		var value *core.Signal[int]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			color = core.NewSignal(cx, core.RGB(0x123456))
			value = core.NewSignal(cx, 0)
			return core.View{Style: core.Style{Width: core.Pt(400)}, Children: []core.Node{Tabs[int]{Variant: TabsUnderline, Equal: true, TestID: "tabs", Value: value.Get, OnChange: value.Set, IndicatorColor: color.Get, IndicatorFit: TabsIndicatorContent,
				Options: []Option[int]{{Value: 0, Label: "One", TestID: "one"}, {Value: 1, Label: "A longer tab", TestID: "two"}}}}}
		}, nsgotest.ManualFrames())
		check := func(tab string) {
			t.Helper()
			// Frames are relative to the parent: the content's to its tab, the tab's and the underline's to the list.
			at, content, f := app.Find(tab).Frame, app.Find(tab).Children[0].Frame, app.Find("tabs").Children[0].Frame
			if content.Size.Width <= 0 || content.Size.Width >= at.Size.Width-12 {
				t.Fatalf("rtl %v: %s's content is %v wide in a %v tab; the test needs content narrower than the tab's underline", rtl, tab, content.Size.Width, at.Size.Width)
			}
			if f.Size.Width != content.Size.Width || f.Origin.X != at.Origin.X+content.Origin.X || f.Origin.Y != at.Origin.Y+at.Size.Height-2 {
				t.Fatalf("rtl %v: underline at %+v, want under %s's content at %+v in the tab at %+v", rtl, f, tab, content, at)
			}
		}
		check("one")
		if got := app.Find("tabs").Children[0].Style.Background; got != core.RGB(0x123456) {
			t.Fatalf("rtl %v: underline color = %x, want 123456", rtl, got)
		}
		color.Set(core.RGB(0xabcdef))
		app.Settle()
		if got := app.Find("tabs").Children[0].Style.Background; got != core.RGB(0xabcdef) {
			t.Fatalf("rtl %v: underline color = %x after a change, want abcdef", rtl, got)
		}
		app.Press("two")
		if app.Find("tabs").Children[0].Style.Transition.Duration == 0 {
			t.Fatalf("rtl %v: the content-fitted underline does not slide to the next tab", rtl)
		}
		app.Advance(500 * time.Millisecond)
		check("two")
		value.Set(5)
		app.Settle()
		if got := app.Find("tabs").Children[0].Frame.Size.Width; got != 0 {
			t.Fatalf("rtl %v: underline is %v wide with no tab selected, want 0", rtl, got)
		}
	}
}

// TestTabsRTL keeps the indicator on the selected tab right to left.
func TestTabsRTL(t *testing.T) {
	e := desktopEnv
	e.RTL = true
	app, _ := start(t, e, tabsContent)
	if app.Find("segmented-1").Frame.Origin.X >= app.Find("segmented-0").Frame.Origin.X {
		t.Fatal("the tabs are not laid out right to left")
	}
	for _, name := range []string{"segmented", "underline"} {
		checkIndicator(t, app, name, name+"-0")
		app.Press(name + "-1")
		checkIndicator(t, app, name, name+"-1")
	}
}

// TestTabsOptionsFuncUpdatesInPlace keeps a tab's row and focus when its options are replaced, while adding and removing rows around it, and its label follows.
func TestTabsOptionsFuncUpdatesInPlace(t *testing.T) {
	headless(t)
	var options *core.Signal[[]Option[string]]
	value := "b"
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		options = core.NewSignalFunc(cx, []Option[string]{{Value: "a", Label: "Alpha", TestID: "a"}, {Value: "b", Label: "Beta", TestID: "b"}}, nil)
		return Tabs[string]{TestID: "tabs", OptionsFunc: options.Get, Value: func() string { return value }, OnChange: func(v string) { value = v }}
	})
	app.Focus("id:b")
	if !app.Find("id:b").Focused {
		t.Fatal("the tab did not take focus")
	}
	options.Set([]Option[string]{{Value: "b", Label: "Beta (new)", TestID: "b"}, {Value: "c", Label: "Gamma", TestID: "c"}})
	app.Settle()
	if !app.Find("id:b").Focused || !app.Find("id:b").Props.Selected {
		t.Fatal("updating options moved focus or selection from the retained tab")
	}
	if !app.Exists("text:Beta (new)") || !app.Exists("id:c") || app.Exists("id:a") {
		t.Fatal("the options function did not update tab rows")
	}
	if got := app.Find("id:b").Props.Label; got != "Beta (new)" {
		t.Fatalf("the retained tab's accessibility label is %q, want the new one", got)
	}
}

// TestTabsSegmentedDarkIndicator uses a translucent foreground fill and a softer
// shadow for the segmented indicator in dark mode so it stays clear over the track.
func TestTabsSegmentedDarkIndicator(t *testing.T) {
	headless(t)
	lightApp, lightPalette := start(t, desktopEnv, tabsContent)
	lightThumb := lightApp.Find("segmented").Children[0].Style
	if lightThumb.Background != lightPalette.Raised {
		t.Fatalf("light segmented indicator background = %x, want raised %x", lightThumb.Background, lightPalette.Raised)
	}
	darkEnv := desktopEnv
	darkEnv.Dark = true
	darkApp, darkPalette := start(t, darkEnv, tabsContent)
	darkThumb := darkApp.Find("segmented").Children[0].Style
	if want := Alpha(darkPalette.Text, 0.14); darkThumb.Background != want {
		t.Fatalf("dark segmented indicator background = %x, want %x", darkThumb.Background, want)
	}
	if darkThumb.Effects == nil || len(darkThumb.Effects.Shadows) != 1 || darkThumb.Effects.Shadows[0] != Shadow(0, 1, 2, 0, Black(0.16)) {
		t.Fatalf("dark segmented indicator effects = %+v, want softer shadow", darkThumb.Effects)
	}
}
