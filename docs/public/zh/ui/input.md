---
title: 输入框
description: 平台的文本框，受控或非受控，带焦点和软件键盘。
group: 组件
order: 33
---

`ui.Input` 是 `ui` 外观的平台文本框，除非设置 `Multiline`，否则只有一行。它的字段与 `base.Input` 相同，所有带样式的输入框都是如此；`ui.Field` 是同一种输入框，没有自己的尺寸，边框不随指针和焦点变化。

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "你的名字",
	TestID:      "name",
}
```

## 受控与非受控

设置 `Value` 后输入框是受控的：应用保存值，并在 `OnChange` 中更新它。设置 `DefaultValue` 后输入框自己保存值。受控输入框可以通过把值设为 `""` 来清空。

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "新任务",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## 字段

`Multiline` 让文本换行，初始高度为三行并随文本增长，除非样式设置了 `Height` 或 `MinHeight`。`Masked` 隐藏字符，`Readonly` 阻止编辑但文本仍可选中和复制，`Disabled` 停用字段，`MaxLength` 限制用户输入的 UTF-16 单元数。`Placeholder` 可以用 `PlaceholderFunc` 跟随变化。

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` 选择软件键盘（`KeyboardEmail`、`KeyboardNumber`、`KeyboardURL` 等），`ReturnKeyType` 设置回车键的标签，`SubmitBehavior` 决定回车是提交还是换行。`AutoCapitalize`、`NoAutoCorrect` 和 `AutoComplete` 调整系统的辅助行为。

`InputSource` 在字段获得焦点时切换到指定的键盘输入源，失去焦点时还原用户原来的输入源，除非设置了 `KeepInputSource`：在 macOS 上用 `"com.apple.keylayout.ABC"` 让输入代码或命令的字段只打英文。ID 由平台决定，Windows 上是键盘布局 ID（如 `"00000409"`），Linux 上是 Fcitx 5 或 IBus 的输入法；未启用的输入源、手机和 Web 上不做任何改变。

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` 合并在输入框的外观之上，`StyleFunc` 再合并在它之上：它们设置的高度、内边距、字体或颜色等取代 `ui` 的，其余保持不变。只要两者都不设置 `BorderColor` 或 `Hover`，边框在指针下仍会加深，获得焦点时仍显示强调色。

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## 焦点与事件

`Ref` 让输入框获得或失去焦点。`OnFocus` 和 `OnBlur` 跟随焦点，`OnSelectionChange` 和 `OnCaretMove` 跟随选区和光标，`OnKeyPress` 收到每个按键，`OnPaste` 收到粘贴内容，`OnCompositionChange` 跟随输入法组合。

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

软件键盘会盖住手机底部。把 `core.EnvKeyboard` 放在内容的底部内边距或外边距上，内容就会停在键盘上方，并随键盘的移动一起动画。`cx.DismissKeyboard()` 关闭键盘，`cx.Env().KeyboardHeight()` 读取高度。

## 校验

规则由应用自己保存。一个 signal 保存错误，`OnBlur` 或 `OnSubmit` 设置它，输入框下方的提示显示它（[表单](../forms.md#校验)）。

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "邮箱",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("请输入邮箱地址。")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## 无障碍

输入框报告 `RoleTextField`，`Role` 设为 `RoleSearchField` 时报告为搜索框，并朗读 `Label`；`Placeholder` 不是标签，因此没有可见标签的输入框要设置 `Label`。`Readonly` 和 `Disabled` 会被报告，`OnSubmit` 是键盘上的动作。在 macOS、iOS 和 Linux 上，单行、密码和多行输入框都会通过原生的无障碍占位提示属性或提示文本提供当前的 `Placeholder`，随更新和清空同步变化，不会为多行输入框绘制的占位标签产生第二个焦点停靠点。

## 平台

iOS、Android、Windows 和 Linux 使用各自的原生文本框；Web 使用 `<input>` 或 `<textarea>`。软件键盘、自动填充、输入法和文字方向属于平台。焦点和输入在每个目标上都可以用相同的方式测试。

## 相关

- [表单](../forms.md)：输入框、校验、选择和键盘。
- [无障碍](../accessibility.md)：标签和焦点。
- [布局](../layout.md)：让内容停在键盘上方。
- gallery 的 Input 和 Form 页面。
