---
title: 無樣式元件
description: NSGo 內建的不含外觀控制項，可逐部分設定樣式，或作為建置專屬樣式元件庫的基礎。
group: 撰寫元件
order: 73
---

`base` 套件包含 `ui` 所擁有的每一個控制項，具備完整的行為、狀態與無障礙支援，但不包含任何外觀樣式。每個部分都接受 `Class`、`Style` 與 `StyleFunc`（[部分與狀態](parts.md)），因此自訂設計的元件庫或應用程式可以直接使用它們來取代 `ui`，或像 `ui` 一樣將它們包裝起來（[外觀與主題](looks.md)）。本頁介紹[元件參考](../ui.md)中的各項控制項；展示畫廊（gallery）將每個無樣式元件與其 `ui` 形式並排呈現。

諸如 `Button`、`Checkbox`、`Select`、`Slider` 與 `NumberInput` 等控制項接受 `Invalid: rejected.Get` 以跟隨驗證訊號。可使用 `Class: "aria-invalid:border-red-500"` 或 `Style.AriaInvalid` 來設定該狀態的樣式。這會更新無障礙與樣式狀態，而不會停用控制項。複合欄位會將此狀態傳遞給其實際輸入框與裝飾性部分；`ColorPicker` 則會將其分享給內部的各個控制項。

若要在檢視之間拖曳應用程式數值，請使用 [`base/dragdrop`](../drag-drop.md)：由拖曳區域連接來源與目標，並由應用程式提供其內容、預覽與外觀。

## Button

`base.Button` 從 `Style` 取得外觀，並從 `Children` 取得內容。`OnPress` 在完成點擊時執行；`OnPressDown` 與 `OnPressUp` 則跟隨按下的開始與結束，讓你能自訂按壓回饋。

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`、`Busy`、`Selected` 與 `Expanded` 會向應用程式、樣式系統與輔助技術回報狀態：仍在執行中的動作用 `Busy`（可使用 `aria-busy:` 設定樣式），切換開關或清單中的目前項目用 `Selected`，展開收合控制項則用 `Expanded`。當列或清單透過其他方式到達按鈕時，`NoFocus` 可將按鈕排除在 Tab 停駐點之外。

## Checkbox

`base.Checkbox` 接受核取方框的樣式、指示器的樣式，以及內部顯示的勾選標記，並依據應用程式擁有的數值顯示狀態。

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

指示器透過 `PartStates` 從核取方塊取得狀態，因此可以使用 `checked:` 類別為其設定樣式，同時避免讓指示器成為第二個焦點停駐點。

若需要取得修飾鍵資訊，請設定 `OnChangeEvent func(checked bool, press core.PressEvent)`。它會取代 `OnChange` 並由應用程式自行管理數值；`press.Modifiers.Shift` 可讓清單實作範圍選取。在桌面平台、網頁、Android 與 iOS/tvOS 上，透過 Space 或 Return 啟用會保留修飾鍵狀態，除非應用程式快捷鍵先攔截了該按鍵；在 Windows 上，Alt 搭配 Space 或 Return 則會開啟視窗選單。在 TSX 中，`onChangeEvent` 會接收 `{ checked, press }`。

## Switch

`base.Switch` 由軌道、軌道內的滑塊，以及作為標籤的子元件組成。點擊時會回報其所請求的新值（即 `Checked` 的相反值）。

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

軌道與滑塊皆接受各自獨立的 `StyleFunc`，且其狀態樣式會跟隨 `Checked`。

## Slider

`base.Slider` 接受 `base.SliderValue`：`base.SingleValue(v)` 或 `base.RangeValue(low, high)`。它可分別設定軌道、指示器、範圍與滑塊的樣式。

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`、`Max` 與 `Step` 皆為響應式讀取函式（reactive getters）：變更其訊號會在滑桿已掛載時（包含拖曳過程中）即時更新滑桿及其無障礙範圍。`Axis` 可將其設為垂直方向，`Inverted` 則將最小值置於遠端（與 `base.Progress` 相同）；`Marks` 可在指示器上放置自訂刻度，而 `OnChangeEnd` 會在拖曳停止時回報數值。

## Select

`base.Select` 接受 `base.SelectItem` 陣列與 `*string` 數值，因此「未選取」即為 `nil`。你可以自行設定觸發器、彈出視窗、選項列與群組的樣式。

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

