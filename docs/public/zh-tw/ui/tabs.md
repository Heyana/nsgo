---
title: 分頁
description: 分頁清單、分段控制項與系統分頁列。
group: 元件
order: 40
---

`ui.Tabs[T]` 可作為分段控制項或分頁清單從少數幾個數值中擇一；應用程式會在其旁顯示所選數值對應的內容。分頁清單是一個 Tab 停駐點：使用方向鍵即可在各分頁之間移動。

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## 變體與尺寸

`Variant` 可設為 `TabsSegmented`（預設值，在凹陷軌道中顯示浮起的滑塊）或 `TabsUnderline`（在已選取分頁下方顯示強調線）。底線變體可設定 `IndicatorColor`，並使用 `IndicatorFit: TabsIndicatorContent` 讓指示器貼齊標籤與圖示寬度，而非佔滿整個分頁；在 TSX 中對應的屬性為 `indicatorColor` 與 `indicatorFit="content"`。`Size` 可設為 `SizeDefault`、`SizeSmall` 或 `SizeLarge`，而 `Equal` 會讓每個分頁維持相同寬度，而非依各自標籤調整大小。

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

每個選項皆可附帶 `Icon`、`Disabled` 函式、`Content` 節點與 `TestID`。`Content` 會取代標籤並繼承分頁的文字樣式。

當分頁需要跟隨訊號變化時，請使用 `OptionsFunc`。NSGo 會依數值原地更新各列，並在該數值仍存在於選項中時保留已選取的分頁與鍵盤焦點；若該數值已消失，則會維持未選取狀態，直到應用程式變更它為止。

## 分頁列

`ui.TabShell` 會在任何視窗寬度下將頂層區段維持在分頁列旁——預設位於底部，或當 `Edge`（或 `EdgeFunc`）設為 `mobile.TabBarLeading`、`mobile.TabBarTrailing` 或 `mobile.TabBarAutomatic`（會跟隨 `cx.Env().VerticalBarEdge()`）時，沿著前緣或後緣垂直排列。當每個分頁皆提供 `SystemIcon` 或 SVG `Icon` 時，iOS 的任意邊緣與 Android 的底部邊緣皆會使用系統分頁列；在 iOS 上指定 SF Symbol 或在 Android 上指定 drawable 的 `SystemIcon` 會優先顯示，否則便顯示 SVG。其他情況下則會使用提供的 SVG 圖示自行繪製分頁列。自行佔據空間的分頁列會主動避開該邊緣的安全區域，因此其旁區段（包含 `base.Nav` 頁面）在該邊緣上的 `core.EnvSafeArea` 為 0；在 iOS 浮動底部系統分頁列下，請使用 `ui.TabBarInset` 讓內容保持足夠間距。

設定 `ShellTab.TextFunc` 可讓分頁標題跟隨語系或應用程式狀態變化，設定 `TabShell.LabelFunc` 則可為分頁列提供動態無障礙名稱。變更標題時，只要圖示維持不變，便會保留已選取的區段與現有的分頁。

`mobile.TabBar` 是 iOS 與 Android 上的系統分頁列，接受 `Items`、`Selected`、`OnChange`，以及選填的 `Edge`（`mobile.TabBarBottom`、`mobile.TabBarLeading`、`mobile.TabBarTrailing` 或 `mobile.TabBarAutomatic`）與 `EdgeFunc`。每個 `mobile.TabBarItem` 接收一個 `Title` 與置於 `Icon` 中的平台圖示名稱；每當 `Icon` 未在 iOS 上指定 SF Symbol 或在 Android 上指定 drawable 時，便會顯示 `SVG: lucide.House()`，大小調整為 24 點並依選取狀態著色。在 TSX 中，`svg` 接收 SVG 文件，而 `edge` 接收 `"bottom"`、`"leading"`、`"trailing"` 或 `"automatic"`。在其他平台上 `mobile.TabBar` 不佔用任何空間，因此請在該處改為顯示 `ui.Tabs`。

`ui.SidebarShell` 會自行讓其側邊欄、頁尾、標頭、內容與右側面板所容納的內容避開左右兩側的安全區域，同時讓背景延伸至視窗邊緣，因此其內部的左右側 `core.EnvSafeArea` 為 0，且 `ui.NavHeader` 會將側邊安全區域加入其內距中。`ui.NavPage` 或 `TabShell` 區段的內容則會透過 `PaddingEnv` 自行避開安全區域，讓需要延伸至邊緣的內容（例如滿版圖片）能夠做到這一點。

## 無障礙設計

該清單會回報 `RoleTabList`，每個分頁回報 `RoleTab`，而已選取的分頁會回報其狀態。清單為單一 Tab 停駐點；Left 與 Up、Right 與 Down、Home 與 End 鍵可在已啟用的分頁之間移動，到達的分頁會回報一次點擊。`Label` 用於命名該清單。

## 平台行為

iOS 與 Android 會以各自的外觀與手勢顯示系統分頁列；macOS、Windows、Linux 與網頁則顯示 `ui.Tabs`。應用程式的內容在每個目標平台上皆相同。

## 相關資源

- [導覽](../navigation.md)：應用程式中的頁面與分頁列。
- [選單](menu.md)：用於執行指令而非切換區段。
- [下拉選單](select.md)：在清單中呈現大量選項。
- 展示畫廊的 Tabs 與 NavStack 頁面。
- [無樣式元件](../authoring/base.md#tabs)：`base.Tabs`，用於打造自訂外觀的分頁。
