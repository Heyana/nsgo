---
title: 对话框
description: 模态表面、sheet 和警告框。
group: 组件
order: 41
---

`ui.Dialog` 是覆盖在变暗的应用上的模态表面，在触摸屏上是从底部弹出的 sheet。`ui.Sheet` 是从底部弹出的面板，`ui.AlertDialog` 请用户确认。

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "重命名文件",
	Description:  "新名称会显示在所有地方。",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "取消", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "保存", OnPress: save},
	},
}
```

## 打开与关闭

对话框是否打开由应用保存。`OnOpenChange` 收到对话框请求的每次变化，附带一个 `base.Reason`，用来区分点击外部、Escape 和返回；`OnClose` 在关闭完成后运行。`NoCloseOnEscape` 和 `NoCloseOnBackdropPress` 让 Escape 和点击外部不关闭它。

`Title` 和 `Description` 位于表面顶部，打开期间会变化时用 `TitleFunc` 和 `DescriptionFunc`，`Footer` 放按钮。`Bare` 把整个表面交给 `Children`，用于应用自己布局的对话框，桌面上内容按 `Style` 和 `StyleFunc` 给出的圆角裁剪在表面边框以内，`Inset` 设置桌面对话框到窗口边缘的最小距离。

在触摸屏的 sheet 上，`Bare` 会移除默认拖拽条和内容内边距，让 `Children` 填满面板。`NoGrabber` 只隐藏拖拽条，`ContentStyle` 和 `ContentStyleFunc` 自定义内容容器，`TitleStyle` 和 `TitleStyleFunc` 自定义标题。`Leading` 和 `Trailing` 把节点（例如“取消”和“完成”按钮）放在标题行两端，标题居中于它们之间。`StartingStyle` 和 `EndingStyle` 自定义 sheet 的进出动画；需要自定义时长时，在其中设置 `Transition`。

`Style` 合并到表面的默认外观上，`StyleFunc` 在它之后合并，并跟踪读取的信号，包括外观设置，在打开期间和重新打开时都会更新。例如，先声明 `colors := ui.ColorsOf(cx)`，再用 `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` 跟随主题。这两个字段也适用于 `ui.AlertDialog`。零值颜色保留已有颜色；要清除颜色，请使用 `core.RGBA(0xFFFFFF, 0)` 这样的非零透明色。

## Sheet

`ui.Sheet` 是覆盖在变暗背景上的底部面板；点击背景、Escape 或返回会调用 `OnClose`。

用 `Label` 或 `LabelFunc` 设置对话框的无障碍名称，用 `TestID` 在测试中查找面板。`LabelFunc` 跟随 signal 更新，保留已经打开的内容。设置 `CloseOnSwipe: true` 后，向下滑动超过关闭阈值也会调用 `OnClose`；在该回调中更新 `Open` 来关闭面板。

设置 `Style` 或 `StyleFunc` 可以自定义表面的背景、圆角和内边距，同时保留拖拽条和安全区域处理。`StyleFunc` 会跟随它读取的 signal 更新。只设置 `CornerRadius` 而不设置 `CornerRadii` 时，四个角都会变圆，取代默认只圆顶部两角的样式。

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## 警告框

`ui.AlertDialog` 请用户确认或知悉某件事。每个操作都会关闭它，点击外部则不会。

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "删除这条笔记？",
	Description:  "此操作无法撤销。",
	Actions: []ui.AlertAction{
		{Text: "取消", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "删除", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

`Role: core.AlertCancel` 标记的操作会在 Escape 或返回时执行，随后调用 `OnOpenChange(false, base.ReasonCancel)`。`Default: true` 让操作在桌面、Web 和 Apple TV 上打开时获得键盘焦点，并让 Return 选择它；点击操作会先执行 `OnPress`，再调用 `OnOpenChange(false, base.ReasonSelect)`。键盘焦点不在其他操作或输入框上时，Return 执行默认操作：焦点移到另一个操作上时，Return 按下那个操作；警告框上方另有对话框时，Return 归那个对话框。执行过一个操作后，无论 `OnOpenChange` 怎么处理，警告框都会关闭，直到 `Open` 先变为 false 再变为 true 之前不再执行其他操作。同一标记出现多次时，以第一个为准。未设置标记时，Escape 直接关闭，不执行操作，Return 没有默认操作。`Role: core.AlertDestructive` 描述操作含义，外观仍由 `Variant` 或 `Color` 决定。

要显示系统自己的警告框或操作表，用 `cx.ShowAlert`（[平台服务](../services.md#警告框与剪贴板)）。

## 行为

模态框会阻挡后面的应用，获得键盘焦点，除非另有设置，否则按 Escape 或返回会关闭。在触摸屏上，对话框是一个 sheet，跟随向下拖动，超过阈值后关闭。

## 无障碍

`ui.Dialog` 报告 `RoleDialog` 或显式设置的 `Role`，`ui.AlertDialog` 在桌面和触摸屏上都报告 `RoleAlertDialog`。它们读出标题，标题不够时读出 `Label`，随后读出描述。打开时焦点移入，关闭时焦点返回；背景不是停靠点。底部按钮保留各自的角色和标签。

## 平台

在 Android 和触摸屏上，对话框是从底部弹出的 sheet；在桌面上是带背景的居中表面。内容相同，只是呈现方式不同。

## 相关

- [导航](../navigation.md)：页面、标签页和模态一起使用。
- [菜单](menu.md)：锚定命令和上下文菜单。
- [无障碍](../accessibility.md)：焦点和对话框角色。
- gallery 的 Dialog、Sheet 和 AlertDialog 页面。
- [无样式组件](../authoring/base.md#dialog)：`base.Dialog` 和 `base.Sheet`，带 iOS 的呈现方式和自定义动画。
