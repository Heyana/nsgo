---
title: 小工具
description: 在 iPhone 與 iPad 的主畫面和鎖定畫面，以及 Mac 的桌面上顯示應用程式的視圖。
group: 平台功能
order: 47
---

## 小工具類型

無論應用程式是否正在執行，小工具都能提供應用程式的即時摘要視圖：顯示於 iPhone 與 iPad 的主畫面及鎖定畫面、Mac 的桌面與通知中心，以及 Android 的啟動器中。每個類型（kind）代表系統小工具庫所提供的一種小工具，並包含其支援的尺寸規格（family）：

```yaml
widgets:
  - kind: counter
    name: Counter                 # shown in the system's gallery of widgets
    description: Shows the count.
    families: [small, medium, circular]  # also large, extra_large, rectangular and inline
# apple: { app_group: group.dev.example.hello }  # the shared App Group, group.<id> by default
```

在 Apple 平台上，小工具由擴充功能負責繪製，並由 `build` 與 `dev` 透過 Xcode 的 Swift 編譯器進行編譯（[建置與發佈](building.md#小工具與即時動態)）。在 Android 上，預先建置的 Java 執行階段會將相同的時間軸繪製為 App Widgets。兩者在應用程式停止後仍會繼續顯示已發佈的內容。

針對沙盒化的 Mac App Store 建置，請設定 `apple.app_group`，並透過 `sign.macos_profile` 與 `sign.widgets_profile` 提供 macOS 的佈建描述檔（provisioning profile）。主應用程式與 `<id>.widgets` 皆須屬於同一團隊，並具備該群組的授權（entitlement）。時間軸與圖片會存放在該共用容器中；直接發佈與開發用的 macOS 建置則維持原有的家目錄位置。[商店簽署](building.md#商店封裝檔)說明了相關建置選項。

`name` 與 `description` 可以指定應用程式語系目錄（catalog）中的訊息鍵值。如此設定時，小工具庫與預覽佔位畫面會使用使用者的語言，並在缺少翻譯時退回使用來源目錄的文字。若值不是目錄鍵值，則會按原樣顯示。

## 時間軸

`widget.Update` 會以時間軸項目的形式，寫入指定類型的小工具自此刻起要顯示的內容：系統會在無須啟動應用程式的情況下，顯示時間最近剛過的項目，並接著在各自的時間點依序顯示後續項目。零值 `Date` 代表現在。若沒有任何項目，小工具會顯示該類型的名稱與說明。在 Apple 平台上，位於前景的應用程式可依需求任意更新小工具，而位於背景的應用程式每天約可更新數十次。

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` 可為特定尺寸規格指定專屬視圖以取代 `Entry.View`，`Entry.Background` 會跨越系統保留的邊距填滿小工具後方背景，而 `Entry.URL` 則會在輕觸小工具時用來開啟應用程式。在未建置小工具功能的應用程式中，`widget.Update` 會回報 `widget.ErrNoWidgets`；在 macOS、iOS 與 Android 以外的平台會回報 `widget.ErrUnsupported`；可在寫入前透過 `widget.Available` 確認支援狀況。此函式可在任意 goroutine 上執行，也能在呼叫 `nsgo.Run` 之前呼叫。

## 視圖

小工具的視圖並非 `core` 的元素：其版面配置方式與 SwiftUI 相同，除非透過 `Style` 指定尺寸或設為填滿，否則視圖大小與其內容一致；除非 `AlignItems` 與 `Justify` 另有指定，否則容器會將其內容置中。`Text`、`Symbol` 以及由位元組或 `Update` 所讀取檔案構成的 `Image` 用於顯示內容；`Column`、`Row`、`Stack`、`Spacer` 與 `Divider` 用於排列版面；`Rectangle`、`Progress` 與 `Gauge` 用於繪製圖形；`Date` 則以系統格式顯示時間，其中 `Relative`、`Offset` 與 `Timer` 會自行持續計時，無須為每一秒都建立時間軸項目。

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` 接受間距（gap）、內距（padding）、尺寸或填滿、對齊方式、帶有圓角的背景、系統文字樣式或字型大小、粗細、設計與變體、行數限制、用於開啟應用程式的 `Link`，以及讓系統將視圖納入著色範圍的 `Accented`。`widget.Primary`、`Secondary` 與 `Accent` 會跟隨系統外觀變化，而 `RGB` 與 `RGBA` 則可透過 `Dark` 為深色模式指定另一種顏色。

## 按鈕

`widget.Button` 自 iOS 17 與 macOS 14 起無須開啟應用程式即可執行 `widget.OnAction`。較舊的系統則只會顯示其子項目。請在 `main` 中、呼叫 `nsgo.Run` 之前註冊該處理函式：系統可能會在不載入視圖的情況下於背景啟動應用程式。小工具按下事件中的 `Activity == nil`，而其 `Action` 即為按鈕的值。

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

請處理該操作並在處理函式返回前呼叫 `widget.Update`，讓系統自動重新載入時間軸時能看見更新結果。此處理函式執行於視圖的 goroutine 之外；若要變更視圖，請使用 `core.Scope.Post`。此機制與[即時動態](live-activities.md)共用相同的處理函式與 intent，且僅需在 `nsgo.yaml` 中設定 `widgets`，無須設定 `live_activities`。

## Android

從啟動器的小工具選取器新增小工具，並在該處調整其大小。繪製器會根據邊界尺寸從允許的規格中選擇對應類型：寬高皆小於 240 點時為 `small`、寬度達 240 點以上為 `medium`、高度達 240 點以上為 `large`，寬度達 500 點以上為 `extra_large`；若該規格未列於設定中，則改用清單中列出的第一個規格。`Entry.URL` 與 `Style.Link` 會透過應用程式在 `url_schemes` 中宣告的 URL scheme 來開啟應用程式。巢狀視圖上的連結會優先於其容器的連結。

未來的時間軸項目使用 Android 的非精確鬧鐘（inexact alarm），應用程式位於背景時系統可能會延遲觸發。計時器視圖使用原生計時元件（chronometer），無須應用程式執行即可持續計時；目前支援大小與顏色設定，自訂字型粗細、設計與斜體樣式則仍在規劃目標中。相對時間（relative date）會依系統排程重新整理。通用繪製器支援文字、圖片、版面配置、形狀、進度條、環形圖以及一組常見的符號圖示；若圖稿必須在雙平台上維持一致，請使用 `Image`。支援任意 SF Symbol 名稱在 Android 上仍為尚未完成的目標。系統僅會為獨立 APK 與 AAB 產生 Android 小工具提供者；嵌入式宿主則不會產生。

## 即時動態

同一個擴充功能也會在鎖定畫面與動態島中繪製應用程式的即時動態（[即時動態](live-activities.md)）。

## 下一步

- [即時動態](live-activities.md)：即時追蹤應用程式正在發生的事件。
- [通知](notifications.md)：小工具之外的通知與圖示標記。
- [建置與發佈](building.md)：擴充功能所共用的 App Group 設定。
