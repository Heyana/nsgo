package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// TabBarEdge is the edge along which a TabBar lays out its tabs. Apps name it mobile.TabBarEdge.
type TabBarEdge uint8

const (
	// TabBarBottom is the default horizontal bottom tab bar.
	TabBarBottom TabBarEdge = iota
	// TabBarLeading places the tab bar vertically along the leading edge (left in LTR, right in RTL).
	TabBarLeading
	// TabBarTrailing places the tab bar vertically along the trailing edge (right in LTR, left in RTL).
	TabBarTrailing
	// TabBarAutomatic follows the edge the system chose (Env.VerticalBarEdge), falling back to TabBarBottom when it is VerticalBarUnspecified.
	TabBarAutomatic
)

// String returns the edge's name: "bottom", "leading", "trailing" or "automatic".
func (e TabBarEdge) String() string {
	switch e {
	case TabBarLeading:
		return "leading"
	case TabBarTrailing:
		return "trailing"
	case TabBarAutomatic:
		return "automatic"
	}
	return "bottom"
}

// Resolve returns the concrete edge (TabBarBottom, TabBarLeading or TabBarTrailing) for env: TabBarAutomatic reads env.VerticalBarEdge when env is non-nil and falls back to TabBarBottom when unspecified.
func (e TabBarEdge) Resolve(env *Env) TabBarEdge {
	switch e {
	case TabBarLeading, TabBarTrailing:
		return e
	case TabBarAutomatic:
		if env != nil {
			switch env.VerticalBarEdge() {
			case VerticalBarLeading:
				return TabBarLeading
			case VerticalBarTrailing:
				return TabBarTrailing
			}
		}
	}
	return TabBarBottom
}

// TabBar is the core's part of mobile.TabBar, which apps use: the
// system's tab bar, a native component iOS and Android register. The
// selected tab takes the theme's accent. Android also reads palette
// roles for the bar, idle tabs, selected tab and indicator.
type TabBar struct {
	Items     []TabBarItem
	ItemsFunc func() []TabBarItem
	// Selected is the selected tab's index; one past the tabs selects
	// none. The bar shows the user's choice at once, as the system's does.
	Selected func() int
	OnChange func(index int)
	// Edge places the tab bar along the bottom (the default), the leading
	// or trailing edge (drawn vertically), or lets the system choose via
	// Env.VerticalBarEdge (TabBarAutomatic). EdgeFunc binds it reactively.
	Edge     TabBarEdge
	EdgeFunc func() TabBarEdge
	// Label names the bar as a group for assistive technology.
	Label     string
	LabelFunc func() string
	Style     Style
	TestID    string
}

// TabBadge is the mark at a tab's icon corner. Text takes precedence over Dot; the zero value clears the badge. Apps name it mobile.TabBadge.
type TabBadge = driver.TabBadge

// TabBarItem is one tab of a TabBar. Apps name it mobile.TabBarItem.
type TabBarItem struct {
	Title string
	// Icon is the platform's name for an icon: an SF Symbol on iOS
	// ("house"), and on Android a drawable of the app or, prefixed
	// "android:", of the system ("android:ic_menu_search").
	Icon string
	// SVG is the tab's icon on iOS and Android where Icon names no SF Symbol or drawable, fitted into 24 points and tinted by the bar as a system icon is.
	SVG   *SVG
	Badge TabBadge
}

// Render implements Node.
func (b TabBar) Render(cx *Scope) Node {
	mobile := func() bool { return cx.Env().mobileBars() }
	return Show{When: mobile, Then: b.native,
		Else: func(*Scope) Node { return View{Style: Style{Display: DisplayNone}, TestID: b.TestID} }}
}

// native is the bar where the platform has one.
func (b TabBar) native(cx *Scope) Node {
	items := func() []TabBarItem {
		if b.ItemsFunc != nil {
			return b.ItemsFunc()
		}
		return b.Items
	}
	return Native{Component: driver.TabBarComponent, Style: b.Style, TestID: b.TestID,
		PropsFunc: func() map[string]any {
			tabs := items()
			list := make([]any, 0, len(tabs))
			badges := make([]TabBadge, len(tabs))
			for i, it := range tabs {
				badges[i] = it.Badge
				tab := map[string]any{"title": it.Title}
				if it.Icon != "" {
					tab["icon"] = it.Icon
				}
				if it.SVG != nil {
					d := &driver.Drawing{}
					it.SVG.Draw(d, Size{Width: 24, Height: 24}, RGB(0xffffff))
					tab["drawing"] = d
				}
				list = append(list, tab)
			}
			selected := 0
			if b.Selected != nil {
				selected = b.Selected()
			}
			edge := b.Edge
			if b.EdgeFunc != nil {
				edge = b.EdgeFunc()
			}
			// Zero clears a custom color and restores the native fallback.
			p := cx.Palette()
			props := map[string]any{"items": list, "selectedIndex": float64(selected), "edge": edge.Resolve(cx.Env()).String(), "tint": float64(p.Accent),
				"background": float64(0), "inactive": float64(0), "indicator": float64(0), "activeContent": float64(0)}
			if b.Label != "" || b.LabelFunc != nil {
				props["label"] = readText(b.Label, b.LabelFunc)
			}
			theme := DefaultTheme()
			base := theme.Light
			if cx.Dark() {
				base = theme.Dark
			}
			container, hasContainer := p.Color("accentContainer")
			baseContainer, baseHasContainer := base.Color("accentContainer")
			if p.Surface != base.Surface || p.TextMuted != base.TextMuted || hasContainer != baseHasContainer || container != baseContainer {
				props["background"] = float64(p.Surface)
				props["inactive"] = float64(p.TextMuted)
				if !hasContainer || container == 0 {
					container = p.Accent.Faded(0.12)
				}
				props["indicator"] = float64(container)
				props["activeContent"] = float64(p.Accent)
			}
			for _, role := range [...]struct{ name, prop string }{
				{"tabBarBackground", "background"},
				{"tabBarIdle", "inactive"},
				{"tabBarSelected", "activeContent"},
				{"tabBarIndicator", "indicator"},
			} {
				if color, _ := p.Color(role.name); color != 0 {
					props[role.prop] = float64(color)
				}
			}
			props["badges"] = driver.TabBadges{Items: badges, Language: cx.Env().Language()}
			props["badgeColor"] = float64(p.Danger)
			return props
		},
		OnEvent: func(name string, data any) {
			if name != "change" || b.OnChange == nil {
				return
			}
			switch i := data.(type) {
			case float64:
				b.OnChange(int(i))
			case int:
				b.OnChange(i)
			}
		}}
}
