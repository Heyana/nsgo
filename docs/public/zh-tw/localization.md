---
title: 在地化
description: 翻譯應用程式的訊息字串，並跟隨應用程式所顯示的語言。
group: 核心概念
order: 62
---

## 訊息

`nsgo.yaml` 旁的 `locales/` 目錄存放應用程式的訊息，每個語言對應一個採用 ICU MessageFormat 格式的 YAML 檔案（例如 `en.yaml`、`de.yaml`）：

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

每則訊息可以是一個字串，或是包含 `message`、提供給譯者的 `description` 與 `placeholders` 的物件。

套件會匯入 `locales/`。`dev`、`build` 與 `test` 會檢查語系目錄，並在旁邊為每則訊息產生對應的 Go 函式，例如 `locales.Greeting(cx, "Ada")`，它會以應用程式目前顯示的語言輸出該訊息，並在語言變更時自動跟隨更新；執行 `go tool nsgo i18n` 則可單獨產生這些函式。訊息支援數字與日期格式化（`{d, date, medium}`），`appName` 會在其各個封裝中以對應語言命名應用程式，而帶有 `apple.info` 鍵值的訊息（例如 `NSCameraUsageDescription`）則會翻譯該 `Info.plist` 項目（[權限](permissions.md#用途說明)）。

## 語言切換

`cx.SetLocale("de")` 可指定應用程式使用的語言，且版面配置會依該語言的書寫方向排版。在獨立應用程式中，系統自身的介面文字——例如文字選取選單、檔案對話框與 GTK 小工具——也會跟隨指定的語言（在 Android 13+、Windows 與 Linux 上會立即生效，在 macOS 與 iOS 上則於下次啟動時生效）；呼叫 `cx.SetLocale("")` 即可恢復跟隨使用者的系統語言。

```sh
go tool nsgo dev -lang en-XA
```

會以偽語系（pseudo locale）啟動應用程式，藉此找出尚未翻譯或遭到截斷的文字。任何建置出來的應用程式在啟動時皆可傳入此參數：`--nsgo-lang=en-XA`。

## 星期、時間與度量單位

`i18n.Regional(cx)` 會回報除了語言之外，使用者偏好的星期、時間與度量單位讀取方式：一週的起始日、12 小時制或 24 小時制、度量衡系統以及溫度單位，優先採用使用者在平台上的設定，若無則採用其所在地區的預設值。除非應用程式另外設定了 `FirstDayOfWeek` 或 `HourCycle`，否則行事曆與日期選擇器會依此起始日顯示每週第一天、時間欄位會依此時制顯示時間，且語系目錄訊息中所格式化的時間也會跟隨使用者的時間制式。

## NSGo 內建文字

NSGo 的元件會以 NSGo 已翻譯的語言顯示其內建文字。若要支援其他語言，可在 YAML 檔案中以套件名稱作為鍵值，例如 `github.com/nsgo-dev/nsgo/base/locales`，來提供該套件在該語言下的訊息翻譯。各平台的選單，以及當系統本身未提供詞彙時（例如分頁或搜尋框）由 Android 與 Windows 螢幕閱讀器播報的角色與狀態名稱，皆會使用 `github.com/nsgo-dev/nsgo/desktop/locales` 中的文字。
