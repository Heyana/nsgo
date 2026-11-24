---
title: 程式碼庫結構
description: NSGo 各部分的所在位置，以及說明設計緣由的文件。
group: 參與貢獻
order: 81
---

NSGo 是一個單一的 Go 模組。應用程式從上層匯入其套件；各平台的後端位於最底層，接收變動指令以建立、更新與移除原生檢視。

## 套件結構

| 套件 | 內容說明 |
| --- | --- |
| `nsgo`（根目錄） | `nsgo.Run` 與 `nsgo.NewView`，依建置標籤選取對應後端 |
| `core` | 應用程式 API：元素、樣式、類別、主題、訊號與服務；版面配置與文字前置處理 |
| `base` | 無樣式元件，每個元件一個檔案；較複雜者獨立為套件（`base/dock`、`base/editor`） |
| `ui` | 具備外觀樣式的 `base` 元件，以及如 `ui/chat` 等複合套件 |
| `mobile`、`desktop` | 僅限手機或僅限桌面的功能 |
| `tsx` | TSX 應用程式、其打包器與編譯器，以及供匯入的模組 |
| `i18n`、`*/locales` | 訊息語系目錄與由其產生的程式碼 |
| `nsgotest` | 模擬使用者操作的測試工具，可於無頭模式或真實平台上執行 |
| `internal/engine`、`internal/driver`、`internal/layout` | 引擎 goroutine、掛載協定與版面配置引擎 |
| `internal/platform/<target>` | 平台後端：單一目標平台的原生檢視、文字與輸入實作 |
| `cmd/nsgo` | `nsgo` 命令列工具 |
| `examples/` | 元件展覽館與其他範例應用程式 |

套件僅能向下相依匯入；`internal/moduletest/layering_test.go` 會在所有目標平台上檢查此相依圖。

## 設計文件

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) 繪製了套件結構、執行緒模型以及從提交到畫面的資料流，並列出 `docs/architecture/` 中的各個主題文件：每個部分各自一篇，例如版面配置、文字、捲動、主題或命令列工具。修改任何部分前請先閱讀對應主題文件；其中記錄了該部分的運作方式，以及與 NSGo 所移植設計來源 [Native Shell](https://github.com/longbridge/nsh) 的差異之處。

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) 載明了所有變更都必須遵守的規範，人類開發者與程式設計代理皆適用。其中幾條原則主導了大多數的改動：

- 每個 API 各有其專屬歸屬：`core` 負責元素與環境，`base` 負責控制項，`ui` 負責外觀樣式，`mobile` 與 `desktop` 則各自負責單一裝置類型。
- `base` 不帶有任何外觀樣式；`ui` 元件即是在 `base` 的形態上加上外觀樣式。
- 絕不依裝置隱含改變行為：在 `core` 或 `base` 中不因桌面而放大控制項，也不因手機而縮小控制項。
- 處處皆為原生檢視，各平台表現一致：某個平台原生具備的功能，其他平台便加以模擬。
- 絕不手寫 C 程式碼，且除了 x86_64 Android 模擬器之外，只需 macOS、Xcode 與 Go 即可建置所有目標平台。

各平台目前尚欠缺的功能列於[開發路線圖](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)。
