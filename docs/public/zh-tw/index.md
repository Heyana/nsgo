---
title: 簡介
description: 什麼是 NSGo、它可在哪些平台執行，以及它的運作方式。
order: 1
---

NSGo 是一個以 Go 撰寫的跨平台原生 UI 框架。你只需使用 Go 或 TSX 撰寫一次應用程式，就能在 iOS 與其 App Clips、tvOS、watchOS、Android、macOS、Windows、Linux 與 Web 上執行，並由各平台自身的視圖繪製。Go 應用程式也能以實驗性質在微信、QQ、支付寶與抖音小程式中執行：這些平台將單一應用程式限制在 2 MB 以內，較大的應用程式會超過此限制。HarmonyOS 則以預覽形式執行 Go 應用程式，並由 ArkUI 繪製。

## 你撰寫的內容

應用程式是一個 Go 的 main 套件。其中的元件會回傳元素：視圖、文字、圖片、輸入框以及 `ui` 中的控制項。狀態儲存在訊號（signal）中，當訊號改變時，只有讀取該訊號的部分會更新。

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

相同的應用程式也可以使用 TSX 撰寫，並以 Tailwind 類別設定樣式（[TSX 應用程式](tsx.md)）。

## 運作方式

Go 引擎負責掌控整個應用程式：響應式圖譜、樣式、flexbox 與網格版面配置，以及文字前處理。每次更新只會產生一批變更，並由輕量後端套用至各平台的原生視圖。動畫在平台的影格時脈上執行，因此不會在每個影格都進行提交（commit）。

| 平台 | 繪製技術 |
| --- | --- |
| iOS | UIKit 與 CoreText |
| Android | Android Views |
| macOS | AppKit 與 CoreText |
| Windows | Direct2D 與 DirectWrite |
| Linux | GTK 4、Pango 與 Cairo |
| Web | WebAssembly 與 DOM 元素 |

每個目標平台都能在配備 Go 的 Mac 上建置；iOS 還需要 Xcode。`nsgo` 工具可用於建立、執行、測試與封裝應用程式。

## 目前狀態

NSGo 仍處於早期階段：0.1.0 是其首個發行版本，API 尚未穩定，缺少的功能列於[開發藍圖](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)。它將 [Native Shell](https://github.com/longbridge/nsh) 的設計移植到 Go。

## 如何閱讀本文件

每一頁都回答一個問題。除非頁面另有說明，否則程式碼範例都是元件的一部分片段：`cx` 是元件接收到的 `*core.Scope`。應用程式使用 `ui` 的元件，它們帶有預設外觀；`base` 則包含相同但無外觀的控制項，供元件作者使用。

- **快速開始**：[環境設定](getting-started.md)，接著是逐步建構的範例應用程式：[待辦事項應用程式](tutorials/todo.md)、[筆記應用程式](tutorials/notes.md)與[熱門 GitHub 儲存庫](tutorials/repos.md)。
- **工具鏈**：[執行與除錯](debugging.md)、[測試](testing.md)、[建置與發佈](building.md)與[嵌入](embedding.md)。
- **元件**：[最常用的元件](ui.md)，包含其變體、狀態與無障礙支援。
- **核心概念**：[元件與狀態](components.md)、[版面配置](layout.md)、[樣式](styles.md)、[文字](text.md)、[清單](lists.md)、[導覽](navigation.md)、[表單](forms.md)、[在地化](localization.md)、[TSX](tsx.md) 以及其他主題。
- **平台功能**：[平台服務](services.md)、[通知](notifications.md)、[觸覺回饋](haptics.md)、[小工具](widgets.md)、[即時動態（Live Activities）](live-activities.md)與[權限](permissions.md)。
- **撰寫元件**：[供他人應用程式使用的元件](authoring/index.md)，包含無樣式與帶樣式的元件。
- **參與貢獻**：[從原始碼建置 NSGo](contributing.md)、熟悉專案結構並提交變更。
- [線上展示](https://nsgo.dev/gallery/)：在瀏覽器中體驗每一個元件。
