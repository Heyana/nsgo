---
title: 撰寫元件
description: 像 NSGo 自身的 base 與 ui 套件一樣，打造讓其他人在應用程式中使用的元件。
group: 撰寫元件
order: 70
---

本系列頁面專為元件作者編寫：無論是控制項函式庫、設計系統，還是讓應用程式以 Go 套件匯入的複雜檢視元件。閱讀前請先熟悉[元件與狀態](../components.md)及[樣式與主題](../styles.md)。

## 元件的基本結構

元件是一個帶有 `Render` 方法的型別，與應用程式中的寫法相同。其欄位即為應用程式傳入的設定；應用程式未指定的欄位會採用零值，因此每個欄位的零值都應該對應最常見的預設情況。

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

應用程式可以像使用任何其他元素一樣使用它：`chips.Chip{Text: "New"}`。

## 行為與外觀分離

NSGo 將每個控制項分為兩套實作。`base` 保存其行為、狀態與無障礙支援而不帶任何外觀，並由應用程式逐部分傳入樣式；`ui` 則在 `base` 之上套用外觀，讓行為邏輯集中於單一位置。元件函式庫也可以遵循相同的拆分方式：

- **無樣式元件**負責處理按壓、鍵盤、焦點以及螢幕閱讀器朗讀的內容，並公開每個部分的樣式欄位，如同 `base.Switch`。應用程式或其他函式庫可為其賦予各自的外觀（[部分與狀態](parts.md)）。
- **有樣式元件**包裝無樣式元件，並僅負責從目前生效的主題中選擇樣式，如同 `ui.Switch`（[外觀與主題](looks.md)）。

像徽章（badge）這類沒有值得共用之行為的元件，則可以直接實作為單純的有樣式元件。

## 套件組織原則

- 一個檔案只放一個元件，並以該元件命名（例如 `chip.go`），連同僅該元件使用的型別；多個元件共用的部分則獨立成一個檔案。
- 為複雜元件建立專屬的套件，其 API 名稱可省略元件名稱，例如 `base/dock` 中的 `dock.Area`。
- 將元件顯示的文字放在套件的 `locales/` 目錄中（[文字、測試與發佈](shipping.md)）。
- 不要依裝置類型隱式變更行為：在手機或桌面平台上會改變的大小或形式，應設為由應用程式傳入的欄位。

## 下一步

- [部分與狀態](parts.md)：受控數值、設定各部分樣式與無障礙設計。
- [外觀與主題](looks.md)：從主題取得顏色、變體與尺寸。
- [無樣式元件](base.md)：NSGo 內建的不含外觀控制項，供自訂樣式或二次封裝使用。
- [文字、測試與發佈](shipping.md)：翻譯、測試與 TSX 支援。
