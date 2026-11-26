---
title: 建置與發佈
description: 為各個平台打包應用程式、製作安裝程式、簽署與公證、在 GitHub 上發佈以及嵌入至其他應用程式。
group: 工具鏈
order: 22
---

## 建置

```sh
go tool nsgo build
```

`build` 會為 `nsgo.yaml` 列出的每個目標平台產生經過裁剪（stripped）的發佈封裝檔，輸出至 `dist/<name>-<version>-<target>`：包含 `.app`、`.ipa`、`.apk`、Windows 與 Linux 的目錄，或是網站檔案（其頁面會在 `import()` 執行時才擷取對應的 TSX 模組）。`-target` 會取代該列表，其中 `all` 代表所有僅需 Go 與 Xcode 即可建置的目標；若系統已安裝 Android NDK，可加上 `android_x64`；[小程式](#小程式)可指定 `miniprogram`，或單獨指定 `miniprogram_wechat`、`miniprogram_qq`、`miniprogram_alipay` 或 `miniprogram_douyin`；[HarmonyOS](#harmonyos) 預覽則可使用 `harmonyos`。`-run` 會啟動為本機建置的封裝檔，或將其安裝至由 `-device` 選定的 iPhone、模擬器或 Android 裝置上。

所有用於發佈的檔案皆會彙整於 `dist/releases`，這是唯一需要檢視的目錄：

| `dist/releases` 中的檔案 | 用途 |
| --- | --- |
| `*.dmg`、`*-setup.exe`、`*.deb`、`*.pkg.tar.gz`、`*.AppImage` | 適用於 macOS、Windows 與 Linux 的[安裝程式](#安裝程式) |
| `*.AppImage.zsync` | 供 AppImageUpdate 與相容啟動器使用的差異更新控制檔 |
| `*-ios.ipa`、`*-tvos.ipa`、`*.apk` | 適用於商店或實機的 iOS、tvOS 與 Android 封裝檔 |
| `*.pkg`、`*.msix`、`*.aab`、`*.flatpak`、`*.tar.gz` 與 `flatpak/` | `store` 渠道建置出的[商店封裝檔](#商店封裝檔) |
| `install.sh`、`install.ps1` | 用於從 [GitHub Releases](#在-github-上發佈) 安裝應用程式的腳本 |
| `appcast-<target>.xml` | 應用程式讀取的[更新](#更新)訂閱源（feed） |
| `homebrew/<name>.rb`、`winget/` | 直接發佈用的 [Homebrew cask 與 winget 清單](#渠道) |
| `*-web.zip` | 網站壓縮檔，解壓後即可直接作為靜態網站提供服務 |
| `miniprogram/<platform>/` | 各平台的小程式專案，可直接透過對應開發者工具上傳 |

每個檔案皆以 `<name>-<version>-<system>-<arch>` 命名（例如 `hello-1.2.0-windows-x64-setup.exe`），方便使用者在發佈頁面上找到符合自己系統的檔案，且旁邊都會附帶對應的 `.sha256` 檢查碼檔案。建置不同版本時會先清空該目錄；相同版本的建置（包含在其他機器上建置）則會累加至同一目錄中。模擬器與 watchOS 的封裝檔則保留在各自原有的 `dist/<name>-<version>-<target>` 目錄下。

網站目錄在預壓縮檔案比原始模組小時，會包含 `.wasm.br` 與 `.wasm.gz`。在支援預壓縮檔案的主機上請啟用此功能，並於原始 `.wasm` 網址路徑下以 `Content-Type: application/wasm`、對應的 `Content-Encoding` 以及 `Vary: Accept-Encoding` 提供服務。原始模組本身亦可在任何靜態主機上運作。[在網頁上發佈](web.md)涵蓋語意標題、瀏覽器連結、單頁中繼資料與 CDN 載入。

### 離線網頁封裝

在 `nsgo.yaml` 中設定 `web.offline: true`，即可讓網頁發佈版本在首次線上啟動時進行快取。

```yaml
web:
  offline: true
```

透過 HTTPS 提供發佈目錄，或使用 `go tool nsgo build -target web -run` 在本機 localhost 測試。Service Worker 會快取網頁、WebAssembly、腳本與打包資源。線上造訪會載入已發佈的最新版本。當安裝成功且先前版本的所有分頁都關閉後，最新版本即可從快取離線啟動。若要存取應用程式的線上服務，仍需網路連線。

## 終端機模式

使用 `go tool nsgo dev -target tui` 可在互動式終端機中執行應用程式。`go tool nsgo test -target tui` 則會透過 `nsgotest.Main` 在終端機中執行測試，每個測試行程執行一個應用程式。終端機決定整體尺寸：每個字元儲存格代表 8 × 16 版面配置點。請明確選擇應用程式的樣式；各元件仍保有其原有的形式與尺寸。`examples/tui` 示範了輸入欄位、按鈕、快捷鍵與視窗調整大小。

使用 `go tool nsgo build -target tui_macos_arm64`（或對應的 `tui_macos_x64`、`tui_windows_arm64`、`tui_windows_x64`、`tui_linux_arm64` 或 `tui_linux_x64`）可建置主控台執行檔。`tui` 建置/檢查群組包含這六種目標，`all` 群組亦同。每個套件目錄內含執行檔與資源，沒有圖形介面安裝程式。這六種目標皆可僅憑 Go 工具鏈完成建置。

此後端支援儲存格文字、背景、實線邊框、裁剪、捲動、焦點與文字編輯。它使用終端機本身的字型與顏色；包含 Mac 在內，Control 皆為 `mod` 鍵。Ctrl+C 在編輯器內會執行複製，若無任何繫結或編輯器接手則會結束程式。括號貼上（bracketed paste）會直接插入文字而不觸發快捷鍵，剪貼簿僅限於行程內部使用。畫布、圖片、媒體、原生託管元件、系統選單/警告對話框、動畫、縮放、雙向文字塑形、開頭/中間省略號與系統剪貼簿橋接目前尚未實作。`base/terminal` 仍是用於嵌入 GUI 應用程式中的終端機控制項。

當 `nsgo dev` 接管終端機時，工具與應用程式的診斷訊息會輸出至 `.nsgo/tui.log`。結束時會先還原螢幕、游標與終端機模式再返回 shell。終端機測試截圖是透過固定的可攜式字型將儲存格光柵化而得，而非直接擷取終端機模擬器畫面；對於該字型缺少的 Unicode 字元，請使用文字斷言進行測試。

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

在 `targets` 中（或在 `nsgo build -target` 與 `nsgo check -target` 中）僅寫作業系統名稱，代表包含其所有架構（`windows` 即代表 `windows_arm64` 與 `windows_x64`；`android` 僅代表 `android_arm64`，因為 x86_64 建置需要 NDK）。`macos_universal`（未包含在 `macos` 與 `all` 中）會同時建置兩種 macOS 架構並將其合併為單一應用程式，包含其函式庫與小工具。Steam 渠道會使用通用應用程式套件。安裝程式建置會將其打包為單一磁碟映像檔，並在直接發佈渠道上產生單一 Sparkle 訂閱源。`nsgo test -target` 也接受測試執行的目標架構名稱：桌面作業系統對應本機架構，Android 對應 `arm64`，因此在 x64 Windows 機器上執行 `windows_x64` 會執行 `windows` 目標測試，而若指定該目標無法執行的架構則會被拒絕並提示應使用的名稱。

命令列旗標會覆寫對應名稱的設定，例如 `-target`、`-version` 或 `-output`；執行 `-h` 可列出所有旗標。

`build.ldflags` 存放應用程式自訂的連結器旗標，寫法與 `go build` 的 `-ldflags` 相同，例如 `-X main.releaseDate=2026-11-17` 或 `-X 'main.channel=beta builds'`。每個連結應用程式的建置都會套用它們：包含 `dev` 及其重新載入、`build`（含 `-embed`）以及所有目標上的 `test -target`。在 `build` 與 `test` 上傳入的 `-ldflags` 會附加於設定之後而非取代，可重複指定多個。它們位於工具自身 `-X` 旗標之前，且連結器會保留同一變數的最後一個值，因此 `core.App` 中的名稱、ID 與版本號仍會維持 `nsgo.yaml` 的設定。工具會傳遞自身的 `-ldflags` 給 `go`（這會取代 `GOFLAGS` 中的設定），因此連結器旗標應寫在此處；而在無頭模式 `test` 中位於目錄參數之後的 `-ldflags` 則是 `go test` 的參數，會全部取代原有設定。

工具為應用程式執行的每個 `go` 命令都會帶入 `build.tags`，涵蓋 `dev` 及其重新載入、`build`（含 `-embed`）、`test -target` 與 `check`，適用於所有目標平台。NSGo 自身的標籤（例如 `dev` 中的 `nsgodev` 或目標測試中的 `nsgotest`）會加入其中，`GOFLAGS` 中 `-tags` 的標籤也會一併保留以免被覆寫；`GOFLAGS` 的其餘旗標則原封不動套用。

`apple.info.LSMinimumSystemVersion` 會提高 macOS 封裝檔的最低系統版本需求，而 `apple.info.MinimumOSVersion` 則會提高所選 iOS、tvOS 或 watchOS 封裝檔的最低版本需求。請填寫加引號的版本字串，例如 `"14.2"` 或 `"18.0"`，包含一至三個數字部分。執行階段的最低底線分別為 macOS 13.0（Go 1.27）、iOS 15.0、tvOS 17.0 與 watchOS 26.0；低於底線或格式錯誤的值將會失敗。行動裝置鍵值必須滿足所有已選取行動目標的底線。App Clip 保留自身的 `app_clip.minimum`。這些數值會套用至封裝中繼資料、資源編譯、簽署專案以及應用程式的連結器；嵌入的 iOS 框架也會使用這些數值，其中 Mac Catalyst 為支援 Go 的 macOS 13 執行階段，最低須為 16.0。

`icon.windows` 與 `icon.linux` 用於為各自的應用程式封裝檔、安裝程式圖示與商店標誌選擇正方形 PNG 圖片。預設路徑為 `assets/IconWindows.png` 與 `assets/IconLinux.png`，若不存在則退回使用 `icon.png`；`dev.icon` 接受相同的設定與 `-dev` 預設檔名。`icon.android.png` 可獨立選擇 Android 點陣圖圖示，即使未設定自適應圖層亦有效。其他目標則保留共用的 `icon.png` 作為後備。

Apple TV 的圖示是一張分層卡片而非正方形：`tvos` 接受 Xcode 的 `.brandassets`（包含其 `App Icon` 圖層堆疊與 Top Shelf 圖片），或是一張寬幅圖片（如 1280×768），工具會將其縮放填滿卡片並依邊緣顏色襯底，同時置中於 Top Shelf 圖片上。若兩者皆無，則會將 PNG 置於卡片高度四分之三大小的位置。Apple Watch 會將圖示裁剪為圓形，因此 `watchos` 接受專為 watchOS 圓形繪製的 `.icon` 或正方形 PNG；若未設定，則依序退回使用 `apple` 與 `png`。兩者皆會如同 `apple` 與 `png` 一樣，自動尋找預設檔案 `assets/AppIconTV.*` 與 `assets/AppIconWatch.*`。

在 Android 上，`icon.android.foreground` 與 `background` 會組成自適應啟動器圖示（adaptive icon），讓啟動器能將其裁剪為各自形狀並套用動態位移，而不是將 `png` 縮小貼在預設底板上。每個圖層尺寸為 108 dp 正方形（即縮放至 432 px 的 PNG）；啟動器會顯示中央 72 dp（288 px）區域，並可能裁掉中央 66 dp（264 px）圓圈以外的所有內容，因此請將標誌保留在該圓圈內，並讓背景填滿整個正方形。`background` 可以是 PNG 圖片或色碼（例如 `"#1F1F1F"`）。`monochrome` 是一個僅看 Alpha 透明度的形狀圖層，供 Android 13 及以上版本在使用者開啟主題圖示時進行著色；若未提供，該圖示將維持原色。封裝檔會保留 `icon.android.png`（否則為 `icon.png`）作為一般點陣圖圖示，不過 Android 9（NSGo 支援的最舊版本）以上的啟動器都會優先採用自適應圖示；若未提供 `png`，工具會將前景疊加在背景上繪製產生。若未設定自適應圖層，啟動器就會顯示 Android 點陣圖或共用的 `png`。

`android.uses_cleartext_traffic: true` 會允許 APK 或 App Bundle 向任意主機發送未加密的明文 HTTP 請求，同時設定 manifest 旗標與網路安全性設定。若未開啟此項，`android.loopback_cleartext: true` 則僅允許對 `localhost`、`127.0.0.1` 與 `::1` 使用明文 HTTP（例如由應用程式自身提供服務的影片）。僅限本機迴圈的原則會拒絕其他主機的明文流量；當兩項設定皆為 true 時，允許所有主機的設定優先。對於嵌入式的 AAR，請在宿主應用程式的 Android 網路安全性設定中進行組態。

每個建置在啟動時也都接受 NSGo 自身的命令列旗標，例如 `--nsgo-fps`（[執行與除錯](debugging.md#所有建置共通的旗標)）。在 `nsgo dev` 中，Command/Control+R 可從任一開啟的視窗觸發重新載入，且即使主視窗關閉，只要仍有其他視窗保持應用程式函式執行，此快捷鍵依然有效。

### 啟動畫面

設定 `launch_screen` 可在 iOS 與 Android 12 及以上版本顯示不透明背景、置中 PNG 圖片，或兩者兼有。圖片路徑相對於 `nsgo.yaml`。圖片會等比例縮放至 128 × 128 點或 dp 範圍內。使用長邊至少 512 像素的透明 PNG 可在高密度螢幕上保持銳利。

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

除非 `appearance` 鎖定了外觀，否則系統會自動選擇淺色或深色外觀。省略 `dark` 中的任一欄位時會沿用對應的淺色設定值。省略背景或圖片則會保留平台的預設行為。嵌入式應用程式的啟動畫面由宿主應用程式提供。

## Android 原生類別

在應用程式的 `nsgo.yaml` 中列出預先編譯的 Android 類別，即可在 `core.Native` 中使用自訂 View。

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

除非使用絕對路徑，否則路徑皆相對於 `nsgo.yaml`。每個路徑指向一個 `.dex` 檔案，或包含 D8 根目錄 `classes.dex`、`classes2.dex` 及後續分片的 JAR 檔案。請使用 D8 搭配 `--min-api 28` 準備這些檔案。在 arm64 上，NSGo 僅憑 Go 即可從這些預先編譯的檔案建置出 APK 或 App Bundle。當列出的檔案變更時，Android 開發工作階段會自動重新建置。對於嵌入式的 AAR，由宿主應用程式的 Android 建置負責提供其 Java 相依性。[D8 文件](https://developer.android.com/tools/d8)

[Android 原生範例](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native)示範了如何註冊一個帶有公開 `Context` 建構函式的類別，並透過 Go prop 進行組態。

## 遊戲

針對遊戲應用程式，可在 `nsgo.yaml` 中設定 `game: true`。Android 會將應用程式歸類為遊戲並啟用系統的遊戲模式最佳化。macOS 與 iOS 會啟用遊戲模式，macOS 會預設歸類為「遊戲」類別，而 Linux 則會在桌面項目中加入 `Game`。若明確設定了 `macos.category` 或 `apple.info` 中的遊戲模式鍵值，則以明確設定優先。

當應用程式需要處理硬體遊戲控制器時，請設定 `gamepad: true`。Android 會將遊戲控制器與觸控螢幕皆宣告為選用。macOS、iOS 與 tvOS 會宣告 Extended Gamepad 設定檔，tvOS 同時還會宣告 Siri Remote 的 Micro Gamepad，若 `apple.info` 中另有設定控制器鍵值則以其為準。

## 授權聲明

`nsgo build` 會在獨立封裝檔的資源中放入 `THIRD_PARTY_NOTICES.txt`。其中收錄了該目標所使用之 Go 模組、Go 執行階段，以及 NSGo 內建 Java 或 JavaScript 執行階段的授權條款全文。網頁建置會像其他資源一樣以內容雜湊命名該檔案。呼叫 `core.ThirdPartyNotices()` 可在應用程式內的致謝畫面顯示相同文字。在開發模式、測試以及嵌入式函式庫建置中，該函式會回傳空字串。[原生函式庫宣告](native-libraries.md)會將其原始授權條款加入同一份聲明中。

預設情況下，未知或缺少第三方授權會導致建置失敗，偵測到 Copyleft 授權則會發出警告。列於應用程式 Go 工作區（`go.work`）中的模組會與應用程式自身模組一同視為第一方程式碼：其中包含的任何授權檔案皆會被收錄，且不會觸發第三方授權檢查。可將任一規則設為 `error` 或 `warn`。`allow` 清單可將精確匹配的模組路徑豁免於這兩項規則，同時仍保留建置時找到的授權全文。

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## 新增函式庫

用 `nsgo add` 新增函式庫，也就是一個 Go 模組。它是先顯示計畫的 `go get`：

```sh
nsgo add github.com/acme/chart           # 最新的正式版本；go.mod 已有時保持不變
nsgo add github.com/acme/chart@v1.3.0    # 這個版本，即使低於 go.mod 中的版本
nsgo add -dry-run github.com/acme/chart  # 只印出計畫，不寫入任何東西
```

```text
github.com/acme/chart
  module     github.com/acme/chart v1.3.0, published 2026-11-20
  owner      github.com/acme, the first module from it in go.mod
  next       import "github.com/acme/chart"

example.com/hello
  go.mod     + require github.com/acme/chart v1.3.0 // indirect
             + require example.com/units v1.0.0 // indirect
             ~ require github.com/nsgo-dev/nsgo v0.4.0 => v0.4.2, a release
  go.sum     + 6 lines
  tools      go tool nsgo then runs v0.4.2, and so does an installed nsgo here
Apply these changes? [y/N]
```

對每個模組，計畫列出它的版本與發布日期，以及它的擁有者：主機，在 github.com 這類程式碼代管服務上還包括帳號；`go.mod` 中還沒有來自這個擁有者的模組時會特別註明。適用時，接著列出撤回（retract）與棄用通知、沒有發布標籤的版本、對該模組的 `replace`，以及其根套件的 import 行。然後是 `go.mod` 中改變的行（包括 `go` 與 `toolchain` 行）、之後會執行另一個版本的 `go tool` 命令、從模組自身主機取得的模組，以及 `go.sum` 收下卻未經總和檢查碼資料庫驗證的模組。`go get` 會把還沒有任何套件匯入的模組標為 `// indirect`，等有套件匯入它之後，`go mod tidy` 會去掉這個標記。

在終端機中，`add` 寫入前會先詢問，`-y` 則不經詢問直接套用計畫；不在終端機中又沒有 `-y` 時，它什麼也不寫，並以狀態 1 結束。以 Ctrl+C 等方式中斷時，`go.mod` 和 `go.sum` 維持原樣，並以狀態 130 結束。`go.mod` 已經依賴的模組維持原樣，只印出其版本，除非輸入了版本。

規劃只執行 go 命令，作用於 `go.mod` 與 `go.sum` 的副本：

- 使用模組已在用的工具鏈並設定 `GOTOOLCHAIN=local`，因此不會下載或執行其他工具鏈；
- 只經由 `GOPROXY` 中第一個 `direct` 之前列出的模組代理取得，`direct` 改為 `off`，因此只有 `GOPRIVATE` 或 `GONOPROXY` 標記的模組會從它們自己的主機取得，`GONOSUMDB` 與 `GOAUTH` 保持不變；
- 設定 `GOWORK=off`，因此在工作區中，它只為這個模組本身規劃 `go.mod`，與 `GOWORK=off go get` 寫出的一致，並在計畫中註明。

無論是否給出 `-y`，`add` 都會拒絕：

- 把 NSGo 的依賴移到虛擬版本或預先發布版本，依賴 NSGo 某個提交的函式庫就會這樣做；要刻意這樣做，先執行 `go get github.com/nsgo-dev/nsgo@<版本>`；
- 需要比模組的工具鏈更新的 Go 的模組；先用 `go get go@<版本>` 提高模組的 Go 版本；
- 經由 `@latest` 這類查詢降低模組的版本；輸入具體版本才會降低，計畫稱之為降級。

只有在規劃之後 `go.mod` 與 `go.sum` 都沒有改變時，它才寫入這兩個檔案，各自先寫入暫存檔再改名覆蓋；發生錯誤或被中斷時，兩個檔案都會還原。有 `vendor` 目錄的模組，計畫會把 `go mod vendor` 列為下一步。

`add` 是 NSGo 0.3.4 之後新增的。在固定為 0.3.4 或更早正式版本的模組中，已安裝的 `nsgo` 不會為它執行模組的 `nsgo`，而是印出引入 `add` 的 `go get github.com/nsgo-dev/nsgo@<版本>`；在那裡，`go tool nsgo add` 執行的是固定的 `nsgo`，它不認識 `add`。

## 來自 Go 套件的函式庫

匯入 `base/terminal` 的應用程式同時會隨附 libghostty-vt：Windows 與 Linux 封裝檔會將該函式庫置於執行檔旁；macOS 與 iOS 套件則會將其簽署並包入框架中。[嵌入指南](embedding.md)說明了 iOS 宿主所需的配套 XCFramework。

相依套件可以在 `nsgo-natives.yaml` 中宣告預先建置的桌面共用函式庫。NSGo 會驗證其 SHA-256 雜湊值、在簽署前將其複製進應用程式，並將其納入安裝程式中；應用程式作者無需執行任何額外步驟。[原生函式庫](native-libraries.md)說明了套件作者使用的格式與載入路徑。

## 命令列程式

應用程式可以在桌面執行檔旁隨附命令列程式，例如用來處理應用程式資料的 CLI 工具。將它們列於 `commands` 下，每個皆從一個 Go main 套件建置：

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

名稱僅限英文字母、數字、`-` 與 `_`，且在任何大小寫下皆不得與應用程式執行檔、其 Linux 命令、`Uninstall`、`AppRun` 或其他命令名稱重複。`macos_universal` 會像合併應用程式一樣，將每個命令的兩種 macOS 架構建置合併為單一檔案。

套件連結時會先套用 `build.ldflags` 與 `-ldflags`，接著套用命令自身的 `ldflags`（其 `-X` 值優先），最後套用應用程式身分資訊，因此 `core.App()` 會像在應用程式中一樣，為命令提供 `nsgo.yaml` 裡的名稱、ID、版本與組建編號。未匯入 `core` 的命令則可透過 `-X` 取得版本號，例如在 `build` 時傳入 `-ldflags "-X main.version=1.2.0"`，應用程式與其命令皆會接收該旗標。

| 封裝 | 命令列程式處理方式 |
| --- | --- |
| macOS | 置於 `Contents/MacOS` 中，在簽署主應用程式前照原樣以 `<id>.<command>` 進行簽署：採 ad hoc 簽署，或使用 `sign.identity` 搭配強化執行階段（hardened runtime）與時間戳記以讓應用程式通過[公證](#公證)，但不帶主應用程式的權限宣告。磁碟映像檔不會設定 `PATH`；[Homebrew cask](#渠道) 會建立每個命令的符號連結，使用者也可自行連結 `/Applications/<name>.app/Contents/MacOS/<command>` |
| Windows | `<command>.exe` 置於 `<name>.exe` 旁，並依[簽署 Windows 封裝檔](#簽署-windows-封裝檔)的方式照原樣簽署，安裝程式會安裝它並在解除安裝時將其移除；搭配 `windows.path` 時，安裝程式也會將應用程式資料夾加入使用者的 `Path`（如同 `windows.command_line`），使安裝後開啟的主控台能直接找到這些命令。若更新時重新命名或移除了某個命令，該檔案會像其他應用程式不再隨附的檔案一樣留在資料夾中，因為解除安裝程式只會移除該版本安裝時所寫入的檔案 |
| Linux | 置於 `/opt/<slug>` 中並於 `/usr/bin/<command>` 建立由 `.deb` 與 Arch 套件擁有的連結，因此移除套件時會一併移除該連結，且 dpkg 與 pacman 會拒絕覆寫其他套件的檔案；AppImage 則將其保存在應用程式旁，不會更動 `PATH` |

安裝程式、檢查碼以及[更新](#更新) appcast 皆會像處理應用程式其他檔案一樣包含這些命令，因此在更新時會隨應用程式一併替換。[商店封裝檔](#商店封裝檔)則不會包含它們，因為 Mac App Store 的沙盒、MSIX 與 Flatpak 僅允許執行應用程式自身的主執行檔。

## 安裝程式

針對桌面目標平台，`build` 會在 `dist/releases` 中產生供使用者安裝應用程式的檔案，每個檔案皆附帶 `.sha256` 檢查碼：

| 目標平台 | 安裝程式 |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`，這是一個顯示應用程式、箭頭與 Applications 連結的磁碟映像檔，方便使用者拖曳安裝，並使用 `sign.identity` 簽署及進行[公證](#公證) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`，為原生 Windows 安裝程式，會為使用者安裝應用程式（若先前已安裝則安裝於原本選定的資料夾，並關閉執行中的應用程式），附帶「開始」功能表捷徑、可選的桌面捷徑以及僅移除其所安裝檔案的解除安裝程式，並於設定了 [`sign.certificate`](#簽署-windows-封裝檔) 時隨應用程式一併簽署；`windows.command_line` 亦會安裝 `.com` 輔助程式並將應用程式資料夾加入使用者的 `Path`，使安裝後開啟的主控台能直接執行應用程式命令，而同樣能找到該輔助程式的 Win+R 則會在啟動應用程式後自動關閉開啟的主控台；解除安裝程式僅在該資料夾是由安裝程式加入時才會將其從 `Path` 移除，絕不會動到使用者原本自訂的路徑項目 |
| Linux | 適用於 Debian 與 Ubuntu 的 `<name>-<version>-linux-<arch>.deb`，以及適用於 Arch Linux 的 `<name>-<version>-linux-<arch>.pkg.tar.gz`，兩者皆會將應用程式安裝於 `/opt/<slug>` 並包含命令、其[命令列程式](#命令列程式)連結、桌面項目與圖示（Arch 套件包含供 `pacman -Qkk` 驗證的 `.MTREE`）；另有 `<name>-<version>-linux-<arch>.AppImage`，無需安裝即可在任何發行版上執行 |

Windows 安裝程式與解除安裝程式僅需 Go 即可在任何系統上建置，並使用原生 Windows 對話框。它們會支援應用程式語系目錄中已有對應翻譯的語言（包含簡體中文與繁體中文），並自動選用系統的介面語言；`/LANG=<LCID>` 可為單次執行指定語言，例如以 `/LANG=2052` 指定簡體中文。`/S` 會以靜默方式安裝或解除安裝，置於最後的 `/D=<folder>` 可指定安裝資料夾（路徑可包含空格），而 `/RELAUNCH` 會在更新後開啟應用程式。若未指定 `/D`，升級時會安裝於所取代之既有安裝的資料夾。傳入 `-installers=false` 或設定 `build.installers: false` 則只建置封裝檔而不製作安裝程式。

安裝程式可採用應用程式自訂的圖片與圖示；若未設定則使用應用程式圖示或內建預設圖片：

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

若為背景 PNG 提供兩倍尺寸的雙生檔案（例如 `DMGBackground@2x.png`），在 Retina 螢幕上便能保持清晰銳利。應用程式圖示位於左側四分之一處，Applications 位於右側四分之一處，各自連同名稱在垂直方向置中（內建背景圖在此處繪有箭頭）；使用自訂背景圖時請為它們預留空間。Linux 套件沒有專屬安裝圖片，它們會直接使用應用程式的圖示。

Linux 套件亦會安裝應用程式的 AppStream 中繼資料 `/usr/share/metainfo/<id>.metainfo.xml`，供軟體中心列出應用程式；其內容由 `linux.summary`、`linux.license`（若有設定）以及應用程式名稱寫成，若 `linux.categories` 未指定分類則預設為 `Utility`。AppImage 會將其置於 AppDir 的 `usr/share` 中，並附帶指向 `usr/share/applications` 內桌面項目的連結。其他系統從 `/usr/share` 讀取的檔案（例如更豐富的 AppStream 中繼資料、額外桌面項目、shell 自動補全或 man 手冊頁）可放在 `linux.share` 指定的目錄中（相對於 `nsgo.yaml`）：`.deb` 與 Arch 套件會將其目錄樹安裝至 `/usr/share`，AppImage 則置於其 AppDir 的 `usr/share`。其中的 `metainfo/<id>.metainfo.xml` 會取代 NSGo 自動產生的檔案；但若包含套件本身會寫入的路徑（例如應用程式的桌面項目或圖示），建置將會失敗。無論如何命名，`resources/` 中的檔案皆會留在 `/opt/<slug>` 內的執行檔旁，而商店建置的 Flatpak 僅採用 NSGo 產生的中繼資料。

AppImage 需要 FUSE（大多數發行版皆已內建），且如同其他套件一樣使用系統的 GTK 4；其執行時期會在首次使用時下載並快取於使用者快取目錄中。

AppImage 會使用 `release.github` 或 GitHub 遠端 `origin` 內嵌一個 `gh-releases-zsync` 定位器，以選取最新穩定版及此應用程式對應的架構。請將每個 `.AppImage` 連同產生的 `.AppImage.zsync` 檔案一併上傳。若託管於其他主機，請將 `linux.appimage_update` 設為 `zsync|https://example.com/latest.AppImage.zsync`（一個支援 HTTP Range 請求的固定網址）；它也接受 `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` 以指向預先發佈（prerelease）渠道。明確設為空字串可停用內嵌探索；若沒有 GitHub 儲存庫或未設定定位器，此欄位亦為空。`.zsync` 檔案仍會照常產生，並獨立於已簽署的 appcast 之外。

## 商店封裝檔

使用 `-channel store` 或 `updates.channel: store` 會為各平台商店建置桌面與 Android 目標（由商店負責更新，藉此取代自行安裝程式與安裝腳本）：

| 目標平台 | 商店封裝檔 |
| --- | --- |
| macOS | 供 Mac App Store 使用的 `<name>-<version>-macos-<arch>.pkg`：應用程式運行於 App 沙盒中，以描述檔的身分憑證簽署，並以團隊的 Installer 身分憑證打包 |
| Windows | 供 Microsoft Store 使用的 `<name>-<version>-windows-<arch>.msix`，發佈時將由商店進行簽署 |
| Linux | 可直接安裝的 `<name>-<version>-linux-<arch>.flatpak`，外加附帶 AppStream 資料的 `.tar.gz` 與適用於 Flathub 或 flatpak-builder（基於 GNOME 執行階段）的 `flatpak/<id>.yml` |
| Android | `<name>-<version>-android-<arch>.aab`，供 Google Play 使用、以 `sign.keystore` 上傳金鑰簽署的 App Bundle |

MSIX 會從語系目錄中的 `appName` 讀取各語言的顯示名稱，若無則退回使用來源語言。其「開始」功能表、工作列與 Store 標誌包含 100%、125%、150%、200% 與 400% 變體。工具可在 macOS 上直接產生資源索引，無需 Windows SDK。

在 `store` 渠道下指定 `-target macos` 會同時選取兩種 macOS 架構並輸出單一 `<name>-<version>-macos-universal.pkg`，效果與 `-target macos_universal` 相同。指定 `-target android_arm64,android_x64` 則會輸出同時包含 `arm64-v8a` 與 `x86_64` 的單一 `<name>-<version>-android-universal.aab`。其中 x86_64 切片需要 Android NDK。

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

未設定 `android.target_sdk` 時，執行階段預設以 API 36 為目標。低於 API 36 的 Android App Bundle 建置將會失敗，因為 [Google Play 要求](https://support.google.com/googleplay/android-developer/answer/11926878)自 2026 年 8 月 31 日起所有新行動應用程式與更新皆須符合此標準。獨立 APK 建置則僅會發出警告並繼續建置。

設定 `sign.macos_profile` 即可簽署供上傳使用的 Mac 封裝檔。在沒有小工具的情況下，未簽署的套件仍可在沙盒中執行，方便你測試對應用程式容器、網路以及使用者選取檔案的存取。包含小工具的商店建置則必須同時提供 `<id>` 的 `sign.macos_profile` 以及 `<id>.widgets` 的 macOS 版 `sign.widgets_profile`，兩者須屬於同一團隊並授予 `apple.app_group`（預設為 `group.<id>`）。兩個套件皆會使用該共用容器；不會新增家目錄例外權限。`-installers=false` 會保留沙盒與描述檔簽署，僅略過產生 `.pkg` 與其 Installer 身分簽署。請分別在合作夥伴中心（Partner Center）上傳 `.msix`，並在 Play Console 上傳 `.aab`。當 GitHub 發佈包含 `flatpak/<id>.yml` 所指名的封存檔後，即可將其提交至 Flathub。`flatpak-builder` 亦可在本地建置。可使用 `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak` 安裝套件；系統必須已安裝或可從已設定的遠端取得對應的 GNOME 執行階段。工具本身僅用 Go 即可建置套件，無需 Linux 工具鏈。當缺乏圖示美術圖檔時，商店封裝檔會使用透明圖示；若未設定 `linux.categories`，則預設使用 `Utility`。若要在商店之外安裝 MSIX，請設定 [`sign.certificate`](#簽署-windows-封裝檔) 來簽署 MSIX；其憑證主體必須與 `windows.store.publisher` 相符，且接收機器必須信任該憑證。若未設定簽署資訊，上傳至商店時將由商店代為簽署。

在 macOS 與 iOS 上，捲動檢視的邊緣效果（`Scroll.EdgeEffects`）、未設定 `IOS.BackdropMaterial` 的 macOS 背景模糊，以及背景色彩濾鏡（單獨使用，或疊加於未設定 `IOS.BackdropMaterial` 的模糊之上）皆透過 Core Animation 的私有 `CABackdropLayer` 與 `CAFilter` 繪製（如同 AppKit 與 UIKit 自身的材質與邊緣效果一樣），因為沒有公開 API 能依半徑進行模糊、在檢視上漸變模糊半徑，或套用背景色彩矩陣。App Store 審查可能會拒絕使用私有 API 的應用程式：NSGo 僅在帶有邊緣條的捲動檢視與上述背景濾鏡時才會呼叫它們，但所有 macOS 與 iOS 建置皆會包含其符號名稱。

`build` 產生的每個 Apple 套件（包含主應用程式、小工具擴充功能、App Clip、Apple Watch 應用程式以及 `-dynamic -embed` 框架）皆附帶隱私權清單 `PrivacyInfo.xcprivacy`，宣告 NSGo 與 Go 執行階段所使用的必要原因 API（required-reason API）：使用者預設值（`CA92.1`）、檔案時間戳記（`C617.1`）以及系統開機時間（`35F9.1`）。`apple.privacy` 可加入應用程式自身的宣告：`accessed_apis`（每個類別的原因代碼）、`tracking`、`tracking_domains` 與 `collected_data`（依 Apple 命名規則填入 `NSPrivacyCollectedDataTypes` 項目）；若 `resources/` 中已存在 `PrivacyInfo.xcprivacy`，則會與這些設定合併而非直接覆蓋。

`build` 為實機產生的每個 iOS、tvOS 與 watchOS 套件（主應用程式、小工具擴充功能與 App Clip）之 `Info.plist` 皆會帶入 App Store Connect 檢查的 SDK 與 Xcode `DT` 鍵值（取自建置所用的 Xcode），以及 `UIRequiredDeviceCapabilities: [arm64]`；模擬器套件則省略這些鍵值，且 `apple.info` 所設定的鍵值會保留其原值。`apple.multiple_scenes: true` 會在 iOS 應用程式的場景清單中啟用 `UIApplicationSupportsMultipleScenes`（預設為 false）：在 iPad 上使用者即可同時開啟多個應用程式視窗，每個視窗都會重新呈現應用程式主檢視並擁有各自獨立的狀態，同時共享應用程式的訊號與儲存空間，直到使用者關閉該視窗為止。

### 發佈至 Google Play

啟用 Google Play Developer API，並授予服務帳戶將應用程式發佈至指定測試軌道的權限。將 `NSGO_PLAY_SERVICE_ACCOUNT` 設為該帳戶的 JSON 金鑰內容或其金鑰檔案路徑。[Google Play API 設定指南](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

該命令會在 `build.output/releases` 中尋找符合專案版本的 App Bundle。當存在多個建置檔案時，可在旗標後方傳入 `.aab` 路徑以指定檔案。軌道可傳入 `internal`、`production` 或封閉測試的軌道 ID。命令會從套件中讀取套件名稱與版本代碼，要求版本代碼必須高於該軌道上的所有現有發佈版本，隨後上傳套件並提交發佈。

## 商店上架資訊

商店中繼資料（標題、副標題、說明、關鍵字、更新說明、網址、螢幕截圖與預覽影片）以純文字 YAML 與 Markdown 形式存放在版本庫的 `store/`（或 `nsgo.yaml` 中的 `store.dir`）目錄下：

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

兩個 `listing.yaml` 檔案皆必須包含 `version: 1` 且拒絕任何未知鍵值。除非透過 `app_store.locale` 或 `google_play.locale` 針對特定商店覆寫，否則標準語系目錄（如 `zh-Hans`）會自動對應至各商店的語系代碼（App Store Connect 為 `zh-Hans`，Google Play 為 `zh-CN`）。

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

驗證會在讀取憑證或發送任何 HTTP 請求前執行，檢查必填欄位、語系代碼、網址協定、媒體檔案與數量（App Store 每個顯示類型最多 10 張螢幕截圖與 3 部預覽影片；Google Play 每個圖片類型最多 8 張螢幕截圖），以及字元長度限制（App Store：標題 30、副標題 30、關鍵字 100、宣傳文字 170、說明 4000、更新說明 4000；Google Play：標題 30、簡短說明 80、完整說明 4000、更新說明 500）。

上傳憑證僅從環境變數讀取：App Store Connect 使用 `NSGO_APP_STORE_KEY`、`NSGO_APP_STORE_KEY_ID` 與 `NSGO_APP_STORE_ISSUER`（若未設定則退回使用公證金鑰 `NSGO_NOTARY_KEY`、`NSGO_NOTARY_KEY_ID` 與 `NSGO_NOTARY_ISSUER`）；Google Play 則使用 `NSGO_PLAY_SERVICE_ACCOUNT`（服務帳戶 JSON 檔案路徑或 JSON 純文字）。

### 商店合規申報

使用 `nsgo.yaml` 追蹤各項發佈問卷的完成狀態。

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

完成對應的問卷後，將項目設為 `true`。對應的 `-channel store` 或 `-channel steam` 建置會針對仍為 `false` 的項目發出提醒。若不打算在 YAML 中追蹤特定項目，可將其省略。

- 在 App Store Connect 中完成[年齡分級問卷](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating)與 [App 隱私權詳細資訊](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy)。
- 針對 Apple 的[加密出口合規問卷](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation)，請將 `apple.info.ITSAppUsesNonExemptEncryption` 設為代表應用程式答案的 YAML 布林值。若 Apple 核發了核准代碼，請以字串形式填入 `apple.info.ITSEncryptionExportComplianceCode`。你也可以省略這些鍵值並在提交時手動回答。Apple 商店建置會檢查這些鍵值的型別以及套件隱私權清單的結構。
- 在 Play Console 中完成[資料安全性](https://support.google.com/googleplay/android-developer/answer/10787469)表單。僅於內部測試軌道發佈的應用程式可豁免。
- 在提交遊戲審查前，於 Steamworks 完成[內容問卷](https://partner.steamgames.com/doc/gettingstarted/contentsurvey)。

## App Store Connect

在 App Store Connect 中建立應用程式，並使用 App Store 發佈描述檔簽署 iOS 實機 IPA，或使用團隊的 Installer 身分憑證簽署 macOS 商店 PKG。在裝有 Xcode 的 Mac 上，將 `NSGO_APP_STORE_KEY` 設為 App Store Connect 團隊金鑰的 `.p8` 檔案路徑或其內容，`NSGO_APP_STORE_KEY_ID` 設為金鑰 ID，`NSGO_APP_STORE_ISSUER` 設為發行者 ID；若未設定，則會像商店上架資訊一樣使用公證金鑰（`NSGO_NOTARY_KEY`、`NSGO_NOTARY_KEY_ID`、`NSGO_NOTARY_ISSUER`）。請賦予該金鑰存取該應用程式及上傳建置版本的權限。

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

工具會在上傳前檢查套件的組建編號，並回報 Apple 對 TestFlight 的處理結果。下次上傳時請遞增 `build_number`。macOS 的組建編號必須跨所有應用程式版本持續遞增，而 iOS 在新版本時可重新開始編號序列。處理完成後，請在 App Store Connect 中另行提交進行 App 審查。[Apple 發佈指南](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## 商店資源包

在 `mobile.asset_packs` 中宣告大型資源目錄。這些目錄中的檔案會與基礎 Android App Bundle 及 iOS 應用程式分開傳遞。資源包名稱必須以小寫英文字母開頭，且僅能包含小寫英文字母、數字與底線。

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

使用 `go tool nsgo build -channel store -target android_arm64` 建置 Android 資源包。使用 `bundletool build-apks --local-testing` 與 `bundletool install-apks` 在實機上測試套件。NSGo 支援開啟已安裝的 `install-time` 資源包。`fast-follow` 模組已包含於套件中以供 Play 傳遞，而透過 Play Asset Delivery SDK 讀取該類資源包的功能仍在待辦規劃中。

在 iOS 上，`go tool nsgo build -target ios` 會將 `install-time` 對應至 Apple 的 essential 原則，並將 `fast-follow` 對應至 prefetch 原則，同時適用於首次安裝與更新。需要 Xcode 26 或更新版本才能建置 Background Assets 下載器。請使用共用的 `apple.app_group` 或預設的 `group.<id>`。主應用程式的 `sign.profile` 與 `<id>.assetdownloader` 擴充功能的 `sign.asset_packs_profile` 兩者皆須包含該群組。請使用 App Store Connect 的資源包上傳工作流程，將 `dist/releases` 中的 `.aar` 檔案連同 IPA 一併上傳。[建立受管資源包](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

在 goroutine 中呼叫 `mobile.AssetPackReady(ctx, "levels")` 檢查資源包是否就緒，並呼叫 `mobile.OpenAssetPackFile("levels", "intro.map")` 開啟相對於宣告目錄的檔案。使用完畢後請關閉回傳的讀取器。[本機測試 Apple 資源包](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Steam Depot

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

設定 `updates.channel: steam` 可將其設為預設渠道。選取 macOS、Windows 與 Linux 目標。將每個 `dist/<name>-<version>-<target>` 目錄上傳至其對應的 Steam depot，並設定啟動選項：macOS 使用應用程式套件，Windows 與 Linux arm64 使用執行檔，Linux x64 則使用 `steam-launch.sh`。更新由 Steam 處理。這些目錄不包含安裝程式、自我更新器或 `steam_appid.txt`。

Linux x64 depot 包含適用於 Steam Linux Runtime 3.0 與 4.0 的 GTK 4 及其相依函式庫。僅當系統缺少 GTK 4.14 或更新版本時，啟動器才會使用它們。請將 `nsgo-gtk/` 保留在啟動器旁：其中的 `licenses/` 存放這些函式庫的授權條款、建置來源原始碼資訊與建置腳本，以符合 LGPL 對 GLib、Pango 與 GTK 的授權要求。Linux arm64 則需要執行環境中已提供 GTK 4。

若要在 Steam 用戶端之外進行本機測試，請在 `nsgo.yaml` 中設定 `steam: {app_id: 480}`（將 480 替換為你的 Steamworks App ID）。`go tool nsgo dev` 會將該 ID 寫入原生建置目錄中的 `steam_appid.txt`，並以該目錄作為工作目錄啟動應用程式。[Steamworks API](https://partner.steamgames.com/doc/api/steam_api)、[SteamPipe](https://partner.steamgames.com/doc/sdk/uploading)。

## 在 GitHub 上發佈

當建置包含 macOS、Windows 或 Linux 目標時，`dist/releases` 還會包含 `install.sh` 與 `install.ps1`，用於從 `release.github` 指定的 GitHub 儲存庫（若未設定則為應用程式 Git 遠端 `origin` 所在的儲存庫）下載並安裝應用程式；若兩者皆無，`build` 不會產生這些腳本並會提示說明。將除了 `miniprogram/` 以外的所有頂層檔案上傳至標籤為 `v<version>` 的發佈版本（`release.tag_prefix` 可替換 `v`；腳本會對該前綴中的保留網址字元進行編碼）：

```sh
gh release create v1.2.0 dist/releases/*.*
```

使用者接著只需執行建置時印出的一行指令即可安裝或更新應用程式：

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

在 macOS 上，腳本會將應用程式從磁碟映像檔複製至 `/Applications`（若無寫入權限則複製至 `~/Applications`）；在 Linux 上，會透過 `sudo` 以 `pacman` 安裝 Arch 套件，或以 `apt-get` / `dpkg` 安裝 Debian 套件；在 Windows 上，則會以靜默模式執行安裝程式，安裝至應用程式原先所在位置。每個腳本都會先依據檢查碼驗證檔案。將應用程式名稱大寫加上 `_VERSION`（例如 `HELLO_VERSION=1.1.0`）可安裝指定版本，而 `<NAME>_DOWNLOAD_URL` 則可改從鏡像站下載。

## 在 itch.io 上發佈

在 `nsgo.yaml` 中將 `itch.project` 設為你的 itch.io 遊戲的 `owner/game` 代稱。若要覆寫渠道名稱，可依建置目標設定 `itch.channels`。預設值分別為網頁版 `html5`、macOS `osx-<arch>`、Windows `windows-<arch>` 與 Linux `linux-<arch>`。

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

在 CI 環境中將 `BUTLER_API_KEY` 設為密鑰後，即可建置並上傳目標封裝。

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

若自訂了 `-output` 與 `-version`，請在兩個指令中傳入相同的參數。上傳作業會取得每個目標的套件目錄，並將其 itch.io 版本設為建置版本。NSGo 會使用 `PATH` 中的 `butler`，或自動從 itch.io 下載固定版本。首次上傳網頁版後，請將 itch.io 遊戲類型設為 HTML 並將上傳項目標記為可在瀏覽器中遊玩。[Butler 文件](https://itch.io/docs/butler/pushing.html)

## 更新

`updates` 可讓應用程式依照其平台與使用者取得來源的渠道方式自動保持最新狀態：

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` 會印出一組金鑰對且不會寫入任何檔案：將公鑰填入 `updates.key` 並提交至版本庫，私鑰則請勿放進版本庫，應存放在密碼管理器中，並在建置發佈的環境（如 GitHub Actions secret）設定為 `NSGO_UPDATES_KEY`。每次 `direct` 建置都會用私鑰簽署安裝程式，並在旁邊產生符合 [Sparkle](https://sparkle-project.org) 格式的 `appcast-<target>.xml`，供應用程式從 `feed` 路徑讀取：請將 appcast 與安裝程式一併上傳。若缺少私鑰，`build` 不會產生 macOS 與 Windows 的 appcast（因為它們僅安裝已簽署的安裝程式）；Linux 與 Android 應用程式讀到未簽署的 appcast 時，則會開啟該發佈版本的網頁而非直接安裝套件。

`-release-notes notes.md` 會將 Markdown 格式的發佈說明（例如變更日誌中的該版本章節）寫入每個 appcast，並轉譯為不含檔案原始 HTML 的 HTML。Sparkle 與 WinSparkle 會在更新對話框中顯示此說明，Linux 與 Android 的更新提示則會將其作為純文字顯示在版本號下方（最多 1,500 個字元）；應用程式可透過 `core.UpdateCheck.Notes` 讀取。

| 目標平台 | direct 建置的更新方式 |
| --- | --- |
| macOS | `build` 封裝於應用程式內的 Sparkle 會每天檢查更新、提示新版本、下載磁碟映像檔、驗證簽章、安裝並重啟應用程式；預設應用程式選單包含「檢查更新…」，自訂選單的應用程式可使用 `updater.Check` 加入此項目 |
| Windows | 位於執行檔旁的 WinSparkle 執行相同流程，在 WinSparkle 支援該語言時會以應用程式啟動時顯示的語言呈現，並以靜默模式執行安裝程式後啟動新版本 |
| Linux、Android | 應用程式會讀取其 appcast，當使用者選擇更新時，將已簽署的套件串流下載至磁碟並驗證（透過 `updater.Status` 與平台任務進度指示器回報進度），接著透過 `pkexec dpkg`、`pkexec pacman`（`.pkg.tar.gz`/`.pkg.tar.zst`）、原子替換 AppImage（於 Linux 上重啟），或 Android 套件安裝程式進行安裝；其他套件格式或未簽署的 appcast 則會改為開啟發佈頁面 |
| iOS | 應用程式向負責更新它的 App Store 查詢，並提供前往該商店頁面的選項 |

對於已發佈的獨立網頁應用程式，`updater.Check(cx)` 會比對目前執行中的版本與重新擷取的 `nsgo.json`，並在確認後提供重新載入選項。即使版本標籤相同但建置檔案有所變動，此機制也能偵測得到。此路徑不需要 appcast 金鑰；明確設為 `none`/`steam` 渠道仍會停用檢查，而嵌入式宿主則自行管理重新載入。小程式使用其平台的更新管理器，且僅在下載的更新就緒後才會重啟。這些目標會在應用程式呼叫更新器時進行檢查；小程式的檢查與下載時機則歸屬於宿主平台。它們不提供自動檢查偏好的設定函式。自訂 `cx.CheckForUpdates` 處理函式時，可使用 `UpdateCheck.Restart` 處理可能缺乏版本標籤的已就緒更新。

### 渠道

渠道決定由誰來更新應用程式。`-channel` 可在單次建置中覆寫 `updates.channel`，因此同一個專案可分別建置對應各種渠道的版本：

| 渠道 | 適用對象 | 建置出的封裝檔 |
| --- | --- | --- |
| `direct` | GitHub、官方網站，以及直接安裝發佈檔案的 Homebrew cask 與 winget | 如上表所述自行更新 |
| `store` | App Store、Mac App Store、Microsoft Store、Google Play、Flathub 與其他商店 | 屬於[商店封裝檔](#商店封裝檔)，不附帶自我更新器；iOS 與 macOS 無需金鑰即可查詢各自的 App Store，透過 Google Play 安裝的應用程式則可使用 Play 的原生更新流程 |
| `steam` | Steam | 產生交由 Steam 更新的 [depot 目錄](#steam-depot) |
| `none` | Linux 發行版以及任何重新打包應用程式的對象 | 不檢查更新 |

商店會拒絕包含自我更新機制的應用程式，因此請務必上傳 `store` 建置，切勿上傳 `direct` 建置。每次包含 macOS 或 Windows 安裝程式的 `direct` 發佈同時也會產生 Homebrew cask（`dist/releases/homebrew/<name>.rb`，宣告 `auto_updates true`、已建置磁碟映像檔的檢查碼與下載網址，以及每個應用程式[命令列程式](#命令列程式)的 `binary` 連結）與 winget 多檔案清單（對應已建置 NSIS 安裝程式的 `dist/releases/winget/<id>.yaml`、`<id>.installer.yaml` 與 `<id>.locale.en-US.yaml`）。只要系統能夠辨識應用程式的安裝來源，應用程式也會一併檢查：從其他商店安裝的 Android 應用程式、Flatpak 或 Snap，以及 TestFlight 建置皆會交由該商店管理更新。在透過 Google Play 安裝的應用程式中手動呼叫 `updater.Check` 時，會在允許的情況下請求 Play In-App Updates 提供即時更新流程；同意、下載、安裝與重啟均由 Play 處理。開發建置永遠不會進入該流程。獨立 Android 套件已內建 SDK；[嵌入式宿主](embedding.md#android)則透過 Gradle 引入。

### 在應用程式內檢查

`updater.Check(cx)` 的行為如同「*檢查更新…*」選單項目，執行檢查並顯示結果；`updater.CheckInBackground(cx)` 僅在發現新版本時才顯示提示，而 `updater.CheckDaily(cx)` 則每天最多執行一次背景檢查。`updater.Status(cx)` 可在 Linux 與 Android 上讀取即時下載與安裝進度。當 `automatic` 開啟時，Linux、`direct` Android 建置與 Apple App Store 建置會在啟動時及每次使用者切回應用程式時依此方式檢查；若檢查失敗（例如處於離線狀態），則會在一小時後重試。開發建置永遠不會執行更新。在 Steam 建置中，這三個函式皆不執行任何動作。

`updater.CapabilityOf(cx)` 無需網路請求即可回報目前複本的更新方式：`ManagedSelf`、`ManagedPage`、`ManagedDevelopment`、`ManagedChannel`、`ManagedStore`（附帶 `Store` 名稱）或 `ManagedUnavailable`。開發建置、停用的渠道或商店安裝在檢查時會給出對應原因的說明；`core.UpdateCheck.Err` 僅保留給諸如訂閱源損毀或內建更新器無法載入等實際錯誤。

在繫結中讀取 `updater.Status(cx)` 可持續追蹤 `Capability`、`AutomaticChecks`、`AutomaticDownloads`、`Available`、`Notes` 與 `LastCheck`，而在 Linux 與 Android 上還能追蹤更新的 `Phase`、`Loaded` 與 `Total`（`Fraction()` 可直接供進度條使用）。`updater.SetAutomaticChecks(cx, on)` 會持久化儲存使用者的選擇，並在執行階段改變每日排程器的行為。Sparkle 與 WinSparkle 各自管理其偏好設定；Linux、Android 與 iOS 則將選擇保存在私有儲存分區中。`updater.SetAutomaticDownloads(cx, on)` 僅支援 macOS，預設值為 `updates.automatic_downloads`；其他平台會回報 false 並忽略該設定呼叫。不支援或由外部管理的建置會將兩項設定皆回報為 false。WinSparkle 會在找到更新的回呼觸發後讀取 appcast 以取得更新中繼資料；安裝流程仍由 WinSparkle 控制。Linux 會在啟動時檢查 Debian 或 Arch 套件所有權以及 AppImage 身分；這些複本會回報 `ManagedSelf`，而其他未經打包的複本在任何檢查執行前皆會回報 `ManagedPage`。

## 登入時啟動

`desktop.SetLaunchAtLogin(cx, on)` 會透過平台的登入機制註冊應用程式，`desktop.LaunchAtLogin(cx, done)` 則用於讀取其狀態。目前工具建置的 MSIX 會自動包含一個預設停用的啟動任務；啟用時會遵守使用者的「啟動應用程式」開關與系統管理員原則。一般 Windows 與 Linux 安裝則使用 Run 登錄檔機碼與 XDG 自動啟動項目。macOS 會使用應用程式套件的原生登入服務。

Flatpak 會請求 Background portal（可能會跳出授權詢問），而無需開放宿主組態檔案的存取權限。後續讀取會等待原生回應。該 portal 沒有唯讀的啟動狀態查詢介面：Flatpak 讀取只能回傳本次執行階段中最後一次確認的回應（收到回應前為 false），無法偵測應用程式外部的變更，也無法在下次執行時復原設定狀態。

由外部工具打包的 Snap 需要在其應用程式區段宣告自動啟動檔案名稱：

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo 會將該檔案寫入 `$SNAP_USER_DATA/.config/autostart`，讓 snapd 透過應用程式的包裝腳本啟動它。`desktop.LaunchedAtLogin(cx)` 可在應用程式建置畫面之前辨識此次是否為登入啟動（包含 MSIX 的 StartupTask 啟用）。

## 檔案類型

`file_types` 用於註冊應用程式可開啟的文件類型：支援從 macOS 的 Finder、Dock 與「打開檔案的應用程式」、Windows 透過 Windows 安裝程式或 MSIX 註冊的檔案總管「開啟檔案」、Linux 上 Debian、Arch、AppImage 與 Flatpak 套件的桌面項目，以及 iOS 的「打開方式」：

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

Windows 安裝程式會將應用程式加入「開啟檔案」清單而不會覆蓋使用者原有的預設應用程式，並在升級與解除安裝時移除其登錄項目。Linux 會從固定對照表中尋找常見副檔名的 MIME 類型，若為應用程式自訂類型的未知副檔名，則賦予 `application/x-<type id>` 並搭配共用的 MIME-info 定義。`mime` 可改為指定現有的 MIME 類型；若匯入其他應用程式的類型且對照表中缺乏其副檔名，則必須設定此欄位。直接執行 AppImage 不會註冊任何關聯：必須透過桌面整合工具安裝其桌面項目與類型定義。

應用程式透過 `desktop.OnOpenFiles` 接收開啟的檔案，包含在 Windows 與 Linux 執行期間的後續啟動，以及 iOS 場景的文件開啟事件。啟動應用程式時帶入的檔案會等待第一個處理函式註冊完成：

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

在 iOS 上，一般文件會以原始檔名匯入至應用程式擁有的暫存檔案中。檔案提供者（file-provider）的讀取會在非 UI 執行緒上協調進行，其安全性存取作用域（security scope）會在處理函式執行前結束。若需永久保存，請將匯入的檔案移至 `cx.DataDir()`；編輯操作僅會影響該複本。iOS 會使用應用程式的文件圖示；`icon` 設定僅適用於 macOS。App Clip 與 tvOS 套件不會註冊文件類型。檔案開啟事件與 `OnOpenLinks` 保持分離。

`desktop.AddRecentFile(cx, path)` 會記錄使用者曾開啟的文件：於 macOS 上出現在 Dock 選單與「檔案 › 打開最近使用過的檔案」、於 Windows 上出現在跳躍清單（Jump List）的「最近」類別，於 Linux 上則寫入桌面的最近使用檔案清單。使用者點選項目時會傳遞給同一個處理函式。`desktop.ClearRecentFiles` 會清除應用程式自身的記錄，在 Linux 上會保留其他應用程式對共用檔案的歷史記錄，且 Linux 同時也會尊重 GTK 的歷史記錄設定。

`desktop.SetDockMenu(cx, items)` 用於設定應用程式圖示上的動作選單：包含 macOS 的 Dock 選單、Windows 的跳躍清單任務，以及 Linux 上支援快速清單的 Ubuntu Dock、Plasma 與 Dash to Dock。Linux 需要已安裝且符合應用程式 `id` 的桌面項目，並支援子選單、分隔線、勾選與停用項目。該選單歸屬於設定它的作用域，跨視窗最後一次呼叫的設定者會覆寫先前設定。

## 連結

`url_schemes` 為應用程式註冊專屬的連結協定（例如 `myapp://settings`），可從瀏覽器、其他應用程式、`open`、`start` 或 `xdg-open` 開啟應用程式。macOS、iOS 與 Android 應用程式皆會註冊它們，Windows 安裝程式與 MSIX 封裝檔以及 Linux 桌面項目（包含 Debian、Arch、AppImage 與 Flatpak 套件）亦同。解除安裝時會一併移除。

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

應用程式透過 `desktop.OnOpenLinks` 接收連結，執行期間同樣有效：在 Windows 與 Linux 上，第二次啟動應用程式會將連結傳遞給已執行中的執行個體並隨即結束。啟動應用程式時帶入的連結會等待第一個處理函式。任何人都能構造這類連結，且在 Windows 與 Linux 上處理函式會收到應用程式啟動時傳入的任意 URL，因此請務必驗證協定（scheme）並在連結變更任何資料前先詢問使用者：

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

在 iOS 與 Android 上，`mobile.OnOpenLinks` 會在啟動時與執行期間接收連結。在 iOS 模擬器上可使用 `xcrun simctl openurl booted myapp://settings` 開啟，在 Android 上則可使用 `adb shell am start -a android.intent.action.VIEW -d myapp://settings` 開啟。在測試中則可使用 `app.OpenLinks("myapp://settings")` 傳遞連結。

若要使用 iOS 通用連結（Universal Links），請透過 `app_links: [links.example.com]` 宣告明確的 HTTPS 主機。每個主機必須是不含協定、路徑、連接埠、IP 位址或萬用字元的 DNS 名稱；名稱會自動轉為小寫且不允許重複。工具會將 `applinks:<host>` 寫入完整應用程式的 Associated Domains 權限宣告中，包含 Xcode 管理的自動簽署；IPA 的描述檔則必須已授權該權限。App Clip 仍保留來自 `app_clip.domains` 的獨立 `appclips:` 宣告。

每個主機都必須在 `https://<host>/.well-known/apple-app-site-association` 提供指名完整應用程式的 `apple-app-site-association` 檔案；僅靠封裝中繼資料無法授權通用連結。[Apple 的網域關聯需求文件](https://developer.apple.com/documentation/xcode/supporting-associated-domains)說明了伺服器端的設定方式。Ad hoc 模擬器簽署會省略受限的開發者權限宣告，因此代理測試並不能驗證伺服器網域關聯是否正確。

在 iOS 上，`mobile.OpenedURL(cx)` 會追蹤最近的 App Clip 叫用連結、通用連結或自訂協定網址，包含在根元件掛載前的初始值。自訂協定連結仍會送達 `mobile.OnOpenLinks`；當該處理函式執行時，叫用網址已是最新值。檔案開啟事件仍保留於 `OnOpenFiles`，不會取代叫用網址。Android/Web 的叫用支援則於 #1475 中另行追蹤。

## Android 接收分享

選擇應用程式透過 Android 分享工作表處理的 MIME 類型：

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

APK 與 App Bundle 建置會同時為單一與多個分享項目註冊這些類型。註冊 [`mobile.OnShared`](services.md#開啟連結與檔案) 即可接收每次分享的內容。

針對使用路徑式網址的網頁應用程式，請將 `web.base` 設為其部署目錄（`/` 或 `/my-app/`），並設定生產環境伺服器為應用程式路由提供 `index.html`。開發伺服器已內建針對 HTML 導覽請求的此項後備處理。重新載入深層連結時，資源路徑仍會相對於該固定的 HTML base 解析。詳見[應用程式網址](addresses.md)。

## iOS 接收分享

宣告分享擴充功能所接受的統一類型識別碼（UTI），請將網址與文字類型置於較寬泛的檔案類型之前：

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` 與 `dev` 會將一個 Go 分享擴充功能 `<id>.share` 一併打包進應用程式。Xcode 會在實機執行時簽署它並註冊其 App Group。若建置 IPA，請為 `<id>` 與 `<id>.share` 啟用相同的 App Group；`sign.profile` 與 `sign.share_profile` 皆須授予該權限。App Clip、tvOS 與 watchOS 不支援分享擴充功能。

該擴充功能會將最多 128 個項目與 512 MiB 的檔案複製到 App Group 中，接著完成分享工作表。當應用程式處於活躍狀態或使用者下次開啟應用程式時，`mobile.OnShared` 會收到這些批次資料；即使在傳遞前擴充功能與主應用程式已結束，資料依然會保留。iOS 並未提供任何受支援的 API 讓分享擴充功能將宿主應用程式切換至前景或啟動它。因此，此交接過程不會自動開啟主應用程式。

## 小程式

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

每個目標都會產生供該平台開發者工具使用的專案（同時位於 `dist/releases/miniprogram/<platform>`），`-run` 與 `dev` 會開啟該工具；`dev` 會在你存檔時重新建置，並讓開發者工具重新載入：

| 目標 | 平台 | 工具 |
| --- | --- | --- |
| `miniprogram_wechat` | 微信（WeChat） | [微信開發者工具](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html)，需開啟服務連接埠（設定 › 安全設定） |
| `miniprogram_qq` | QQ | [QQ 開發者工具](https://q.qq.com/wiki/tools/devtool/)，於 macOS 上 |
| `miniprogram_alipay` | 支付寶（Alipay） | [支付寶小程式開發者工具](https://opendocs.alipay.com/mini/ide/download)，於 macOS 上 |
| `miniprogram_douyin` | 抖音（Douyin）及字節跳動旗下其他應用 | [抖音開發者工具](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download)，於 macOS 上 |

`miniprogram.appid` 設定應用程式在各平台（`wechat`、`qq`、`alipay`、`douyin`）的小程式 AppID；若未設定，微信開發者工具會使用遊客 AppID 預覽，而其他工具則會提示輸入 AppID。在微信上，`-run` 與每次 `dev` 重建會透過 CLI 關閉並自動開啟專案，使開發者工具編譯最新檔案。此行為已於 DevTools 2.01.2510290 上驗證。平台的導覽列會顯示應用程式標題；啟用 `custom_navigation` 後，應用程式會填滿整個頁面，並透過安全區域避開膠囊選單按鈕。應用程式會將手機系統讀取為 `Env.Simulated` 為 true（與在網頁上使用 `--nsgo-os` 時相同）：堆疊中的頁面會顯示各自獨立的 `Header`，因為沒有系統標題列取代它。QQ、支付寶與抖音尚未在各自工具中實測，且支付寶目前尚未支援由應用程式接管返回鍵、關鍵影格與鍵盤高度。

小程式目前仍屬於實驗性功能。小程式僅能執行 Go 應用程式而無法執行 TSX，且各平台將應用程式壓縮後的模組大小限制在 2 MB 以內：使用 `base` 建置的幾個畫面（如 `examples/gallery-miniprogram`）可符合限制，使用 `ui` 控制項的應用程式目前則尚無法符合，超出時 `build` 會給出提示。著色器在小程式中不會繪製任何內容，且網頁檢視、影片、原生元件與應用程式的自訂字型檔案目前尚未顯示。

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` 會在 `dist/releases` 中產生 `<name>-<version>-harmonyos.hap`。引擎作為 WebAssembly 在 ArkWeb 中執行，並由 ArkTS 宿主透過 ArkUI 檢視繪製其掛載的內容，因此文字、輸入欄位、捲動與對話框皆為系統原生元件。當 `PATH` 中有 `hdc` 時，`-run` 會將 HAP 安裝至已連接的實機或模擬器並啟動它；`nsgo test -target harmonyos` 則會以獨立的 `.nsgotest` 套件身分識別碼，在模擬器上透過未簽署的 HAP 執行應用程式測試，並在執行結束後移除該測試用 HAP，確保原本已安裝的應用程式不受影響。

若要從範本編譯 ArkTS 宿主並使用應用程式自身的 `id` 作為套件名稱，請將 `hvigorw` 加入 `PATH` 並將 `OHOS_BASE_SDK_HOME` 指向包含 API 26（`26.0.0`，即範本的編譯與目標 SDK 版本）的 OpenHarmony SDK。`build.harmonyos_bundles` 可選擇 `auto`（預設值）、`exact`（強制需要 SDK）或 `padded`（將 ID 填充至已提交宿主專案的固定長度，無需 SDK）。目標測試使用相同設定，並會印出實際打包的宿主類型。`harmonyos.permissions` 列出應用程式請求的 `ohos.permission` 權限名稱（例如 `MICROPHONE` 與 `CAMERA`），而 `widget.title` 則決定主畫面卡片顯示的標題。

HarmonyOS 目前屬於預覽功能。模擬器可直接安裝未簽署的 HAP。[架構筆記](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md)列出了預覽版支援繪製的 ArkUI 功能。

### 已簽署的 HAP

設定 `harmonyos.signing` 即可使用 AppGallery Connect 的憑證資料簽署 HAP。在華為手機上直接安裝需要使用針對該裝置的除錯憑證與描述檔。公開發佈時請改用發佈簽署資料。[華為簽署指南](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

路徑皆相對於 `nsgo.yaml`。請在環境變數中設定 `NSGO_HARMONY_KEYSTORE_PASSWORD` 與 `NSGO_HARMONY_KEY_PASSWORD`。簽署過程會使用 SDK 內的 `hap-sign-tool.jar` 以及來自 `JAVA_HOME` 或 `PATH` 的 Java。建置必須保留應用程式精確的 `id` 以符合描述檔，因此即使在 `auto` 模式下進行簽署也必須具備 SDK 與 `hvigorw`。在應用程式目錄執行 `go tool nsgo doctor` 可在不需輸入密碼的情況下檢查簽署組態與描述檔套件名稱。`build -run` 會將已簽署的 HAP 交給 `hdc` 安裝。

## 簽署

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

金鑰庫密碼取自環境變數 `NSGO_KEYSTORE_PASSWORD` 與 `NSGO_KEY_PASSWORD`。`-team` 用於簽署 iPhone 封裝檔。`-profile`、`-widgets-profile`、`-share-profile`、`-clip-profile` 與 `-macos-profile` 可從命令列分別覆寫主應用程式、小工具擴充功能、分享擴充功能、App Clip 與 Mac App Store 的描述檔。`-installer` 則可覆寫 Mac Installer Distribution 身分憑證。

### 簽署 Windows 封裝檔

Authenticode 用於識別 Windows 下載檔案的發佈者；[SmartScreen 則會另外檢查其信譽評等](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation)。`sign.certificate`（`.pfx` 檔案）會簽署執行檔、命令輔助程式、應用程式資源帶入的程式與函式庫、安裝程式及其解除安裝程式，以及為 Store 渠道建置的 MSIX；其密碼取自 `NSGO_CERTIFICATE_PASSWORD`。簽章是由 Go 直接產生的（與 `signtool` 的格式相同），因此 Mac 無需 Windows SDK 即可完成簽署：

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` 預設使用 DigiCert 的伺服器，設為 `none` 則不會為簽章加上時間戳記（此情況下簽章將在憑證過期後失效）。若憑證存放在硬體權杖（token）或雲端服務中，則可改用自訂命令進行簽署，其中 `%1` 代表要簽署的檔案，建置時會對上述每個檔案執行該命令：

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

若該命令回傳成功，但應用程式執行檔、命令輔助程式、PE 資源或安裝程式執行檔仍缺少內嵌的 Authenticode 簽章，`build` 將會中止並報錯。

安裝程式內部的解除安裝程式會在嵌入前先行簽署，所用的 `sign.certificate` 或 `sign.sign_command` 與應用程式及安裝程式相同。`-certificate`、`-sign-command` 與 `-timestamp-url` 可從命令列設定上述選項。

### 公證

經過 Apple 公證的應用程式在 Mac 上下載開啟時，不會出現 Gatekeeper 的警告。請使用 Developer ID 進行簽署，並指定 `notarytool` 保存在鑰匙圈中的憑證設定檔名稱：

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

接著 `build` 會以強化執行階段（hardened runtime）進行簽署、提交磁碟映像檔、等待數分鐘取得 Apple 的審核結果，並將公證票券（ticket）裝訂（staple）至磁碟映像檔與應用程式；若遭拒絕則會印出 Apple 的錯誤日誌。在 CI 環境中，可使用 App Store Connect API 金鑰的 `.p8` 檔案、金鑰 ID 與發行者 ID，透過 `NSGO_NOTARY_KEY`、`NSGO_NOTARY_KEY_ID` 與 `NSGO_NOTARY_ISSUER` 取代鑰匙圈設定檔。除非應用程式主動宣告權限，否則強化執行階段會拒絕存取麥克風、相機、行事曆與位置資訊；只要 `apple.info` 中包含 `NSMicrophoneUsageDescription`、`NSCameraUsageDescription`、行事曆或提醒事項用途說明，或位置用途說明，`build` 便會自動加上對應權限。

## 小工具與即時動態

`build` 與 `dev` 會直接呼叫 Xcode 的 Swift 編譯器，將繪製應用程式小工具與即時動態的擴充功能編譯進 macOS 與 iOS 套件中，無需額外步驟。在 iOS 上，主應用程式與其小工具共用一個 App Group：在 iPhone 上使用 `-run` 時 Xcode 會簽署兩者並註冊該群組；若建置 IPA，請為 `<id>` 與 `<id>.widgets` 啟用該群組，並以 `sign.widgets_profile` 簽署擴充功能。Android 獨立 APK 與 AAB 會從 `widgets` 納入 App Widget 提供者與資源；預先建置的 Java 執行階段會負責繪製其時間軸，應用程式建置無需安裝 Android SDK 或 Java 編譯器。Android 16 會將即時動態顯示為 Live Updates，且無需額外的擴充功能。詳情請參閱[小工具](widgets.md)與[即時動態](live-activities.md)。

## Android App Links

`app_links` 列出點擊其連結時應開啟完整 Android 應用程式的明確 HTTPS 主機：

```yaml
app_links: [example.com, shop.example.com]
```

APK 與 App Bundle 會為每個主機宣告一個帶有 `autoVerify` 的可瀏覽 `ACTION_VIEW` intent filter。請使用不含協定、路徑、連接埠、萬用字元或 IP 位址的 DNS 主機名稱。請在 `https://<host>/.well-known/assetlinks.json` 提供包含應用程式套件 ID 與簽署憑證指紋的檔案；Android 會在將連結導向應用程式前驗證該關聯。工具不會代為發佈關聯檔案；[Android 驗證指南](https://developer.android.com/training/app-links/verify-applinks)說明了其設定方式。嵌入式的 Android 宿主須自行宣告 intent filter，並將接受的連結轉發給其 `NsgoView`。

`mobile.OpenedURL(cx)` 會在首次根元件算繪前讀取已接受的叫用連結，並在另一個連結開啟執行中的應用程式時更新。`mobile.OnOpenLinks` 仍會接收每一個已接受的連結。佔滿整個頁面的網頁應用程式會透過 `mobile.OpenedURL` 讀取目前的頁面網址，包含瀏覽器歷史記錄與 hash 變更；嵌入在其他頁面中的應用程式則不會讀取外層頁面的網址。當應用程式同時需要寫入網址時，請改用 [`cx.AppURL`](addresses.md)。

## App Clip

App Clip 是 iOS 應用程式的輕量部分，無需安裝完整應用程式即可透過連結、行動條碼或 NFC 標籤啟動。`app_clip` 會將一個 App Clip 建置進 iOS 套件中，並採用獨立的 main 套件以確保僅連結實際需要顯示的程式碼：

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

App Clip 可透過 `mobile.OpenedURL` 讀取開啟它的連結（當另一個連結再次開啟它時該值會更新），並可透過 `mobile.IsAppClip` 判斷目前程式碼是否以 App Clip 身分執行。系統允許下載的 App Clip 大小上限為 15 MB（從 iOS 17 起，若僅透過連結開啟則放寬至 100 MB），超過時 `build` 會拒絕建置；連結 `ui` 套件的 App Clip 約佔 11 MB。執行 `dev -target ios_simulator -clip https://example.com/order?table=12` 可在模擬器中單獨執行該 App Clip 並從指定連結開啟；`-target ios` 則會在 iPhone 上執行並印出該連結的 QR Code：在「設定 > 開發者 > 本機體驗」中加入該連結後，即可用相機掃描開啟。若開發除錯符號使 App Clip 超過大小限制，請加上 `-release`。Apple 僅允許 Apple Developer Program 的付費團隊簽署 App Clip，個人開發者團隊無法簽署。

`dev -target ios` 與 `build -run -target ios` 也會將 App Clip 保留在完整應用程式內部：Xcode 會將其作為獨立目標簽署，並在簽署主應用程式前將其嵌入。自動簽署需要在 Xcode 中登入已驗證的付費團隊帳號，並具備主應用程式與 App Clip 兩者的描述檔。若要上架 App Store，請為 `<id>` 啟用 App Clips 功能、使用 `sign.clip_profile` 簽署 App Clip，並在對應網域提供 `apple-app-site-association` 檔案。版本庫中的 `examples/appclip` 是一個咖啡廳範例，其 App Clip 可直接於餐桌點餐。

若要將訂單或其他本機資料交接給完整應用程式，請在 `app_clip` 旁設定明確的 App Group：

```yaml
apple:
  app_group: group.dev.example.hello
```

如此一來，App Clip 與完整應用程式便會在該群組的容器中共享 `cx.DataDir()` 與 `cx.Storage()`。App Clip 在資料目錄下寫入的檔案與鍵值儲存項目，在完整應用程式啟動時皆可讀取；`cx.LogsDir()` 亦跟隨該目錄，而兩個套件各自保有獨立的快取目錄。Xcode 簽署時會為兩個目標宣告該群組；使用手動描述檔時，請為兩個 App ID 啟用相同群組並在 `sign.profile` 與 `sign.clip_profile` 中授予權限。缺少授權將無法完成簽署，而若無法存取該群組則會使啟動中止。宿主明確指定的資料目錄仍保有較高優先權。

若未設定 `apple.app_group`，兩個套件會各自保留獨立的沙盒儲存空間。開啟此設定並不會自動遷移先前儲存於各別沙盒中的資料，而僅包含小工具的應用程式則會維持原有的儲存位置。共用的機密資料請存放在鑰匙圈中，切勿存放在一般儲存空間。上述咖啡廳範例會持久化保存 App Clip 的訂單，並在完整應用程式中顯示出來。

在 App Clip 內由明確的使用者動作觸發時，可呼叫 `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` 請求 StoreKit 推薦對應的完整應用程式。`AppClipOverlayBottomRaised` 則會為底部列預留空間。回傳的函式可用於關閉該推薦請求；作用域清理時也會自動關閉，且舊的請求無法關閉較新的請求。在原生 iOS App Clip 以外的環境呼叫此函式不會有任何效果。是否能夠載入並顯示推薦橫幅由 App Store 決定，因此該呼叫不會回報是否已顯示推薦或使用者是否已安裝。請確保 App Clip 的核心任務無需安裝完整應用程式即可正常完成。[Apple 推薦覆蓋層指南](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## 嵌入其他應用程式

`-embed` 可將應用程式打包以供你自己的 iOS 或 Android 應用程式嵌入：產出為 XCFramework（加上 `-dynamic` 時則包含動態框架）或 AAR，由宿主透過 `NSGoView` 或 `NsgoView` 顯示畫面。Go 桌面程式可透過 `nsgo.NewView` 顯示應用程式，網頁則可透過網站產出的 `nsgo.js` 載入（主機位址為頁面自身位址或 `nsgo.yaml` 中 `web.host` 所指定的位址）。版本庫中的 `examples/embed-*` 展示了各種平台的範例；[嵌入指南](embedding.md)則涵蓋宿主端的整合細節。
