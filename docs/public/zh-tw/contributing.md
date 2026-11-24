---
title: 開發環境設定
description: 從原始碼建置 NSGo、執行元件展覽館（gallery）並測試變更。
group: 參與貢獻
order: 80
---

## 環境需求

安裝有 Xcode 與 Go 1.27.3 或更新版本的 macOS 可建置所有 arm64 目標平台，包含 Android 的 APK。若要在實體裝置上執行，則須具備[環境設定](getting-started.md#requirements)中列出的工具，而 x86_64 Android 模擬器的目標 `android_x64` 則需要 Android NDK，因為 Go 僅在啟用 cgo 時才能連結 android/amd64。製作 Windows 安裝程式需要 NSIS（`brew install makensis`）；若未安裝，`build` 只會保留其腳本。

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

修改程式碼前請先閱讀 [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) 與 [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md)。

## 元件展覽館

元件展覽館（gallery）收錄了每個元素、`base` 與 `ui` 元件及服務的示範。寬視窗會在側邊欄的分頁下顯示各個示範；窄視窗則在底部列顯示分頁與設定，並將示範畫面推疊於其上。從儲存庫根目錄執行：

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## 檢查與測試

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` 會改為檢查它所指定的目標平台而非目前主機；在 NSGo 儲存庫中同時也會檢查開發用戶端（dev client），而未加參數的 `check` 亦會檢查它。若要檢查一項變更所涉及的平台，請在單次檢查中一併列出所有平台（例如 `check -target web,android -jobs 2`），而不要同時啟動多個檢查指令：`-jobs n` 會同時檢查最多 n 個目標平台，且每個 `go vet` 使用 n 個編譯工作執行緒（預設為一次一個目標平台並採用 Go 的預設值）；若同一個模組中已有另一個 check 正在執行，新啟動的 check 會等待其完成並顯示提示訊息。CI 的 `check.yml` 會檢查所有目標平台。

在儲存庫根目錄或 `internal/dev` 中執行時，無頭（headless）指令會額外以 `nsgodev` 建置標籤再測試一次 `internal/dev`——開發用戶端的檔案需要該標籤——使其測試能在本地與 CI 中執行。兩次執行接受相同的篩選器與旗標，並共用同一份統計結果與 JUnit 報告，其中第二次執行的套件名稱會標示為 `internal/dev [nsgodev]`。

`check -target all` 會在任何主機上檢查 macOS、Windows、Linux、Android arm64 與網頁目標平台。當 Xcode SDK 可用時，它也會檢查 iOS、tvOS 與 watchOS，並列出略過的目標。明確指定的目標平台（例如 `check -target ios`）若缺少對應工具將會失敗，即使與 `all` 並用亦然。單一目標失敗不會阻止其餘目標的檢查，而只要有任何必要檢查失敗，指令就會回傳失敗。`build -target all` 仍需要安裝 Xcode 的 macOS。在 NSGo 自身的儲存庫中，每個指定的目標平台也會連同 `nsgotest` 一併檢查其平台測試宿主；CI 會明確指定預覽版的 `harmonyos` 目標，因為它不包含在 `all` 之中。

Linux CI 工作會使用 Node.js 與 Go 的 Wasm 啟動器執行 HarmonyOS Go 後端的 `js/wasm` 測試。透過 `hdc` 連接 HarmonyOS 模擬器即可於本地執行展覽館測試。編譯 [HarmonyOS 宿主範本](building.md#harmonyos)需要 OpenHarmony SDK API 26。

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

在 Linux 桌面的 Wayland 工作階段中，可使用 `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux` 執行真實的全域快捷鍵入口（portal）驗收測試。該 ID 必須具備 `.desktop` 項目。請在合成器的提示中授予 Ctrl+Alt+K 權限，接著在另一個應用程式取得焦點時按下該快捷鍵；該檢查會同時等待註冊與觸發完成。若未設定此環境變數，則會略過該測試。

若 Linux 主機上可使用 Docker，執行 `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` 可在拋棄式的 Debian、Ubuntu 與 Arch 容器中驗證真實安裝流程。它會下載官方映像檔並在其中安裝 GTK 相依套件、驗證每個套件資料庫與已安裝檔案，隨後移除套件，絕不會動用主機的套件資料庫。

## 發佈

NSGo 會在下游客戶專案需要特定標籤版本或維護者有已準備好交付的變更時隨需發佈。若要為某個函式庫請求發佈，請連結相關工作並指出包含其所需最後一項 NSGo 變更的 commit。第一個包含該 commit 的發佈標籤，即為該函式庫可指定相依的版本。

若要在正式打標籤前測試已合併的變更，請在相依模組中執行 `go get github.com/nsgo-dev/nsgo@main`。你也可以用 commit ID 取代 `main` 以選取你已驗證的修訂版本。Go 會將產生的偽版本（pseudo-version）記錄在 `go.mod` 中。

維護者會在 `CHANGELOG.md` 中為變更填入版本號與日期、保留一個全新的 Unreleased 區段，並在 `main` 上為該 commit 打上標籤。[發佈工作流程](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml)會檢查該標籤，並在測試工作流程通過後正式發佈；已發佈的更新日誌隨後會自 `CHANGELOG.md` 移除（該檔案僅保留尚未發佈的變更），並公布於 [Releases](https://github.com/nsgo-dev/nsgo/releases)。

## NSH

NSGo 以 [Native Shell](https://github.com/longbridge/nsh) 作為設計參考；NSGo 的測試皆不依賴它。若要閱讀其原始碼，請將其複製到本儲存庫旁並簽出其最新的發佈標籤，切勿直接使用 `main`：

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## 下一步

- [程式碼庫結構](contributing/codebase.md)：NSGo 各部分的所在位置。
- [提交變更](contributing/changes.md)：pull request 所需的項目。
