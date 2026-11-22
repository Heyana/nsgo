package ui

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestFormSection divides the rows from the start of the titles above, has a
// pressable row press and a field edit its text.
func TestFormSection(t *testing.T) {
	headless(t)
	pressed := 0
	var text *core.Signal[string]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		text = core.NewSignal(cx, "")
		return FormSection{Title: "Account", Footer: "Kept on this device.", TestID: "section", Children: []core.Node{
			FormField{Title: "Name", Input: base.Input{Value: text.Get, OnChange: text.Set, Placeholder: "Ada"}},
			FormRow{Icon: lucide.Bell, Title: "Alerts", Value: func() string { return "On" }, OnPress: func() { pressed++ }, TestID: "alerts"},
			FormRow{Title: "Sign out", Variant: FormRowDanger, OnPress: func() { pressed++ }},
		}}
	})
	card := app.Find("id:section").Children[1]
	if got := cardRows(card); got != "r|r|r" {
		t.Fatalf("card holds %q, want 3 rows and 2 hairlines", got)
	}
	if x := card.Children[1].Frame.Origin.X; x != formRowPad {
		t.Errorf("hairline below a field starts at %v, want %v", x, formRowPad)
	}
	if x, want := card.Children[3].Frame.Origin.X, formRowPad+formIconTile+formRowGap; x != want {
		t.Errorf("hairline below a row with an icon starts at %v, want %v", x, want)
	}
	app.Press("id:alerts")
	app.Press("Sign out")
	if pressed != 2 {
		t.Fatalf("pressed %d rows, want 2", pressed)
	}
	app.Type("label:Name", "Grace")
	if text.GetUntracked() != "Grace" {
		t.Fatalf("field holds %q", text.GetUntracked())
	}
}

func TestStyledFormSectionAlignsDividersWithLeadingSlots(t *testing.T) {
	fill, nextFill, border := core.RGB(0x223344), core.RGB(0x445566), core.RGB(0x667788)
	var background *core.Signal[core.Color]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		background = core.NewSignal(cx, core.Color(0))
		withIcon := core.Each(func() []string { return []string{"Alerts"} }, func(s string) string { return s },
			func(_ *core.Scope, item func() string, _ func() int) core.Node {
				return &FormRow{Title: item(), TestID: "alerts", Icon: lucide.Bell, TileSize: 30}
			})
		return FormSection{Title: "Account", Footer: "Kept on this device.", TestID: "section",
			CardStyle:     core.Style{Background: fill, BorderColor: border, BorderWidth: 2, CornerRadius: 16},
			CardStyleFunc: func() core.Style { return core.Style{Background: background.Get()} },
			Children: []core.Node{
				FormRow{Title: "Artwork", Detail: "Photo library", LeadingWidth: 38,
					Leading:     core.View{TestID: "artwork", Style: core.Style{Width: core.Pt(38), Height: core.Pt(32)}},
					TitleStyle:  core.Style{FontSize: 17, FontWeight: Semibold},
					DetailStyle: core.Style{FontSize: 14, LineHeight: 18}},
				withIcon,
				FormRow{Title: "Sign out"},
			}}
	})
	section := app.Find("section")
	card := section.Children[1]
	if section.Children[0].Text != "Account" || section.Children[2].Text != "Kept on this device." || section.Style.Background == fill {
		t.Fatal("card style changed the section header, outer view or footer")
	}
	if style := card.Style; style.Background != fill || style.BorderColor != border || style.BorderWidth != 2 || style.CornerRadius != 16 {
		t.Fatalf("card style = %+v", style)
	}
	app.Post(func() { background.Set(nextFill) })
	app.Settle()
	if got := card.Style.Background; got != nextFill {
		t.Fatalf("card background = %v, want the reactive fill %v", got, nextFill)
	}
	if size := app.Find("artwork").Frame.Size; size.Width != 38 || size.Height != 32 {
		t.Fatalf("leading node size = %v, want 38 by 32", size)
	}
	tile := app.Find("alerts").Children[0]
	if size := tile.Frame.Size; size.Width != 30 || size.Height != 30 || tile.Style.CornerRadius != 7.5 {
		t.Fatalf("tile = %v with radius %v, want 30 by 30 with radius 7.5", size, tile.Style.CornerRadius)
	}
	if got := cardRows(card); got != "r|r|r" {
		t.Fatalf("card holds %q, want three rows and two dividers", got)
	}
	artwork, alerts := app.Find("text:Artwork"), app.Find("text:Alerts")
	for i, title := range []*nsgotest.View{artwork, alerts} {
		if got, want := card.Children[2*i+1].Bounds().Origin.X, title.Bounds().Origin.X; got != want {
			t.Errorf("divider below %s starts at %v, want %v", title.Text, got, want)
		}
	}
	if style := artwork.TextStyle; style.Font.Size != 17 || style.Font.Weight != Semibold {
		t.Fatalf("title style = %+v", style)
	}
	if style := app.Find("text:Photo library").TextStyle; style.Font.Size != 14 || style.LineHeight != 18 {
		t.Fatalf("detail style = %+v", style)
	}
}

