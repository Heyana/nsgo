---
title: 布局
description: Flexbox、grid、尺寸与定位，以及安全区域和键盘如何参与布局。
group: 核心概念
order: 51
---

## 盒子

每个元素都是一个由引擎布局的盒子。默认值沿用 React Native：纵向 flexbox，子元素默认保持自身尺寸，交叉轴方向拉伸，尺寸包含内边距和边框。

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "一"},
		core.Text{Content: "二"},
	},
}
```

`Column`、`Row`、`ColumnReverse` 和 `RowReverse` 设置主轴；`Wrap` 与 `WrapReverse` 让子元素换行。`JustifyContent` 沿主轴排列，`AlignItems` 或单个子元素的 `AlignSelf` 沿交叉轴排列。`Gap`、`RowGap` 和 `ColumnGap` 设置间距，样式中的内外边距使用 `core.All`、`core.Symmetric` 或 `core.Edges`。

## 尺寸

`Width`、`Height`、`MinWidth`、`MaxWidth` 和 `FlexBasis` 接受 `core.Pt` 表示点，或 `core.Pct` 表示父容器的百分比。`FlexGrow` 分配剩余空间，`FlexShrink` 允许盒子让出空间；两者都不设时盒子保持自身尺寸。`AspectRatio` 是宽除以高。`AutoMargin` 让它指定的边占据剩余空间，因此可以让一个盒子被推到另一端。

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

没有指定宽高时，内容决定盒子的大小。`core.MinContent`、`core.MaxContent` 和 `core.FitContent` 采用 CSS 的同名尺寸：文本的 min-content 宽度是最宽单词的宽度。行内文本只有设置 `FlexShrink: 1` 才会收缩并换行；需要占据剩余空间时再加上 `FlexGrow: 1`。未设置的最小尺寸已经是 0，与 React Native 相同，所以 `MinWidth: core.Pt(0)` 不起作用；当行比最宽的单词还窄时，收缩的文本会把单词断开。`MinWidth: core.AutoMinimum` 让它停在最宽单词的宽度，与 CSS 的自动最小尺寸相同。

填满窗口的页面要设置 `FlexGrow: 1` 和 `FlexShrink: 1`，它和需要滚动的列表或滚动视图之间的每一层盒子也是如此。只设置 `FlexGrow` 时，盒子会长到内容的大小，长列表会把它撑出窗口，而不是在里面滚动。

## Grid

`DisplayGrid` 用 CSS grid 排列子元素。`GridTemplateColumns` 和 `GridTemplateRows` 接受 CSS 轨道语法，`GridRow` 与 `GridColumn` 按网格线或跨度放置元素。

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` 会按可用宽度排出尽可能多的列。`GridFlow` 选择网格填充顺序，`JustifyItems`、`JustifySelf` 和 `AlignItems` 把元素放进各自的格子。

## 定位

`Relative` 是默认值，盒子留在流中，`Top`、`Right`、`Bottom` 和 `Left` 让它偏移。`Absolute` 把盒子移出流，放进最近的已定位祖先。`Fixed` 把它放进根视图或所在的模态，不受滚动影响；`Sticky` 在最近的滚动视图中吸附，并限制在父容器内：`Top` 距离顶部，`Top` 为自动值时则用 `Bottom` 距离底部（横向滚动对应 `Left` 和 `Right`）。设置 `Position: core.Sticky, Bottom: core.Pt(0)` 的底栏在长内容滚动时留在底部，在短内容后正常排列。`ZIndex` 决定盒子在同级之间的堆叠顺序。

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` 让盒子留在流中并忽略 inset，因此其中的绝对定位子元素会去找更上层的祖先。`Overflow` 把子元素裁剪在盒子的边框以内，与 CSS 裁剪到内边距盒一致，每个角的半径减去它相邻两边的边框宽度；`DisplayNone` 把盒子移出布局。

在 Android、Windows、Linux、Web 和 Apple 上，盒子自身的外阴影和描边留在裁剪之外，与 CSS 一致；Apple 上的各类视图都是如此，包括各角半径不同或滚动内容缩放的情况。HarmonyOS 仍在盒子外缘裁剪，裁剪范围包括边框。

## 方向与环境

`LayoutDirection` 让一个子树从左到右或从右到左布局，覆盖应用的方向：`DirectionRTL` 和 `DirectionLTR` 设置它，默认继承。内外边距、边框和 inset 中的左右会随方向互换，因此它们像 React Native 一样是起点和终点。`PhysicalEdges` 和 `PhysicalBorders` 让指定边保持字面位置。`WritingDirection` 决定混合方向文本的排列，`WritingAuto` 从第一个有方向的字符取方向。

`PaddingEnv` 和 `MarginEnv` 在指定边加上环境值。`core.EnvSafeArea` 是该边的安全区域内边距，`core.EnvKeyboard` 是软件键盘的高度，因此内容会停在键盘上方，并随键盘的移动一起动画。

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` 表示每一条边，`cx.Env().SafeArea()` 和 `cx.Env().KeyboardHeight()` 在代码中读取这些值。

在可折叠或双屏手机上，`cx.Env().Geometry()` 列出折痕或铰链把窗口分成的区域（每个区域带有落在其中的安全区域）以及区域之间的折痕；它的 `State` 和 `Angle` 表示设备打开的程度（`core.HingeClosed`、`HingeHalfOpen` 或 `HingeOpen`），即使没有折痕穿过窗口也会报告，例如在外屏上或完全展开时。其他设备报告一个区域和 `HingeUnknown`。在 iOS 27.1 上，`cx.Env().VerticalBarEdge()` 报告系统为纵向栏选择的边（`core.VerticalBarLeading`、`core.VerticalBarTrailing` 或 `core.VerticalBarUnspecified`），当 `mobile.TabBar` 或 `ui.TabShell` 的 `Edge` 为 `mobile.TabBarAutomatic` 时会自动跟随。框架不会据此隐式布局，由应用自己选择分栏。

## 响应式类名

`Class` 接受 Tailwind CSS 类名：`flex`、`flex-col`、`flex-wrap`、`items-center`、`justify-between`、`gap-3`、`p-4`、`w-full`、`grid`、`grid-cols-3`、`col-span-2` 等。断点跟随窗口，外观、方向和减少动态效果跟随环境：

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`、`md:`、`lg:`、`xl:` 和 `2xl:` 分别从 40、48、64、80 和 96 rem 宽开始生效，`max-md:` 在某个断点以下生效。`portrait:`、`dark:`、`rtl:`、`motion-reduce:`、`contrast-more:`、`pointer-fine:`、`pointer-coarse:`、`ios:`、`android:` 和 `web:` 跟随环境。类名合并在 `Style` 之下，状态类名（`hover:`）会成为状态样式，两者可以混用。

## 下一步

- [样式与主题](styles.md)：颜色、圆角、阴影和子树的外观。
- [文本](text.md)：文本如何取得尺寸并换行。
- [列表与滚动](lists.md)：滚动内容和回收的行。
