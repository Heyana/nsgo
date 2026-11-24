---
title: 環境設定
description: 安裝 NSGo 所需的工具、建立應用程式，並在每個目標平台上執行。
group: 快速開始
order: 10
---

## 系統需求

- [Go](https://go.dev/dl/) 1.27.3 或更新版本。
- 若要建置 iOS、tvOS 與 watchOS 版本，需要一台安裝 Xcode 的 Mac。其他所有目標平台皆可直接在 macOS 上單獨使用 Go 建置；x86_64 Android 模擬器另外還需要 Android NDK。
- 若要在 iPhone 上執行，需在 Xcode 的 Accounts 中登入 Apple ID、信任該 iPhone，並開啟開發者模式。
- 若要在 Android 上執行，需要 Android SDK platform-tools 中的 `adb`。
- 若要在 Windows 上執行，只需安裝 Go 即可，它同時也能建置 Windows 與網頁套件。
- 在 Linux 上，需要 GTK 4.14 或更新版本、Pango 與 Cairo。網路圖片使用 libsoup 3，網頁檢視（web view）使用 WebKitGTK 6.0，而音訊與視訊則使用 GStreamer 外掛程式。在具有 AppArmor 使用者命名空間限制的系統上，[WebKitGTK 需要設定 bubblewrap 設定檔](debugging.md#linux-網頁檢視)。

## 建立應用程式

建立一個模組、加入 NSGo 及其工具，然後讓工具產生一個應用程式：

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` 會將 `nsgo` 一次性加入模組中，因此不需要額外安裝即可透過 `go tool nsgo` 執行。`init` 會產生以下檔案：

| 檔案 | 說明 |
| --- | --- |
| `app.go` | 應用程式本體，一個計數器：包含 `Root` 元件與呼叫 `nsgo.Run(Root)` 的 `main` |
| `app_test.go` | 按下按鈕的測試 |
| `nsgo.yaml` | 應用程式的名稱、ID、版本與目標平台 |

改用 `go tool nsgo init -template tsx` 則會產生 TSX 應用程式，而 `-template mixed` 會產生一個以 TSX 為進入點並使用自有 Go 元件的 Go 應用程式。

### 來自 Go 模組的範本

傳入範本的模組路徑與選用的版本號。若省略版本號，將會解析為 Go 模組代理能找到的最新版本。

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

請將起始範本發佈在模組的 `template/` 目錄中，並包含 `nsgo.yaml` 與 `main.go`。工具會使用 Go 的 `text/template` 渲染這兩個檔案以及任何以 `.tmpl` 結尾的檔案，並從輸出檔名中移除 `.tmpl`；其他檔案（包含二進位資源）則會原封不動地複製。

範本會接收到 `.Name`（應用程式的顯示名稱）、`.ID`（產生的應用程式 ID）、`.Slug`（小寫且以連字號分隔的名稱）與 `.Module`（`-module` 的值，或預設為 slug）。由於 Go 下載模組時會排除巢狀的 `go.mod` 檔案，請隨附一個 `go.mod.tmpl` 來宣告產生後應用程式的模組與相依套件。若未提供，`-module` 會寫入與內建範本相同的精簡 `go.mod`。

## 執行應用程式

```sh
go tool nsgo dev
```

`dev` 會為目前這台機器建置並開啟應用程式。每次儲存時都會重新建置並重啟應用程式；若僅修改 TSX，則會直接在執行中的應用程式內就地更新。若建置失敗，畫面頂層會顯示錯誤訊息，並附帶複製錯誤與重新建置的按鈕。

使用 `-target` 在其他平台上執行：

```sh
go tool nsgo dev -target web                # 在瀏覽器中
go tool nsgo dev -target ios_simulator      # 在 iOS 模擬器中
go tool nsgo dev -target tvos_simulator     # 在 Apple TV 模擬器中
go tool nsgo dev -target watchos_simulator  # 在 Apple Watch 模擬器中
go tool nsgo dev -target android            # 在已連接的裝置或模擬器上
```

`go tool nsgo doctor` 會列出每個目標平台，說明這台機器是否能建置與執行它，以及無法建置或執行時缺少的項目（例如 Xcode、`adb`、已連接的裝置或 GTK）。

[執行與除錯](debugging.md)涵蓋裝置管理、追蹤更新、效能監控與其他語言支援。

## 搭配編碼代理使用

NSGo 的 skill 可以教導 Claude Code、Codex 與 Cursor 等編碼代理如何撰寫、測試與發佈 NSGo 應用程式。請將它加入應用程式目錄，或加上 `-g` 套用到所有專案：

```sh
bunx skills add nsgo-dev/nsgo
```

會自行閱讀文件的代理可以從 <https://nsgo.dev/llms.txt> 開始，其中列出了每個頁面位於 `https://nsgo.dev/docs/<page>.md` 的 Markdown 連結；而 <https://nsgo.dev/llms-full.txt> 則包含所有頁面的完整內容。代理也可以直接操作執行中的應用程式（參閱[執行與除錯](debugging.md#代理程式)）。

## 下一步

- [範例：待辦事項應用程式](tutorials/todo.md)：狀態、清單與測試，約十分鐘完成。
- [範例：筆記應用程式](tutorials/notes.md)：導覽堆疊與保存在裝置上的筆記。
- [範例：熱門 GitHub 儲存庫](tutorials/repos.md)：從網路載入資料，並處理錯誤與重試。