相鄰且具有相同 `Group` 的項目會共用同一個標頭，鍵入搜尋（typeahead）會比對項目標題，`SearchField` 可繪製自訂搜尋欄位，而 `Cleanable` 則會顯示清除按鈕。`OnCancel` 會接收 Escape、返回鍵與彈出視窗外部的點擊。

## Tabs

`base.Tabs` 包含多個 `base.Tab`，並根據 `IndicatorStyle` 繪製指示器；應用程式負責保存目前選取的分頁。`base.Tab.OnPress` 在每次點擊時皆會執行，包含點擊已選取的分頁時。

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` 接受背景遮罩、圖層與彈出視窗的樣式、`Animation` 或 `TransitionShader`，以及你的子元件。`Open`、`DefaultOpen` 與 `OnOpenChange` 控制其開關狀態；`OnOpen`、`OnClose` 與 `OnCancel` 回報所發生的事件。`OnConfirm` 為預設動作：按下 Return 會觸發它，除非鍵盤焦點位於會自行處理 Return 鍵的按鈕或輸入欄位上，或有另一個強制回應視窗覆蓋該對話框，隨後對話框會以 `ReasonSelect` 關閉。`Role` 與 `AccessibilityDescription` 決定螢幕閱讀器朗讀的內容，確認對話框請使用 `RoleAlertDialog`。`NoCloseOnSwipe` 可防止 iOS 上透過滑動手勢關閉對話框，`Presentation` 選擇 iOS 的呈現方式，而 `StatusBar` 則可在對話框開啟期間變更狀態列外觀。

`base.Sheet` 是停駐於任意邊緣的面板，具備背景遮罩與自訂內容；設定 `CloseOnSwipe` 後，底部工作表會跟隨向下拖曳手勢並在超過閾值時關閉。它與 `base.Dialog` 一樣接受 `OnConfirm`、`Role` 與 `AccessibilityDescription`。`base.ShowAlertDialog` 會顯示系統的警告對話框或動作工作表，並依角色排列按鈕：

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` 會從自訂的觸發元素開啟 `base.MenuItem` 列表，而 `base.ContextMenu` 則會在指標位置開啟選單。項目點擊時會觸發 `OnSelect` 並關閉選單，除非設定了 `NoCloseOnSelect`；`Checked` 可使其成為可勾選項目，`Indicator` 在項目內容前繪製自訂標記，而 `Role` 決定輔助技術朗讀的類型：選單項目、核取方塊或單選項目。`base.Submenu` 則可容納更多子項目。

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`、`Align`、`Offset` 與 `Margin` 用於在桌面平台上定位彈出選單，`PopupStyle` 與 `BackdropStyle` 則用於設定其樣式。

## EmojiPicker

`base.EmojiPicker` 會依據應用程式的語言（或明確指定的 `Languages` / `LanguagesFunc`），透過在地化的 CLDR 名稱、關鍵字與簡碼搜尋所有 Unicode 表情符號，並提供分類分頁、選填的 `Recent` 最近使用項目、膚色選擇、鍵盤導覽與虛擬化格線。所有預設標籤與分類名稱皆來自 `base/locales`，且表情符號資料集僅會連結進實際使用 `EmojiPicker` 或其輔助函式（`base.SearchEmojis`、`base.EmojisInCategory`、`base.EmojiByString`）的應用程式中。

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` 與 `DefaultCategory` 選擇瀏覽分組。分類分頁與膚色選項各自作為單一 Tab 停駐點，並支援方向鍵導覽。`EmojiPickerRef` 提供 `Focus`、`Query`、`SetQuery`、`SetCategory` 與 `MatchedCount`；含樣式的選擇器共用相同的查詢狀態。`LabelFunc`、`SearchLabelFunc` 與 `PlaceholderFunc` 可原地更新文字。`Emoji.WithSkinTone` 會將所選膚色套用至雙人組合中的兩個人物，預設膚色則會恢復為中性形式；以表情符號搜尋時會保留其精確序列。

若需搭配 Lucide 分類圖示、可清除搜尋欄位、高亮表情符號預覽與膚色色票的主題化選擇器，請使用 `ui.EmojiPicker`。
