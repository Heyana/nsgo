---
title: 导航
description: 原生导航栈、系统标题栏、模态、菜单和标签栏。
group: 核心概念
order: 56
---

## 导航栈

`base.NewNav` 保存一种路由类型的栈：`Push` 添加页面，`Pop` 移除顶部页面，`PopToRoot` 回到第一页，`Replace` 替换顶部页面。它的 `Stack` 把每个路由渲染成一页，顶部页面显示时，下面的页面保持挂载和状态。`ui.NavPage` 给每页加上 `ui` 的背景和一个 `ui.NavHeader`，包含标题、返回按钮和其他按钮。

watchOS 没有系统的导航栏、标签栏和开关，所以在那里 `ui.NavPage` 会在页面上方绘制它的 `ui.NavHeader`（包括返回按钮），`ui.TabShell` 绘制自己的标签栏，`ui.SwitchSystem` 使用 `ui` 自己的开关。

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "收件箱", Children: []core.Node{
			ui.Button{Text: "打开一条消息", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "消息"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "收件箱",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "归档", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

在 iOS 和 Android 上，平台根据页面标题和标题栏的按钮绘制系统标题栏，带返回按钮和转场；在 macOS、Windows、Linux 和 Web 上由 `ui` 绘制。

Android 使用 Material 3 顶部应用栏。普通应用栏高 64 dp，标题为 22 sp。`HeaderLargeTitle` 会给出高 152 dp、标题为 28 sp 的大标题栏，页面顶部的滚动视图把内容滚到它下面时收缩成普通应用栏，内容回到顶部时展开。

给页面设置自己的 `Header` 即可取代 `ui.NavHeader`。在 iOS 和 Android 上，它成为系统标题栏的标题视图，取代 `HeaderTitle`，旁边保留页面的 `HeaderLeft`、`HeaderRight` 和返回按钮；当其中的搜索框等视图伸展或使用百分比宽度时，它会填满两端之间的空间，否则按内容大小显示。在其他平台上，它绘制在内容上方。`ui.NavHeader.Scroll` 仍然把 iOS 和 Android 页面的滚动连接到标题栏的模糊效果。

`NoPopOnBack` 在按下返回时保留页面，`OnBackPress` 听到返回，`NoPopOnSwipe` 关闭 iOS 的边缘滑动返回。`cx.OnBackPress(func() bool)` 在栈之前给应用一个处理器，用于未保存的更改。`OnDismiss` 报告用户已经弹出页面，栈会把页面移除。

## 沉浸式全屏

调用 `mobile.Immersive(cx, true)` 为窗口请求全屏，调用 `mobile.Immersive(cx, false)` 恢复窗口原有的系统栏设置。页面或对话框可设置 `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}`，在显示期间请求全屏。TSX 页面或对话框使用 `statusBar={{ immersive: true }}`，窗口的默认值可在 `nsgo.yaml` 中设置为 `status_bar: { immersive: true }`。

在 Android 上，从屏幕边缘滑动可短暂显示状态栏和导航栏。在 iOS 上，状态栏隐藏，底部主屏幕指示条的隐藏和显示由 UIKit 决定。`cx.Env().SafeArea()` 继续报告系统提供的边距，包括屏幕开孔占用的区域。

## 模态

`ui.Dialog` 在变暗的应用上显示一个表面，在触摸屏上从底部出现；`ui.Sheet` 从底部显示面板。除非另有设置，Escape 和点击背景都会关闭它们。

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "重命名",
	Description:  "新名称会显示在所有地方。",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "取消", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "保存", OnPress: save},
	},
}
```

`ui.AlertDialog` 请用户确认，`cx.ShowAlert` 显示系统自己的警告框或操作表（[对话框](ui/dialog.md)）。

## 菜单

`ui.Menu` 从一个按钮打开 `ui.MenuItem`：在按钮旁，或在 `Presentation` 要求时作为底部 sheet，在任何设备上都一样。`ui.ContextMenu` 用右键或长按做同样的事，`ui.Popover`、`ui.HoverCard` 和 `ui.Tooltip` 显示其他锚定内容。

```go
ui.Menu{Trigger: ui.Button{Text: "操作", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "重命名", OnSelect: rename},
	ui.MenuItem{Text: "复制一份", OnSelect: duplicate},
	ui.MenuItem{Text: "删除", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## 标签页

`ui.Tabs[T]` 是分段控件或标签列表，指示器会滑到选中的选项；`TabStyle` 合并到每个标签的样式上，其中的 `Selected` 合并到选中标签的样式上，`IndicatorStyle` 合并到指示器的样式上。下划线标签可以设置 `IndicatorColor`，并用 `IndicatorFit: TabsIndicatorContent` 让线条适配标签内容。`ui.Sidebar` 为桌面侧边栏保存分组和行，`ui.TabShell` 在所有窗口宽度下都将顶层分区放在标签栏旁（默认在底部，可通过 `Edge` / `EdgeFunc` 设为前侧或后侧纵向排列，或用 `mobile.TabBarAutomatic` 跟随 `cx.Env().VerticalBarEdge()`）。iOS 上每个标签都有 `SystemIcon` 时使用系统标签栏；Android 在底部时每个标签有 `SystemIcon` 或 SVG `Icon` 即可，`SystemIcon` 不是 Android drawable 名称（例如 SF Symbol 名称）时显示 SVG；其他情况由 shell 用 SVG 图标绘制标签栏。推入详情页时可让 `BarHidden` 返回 true，隐藏系统或绘制的标签栏并恢复分区的安全区域；隐藏期间或标签栏纵向排列时 `TabBarInset` 为 0。

`mobile.TabBar` 是 iOS 和 Android 的系统标签栏，带 Liquid Glass 或 Material 外观；它接收 `mobile.TabBarItem{Title, Icon, SVG}` 组成的 `Items`、选中索引、`OnChange`，以及可选的 `Edge`（`mobile.TabBarBottom`、`mobile.TabBarLeading`、`mobile.TabBarTrailing` 或在 iOS 27.1 上跟随 `cx.Env().VerticalBarEdge()` 并在其他平台回退到底部的 `mobile.TabBarAutomatic`）或响应式 `EdgeFunc`。Android 的 `tabBarBackground`、`tabBarIdle`、`tabBarSelected` 和 `tabBarIndicator` 分别设置标签栏背景、未选中项的图标和文字、选中项的图标和文字，以及选中项的胶囊背景。自定义调色板分别回退到 `Surface`、`TextMuted`、`Accent` 和 `accentContainer`，未设 `accentContainer` 时使用 12% 不透明度的强调色。默认调色板保留 Material 的背景和未选中项颜色，选中项使用强调色。其他平台上它不占空间，因此应用在那里显示 `ui.Tabs`。`mobile.IsAvailable(cx)` 说明标签栏是否显示。`Icon` 是平台图标名；在 Android 上，`Icon` 找不到对应 drawable 时显示 `SVG`（例如 `lucide.House()`），并像 drawable 一样由标签栏着色。

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "首页", Icon: "house"}, {Title: "搜索", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## 下一步

- [表单](forms.md)：栈中的页面放什么。
- [动画](animation.md)：页面和模态如何移动。
- [布局](layout.md)：标题栏周围的安全区域。
