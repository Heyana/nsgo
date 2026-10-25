package core

import "github.com/oneo-me/nsgo/internal/driver"

// TabBar is the system's tab bar for an app's top-level sections, as
// NSH's nsh/mobile TabBar: UITabBar on iOS (Liquid Glass from iOS 26), a
// Material 3 navigation bar on Android, each in the system's look. The app
// gives the tabs, says which is selected, hears the user's choice with
// OnChange and shows the selected tab's content; it places the bar with
// its style, whose height is the system's where the style leaves it open.
// Desktops and the web have no system tab bar: there it takes no room and
// shows nothing, and the app shows its tabs with base.Tabs.
type TabBar struct {
	Items []TabBarItem
	// ItemsFunc replaces Items, again when what it reads changes.
	ItemsFunc func() []TabBarItem
	// Selected is the selected tab's index; one past the tabs selects
	// none. The bar shows the user's choice at once, as the system's does.
	Selected func() int
	OnChange func(index int)
	Style    Style
	TestID   string
}

// TabBarItem is one tab of a TabBar.
type TabBarItem struct {
	Title string
	// Icon is the platform's name for an icon: an SF Symbol on iOS
	// ("house"), and on Android a drawable of the app or, prefixed
	// "android:", of the system ("android:ic_menu_search").
	Icon string
}

// Render implements Node.
func (b TabBar) Render(cx *Scope) Node {
	mobile := func() bool { os := cx.Env().OS(); return os == OSIOS || os == OSAndroid }
	return Show{When: mobile, Then: b.native,
		Else: func(*Scope) Node { return View{Style: Style{Display: DisplayNone}, TestID: b.TestID} }}
}

// native is the bar where the platform has one.
func (b TabBar) native(*Scope) Node {
	items := func() []TabBarItem {
		if b.ItemsFunc != nil {
			return b.ItemsFunc()
		}
		return b.Items
	}
	return Native{Component: driver.TabBarComponent, Style: b.Style, TestID: b.TestID,
		PropsFunc: func() map[string]any {
			list := []any{}
			for _, it := range items() {
				tab := map[string]any{"title": it.Title}
				if it.Icon != "" {
					tab["icon"] = it.Icon
				}
				list = append(list, tab)
			}
			selected := 0
			if b.Selected != nil {
				selected = b.Selected()
			}
			return map[string]any{"items": list, "selectedIndex": float64(selected)}
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
