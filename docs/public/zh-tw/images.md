---
title: 圖片與圖示
description: 顯示來自 URL、應用程式資源、位元組或應用程式繪製像素的圖片，並繪製 Lucide 圖示與 SVG。
group: 核心概念
order: 54
---

## 顯示圖片

`core.Image` 用於顯示點陣圖或 SVG：來源可以是 URL、檔案、應用程式資源中的 `asset:` 路徑，或是應用程式持有的位元組資料。平台會在主執行緒之外進行載入、解碼與快取，再由主執行緒繪製結果。

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` 會填滿方框並裁切超出部分；`core.Contain` 會完整容納整張圖片；`core.Center` 維持圖片原始大小；而 `core.ScaleDown` 除非圖片大於方框，否則維持原始大小。`BlurRadius` 以點（point）為單位對圖片套用模糊，`TintColor` 會替不透明像素重新上色，而 `ObjectPosition` 則決定圖片在方框內的位置。

縮放後的點陣圖預設使用平滑取樣。在 Go 中設定 `Sampling: core.ImageSamplingNearest`，或在 TSX 中設定 `sampling="nearest"`，即可保留像素藝術與像素緩衝區的銳利邊緣。Apple 平台、Windows、Linux、Android 與網頁皆支援此設定；HarmonyOS 與小程式也會套用該設定，但其輸出結果尚未於實體裝置上驗證。若要在瀏覽器中使用 CSS `pixelated` 獲得精確的像素邊緣，請將每個來源像素縮放為整數倍的裝置像素。

`Source` 會宣告版面配置所使用的尺寸：指定其中一個維度即可推導另一個維度，而未設定尺寸的圖片在樣式指定其大小之前皆為 0 × 0。`core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` 可顯示透過 `go:embed` 嵌入的位元組資料，而 `Size` 則為 SVG 指定其固有尺寸（intrinsic size）。

搭配 [`desktop.WindowPointerPainted`](services.md) 使用時，在 macOS 與 Windows 上設定 `HitTestAlpha: 0.5` 可讓指標穿透不透明度低於一半的像素。TSX 可透過 `hitTestAlpha` 設定相同的門檻值。

## 應用程式繪製的畫格

`Pixels` 用於顯示應用程式自行繪製的 `*image.RGBA`（例如影片播放器或模擬器的畫格），完全不需經過編碼、載入或解碼。只要將每個畫格寫入同一個緩衝區並遞增 `Version`：檢視就會在不觸發載入事件且沒有淡入效果的情況下顯示該畫格，同時保留 Alpha 通道，並在 macOS、iOS、tvOS、watchOS、Android、Windows 與 Linux 上就地切換，中間不會出現空白畫格。若未指定 `Size`，則會將其像素直接以點為單位計量。

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // 將下一個畫格寫入 frame.Pix
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo 會在帶有新 `Version` 的提交（commit）執行時複製像素，因此應用程式可以在下一次 `RequestAnimationFrame` 回呼中寫入下一個畫格；運行於獨立 goroutine 上的解碼器則會寫入第二個緩衝區並發送交換通知。相同的 `Version` 不會被重複繪製。`cx.ImageSize` 會立即以緩衝區大小回應像素來源的尺寸。`core.Canvas` 在所有平台上都能以相同的複製方式透過 `DrawImage` 繪製像素；小程式則需要微信 2D 畫布（WeChat 2D canvas）。`core.Shader` 的 `Images` 也以相同方式接收像素，因此著色器可在每個支援繪製著色器的平台上對應用程式寫入的畫格（例如模擬器畫面）進行濾鏡處理。

Apple 平台、Android、Windows、Linux 與網頁會直接更新像素圖片，微信小程式則使用 2D 畫布。HarmonyOS 會為每個版本建立一個 `PixelMap`（已於裝置上驗證），且其資料複製須跨越字串橋接層，因此每畫格重寫的大型緩衝區在該平台上的執行幀率會遠低於 60 fps（詳見[像素緩衝區](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)）。

## 擷取檢視畫面

傳入已掛載檢視的 `core.Ref` 呼叫 `cx.Capture(&ref, done)`，或傳入 `nil` 呼叫 `cx.Capture(nil, done)` 來擷取目前的繪圖表面。回呼函式會在引擎 goroutine 上收到符合繪圖表面像素縮放比例的 PNG 位元組資料，或收到一個錯誤。使用 `core.ImageSource{Data: png}` 即可顯示擷取結果。若要分享該圖片，請將位元組寫入應用程式快取目錄中的 `.png` 檔案，並將其路徑傳入 `core.ShareOptions.Files`（參閱[分享檔案](services.md#開啟連結與檔案)）。

擷取功能會取得檢視邊界內可見的繪圖表面像素，支援 macOS、iOS、tvOS、Windows、Linux、Android 與網頁。Android 會擷取 Activity 的視窗。網頁擷取會將應用程式的 DOM 繪製到畫布中（包含 NSGo 著色器輸出），但會略過內嵌框架、無法讀取的圖片以及部分 CSS 效果。

## 資源與縮放倍率

位於 `resources/` 目錄下的檔案會隨應用程式一同封裝發佈。`core.Src("asset:images/cover.png")` 可載入這類資源，而 `Scales: core.ScalesOf(1, 2, 3)` 會依據螢幕的像素比例自動選擇 `cover.png`、`cover@2x.png` 或 `cover@3x.png`。

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` 會在圖片載入期間顯示替代來源，而 `FadeDuration` 則會將載入完成的圖片淡入覆蓋其上；設為 0 表示由平台自行決定，設為 1 奈秒則會立即顯示。`OnLoadStart`、`OnProgress`、`OnLoad`、`OnError` 與 `OnLoadEnd` 會追蹤載入過程。`cx.ImageSize` 可在不顯示圖片的情況下讀取來源的像素尺寸，`cx.PrefetchImage` 可將圖片預先下載至快取，`cx.SetImageCacheLimit` 可設定記憶體中已編碼圖片的位元組上限（預設為 `core.DefaultImageCacheLimit`，即 32 MiB；負值會限制為 0），而 `cx.KeepImage` 則可將圖集或紋理釘選於快取中避免被逐出，直到呼叫其回傳的 `release` 函式或其作用域銷毀為止。此上限套用於每個快取，不包含已釘選項目、正在載入的項目以及目前的繪製通道；它不會限制已解碼像素、處理程序記憶體或瀏覽器快取。設為零則會停用未釘選項目的保留。原生後端會將遠端圖片保存在各自的磁碟快取中。

