---
title: 與其他框架的比較
description: NSGo 相較於 NSH、MyGo 與其他 UI 框架的定位，以及它所取捨的特性。
order: 2
---

NSGo 在眾多 UI 框架中的定位來自三項選擇：應用程式以 Go（或建構於其上的 TSX）撰寫、每個平台皆使用各自的原生檢視繪製，以及單一程式碼庫同時涵蓋手機、桌面與網頁。其他框架做出了不同的選擇，而每種選擇都有其代價。

## 一覽

| 框架 | 語言 | 繪製方式 | 目標平台 |
| --- | --- | --- | --- |
| NSGo | Go、TSX | 各平台的原生檢視 | iOS、App Clips、tvOS、watchOS、Android、macOS、Windows、Linux、網頁；實驗性支援小程式 |
| [NSH](https://nsh.dev) | Rust、TSX | 各平台的原生檢視 | iOS、Android、macOS、Windows、Linux、網頁、GPUI 應用程式、終端機；透過社群平台支援 HarmonyOS 與微信小程式 |
| [MyGo](https://github.com/egoist/mygo) | Go、網頁 | 自建 GPU 轉譯器或系統 webview | macOS、Windows、Linux、網頁 |
| [Fyne](https://fyne.io)、[Gio](https://gioui.org) | Go | 自建轉譯器 | 桌面、手機、網頁 |
| [Wails](https://wails.io) | Go、網頁 | 系統 webview | 桌面 |
| [Flutter](https://flutter.dev) | Dart | 自建轉譯器 | 手機、桌面、網頁 |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | 自建轉譯器；Android 上為 Compose | 手機、桌面、網頁 |
| [React Native](https://reactnative.dev) | JavaScript、TSX | 各平台的原生檢視 | iOS、Android；透過社群平台支援其他平台 |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | 各平台的原生控制項 | iOS、Android、macOS、Windows |
| [Avalonia](https://avaloniaui.net) | C# | 自建轉譯器 | 桌面、手機、網頁 |
| [Qt](https://www.qt.io) | C++、QML | 自建轉譯器 | 桌面、手機、嵌入式裝置 |
| [Electron](https://www.electronjs.org)、[Tauri](https://tauri.app) | 網頁；Tauri 搭配 Rust | 內嵌 Chromium；Tauri 使用系統 webview | 桌面；Tauri 另支援手機 |

## 優勢

- **原生檢視。** 文字、輸入法、選取、捲動、無障礙功能與選單皆由平台自身提供，因此應用程式的行為完全符合使用者在該平台上的預期；而自行繪製的框架則必須逐一重建這些功能。針對單一平台獨有的特性（例如 iOS 的 Liquid Glass），其他平台會以相同方式模擬，而非直接捨棄。
- **細粒度更新。** 元件只會執行一次，並由訊號（signal）僅更新讀取該訊號的部分，無須虛擬 DOM 或在每一幀重新執行檢視函式來進行差異比對；動畫則運行於平台的影格時脈上。
- **最小封裝體積與最快啟動速度。** 在[基準測試](https://nsgo.dev/benchmarks/)中，相同的計數器應用程式在 macOS、Windows 與 Linux 上打包後僅約 8 MB，小於 MyGo、GPUI 與 NSH（NSH 為 33 至 45 MB）；其啟動速度也最快，記憶體佔用亦低於 MyGo 與 GPUI 等直接於 GPU 上繪製的框架。
- **純 Go 實作。** 引擎、版面配置與各平台後端皆以 Go 撰寫，Go 應用程式無須 C、cgo 或 JavaScript 引擎。
- **一台 Mac 建置所有平台。** 只要有一台安裝 Go 與 Xcode 的 Mac，即可交叉編譯所有目標平台：無須 NDK、Android SDK、JDK、GTK 或 Windows 工具鏈。
- **一道指令產出所有發佈檔案。** `nsgo build` 可產出經簽署與公證的 macOS 磁碟映像檔、經簽署的 Windows 安裝程式、deb 與 Arch 套件及 AppImage、Mac App Store、Microsoft Store、Google Play 與 Flathub 套件，以及從 GitHub releases 安裝的腳本（參閱[建置與發佈](building.md)）。
- **使用各平台自身的更新機制。** 在 macOS 上由 Sparkle、在 Windows 上由 WinSparkle 下載、驗證並安裝下一版本；在 Linux 與 Android 上應用程式會偵測新版並開啟其發佈頁面；在 iOS 上則由 App Store 處理。透過商店安裝的應用程式會交由該商店管理更新（參閱[更新](building.md#更新)）。
- **涵蓋所有平台。** 手機、平板、電視、手錶、桌面與網頁共用同一套引擎與 API，並支援 iOS App Clips、iPhone 與 Mac 小工具，以及實驗性支援微信、QQ、支付寶與抖音小程式。
- **可嵌入既有應用。** NSGo 應用程式可以作為一個檢視嵌入現有的 iOS、Android、Go 桌面或網頁應用程式中，方便以逐個畫面的方式漸進遷移（參閱[嵌入](embedding.md)）。
- **快速的開發迴圈。** 儲存 Go 檔案即可重新建置並重啟應用程式；修改 TSX 則會原地更新執行中的應用程式並保留其狀態；建置錯誤與轉譯期間的 panic 會直接顯示在應用程式畫面上（參閱[執行與除錯](debugging.md)）。
- **一套測試通用於各處。** 測試能像使用者一樣透過文字或角色尋找並操作控制項，既可在無視窗環境下快速執行，也可無須修改直接執行於各目標平台的真實視窗中、透過平台自身輸入事件運作並儲存螢幕截圖（參閱[測試](testing.md)）。
- **內建在地化支援。** 應用程式與元件的字串集中存放於語系目錄（catalog）中，並可透過偽語系（pseudo-locale）找出尚未翻譯、遭到截斷或尚未支援由右至左排版的文字（參閱[在地化](localization.md)）。
- **以 Go 撰寫著色器。** `core.Shader` 可在每個平台上執行以 Go 語法或 WGSL 撰寫的 GPU 著色器（參閱[畫布與著色器](canvas.md#著色器)）。
- **視需要選用 TSX。** 畫面亦可使用 TSX 搭配 Tailwind 類別撰寫，並運行於與 Go 元件相同的引擎上。

## 劣勢

- **仍處於早期階段。** 0.1.0 為首個釋出版本，API 尚未穩定；[開發路線圖](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)列出了尚欠缺的功能，例如在執行階段載入 TSX 時的堆積記憶體限制。
- **生態系較小。** 相較於 Flutter、React Native 或 Qt，目前網路上的第三方元件、範例與解答仍較少。
- **平台之間存在差異。** 使用原生檢視意味著各平台之間會有細微差異，因此應用程式仍須在各平台上檢查。自行繪製的框架則在各處看起來完全一致。
- **專屬的橋接機制。** 在 Android 上，Go 程式會與 Java 執行環境並行運作並透過管線通訊；而在 Linux 上，使用者需要 GTK 4.14 或更新版本。
- **TSX 採直譯執行。** TSX 運行於以 Go 撰寫的 JavaScript 直譯器 goja 中，速度慢於 React Native 或 Electron 所使用的引擎。

## NSH

NSGo 移植了 NSH 的設計：其架構、後端、API 命名以及 `base` 中的無樣式元件皆源自 NSH（參閱[專案狀態](index.md#目前狀態)）。

- **選擇 NSGo** 的情境：想以 Go 而非 Rust 或 TSX 撰寫應用程式、希望只用安裝 Go 與 Xcode 的 Mac 建置所有目標平台，或需要 NSGo 額外提供的功能——以 Go 撰寫的 GPU 著色器、向量圖示、終端機、應用程式自行開啟的視窗、面板與系統列圖示，以及 `ui` 中的預設樣式元件。
- **選擇 NSH** 的情境：需要較成熟的專案、以 QuickJS 搭配位元組碼與堆積限制執行的 TSX、GPUI 後端，或 32 位元 Android 支援。

## MyGo

兩者皆為無須 cgo、以 Go 撰寫的 UI 框架，但 MyGo 是直接在 GPU 上繪製其原生 UI，且每一幀皆透過檢視函式重新建構；而 NSGo 則是將變更之處交給原生檢視。

- **選擇 MyGo** 的情境：需要在所有桌面平台上外觀完全一致的桌面應用程式、具備型別化 Go 呼叫的網頁視窗，以及其完善的桌面 API 與發佈工具：系統列圖示、檔案關聯、安裝程式、公證與簽署的差異更新。
- **選擇 NSGo** 的情境：希望同時觸及手機平台、使用各平台自身的文字處理、輸入法與無障礙功能，或以訊號管理狀態而非從應用程式資料重繪畫面。

## 其他 Go 工具包

- **Fyne 與 Gio** 與 MyGo 一樣自行繪製所有內容，且兩者皆可支援手機與網頁。Fyne 提供具備自家風格的完整元件組；Gio 則是即時模式（immediate mode），偏向底層且富有彈性。兩者皆已有正式釋出版本與使用者群；但外觀與操作皆非原生體驗。
- **Wails** 將網頁前端放入系統 webview 並由 Go 擔任後端：任何網頁函式庫都能使用，但介面本質上是網頁，且僅支援桌面平台。

## 其他語言的框架

- **Flutter 與 Compose Multiplatform** 自行繪製每一個像素：在所有平台上外觀一致且相當成熟、擁有龐大生態系，但文字、捲動與輸入皆為重建而非原生實作，且需要學習 Dart 或 Kotlin。
- **React Native** 與 NSGo 同樣採用反應式核心驅動原生檢視的模式，且擁有同類框架中最大的生態系。它以 iOS 與 Android 為中心，並以 JavaScript 執行應用邏輯；NSGo 則以 Go 執行，並將桌面與網頁視為一等公民。
- **.NET MAUI** 以 C# 封裝各平台的控制項，但不支援 Linux 與網頁。**Avalonia** 則是自行繪製介面，風格接近 WPF，支援所有桌面平台、手機與網頁。
- **Qt** 成熟且高效，並可觸及嵌入式裝置，但它自行繪製小工具、需使用 C++ 或 QML，且採用 LGPL 或商業授權。
- **Electron 與 Tauri** 呈現網頁內容：可使用完整的網頁工具與函式庫，但代價是瀏覽器等級的記憶體消耗（尤以 Electron 為甚），以及像網頁般的操作介面。

## 如何選擇

若你需要從單一 Go 程式碼庫打造在手機、桌面與網頁上皆具備原生體驗的應用程式，且能接受早期階段的專案，請選擇 NSGo。若應用程式必須在短期內基於穩定的 API 上線，請從上表選擇成熟的框架：手機應用可選 Flutter 或 React Native，純桌面應用可選 MyGo、Wails 或 Tauri。
