---
title: 无样式组件
description: NSGo 自己的无外观控件，可以逐个部件设置样式，或在其上构建带样式的组件库。
group: 编写组件
order: 73
---

`base` 包含 `ui` 中的每个控件，带有行为、状态和无障碍，但没有外观。每个部件都接受 `Class`、`Style` 和 `StyleFunc`（[部件与状态](parts.md)），所以有自己设计的组件库或应用可以用它们代替 `ui`，或者像 `ui` 那样包装它们（[外观与主题](looks.md)）。本页覆盖[组件参考](../ui.md)中的控件；gallery 在每个控件的 `ui` 形式旁边展示它。

控件（如 `Button`、`Checkbox`、`Select`、`Slider` 和 `NumberInput`）接受 `Invalid: rejected.Get`，跟随校验信号。用 `Class: "aria-invalid:border-red-500"` 或 `Style.AriaInvalid` 设置该状态的样式。它更新无障碍状态和样式，不会禁用控件。复合字段会把它传给实际输入框和装饰部件；`ColorPicker` 会把它共享给内部控件。

## Button

`base.Button` 的外观来自 `Style`，内容来自 `Children`。`OnPress` 在完整的按压后运行；`OnPressDown` 和 `OnPressUp` 跟随按压的开始和结束，用于你自己的反馈。

```go
base.Button{
	Label:   "保存",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "保存", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`、`Busy`、`Selected` 和 `Expanded` 把状态报告给应用、样式和辅助技术：仍在进行的操作用 `Busy`（以 `aria-busy:` 设置样式），开关按钮或列表的当前项用 `Selected`，展开控件用 `Expanded`。行或列表以其他方式到达按钮时，`NoFocus` 把它移出 Tab 停靠点。

## Checkbox

`base.Checkbox` 接受框的样式、指示器的样式，以及指示器里你自己的标记，标记根据应用保存的值显示。

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "记住我",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "记住我"}},
}
```

指示器通过 `PartStates` 从框取得状态，所以 `checked:` class 可以给它设置样式，而它不会成为第二个停靠点。

需要修饰键时，设置 `OnChangeEvent func(checked bool, press core.PressEvent)`。它替代 `OnChange`，值仍由应用保存；列表可以用 `press.Modifiers.Shift` 选择范围。桌面和 Web 的空格/回车激活保留修饰键，应用快捷键会先处理按键；在 Windows 上，Alt 加空格或回车交给窗口菜单。在 TSX 中，`onChangeEvent` 接收 `{ checked, press }`。

## Switch

`base.Switch` 由轨道、轨道中的滑块，以及作为标签的子元素组成。按压报告它请求的值，即 `Checked` 的反面。

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "深色外观",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

轨道和滑块有各自的 `StyleFunc`，它们的状态样式跟随 `Checked`。

## Slider

`base.Slider` 接受 `base.SliderValue`：`base.SingleValue(v)` 或 `base.RangeValue(low, high)`。轨道、指示器、范围和旋钮分别设置样式。

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "等级",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`、`Max` 和 `Step` 是响应式的 getter：改变它们的信号会更新已挂载的滑块及其无障碍范围，拖动中也一样。`Axis` 让它垂直，`Inverted` 把最小值放到另一端，`base.Progress` 的这两个字段也一样；`Marks` 在指示器上放置你自己的刻度，`OnChangeEnd` 在拖动停止时报告一次值。

## Select

