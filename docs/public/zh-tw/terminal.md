---
title: 終端機
description: 在終端機網格中執行程式或顯示遠端工作階段。
group: 核心概念
order: 63
---

`base/terminal` 提供終端機工作階段（`Terminal`）與其 `View`。桌面平台執行本機程式；iOS 與瀏覽器則使用 `Options.Conn`，即連線至遠端程式的 `io.ReadWriteCloser`。應用程式擁有該工作階段，並在結束時將其關閉。

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` 可選擇預設的方塊、豎線或底線游標。`CursorBlink` 會啟用預設閃爍效果；零值則維持恆亮不閃爍。程式可透過 DECSCUSR 或 DEC mode 12 覆蓋游標形狀與閃爍狀態。未取得焦點時游標會呈現空心且不閃爍；輸入法組字期間與系統啟用減少動態效果時也會保持不閃爍。閃爍由平台的動畫時鐘驅動，不會觸發引擎重繪終端機文字。

`View.Font`、`Theme` 與 `DarkTheme` 用於設定字型與調色盤。網格會自行繪製 Unicode 製表符、方塊字元以及 Powerline 分隔符號 U+E0B0–U+E0B7，並將相鄰儲存格對齊至裝置像素。其餘文字則使用平台字型與其備援字型。

`MinimumContrast` 會提高一般文字與其實際儲存格背景之間的對比度，包含反轉色彩、淡化文字與選取範圍。數值 1–21 採用 WCAG 的對比度比例；零值則保留原始顏色。圖形與表情符號會維持原有色彩。即使啟用 `Transparent`，對比度計算仍以終端機設定的背景色為準。

`Padding` 以點（point）為單位在網格外部保留間距。`PaddingColor` 決定該區域的背景色：

| 數值 | 行為 |
| --- | --- |
| `PaddingBackground` | 終端機背景色，包含 OSC 11 變更 |
| `PaddingExtend` | 最鄰近的儲存格背景色；在主畫面（primary screen）上，若相鄰提示字元列、Powerline 列或包含預設背景儲存格的列，則抑制上下邊緣的延伸 |
| `PaddingExtendAlways` | 延伸所有邊緣，不套用上述例外 |

TSX 應用程式會匯入 `tsx/terminal`，透過其副作用將 `Terminal` 註冊至 `nsgo/base`。其 props 包含 `cursorBlink`、`minimumContrast` 與 `paddingColor`；其中 `paddingColor` 接受 `background`、`extend` 與 `extend-always`。外觀相關的 props 可在工作階段持續執行期間動態變更。

## 在瀏覽器中

應用程式會內嵌鎖定版本的 libghostty-vt WebAssembly 模組，並執行相同的終端機檢視與輸入路徑。建置應用程式不需要安裝 Zig、原生函式庫，也不需在執行期間額外下載。

請將瀏覽器的 WebSocket 轉接為 `io.ReadWriteCloser`：以二進位位元組傳送資料，並將收到的訊息串接或拆分為位元組串流。驗證、重新連線以及任何調整大小或控制訊息皆由應用程式與其伺服器負責；NSGo 僅處理終端機位元組。[瀏覽器驗收測試範例](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go)展示了包含關閉處理的完整位元組串流轉接器。

## 動作與縮放

`View.Keybinds` 可透過 `Keybind{Key, Action, Amount}` 覆蓋預設快速鍵。按鍵名稱與 `Scope.OnKey` 相同，例如 `mod+=` 或 `ctrl+shift+c`。較後面的項目會覆蓋前面項目。當此終端機的輸入區取得焦點且沒有輸入法正在組字時，按鍵繫結才會觸發。

可用的動作包含 `CopyToClipboard`、`PasteFromClipboard`、`SelectAll`、`ClearScreen`、`IncreaseFontSize`、`DecreaseFontSize`、`ResetFontSize`、`SetFontSize` 與 `Ignore`。視窗與分頁動作屬於應用程式的快速鍵配置。`Ignore` 會消耗按鍵而不將其傳送給程式。

預設情況下，`mod+=` 與 `mod+shift+=` 會將字型放大 1 點，`mod+-` 會縮小 1 點，而 `mod+0` 會重設為 `View.Font.Size`（未設定時為 13）。`Amount` 用於設定放大或縮小的步進值（為 0 時預設為 1），或是 `SetFontSize` 的目標大小。字型大小會限制在 1–255 點之間；非有限數值的請求將被忽略。縮放會保留工作階段與外層檢視，更新網格的行數與列數，並調整程式的終端機尺寸。

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` 會追蹤目前字型大小以供顯示用的控制項讀取；`Ref.SetFontSize(size)` 與 `Ref.Perform(action, amount)` 如同事件處理函式一樣在應用程式引擎上執行。`Perform` 會回報該動作是否可用。尚未掛載的 ref 會回傳 0 大小並拒絕執行動作。

## 情境選單

次要點擊（右鍵點擊）會開啟無樣式的 `base.ContextMenu`，其中包含已在地化的複製、貼上、全選與清除項目。複製需要有已選取的文字。清除會移除提示字元上方的內容，並在替代畫面（alternate screen）上停用。觸控螢幕會以長按方式呈現情境選單。可使用 `MenuClass`/`MenuStyle` 為選單面板設定樣式，並以 `MenuItemClass`/`MenuItemStyle` 為選單項目設定樣式。

將 `RightClickAction` 設為 `RightClickPaste`、`RightClickCopy`、`RightClickCopyOrPaste`（有選取文字時複製，否則貼上）或 `RightClickIgnore`。設為忽略時會將事件留給外層應用程式選單處理；當你提供呼叫 ref 動作的樣式化 `ui.ContextMenu` 時，請使用此選項。

匯入 `tsx/terminal` 即可將 `<Terminal>` 註冊至 `nsgo/base`。其響應式 props `keybinds` 與 `rightClickAction` 會在保留工作階段的情況下更新檢視。動作值使用 Ghostty 的命名，例如 `increase_font_size` 與 `copy_to_clipboard`；右鍵動作值則為 `context-menu`、`paste`、`copy`、`copy-or-paste` 與 `ignore`。選單樣式 props 使用 Go 欄位名稱的小駝峰命名法（lower camel case）。

## 滑鼠回報

啟用滑鼠追蹤的程式會收到其所請求的按下、放開、拖曳與懸停事件，並由 libghostty-vt 依程式選定的協定進行編碼。按住 Shift 可繞過點擊與拖曳擷取以進行文字選取。`View.MouseReporting` 是選用的響應式 `func() bool`，為 nil 時預設為 true；回傳 false 可在程式請求滑鼠擷取時仍保留滑鼠選取與原生捲回捲動行為。在啟用滑鼠回報期間，程式對右鍵點擊的處理優先於外層的情境選單。

在替代畫面上，若啟用替代捲動模式（alternate scroll mode）且程式未請求滑鼠追蹤，垂直滾輪輸入會依一般或應用程式游標鍵模式轉換為方向鍵。`MouseScrollMultiplier{Precision: 1, Discrete: 3}` 設定觸控板與滾輪的預設回報次數；零值會採用上述預設值，正數則會限制在 0.01–10,000 之間。未滿一步的小數輸入會持續累積，直到足夠構成完整的一步為止。一般捲回緩衝區則保留平台的原生捲動與慣性效果。

匯入 `tsx/terminal` 即可將 `<Terminal>` 註冊至 `nsgo/base`。其 `mouseReporting` prop 預設為 true；`mouseScrollPrecision` 與 `mouseScrollDiscrete` 用於設定捲動倍率。
