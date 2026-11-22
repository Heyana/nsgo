---
title: Navigation
description: Native navigation stacks, system headers, modals, menus and tab bars.
group: Core concepts
order: 56
---

## A stack

`base.NewNav` keeps a stack of routes of one type: `Push` adds a page, `Pop` removes the top page, `PopToRoot` returns to the first and `Replace` swaps the top. Its `Stack` renders every route as a page, keeping the pages below mounted with their state while the top one shows. `ui.NavPage` gives each page `ui`'s background and a `ui.NavHeader` with its title, back button and buttons.

watchOS has no system navigation bar, tab bar or switch, so there `ui.NavPage` draws its `ui.NavHeader` above the page, back button included, `ui.TabShell` draws its own tab bar and `ui.SwitchSystem` is `ui`'s own switch.

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

On iOS and Android the platform draws the system's header from the page's title and the header's buttons, with its back button and its transition; on macOS, Windows, Linux and the web `ui` draws it.

Android uses a Material 3 top app bar. A regular bar is 64 dp with a 22 sp title. `HeaderLargeTitle` gives a 152 dp large bar with a 28 sp title that collapses into the regular bar as the scroll view at the top of the page scrolls its content under it, and expands as the content returns to the top.

Set `HeaderTitleStyle` and `HeaderLargeTitleStyle` to style the native compact and expanded title fonts on iOS and Android: `FontSize`, `FontWeight`, `FontFamily` and `FontStyle`. Unset fields retain each platform's defaults; `HeaderTitleStyleFunc` and `HeaderLargeTitleStyleFunc` follow changes. `HeaderStyle` still supplies the background, title color and button tint. In TSX these are `headerTitleStyle` and `headerLargeTitleStyle` style objects or arrays.

A custom `HeaderTitle` replaces only the compact title on Android; `Title` remains the expanded large title. Set `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` to put `HeaderRight` beside that large title. Its controls move into the pinned toolbar as the title collapses and remain pressable throughout. TSX uses `android={{ headerRightWithLargeTitle: true }}`. The large band grows when its font needs more room. Standalone Android builds enable native RTL layout; an embedded host must set `android:supportsRtl="true"` on its application so native headers follow the app's direction.

Give a page its own `Header` to replace `ui.NavHeader`. On iOS and Android it becomes the system header's title view, in place of `HeaderTitle`, beside the page's `HeaderLeft`, `HeaderRight` and the back button; it fills the room between those ends when its views grow or take a percentage width, such as a search field, and takes its content's size otherwise. Elsewhere it is drawn above the content. `ui.NavHeader.Scroll` still connects an iOS or Android page's scrolling to its header's blur.

`NoPopOnBack` keeps a page when back is pressed, `OnBackPress` hears the press and `NoPopOnSwipe` turns off iOS's swipe back. `cx.OnBackPress(func() bool)` gives the app a handler before the stack, for unsaved changes. `OnDismiss` reports that the user popped the page, and the stack removes it.

## Immersive full screen

Call `mobile.Immersive(cx, true)` to request full screen for the window and `mobile.Immersive(cx, false)` to return to its regular system-bar settings. For a page or dialog, use `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` to request it while that layer is shown. In TSX, use `statusBar={{ immersive: true }}` on a page or dialog, or `status_bar: { immersive: true }` in `nsgo.yaml` for the window default.

On Android an edge swipe briefly shows the status and navigation bars. On HarmonyOS the status bar and the navigation indicator hide, and an edge swipe briefly shows them. On iOS the status bar hides and UIKit decides when to hide or reveal the home indicator. `cx.Env().SafeArea()` continues to report the insets the system provides, including display cutouts.

## Modals

`ui.Dialog` shows a surface over a dimmed app, a sheet from the bottom on touch screens, and `ui.Sheet` a panel from the bottom. Each closes on Escape and a press on its backdrop unless told not to.

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` asks the user to confirm, and `cx.ShowAlert` shows the system's own alert or action sheet ([Dialog](ui/dialog.md)).

## Menus

`ui.Menu` opens `ui.MenuItem`s from a button: beside it, or as a sheet from the bottom when its `Presentation` asks, on any device. `ui.ContextMenu` does the same for a secondary click or a long press, and `ui.Popover`, `ui.HoverCard` and `ui.Tooltip` show other anchored content.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Tabs

`ui.Tabs[T]` is a segmented control or a tab list, with an indicator that slides to the chosen option; `TabStyle` merges over each tab's style, its `Selected` over the chosen tab's, and `IndicatorStyle` over the indicator's. Underline tabs can set `IndicatorColor` and fit the line to their content with `IndicatorFit: TabsIndicatorContent`. `ui.Sidebar` holds sections and rows for a desktop sidebar, and `ui.TabShell` keeps top-level sections beside a tab bar (bottom by default, or vertical on the leading or trailing edge with `Edge` / `EdgeFunc`, including `mobile.TabBarAutomatic` to follow `cx.Env().VerticalBarEdge()`) at every window width. It uses the system's bar on iOS when every tab has a `SystemIcon`, and on Android on the bottom edge when every tab has a `SystemIcon` or an SVG `Icon`, showing the SVG wherever `SystemIcon` names no Android drawable, such as an SF Symbol's name; otherwise it draws the bar with the SVG icons. Set `BarHidden` while a pushed detail page is visible to hide either bar and restore the section's safe area; `TabBarInset` becomes 0 while it is hidden or placed vertically.

`mobile.TabBar` is the system's tab bar on iOS and Android, with its Liquid Glass or Material look; it takes `Items` of `mobile.TabBarItem{Title, Icon, SVG}`, the selected index, `OnChange`, and an optional `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing`, or `mobile.TabBarAutomatic`, which follows `cx.Env().VerticalBarEdge()` on iOS 27.1 and falls back to the bottom edge elsewhere) or reactive `EdgeFunc`. Android's palette roles `tabBarBackground`, `tabBarIdle`, `tabBarSelected` and `tabBarIndicator` set the bar's background, idle icons and labels, selected icon and label, and selected pill. Custom palettes fall back to `Surface`, `TextMuted`, `Accent` and `accentContainer` respectively, with a 12% accent pill when `accentContainer` is unset. The default palette keeps Material's background and idle colors, with the selected tab tinted by the accent. Elsewhere it takes no room, so an app shows `ui.Tabs` there. `mobile.IsAvailable(cx)` says whether the bar shows. `Icon` names a platform icon; on Android, `SVG`, such as `lucide.House()`, shows wherever `Icon` names no drawable, tinted by the bar as a drawable is.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Next

- [Forms](forms.md): what a page of a stack holds.
- [Animation](animation.md): how pages and modals move.
- [Layout](layout.md): the safe area around a header.
