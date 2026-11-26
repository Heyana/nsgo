---
title: 文字
description: 使用文字片段、字型、裝飾、選取與文件顯示文字。
group: 核心概念
order: 53
---

## 文字

`core.Text` 代表一個段落，在引擎端由平台自身的文字引擎進行排版，並由主執行緒繪製。其大小取決於字型與寬度，並會在其方塊內自動換行。

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` 為固定內容，而 `ContentFunc` 會追蹤訊號，因此會變動的文字應在該函式中讀取訊號。`MaxLines` 限制顯示的行數，`EllipsizeMode` 會在開頭、中間或結尾截斷多餘內容，而 `WhiteSpaceNoWrap` 可將段落維持在單行內。`AdjustsFontSizeToFit` 會縮小文字直到符合其方塊與行數限制，最小可縮放至 `MinimumFontScale`（至少為 0.1）。公式與其他相對於字型的附件會隨之縮小，並在空間允許時恢復為原始大小；明確指定的點數大小與內距則會維持原設定值。

文字相關欄位會像 CSS 一樣繼承：位於設有 `FontSize`、`Color` 或 `LineHeight` 的檢視內的文字，除非自行設定，否則會沿用這些屬性。`TextAlign`、`TextTransform`、`LetterSpacing`、`TextDecoration`、`TextDecorationStyle`、`TextDecorationColor` 以及 `TextShadowColor`、`TextShadowOffset` 與 `TextShadowRadius` 可為行與字形套用樣式。

## 文字片段與連結

`Spans` 可將一段文字組成多個片段（run），每個片段皆有各自的樣式。設有 `OnPress` 的片段即為連結，而點擊其他片段則會觸發文字本身的 `OnPress`。

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` 會在所讀取的資料變更時重新傳回片段，並僅重新準備有差異的部分，因此程式碼編輯器的各列可在語法突顯抵達時重新上色。片段的 `Background` 與 `CornerRadius` 會在每一行的字元後方繪製背景方塊，而其 `Cursor` 會優先於文字本身的設定來指定指標懸停時的外觀。`Visibility: core.VisibilityHidden` 會原地隱藏片段：其中的字詞仍保留原有空間，因此顯示時各行位置不會移動，且複製與螢幕閱讀器會略過該片段，適合用於逐字淡入的效果。

在帶有 `Background` 的片段上設定 `BackgroundInsetTop: core.Pct(62)`，即可突顯每一行中該方塊下半部 38% 的區域。`BackgroundInsetBottom` 則會修剪下緣。這兩個內縮值皆可接受點數，或以從字型上緣（ascent）到下緣（descent）的方塊高度百分比表示，且不會改變文字版面配置。在 TSX 中，請於巢狀 `<Text>` 的樣式裡設定 `backgroundInsetTop: "62%"` 與 `backgroundInsetBottom`。圓角背景與垂直內縮也會在 HarmonyOS 及四個小程式目標平台上繪製。

`Attachment` 可將檢視放入文字中，取代該片段的內容。它會依自身大小進行版面配置、隨周圍文字一同換行並接收自身的點擊事件，因此圖示、圖片或小型按鈕皆可嵌入行內。進行選取時會以該片段的 `Content` 取代其位置進行複製，若為空則略過。預設情況下，其第一行文字會使用實際字型的度量指標與周圍的基線對齊。`Em` 畫布會提供自身的基線，而沒有基線的檢視則會以其底邊對齊。設定 `Style.VerticalAlign` 可選擇其他對齊方式。

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## 字型

`FontFamily` 可指定單一字型家族或 CSS 清單（`Inter, "Noto Sans SC", sans-serif`），清單中的每個字型會用於補齊前面字型所缺乏的字元，且清單中的每個字型家族皆會套用指定的字重與樣式。在 macOS 與 iOS 上，一行的高度與基線取決於該行字元實際使用的字型，因此完全以後續字型家族排版的行會採用該字型家族的高度與基線，而瀏覽器則會以第一個字型家族的高度與基線作為下限。`FontWeight`、`FontStyle`、`FontVariant`、`LineHeight`、`LineHeightMultiple`、`LetterSpacing` 與 `LetterSpacingEm` 用於設定字型外觀與間距，`core.FontMonospace` 則為平台的等寬字型家族。

`Language` 為文字的 BCP 47 語言標籤，在文字、片段與輸入欄位上會如同上述欄位一樣向下繼承。漢字會依該語言的字形繪製，因此即使系統偏好中文，設定為 `ja` 的 `直` 仍會採用日文字形；而韓文（`ko`）會在詞語之間斷行，如同 CSS 的 `word-break: keep-all`。未設定該標籤的文字與輸入欄位會使用應用程式目前顯示的語言，並在語言變更時隨之更新。可在檢視上設定以套用至整個子樹，或在 `Span` 上設定以套用至單一片段：

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

