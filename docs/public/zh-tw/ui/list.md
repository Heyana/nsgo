---
title: 清單
description: 任意數量的資料列，僅建構可視範圍內的項目，並支援選取與啟用。
group: 元件
order: 38
---

`ui.List` 會在原生捲動檢視上繪製資料列，並僅建構靠近螢幕區域的列，同時支援選取、啟用與鍵盤操作。

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## 列與識別鍵

`Count` 指定總列數，`Row` 負責建構單一列。`Key` 賦予每一列唯一識別鍵：當上方有其他列新增或移除時，仍留在畫面上的列會保留各自的儲存格（cell）與狀態。當資料列可能被新增、移除或重新排序時，請使用 ID 而非索引值；索引值是一個訊號，會隨著儲存格顯示其他列而變動。

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` 是每列的預期高度，若為 0 則預設為 50 點。各列的高度可以不同；清單會實際量測已建構的列，並估算其餘列的大小。

## 版面配置

`Header`、`Footer` 與 `Empty` 用於放置資料列前後以及無資料時的內容。`Gap` 用於分隔各列，`Columns` 可將各列並排顯示，而清單的內距會套用至內容上，使捲軸保持貼齊邊緣。請為清單指定高度，或讓它填滿其父層容器。

## 選取與啟用

`ui.List` 接受一個透過 `ui.NewSelection(cx, multiple)` 建立的 `Selection`。識別鍵、標籤、啟用動作與鍵盤操作皆會與其連動：Up、Down、Home 與 End 移動游標，Shift 延伸選取範圍，Mod+A 全選，在多選模式下 Space 切換選取狀態，而 Enter 或雙擊則可啟用該列。

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange` 會在點擊、按鍵或鍵入搜尋（typeahead）改變選取的列之後執行，而不會在應用程式自行設定 `Selection` 時觸發；如此一來，詳細資料窗格即可跟隨使用者的選擇，同時應用程式也能將選取狀態移至透過其他方式開啟的項目。

`TapSelects` 可讓手機上的輕觸動作改為選取而非啟用。`RowLabel` 則是輔助技術所朗讀的內容，也是鍵入搜尋比對的依據。

## 載入與捲動

`OnLoadMore` 會在接近尾端時執行，每個列數門檻觸發一次。`Ref` 可透過 `ScrollToIndexAt`、`ScrollToEnd` 與 `ScrollToIndex` 進行捲動。底層的純清單元件 `core.List` 還提供 `OnLoadMoreAtStart`、用於聊天室的 `Inverted`、`StickyHeaderIndices`、`MaintainVisibleContentPosition`、`Viewport` 與 `ViewabilityConfig`，以滿足低階的使用情境（參閱[清單與捲動](../lists.md)）。

## 無障礙支援

此清單會作為單一 list 或 listbox 呈現，由 `Label` 命名，並以各列的文字或 `RowLabel` 作為該列的名稱。支援選取的清單會回報游標所在的列，並在鍵盤移動時進行語音播報。當螢幕閱讀器移動至可視範圍外時，會要求清單捲動至該處，進而建構到達的列。

## 平台特性

原生捲動檢視與虛擬化機制在每個目標平台上皆相同。由於儲存格會被回收重用，列的狀態必須跟隨其 `Key`；通常的原則是將建構狀態保留在應用程式中，而非列內部。

## 相關資源

- [清單與捲動](../lists.md)：完整指南，包含固定標頭與載入更多項目。
- [捲動](scroll.md)：純粹的捲動檢視。
- [DataTable](../lists.md#tables)：包含大量列與欄的表格。
- 展示畫廊（gallery）的 List、Table 與 DataTable 頁面。
