package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// TestToggleGroupAndToolbar put toggle buttons in one stop of the focus.
func TestToggleGroupAndToolbar(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var bold *core.Signal[bool]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			bold = core.NewSignal(cx, false)
			return Toolbar{TestID: "bar", Label: "Format", Children: []core.Node{
				ToggleGroup{TestID: "marks", Children: []core.Node{
					ToggleButton{Text: "Bold", TestID: "bold", On: bold.Get, OnChange: bold.Set},
					ToggleButton{Text: "Italic", TestID: "italic"},
				}},
				ToolbarSeparator{TestID: "line"},
				ToolbarGroup{Label: "More", TestID: "more", Children: []core.Node{Link{Text: "Help", Href: "/help", OpenWith: base.LinkOpenApp, TestID: "help"}}},
			}}
		})
		if app.Find("bar").Props.Role != core.RoleToolbar || app.Find("marks").Props.Role != core.RoleGroup {
			t.Fatal("a toggle group in a toolbar is one of its groups")
		}
		if !app.Find("bold").Props.Focusable || app.Find("italic").Props.Focusable || app.Find("help").Props.Focusable {
			t.Fatal("the toolbar is not one stop of the focus")
		}
		app.Press("bold")
		if !bold.Get() || !app.Find("bold").Props.Pressed {
			t.Fatal("a press does not turn the toggle on")
		}
	})
}

// TestLink reports where it goes, then the press.
func TestLink(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var log []string
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			return Link{Text: "Docs", Href: "/docs", OpenWith: base.LinkOpenApp, TestID: "docs",
				OnOpen: func(href string) { log = append(log, href) }, OnPress: func() { log = append(log, "press") }}
		})
		app.Press("docs")
		if len(log) != 2 || log[0] != "/docs" || log[1] != "press" || app.Find("docs").Props.Role != core.RoleLink {
			t.Fatalf("log = %v", log)
		}
	})
}

// TestLinkTextColor draws a link's text and hover underline in the color its Style gives, the accent when it gives none.
func TestLinkTextColor(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		ink, hover := core.RGB(0xf2dcb5), core.RGB(0xffeedd)
		app, p := start(t, e, func(cx *core.Scope) core.Node {
			return core.View{Children: []core.Node{
				Link{Text: "Default", TestID: "default"},
				Link{Text: "Plain", TestID: "plain", Style: core.Style{Color: ink}},
				Link{Text: "Custom", TestID: "custom", Style: core.Style{Color: ink, Hover: &core.Style{Color: hover}}},
				Link{Text: "Transparent", TestID: "transparent", Style: core.Style{Color: core.Transparent}},
				Link{Text: "Hovered", TestID: "hovered", Style: core.Style{Hover: &core.Style{Color: hover}}},
			}}
		})
		if got := app.Find("default >> Default").TextStyle.Color; got != p.Accent {
			t.Fatalf("default link color = %#x, want %#x", got, p.Accent)
		}
		if got := app.Find("transparent >> Transparent").TextStyle.Color; got != core.Transparent {
			t.Fatalf("transparent link color = %#x", got)
		}
		text := app.Find("custom >> Custom")
		if text.TextStyle.Color != ink {
			t.Fatalf("custom link color = %#x, want %#x", text.TextStyle.Color, ink)
		}
		id := text.ID
		app.Hover("plain")
		text = app.Find("plain >> Plain")
		// A zero decoration color uses the glyph color on every backend.
		if text.TextStyle.Color != ink || text.TextStyle.Decoration != core.TextDecorationUnderline || text.TextStyle.DecorationColor != 0 {
			t.Fatalf("custom link underline = %+v", text.TextStyle)
		}
		app.Hover("custom")
		text = app.Find("custom >> Custom")
		if text.TextStyle.Color != hover || text.TextStyle.Decoration != core.TextDecorationUnderline || text.TextStyle.DecorationColor != 0 {
			t.Fatalf("hovered link style = %+v", text.TextStyle)
		}
		app.Hover("default")
		text = app.Find("custom >> Custom")
		if text.TextStyle.Color != ink || text.TextStyle.Decoration != 0 || text.ID != id {
			t.Fatalf("link did not restore its custom color in place: %+v", text.TextStyle)
		}
		text = app.Find("default >> Default")
		if text.TextStyle.Color != p.Accent || text.TextStyle.Decoration != core.TextDecorationUnderline || text.TextStyle.DecorationColor != 0 {
			t.Fatalf("default link underline = %+v", text.TextStyle)
		}
		e.Dark = !e.Dark
		app.SetEnvironment(e)
		want := core.DefaultTheme().Light.Accent
		if e.Dark {
			want = core.DefaultTheme().Dark.Accent
		}
		if text = app.Find("default >> Default"); text.TextStyle.Color != want || text.TextStyle.Decoration != core.TextDecorationUnderline || text.TextStyle.DecorationColor != 0 {
			t.Fatalf("default link did not follow the new appearance: %+v", text.TextStyle)
		}
		if text = app.Find("custom >> Custom"); text.TextStyle.Color != ink || text.ID != id {
			t.Fatal("an appearance change replaced the custom color or rebuilt the link")
		}
		// A hover color alone keeps the accent until the pointer enters.
		if got := app.Find("hovered >> Hovered").TextStyle.Color; got != want {
			t.Fatalf("hover-only link color = %#x, want the accent %#x", got, want)
		}
		app.Hover("hovered")
		if text = app.Find("hovered >> Hovered"); text.TextStyle.Color != hover || text.TextStyle.Decoration != core.TextDecorationUnderline {
			t.Fatalf("hover-only link under the pointer = %+v", text.TextStyle)
		}
	})
}

