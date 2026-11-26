---
title: 原生函式庫
description: 透過 NSGo 標準的打包、授權與簽署步驟，隨 Go 套件一同發佈預先建置的共用函式庫與 Apple 框架。
group: 工具鏈
order: 23
---

Go 套件只要在其套件目錄中放置 `nsgo-natives.yaml`，即可發佈其透過 purego 載入的共用函式庫；自版本 2 起，也能發佈其 iOS 與 tvOS 框架。當應用程式匯入該套件時，NSGo 會為目標平台選取對應的宣告、驗證檔案，並在進行簽署與製作安裝程式前將其納入。應用程式本身不需要任何額外設定或打包指令。

桌面應用程式建置與目標平台測試會對應用程式程式碼停用 cgo。原生程式碼必須使用 purego，並宣告 `nsgo-natives.yaml` 函式庫。NSGo 會在建置開始前拒絕含有 cgo 檔案的應用程式套件。

## 宣告函式庫

```yaml
version: 2
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

版本 2 支援的鍵值為 `macos_arm64`、`macos_x64`、`windows_arm64`、`windows_x64`、`linux_arm64`、`linux_x64`、`ios`、`ios_simulator`、`tvos` 與 `tvos_simulator`。版本 1 只支援其中六個桌面鍵值；檔案只會依其版本的鍵值讀取，因此 iOS 與 tvOS 建置不會從版本 1 的檔案打包任何內容。請為該 Go 套件會被選用的每一個目標平台進行宣告；`[]` 明確表示該目標平台不需要隨附函式庫。你也可以使用 Go 建置標籤（build tags），讓套件排除於其不支援的目標平台之外。`macos_universal` 會同時採用上述兩個 macOS 項目，終端目標平台則採用其桌面平台的項目：`tui_macos_arm64` 採用 `macos_arm64`。

每個函式庫項目包含：

- `name`：安裝後的檔案名稱，在 macOS 上以 `.dylib` 結尾，在 Windows 上以 `.dll` 結尾，在 Linux 上則以 `.so` 或含版本號的 `.so.*` 結尾。在 iOS 與 tvOS 上則是框架名稱 `<Name>.framework`，其中 `Name` 由 ASCII 字母、數字、`.` 與 `-` 組成，且以字母或數字開頭與結尾。
- `path` 或 `url` 兩者擇一設定：`path` 為相對於此套件目錄的檔案路徑，`url` 則為函式庫檔案本身的 HTTP(S) 網址。版本 2 只接受 https 網址，重新導向離開 https 時下載便會失敗。請發佈解壓縮後的函式庫檔案，而非壓縮封存檔網址；框架則只發佈其二進位檔。網址中不得內嵌使用者名稱與密碼。
- `sha256`：原始檔案的 SHA-256 雜湊值，無論採用哪一種來源皆為必填。例如在 macOS 上可透過 `shasum -a 256 lib/macos-arm64/libexample.dylib` 印出此值。
- `licenses`：指向原始授權條款與聲明檔案的非空路徑清單，相對於此套件目錄。這些文字內容會隨附於 `THIRD_PARTY_NOTICES.txt`，並在發佈建置中透過 `core.ThirdPartyNotices()` 取得。應用程式現有的[授權政策](building.md#授權聲明)（包含精確模組例外規則）亦會獨立套用至原生授權條款。

本機路徑與符號連結必須位於進行宣告的套件目錄之內。NSGo 會拒絕未知欄位、檢查碼不符、屬於其他作業系統或架構的檔案，以及互相衝突的安裝檔名。兩個套件可以共用名稱與檢查碼皆相同的檔案。單一檔案大小上限為 1 GiB。

## 打包與載入

| 目標平台 | 函式庫目錄 |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows、Linux 與終端目標平台 | 執行檔同層目錄 |
| iOS 與 tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

套件會根據執行檔的位置（而非工作目錄）從該目錄載入函式庫。預先建置相依函式庫時，請確保其關聯函式庫能從發佈所在的同一目錄載入。請宣告每一個所需的關聯檔案，以及載入器需要的每一個檔案名稱。

NSGo 在重複使用快取的下載檔案前會再次檢查其完整性，並透過檔案鎖定來協調並行建置。內容快取位於作業系統的使用者快取目錄下的 `nsgo/natives`。本機檔案則會在複製時進行驗證。

通用（universal）macOS 套件要求兩種架構使用相同的函式庫名稱。NSGo 會使用 `lipo` 合併這兩個架構切片；若發佈者提供的函式庫本身已是通用二進位檔，亦可同時用於兩個項目。函式庫會在應用程式之前完成簽署，隨後再通過應用程式正常的磁碟映像檔製作、簽署與公證步驟。即使設定了 `build.installers: false`，Windows 簽署仍會包含已宣告的 DLL。Linux 安裝程式則會包含與執行檔目錄相同的檔案與聲明。終端目標平台會對 macOS 函式庫進行臨時（ad hoc）簽署。

開發模式、`-run` 與目標平台測試皆使用相同的套件選取與複製步驟，桌面平台以及 iOS 與 tvOS 皆然。系統會根據建置的目標平台、標籤與產生的 Go overlay（包含本機替換與 vendor 套件）來選取宣告內容。Android、鴻蒙、watchOS、Web 與小程式不會從此檔案打包任何內容。

## iOS 與 tvOS 框架

NSGo 會檢查每個框架的二進位檔，不符合以下條件時便停止建置並說明原因：

- 是為目標平台（實機或模擬器）建置的 arm64 Mach-O 動態函式庫，以其 `LC_BUILD_VERSION` 為準；通用檔案取其 arm64 切片；
- 最低系統版本不高於應用程式的最低版本，即 `apple.info.MinimumOSVersion`，未設定時為 iOS 15.0 與 tvOS 17.0（[建置與發佈](building.md#nsgoyaml)）；
- 安裝名稱為 `@rpath/<Name>.framework/<Name>`；
- 透過 `@rpath` 載入的框架，皆為應用程式的套件為同一目標平台宣告的框架。

NSGo 會為框架寫入 `Info.plist`，其 `CFBundleIdentifier` 為應用程式 ID 加一個句點再加小寫的 `Name`，並帶有平台與應用程式的最低系統版本。模擬器建置會對框架進行臨時簽署。實機建置則將框架交由應用程式的簽署流程處理：`sign.profile` 會在簽署應用程式之前以應用程式的身分為其簽署，Xcode 在實機上執行 `nsgo build -run` 與 `nsgo dev` 時亦然。會載入關聯框架的框架應帶有 `@loader_path/..` 執行路徑，以便在 `Frameworks` 中找到它們。

當應用程式連結了宣告 iOS 框架的套件時，`nsgo build -embed` 會停止並指出該套件：供宿主應用程式使用的 XCFramework 目前還無法攜帶這些框架。

## 執行 vendor 時保留檔案

Go 的 vendor 操作需要透過內嵌資料來保留套件目錄底下的檔案。套件可以在執行時期不實際讀取這些位元組的情況下，將其資訊清單與授權檔案標記為納入範圍：

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

使用 `path` 時，也請將本機函式庫路徑納入該 embed 指示詞中，例如 `lib/*`。使用 URL 下載時，Go 套件內只需要資訊清單與授權檔案即可。
