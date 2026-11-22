---
title: Drag and drop
description: Transfer application values between views in one window.
group: Core concepts
order: 57
---

`base/dragdrop` connects a source and a target in the same `Area`, in one window. The app keeps its views, styles, selection and the action that handles the value. A source starts after four points of movement by default, leaving an ordinary press intact.

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

`Data` runs once when the drag starts. Return a slice to carry several selected items together; copy mutable data there if it must remain a snapshot. A nil `Data` disables dragging. `Threshold` sets the movement needed. `Activation: dragdrop.AfterLongPress` first waits for `View.DelayLongPress` (500 ms by default), then movement; choose it explicitly where a swipe should scroll first.

`View` retains its ordinary props, including press handlers and accessibility. A source owns `View.Responder`; leave it unset. Its children may include a button with its normal action. `Preview` follows the pointer without taking input or remounting the source; without one, the source's label is shown. Give a source a label or preview. Nested areas operate independently.

Targets need `OnDrop` to accept a value. The innermost target gets the first chance; when `Accept` rejects it, an accepting ancestor may take it. `OnEnter`, `OnMove` and `OnLeave` can update the target's appearance. Each event carries `Data`, `Position` in the target and `PagePosition` in the surface. Disabling or removing the active target clears its entered state; release checks acceptance again. A target behind another view, clipped, disabled or below a modal cannot receive the drop.

`OnStart` and `OnEnd` describe the source's session. `OnEnd` receives `Result{Data, Dropped, Target}`. Escape, native touch cancellation, disabling or removing the source, or releasing outside an accepting target ends without dropping. Target placement uses the last committed geometry; native animations between commits are not sampled. This transfers values within the window; system files still use `OnFileDrop`.

Keep a menu, keyboard command or accessible action that performs the same operation. Dragging is a shortcut to that action. Tests use `app.DragTo("id:poster", "id:playlist")`, or a selected surface's `DragTo`; TSX tests use `await app.dragTo("id:poster", "id:playlist")`. Both views must already fit in the visible window; the step sends a touch sequence through the responder without synthesizing a drop or scrolling during the drag. See [Testing](testing.md).
