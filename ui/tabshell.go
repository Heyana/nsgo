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

// TabShell is an app's top-level sections beside a drawn tab bar, which keeps clear of its edge's safe area, or the system's on iOS where every tab has a SystemIcon and on Android (on the bottom edge) where every tab has a SystemIcon or an Icon. On the bottom edge iOS's floats over the sections, which keep what they must show clear of it with TabBarInset. Every other bar takes room of its own, and the safe area of the sections on the bar's edge (core.Insets, core.EnvSafeArea) is 0 while it shows. Set Edge (or EdgeFunc) to mobile.TabBarLeading or mobile.TabBarTrailing to draw the bar vertically along that edge, or mobile.TabBarAutomatic to follow core.Env.VerticalBarEdge. The Selected tab's content fills the rest; Selected is the app's, set from OnChange. The bar hides while the software keyboard shows, so content padded by core.EnvKeyboard meets the keyboard without an offset. It keeps this form at every window width.
type TabShell struct {
	Tabs     []ShellTab
	Selected func() int
	OnChange func(index int)
	// Edge places the tab bar along the bottom (the default), the leading
	// or trailing edge (drawn vertically), or lets the system choose via
	// core.Env.VerticalBarEdge (mobile.TabBarAutomatic). EdgeFunc binds it
	// reactively.
	Edge     mobile.TabBarEdge
	EdgeFunc func() mobile.TabBarEdge
	// BarHidden hides the system or drawn bar while it reports true, and restores the section's safe area and TabBarInset.
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
	resolvedEdge := func() mobile.TabBarEdge {
		edge := s.Edge
		if s.EdgeFunc != nil {
			edge = s.EdgeFunc()
		}
		return edge.Resolve(env)
	}
	hasSystem := s.hasSystemBar(cx)
	useNative := func() bool {
		if !hasSystem {
			return false
		}
		return env.OS() == core.OSIOS || resolvedEdge() == mobile.TabBarBottom
	}
	floating := func() bool {
		return env.OS() == core.OSIOS && hasSystem && resolvedEdge() == mobile.TabBarBottom
	}
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
				// A bar that takes room of its own keeps clear of its edge's safe area itself, so the section ends beside it; while floating, hidden, or covered by the keyboard, the section keeps its safe area.
				outer := core.InsetsFunc(cx)
				core.ProvideInsets(cx, func() core.Edges {
					in := outer()
					if floating() || env.KeyboardHeight() > 0 || hidden() {
						return in
					}
					switch resolvedEdge() {
					case mobile.TabBarLeading:
						if env.RTL() {
							in.Right = 0
						} else {
							in.Left = 0
						}
					case mobile.TabBarTrailing:
						if env.RTL() {
							in.Left = 0
						} else {
							in.Right = 0
						}
					default:
						in.Bottom = 0
					}
					return in
				})
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
	tabBarInsetKey.Provide(cx, func() core.Points {
		if !floating() || env.KeyboardHeight() > 0 || hidden() {
			return 0
		}
		return height.Get()
	})
	return core.View{TestID: s.TestID, Style: FillRest, StyleFunc: func() core.Style {
		st := core.Style{Background: colors.Background(), Color: colors.Text()}
		switch resolvedEdge() {
		case mobile.TabBarLeading:
			st.Direction = core.RowReverse
		case mobile.TabBarTrailing:
			st.Direction = core.Row
		default:
			st.Direction = core.Column
		}
		return st
	}, Children: []core.Node{
		core.View{Style: FillRest, Children: sections},
		core.View{Children: []core.Node{s.bar(cx, selected, change, resolvedEdge, useNative)}, OnLayout: func(b core.Bounds) { height.Set(b.Size.Height) },
			StyleFunc: func() core.Style {
				var st core.Style
				if floating() {
					st = core.Style{Position: core.Absolute, Right: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0)}
				} else if resolvedEdge() != mobile.TabBarBottom {
					st = core.Style{Direction: core.Column}
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
func (s TabShell) bar(cx *core.Scope, selected func() int, change func(int), resolvedEdge func() mobile.TabBarEdge, useNative func() bool) core.Node {
	label := s.Label
	if s.LabelFunc != nil {
		label = ""
	}
	nativeBar := func(cx *core.Scope) core.Node {
		android := cx.Env().OS() == core.OSAndroid
		return mobile.TabBar{ItemsFunc: func() []mobile.TabBarItem { return s.systemItems(android) },
			Selected: selected, OnChange: change, EdgeFunc: resolvedEdge, Label: label, LabelFunc: s.LabelFunc,
			Style: core.Style{FlexGrow: 1}, TestID: "tab-bar"}
	}
	drawnBar := func(cx *core.Scope) core.Node {
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
				Style: core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Gap: 2, CornerRadius: RadiusControl, Transition: quick},
				StyleFunc: func() core.Style {
					st := core.Style{Color: colors.Muted(), Hover: &core.Style{Color: colors.Text()}, Active: &core.Style{Background: colors.Pressed()},
						Selected: &core.Style{Color: colors.Accent()}, FocusVisible: FocusRing(cx, -2)}
					if resolvedEdge() == mobile.TabBarBottom {
						st.FlexGrow = 1
						st.FlexBasis = core.Pt(0)
						st.MinWidth = core.Pt(0)
						st.MinHeight = core.Pt(RowHeight + 4)
					} else {
						st.MinWidth = core.Pt(56)
						st.MinHeight = core.Pt(RowHeight + 12)
						st.Padding = core.Symmetric(6, 4)
					}
					return st
				},
				Children: content})
		}
		return core.View{
			StyleFunc: func() core.Style {
				st := core.Style{Background: colors.Surface()}
				switch resolvedEdge() {
				case mobile.TabBarLeading:
					st.Direction = core.Column
					st.FlexGrow = 1
					st.MinWidth = core.Pt(72)
					st.Padding = core.Symmetric(8, 6)
					st.Borders = core.Edges{Right: 1}
					st.BorderColors = core.EdgeColors{Right: colors.Line()}
					st.PaddingEnv = core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea}
				case mobile.TabBarTrailing:
					st.Direction = core.Column
					st.FlexGrow = 1
					st.MinWidth = core.Pt(72)
					st.Padding = core.Symmetric(8, 6)
					st.Borders = core.Edges{Left: 1}
					st.BorderColors = core.EdgeColors{Left: colors.Line()}
					st.PaddingEnv = core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea, Right: core.EnvSafeArea}
				default:
					st.Direction = core.Row
					st.Padding = core.Symmetric(4, 8)
					st.Borders = core.Edges{Top: 1}
					st.BorderColors = core.EdgeColors{Top: colors.Line()}
					st.PaddingEnv = core.EnvEdges{Right: core.EnvSafeArea, Bottom: core.EnvSafeArea, Left: core.EnvSafeArea}
				}
				return st
			}, Children: []core.Node{
				base.Tabs{Label: label, LabelFunc: s.LabelFunc, TestID: "tab-bar",
					StyleFunc: func() core.Style {
						if resolvedEdge() == mobile.TabBarBottom {
							return core.Style{Direction: core.Row, FlexGrow: 1, Gap: 4}
						}
						return core.Style{Direction: core.Column, FlexGrow: 1, JustifyContent: core.JustifyCenter, Gap: 4}
					}, Children: tabs},
			}}
	}
	return core.Show{When: useNative, Then: nativeBar, Else: drawnBar}
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
