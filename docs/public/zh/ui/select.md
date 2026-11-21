---
title: 选择器
description: 从列表中选一个值，以弹出列表或 sheet 显示。
group: 组件
order: 36
---

`ui.Select[T]` 从一组任意可比较类型的值中选一个。列表在任何设备上都在触发器下方打开；`Presentation: base.PresentSheet` 改为从底部打开 sheet，手机应用可以这样选择。

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "小"}, {Value: "m", Label: "中"}, {Value: "l", Label: "大"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "尺寸",
}
```

## 选项与搜索

`Options` 由 `ui.Option[T]` 组成，每项有 `Value`、`Label`，以及可选的 `Group`、`Icon`、`Disabled` 函数或应用自己的 `Content`。在 `Select` 或 `Combobox` 中，相邻且 `Group` 相同（非空）的选项显示在该分组的标题下，方向键和输入跳转会越过标题，屏幕阅读器把它读作标题。选项跟随 signal 时使用 `OptionsFunc`，打开的列表会原地更新并保留字段的值和焦点。未选择时显示 `Placeholder`。`Searchable` 允许输入过滤列表，`OnClear` 显示清除按钮，`Disabled` 停用它。

```go preview=select/searchable height=400
ui.Select[string]{
	Options:           languages,
	Value:             language.Get,
	OnChange:          language.Set,
	Searchable:        true,
	SearchPlaceholder: "搜索语言",
	OnClear:           func() { language.Set("") },
	Placeholder:       "选择语言",
}
```

`Style` 决定下拉框的布局，未设置宽度时为 200 点宽；其中的高度、内边距和字体作用于字段本身，与 [Input](input.md) 相同，因此下拉框可以与 `SizeSmall` 的按钮对齐：

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## 行为

焦点在触发器上时，上、下、Return 和空格打开列表；方向键移动高亮，typeahead 按项标题跳转。选择一项会关闭列表并报告值。Escape、返回和点击外部会关闭列表而不改变值。`Empty` 是搜索无结果时显示的文字。

## 无障碍

`Label` 为辅助技术提供字段名称。使用 `LabelFunc` 时，名称随其读取的 signal 原地更新。名称为空时使用 `Placeholder`。触发器和列表带有选择器的角色，并报告选中项。Tab 到达触发器，Return 或空格打开它。

## 平台

在 macOS、Windows 和 Linux 上，列表是触发器旁的弹出层。在 iOS 和 Android 上它是 sheet，选中的行留在字段上。选项和值在每个目标上相同。

## 相关

- [表单](../forms.md)：表单中的选择器。
- [菜单](menu.md)：命令而不是值。
- [标签页](tabs.md)：同时显示的少量选项。
- gallery 的 Select 和 Combobox 页面。
- [无样式组件](../authoring/base.md#select)：`base.Select`，带分组和自定义触发器。
