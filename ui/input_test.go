package ui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestInput sizes a field and a multiline one, and checks a disabled
// field.
func TestInput(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		app, p := start(t, e, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				Input{Label: "Email", TestID: "email", Placeholder: "name@example.com"},
				Input{Label: "Notes", TestID: "notes", Multiline: true},
				Input{Label: "Off", TestID: "off", Disabled: func() bool { return true }},
			}}
		})
		if f := app.Find("email"); f.Frame.Size.Height != ButtonHeight || f.Style.BorderColor != p.FieldBorder {
			t.Fatalf("field = %v tall, edge %x; want %v and the field border", f.Frame.Size.Height, f.Style.BorderColor, ButtonHeight)
		}
		if got := app.Find("email").Input.CaretColor; got != p.Accent {
			t.Fatalf("caret = %x, want the accent", got)
		}
		if h := app.Find("notes").Frame.Size.Height; h <= ButtonHeight {
			t.Fatalf("multiline = %v tall, want more than a line", h)
		}
		if !app.Find("off").Input.Disabled {
			t.Fatal("the disabled field takes input")
		}
	})
}

// TestInputMergesCallerStyle sizes a field with its Style, colors one with its Style, gives one a StyleFunc and sizes multiline fields below their three lines, and checks each keeps the rest of ui's look: the focus rim and the hover rim stay while the caller's style sets neither.
func TestInputMergesCallerStyle(t *testing.T) {
	ink, paper := core.RGB(0x112233), core.RGB(0xfedcba)
	each(t, func(t *testing.T, e core.Environment) {
		app, p := start(t, e, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(240), Gap: 8}, Children: []core.Node{
				Input{TestID: "styled", Placeholder: "Search", Style: core.Style{
					Height: core.Pt(24), FontSize: 12, Padding: core.Edges{Left: 10, Right: 28},
				}, StyleFunc: func() core.Style { return core.Style{FontFamily: "Menlo"} }},
				Input{TestID: "colored", Style: core.Style{Color: ink, Background: paper}},
				Input{TestID: "plain"},
				Input{TestID: "short", Multiline: true, Style: core.Style{Height: core.Pt(40)}},
				Input{TestID: "lower", Multiline: true, StyleFunc: func() core.Style { return core.Style{MinHeight: core.Pt(40)} }},
				Input{TestID: "notes", Multiline: true},
			}}
		})
		f := app.Find("styled")
		if f.Frame.Size.Height != 24 {
			t.Fatalf("height = %v, want 24", f.Frame.Size.Height)
		}
		if f.Input.Style.Font.Size != 12 {
			t.Fatalf("font size = %v, want 12", f.Input.Style.Font.Size)
		}
		if right := f.Frame.Size.Width - f.Input.Content.Origin.X - f.Input.Content.Size.Width; right < 28 {
			t.Fatalf("right inset = %v, want at least 28", right)
		}
		if f.Input.Style.Font.Family != "Menlo" {
			t.Fatalf("font family = %q, want Menlo", f.Input.Style.Font.Family)
		}
		if f.Style.BorderColor != p.FieldBorder || f.Input.Style.Color != p.Text {
			t.Fatalf("styled field: edge %x, text %x; want the field border and the text color", f.Style.BorderColor, f.Input.Style.Color)
		}
		if c := app.Find("colored"); c.Input.Style.Color != ink || c.Style.Background != paper || c.Style.BorderColor != p.FieldBorder {
			t.Fatalf("colored field: text %x, fill %x, edge %x; want the style's and the field border", c.Input.Style.Color, c.Style.Background, c.Style.BorderColor)
		}
		if f := app.Find("plain"); f.Frame.Size.Height != ButtonHeight || f.Input.Style.Font.Size != 13 {
			t.Fatalf("plain field: %v tall, %v pt text; want %v and 13", f.Frame.Size.Height, f.Input.Style.Font.Size, ButtonHeight)
		}
		if h := app.Find("short").Frame.Size.Height; h != 40 {
			t.Fatalf("multiline field with a 40 pt height = %v tall", h)
		}
		if h := app.Find("lower").Frame.Size.Height; h != 40 {
			t.Fatalf("multiline field with a 40 pt minimum = %v tall", h)
		}
		if h := app.Find("notes").Frame.Size.Height; h != ButtonHeight+2*TypeBody.LineHeight {
			t.Fatalf("plain multiline field = %v tall, want three lines (%v)", h, ButtonHeight+2*TypeBody.LineHeight)
		}
		if e.Idiom == core.IdiomDesktop {
			app.Hover("styled")
			if got := app.Find("styled").Style.BorderColor; got == p.FieldBorder {
				t.Fatalf("hovered edge = %x, want darker than the field border", got)
			}
			app.HoverOut()
		}
		app.Focus("styled")
		if got := app.Find("styled").Style.BorderColor; got != p.Accent {
			t.Fatalf("focused edge = %x, want the accent %x", got, p.Accent)
		}
	})
}

// TestComboboxKeepsChevronInset checks that a combobox's text stops before its chevron.
func TestComboboxKeepsChevronInset(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(240)}, Children: []core.Node{
				Combobox[string]{TestID: "fruit", Label: "Fruit", Options: []Option[string]{{Value: "a", Label: "Apple"}}},
			}}
		})
		var field *nsgotest.View
		var walk func(*nsgotest.View)
		walk = func(v *nsgotest.View) {
			for _, child := range v.Children {
				if field != nil {
					return
				}
				if child.Kind == nsgotest.KindInput {
					field = child
					return
				}
				walk(child)
			}
		}
		walk(app.Find("fruit"))
		if field == nil {
			t.Fatal("combobox has no input field")
		}
		if right := field.Frame.Size.Width - field.Input.Content.Origin.X - field.Input.Content.Size.Width; right < 28 {
			t.Fatalf("text runs under the chevron: right inset %v", right)
		}
	})
}
