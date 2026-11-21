---
title: 标签页
description: 标签列表、分段控件和系统标签栏。
group: 组件
order: 40
---

`ui.Tabs[T]` 以分段控件或标签列表的形式从几个值中选一个，应用在旁边显示所选值的内容。标签列表是一个 Tab 停靠点：方向键在其中移动。

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "概览"},
		{Value: "activity", Label: "动态"},
		{Value: "settings", Label: "设置"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "分区",
}
```

## 变体与尺寸

`Variant` 是 `TabsSegmented`（默认，凸起的指示块在凹陷轨道中）或 `TabsUnderline`（选中标签下方的强调线）。下划线标签可以设置 `IndicatorColor`，并用 `IndicatorFit: TabsIndicatorContent` 让指示器跟随标签文字和图标，而不是整个标签；TSX 中对应 `indicatorColor` 和 `indicatorFit="content"`。`Size` 是 `SizeDefault`、`SizeSmall` 或 `SizeLarge`，`Equal` 让每个标签等宽，而不是按标签文字适应。

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

选项可以带 `Icon`、`Disabled` 函数、`Content` 节点和 `TestID`。`Content` 替代标签文字，并继承标签的文本样式。

标签跟随 signal 时使用 `OptionsFunc`。NSGo 按选项值原地更新标签；只要该值仍在列表中，就会保留选中状态和键盘焦点。值被移除后保持未选中，直到应用更新它。

## 标签栏

`ui.TabShell` 将顶层分区放在底部标签栏上方，窗口宽度不会改变这一形式。iOS 上每个标签都提供 `SystemIcon` 时使用系统标签栏；Android 上每个标签提供 `SystemIcon` 或 SVG `Icon` 即可，此时能对应 Android drawable 的 `SystemIcon` 优先，其他名称（例如 SF Symbol 名称）显示 SVG。其他情况使用所提供的 SVG 绘制标签栏。它自己绘制的底部栏会避开底部安全区域，因此其上方分区（包括 `base.Nav` 的页面）底部的 `core.EnvSafeArea` 为 0；在 iOS 浮动的系统标签栏下，用 `ui.TabBarInset` 让内容避开它。

使用 `ShellTab.TextFunc` 让标签页标题随语言或应用状态更新，并用 `TabShell.LabelFunc` 更新标签栏的无障碍名称。在图标保持原样时，更新标题会保留当前选中的分区和现有标签。

`mobile.TabBar` 是 iOS 和 Android 的系统标签栏，接收 `Items`、`Selected` 和 `OnChange`。每个 `mobile.TabBarItem` 带 `Title` 和平台图标名 `Icon`；在 Android 上，`Icon` 找不到对应 drawable 时显示 `SVG: lucide.House()`，适配到 24 点并按选中状态着色。iOS 只使用 `Icon` 中的 SF Symbol。TSX 的 `svg` 接收 SVG 文档。其他平台上 `mobile.TabBar` 不占空间，因此在那里显示 `ui.Tabs`。

`ui.SidebarShell` 自己让侧栏、页脚、标题栏、内容和右侧面板中的内容避开左右两侧的安全区域，背景仍铺到窗口边缘，因此其中左右两侧的 `core.EnvSafeArea` 为 0；`ui.NavHeader` 会在内边距上加上两侧的安全区域。`ui.NavPage` 的内容和 `TabShell` 分区的内容用 `PaddingEnv` 自行避开安全区域，这样需要铺满到边缘的内容（例如全出血图片）仍然可以铺满。

## 无障碍

列表报告 `RoleTabList`，每个标签报告 `RoleTab`，选中的标签报告自己的状态。列表是一个 Tab 停靠点；左/上、右/下、Home 和 End 在启用的标签间移动，到达的标签会报告按压。`Label` 命名列表。

## 平台

iOS 和 Android 显示系统标签栏，带各自的外观和手势；macOS、Windows、Linux 和 Web 显示 `ui.Tabs`。应用的内容在每个目标上相同。

## 相关

- [导航](../navigation.md)：应用中的页面和标签栏。
- [菜单](menu.md)：命令而不是分区。
- [选择器](select.md)：在列表中选择多项。
- gallery 的 Tabs 和 NavStack 页面。
- [无样式组件](../authoring/base.md#tabs)：`base.Tabs`，用于自己设计外观的标签页。
