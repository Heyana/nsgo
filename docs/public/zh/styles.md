---
title: 样式与主题
description: 类型化样式、Tailwind 类名、状态样式，以及浅色和深色主题。
group: 核心概念
order: 52
---

## 样式

`core.Style` 用一个 Go 值描述盒子的布局、外观和文本。每个字段的零值都表示未设置，因此样式可以叠加：`Merge` 只替换后一个样式设置的字段，子元素继承父元素设置的文本字段。要在已有长度的样式之上把长度设为无，在 `Padding`、`Margin`、`Borders`、`BorderWidth`、`CornerRadius` 或间距中使用 `core.Zero`，就像颜色使用 `core.Transparent` 一样：`Padding: core.All(core.Zero)`。

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

样式支持 CSS 的颜色、圆角、边框、描边、阴影、渐变、滤镜、背景模糊、遮罩、混合模式、3D 变换和过渡。`core.RGB`、`core.RGBA` 和 `core.ParseColor` 生成颜色，`core.LinearGradient` 及其同类生成渐变，`core.Ring` 生成 Tailwind 的 ring，`core.AllCorners` 设置每个角。`BorderCurveContinuous` 让圆角像 iOS 一样平滑接入边。盒子的阴影和描边留在 `Overflow` 的裁剪之外，例外见[布局](layout.md#定位)。背景滤镜像 CSS 的 `backdrop-filter` 一样改变视图背后的内容：`Blur` 按半径模糊，颜色滤镜改变颜色。iOS 对不带颜色滤镜的模糊绘制 `View.IOS.BackdropMaterial` 指定的系统材质；指定 `core.BackdropUltraThin` 以外的材质并带模糊时，iOS 绘制该材质，macOS 绘制 AppKit 最接近的材质，且材质不应用颜色滤镜。Android 从 Android 12 起绘制背景滤镜。`Style.GlassEffect`（`core.GlassRegular` 或 `core.GlassClear`，TSX 中为 `glassEffect`）在 macOS 26 和 iOS 26 上用 Liquid Glass 替代背景模糊（Windows 上使用 NSGo 的玻璃着色器；其他平台会回退到视图的背景模糊）。设置 `Style.GlassInteractive`（TSX 中为 `glassInteractive`）会在 macOS 27（`NSGlassEffectView.effectIsInteractive`）和 iOS 26（`UIGlassEffect.isInteractive`）上开启 Liquid Glass 的按压反馈，同时保持 `OnPress` 与可按压子视图正常工作。

`Transform` 会让指针目标随视图一起移动：点击、悬停和手势从绘制后的位置开始，报告的指针坐标会撤销变换。变换到父视图边界外的子视图仍可接收输入，除非被裁剪或滚动视口遮住。小程序目前还不会撤销其渲染层运行的关键帧动画（见[路线图](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)）。

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## 状态

视图的外观可以随状态改变。`Hover`、`Focus`、`FocusWithin`、`Active`、`Selected`、`Checked`、`Disabled` 和 `aria-*` 状态都是 `*Style`，在状态成立时覆盖其余样式。应用通过函数设置 `Selected`、`Checked`、`Disabled` 和 `Expanded`，平台报告按压和焦点。

`Active` 和 `group-active:` 在视图被按下时生效，这需要视图有按压处理函数，或有正持有触摸的 `Responder`。给普通视图加 `Active`、`active:` 或 `group` 不会让它变得可按压，也不会让它成为键盘 Tab 焦点；需要它自己接收焦点时设置 `Focusable`。

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

组件的某个部件可以借用外层控件的状态而不自己报告：`PartStates` 提供 `Checked`、`Selected`、`Disabled` 等，因此复选框的勾选标记会跟随复选框，屏幕阅读器也只会读到控件本身。

## 类名

`View`、`Text`、`Span`、`Input`、`Image`、`Scroll` 和 `Canvas` 上的 `Class` 接受 Tailwind CSS 4 类名，`base` 组件的每个部件也都有与样式并列的类名。工具类、变体和任意值都与 Tailwind 一致，使用它的默认主题和调色板。

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

类名合并在样式之下，因此样式字段优先。`ClassFunc` 让类名跟随变化，状态变体（`hover:bg-zinc-100`、`checked:opacity-100`）会成为样式的状态。`group` 和 `group-hover:` 让子元素跟随父元素的状态，`first:` 和 `last:` 跟随子元素的位置，`aria-checked:`、`data-open:` 和 `focus-within:` 跟随控件展示的状态。`pointer-fine:` 和 `pointer-coarse:` 按鼠标或触屏为子树设置样式，与 CSS 的 `pointer` 媒体特性一致，通过 `cx.Env().Pointer()` 读取（Web 端跟随浏览器的 `pointer` 媒体查询，桌面与 Mac 上为 fine，手机、平板与手表上为 coarse，TV 上两者都不匹配），并可与 `dark:`、`hover:` 等其他变体组合，由应用的类名显式选择，而非由 `core` 隐式更改。

用 `peer` 标记控件，就能让后面的同级元素跟随它的状态：例如复选框设置 `Class: "peer"`，后面的标签设置 `Class: "peer-checked:text-accent peer-disabled:opacity-50"`。peer 只影响同一父元素下排在它后面的同级元素（`Show` 和 `Each` 里的也算），不影响前面的同级元素，也不影响它内部的元素。`peer/field` 配合 `peer-checked/field:` 区分命名的标记，`peer-data-invalid:` 跟随控件的布尔 `invalid` 数据状态。标记和状态的变化都会响应式更新。标记本身不会让元素成为 Tab 焦点或接收点按，所以 `peer-active:` 只跟随本身接收点按的 peer，例如按钮。

`has-[>svg]:px-3` 在父元素有直接图标子元素时改变其内边距；`[&_svg]:size-4 [&_svg]:shrink-0` 设置后代图标的尺寸。`svg` 匹配 `core.Icon` 和 `SVGView`，条件渲染的区域不算一层元素。子元素、类名或状态变化时会重新匹配。`hover:[&_svg]` 跟随父元素的悬停，`[&_svg]:hover` 跟随各图标的悬停。选择器里的 `active` 只跟随本身接收点按的元素（例如按钮），与 `peer-active:` 相同。匹配规则先比较选择器优先级，再比较变体顺序；内联样式优先，除非类名标为 important。

选择器在所有目标上匹配 NSGo 元素，支持列表、子元素（`>`）、后代（`_`）、相邻（`+`）和后续兄弟（`~`）关系，类名、`data-*` 存在或相等条件、角色、支持的布尔无障碍状态，以及 `:is`、`:where`、`:not`、`:has` 和子元素位置。类型包括 `view`/`div`、`text`/`span`（整个文本节点）、`scroll`、`input`、`textarea`、`image`/`img`、`canvas`、`svg`、`video`、`webview` 和 `native`。任意选择器的每个分支使用一个顶层 `&`；省略时测试元素自身。浏览器专属选择器、伪元素、at-rule、转义和函数内显式的 `&` 均视为未知；选择器不会触及后端实现视图或单独的文本片段。

`Span.Class` 和 `Screen.HeaderClass` 里没有可跟随的同级元素，其中的 `peer-*` 变体会被忽略，并按类名和位置各记录一次日志；span 要跟随 peer 时，把变体写在包含它的 `Text` 上。

主题的 `Tailwind` 字段用颜色、断点和工具类扩展默认主题；调色板没有的颜色可以写成 `Palette` 的角色名（`bg-accent`、`text-text-muted/70`），或主题定义的颜色，从节点所在主题的当前外观读取。

## 主题

`core.Theme` 是同一种外观在浅色和深色下的定义：两套 `Palette`、一个圆角半径和字体系列。调色板的角色有 `Background`、`Surface`、`Text`、`TextMuted`、`Border`、`Accent`、`AccentText`、`Danger`、`Warning`、`Success`、`Selection`、`Focus` 和 `Scrollbar`；`Colors` 保存组件库添加的角色，例如 `inset` 和 `raised`。`core.DefaultTheme()` 是带蓝色强调色的中性主题，包含 `ui` 的角色，应用设置其他主题前显示的就是它；`ui.WarmDuskTheme()` 是暖色调色板。

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` 把主题给当前 scope 及其之后构建的所有内容，因此侧边栏或面板可以有各自的主题；传入 nil 会恢复上层的主题。`core.ParseTheme` 读取 YAML 或 JSON 主题文件，或 shadcn/ui 主题文件的 CSS 变量，其中包含角色颜色、圆角和字体系列。

`cx.SetAppearance` 强制整个应用使用浅色或深色，`cx.SetSubtreeAppearance` 只作用于子树，`core.AppearanceSystem` 重新跟随系统。`cx.Dark()` 返回子树当前的外观，`cx.Palette()` 返回应用了嵌入宿主覆盖后的颜色。

若祖先没有设置 `Style.Color`，文字和使用 `currentColor` 的图标会使用子树的 `Palette.Text`，并随主题、外观和宿主 token 原地更新。显式设置的颜色（包括 `core.Transparent`）继续向下继承。

## 组件中的颜色

组件在构建时和 `StyleFunc` 中读取颜色，因此主题或外观变化会原地生效，不需要重新构建组件。`ui.ColorsOf(cx)` 给出 `ui` 使用的角色，`ui.TintsOf(cx)` 给出它们的半透明填充，`ui.FinOf(cx)` 给出 `ui` 的层次质感。

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "你好", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` 读取的值变化时会再次运行，因此其中的 `cx.Dark()` 和 `cx.Palette()` 会跟随主题。`ClassFunc`、span 的 `StyleFunc` 和组件的 `StyleFunc` 字段同理。

## 指针样式

`Style.Cursor` 设置命名指针（如 `core.CursorText`、`core.CursorColResize`）；普通可点击控件保持 `core.CursorAuto` 以沿用系统箭头。在桌面和 Web 端需要画笔、准星等自定义图像指针时，可将 `Style.CursorImage` 设为 `*core.CustomCursor`，提供 1x 的 `Image`、可选的高分屏 `Image2x`（在 `PixelRatio >= 1.5` 时自动选用）、按逻辑点指定并裁剪到图像范围内的 `Hotspot`，以及加载失败或作为 Web CSS 回退使用的 `Fallback` 命名指针。可通过 `cx.Env().SupportsCustomCursor()` 判断当前平台是否支持自定义图像指针；触屏平台会忽略它。

## 下一步

- [布局](layout.md)：盒子放在哪里。
- [文本](text.md)：字体、span 和装饰。
- [动画](animation.md)：样式之间的过渡。