// TestPagination moves between pages, the current one selected.
func TestPagination(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var page *core.Signal[int]
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			page = core.NewSignal(cx, 1)
			return Pagination{Page: page.Get, TotalPages: func() int { return 20 }, OnChange: page.Set, Label: "Pages",
				PreviousLabel: "Previous", NextLabel: "Next", TestID: "pages"}
		})
		if !app.Find("label:Previous").Props.Disabled {
			t.Fatal("Previous is not disabled on the first page")
		}
		app.Press("label:Next")
		if page.Get() != 2 || app.Find("label:Previous").Props.Disabled {
			t.Fatalf("page = %d after Next", page.Get())
		}
		app.Press(`text:"20"`)
		if page.Get() != 20 || !app.Find("label:Next").Props.Disabled {
			t.Fatalf("page = %d after the last page", page.Get())
		}
	})
}

// TestOtpInput keeps digits up to its length and reports the full code.
func TestOtpInput(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var code, done string
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			return OtpInput{Length: 4, Groups: 2, Label: "Code", OnChange: func(v string) { code = v }, OnComplete: func(v string) { done = v }}
		})
		app.Type("label:Code", "12a345")
		if code != "1234" || done != "1234" {
			t.Fatalf("code = %q, done = %q", code, done)
		}
	})
}