// TestFormSectionDividesRowsOfEach gives each row from core.Each the hairline below it, from where its title starts, and keeps the card's last view free of one, as rows come, go and move, and as an Each next to rows of their own empties.
func TestFormSectionDividesRowsOfEach(t *testing.T) {
	headless(t)
	var list, more *core.Signal[[]string]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		list, more = core.NewSignalFunc(cx, []string{"one", "two"}, nil), core.NewSignalFunc(cx, []string(nil), nil)
		row := func(cx *core.Scope, item func() string, _ func() int) core.Node {
			if item() == "bell" {
				return FormRow{Icon: lucide.Bell, Title: item()}
			}
			return FormRow{Title: item()}
		}
		key := func(s string) string { return s }
		return core.View{Children: []core.Node{
			FormSection{TestID: "each", Children: []core.Node{core.Each(list.Get, key, row)}},
			FormSection{TestID: "mixed", Children: []core.Node{
				FormRow{Title: "Top"}, core.Each(more.Get, key, row), FormRow{Title: "Add"},
			}},
		}}
	})
	card := func(id string) *nsgotest.View { return app.FindMounted("id:" + id).Children[0] }
	check := func(step, id, want string) {
		t.Helper()
		app.Settle()
		if got := cardRows(card(id)); got != want {
			t.Errorf("%s: %s card holds %q, want %q", step, id, got, want)
		}
	}
	check("start", "each", "r|r")
	check("start", "mixed", "r|r")
	list.Set([]string{"one", "two", "three"})
	check("append", "each", "r|r|r")
	list.Set([]string{"two", "three"})
	check("remove the first", "each", "r|r")
	list.Set([]string{"three", "bell", "two"})
	check("insert and move", "each", "r|r|r")
	if x, want := card("each").Children[3].Frame.Origin.X, formRowPad+formIconTile+formRowGap; x != want {
		t.Errorf("hairline below a row with an icon starts at %v, want %v", x, want)
	}
	if x := card("each").Children[1].Frame.Origin.X; x != formRowPad {
		t.Errorf("hairline below a row without one starts at %v, want %v", x, formRowPad)
	}
	list.Set([]string{"two", "three", "bell"})
	check("move the last", "each", "r|r|r")
	if x := card("each").Children[3].Frame.Origin.X; x != formRowPad {
		t.Errorf("after the move the hairline below the second row starts at %v, want %v", x, formRowPad)
	}
	list.Set(nil)
	check("empty", "each", "")
	more.Set([]string{"one"})
	check("fill the middle", "mixed", "r|r|r")
	more.Set(nil)
	check("empty the middle", "mixed", "r|r")
}

// cardRows spells a card's views: r for a row, | for a hairline shown and nothing for one hidden.
func cardRows(card *nsgotest.View) string {
	var b strings.Builder
	for _, v := range card.Children {
		switch {
		case v.Style.Hidden:
		case v.Frame.Size.Height == 1:
			b.WriteByte('|')
		default:
			b.WriteByte('r')
		}
	}
	return b.String()
}

func TestFormSectionOverridesActualChildDividerInsets(t *testing.T) {
	headless(t)
	var items *core.Signal[[]string]
	var seen []string
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		items = core.NewSignalFunc(cx, []string{"flush", "custom"}, nil)
		rows := core.Each(items.Get, func(item string) string { return item }, func(_ *core.Scope, item func() string, _ func() int) core.Node {
			return core.View{TestID: item(), Style: core.Style{Height: core.Pt(32)}}
		})
		return FormSection{TestID: "custom-insets", DividerInset: func(row core.Node) (core.Points, bool) {
			if row, ok := row.(core.View); ok {
				seen = append(seen, row.TestID)
				if row.TestID == "flush" {
					return 0, true
				}
				return 27, true
			}
			return 0, false
		}, Children: []core.Node{FormRow{Title: "Automatic", Icon: lucide.Bell}, rows, FormRow{Title: "Last"}}}
	})
	check := func() {
		t.Helper()
		card := app.Find("custom-insets").Children[0]
		for i, want := range []core.Points{formRowPad + formIconTile + formRowGap, 0, 27} {
			if got := card.Children[2*i+1].Frame.Origin.X; got != want {
				t.Fatalf("divider %d inset = %v, want %v", i, got, want)
			}
		}
	}
	check()
	if strings.Join(seen, ",") != "flush,custom" {
		t.Fatalf("callback rows = %v, want actual Each rows", seen)
	}
	app.Post(func() { items.Set([]string{"custom", "flush"}) })
	app.Settle()
	card := app.Find("custom-insets").Children[0]
	if card.Children[3].Frame.Origin.X != 27 || card.Children[5].Frame.Origin.X != 0 {
		t.Fatal("dividers did not keep their row's inset after reordering")
	}
	app.Post(func() { items.Set(nil) })
	app.Settle()
	if got := cardRows(app.Find("custom-insets").Children[0]); got != "r|r" {
		t.Fatalf("empty Each left %q", got)
	}
}
