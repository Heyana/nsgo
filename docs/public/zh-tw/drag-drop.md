---
title: 拖放操作
description: 在同一個視窗內的檢視之間傳遞應用程式數值。
group: 核心概念
order: 57
---

`base/dragdrop` 可將同一個視窗內、位於同一個 `Area` 中的拖曳來源與放置目標連接起來。應用程式依然保有原本的檢視、樣式、選取狀態以及處理該數值的動作。來源預設在移動四個點（pt）後才開始拖曳，讓一般的點擊操作維持不受影響。

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

`Data` 會在拖曳開始時執行一次。若要同時攜帶多個選取項目，請回傳切片；若資料必須維持為開始拖曳當下的快照，請在其中複製可變資料。若 `Data` 為 nil，則會停用拖曳。`Threshold` 可設定觸發拖曳所需的移動距離。`Activation: dragdrop.AfterLongPress` 會先等待 `View.DelayLongPress`（預設為 500 毫秒），再於移動時開始拖曳；在滑動應優先觸發捲動的介面中，請明確選用此模式。

`View` 會保留其一般屬性，包括點擊處理常式與無障礙設定。來源會佔用 `View.Responder`，請將該欄位留空。其子元素可以包含具備正常動作的按鈕。`Preview` 會跟隨指標移動，且不會接收輸入事件或重新掛載來源；若未設定 `Preview`，則會顯示來源的標籤。請為來源提供標籤或預覽。巢狀的區域彼此獨立運作。

目標必須設定 `OnDrop` 才會接收數值。最內層的目標會優先獲得機會；若其 `Accept` 拒絕該數值，可接受該數值的上層目標便有機會接手。`OnEnter`、`OnMove` 與 `OnLeave` 可用來更新目標的外觀。每個事件皆帶有 `Data`、位於目標內的 `Position` 以及位於表面內的 `PagePosition`。停用或移除目前作用中的目標會清除其進入狀態；放開時也會再次檢查是否接受。位於其他檢視後方、遭到裁剪、已停用或位於強制回應視窗下方的目標無法接收放置事件。

`OnStart` 與 `OnEnd` 描述來源的拖曳工作階段。`OnEnd` 會收到 `Result{Data, Dropped, Target}`。按下 Escape、原生觸控取消、停用或移除來源，或是在可接受的目標之外放開，皆會在未完成放置的情況下結束。目標位置採用最後一次提交的幾何資訊；兩次提交之間的原生動畫不會被取樣。此機制用於在視窗內傳遞應用程式數值；系統檔案拖放仍使用 `OnFileDrop`。

請務必同時提供選單、鍵盤指令或無障礙動作來執行相同的操作，拖曳只是該動作的快捷方式。測試中可使用 `app.DragTo("id:poster", "id:playlist")`，或特定選定表面的 `DragTo`；TSX 測試則使用 `await app.dragTo("id:poster", "id:playlist")`。兩個檢視都必須已經位於可見視窗範圍內；該步驟會直接透過回應器（responder）傳送觸控序列，而不會合成假放置事件或在拖曳過程中捲動畫面。詳見[測試](testing.md)。