func TestOtpInputCellStyles(t *testing.T) {
	app, palette := start(t, core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
			OtpInput{Length: 2, TestID: "plain", Label: "Plain code", Size: 40},
			OtpInput{Length: 2, TestID: "styled", Label: "Styled code", CellWidth: 44, CellHeight: 52, CornerRadius: 10,
				TextStyle: core.Style{FontSize: 22, FontWeight: Semibold, FontVariant: core.FontTabularNums, LineHeight: 28}},
			OtpInput{Length: 2, TestID: "caret", Label: "Custom caret", Size: 46, CellHeight: 54, CaretHeight: 24},
		}}
	})
	for _, c := range []struct {
		id, label string
		size      core.Size
		radius    core.Points
		caret     core.Points
	}{
		{"plain", "Plain code", core.Size{Width: 40, Height: 40}, RadiusControl, 16},
		{"styled", "Styled code", core.Size{Width: 44, Height: 52}, 10, 26},
		{"caret", "Custom caret", core.Size{Width: 46, Height: 54}, RadiusControl, 24},
	} {
		app.Focus("label:" + c.label)
		cell := app.Find(c.id).Children[0].Children[0]
		if cell.Frame.Size != c.size || cell.Style.CornerRadius != c.radius || cell.Style.BorderColor != palette.Accent {
			t.Fatalf("%s cell: size %v, radius %v, border %v; want %v, %v, accent", c.id, cell.Frame.Size, cell.Style.CornerRadius, cell.Style.BorderColor, c.size, c.radius)
		}
		if len(cell.Children) != 1 || cell.Children[0].Frame.Size.Height != c.caret {
			t.Fatalf("%s cell lacks its %v point caret", c.id, c.caret)
		}
	}
	app.Type("label:Styled code", "1")
	digit := app.Find("styled").Children[0].Children[0].Children[0]
	text := digit.TextStyle
	if digit.Text != "1" || text.Font.Size != 22 || text.Font.Weight != Semibold || text.Font.Variant != core.FontTabularNums || text.LineHeight != 28 {
		t.Fatalf("digit typography = %+v", text)
	}
}

// TestTouchControlSizes lays out a full-width Button, OtpInputs and form rows on phones and tablets of both mobile platforms, where they reach touch sizes and a six-digit code fits a phone's column, and on desktops and a TV, which keep the compact sizes; each still takes presses and typing.
func TestTouchControlSizes(t *testing.T) {
	headless(t)
	for _, c := range []struct {
		name              string
		os                core.OS
		idiom             core.Idiom
		button, cell, row core.Points
	}{
		{"ios", core.OSIOS, core.IdiomPhone, 48, 44, 48},
		{"android", core.OSAndroid, core.IdiomPhone, 48, 44, 48},
		{"ipad", core.OSIOS, core.IdiomTablet, 48, 44, 48},
		{"android-tablet", core.OSAndroid, core.IdiomTablet, 48, 44, 48},
		{"macos", core.OSMacOS, core.IdiomDesktop, 32, 36, 40},
		{"windows", core.OSWindows, core.IdiomDesktop, 32, 36, 40},
		{"linux", core.OSLinux, core.IdiomDesktop, 32, 36, 40},
		{"web", core.OSWeb, core.IdiomDesktop, 32, 36, 40},
		{"tv", core.OSAndroid, core.IdiomTV, 32, 36, 40},
	} {
		t.Run(c.name, func(t *testing.T) {
			pressed := 0
			completed := ""
			app, _ := start(t, core.Environment{OS: c.os, Idiom: c.idiom}, func(*core.Scope) core.Node {
				return core.View{TestID: "column", Style: core.Style{Width: core.Pt(320), AlignItems: core.AlignStart, Gap: 8}, Children: []core.Node{
					Button{Text: "Action", Size: SizeLarge, Style: core.Style{Width: core.Pct(100)}, TestID: "button", OnPress: func() { pressed++ }},
					OtpInput{Length: 6, Groups: 2, TestID: "otp", Label: "Code", OnComplete: func(code string) { completed = code }},
					OtpInput{Length: 4, Size: 50, TestID: "custom-otp", Label: "Custom code"},
					FormRow{Title: "Setting", OnPress: func() { pressed++ }, TestID: "row"},
					FormRow{Title: "Read only", TestID: "plain-row"},
					FormField{Title: "Name", TestID: "field"},
				}}
			})
			button := app.Find("button")
			if got := button.Frame.Size; got.Width != 320 || got.Height != c.button || button.Children[0].Frame.Size != got {
				t.Fatalf("button and face = %v, %v; want 320 x %v", got, button.Children[0].Frame.Size, c.button)
			}
			otp := app.Find("otp")
			for _, group := range otp.Children[:len(otp.Children)-1] {
				for _, cell := range group.Children {
					if got := cell.Frame.Size; got.Width != c.cell || got.Height != c.cell {
						t.Fatalf("OTP cell = %v, want a %v square", got, c.cell)
					}
				}
			}
			if got := otp.Frame.Size.Width; got > 320 {
				t.Fatalf("six OTP digits overflow the phone column: %v > 320", got)
			}
			if got := app.Find("custom-otp").Children[0].Children[0].Frame.Size; got.Width != 50 || got.Height != 50 {
				t.Fatalf("custom OTP cell = %v, want a 50 point square", got)
			}
			for _, id := range []string{"row", "plain-row", "field"} {
				if got := app.Find(id).Frame.Size.Height; got != c.row {
					t.Fatalf("%s height = %v, want %v", id, got, c.row)
				}
			}
			app.Press("button")
			app.Press("row")
			app.Type("label:Code", "123456")
			if pressed != 2 || completed != "123456" {
				t.Fatal("touch sizing lost a press or OTP editing")
			}
		})
	}
}

