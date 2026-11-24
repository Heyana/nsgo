---
title: 選擇器
description: 從清單中選取單一值，可呈現為彈出選單或工作表。
group: 元件
order: 36
---

`ui.Select[T]` 可從任意可比較型別的值清單中選取其中一項。在所有裝置上清單預設都會在觸發器下方開啟；設定 `Presentation: base.PresentSheet` 則可改為從底部滑出的工作表形式開啟，符合手機應用程式的常見需求。

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## 選項與搜尋

`Options` 為一組 `ui.Option[T]`，每項包含 `Value`、`Label`，以及可選的 `Group`、`Icon`、`Disabled` 函式或由應用程式自訂的 `Content`。在 `Select` 或 `Combobox` 中，相鄰且屬於同一非空 `Group` 的選項會顯示於該群組標題之下；方向鍵與鍵入搜尋會略過該標題，而螢幕閱讀器則會將其朗讀為標題。當選項會跟隨訊號變動時，請使用 `OptionsFunc`，讓已開啟的清單能就地更新並保留欄位的目前值與焦點。`Placeholder` 會在尚未選取任何選項時顯示。`Searchable` 支援透過打字篩選清單、`OnClear` 會顯示清除按鈕，而 `Disabled` 則可停用選擇器。

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` 用於設定選擇器的版面配置，未設定寬度時預設為 200 點；其高度、內距與字體設定會套用至欄位本身，就像 [Input](input.md) 一樣，因此選擇器能夠與 `SizeSmall` 按鈕對齊：

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## 行為

在觸發器取得焦點時，按下 Up、Down、Return 與 Space 會開啟清單；方向鍵可移動反白項目，鍵入搜尋則可依項目標題跳轉。選取項目後會關閉清單並回報其值。按下 Escape、返回鍵或點擊外部區域會關閉清單而不變更數值。`Empty` 則為搜尋無結果時所顯示的文字。

## 無障礙支援

`Label` 用於為輔助技術命名此欄位。當訊號變動時，可使用 `LabelFunc` 就地更新該名稱。若標籤為空，則會改用 `Placeholder`。觸發器與清單皆帶有選擇器的無障礙角色，並會回報目前已選取的項目。鍵盤可透過 Tab 抵達觸發器，並以 Return 或 Space 開啟它。

## 平台特性

在 macOS、Windows 與 Linux 上，清單會以彈出選單的形式顯示在觸發器旁。在 iOS 與 Android 上則會以工作表呈現，而已選取的列會維持顯示於欄位上。相同的選項與數值設定在每個目標平台上皆可運作。

## 相關資源

- [表單](../forms.md)：表單中的選擇器。
- [選單](menu.md)：用於執行命令而非選取數值。
- [分頁](tabs.md)：一次顯示少數選項。
- 展示畫廊（gallery）的 Select 與 Combobox 頁面。
- [無樣式元件](../authoring/base.md#select)：`base.Select`，支援分組與自訂觸發器。