## 動畫與向量圖片

GIF 與 WebP 動畫會在檢視中播放，每個畫格依照各自的延遲時間顯示，並依檔案設定循環播放。SVG 文件會作為圖片載入並在任何尺寸下保持銳利；單色 SVG 還可使用 `TintColor` 進行著色。

## 圖示

`core.Icon` 會以繼承的文字顏色繪製 SVG，除非樣式另有指定尺寸，否則其高度與字型大小相同。其方框會保留 SVG 的長寬比，因此正方形方框不會將圖示拉伸變形。

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` 為每個 Lucide 圖示提供對應的存取子，並以 PascalCase 命名（例如 `lucide.ArrowUpRight`）。除非透過 `Label` 為圖示命名，否則對輔助技術而言它僅為裝飾性元素。`ui.Icon` 功能相同但帶有 `ui` 的預設樣式，而 `core.SVG` 與 `core.SVGView` 則可繪製應用程式自行建構的路徑。

## 帶有樣式的圖片

`ui.Image` 是置於圓角框中的圖片元件，載入期間會顯示佔位符，載入失敗時則會顯示淡色圖片圖示。除非樣式同時指定了寬與高，否則它會維持來源的長寬比（若沒有來源尺寸則預設為 3:2）。

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## 下一步

- [文字](text.md)：在行內作為 span 嵌入圖片。
- [樣式與主題](styles.md)：圓角、陰影與著色。
- [清單與捲動](lists.md)：可重複回收利用的圖片列。
