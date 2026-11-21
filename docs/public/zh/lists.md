---
title: 列表与滚动
description: 带原生手势的滚动视图，以及为任意数量条目回收行的列表。
group: 核心概念
order: 55
---

## 滚动视图

`core.Scroll` 是平台自己的滚动视图，因此惯性滑动、回弹、滚动条、吸附和触摸滚动都是原生的。子元素按纵向排开，设置 `Horizontal` 后按横向排开，长度随内容而定。

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

滚动视图填满容器，内边距放在内容上，因此滚动条贴着边缘。除非样式另有设置，它会收缩以适应父容器。兄弟视图收缩时若它必须保持请求的尺寸，设置 `NoShrink`。`HideIndicator` 隐藏平台指示器，`Bar` 绘制应用自己的滚动条。

`Behavior` 和 `BehaviorFunc` 决定滚动方式：`Paging`、`SnapToInterval` 和 `SnapToOffsets` 配合 `SnapToAlignment`、`DecelerationRate`、`NoBounces`、`ContentInset` 以及键盘相关规则。`MinZoomScale` 和 `MaxZoomScale` 允许双指缩放，`OnRefresh` 和 `Refreshing` 让下拉刷新，`OffsetValues` 在主线程跟随偏移，用于随滚动移动的效果。

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` 每轮最多报告一次偏移，或按 `ScrollEventThrottle` 的间隔报告。`OnScrollDragStart`、`OnScrollDragEnd`、`OnMomentumStart`、`OnMomentumEnd` 和 `OnScrollEnd` 标出用户手势的起止。`Ref` 用 `ScrollTo`、`ScrollToEnd`、`ZoomTo` 和列表的 `ScrollToIndex` 从代码滚动。

## 列表

`core.List` 在原生滚动视图上显示长数据，只创建屏幕附近的行。`Count` 给出条目数，`Row` 构建一行；`Key` 给每行一个身份，因此在上面增删行时，保留下来的行会保留自己的单元格。

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` 对每个复用的单元格只运行一次，复用到另一行时会更新 `r.Index()`，因此应在响应式函数（`ContentFunc`、`StyleFunc`、`NewMemo`）里读取 `r.Index()` 和条目 signal，而不要在构建行组件时读成静态值。空闲单元格的作用域会被挂起（park），其中的计算会等到单元格重新绑定到有效行时再运行。

`Header`、`Footer`、`Empty` 和 `Separator` 放置在列表首尾和行之间。`NumColumns` 让行并排，`Gap` 和 `ColumnGap` 设置间距，`Horizontal` 横向滚动。`StickyHeaderIndices` 把这些行固定在视口起点，直到下一行把它们推走，`StickyHeaderOffset` 让它们上方的内容保持可见。

`OnViewableItemsChange` 按 `ViewabilityConfig` 的阈值报告可见行，`OnLoadMore` 在视口接近末尾时运行，每批行只运行一次。`OnLoadMoreAtStart` 在开头做同样的事，`InitialScrollIndex` 让列表从某一行打开。`Alignment: core.ListAlignEnd` 让列表停在末尾，像聊天一样且行保持阅读顺序：打开时就在最新一行，用户在末尾时跟随新行，较短的对话停在底部。`Inverted` 则把第 0 行放在末尾，`MaintainVisibleContentPosition` 在用户接近末尾时跟随增长的内容。

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` 把某一行滚到视口中间，`list.ScrollToEnd(true)` 滚到末尾。`list.ListPosition()` 读取视口从哪一行、该行内多远处开始，用于保存阅读位置：`list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` 让重新创建的列表回到那里。列表也可以和其他内容放在同一个页面里：设置 `Viewport: core.ListViewportParent`，外层的滚动视图会带着列表一起滚动。

## 表格

`base/datatable` 显示百万行的表格，行和列都虚拟化，带固定表头，可以把列固定在左侧（`Fixed`）或右侧（`FixedRight`），其余列在它们下面滚动，支持单选或多行选择、排序、调整和移动列，以及应用自己的单元格。`MultiSelect` 支持 Command 或 Control 切换、Shift 范围选择和 Shift 键盘扩展，`SelectedRows` 与 `OnSelectedRowsChange` 可让应用控制所选行。`CellTooltip` 为单元格提供提示，同一段文字也作为单元格的描述提供给屏幕阅读器，外观由 `Styles.CellTooltip` 设定；`RenderCell` 绘制的内容不接收指针，因此放在单元格里的 `ui.Tooltip` 不会显示。`base.Table` 用于较小数据的普通表格，`base/plot` 绘制图表。

## 下一步

- [动画](animation.md)：跟随滚动视图的效果。
- [文本](text.md)：会换行的文本行。
- [导航](navigation.md)：内容可以滚动的页面。