// TestScrollbarTheme replaces the indicators, and the view's own bar goes
// with the theme.
func TestScrollbarTheme(t *testing.T) {
	headless(t)
	each(t, func(t *testing.T, e core.Environment) {
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			base.SetScrollbarTheme(cx, ScrollbarTheme(cx))
			return core.Scroll{TestID: "list", Style: core.Style{Height: core.Pt(100), Width: core.Pt(100)},
				Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(800)}}}}
		})
		if s := app.Find("list").Scroll; !s.HideIndicator {
			t.Fatal("the theme does not replace the platform's indicator")
		}
	})
}

// TestMultilineInputScrollbarTheme draws a Scrollbar over a multiline Input,
// follows scrolling, and drags its thumb to scroll the input's text (#1596).
func TestMultilineInputScrollbarTheme(t *testing.T) {
	headless(t)
	each(t, func(t *testing.T, e core.Environment) {
		var text strings.Builder
		for i := range 30 {
			fmt.Fprintf(&text, "line %d of multiline notes\n", i)
		}
		var scrolled []core.Points
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			theme := ScrollbarTheme(cx)
			theme.Mode = base.ScrollbarAlways
			theme.Vertical.Label = "Notes scrollbar"
			base.SetScrollbarTheme(cx, theme)
			return Input{
				TestID:       "notes",
				Multiline:    true,
				DefaultValue: text.String(),
				OnScroll:     func(ev core.ScrollEvent) { scrolled = append(scrolled, ev.Offset.Y) },
				Style:        core.Style{Width: core.Pt(220), Height: core.Pt(80)},
			}
		})
		in := app.Find("notes")
		if !in.Input.HideIndicator {
			t.Fatal("the theme does not hide the multiline input's native indicator")
		}
		if in.Content.Height <= in.Frame.Size.Height {
			t.Fatalf("multiline input content %v does not overflow frame %v", in.Content, in.Frame.Size)
		}
		if len(in.Children) != 1 {
			t.Fatalf("multiline input children = %d, want 1 bar host", len(in.Children))
		}
		bar := app.Find("label:Notes scrollbar")
		if bar == nil {
			t.Fatal("expected Scrollbar over multiline input")
		}
		// Scrolling the multiline input updates its offset and OnScroll.
		app.Scroll("notes", 60)
		if app.Find("notes").Offset.Y <= 0 {
			t.Fatalf("after Scroll: offset = %v, want > 0", app.Find("notes").Offset)
		}
		if len(scrolled) == 0 || scrolled[len(scrolled)-1] <= 0 {
			t.Fatalf("OnScroll did not receive multiline scroll offset: %v", scrolled)
		}
		// Dragging the scrollbar thumb moves the multiline input's text offset.
		before := app.Find("notes").Offset.Y
		app.Drag("label:Notes scrollbar", core.Point{Y: 20})
		after := app.Find("notes").Offset.Y
		if after <= before {
			t.Fatalf("dragging scrollbar thumb did not advance input offset: before=%v after=%v", before, after)
		}
	})
}
