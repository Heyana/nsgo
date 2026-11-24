---
title: 版面配置
description: Flexbox、網格、尺寸與定位，以及在版面配置中處理安全區域與鍵盤。
group: 核心概念
order: 51
---

## 方盒

每個元素都是由引擎負責排版配置的方盒。預設行為與 React Native 相同：直欄（column）flexbox、子元素除非主動伸展或收縮否則維持原本尺寸、項目沿交叉軸拉滿，且尺寸包含內距（padding）與邊框（border）。

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`、`Row`、`ColumnReverse` 與 `RowReverse` 用於設定主軸；`Wrap` 與 `WrapReverse` 讓子元素換行排列。`JustifyContent` 控制子元素沿主軸的對齊位置，而 `AlignItems`（或單一子元素上的 `AlignSelf`）則控制沿交叉軸的對齊方式。`Gap`、`RowGap` 與 `ColumnGap` 用於分隔元素，`style` 的內距與外距則使用 `core.All`、`core.Symmetric` 或 `core.Edges`。

## 尺寸

`Width`、`Height`、`MinWidth`、`MaxWidth` 與 `FlexBasis` 接受代表點數（point）的 `core.Pt` 或代表父元素百分比的 `core.Pct`。`FlexGrow` 用於分配剩餘空間，而 `FlexShrink` 讓方盒在空間不足時讓出部分空間；兩者皆未設定的方盒會保持原本尺寸。`AspectRatio` 為寬度除以高度的比例。`AutoMargin` 會佔滿指定側邊的剩餘空間，因此指定其中一側會將方盒推向另一側。

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

在未指定寬度或高度之前，方盒的大小由其內容決定。`core.MinContent`、`core.MaxContent` 與 `core.FitContent` 對應 CSS 所定義的尺寸概念：例如文字的 min-content 寬度就是其最寬單字的寬度。位於橫列（row）中的文字必須設定 `FlexShrink: 1` 才會收縮並換行；若要讓它佔滿剩餘空間，請加上 `FlexGrow: 1`。未設定的最小寬度預設即為 0（與 React Native 相同），因此 `MinWidth: core.Pt(0)` 不會改變任何行為，且當橫列比最寬單字更窄時，正在收縮的文字會在單字內部斷行。`MinWidth: core.AutoMinimum` 則會將其最小寬度限制在最寬單字的寬度，如同 CSS 的自動最小值（automatic minimum）。

填滿視窗的畫面需要設定 `FlexGrow: 1` 與 `FlexShrink: 1`，位於它與應該可捲動的清單或捲動視圖之間的每一層方盒也是如此。若僅設定 `FlexGrow`，方盒會隨內容高度一同擴張，導致長清單將方盒撐出視窗範圍，而不是在內部捲動。

## 網格

`DisplayGrid` 使用 CSS Grid 排列子元素。`GridTemplateColumns` 與 `GridTemplateRows` 接受 CSS 軌道（track）語法，而 `GridRow` 與 `GridColumn` 則依照網格線或跨越數（span）放置項目。

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` 會依可用空間建立盡可能多的欄。`GridFlow` 決定網格填充的順序，而 `JustifyItems`、`JustifySelf` 與 `AlignItems` 則控制項目在儲存格內的對齊位置。

## 定位