`base.Select` 接受 `base.SelectItem` 和 `*string` 值，所以“未选择”是 `nil`。触发器、弹出层、行和分组的样式由你设置。

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "小"}, {Value: "m", Title: "中"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "尺寸",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

相邻的同 `Group` 项共用一个表头，typeahead 按项的标题匹配，`SearchField` 绘制你自己的搜索框，`Cleanable` 显示清除按钮。`OnCancel` 收到 Escape、返回和点击外部。

## Tabs

`base.Tabs` 包含 `base.Tab`，由 `IndicatorStyle` 绘制指示器；应用保存选中的标签。`base.Tab.OnPress` 在每次按压时运行，包括已选中的标签。

```go
base.Tabs{Label: "分区", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "概览", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "概览"}}},
	base.Tab{Label: "动态", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "动态"}}},
}}
```

## Dialog

`base.Dialog` 接受背景、图层和弹出层的样式、一个 `Animation` 或 `TransitionShader`，以及你的子元素。`Open`、`DefaultOpen` 和 `OnOpenChange` 控制它；`OnOpen`、`OnClose` 和 `OnCancel` 报告发生了什么。`OnConfirm` 是默认操作：Return 执行它，然后以 `ReasonSelect` 关闭对话框；键盘焦点在自己接收 Return 的按钮或输入框上，或上方另有模态层时除外。`Role` 和 `AccessibilityDescription` 决定屏幕阅读器读出的内容，确认框用 `RoleAlertDialog`。`NoCloseOnSwipe` 让 iOS 的滑动不关闭它，`Presentation` 选择 iOS 的呈现方式，`StatusBar` 在打开期间改变状态栏。

`base.Sheet` 是任意一侧的面板，带背景和你的内容；设置 `CloseOnSwipe` 后，底部 sheet 跟随向下拖动，超过阈值后关闭。它和 `base.Dialog` 一样接受 `OnConfirm`、`Role` 和 `AccessibilityDescription`。`base.ShowAlertDialog` 显示系统的警告框或操作表，按钮按角色区分：

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "删除这条笔记？",
	Actions: []core.AlertAction{
		{Label: "取消", Role: core.AlertCancel},
		{Label: "删除", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "删除" {
		remove()
	}
})
```

## Menu

`base.Menu` 从你自己的触发器打开 `base.MenuItem`，`base.ContextMenu` 在指针处打开。条目报告 `OnSelect`，除非设置 `NoCloseOnSelect`，否则关闭菜单；`Checked` 让它成为可勾选条目，`Indicator` 在内容前绘制你的标记，`Role` 选择辅助技术听到的内容：菜单项、复选框或单选项。`base.Submenu` 包含更多条目。

```go
base.Menu{Trigger: core.Text{Content: "操作"}, Children: []core.Node{
	base.MenuItem{Label: "显示侧边栏", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "分享", Children: []core.Node{
		base.MenuItem{Label: "复制链接", OnSelect: copyLink},
		base.MenuItem{Label: "邮件", OnSelect: emailLink},
	}},
}}
```

桌面上由 `Placement`、`Align`、`Offset` 和 `Margin` 定位弹出层，`PopupStyle` 和 `BackdropStyle` 设置它的样式。

## EmojiPicker

`base.EmojiPicker` 按应用语言（或显式传入的 `Languages` / `LanguagesFunc`）的本地化 CLDR 名称、关键词和短代码搜索所有 Unicode 表情，提供分类标签、可选的 `Recent` 最近使用、肤色选择、键盘导航和虚拟化网格。所有默认文案和分类名都来自 `base/locales`，表情数据仅在使用 `EmojiPicker` 或其查询函数（`base.SearchEmojis`、`base.EmojisInCategory`、`base.EmojiByString`）时才会被链接。

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` 和 `DefaultCategory` 选择浏览分类。分类标签与肤色选项各自只有一个 Tab 停靠点，可通过方向键切换。`EmojiPickerRef` 提供 `Focus`、`Query`、`SetQuery`、`SetCategory` 和 `MatchedCount`，带样式版本使用同一份查询状态。`LabelFunc`、`SearchLabelFunc` 和 `PlaceholderFunc` 可原位更新文案。`Emoji.WithSkinTone` 会将所选肤色同时应用到双人表情中的两个人，默认肤色恢复为中性形式；直接按表情搜索时保留其完整序列。

如需带主题外观的版本（包含 Lucide 分类图标、可清除搜索框、高亮表情预览与肤色色块），请使用 `ui.EmojiPicker`。
