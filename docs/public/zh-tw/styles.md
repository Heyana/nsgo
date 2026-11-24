---
title: 樣式與主題
description: 型別化樣式、Tailwind 類別、狀態樣式，以及淺色與深色主題。
group: 核心概念
order: 52
---

## 樣式

`core.Style` 是代表方框版面配置、外觀與文字的單一 Go 數值。每個欄位的零值皆代表未設定，因此樣式可以組合：`Merge` 僅會覆蓋較後樣式有設定的欄位，而子元素會繼承父元素所設定的文字欄位。若要在已指定長度的樣式上將該長度設為零，請在 `Padding`、`Margin`、`Borders`、`BorderWidth`、`CornerRadius` 或間距（gap）中使用 `core.Zero`，如同對顏色使用 `core.Transparent` 一般：`Padding: core.All(core.Zero)`。

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

樣式支援 CSS 的顏色、圓角、邊框、外框線、陰影、漸層、濾鏡、背景模糊、遮罩、混合模式、3D 變換與轉場效果。`core.RGB`、`core.RGBA` 與 `core.ParseColor` 用於建立顏色，`core.LinearGradient` 及其相關函式用於建立漸層，`core.Ring` 用於建立 Tailwind 的環形外框（ring），而 `core.AllCorners` 則可分別為四個角落指定圓角半徑。`BorderCurveContinuous` 會像 iOS 一樣將圓角平順地融入邊緣。方框的陰影與外框線會保留在其 `Overflow` 裁切範圍之外，除非符合[版面配置](layout.md#定位)中說明的情況。背景濾鏡會像 CSS 的 `backdrop-filter` 一樣改變檢視背後的內容：`Blur` 會依指定半徑將其模糊，而色彩濾鏡則會改變其顏色。在 iOS 上，未搭配色彩濾鏡的模糊效果會繪製 `View.IOS.BackdropMaterial` 所指定的系統材質；若模糊指定了 `core.BackdropUltraThin` 以外的材質，在 iOS 上會繪製該材質，在 macOS 上則繪製 AppKit 中最相近的材質，並略過色彩濾鏡。Linux 與 Android 不會繪製任何背景濾鏡。`Style.GlassEffect`（`core.GlassRegular` 或 `core.GlassClear`，在 TSX 中為 `glassEffect`）會在 macOS 26 與 iOS 26 上以 Liquid Glass 取代背景模糊進行繪製（在 Windows 上則使用 NSGo 的玻璃著色器；請為玻璃檢視同時設定模糊效果作為其他平台的備援）。設定 `Style.GlassInteractive`（在 TSX 中為 `glassInteractive`）會在 macOS 27（`NSGlassEffectView.effectIsInteractive`）與 iOS 26（`UIGlassEffect.isInteractive`）上啟用系統為 Liquid Glass 提供的互動按壓回饋，同時維持 `OnPress` 與可按壓子元件的正常運作。

`Transform` 會隨著檢視一併移動指標命中目標：按壓、懸停與手勢皆從檢視實際繪製的位置觸發，且回報的指標座標會還原該變換。超出父檢視範圍的已變換子元素仍然可以觸及，除非遭到裁切或捲動檢視區隱藏。小程式目前尚未還原由其算繪層執行的關鍵影格變換（見[開發藍圖](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)）。

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## 狀態

檢視的外觀可隨著狀態改變。`Hover`、`Focus`、`FocusWithin`、`Active`、`Selected`、`Checked`、`Disabled` 以及各類 `aria-*` 狀態皆為 `*Style` 值，會在該狀態成立期間合併覆蓋於其餘樣式之上。應用程式透過函式設定 `Selected`、`Checked`、`Disabled` 與 `Expanded`，而平台則負責回報按壓與焦點。

`Active` 與 `group-active:` 會在檢視被按住期間套用，這需要檢視設有按壓處理函式或有 `Responder` 持有觸控事件。僅為一般檢視設定 `Active`、`active:` 或 `group` 樣式並不會使其變得可按壓或成為鍵盤 Tab 停駐點；若需讓檢視本身能取得焦點，請設定 `Focusable`。

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

元件中的某個部分可以承接外層控制項的狀態，而無需自行回報：`PartStates` 會提供 `Checked`、`Selected`、`Disabled` 等狀態，因此核取方塊的勾選標記在螢幕閱讀器中也能與核取方塊保持同步。

## 類別

`View`、`Text`、`Span`、`Input`、`Image`、`Scroll` 與 `Canvas` 上的 `Class` 接受 Tailwind CSS 4 類別，而 `base` 元件的每個部分在樣式之外也都有對應的類別。各種工具類別（utility）、變體（variant）與任意值（arbitrary value）皆依 Tailwind 預設主題與調色盤的定義運作。

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

類別會合併於樣式下方，因此 `Style` 中的欄位會優先勝出。透過 `ClassFunc`，類別可以隨著變動的狀態更新，而狀態變體（如 `hover:bg-zinc-100`、`checked:opacity-100`）會轉換為樣式對應的狀態。`group` 與 `group-hover:` 會依據父元素的狀態為子元素設定樣式，`first:` 與 `last:` 會跟隨子元素的排列位置，而 `aria-checked:`、`data-open:` 與 `focus-within:` 則跟隨控制項顯示的狀態。`pointer-fine:` 與 `pointer-coarse:` 會透過 `cx.Env().Pointer()` 對應 CSS 的 `pointer` 媒體特性，分別為滑鼠或觸控螢幕設定子樹樣式（在網頁上對應瀏覽器的 `pointer` 查詢；在桌面裝置與 Mac 上為 fine；在手機、平板與手錶上為 coarse；在電視上則兩者皆非），並可與 `dark:`、`hover:` 等其他變體組合使用，讓應用程式明確決定觸控或指標裝置的尺寸，而非由 `core` 隱式更改。

將控制項標記為 `peer` 即可依據其狀態為後續的同層兄弟元素設定樣式，例如在設有 `Class: "peer"` 的核取方塊後方接上設有 `Class: "peer-checked:text-accent peer-disabled:opacity-50"` 的標籤。`peer` 只會為同一父元素下位於其後的同層兄弟元素（包含 `Show` 與 `Each` 之中的項目）套用樣式，絕不會影響前面的兄弟元素或其內部的檢視。使用 `peer/field` 搭配 `peer-checked/field:` 可為特定群組命名，而 `peer-data-invalid:` 則對應控制項的布林 `invalid` 資料狀態。標記與狀態皆會響應式更新。標記檢視並不會使其成為 Tab 停駐點或接收按壓事件，因此 `peer-active:` 僅會跟隨本身可接收按壓的 `peer`（例如按鈕）。

`has-[>svg]:px-3` 會在父元素擁有直接子圖示時變更其內距；`[&_svg]:size-4 [&_svg]:shrink-0` 則會設定其後代圖示的尺寸。`svg` 會選取 `core.Icon` 與 `SVGView`，包含位於條件區域內的項目。當子元素、類別或狀態變更時，這些選取器會隨之更新。`hover:[&_svg]` 跟隨父元素的懸停狀態；`[&_svg]:hover` 則跟隨每個圖示自身的懸停狀態。選取器中的 `active` 如同 `peer-active:` 一樣，僅會跟隨可接收按壓的檢視（例如按鈕）。符合條件的選取器會先依權重（specificity）比較，再依變體順序決定；除非類別標記為 important，否則行內樣式優先勝出。

選取器在所有目標平台上皆會比對 NSGo 元素。它們支援選取器清單、子代（`>`）、後代（`_`）、相鄰同層（`+`）與後續同層（`~`）組合器、類別、`data-*` 存在性／相等比對、角色（role）、支援的布林無障礙狀態，以及 `:is`、`:where`、`:not`、`:has` 與子元素位置。可用的型別包括 `view`/`div`、`text`/`span`（整個文字節點）、`scroll`、`input`、`textarea`、`image`/`img`、`canvas`、`svg`、`video`、`webview` 與 `native`。請在每個任意選取器選項中使用一個頂層 `&`；若省略 `&`，該選取器會測試元素自身。僅限瀏覽器的選取器、偽元素、at 規則、跳脫字元以及函式內部的明確 `&` 皆視為未知選取器；這些選取器也不會觸及後端實作檢視或個別的文字片段（span）。

`Span.Class` 或 `Screen.HeaderClass` 中的 `peer-*` 變體沒有可跟隨的同層兄弟元素。它們會被忽略，並依每個類別與情境記錄一次警告；請改將文字片段的 `peer` 變體放在其外層的 `Text` 上。

主題的 `Tailwind` 欄位可用來為預設主題擴充顏色、中斷點與工具類別；若調色盤中找不到某個顏色名稱，則會從節點當前外觀所顯示的主題中讀取 `Palette` 角色（如 `bg-accent`、`text-text-muted/70`）或主題自訂顏色。

## 主題

`core.Theme` 代表兼顧兩種外觀的完整視覺主題：包含淺色與深色各一組 `Palette`、圓角半徑與字型家族。調色盤的角色包含 `Background`、`Surface`、`Text`、`TextMuted`、`Border`、`Accent`、`AccentText`、`Danger`、`Warning`、`Success`、`Selection`、`Focus` 與 `Scrollbar`；`Colors` 則存放套件庫額外新增的角色，例如 `inset` 與 `raised`。應用程式在設定其他主題前預設顯示的 `core.DefaultTheme()` 是帶有藍色強調色與 `ui` 角色的中性主題，而 `ui.WarmDuskTheme()` 則是暖色調調色盤。

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` 會為該作用域及其底下建置的所有內容套用主題，因此側邊欄或面板也能擁有各自的主題；傳入 nil 主題則會沿用上層的主題。`core.ParseTheme` 可讀取 YAML 或 JSON 主題檔案，或 shadcn/ui 主題檔案的 CSS 變數，包含角色顏色、圓角半徑與字型家族。`core.DefaultTheme`（應用程式在設定主題前所顯示的主題）與 `ui.WarmDuskTheme` 會回傳可直接修改的主題。

`cx.SetAppearance` 會強制整個應用程式切換為淺色或深色外觀，`cx.SetSubtreeAppearance` 則僅針對子樹進行設定，而 `core.AppearanceSystem` 會恢復跟隨系統外觀。`cx.Dark()` 回傳子樹目前顯示的外觀是否為深色，而 `cx.Palette()` 則回傳套用嵌入宿主覆蓋設定後的調色盤顏色。

當沒有任何上層元素設定 `Style.Color` 時，文字與 `currentColor` 圖示會使用該子樹的 `Palette.Text`，並就地跟隨主題、外觀與宿主權杖（token）的變更更新。明確指定的顏色（包含 `core.Transparent`）仍會繼續向下繼承。

## 元件中的顏色

元件會在建置時及其 `StyleFunc` 中讀取顏色，因此當主題或外觀變更時，畫面會就地更新而無需重建元件。`ui.ColorsOf(cx)` 提供 `ui` 所使用的角色名稱，`ui.TintsOf(cx)` 提供其半透明填色，而 `ui.FinOf(cx)` 則提供 `ui` 的分層表面質感（finish）。

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

當 `StyleFunc` 內部讀取的狀態改變時，它會重新執行，因此其中的 `cx.Dark()` 與 `cx.Palette()` 也會跟隨主題更新。這同樣適用於 `ClassFunc`、文字片段的 `StyleFunc` 以及元件的各個 `StyleFunc` 欄位。

## 游標

`Style.Cursor` 用於設定具名指標游標（例如 `core.CursorText`、`core.CursorColResize` 等）；請將可按壓元件保留為 `core.CursorAuto`，以維持平台預設的箭頭游標。針對桌面平台與網頁上的繪圖工具、遊戲或自訂畫布指標，`Style.CursorImage` 接受一個 `*core.CustomCursor`，其中包含 1x `Image`、選用的 HiDPI `Image2x`（在 `PixelRatio >= 1.5` 時自動選用）、限制在圖片範圍內的邏輯點 `Hotspot`，以及 `Fallback` 具名游標。可透過 `cx.Env().SupportsCustomCursor()` 檢查目前目標平台是否顯示自訂游標圖片；觸控平台會忽略它們。

## 下一步

- [版面配置](layout.md)：方框如何配置位置。
- [文字](text.md)：字型、文字片段與裝飾樣式。
- [動畫](animation.md)：樣式之間的轉場效果。
