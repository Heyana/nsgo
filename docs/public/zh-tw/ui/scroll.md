---
title: 捲動
description: 平台原生的捲動檢視，支援分頁、吸附對齊、縮放與重新整理。
group: 元件
order: 39
---

`core.Scroll` 是平台原生的捲動檢視。其子項目預設以直欄排列，或加上 `Horizontal` 後改為橫列排列，並會在內容捲動時填滿其容器。

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

內距應設定在內容上，而非捲動檢視本身，如此捲軸才會貼齊邊緣。除非樣式另有指定，否則捲動檢視會縮小以適應其父層大小。若必須在同層元素縮小時仍維持其請求尺寸，請設定 `NoShrink`。

## 行為

`Behavior` 可選擇其捲動方式：`Paging`、`SnapToInterval` 或搭配 `SnapToAlignment` 的 `SnapToOffsets`、`DecelerationRate`、`NoBounces`、`ContentInset` 以及鍵盤規則。

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` 會隱藏平台的捲軸，而 `Bar` 則可由應用程式自行繪製捲軸。`MinZoomScale` 與 `MaxZoomScale` 允許雙指縮放，`NoPinchZoom` 則會忽略縮放手勢。`CenterContent` 可將小於可視區域的內容置中顯示。

## 重新整理與事件

`OnRefresh` 可讓檢視支援下拉重新整理；在作業執行期間請將 `Refreshing` 設為真。`PullValue` 會追蹤內容被下拉的距離，供應用程式自訂的指示器使用，而 `Behavior.RefreshIndicator` 則設定其高度。

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` 每次介面更新最多回報一次偏移量，或依 `ScrollEventThrottle` 的間隔回報；`Stale` 會標記來自平台套用程式碼捲動之前的事件。`OnScrollDragStart`、`OnScrollDragEnd`、`OnMomentumStart`、`OnMomentumEnd` 與 `OnScrollEnd` 分別標示使用者手勢的各個階段起訖。

`OffsetValues` 會在主執行緒上追蹤偏移量，可用於製作無須每幀提交即可收合的標頭。`Ref` 可從程式碼中以 `ScrollTo`、`ScrollToEnd` 與 `ZoomTo` 進行捲動，而內部任何檢視的參照則可透過 `Reveal` 將自己捲動至可視範圍內，並同時捲動其外層的所有捲動檢視：

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox` 則可改為將檢視內部的特定方框捲動至可視範圍內，例如來自文字 `TextRangeBoxes` 的某個字詞方框，而 `Margin` 可在最外層捲動檢視的邊緣為上方覆蓋的列保留淨空空間。

當鍵盤將焦點移動至某個檢視時，該檢視會自動捲動至可視範圍內。

## 版面配置

`Horizontal` 可進行水平捲動。`Behavior.ContentInset` 會在內容周圍加入隨內容一同捲動的空間。若頁面需要填滿安全區域，請將 `PaddingEnv` 設定於內容上，而非捲動檢視上。

## 無障礙支援

`Label` 用於命名捲動區域，`Role` 則說明其角色類型（例如 list box）。平台原生的捲動手勢、鍵盤捲動與螢幕閱讀器捲動皆會保留；`Busy` 可回報仍在載入中的檢視。

## 平台特性

每個後端皆使用平台的捲動檢視，因此慣性滑動、回彈、捲軸與觸控捲動皆為原生體驗。邊緣效果各不相同：macOS 與 iOS 會呈現橡皮筋回彈、Android 會拉伸變形、Windows 與 Linux 會直接停止，而 Web 則跟隨瀏覽器行為。`EdgeEffects` 與 `EdgeBar` 可在應用程式需要的位置於內容上方加入邊緣條。在 iOS 與 Android 手機及平板上，內容溢出的垂直與水平捲動檢視會在作用軸向旁顯示可觸控拖曳的捲動控制柄（scroll handle），用於在完整內容範圍內快速捲動；無須捲動即可完整顯示的短捲動檢視、縮放中的檢視、`Behavior.NoScroll`、`HideIndicator` 以及使用自訂 `Bar` 或 `base.SetScrollbarTheme` 的檢視則會將其隱藏。在測試中，`nsgotest.App.ScrollHandle` 會回報其在可視區域的邊界，並可直接以 `DragAt` 或 `Touch` 進行拖曳。在 Windows 上，覆蓋式指示器僅會在滾輪、觸控、拖曳捲軸及其慣性滑動時出現，不會因應用程式發起的 `ScrollTo`、`ScrollBy`、顯示定位（reveal）、內縮或動畫而顯示。

## 相關資源

- [清單與捲動](../lists.md)：捲動與清單的完整指南。
- [清單](list.md)：可回收重用的資料列。
- [版面配置](../layout.md)：內距與安全區域。
- 展示畫廊（gallery）的 Scroll 與 Lists 頁面。
