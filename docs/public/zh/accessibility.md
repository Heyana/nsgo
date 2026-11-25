---
title: 无障碍
description: 面向屏幕阅读器和键盘的标签、角色、值、操作和焦点。
group: 核心概念
order: 61
---

## 屏幕阅读器读到什么

视图的 `Label` 是辅助技术朗读的内容，`AccessibilityDescription` 是在标签之后朗读的补充说明，`Hint` 说明激活它会发生什么，`Role` 说明它是什么：`core.RoleButton`、`RoleLink`、`RoleHeader`、`RoleImage`、`RoleCheckbox`、`RoleSwitch`、`RoleRadio`、`RoleTab`、`RoleTextField`、`RoleSearchField`、`RoleStatus`、`RoleMenu`、`RoleMenuItem`、`RoleDialog`、`RoleAlertDialog`、`RoleAlert`、`RoleAdjustable`、`RoleProgressBar`、`RoleTable` 等。状态视图默认是礼貌播报的 live region；搜索框仍是文本输入框，但辅助技术会将其读作搜索框。对话框打开时会朗读它的角色和标签，再朗读它的说明，`ui.Dialog` 的标题和描述就是这样朗读的。没有标签的文本会朗读它的内容。`LabelFunc`、`HintFunc` 和 `AccessibilityDescriptionFunc` 在同一个视图上绑定会变化的无障碍文本，让控件在切换语言时保留焦点和状态。所有支持 `Label` 的 `core` 元素也接受 `LabelFunc`，`View` 和 `Text` 接受 `HintFunc`。

`ui.Button`、`ui.Checkbox`、`ui.Radio`、`ui.MenuItem` 和 `ui.AlertAction` 接受 `TextFunc`，无障碍名称也随文字变化。按钮、复选框和单选框还可以通过 `LabelFunc` 单独指定无障碍名称。滑块、选项卡、开关、单选组、进度条和弹出层触发器也接受 `LabelFunc`。选项卡、选择器和组合框通过 `ui.Option.LabelFunc` 更新选项文字。数字输入框、验证码输入框、进度环和两种加载指示器也接受 `LabelFunc`。

```go
core.View{
	Role:  core.RoleButton,
	Label: "加入购物车",
	Hint:  "加入商品并打开购物车",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` 给出范围值或文本值：滑块的最小值、最大值和当前值，或“3 件商品”这样的文本。`Accessible` 把视图变成一个元素，因此卡片的名称和价格会一起朗读，而不是两个停靠点；没有标签时它会朗读子元素的文本。`AccessibilityElementsHidden` 隐藏装饰，例如箭头或分隔线，`ImportantForAccessibility` 隐藏整个子树。

```go
core.View{Accessible: true, Label: "苹果，每公斤 12 元", Children: []core.Node{
	core.Text{Content: "苹果"},
	core.Text{Content: "每公斤 12 元"},
}}
```

## 可见标签

