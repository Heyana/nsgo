// Command gallery shows NSGo's elements and base components: navigation,
// controls, popups, dialogs and images.
package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

//go:embed logo.png
var logo []byte

var (
	ink     = core.RGB(0x1f2328)
	muted   = core.RGB(0x6e7781)
	accent  = core.RGB(0x0969da)
	surface = core.RGB(0xffffff)
	line    = core.RGB(0xd0d7de)

	card   = core.Style{Background: surface, CornerRadius: 12, BorderWidth: 1, BorderColor: line, Padding: core.All(16), Gap: 12}
	row    = core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}
	button = core.Style{Background: accent, CornerRadius: 8, Padding: core.Symmetric(8, 14), AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.8)}}
	onAccent = core.Style{Color: surface, FontWeight: core.FontWeightSemibold, FontSize: 15}
	popup    = core.Style{Background: surface, CornerRadius: 10, BorderWidth: 1, BorderColor: line, Padding: core.All(6),
		ShadowColor: core.RGBA(0x000000, 0.15), ShadowRadius: 12, ShadowOffset: core.Point{Y: 4}}
	item = core.Style{Padding: core.Symmetric(8, 12), CornerRadius: 6, Hover: &core.Style{Background: core.RGB(0xf3f4f6)}}
)

func label(s string) core.Node { return core.Text{Content: s, Style: onAccent} }

type demo struct {
	name string
	page func(cx *core.Scope) core.Node
}

var demos = []demo{
	{"Controls", controls},
	{"Popups", popups},
	{"Images", images},
}

// Root is a stack: the list of demos, each pushed over it.
// NSGO_GALLERY_OPEN names a demo to open at start, for screenshots.
func Root(cx *core.Scope) core.Node {
	nav := base.NewNav(cx, "")
	if open := os.Getenv("NSGO_GALLERY_OPEN"); open != "" {
		nav.Push(open)
	}
	return nav.Stack(core.Style{FlexGrow: 1, Background: core.RGB(0xf4f5f7), Color: ink},
		func(cx *core.Scope, name string, depth func() int) core.Screen {
			if name == "" {
				return core.Screen{Title: "Gallery", Header: header("Gallery", nil), TestID: "home",
					Children: []core.Node{index(nav)}}
			}
			for _, d := range demos {
				if d.name == name {
					return core.Screen{Title: name, Header: header(name, func() { nav.Pop(1) }), TestID: "screen-" + name,
						Children: []core.Node{core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
							core.View{Style: core.Style{Padding: core.All(16), Gap: 16}, Children: []core.Node{d.page(cx)}},
						}}}}
				}
			}
			return core.Screen{}
		})
}

func header(title string, back func()) core.Node {
	children := []core.Node{}
	if back != nil {
		children = append(children, base.Button{OnPress: back, Label: "Back", Style: core.Style{Padding: core.Symmetric(4, 8)},
			Children: []core.Node{core.Text{Content: "‹ Back", Style: core.Style{Color: accent, FontSize: 16}}}})
	}
	children = append(children, core.Text{Content: title, Style: core.Style{FontSize: 20, FontWeight: core.FontWeightBold}})
	return base.SafeAreaView{Edges: base.Top, Style: core.Style{Background: surface, BorderColor: line},
		Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
			Padding: core.Symmetric(12, 16)}, Children: children}}}
}

func index(nav *base.Nav[string]) core.Node {
	var items []core.Node
	for _, d := range demos {
		items = append(items, base.Button{OnPress: func() { nav.Push(d.name) }, Label: d.name, TestID: "open-" + d.name,
			Style: core.Style{Padding: core.All(16), Background: surface, CornerRadius: 10, BorderWidth: 1, BorderColor: line,
				Active: &core.Style{Background: core.RGB(0xeef1f4)}},
			Children: []core.Node{core.Text{Content: d.name, Style: core.Style{FontSize: 17}}}})
	}
	return core.View{Style: core.Style{Padding: core.All(16), Gap: 10}, Children: items}
}