`Relative` 為預設值，讓方盒留在版面流程中，並透過 `Top`、`Right`、`Bottom` 與 `Left` 進行位移。`Absolute` 會將方盒移出流程，並相對於其最近的已定位祖先元素進行定位。`Fixed` 會將其固定於根視圖或所屬的模態視圖（modal）中，不隨捲動而移動；`Sticky` 則將其釘選在最近的捲動視圖中其父元素的範圍內：透過 `Top` 從頂部算起，或在 `Top` 為 auto 時透過 `Bottom` 從底部算起（水平方向則使用 `Left` 與 `Right`）。設定 `Position: core.Sticky, Bottom: core.Pt(0)` 的頁尾，在長內容捲動時會固定於底部，而在內容較短時則自然接續在內容之後。`ZIndex` 用於調整同層兄弟方盒之間的堆疊順序。

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` 會讓方盒留在流程中並忽略其位移設定，因此絕對定位的子元素會繼續往上尋找更上層的定位祖先。`Overflow` 會將子元素裁切至方盒邊界內，而 `DisplayNone` 則會將方盒移出版面配置。

方盒自身的外陰影與外框線（outline）會維持在裁切範圍之外，這點與 CSS 一致，適用於 Android、Windows、Linux、Web 與 Apple 平台，且支援各種視圖類型，包含不同圓角半徑的轉角以及縮放後的捲動內容。在實體 iPhone 上完成驗證之前，`ui.Dialog` 的 `Bare` 仍會將裁切套用在具有相同圓角並依邊框寬度內縮的內層視圖上，以確保所有目標平台都能正確保留這些效果：

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## 排版方向與環境

`LayoutDirection` 可覆蓋應用程式的預設方向，將子樹以由左至右或由右至左的方式排版：`DirectionRTL` 與 `DirectionLTR` 用於明確設定方向，預設則繼承上層。外距、內距、邊框與位移中的 left 與 right 會隨著排版方向互換，因此它們如同 React Native 一樣代表起點與終點。`PhysicalEdges` 與 `PhysicalBorders` 則會將指定的側邊固定在原本的實體位置。`WritingDirection` 用於決定混合方向文字段落的排列順序，而 `WritingAuto` 則依第一個強方向字元決定。

`PaddingEnv` 與 `MarginEnv` 會將環境內縮值加到對應側邊上。`core.EnvSafeArea` 代表該邊緣的安全區域內縮值，而 `core.EnvKeyboard` 則代表軟體鍵盤的高度，讓內容在鍵盤移動動畫期間始終保持在鍵盤上方。

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` 代表所有四個邊緣，而 `cx.Env().SafeArea()` 與 `cx.Env().KeyboardHeight()` 可在程式碼中讀取這些數值。

在摺疊或雙螢幕手機上，`cx.Env().Geometry()` 會列出摺痕或轉軸將視窗劃分出的各個區域（各自包含落在其中的安全區域），以及區域之間的摺疊位置；其 `State` 與 `Angle` 會回報裝置展開的程度（`core.HingeClosed`、`HingeHalfOpen` 或 `HingeOpen`），即使視窗未被摺痕跨越（例如封面螢幕或完全攤平的螢幕）亦然。其他裝置則會回報單一區域與 `HingeUnknown`。在 iOS 27.1 上，`cx.Env().VerticalBarEdge()` 會回報系統為垂直工具列選擇的邊緣（`core.VerticalBarLeading`、`core.VerticalBarTrailing` 或 `core.VerticalBarUnspecified`），當 `Edge` 設為 `mobile.TabBarAutomatic` 時，`mobile.TabBar` 與 `ui.TabShell` 會跟隨該設定。引擎不會隱式地依據幾何資訊自動配置版面：應用程式需自行選擇各個窗格。

## 響應式類別

`Class` 接受 Tailwind CSS 類別：`flex`、`flex-col`、`flex-wrap`、`items-center`、`justify-between`、`gap-3`、`p-4`、`w-full`、`grid`、`grid-cols-3`、`col-span-2` 等。斷點會跟隨視窗尺寸，而顯示外觀、排版方向與減少動態效果（reduced motion）則跟隨系統環境：

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`、`md:`、`lg:`、`xl:` 與 `2xl:` 分別在寬度達到 40、48、64、80 與 96 rem 起套用；`max-md:` 則套用於低於對應斷點的寬度。`portrait:`、`dark:`、`rtl:`、`motion-reduce:`、`contrast-more:`、`pointer-fine:`、`pointer-coarse:`、`ios:`、`android:` 與 `web:` 則會依系統環境套用。類別會合併在 `Style` 之下，而狀態類別（例如 `hover:`）會轉換為狀態樣式，因此兩者可以混用。

## 下一步

- [樣式與主題](styles.md)：色彩、圓角、陰影與子樹的外觀。
- [文字](text.md)：文字如何計算尺寸與換行。
- [清單與捲動](lists.md)：可捲動內容與可重複利用的列（row）。
