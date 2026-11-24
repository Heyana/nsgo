---
title: 元件與狀態
description: 元素、元件、訊號與樣式。
group: 核心概念
order: 50
---

## 元素

畫面是一棵由元素（element）組成的樹，而元素即為 Go 的 struct 值。`core` 套件提供基礎原語：`View`、`Text`、`Image`、`Scroll`、`Input` 等。`ui` 套件提供應用程式使用的控制項，例如 `Button`、`Select`、`DatePicker` 與 `EmojiPicker`，並具備跟隨主題的外觀。（`base` 套件則包含相同但無樣式的控制項，供[元件作者](authoring/index.md)使用。）

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` 用於顯示網頁內容；空的 `WebSource` 會載入 `about:blank` 並回報常見的載入與導覽事件。將 `SourceFunc` 設為空來源或呼叫 `Ref.Load(core.WebSource{})` 可清除先前的頁面。當頁面自身腳本執行前的腳本需跟隨訊號變化時，請設定 `InjectedJavaScriptBeforeContentFunc`：當它改變時，檢視會以新腳本重新載入其來源，取代目前顯示的頁面——即使該頁面是由使用者或 `Ref.Load` 導覽而來亦然。設定 `UserAgentSuffix`（TSX 中為 `userAgentSuffix`）可在平台預設 User-Agent 或自訂 `UserAgent` 之後附加標記，例如 `Version/26.0 Safari/605.1.15`；在 macOS 與 iOS 上，附加於平台預設 User-Agent 之後的後綴僅在檢視首次建立時生效，而 HarmonyOS 與網頁則維持各自的 User-Agent。掛上 `core.Ref` 即可呼叫 `Reload`、`GoBack` 與 `GoForward`。在原生目標平台上，這些指令會讓應用程式自身的 HTML 與資料來源免受 `core.AccessWebSchemes` 限制；建立新導覽的頁面仍須遵守存取政策。

在 tvOS 上，`cx.Env().HasWebView()` 為 false。掛載 `WebView` 時會直接透過 `OnError` 回報 `*core.WebError`，而不會載入 WebKit；請為該目標平台提供對應的應用程式內容。

在 macOS、iOS、Windows 與 Linux 上，`OnNavigationStateChange` 也會回報同文件內的 URL、瀏覽歷程與標題變更。

將 `WebView.Media` 設為 `core.WebMicrophone`、`core.WebCamera` 或兩者，即可允許頁面發起 `getUserMedia` 請求。請在 `apple.info` 中宣告使用說明，並在 `android.permissions` 中宣告 `RECORD_AUDIO` 或 `CAMERA`；系統仍會向使用者詢問授權。在 Android 上，若系統權限提示開啟期間應用程式移除了某個請求的裝置、離開頁面或關閉檢視，待決的請求將會被拒絕。發生此變更後，頁面必須重新請求存取權限。

`WebView.Muted` 與 `MutedFunc`（TSX 中為 `muted`）在 Windows 與 Linux 上會將頁面所有音訊靜音，包含 Web Audio 與內嵌框架。macOS 12 與 iOS 15 及以上版本則會在靜音期間暫停媒體播放。其他宿主維持平常的播放行為。更新會立即套用至目前頁面並延續至後續導覽。

在 TSX 中，變更 `injectedJavaScriptBeforeContentLoaded` 會如同 `InjectedJavaScriptBeforeContentFunc` 一般在同一個 web view 內以新腳本重新載入來源；變更 `injectedJavaScript` 則會建立新的 web view 並重新載入來源，正如變更 `userAgent`、`userAgentSuffix` 或 `mobile` 時一樣。在 HarmonyOS 上，變更任一注入腳本都會建立新的 web view 並開啟新的原生瀏覽歷程。在 Windows 與 HarmonyOS 上，每個注入腳本皆於主框架的區塊（block）內執行，因此其頂層 `let`、`const` 與 `class` 宣告會維持在該區塊區域內；若要讓頁面看到全域變數，請指派至 `window`。

設定 `Mobile`（TSX 中為 `mobile`）可在 Windows、macOS 與 Linux 上以手機版視角顯示頁面，適用於像 YouTube Shorts 這類僅在手機上支援垂直滑動切換影片的網站：除非已設定 `UserAgent`，否則頁面會取得手機的 User-Agent 與無懸停（hover）的觸控螢幕環境，且滑鼠拖曳或在不可捲動區域上的滾輪／觸控板捲動會以手指滑動的形式傳達給頁面。Windows 會像 Chrome 的裝置模式一樣模擬手機；macOS 與 Linux 則透過腳本模擬，因此頁面樣式表仍會偵測到滑鼠，且觸控事件屬於不受信任（untrusted）事件，頁面可能會予以忽略。iOS、Android、HarmonyOS、網頁與小程式則依原本樣貌顯示頁面。

在 macOS、iOS、Windows 與 Linux 上，`OnOpenWindow` 會接收網頁發出的開新視窗請求。該檢視不會建立彈出視窗，請根據請求的 `URL` 與 `UserInitiated` 決定是要在同一個檢視中透過 `Ref.Load` 導覽、改於他處開啟 URL，或是忽略該請求。受限制的檢視會將遭拒絕的請求送往 `OnError`。

設定 `OnDownload` 可在 macOS、iOS、Linux、Windows 與 Android 上儲存網頁下載的檔案。下載內容會儲存於應用程式資料目錄內的 `downloads/<id>/<file name>`，並回報開始、完成（含位元組數）或失敗；未設定 `OnDownload` 或受限制的檢視會取消下載。

`core.Video` 透過平台的播放器播放檔案或 URL。在 macOS、iOS 與 tvOS 上，針對有限長度的影片剪輯，可將 `Video.AudioSource` 設為另一個包含音訊的檔案或 URL，播放器的聲音便會來自該來源。畫面與聲音會同時啟動，且 `Ref.Seek` 會將兩者同步移至同一時間軸位置。播放長度以影片為準，較短的音訊在結束後會維持靜音。

透明影片剪輯可透出其後方內容：在 macOS 上使用含 Alpha 通道的 `.mov` HEVC 檔案（iOS 尚未驗證），在 Linux 上則使用含 Alpha 通道的 VP9 WebM 並搭配 GStreamer 的 codecalpha 外掛（`gstreamer1.0-plugins-bad`，Linux 套件可在 `nsgo.yaml` 中將其列為建議相依套件）。在 macOS、iOS、Windows 與 Linux 上，若本機剪輯的 Alpha 通道無法保留（例如 Apple 平台上的 WebM、Linux 上的 HEVC，以及 Windows 機器上的任何格式——在 Media Foundation 的 Alpha 輸出經實際驗證前皆然），`OnError` 會回報錯誤而非將影片顯示於不透明方框內：該錯誤為 `*core.MediaError`，其 `Reason` 為 `core.MediaErrorAlphaUnsupported`，讓應用程式能改以自行解碼該剪輯。Android 與網頁，以及各平台上的遠端 URL，則依解碼器輸出直接播放該類剪輯，畫面可能為不透明。

`OnPause` 與 `OnPlay` 在 Apple、Android、Windows、Linux 與網頁上會跟隨內建控制項的動作。當由控制項觸發暫停時，`OnPause` 會傳入 `VideoPauseControls`。`Paused` 會在來源開啟或數值變更時套用。音量、播放速率與版面配置變更會維持原生的暫停或播放狀態，包括在 `Paused` 仍為 true 時透過控制項按下播放的情況。若應用程式顯示自訂的播放按鈕，請將 `PausedFunc` 繫結至你的狀態，並在上述回呼中更新它。

`Rate` 用於設定播放速度，而 `RateFunc` 可在播放期間動態更改速度。請使用 `OnLoad` 傳回的 `VideoInfo.RateChangeable` 來決定是否顯示調速控制項。在 Linux 上，若安裝了 GstPlay 與 GStreamer 的 `scaletempo` 濾鏡，即可變更可搜尋（seekable）影片的播放速度；備援播放器則使用 1×。

設定 `Video.Headers` 可在 Android 上以及 Linux、macOS、iOS 與 tvOS 上的直接 HTTP 檔案（如 MP4 或 MOV）傳送 HTTP 請求標頭，例如 `Referer`、`User-Agent` 或 `Cookie`。

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

影片在掛載時會複製該 map，並在 `SourceFunc` 提供新 URL 時使用相同的標頭。在 Apple 平台上，請使用已知位元組長度且支援 range 請求的來源以利搜尋時間軸。播放器會在同源重新導向時繼續傳送設定好的標頭。相同的標頭在 Apple 平台上亦適用於直接 HTTP 的 `AudioSource` 檔案。以 `.m3u` 或 `.m3u8` 結尾的播放清單 URL 則使用原生載入器。

在 Linux 上，請安裝 GstPlay 以及 GStreamer 的 `scaletempo` 與 `souphttpsrc` 外掛。若你設定了 `User-Agent`，或你的 `souphttpsrc` 缺乏 `location-trusted` 屬性，請直接使用最終的媒體 URL。具備該屬性且使用預設 User-Agent 時，GStreamer 會在同源重新導向時保留應用程式標頭。應用程式的標頭僅套用至來源檔案，自適應串流媒體片段則各自發出請求。

## 元件

元件是一個接收作用域（scope）並回傳元素的函式。它只會執行一次；之後的變化完全由訊號驅動。

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

帶有屬性（props）的元件是一個具有 `Render` 方法的型別：

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` 可透過 `NoClose` 省略關閉按鈕、新增 `Trailing` 節點，並透過 `OnPress` 關閉快訊通知（toast）。`Toasts.NoClose` 則會隱藏整個堆疊中的所有關閉按鈕。