func controls(cx *core.Scope) core.Node {
	agree := core.NewSignal(cx, false)
	wifi := core.NewSignal(cx, true)
	size := core.NewSignal(cx, "M")
	tab := core.NewSignal(cx, 0)

	check := core.Style{Width: core.Pt(20), Height: core.Pt(20), CornerRadius: 5, BorderWidth: 2, BorderColor: line,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
		Checked: &core.Style{Background: accent, BorderColor: accent}}
	radio := func(v string) core.Node {
		return base.Radio{Checked: func() bool { return size.Get() == v }, OnChange: func(bool) { size.Set(v) }, Label: v,
			Style: row, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9, BorderWidth: 2, BorderColor: line},
					Checked: func() bool { return size.Get() == v }},
				core.Text{Content: v},
			}}
	}
	tabStyle := core.Style{Padding: core.Symmetric(6, 12), CornerRadius: 6,
		Selected: &core.Style{Background: accent, Color: surface}}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		core.View{Style: card, Children: []core.Node{
			base.Checkbox{Checked: agree.Get, OnChange: agree.Set, Label: "Agree", Style: row, IndicatorStyle: check,
				Indicator: core.Show{When: agree.Get, Then: func(*core.Scope) core.Node {
					return core.Text{Content: "✓", Style: core.Style{Color: surface, FontSize: 13, FontWeight: core.FontWeightBold}}
				}},
				Children: []core.Node{core.Text{Content: "I agree to the terms"}}},
			base.Switch{Checked: wifi.Get, OnChange: wifi.Set, Label: "Wi-Fi", Style: row,
				TrackStyle: core.Style{Width: core.Pt(44), Height: core.Pt(26), CornerRadius: 13, Background: core.RGB(0xd0d7de),
					Padding: core.All(3), Checked: &core.Style{Background: core.RGB(0x1f883d), AlignItems: core.AlignEnd}},
				ThumbStyle: core.Style{Width: core.Pt(20), Height: core.Pt(20), CornerRadius: 10, Background: surface},
				Children:   []core.Node{core.Text{ContentFunc: func() string { return fmt.Sprintf("Wi-Fi %v", map[bool]string{true: "on", false: "off"}[wifi.Get()]) }}}},
		}},
		core.View{Style: card, Children: []core.Node{
			core.Text{Content: "Size", Style: core.Style{Color: muted}},
			base.RadioGroup{Label: "Size", Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{radio("S"), radio("M"), radio("L")}},
		}},
		core.View{Style: card, Children: []core.Node{
			base.Tabs{Style: core.Style{Direction: core.Row, Gap: 6}, Children: []core.Node{
				base.Tab{Selected: func() bool { return tab.Get() == 0 }, OnPress: func() { tab.Set(0) }, Style: tabStyle, Label: "Day",
					Children: []core.Node{core.Text{Content: "Day"}}},
				base.Tab{Selected: func() bool { return tab.Get() == 1 }, OnPress: func() { tab.Set(1) }, Style: tabStyle, Label: "Week",
					Children: []core.Node{core.Text{Content: "Week"}}},
			}},
			core.Text{ContentFunc: func() string { return []string{"Today's view", "This week's view"}[tab.Get()] }},
		}},
	}}
}