請將應用程式的字型打包於 `resources/` 目錄下並在第一個檢視之前註冊，或在應用程式執行期間註冊字型的位元組資料：

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // a TrueType or OpenType face
```

在字型載入前已完成版面配置的文字會重新準備，而畫布也會重新執行 `Draw`，使其字形與文字量測使用已載入的字型。`cx.Env().FontScale()` 代表使用者的文字縮放比例，除非設定了 `NoFontScaling`，否則文字皆會跟隨此比例；`MaxFontSizeMultiplier` 可為其設定上限。

## 選取與量測

`SelectableOn` 讓使用者能夠選取並複製文字，而在 `View` 上設定 `Selectable` 則可讓其中的文字如網頁一般依文件順序一併選取。`SelectionColor` 用於設定反白選取的顏色。當使用者移動選取範圍時，`OnTextSelection` 會接收一個 `core.TextSelection`：包含其純文字 `Text`、`Markdown` 與 `HTML`（保留該區域內任何 `TextView` 與樣式化文字經解析後的行內與區塊結構，包含部分片段與連結）、每個文字對應的 `Ranges`，以及所觸及文字在檢視中的 `Bounds`，可用於複製為 Markdown 或 HTML、建立單字清單或翻譯選取的字詞；`desktop.LookUp` 可在系統字典中顯示這些字詞（參閱[服務](services.md#開啟連結與檔案)）。`cx.MeasureText` 可依指定字型量測字串大小，以便讓欄位貼合其文字寬度；呼叫它的反應式函式會在可用字型變更時更新，而 `OnTextLayout` 會在每次版面配置後回報各行資訊。

## 文件

`base/textview` 使用相同的基礎元件來顯示 Markdown 或 HTML：包含標題、段落、清單、表格、程式碼、註腳、提示框、圖片、公式與連結。

```go
textview.TextView{
	Markdown: "## NSGo\n\nNative views from **Go**.",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

公式（`$…$`、`$$…$$`）預設會直接顯示其 TeX 原始碼，直到應用程式傳入來自 `base/formula` 的 `RenderMath: formula.RenderMath` 才會進行排版；它會以周圍文字的大小與顏色排版公式，以應用程式的語言朗讀，並在複製時輸出為 TeX；包含字型在內，這會為桌面應用程式增加約 6.5 MB、為網頁應用程式增加約 3 MB（經 gzip 壓縮後）的體積。`formula.View` 可在任何其他位置以 TeX 或 Typst 顯示公式；其 `OnHit` 回呼會回報 Untie 原始碼區段、最近的游標位置以及重疊的診斷資訊，供公式編輯器使用。在 watchOS 上，公式使用相同的畫布繪製路徑；連結該套件會增加約 6.1 MB 的執行檔大小（包含 1.4 MB 的字型）。在手錶模擬器上實測載入並解析該字型（含其持有的位元組副本）約會保留 3.9 MB 的 Go 堆積記憶體。模擬器的量測結果既不能代表實體手錶的記憶體上限，也不能代表其可用餘裕；請務必在實際執行應用程式的手錶上進行量測。若設定的字型無法涵蓋文字片段（`\text{}` 或 Typst 字串），將會改用系統字型，包含 CJK 與其他書寫系統；明確指定的 `formula.Fonts.Fallbacks` 會優先使用。

對於分段抵達的原始碼，請將 `SourceFunc` 傳入 `formula.View` 並設定 `Streaming: true`，或將 `StreamingFunc` 繫結至應用程式的接收狀態。這會在有效內容持續排版的同時，隱藏可恢復的錯誤與預留位置墨跡（placeholder ink）。接收完成後請清除該旗標，以顯示剩餘的錯誤。在 `TextView` 中，請提供回傳該 `formula.View` 的 `RenderMath` 函式，並傳入所提供的 `Math.TeX` 與 `Math.Display`；其 `StreamingFunc` 即可讀取相同的接收狀態。

區塊公式（display formula）會在安全的位置（關係符號與運算子處）依據區塊在內距與邊框內的寬度自動折行，同時維持文字的字型大小。調整大小或變更文字大小時會重新排布各行。無法折行的部分仍可橫向捲動；行內公式則維持其原始大小。

`mermaid` 程式碼區塊預設會顯示其原始碼，直到應用程式傳入來自 `base/diagram` 的 `RenderDiagram: diagram.RenderDiagram` 才會透過 limu 繪製為原生圖表：包含流程圖、循序圖、類別圖、狀態圖與實體關聯圖、甘特圖、圓餅圖、心智圖、時間軸等，並採用平台的字型與應用程式目前的淺色或深色外觀。帶有連結的圖表項目在點擊時會透過應用程式的 URL 服務開啟，而指標停留於項目上方時會顯示其 Mermaid 工具提示。圖表會依據文字的可用寬度進行版面配置，在維持標籤大小的同時將較寬的流程圖轉為由上而下排列；仍無法容納的圖表則支援橫向捲動。複製時會取得其原始碼，而仍在串流接收中的區塊在完成前會以程式碼形式顯示。`diagram.View` 可在任何其他位置顯示圖表，並隨其容器寬度變化而調整。其 `Icons` 會將諸如 `"fa:fa-car"` 等 Mermaid 名稱對應至 `*core.SVG` 文件，包含來自 `icons/lucide` 的圖示；找不到的名稱不會留下行內空隙。圖片節點會透過共用的畫布圖片載入器載入 HTTP(S)、檔案或 `asset:` 來源，當設定 `constraint: "on"` 時會等比縮放並置中，否則會拉伸填滿。Mermaid 會在載入前過濾圖片位址（包含拒絕 `data:` URL）。若要在文字檢視中提供圖示，請將 `RenderDiagram` 設為回傳 `diagram.View{Source: code.Code, Icons: icons}` 的回呼函式。

若要將你的 `ui` 調色盤套用至圖表，請從 `RenderDiagram` 回呼回傳帶有 `ThemeFunc` 的 `diagram.View`。該函式會在畫布繪製時讀取顏色，因此當調色盤或子樹外觀變更時，既有的檢視也會隨之重新著色。保留為零值的顏色將沿用圖表的預設值。

```go
func drawDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ThemeFunc: func() *diagram.Theme {
		return &diagram.Theme{
			Text: colors.Text(), Surface: colors.Surface(), Border: colors.Border(),
			Line: colors.Muted(), Accent: colors.Accent(), Critical: colors.Danger(),
		}
	}}
}
// ui.TextView{Markdown: source, RenderDiagram: drawDiagram}
```

`textview.Parse` 可讀取 Markdown，而 `textview.ParseHTML` 可讀取 HTML，將其轉換為應用程式同樣可以建構的區塊。`Options.Streaming` 會修復分段抵達的文件，補齊結尾尚未閉合的語法並暫緩顯示語意需視後續字元而定的最後一行（例如單獨出現的清單標記），使串流回覆不會閃現出原始標記文字；而 `StreamingCaret` 會在結尾繪製輸入游標，適合用於邊串流邊輸出的回覆內容。`FadeIn` 會讓文件新增的字元依序淡入，且不會落後抵達太久，使成批抵達的回覆讀起來依然流暢。

`textview.Search` 可在透過 `Search` 欄位或其上層的 `textview.ProvideSearch` 加入其中的多個檢視間搜尋文字。`SetQuery`、`Open` 與 `Close` 可從尋找列控制搜尋，`Count` 與 `Selected` 回報目前的搜尋狀態，而 `Next` 與 `Previous` 則依閱讀順序依序切換符合項目。各檢視會以 `PartSearch` 標記每個符合項目，並以 `PartSearchSelected` 標記目前選中的項目，供應用程式樣式著色並將其捲動至可視範圍內，同時不干擾使用者原本的文字選取。

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## 下一步

- [圖片與圖示](images.md)：圖片與 SVG，也能放入文字中。
- [樣式與主題](styles.md)：可繼承的顏色與文字欄位。
- [版面配置](layout.md)：文字如何決定其寬度。

## 富文本編輯

`ui/richeditor.View` 會以主題外觀顯示 `base/richeditor.Session`；`base/richeditor.View` 則搭配應用程式提供的樣式實作其行為。在 iOS 與 Android 上，長按與連點兩下皆可選取單字；拖曳選取控制柄可跨區塊延伸選取範圍。原生編輯選單會視選取狀態與剪貼簿內容提供「複製」與「全選」，並在工作階段可編輯時提供「剪下」與「貼上」。`Session.SetEditable` 也會更新已開啟的選單。可使用基礎檢視上的 `Styles.SelectionHandle` 或 `Classes.SelectionHandle` 來設定選取控制柄的樣式。

若要實作自訂文字控制項，請聚焦一個 `core.Input` 並呼叫 `Ref.OpenEditMenu(bounds, actions)`，傳入以表面點（surface points）為單位的邊界以及 `core.EditCut`、`EditCopy`、`EditPaste` 與 `EditSelectAll` 的位元遮罩。在 iOS 與 Android 上，`Input.OnEditAction` 會接收「剪下」、「複製」與「全選」動作；「貼上」則使用 `Input.OnPaste`。呼叫 `Ref.CloseEditMenu` 即可關閉選單。
