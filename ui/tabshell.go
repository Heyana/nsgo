package ui

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
)

// ShellTab is a top-level section of a TabShell.
type ShellTab struct {
	Text     string
	TextFunc func() string
	// Icon draws the tab's SVG, in Android's native bar too wherever SystemIcon names no drawable there.
	Icon func() *core.SVG
	// SystemIcon is the platform's name for the tab's icon on iOS and Android, as mobile.TabBarItem's Icon.
	SystemIcon string
	// Content is the section, such as a base.Nav's stack of pages. It is
	// built when the tab is first selected and kept while others show.
	Content func(cx *core.Scope) core.Node
	TestID  string
}

// TabShell is an app's top-level sections over a drawn tab bar, which keeps clear of the bottom safe area, or the system's on iOS where every tab has a SystemIcon and on Android where every tab has a SystemIcon or an Icon. iOS's floats over the sections, which keep what they must show clear of it with TabBarInset. Every other bar takes room of its own, and the bottom safe area of the sections above it (core.Insets, core.EnvSafeArea) is 0 while it shows. The Selected tab's content fills the rest; Selected is the app's, set from OnChange. The bar hides while the software keyboard shows, so content padded by core.EnvKeyboard meets the keyboard without an offset. It keeps this form at every window width.
type TabShell struct {
	Tabs     []ShellTab
	Selected func() int
	OnChange func(index int)
	// BarHidden hides the system or drawn bar while it reports true, and restores the section's bottom safe area and TabBarInset.
	BarHidden func() bool
	// Label names the bar for assistive technology.
	Label     string
	LabelFunc func() string
	TestID    string
}

// tabBarInsetKey provides a floating tab bar's inset to its sections.
var tabBarInsetKey = core.NewKey[func() core.Points]("tabBarInset", nil)

// TabBarInset is how much of a TabShell section's bottom the floating tab
// bar covers, read in a StyleFunc: 0 where the bar takes room of its own.
func TabBarInset(cx *core.Scope) core.Points {
	if inset := tabBarInsetKey.Use(cx); inset != nil {
		return inset()
	}
	return 0
}

// Render implements core.Node.
func (s TabShell) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	selected := func() int {
		if s.Selected == nil {
			return 0
		}
		return s.Selected()
	}
	change := func(i int) {
		if s.OnChange != nil {
			s.OnChange(i)
		}
	}
	env := cx.Env()
	floating := env.OS() == core.OSIOS && s.hasSystemBar(cx)
	hidden := func() bool { return s.BarHidden != nil && s.BarHidden() }
	var sections []core.Node
	for i, tab := range s.Tabs {
		// A section is built when first selected, then hidden while others
		// show, keeping its state.
		seen := false
		sections = append(sections, core.View{TestID: tab.TestID, Children: []core.Node{core.Show{
			When: func() bool { seen = seen || selected() == i; return seen },
			Then: func(cx *core.Scope) core.Node {
				if tab.Content == nil {
					return nil
				}
				if !floating {
					// A bar that takes room of its own keeps clear of the bottom safe area itself, so the section ends above it; while the keyboard hides the bar, the section reaches the bottom again.
					outer := core.InsetsFunc(cx)
					core.ProvideInsets(cx, func() core.Edges {
						in := outer()
						if env.KeyboardHeight() == 0 && !hidden() {
							in.Bottom = 0
						}
						return in
					})
				}
				return tab.Content(cx)
			}}},
			StyleFunc: func() core.Style {
				s := FillRest
				if selected() != i {
					s.Display = core.DisplayNone
				}
				return s
			}})
	}
	height := core.NewSignal(cx, core.Points(0))
	if floating {
		tabBarInsetKey.Provide(cx, func() core.Points {
			if env.KeyboardHeight() > 0 || hidden() {
				return 0
			}
			return height.Get()
		})
	}
	return core.View{TestID: s.TestID, Style: FillRest, StyleFunc: func() core.Style {
		return core.Style{Background: colors.Background(), Color: colors.Text()}
	}, Children: []core.Node{
		core.View{Style: FillRest, Children: sections},
		core.View{Children: []core.Node{s.bar(cx, selected, change)}, OnLayout: func(b core.Bounds) { height.Set(b.Size.Height) },
			StyleFunc: func() core.Style {
				var st core.Style
				if floating {
					st = core.Style{Position: core.Absolute, Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0)}
				}
				if env.KeyboardHeight() > 0 || hidden() {
					st.Display = core.DisplayNone
				}
				return st
			}},
	}}
}