func popups(cx *core.Scope) core.Node {
	dialog := core.NewSignal(cx, false)
	sheet := core.NewSignal(cx, false)
	picked := core.NewSignal(cx, "nothing")
	return core.View{Style: card, Children: []core.Node{
		base.Popover{Style: button, PopupStyle: popup, Offset: 6, Trigger: label("Popover"),
			Children: []core.Node{core.Text{Content: "A popover beside its trigger.", Style: core.Style{Padding: core.All(8)}}}},
		base.Menu{Style: button, PopupStyle: popup, Offset: 6, Trigger: label("Menu"), Children: []core.Node{
			base.MenuItem{Style: item, OnSelect: func() { picked.Set("Copy") }, Label: "Copy", Children: []core.Node{core.Text{Content: "Copy"}}},
			base.MenuItem{Style: item, OnSelect: func() { picked.Set("Paste") }, Label: "Paste", Children: []core.Node{core.Text{Content: "Paste"}}},
			base.MenuSeparator{Style: core.Style{Height: core.Pt(1), Background: line, Margin: core.Symmetric(4, 0)}},
			base.MenuItem{Style: item, Label: "Delete", Children: []core.Node{core.Text{Content: "Delete…", Style: core.Style{Color: core.RGB(0xcf222e)}}},
				OnSelect: func() {
					base.ShowAlertDialog(cx, base.AlertDialog{Title: "Delete it?", Description: "This cannot be undone.",
						Actions: []core.AlertAction{{Label: "Cancel", Role: core.AlertCancel}, {Label: "Delete", Role: core.AlertDestructive}}},
						func(choice string) { picked.Set("alert: " + choice) })
				}},
		}},
		core.Text{ContentFunc: func() string { return "Picked " + picked.Get() }, Style: core.Style{Color: muted}},
		base.Button{OnPress: func() { dialog.Set(true) }, Style: button, Children: []core.Node{label("Dialog")}},
		base.Button{OnPress: func() { sheet.Set(true) }, Style: button, Children: []core.Node{label("Sheet")}},
		base.Tooltip{Trigger: core.Text{Content: "Hover me (desktop)", Style: core.Style{Color: accent}}, PopupStyle: popup,
			Children: []core.Node{core.Text{Content: "A tooltip", Style: core.Style{FontSize: 13}}}},
		base.Dialog{Open: dialog.Get, OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
			BackdropStyle: core.Style{Background: core.RGBA(0x000000, 0.35)},
			PopupStyle:    core.Style{Background: surface, CornerRadius: 14, Padding: core.All(20), Gap: 12, Width: core.Pct(80)},
			Label:         "Dialog", Children: []core.Node{
				core.Text{Content: "A dialog", Style: core.Style{FontSize: 18, FontWeight: core.FontWeightBold}},
				core.Text{Content: "Escape, back or the backdrop closes it."},
				base.Button{OnPress: func() { dialog.Set(false) }, Style: button, Children: []core.Node{label("Close")}},
			}},
		base.Sheet{Open: sheet.Get, OnOpenChange: func(open bool, _ base.Reason) { sheet.Set(open) },
			BackdropStyle: core.Style{Background: core.RGBA(0x000000, 0.35)},
			PopupStyle: core.Style{Background: surface, Padding: core.All(20), Gap: 12, CornerRadius: 16,
				Height: core.Pct(40)}, Label: "Sheet", Children: []core.Node{
				base.SafeAreaView{Edges: base.Bottom, Children: []core.Node{
					core.Text{Content: "A sheet from the bottom", Style: core.Style{FontSize: 18, FontWeight: core.FontWeightBold}},
				}},
			}},
	}}
}

func images(cx *core.Scope) core.Node {
	status := core.NewSignal(cx, "loading…")
	return core.View{Style: card, Children: []core.Node{
		core.View{Style: row, Children: []core.Node{
			core.Image{Source: core.ImageSource{Data: logo, Size: core.Size{Width: 96, Height: 96}}, Label: "Logo", TestID: "logo"},
			core.Image{Source: core.ImageSource{Data: logo}, ResizeMode: core.Contain, Label: "Tinted",
				Style: core.Style{Width: core.Pt(48), Height: core.Pt(48), TintColor: core.RGB(0xcf222e)}},
		}},
		core.Image{Source: core.Src("https://picsum.photos/id/1015/600/300"), ResizeMode: core.Cover, Label: "River",
			Style:       core.Style{Height: core.Pt(160), CornerRadius: 10, Overflow: core.Hidden},
			OnLoad:      func(px core.Size) { status.Set(fmt.Sprintf("loaded %g×%g px", px.Width, px.Height)) },
			OnError:     func(err error) { status.Set("error: " + err.Error()) },
			Placeholder: core.ImageSource{Data: logo}},
		core.Text{ContentFunc: status.Get, Style: core.Style{Color: muted, FontSize: 13}},
	}}
}

func main() { nsgo.Run(Root) }