把控件的 `Ref` 作为 `LabelFor` 传给 `core.Text` 或 `core.View`。点击标签会聚焦控件并执行它的按压操作，例如切换复选框。标签当前可见的文本会成为控件的无障碍名称，无需重复填写 `Label`；标签的无障碍状态和样式也会跟随控件的 `Disabled`、`Invalid`。标签不会因此变成按钮或增加 Tab 停靠点。

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "邮箱", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` 按给定顺序组合多个可见元素的名称，优先于 `Label`，但不转发点击。未设置它时，`Label` 优先于按树顺序排列的 `LabelFor` 名称。文本更新、移动、隐藏、卸载及重新挂载都会更新关联。缺失、重复、循环和隐藏的引用会被忽略；关联仅限同一表面、同一隔离范围。标签不能激活模态层后面的控件。

`base` 和 `ui` 的按钮、复选框、单选框、开关、选择框及数字输入框会把 `Ref`、`LabelledBy` 传给实际控件；滑块提供 `ThumbRef` 和 `StartThumbRef`。TSX 使用 `labelFor={field()}` 和 `labelledBy={[label()]}`，通过 `ref={node => field.set(node)}` 把节点写入信号，因此放在输入框前面的标签也能在输入框挂载后建立关联。

## 操作

`AccessibilityActions` 列出屏幕阅读器在元素上提供的操作：标准操作（`core.ActionActivate`、`ActionIncrement`、`ActionDecrement`、`ActionEscape`、`ActionMagicTap`）和应用自己的操作，每个都有 `Name` 和 `Label`。`OnAccessibilityAction` 接收名称。可在 `core.View` 或 `core.Input` 上使用 `AccessibilityActionsFunc`，更新可用操作或翻译操作标签，原元素保持挂载。

```go
core.View{
	Role:  core.RoleAdjustable,
	Label: "数量",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d 件", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` 标记允许同时选择多个项目的网格、列表框或树。`base.DataTable` 在多行选择时设置它；Web、Windows 和 Linux 会暴露对应的辅助功能标记。

`Expanded` 说明展开控件或菜单按钮是否打开，`Busy` 表示内容仍在加载，`Invalid` 表示应用拒绝了字段的内容或控件的值（用 `aria-invalid:` 设置样式），`Checked`、`Selected` 和 `Pressed` 报告控件状态。控件的部件可以通过 `PartStates` 借用它的状态，因此指示器会跟随复选框，而不会成为单独的停靠点。

`SelectionItem` 独立于当前 `Selected` 值，标记视图是可选集合中的一个项目。`base.DataTable` 在表格启用行或单元格选择时为它们设置此标记，`base.Tree` 为每一行设置；Windows 始终保留 SelectionItem 模式，Web 写入 `aria-selected="true"` 或 `"false"`。未启用选择的表格不会给行添加这两个标记。

## 实时变化

`LiveRegion` 让屏幕阅读器在视图标签和其中文本变化时朗读：`core.LiveRegionPolite` 等用户空闲，`LiveRegionAssertive` 打断当前朗读。`cx.Announce` 朗读一次消息。

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("购物车里有 %d 件商品", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## 焦点与键盘

工具提示在触发控件获得焦点前就提供描述。`View.Tooltip`、`base.Tooltip` 和 `ui.Tooltip` 将当前文本作为控件的无障碍描述；与控件名称相同的文字会去重，应用设置的 `AccessibilityDescription` 优先保留，`Disabled` 的工具提示不提供描述。包裹控件的容器会把描述传给其中的控件。气泡及其子元素对屏幕阅读器隐藏，不会作为警报播报，因此 `ui.IconButton` 的默认提示不会重复朗读标签。

`base.DataTable` 将带标签的网格作为键盘和辅助功能的焦点停靠点；指针命中层对辅助技术隐藏。

在桌面、Web 和 Apple TV 上，可按压的视图和文本框一起成为 Tab 停靠点；`NoFocus` 让一个视图退出，`Focusable` 让没有按压的视图成为停靠点。`Style.FocusVisible` 只在键盘移动焦点时绘制焦点环（`focus-visible:`），`Style.Focus` 对应任何焦点，`Style.FocusWithin` 对应视图内的焦点，`OnFocus` 和 `OnBlur` 听到焦点变化（获得焦点的视图离开树时也会收到 `OnBlur`），`Ref.Focus()` 和 `Ref.SetAccessibilityFocus()` 移动焦点；`core.FocusedRef(cx)` 返回指向当前焦点视图的 ref，供没有触发器、由按键打开的界面（例如查找栏）在关闭时把焦点交还。Return 和空格按下获得焦点的视图，方向键在标签列表、单选组和菜单中移动。键盘把焦点移到视图上时，它的 `Tooltip` 立即显示，直到焦点离开或按下 Escape。弹出层的 `OnOpen` 在内容就绪后运行，在其中调用 `Focus` 就能把焦点移进去，非模态对话框应当这样做；popover 的内容在 Tab 顺序中紧跟触发器，弹出层关闭时焦点回到触发器。

`cx.Env().ScreenReader()` 说明是否有屏幕阅读器运行，`cx.Env().ReduceMotion()` 说明用户是否要求减少动态效果，`cx.Env().IncreaseContrast()` 说明用户是否要求增强对比度（macOS、iOS 和 watchOS 的“增强对比度”、Windows 的对比度主题、GNOME 的高对比度、GTK 4.20 的对比度设置或高对比度 GTK 主题、Android 14 高于标准的对比度设置，Web 上的 `prefers-contrast: more` 或 `forced-colors: active`），`cx.Env().FontScale()` 是文本大小，`cx.Env().RTL()` 是布局方向。除非设置 `NoFontScaling`，文本会跟随文本大小；从右到左的布局使用逻辑边时会镜像物理边。

## 下一步

- [样式与主题](styles.md)：焦点和禁用状态。
- [表单](forms.md)：字段的标签和值。
- [画布与着色器](canvas.md)：暴露绘图的部件。
