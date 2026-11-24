---
title: 清單與捲動
description: 具備原生手勢的捲動檢視，以及可回收列位以容納任意數量項目的清單。
group: 核心概念
order: 55
---

## 捲動檢視

`core.Scroll` 是平台原生的捲動檢視，因此慣性滑動（fling）、回彈（bounce）、捲軸、對齊吸附與觸控捲動皆為原生體驗。其子項目會以直欄排列，或加上 `Horizontal` 後改為橫列排列，並依實際內容需求延伸長度。

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

捲動檢視會填滿其容器，而內距（padding）應設定於其內容上，如此捲軸才會貼齊邊緣。除非樣式另有指定，否則它會縮小以適應其父層大小。若必須在同層元素縮小時仍維持其請求尺寸，請設定 `NoShrink`。`HideIndicator` 會隱藏平台的捲動指示器，而 `Bar` 則可由應用程式自行繪製捲軸。

`Behavior` 與 `BehaviorFunc` 可決定其捲動方式：`Paging`、`SnapToInterval` 與搭配 `SnapToAlignment` 的 `SnapToOffsets`、`DecelerationRate`、`NoBounces`、`ContentInset` 以及鍵盤收合規則。`MinZoomScale` 與 `MaxZoomScale` 允許雙指縮放，`OnRefresh` 與 `Refreshing` 可實作下拉重新整理，而 `OffsetValues` 則會在主執行緒上追蹤捲動偏移量，供跟隨捲動位置移動的特效使用。

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

`OnScroll` 每次介面更新最多回報一次偏移量，或依 `ScrollEventThrottle` 的間隔回報。`OnScrollDragStart`、`OnScrollDragEnd`、`OnMomentumStart`、`OnMomentumEnd` 與 `OnScrollEnd` 分別對應使用者手勢的各個起訖階段。透過 `Ref`，可從程式碼中呼叫 `ScrollTo`、`ScrollToEnd`、`ZoomTo`，以及用於清單的 `ScrollToIndex` 來進行捲動。

## 清單

`core.List` 會在原生捲動檢視上繪製長篇資料，同時僅建立靠近螢幕可見區域的列。`Count` 指定總列數，`Row` 負責建構單一列；`Key` 賦予每一列唯一識別鍵，使留在畫面上的列在上方其他列新增或移除時，仍能保留各自的儲存格（cell）。

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

`Row` 在每個被回收重用的儲存格上僅會執行一次，並在該儲存格重複用於另一列時更新 `r.Index()`，因此請在響應式函式（`ContentFunc`、`StyleFunc`、`NewMemo`）內部讀取 `r.Index()` 與項目訊號，而不要在建構該列時擷取一次性數值。閒置中的儲存格會被暫存（park），其內部的計算會暫停直到該儲存格再次繫結至作用中的列為止。

`Header`、`Footer`、`Empty` 與 `Separator` 用於放置清單前後與各列之間的內容。`NumColumns` 可將各列並排顯示，`Gap` 與 `ColumnGap` 設定列與欄之間的間距，而 `Horizontal` 則可水平捲動。`StickyHeaderIndices` 會將指定的列固定在可視區域（viewport）起點，直到下一個固定標頭將其推離，而 `StickyHeaderOffset` 可為其上方的頁首保留空間以免重疊。

`OnViewableItemsChange` 會依據 `ViewabilityConfig` 的門檻值回報目前可見的列，`OnLoadMore` 則會在可視區域接近尾端時執行，每達設定列數觸發一次。`OnLoadMoreAtStart` 會在接近起點時執行相同操作，而 `InitialScrollIndex` 可讓清單一開始就開啟在指定的列。`Alignment: core.ListAlignEnd` 會讓清單停靠於尾端，如同聊天室以閱讀順序排列各列一樣：開啟時位於最新一列、在使用者停留在尾端時自動跟隨新加入的列，並在對話內容較短時將其置底顯示。`Inverted` 則是改為將第 0 列置於尾端，而 `MaintainVisibleContentPosition` 會在使用者靠近尾端時跟隨持續增長清單的末端。

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` 會將指定列捲動至可視區域中央，`list.ScrollToEnd(true)` 則會捲動至尾端。`list.ListPosition()` 會讀取可視區域起點所在的列及其捲入偏移量，藉此保存閱讀位置：呼叫 `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` 即可讓重新建立的清單回到該位置。清單也可以與其他內容放在同一頁面中：將 `Viewport` 設為 `core.ListViewportParent`，外層的捲動檢視便會將清單連同其餘內容一併捲動。

## 表格

`base/datatable` 可顯示高達百萬列的表格，支援列與欄的虛擬化、固定表頭、將欄位固定於左側（`Fixed`）或右側（`FixedRight`）同時讓其餘欄位在其下方捲動、排序、單列或多列選取、調整欄寬與移動欄位，以及應用程式自訂儲存格。`MultiSelect` 支援以 Command 或 Control 鍵切換選取、Shift 鍵連續範圍選取以及搭配 Shift 的鍵盤延伸選取，而 `SelectedRows` 與 `OnSelectedRowsChange` 則可讓應用程式控制選取的列。`CellTooltip` 可為儲存格加上工具提示，該提示同時會向螢幕閱讀器描述儲存格內容，並可透過 `Styles.CellTooltip` 設定樣式；由於 `RenderCell` 繪製的內容不接收指標事件，因此在儲存格內放置 `ui.Tooltip` 並不會顯示。`base.Table` 是適用於較少量資料、呈現應用程式自訂列的簡易表格，而 `base/plot` 則用於繪製圖表。

若需跟隨語系或應用程式狀態變化的資料格線名稱，請使用 `DataTable.LabelFunc`。`ColumnGroup.LabelFunc` 則會在分組欄位標題保持於可視範圍內時更新其名稱。

## 下一步

- [動畫](animation.md)：跟隨捲動檢視執行的動態效果。
- [文字](text.md)：會自動換行的文字段落與排版。
- [導覽](navigation.md)：包含可捲動內容的頁面與畫面切換。
