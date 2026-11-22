---
title: 表单
description: 文本框、焦点、软件键盘、开关、选择器和校验。
group: 核心概念
order: 57
---

## 文本框

`ui.Input` 是 `ui` 外观的平台文本框，字段与 `base.Input` 相同。设置 `Value` 后它是受控的：应用保存值并在 `OnChange` 中更新；设置 `DefaultValue` 后它自己保存值。

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "你的名字",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` 隐藏字符，`Readonly` 阻止编辑但文本仍可选中和复制，`Disabled` 停用字段，`Multiline` 让文本换行并随之增长，除非样式指定了尺寸。多行字段的各行及其占位文字都按样式的 `LineHeight` 排布，字体在每行中居中，与 `core.Text` 一致；单行字段的那一行在其高度内居中。`KeyboardType` 选择软件键盘（`core.KeyboardEmail`、`core.KeyboardNumber`、`core.KeyboardURL` 等），`AutoComplete` 告诉系统字段内容以便自动填充，`MaxLength` 限制用户输入的 UTF-16 单元数。`AutoCapitalize` 和 `NoAutoCorrect` 调整键盘行为。

`OnSelectionChange` 和 `OnCaretMove` 报告选区和光标位置，用于在光标处打开菜单。`Suggest`（一个 `core.Suggest`）跟随 `@` 这类触发字符之后的查询，字段旁的 `ui.SuggestList` 在光标处为它列出候选行，可用上下方向键、回车或点按选择，`chat.Composer` 的 `Suggest` 就这样补全提及。`OnKeyPress` 听到每个按键，`OnPaste` 听到粘贴内容，`OnCompositionChange` 跟随输入法组合。

在 iOS 和 Android 上，`cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` 会先于文本框的提交、失焦或换行处理软件键盘的 Return。`KeyAuto` 和 `KeyNotEditing` 将它留给 `SubmitBehavior`；iOS 输入法用来确认组词的 Return 仍由输入法处理。

## 焦点与键盘

`Ref.Focus()` 让文本框获得焦点，在桌面、Web 和 Apple TV 上也能聚焦启用的可按压或 `Focusable` 视图。`Ref.Blur()` 只清除自身视图的焦点，`cx.DismissKeyboard()` 关闭软件键盘。`OnFocus` 和 `OnBlur` 跟随焦点，`View.OnFocus`/`OnBlur` 对任何可聚焦视图做同样的事。获得焦点的视图离开树时（它所在的 `Show` 关闭或它所在的行被移除），会在移除它的那次提交之后收到一次 `OnBlur`（平台先报告下一个焦点时，则在那个视图的 `OnFocus` 之前），留下的外层视图收到 `OnFocusOut`。

元素位于没有任何位置显示的 `Holder` 内容中时，`Ref` 的聚焦、失焦、滚动和缩放请求不起作用；元素离开树时仍在排队的请求也会被丢弃。内容再次显示时不会重放这些请求，需要在内容放置后重新发出。

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

软件键盘会盖住手机底部。把 `core.EnvKeyboard` 用作底部内边距或外边距，内容就会停在键盘上方，并随键盘的移动一起动画：

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

表单外的滚动视图可以设置拖动时收起键盘，以及键盘显示时点击的处理方式：

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` 和 `cx.Env().KeyboardDuration()` 在代码中读取键盘的尺寸和时间，`cx.Env().SafeArea()` 读取屏幕四周的内边距。

## 选择

`ui.Switch` 是立即生效的设置，`ui.Checkbox` 有选中、未选中和不确定三种状态，`ui.Radio` 是 `ui.RadioGroup` 中的一个选项。`ui.Select[T]` 在触发器下方打开选项列表，`Presentation` 要求时作为 sheet 打开；`ui.Tabs[T]` 同时显示少量选项。

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "通知"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "小"}, {Value: "m", Label: "中"}, {Value: "l", Label: "大"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "尺寸",
}
```

`ui.NumberInput` 带边界地步进和拖动数字，`ui.DatePicker` 和 `ui.TimeField` 编辑日期和时间，`ui.ColorPicker` 编辑颜色，`ui.TagInput` 编辑一组标签。

`ui.OtpInput` 在格子中输入一次性验证码。`Size` 设置格子的默认宽度和高度，在桌面和电视上为 36 点，在触屏设备上为 44 点。`CellWidth` 和 `CellHeight` 可分别覆盖宽度和高度。`TextStyle` 设置格子的文字样式，`CornerRadius` 设置圆角，`CaretHeight` 可覆盖根据行高计算的光标高度。

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "数量"}
```

## 分组行

`ui.FormSection` 将各行放在同一张卡片上，标题在卡片上方，页脚在下方。`CardStyle` 可设置卡片的背景、边框和圆角。`CardStyleFunc` 在其上合并样式，并跟随读取的信号更新。`DividerInset` 接收每个子行，包括 `core.Each` 创建的行：返回缩进值和 `true` 可覆盖该行的分隔线缩进，返回 `false` 则保留自动标题对齐（其他节点默认为 12 点）。缩进为零时分隔线铺满宽度。

`ui.FormRow` 将 `Leading` 节点或图标底板放在标题前。用 `LeadingWidth` 为自定义节点预留宽度，使下方分隔线与标题对齐。预留宽度默认为 24 点。`TileSize` 同时调整图标底板、图标和圆角的大小。`TitleStyle` 和 `DetailStyle` 可覆盖标题和详情的文字样式。`TitleMaxLines` 限制标题行数（零为不限），`MinHeight` 以点覆盖该行的最小高度，`Below` 在同一文字列内放置标题和详情下方的节点。`MinHeight` 为零时保留默认值：40 点，触屏设备上为 48 点。

单选组可在 section 外包一层 `base.RadioGroup`，并为各行设置 `Role: core.RoleRadio`、`Checked` 和 `OnPress`。行通过 `base.Radio` 提供选中状态的无障碍信息、单一 Tab 停靠点和方向键选择；已选中的行再次按下时不触发操作。选中值由应用保存。

## 校验

规则由应用自己保存：一个 signal 保存错误，`OnBlur` 或 `OnSubmit` 设置它，有错误时 `core.Show` 在输入框下方添加提示。

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "邮箱",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("请输入邮箱地址。")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` 让出现的提示有动画，带 `ReturnKeyNext` 的 `OnSubmit` 把键盘移到下一个字段。

## 下一步

- [导航](navigation.md)：表单所在的页面。
- [文本](text.md)：文本框的文本如何设置样式。
- [无障碍](accessibility.md)：屏幕阅读器需要的标签和值。
