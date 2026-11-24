---
title: 文字
description: 段落、文字片段、連結、字型與文件。
group: 元件
order: 32
---

`core.Text` 是由平台文字引擎排版的單一段落。`ui.Text` 與 `ui.Textf` 是使用 `ui` 字體樣式建立單行文字的輔助函式；其元素本質上仍是 `core.Text`，因此同樣接受 `Style` 與 `Spans`。

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## 內容

`Content` 為固定內容，而 `ContentFunc` 會追蹤訊號。`MaxLines` 限制顯示行數，並由 `EllipsizeMode` 在開頭、中間或結尾截斷多餘內容；`Style.WhiteSpaceNoWrap` 可讓段落維持在單行內。`TextAlign`、`TextTransform`、`LineHeight` 與 `LetterSpacing` 用於設定段落樣式，而 `OnTextLayout` 會在版面配置完成後回報其各行資訊。

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## 文字片段與連結

`Spans` 可由多個片段組成一段文字，每個片段各自擁有獨立樣式。設有 `OnPress` 的片段即為連結，而其 `Background` 與 `CornerRadius` 會繪製於每一行該片段字元的後方。當所讀取的訊號變更時，`SpansFunc` 會重新產生片段，適合用於語法突顯的程式碼或會變動的句子。

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` 可在文字行中放入檢視，取代該片段的內容：例如圖示、圖片或小型按鈕。它會依自身大小進行版面配置、隨文字一同換行，並接收自身的點擊事件。

## 字型與樣式

`FontFamily` 指定字型家族名稱或 CSS 清單。`FontSize`、`FontWeight`、`FontStyle`、`FontVariant`、`LineHeight`、`LetterSpacing`、`TextDecoration` 以及 `TextShadowColor` 搭配其偏移量與半徑用於設定文字樣式。`core.FontMonospace` 為平台的等寬字型家族。

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

請於第一個檢視之前使用 `nsgo.Fonts` 註冊打包的字型，或在應用程式執行期間使用 `cx.RegisterFont` 註冊字型。在字型載入前已完成準備的文字會重新準備。

## 文件

`base/textview.TextView` 使用相同的基礎原語顯示 Markdown 或 HTML：包含標題、段落、清單、表格、程式碼、註腳、提示框、圖片、公式與連結。`textview.Parse` 讀取 Markdown 而 `textview.ParseHTML` 讀取 HTML，且 `Options.Streaming` 可修復仍在串流抵達中的文件。

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` 可搜尋 `TextView` 所顯示的文字並突顯符合項目。使用 `textview.ProvideSearch` 為多個檢視提供同一個搜尋實例，接著呼叫 `Open`、`SetQuery`、`Next` 與 `Previous`；`Count` 與 `Selected` 會回報搜尋結果。搜尋部位可使用 `textview.PartSearch` 與 `textview.PartSearchSelected` 設定樣式。

## 選取與無障礙設計

`SelectableOn` 讓使用者能夠選取並複製文字。在 `core.View` 上設定 `Selectable` 則可讓其中的文字依文件順序一併選取。`Role`（例如 `RoleHeader`）會向輔助技術說明文字的語意角色，而當文字本身的內容不適合作為標籤時，可使用 `Label` 取代之。

## 平台行為

文字由平台的文字引擎進行版面配置，因此斷行、字型與輸入法皆會遵循系統行為。CJK 斷行、表情符號（emoji）與由右至左文字皆跟隨系統設定。應用程式可透過 `cx.Env().FontScale()` 讀取文字大小縮放比例，且文字除非設有 `NoFontScaling`，否則都會跟隨該比例縮放。

## 相關資源

- [文字](../text.md)：文字、片段、字型與文件的完整指南。
- [圖片與圖示](../images.md)：行內圖片與圖示。
- [無障礙設計](../accessibility.md)：標題、標籤與選取。
- 展示畫廊的 Text 與 TextView 頁面。
