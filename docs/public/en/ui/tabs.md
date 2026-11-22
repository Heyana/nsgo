---
title: Tabs
description: A tab list, a segmented control and the system tab bar.
group: Components
order: 40
---

`ui.Tabs[T]` chooses one of a few values, as a segmented control or a tab list; the app shows the chosen value's content beside it. A tab list is one Tab stop: the arrows move through the tabs.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## Variants and sizes

`Variant` is `TabsSegmented` (the default, a raised thumb in a recessed track) or `TabsUnderline` (an accent line under the selected tab). An underline can set `IndicatorColor` and use `IndicatorFit: TabsIndicatorContent` to follow its label and icon rather than the whole tab; in TSX they are `indicatorColor` and `indicatorFit="content"`. `Size` is `SizeDefault`, `SizeSmall` or `SizeLarge`, and `Equal` gives every tab the same width instead of fitting its label.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

An option can carry an `Icon`, a `Disabled` function, a `Content` node and a `TestID`. `Content` replaces the label and inherits the tab's text style.

Use `OptionsFunc` when the tabs follow a signal. NSGo updates rows in place by their values, keeping the selected tab and keyboard focus when that value remains offered; a value that disappears is left unselected until the app changes it.

## Tab bars

`ui.TabShell` keeps top-level sections beside a tab bar at every window width — on the bottom by default, or vertically along the leading or trailing edge when `Edge` (or `EdgeFunc`) is `mobile.TabBarLeading`, `mobile.TabBarTrailing`, or `mobile.TabBarAutomatic` (which follows `cx.Env().VerticalBarEdge()`). It uses the system's bar when every tab supplies `SystemIcon` or an SVG `Icon`, on any edge on iOS and on the bottom edge on Android; a `SystemIcon` that names an SF Symbol on iOS or a drawable on Android takes precedence, and the SVG shows otherwise. Otherwise it draws the bar with the supplied SVG icons. A bar that takes room of its own keeps clear of its edge's safe area itself, so `core.EnvSafeArea` on that edge of a section beside it, a `base.Nav` page's included, is 0; under iOS's floating bottom system bar, keep content clear with `ui.TabBarInset`.

Set `ShellTab.TextFunc` for tab titles that follow locale or application state and `TabShell.LabelFunc` for the bar's accessible name. Changing a title preserves the selected section and the existing tab as long as its icon stays the same.

`mobile.TabBar` is the system's tab bar on iOS and Android, with `Items`, `Selected`, `OnChange`, and optional `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing`, or `mobile.TabBarAutomatic`) / `EdgeFunc`. Each `mobile.TabBarItem` takes a `Title` and a platform icon name in `Icon`; `SVG: lucide.House()` shows wherever `Icon` names no SF Symbol on iOS or drawable on Android, fitted into 24 points and tinted for selection. In TSX, `svg` takes an SVG document and `edge` takes `"bottom"`, `"leading"`, `"trailing"`, or `"automatic"`. Elsewhere `mobile.TabBar` takes no room, so show `ui.Tabs` there.

`ui.SidebarShell` keeps what its sidebar, footer, headers, content and right panel hold clear of the left and right safe area itself, their backgrounds running to the window's edges, so `core.EnvSafeArea` on those sides is 0 inside them, and `ui.NavHeader` adds the side safe area to its padding. A `ui.NavPage`'s or a `TabShell` section's content keeps clear of the safe area itself, with `PaddingEnv`, so what should run to the edges, such as a full-bleed image, can.

## Accessibility

The list reports `RoleTabList`, each tab `RoleTab`, and the selected tab its state. The list is one Tab stop; Left and Up, Right and Down, Home and End move between enabled tabs, and the tab reached reports a press. `Label` names the list.

## Platform

iOS and Android show the system's tab bar with their own look and gestures; macOS, Windows, Linux and the web show `ui.Tabs`. The app's content is the same on every target.

## Related

- [Navigation](../navigation.md): pages and tab bars in an app.
- [Menu](menu.md): commands rather than sections.
- [Select](select.md): many options in a list.
- The gallery's Tabs and NavStack pages.
- [Unstyled components](../authoring/base.md#tabs): `base.Tabs`, for tabs of a look of your own.