`ui.Toasts.Style` 與 `StyleFunc` 可自訂每個快訊通知的表面樣式，包含背景、邊框、圓角與內距。堆疊的樣式會合併覆蓋於主題之上，接著依序套用每個 `ToastContent.Style` 與 `StyleFunc`。每個 `StyleFunc` 會在對應的 `Style` 之後執行，並於其讀取的訊號變更時更新。這些樣式中的 `Color` 會為標題著色，並以降低的不透明度為描述文字著色，確保文字在深色或彩色表面上依然清晰可讀。進場、退場與滑動階段仍會套用其各自的動態樣式。在 TSX 中，請設定 `Toasts` 的 `style` 屬性，並在呼叫 `show` 顯示單一快訊通知時傳入 `style` 或 `styleFunc`。

## 訊號

訊號（signal）保存一個數值。當元素呼叫的函式（例如 `ContentFunc`）內部讀取了該訊號，畫面的該部分便會訂閱它，且日後數值變更時僅會更新該部分。

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

存放整個切片（slice）的訊號在變更時會更新所有讀取它的列。`core.NewCollection` 依鍵值存放項目，每個項目各自放在獨立的訊號中，因此每一列只追蹤自己的項目，更改單一項目時只會重跑該列：

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

其他狀態建構方式同樣建立在訊號之上：

