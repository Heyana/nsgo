---
title: 對話框
description: 強制回應表面、工作表與提示框。
group: 元件
order: 41
---

`ui.Dialog` 是顯示於調暗的應用程式畫面之上的強制回應表面，在觸控螢幕上則會呈現為從底部滑出的工作表。`ui.Sheet` 是從底部滑出的面板，而 `ui.AlertDialog` 則用於請使用者確認操作。

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## 開啟與關閉

對話框是否開啟的狀態由應用程式持有。`OnOpenChange` 會接收對話框請求的每一次狀態變更，並附帶一個 `base.Reason` 用以區分點擊外部區域、按下 Escape 或返回鍵；`OnClose` 則在對話框關閉後執行。`NoCloseOnEscape` 與 `NoCloseOnBackdropPress` 可防止按下 Escape 或點擊外部區域時關閉對話框。

`Title` 與 `Description` 位於表面頂端（若會在開啟期間變動，請使用 `TitleFunc` 與 `DescriptionFunc`），而 `Footer` 則容納操作按鈕。`Bare` 會將整個表面交給 `Children` 自行配置，適用於應用程式自訂版面的對話框；在桌面平台上，系統會將其裁切於表面邊緣與 `Style`、`StyleFunc` 所設定的圓角內，而 `Inset` 則設定其在桌面視窗邊緣的最短距離。

在觸控裝置的工作表上，`Bare` 會移除預設的拖曳把手與內容內距，讓 `Children` 填滿整個面板。`NoGrabber` 僅隱藏拖曳把手，`ContentStyle` 與 `ContentStyleFunc` 可自訂內容容器，而 `TitleStyle` 與 `TitleStyleFunc` 可自訂標題。`Leading` 與 `Trailing` 會在標題列的兩端放置節點（例如取消與完成按鈕），並將標題置中於兩者之間。`StartingStyle` 與 `EndingStyle` 可自訂工作表的轉場動畫；當你需要自訂時間曲線時，請在其中包含 `Transition`。

`Style` 會合併覆蓋於表面預設外觀之上。`StyleFunc` 則在其之後合併，並在對話框開啟期間與重新開啟時追蹤所讀取的訊號（包含外觀主題）。例如，若搭配 `colors := ui.ColorsOf(cx)`，設定 `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` 即可跟隨主題變化。這兩個欄位同樣適用於 `ui.AlertDialog`。零值顏色會保留現有顏色；若要清除顏色，請使用非零的透明色（例如 `core.RGBA(0xFFFFFF, 0)`）。

## 工作表

`ui.Sheet` 是顯示於調暗背景遮罩上的底部面板；點擊背景遮罩、按下 Escape 或返回鍵時會呼叫 `OnClose`。

設定 `Label` 或 `LabelFunc` 可指定其無障礙對話框名稱，並可設定 `TestID` 以便在測試中選取該面板。`LabelFunc` 會追蹤其讀取的訊號，同時不會替換已開啟的內容。設定 `CloseOnSwipe: true` 時，向下滑動超過關閉門檻後也會呼叫 `OnClose`；請在該回呼中更新 `Open` 以關閉工作表。

設定 `Style` 或 `StyleFunc` 可自訂表面的背景、圓角半徑與內距，同時保留拖曳把手與安全區域的處理。`StyleFunc` 會追蹤其所讀取的訊號。僅設定 `CornerRadius` 而未設定 `CornerRadii` 時，會將四個角落都設為圓角，取代預設僅頂部圓角的樣式。

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## 提示框

`ui.AlertDialog` 用於請使用者確認或知悉某項訊息。每個動作按鈕都會將其關閉，而點擊外部區域則不會關閉。

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

具有 `Role: core.AlertCancel` 的動作會在按下 Escape 或返回鍵時執行，接著呼叫 `OnOpenChange(false, base.ReasonCancel)`。`Default: true` 會在桌面平台、網頁與 Apple TV 上開啟提示框時賦予該動作鍵盤焦點，並讓 Return 鍵選取它；點擊動作會先執行 `OnPress`，再呼叫 `OnOpenChange(false, base.ReasonSelect)`。當鍵盤焦點不在其他動作或欄位上時，Return 鍵會執行預設動作：若焦點已移至另一個動作，Return 鍵會觸發該動作；若提示框上方另有對話框，Return 鍵則會保留給該對話框。一旦某個動作執行後，無論 `OnOpenChange` 的實作為何，提示框都會關閉，且在 `Open` 變為 false 再變回 true 之前不會再執行其他動作。若多個動作帶有相同的標記，以第一個為準。若沒有任何標記，按下 Escape 會直接關閉而不執行任何動作，且 Return 鍵沒有預設動作。`Role: core.AlertDestructive` 用於描述破壞性動作的語意；外觀則由 `Variant` 或 `Color` 決定。

若要顯示系統原生的提示框或動作表，請使用 `cx.ShowAlert`（[平台服務](../services.md#alerts-and-the-clipboard)）。

## 行為

強制回應表面會阻擋其背後的應用程式操作、取得鍵盤焦點，除非另有設定，否則會在按下 Escape 與返回鍵時關閉。在觸控螢幕上，對話框會以工作表形式呈現，可跟隨向下拖曳並於超過門檻時關閉。

## 無障礙設計

在桌面平台與觸控螢幕上，`ui.Dialog` 會回報 `RoleDialog`（或其明確指定的 `Role`），而 `ui.AlertDialog` 則回報 `RoleAlertDialog`。兩者皆會先朗讀標題（或在標題不足以說明時朗讀 `Label`），接著朗讀描述內容。對話框開啟時焦點會移入其中，關閉時則歸還原處；背景遮罩不會成為焦點停駐點。頁尾中的按鈕則各自保留原有的角色與標籤。

## 平台行為

在 Android 與觸控螢幕上，對話框會以從底部滑出的工作表呈現；在桌面平台上則是搭配背景遮罩的置中表面。兩者內容完全相同，僅呈現方式有所差異。

## 相關主題

- [導覽](../navigation.md)：頁面、分頁與強制回應視窗的整合使用。
- [選單（Menu）](menu.md)：錨點指令與右鍵選單。
- [無障礙設計](../accessibility.md)：焦點與對話框角色。
- 元件展覽館中的 Dialog、Sheet 與 AlertDialog 頁面。
- [無樣式元件](../authoring/base.md#dialog)：`base.Dialog` 與 `base.Sheet`，支援 iOS 原生呈現方式與自訂動態效果。
