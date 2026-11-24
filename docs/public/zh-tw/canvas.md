---
title: 畫布與著色器
description: 在引擎上記錄 2D 繪圖，並使用 GPU 片段著色器填滿檢視。
group: 核心概念
order: 59
---

## 畫布

`core.Canvas` 在引擎上記錄 2D 繪圖指令並交由平台重播，運作方式如同網頁的 `<canvas>`。當 `Draw` 讀取的訊號、畫布尺寸或可用字型變更時，`Draw` 會重新執行，平台則會在主執行緒上繪製該記錄。透過 `cx.RegisterFont` 載入的字型在可用時也會重新觸發 `Draw`，同步更新文字與依據 `TextMetrics` 計算的幾何形狀。

若要讓畫布尺寸以繼承字型的 em 為單位，請使用 `Em` 或 `EmFunc`。對於高度取決於寬度的內容，`MeasureEm(width, fontSize)` 會在指定可用寬度（單位為點，無邊界時為 `core.Auto`）與解析後字型大小下回傳 `EmBox`。其內部讀取操作會被追蹤，且它也會在版面配置階段執行，因此請勿在其中寫入訊號。`Ref.ContentBox()` 則會追蹤已掛載檢視在內距與邊框內的版面配置方塊（以自身座標系表示）。

```go
core.Canvas{
	Style: core.Style{Height: core.Pt(220)},
	Draw: func(d *core.Drawing, size core.Size) {
		d.SetFillColor(core.RGB(0xf1f5f9)).FillRect(0, 0, float32(size.Width), float32(size.Height))
		d.SetStrokeColor(core.RGB(0x007aff)).SetLineWidth(2).SetLineCap(core.CapRound)
		d.BeginPath().MoveTo(24, 160).LineTo(90, 80).LineTo(160, 120).LineTo(230, 40).Stroke()
	},
}
```

繪圖記錄介面遵循網頁的 `CanvasRenderingContext2D`：`Save` 與 `Restore`、變換矩陣、線段/曲線/矩形與圓弧路徑、純色與漸層填滿與描邊、裁剪、虛線、陰影、文字及圖片。`core.ParsePath` 可從 SVG 路徑資料建立 `core.Path`，而 `FillFromView` 與 `StrokeFromView` 會採用畫布檢視所繼承的 `Fill` 與 `Stroke` 樣式，讓圖表能自動配合應用程式的主題。

在網頁上，位於已縮放 `core.Scroll` 內或套用了縮放 `Style.Transform` / `AnimatedStyle` 的畫布，會在動作停止後依累積縮放比例重新繪製，最高可達螢幕像素比的四倍。若縮放後的點陣圖將超過 4096 × 4096 像素，畫布只會繪製目前可見的區域並在捲動或平移時更新，藉此在不重新執行 `Draw` 的前提下保持線條與字形銳利。Apple、Android、Windows 與 Linux 同樣會在捲動縮放與樣式變換下保持畫布與文字清晰銳利。

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## 文字、圖片與繪圖部分

`MeasureText`、`TextMetrics` 與 `SetTextAlign` 會使用平台的文字引擎進行排版，因此標籤能與應用程式其餘部分使用相同字型。`DrawImage`、`DrawImageIn` 與 `DrawImageFrom` 負責繪製圖片；`DrawSVG` 與 `DrawSVGIn` 可將解析後的 `*core.SVG` 文件（`core.ParseSVG`）依原始尺寸繪製或適配填滿矩形——其中的 `<text>` 與 `<tspan>` 元素會由平台自身的文字引擎量測與繪製，且 `currentColor` 會採用繪圖目前的填滿色；`OnPress` 則可使用 `IsPointInPath` 與 `IsPointInStroke` 對上一次繪圖進行點擊測試。原生後端會快取最近繪製的 64 個未釘選（unkept）畫布圖片來源；呼叫 `release := cx.KeepImage(src)` 可將精靈圖集或長期使用的紋理釘選於快取中以免被逐出，直到呼叫 `release()` 或 `cx` 被處置為止。

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

縮放後的點陣圖預設使用平滑取樣（smooth sampling）。若要繪製像素藝術，請在 `DrawImage`、`DrawImageFrom` 或 `DrawAtlas` 之前呼叫 `d.SetImageSmoothing(false)`，或在 TSX 中設定 `ctx.imageSmoothingEnabled = false`。`Save` 與 `Restore` 會連同其餘繪圖狀態一併保存此設定。Apple 平台、Windows、Linux、Android 與網頁皆支援最近鄰取樣（nearest-neighbor sampling）；HarmonyOS 與小程式也會套用此設定，但尚未在實機上驗證其輸出。

`d.DrawVertices(image, vertices)` 用於繪製帶紋理的網格。每連續三個 `core.CanvasVertex` 構成一個三角形。`Position` 以畫布點為單位，`TexCoord` 則以圖片點為單位（與 `DrawImageFrom` 相同）。每個頂點的 `Color` 會與紋理相乘並在三角形上內插漸變，其零值表示維持紋理原色。更新頂點並重新記錄繪圖即可使網格產生變形。圖片可以是任意 `ImageSource`，包含像素緩衝區或 `OffscreenImage` 的 `Source()`；Android 畫布與小程式目前尚未支援繪製離屏圖片。`SetImageSmoothing(false)` 同樣適用於網格。在網頁上，任意網格需要使用行內（inline）或同源圖片。小程式透過網頁播放器繪製網格，但尚未在微信開發者工具或實機上驗證。HarmonyOS 會裁剪每個三角形並透過仿射變換繪製圖片；與 `DrawImage` 相同，它支援編碼圖片與像素緩衝區但不支援離屏圖片，且只會套用每個三角形的平均頂點透明度而不套用顏色著色。它僅在不透明三角形之間隱藏接縫，且尚未在實機上驗證。