// bar is the tab bar: the tabs' icons over their text, the selected
// one in the accent.
func (s TabShell) bar(cx *core.Scope, selected func() int, change func(int)) core.Node {
	label := s.Label
	if s.LabelFunc != nil {
		label = ""
	}
	if s.hasSystemBar(cx) {
		android := cx.Env().OS() == core.OSAndroid
		return mobile.TabBar{ItemsFunc: func() []mobile.TabBarItem { return s.systemItems(android) },
			Selected: selected, OnChange: change, Label: label, LabelFunc: s.LabelFunc, TestID: "tab-bar"}
	}
	colors := ColorsOf(cx)
	var tabs []core.Node
	for i, tab := range s.Tabs {
		var content []core.Node
		if tab.Icon != nil {
			content = append(content, Icon(tab.Icon, 22, Inherited))
		}
		text := tab.Text
		if tab.TextFunc != nil {
			text = ""
		}
		content = append(content, core.Text{Content: text, ContentFunc: tab.TextFunc,
			Style: TypeCaption.Merge(core.Style{FontWeight: Medium}), MaxLines: 1})
		tabs = append(tabs, base.Tab{Label: text, LabelFunc: tab.TextFunc, Selected: func() bool { return selected() == i },
			OnPress: func(core.PressEvent) { change(i) },
			Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), AlignItems: core.AlignCenter,
				JustifyContent: core.JustifyCenter, Gap: 2, MinHeight: core.Pt(RowHeight + 4), CornerRadius: RadiusControl,
				Transition: quick},
			StyleFunc: func() core.Style {
				return core.Style{Color: colors.Muted(), Hover: &core.Style{Color: colors.Text()}, Active: &core.Style{Background: colors.Pressed()},
					Selected: &core.Style{Color: colors.Accent()}, FocusVisible: FocusRing(cx, -2)}
			},
			Children: content})
	}
	return core.View{Style: core.Style{Direction: core.Row, Padding: core.Symmetric(4, 8), Borders: core.Edges{Top: 1},
		PaddingEnv: core.EnvEdges{Right: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea}},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Surface(), BorderColors: core.EdgeColors{Top: colors.Line()}}
		}, Children: []core.Node{
			base.Tabs{Label: label, LabelFunc: s.LabelFunc, TestID: "tab-bar", Style: core.Style{Direction: core.Row, FlexGrow: 1, Gap: 4}, Children: tabs},
		}}
}

// hasSystemBar says whether the system's tab bar shows the tabs, checking the icon sources without reading their reactive values.
func (s TabShell) hasSystemBar(cx *core.Scope) bool {
	if !mobile.IsAvailable(cx) {
		return false
	}
	for _, tab := range s.Tabs {
		if tab.SystemIcon == "" && (cx.Env().OS() != core.OSAndroid || tab.Icon == nil) {
			return false
		}
	}
	return true
}

// systemItems are the tabs for the system's tab bar. On Android each also carries its SVG, which the bar shows where SystemIcon names no drawable, such as an SF Symbol name shared with iOS.
func (s TabShell) systemItems(android bool) []mobile.TabBarItem {
	items := make([]mobile.TabBarItem, len(s.Tabs))
	for i, tab := range s.Tabs {
		title := tab.Text
		if tab.TextFunc != nil {
			title = tab.TextFunc()
		}
		items[i] = mobile.TabBarItem{Title: title, Icon: tab.SystemIcon}
		if android && tab.Icon != nil {
			items[i].SVG = tab.Icon()
		}
	}
	return items
}