- `core.NewSelector` 判斷某個鍵是否為目前選取項目，且僅重新執行選取離開與選取到達的列。
- `core.NewHistory` 保存帶有復原（undo）與重做（redo）功能的數值，並提供 `CanUndo` 與 `CanRedo` 訊號供「編輯」選單使用。
- `core.NewPersisted` 將具備型別的數值以版本化 JSON 形式儲存在應用程式儲存空間中，並負責遷移舊版本寫入的資料。
- `core.From` 追蹤圖表外部的資料來源，例如在另一個 goroutine 上送出資料的訂閱。

## 樣式

`core.Style` 遵循 CSS 模型：flexbox 與 grid、內距與外距、圓角、邊框、陰影、漸層、變換（transform），以及如 `Active` 等狀態。元素也可透過 `Class` 使用 Tailwind 類別：

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

主題（theme）為 `ui` 的元件提供淺色與深色模式下的外觀。

## 桌面應用程式

`nsgo.Run(Root)` 顯示單一根檢視。桌面應用程式也可以改由自己開啟想要顯示的內容：`nsgo.Run(App)` 只執行一次 `func App(app *core.Scope)`，接著便透過 `desktop` 開啟視窗、面板、系統列圖示與全域快捷鍵，並與其共用訊號。這適用於 macOS、Windows 與 Linux；在 macOS 上，`examples/tray` 不會出現在 Dock 中，而是常駐於選單列。`nsgo.Run(App, nsgo.StartHidden())` 會在不顯示的情況下開啟主視窗，讓系統列應用程式或系統於登入時啟動的應用程式（`desktop.LaunchedAtLogin`）不會閃爍視窗；應用程式可於需要時以 `app.MainWindow().Show()` 顯示視窗。手機與網頁則在啟動時直接顯示應用程式。

