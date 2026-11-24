---
title: 拖放
description: 在同一窗口的视图之间传递应用数据。
group: 核心概念
order: 57
---

`base/dragdrop` 将同一窗口中同一个 `Area` 内的拖动源（source）和放置目标（target）连接起来。应用保留原有的视图、样式、选中状态以及处理该值的操作。默认情况下，拖动源在指针移动 4 个点（pt）后才会启动拖动，从而不影响普通的按压操作。

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` 在拖动开始时运行一次。若需同时携带多个选中项，可返回一个切片；如果数据必须保持为快照，请在此处复制可变数据。`Data` 为 nil 时会禁用拖动。`Threshold` 用于设置触发拖动所需的移动距离。`Activation: dragdrop.AfterLongPress` 会先等待 `View.DelayLongPress`（默认为 500 毫秒），再检测移动；在轻扫手势应优先触发滚动的场景中，请显式选用此项。

`View` 会保留其常规属性，包括按压处理器和无障碍属性。拖动源内部占用 `View.Responder`，请将其留空。其子视图可以包含具有正常操作的按钮。`Preview` 会跟随指针移动，既不接收输入，也不会重新挂载拖动源；若未提供 `Preview`，则会显示拖动源的标签。请务必为拖动源提供标签或预览。嵌套的 `Area` 彼此独立运行。

放置目标需要设置 `OnDrop` 才能接收值。最内层的目标最先获得接收机会；当其 `Accept` 拒绝该值时，愿意接收的上层祖先目标可以接手处理。`OnEnter`、`OnMove` 和 `OnLeave` 可用于更新目标的外观。每个事件都携带 `Data`、目标内的局部位置 `Position` 以及所在界面中的位置 `PagePosition`。禁用或移除当前处于活动状态的目标会清除其进入状态；释放指针时会再次检查是否接收。被其他视图遮挡、被裁剪、被禁用或位于模态框下方的目标无法接收放置。

`OnStart` 和 `OnEnd` 描述拖动源的会话过程。`OnEnd` 会收到 `Result{Data, Dropped, Target}`。按下 Escape、原生触摸被取消、禁用或移除拖动源，或者在可接收的目标之外释放指针，都会结束拖动而不触发放置。目标位置判定使用的是上一次提交的几何信息；两次提交之间的原生动画不会被采样。该机制用于在窗口内部传递值；拖入系统文件仍应使用 `OnFileDrop`。

请保留能够执行相同操作的菜单项、键盘命令或无障碍操作。拖动只是该操作的快捷方式。测试可使用 `app.DragTo("id:poster", "id:playlist")`，或所选界面的 `DragTo`；TSX 测试使用 `await app.dragTo("id:poster", "id:playlist")`。这两个视图都必须已完整位于可见窗口内；该步骤会通过响应器发送触摸序列，而不会直接合成放置事件或在拖动期间滚动。请参阅[测试](testing.md)。