`DrawTintedImage`、`DrawTintedImageIn` 與 `DrawTintedImageFrom` 會在合成前將圖片的 sRGB 與 Alpha 通道乘上指定顏色。在預設的 source-over 混合模式下，透明像素會保持背景可見。在 TSX 中請使用 `drawTintedImage(image, tint, ...args)`，座標參數格式與 `drawImage` 相同。網頁上的跨來源圖片必須允許匿名 CORS 請求，才能對其像素進行著色。HarmonyOS 支援對位元組、應用程式資源與像素緩衝區的圖片著色，但尚未在實機上驗證；且當地目前尚未支援離屏圖片。

使用 `d.DrawAtlas(source, rects, xforms, alphas)` 依序從單一圖片繪製多個精靈圖。每個 `core.Bounds` 在圖片自身點座標系中選取一個圖塊。對應的 `core.RSXform` 將圖塊左上角映射至 `TX, TY`，其中 `SCos` 與 `SSin` 分別為縮放比例乘上旋轉角的餘弦與正弦值。例如，`core.RSXform{SCos: 0.5, TX: 40, TY: 20}` 會在 (40, 20) 處以一半大小繪製一個 32 點的圖塊。傳入 `nil` 表示使用完整不透明度，或傳入會與目前繪圖不透明度相乘的係數陣列。

`Drawing.Node` 可為繪圖中的某個部分（例如圖表的長條或地圖的區域）命名，並設定邊界、角色、標籤與選取狀態。輔助技術會將這些部分作為畫布的子節點朗讀，測試也能透過標籤、文字或測試 ID 找到它們；點擊該部分等同於點擊其中心點。

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## 離屏圖片

`cx.NewOffscreenImage(size, scale, draw)` 會將 `core.Drawing` 預先繪製一次並保留為離屏圖片，供後續影格透過 `DrawImage`、`DrawImageIn`、`DrawImageFrom`、`DrawAtlas` 或 `core.Image` 繪製。傳入 `scale <= 0` 會自動使用目前的顯示器像素比。僅當呼叫 `Invalidate()`、`Resize(size, scale)` 或註冊新字型時才會重新繪製，並在 `cx` 被處置或呼叫 `Release()` 時自動釋放。

```go
tiles := cx.NewOffscreenImage(core.Size{Width: 512, Height: 512}, 0, func(d *core.Drawing) {
	// Drawn once into the offscreen image until tiles.Invalidate() is called.
	for y := range 16 {
		for x := range 16 {
			d.SetFillColor(tileColor(x, y)).FillRect(float32(x*32), float32(y*32), 32, 32)
		}
	}
})

core.Canvas{
	Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)},
	Draw: func(d *core.Drawing, _ core.Size) {
		// Reuses the cached surface in a single DrawImage operation per frame.
		d.DrawImage(tiles.Source(), -cameraX.Get(), -cameraY.Get())
	},
}
```

## 動畫參數

`Drawing.Bind` 會將上一個繪圖操作的參數繫結至動畫節點，平台會在畫面時脈上僅重繪該部分，無需觸發元件提交。`core.ArgX`、`ArgY`、`ArgR`、`ArgA` 與 `ArgColor` 分別代表不同參數；macOS、iOS 與 Linux 僅重繪變更的區域，Windows 會重新記錄繪圖，而 Android 與網頁則會重繪整個畫布。

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## 著色器

`core.Shader` 使用 GPU 為每個像素執行片段著色器來填滿其方塊區域。`core.CompileShader` 接受以 Go 語法編寫並包含 `Fragment` 函式的程式，而 `core.CompileWGSL` 則接受 WGSL。`Uniforms` 用於設定程式的匯出變數，`Images` 則可提供最多四張紋理圖片。

```go
var program = core.MustCompileShader(`
package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	v := 0.5 + 0.5*sin(pos.x/40)
	return Tint * vec4(v, v, v, 1)
}
`)

core.Shader{
	Program:  program,
	Uniforms: core.Uniforms{"Tint": core.RGB(0x007aff)},
	Style:    core.Style{Height: core.Pt(220), CornerRadius: 16},
}
```

Go 著色器會在 `dst` 接收像素中心座標，在 `pos` 接收以點為單位的相同位置，並可使用 GLSL 的內建函式；`rectSize`、`pixelScale`、`image0At` 至 `image3At` 及其尺寸則是 NSGo 額外提供的項目。WGSL 程式須編寫 `fn shade(uv: vec2f) -> vec4f` 並讀取 `nsh.time` 與 `nsh.size`；`Paused` 可暫停其時脈。沒有 GPU 著色器路徑的平台則不會繪製任何內容。

`core.ShaderEffect` 會透過 WGSL 繪製其子元件，而 `core.ScreenEffect` 則覆蓋整個介面。`core.ShaderTransition` 與 `core.ScreenTransition` 會在 `Value` 變更前後取樣畫面。`core.TransitionShader` 則將此功能應用於導覽與對話框轉場。在 Android 上，擷取檢視以套用效果需要 Android 10 以上版本以及支援外部紋理的 OpenGL ES 3.0。`core.Canvas` 與 `core.Shader` 皆接受 `Label`，且其子元件會繪製在它們上方。

## 下一步

- [圖片與圖示](images.md)：畫布所能繪製的圖片。
- [動畫](animation.md)：畫布所繫結的數值。
- [無障礙設計](accessibility.md)：畫布向外公開的各個部分。