`desktop.SetWindowLevel` 可讓視窗維持在其他視窗之上（如計時器或桌面寵物）或之下，而 `desktop.SetWindowFrame` 則移動並調整其大小。在視窗作用域中，`desktop.CanSetWindowLevel` 與 `desktop.CanPlaceWindows` 會指出這些呼叫是否生效：在 macOS、Windows 與 X11 下的 Linux 兩者皆可生效，但在一般 Wayland 頂層視窗中兩者皆不適用，因為該環境由合成器（compositor）負責堆疊與擺放視窗。

以 `desktop.WindowOptions{Transparent: true}` 開啟的視窗，或以 `nsgo.WindowTransparent()` 開啟的主視窗，沒有背景與外框，因此應用程式未繪製任何內容之處都會透出桌面，適合桌面寵物或小工具；設有 `Desktop.WindowDrag` 的檢視可拖曳該視窗。`NoShadow` 會移除系統陰影，`SkipTaskbar` 會讓視窗不出現在 Windows 工作列與 Alt+Tab 及 X11 工作清單中，而 `desktop.SetWindowTransparent`、`SetWindowShadow` 與 `SetSkipTaskbar` 則可在之後動態修改這些設定。Windows 僅能在最初以透明開啟的視窗中變更透明度（該視窗亦無陰影），而 Wayland 沒有讓視窗自工作清單隱藏的提示機制。在視窗作用域中，`desktop.CanMakeWindowsTransparent` 與 `desktop.CanSkipTaskbar` 會指出這些功能是否生效：透明效果在 Windows 上需要 DirectComposition、在 Linux 上需要合成器（純 X11 無合成器時不支援），而在 macOS 上 `desktop.SetDockVisible` 可將應用程式自 Dock 隱藏。

## 終端機

匯入 `base/terminal` 並使用 `terminal.New(terminal.Options{Conn: conn})` 為遠端串流建立工作階段，或在 macOS、Linux 與 Windows 上設定 `Command` 執行本機程式。渲染 `terminal.View{Terminal: session}` 並在工作階段結束時呼叫 `Close`。`Options.ImageLimit` 限制 Kitty 圖形快取的位元組上限：設為 0 時使用 64 MiB，負數則停用圖片。程式可傳送 RGB、RGBA 或 PNG 圖片以及 Unicode 預留位置配置（placeholder placement）。

呼叫 `Search(text)` 可在畫面與捲回緩衝區中標示符合項目，`SearchNext` 與 `SearchPrevious` 用於選取項目，`EndSearch` 則清除標示。`Options.OnSearch` 會回報 `SearchStatus`；`Theme.Search`、`SearchText`、`SearchSelected` 與 `SearchSelectedText` 用於設定符合項目的顏色。會發出 OSC 133 標記的 Shell 支援在目前作用中的指令輸入範圍內透過 Alt/Option+點擊移動游標。NSGo 不會主動安裝 Shell 整合腳本。
