---
title: 執行與除錯
description: 在任何目標平台上以熱重載執行應用程式，並檢視錯誤、更新、版面配置、效能與其他語言。
group: 工具鏈
order: 20
---

## 執行

```sh
go tool nsgo dev
```

`dev` 會為目前這台機器建置並開啟應用程式，或建置 `nsgo.yaml` 中 `dev.target` 所指定的目標平台。每次儲存 Go 檔案時，它會重新建置並重啟應用程式，若應用程式匯入了 `go.mod` 或 `go.sum` 尚缺少的模組套件，會先執行 `go mod tidy`；若僅修改 TSX，則會在原地更新執行中的應用程式並保留其狀態。當應用程式關閉時，執行也隨之結束。

在 macOS 與 Windows 上，原生開發模式重建後會在目前工作階段內，以上次一般視窗的位置與大小重新開啟主檢視視窗。Windows 也會還原全螢幕狀態。在 Linux 上，GTK 會還原其一般大小並請求上次的全螢幕模式，視窗位置則由視窗管理員決定。

`-target` 可於其他平台執行，而 `-device` 可依名稱、UDID 或序號選取 iPhone、Apple TV、Apple Watch、模擬器或 Android 裝置：

```sh
go tool nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
go tool nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target ios                # on the connected iPhone, signed by Xcode
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator, over adb
go tool nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

開發伺服器預設會監聽所有網路介面。若要將其限制在本機迴路（包含 Android 透過 `adb reverse` 連線的情況），請使用 `-host 127.0.0.1`；或指定本機介面的 IP 位址以僅在該介面上監聽。若要在實體 Apple 裝置上使用開發功能，請使用區域網路可連線的位址。

`-h` 會列出所有旗標。

## Go 除錯器

將與你 Go 版本相容的 [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) 放入 `PATH` 並命名為 `dlv`。接著啟動保留除錯資訊且停用 Go 最佳化的原生桌面應用程式。

```sh
go tool nsgo dev -target native -debug
```

複製 Delve 的 `API server listening at` 那一行所顯示的連接埠。在安裝了 Go 擴充功能的 VS Code 中新增附加設定，並將 `40000` 替換為該連接埠。

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

在 GoLand 中，新增一個 **Go Remote** 執行設定，主機填入 `127.0.0.1` 並使用相同連接埠。在應用程式的 Go 程式碼中設定中斷點、附加除錯器，並執行會觸及該處的操作。Go 成功重新建置後，請於相同位址重新連線除錯器，使其將中斷點傳送至新程序。TSX 的修改仍會使用熱重載。

`-debug` 可在 macOS、Windows 與 Linux 上搭配 `-target native` 執行。請將開發建置用於除錯，並把 `-release` 留作測量發佈建置效能之用。

## 元件預覽

在可匯入的 Go 套件中，於元件旁放入一個或多個 `PreviewXxx` 函式，並將它們儲存在受 preview 建置標籤保護的 Go 檔案中。

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

勾選想同時顯示的測試範例（fixture），並選擇語系、主題、淺色或深色外觀、視埠大小與版面方向。預覽會採用距離最近的應用程式的 `nsgo.yaml`、翻譯語系目錄、設定的 `theme` 與資源。在旗標後方傳入應用程式目錄，即可選取同一模組中的其他設定；在此情況下，預覽路徑會相對於該應用程式目錄解析。

儲存檔案會重新建置預覽並保留你的選取項目與顯示設定。`nsgopreview` 標籤會讓這些測試範例被排除於一般發佈建置之外。在帶有該標籤的 `_test.go` 檔案中以 `nsgotest.Start(t, PreviewEmpty)` 使用同一個測試範例，並透過 `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` 執行，即可取得原生螢幕截圖。

在以 `-target ios_simulator` 完成主應用程式的首次安裝後，僅修改 Go 程式碼的重建只會替換已安裝的可執行檔並重新啟動應用程式。編輯 `nsgo.yaml`、打包資源或應用程式圖示則會觸發重新安裝。

若要以更短的 Go 重建迴圈進行 Android UI 開發，請使用 `go tool nsgo dev -target android -android-host`。此工具會先安裝一次原生執行環境，並將 Go 程式運行於開發機上。每次 Go 重建都會透過 adb 重新連線至同一個 Activity。修改打包資源、預先編譯的類別、啟動器圖示或 `nsgo.yaml` 時，則會重新安裝執行環境 APK。Go 程序會使用開發機的建置條件、檔案系統與網路。`Scope.DiskSpace` 回報的是包含開發機 `DataDir` 的磁碟區空間。若需驗證 `_android.go` 檔案與裝置上的 Go 行為，請改用一般的 Android 開發指令。

## 錯誤

建置失敗的訊息會直接顯示於執行中的應用程式畫面上，並附帶複製錯誤與重新建置按鈕；修正檔案後，下次儲存就會取代該錯誤畫面。元件在轉譯過程中發生 panic 也會以同樣方式顯示。在應用程式中按下 Command+R（非 Apple 平台為 Control+R），隨時都能重新建置並啟動應用程式——即使正顯示錯誤畫面亦然，除非應用程式自行透過 `cx.OnKey` 或啟用的應用程式選單項目繫結了該按鍵；在瀏覽器中該按鍵則維持原本的重新整理頁面功能。應用程式自身的輸出（例如 `log.Print`）會顯示於終端機中工具輸出行的下方。

執行前，`go tool nsgo check` 會檢查應用程式並對 TSX 執行型別檢查；`-target all` 則會針對所有目標平台進行檢查，因此位於其他平台建置標籤後的程式碼也能在 Mac 上完成驗證。

### 打包後 Apple 應用程式的日誌

在 macOS、iOS、tvOS 與 watchOS 上，預設的 Go logger 會寫入 Apple 的統合日誌（unified log），包括 `log.Print`、NSGo 診斷訊息與 TSX 主控台輸出。在「系統監視程式 / 主控台（Console）」中，以執行中應用程式的 Bundle Identifier 篩選子系統（subsystem），並將分類（category）設為 `nsgo`。在 macOS 上你也可以直接查詢日誌存放區：

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

日誌紀錄會使用預設層級並保留其警告或錯誤前綴。由於其內容為公開可見，請勿將憑證或其他機密寫入日誌中。過長的訊息會被拆分成多筆紀錄。開發期間終端機輸出仍可使用；自訂的日誌寫入器或 `slog` handler 則保有各自的輸出控制權。

### Linux 網頁檢視

Ubuntu 24.04 及更新版本可能會透過 AppArmor 限制非特權使用者命名空間。當 bubblewrap 無法啟動 WebKitGTK 的沙盒時，NSGo 會記錄錯誤並顯示預留位置。部分 WebKitGTK 版本在開啟第一個網頁檢視時甚至會以 `SIGTRAP` 結束。參閱 [Ubuntu AppArmor 文件](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)。

若你的發行版提供 bubblewrap 的 AppArmor 設定檔（例如 `bwrap-userns-restrict`），請直接使用它。否則可由管理員以 [NSGo Linux CI 的設定檔](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml)為起點，載入一個授予 `/usr/bin/bwrap` `userns` 權限的設定檔。因為該設定檔會套用至所有使用系統 bubblewrap 的應用程式，必須由管理員管理。

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` 會關閉 WebKit 的子程序沙盒。請僅於本機開發且載入受信任內容時使用。

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` 會在瀏覽器中開啟由 `nsgo dev` 提供的 DevTools，適用於所有目標平台，包含手機上的應用程式。它能即時顯示執行中應用程式的狀態：

- **Elements**：以樹狀結構顯示所有檢視。每個項目皆顯示建立它的元件、連往編輯器對應行號的連結、測試尋找它時使用的選擇器、它的類別、邊界框、實際配置的外距、邊框與內距，以及其設定的樣式。滑鼠懸停在某列上時，會在應用程式中繪製出它的外距、內距與內容區域，就像瀏覽器的 DevTools 一樣；而 **Pick** 讓你能直接點選任何視窗、面板、工作表（sheet）或嵌入檢視中的元素，在樹狀結構中定位它。
- **State**：顯示你的程式碼所建立的訊號，依所屬的元件與計算（computation）分組，列出其目前數值、memo 的數值，以及各有多少個計算讀取它。沒有任何計算讀取的數值不會改變畫面上的任何內容。字串、布林值與數字都可以直接編輯，讓你無須修改程式碼即可測試不同狀態。NSGo 內部自行建立的數值則收納在 **NSGo's own** 之下。
- **Profiler**：顯示每次提交的耗時，拆分為提交前的應用程式碼執行時間、包含版面配置的提交時間，以及在平台主執行緒上套用變更的時間。同時顯示觸發該次提交的寫入行號，並預設展開執行耗時最長的計算。另一份清單則依建立行號列出過去一秒內執行次數最多的計算。Android 的執行環境是在自身的 Java 程式碼中套用批次更新，因此 Android 的提交不會顯示平台耗時。
- **Recorder**：記錄你在應用程式中的點擊、輸入、清除與捲動操作，並寫成一段可直接複製進 `_test.go` 檔案的 `nsgotest` 測試程式碼。在 Elements 面板中，**Assert shown** 與 **Assert text** 可為選取的檢視加入斷言檢查。文字檢查會以應用程式當時顯示的語言記錄文字，因此請在測試執行所使用的語言下進行錄製。
- **Highlight updates** 會框出每次提交所變更的檢視，效果與 `-highlight` 相同。
- **Layout** 會在裝置上顯示或隱藏[版面配置稽核](#layout)，功能與啟動時加上 `-layout` 相同。

TSX 應用程式的狀態、計算與寫入位置會透過套件的 source map 以 TSX 檔案與行號命名，並連結至應用程式中的原始檔案。

DevTools 透過 `nsgo dev` 的伺服器讀取應用程式資訊，該伺服器會監聽你的網路以便手機連線。每次執行都會產生一組權杖（token），`nsgo dev` 印出並開啟的 DevTools 與網頁目標網址在 `#` 後方皆帶有此權杖，且伺服器僅回應攜帶該權杖的請求，因此請務必透過這些網址開啟 DevTools 與網頁目標。任何取得該網址的人都能在執行期間修改應用程式狀態，並在 Recorder 產生的測試中加入步驟，因此執行錄製好的測試前請先檢視其內容。區域網路中的其他人與你造訪的其他網站網頁皆無法存取上述任何資訊。

## 更新追蹤

```sh
go tool nsgo dev -highlight
```

`-highlight` 會將每次提交所變更的檢視框出半秒鐘，讓你在使用應用程式的同時，一眼看出是否有超出預期範圍的重繪。外框會繪製在所有視窗、面板、強制回應視窗、iOS 工作表以及嵌入檢視（`nsgo.NewView`）上方。執行期間也可從 DevTools 隨時開啟或關閉此功能。

```sh
go tool nsgo dev -trace
```

`-trace` 會印出執行中應用程式的每一次提交：編號、耗時以及發送的掛載項目，讓你能掌握某次更新建立、變更或移除了哪些檢視。在項目上方會列出觸發該次提交的原因：自上次提交以來寫入訊號的程式碼行號（寫入次數最多者排在最前），以及執行的繫結（binding）、效果（effect）與 memo 數量。

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

若某一行重複寫入多次，或單次寫入觸發了大量繫結，便是適合以批次寫入（`cx.Batch`）或縮小繫結讀取範圍的地方。`go tool nsgo test -trace` 則會在測試的每一個步驟下方印出相同資訊。

## 執行期效能分析檔

使用 `nsgo dev` 擷取執行中應用程式的 Go CPU 活動、堆積（heap）或執行追蹤（execution trace）。應用程式會將擷取資料傳回開發伺服器，並由伺服器儲存於執行該指令的機器上。

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

CPU 效能分析範例同樣適用於桌面、iPhone 或 iOS 模擬器。在 iPhone 上，若系統跳出提示請允許「區域網路」存取權限。堆積分析屬於快照形式，亦可用於網頁目標。若需排程器與 goroutine 事件，請執行 `go tool nsgo dev -profile trace`，再以 `go tool trace` 開啟 `.nsgo/profiles/execution.trace`。CPU 與執行追蹤預設持續 10 秒。`-profile-output` 可指定本機輸出檔案路徑。進行元件預覽時，預設路徑與相對路徑皆以應用程式目錄為基準。開發執行會儲存第一份完整的擷取結果，隨後繼續執行應用程式。

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Go 的瀏覽器執行環境支援堆積分析與執行追蹤。若要進行瀏覽器 CPU 取樣，請使用瀏覽器的 Performance 工具。`-profile` 與 `-release` 只能擇一使用。透過 `nsgo build` 發佈時不會包含開發擷取用戶端。參閱 [Go 執行期診斷文件](https://go.dev/doc/diagnostics)。

## 版面配置

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` 會隨應用程式啟動 DevTools 的版面配置稽核，覆蓋於所有視窗、面板、iOS 工作表與嵌入檢視之上，框出每個檢視的區域：藍色表示邊框方塊（border box）、綠色表示內距的內緣、橘色表示外距的外緣，並在捲動檢視或裁剪檢視遮蔽處予以截斷。`grid` 會繪製 8 點間距的格線，而 `grid=N` 則繪製 N 點間距的格線（最小為 4）。數值請接在 `=` 後方：單獨使用旗標等同於 `bounds`。

角落的讀數會以點為單位顯示視窗大小，並附帶按鈕可開關邊界方塊、格線與尺規。在 Apple TV 與其他僅支援遙控器的裝置上，焦點可移動至讀數按鈕，無須離開應用程式即可透過遙控器切換。開啟尺規時，點擊動作會選取兩個點位而非傳遞給應用程式，讀數會顯示兩點在水平與垂直方向上的距離；第三次點擊則會重新開始測量。在 Apple TV 上或使用鍵盤時，方向鍵（或滑動 Siri Remote 觸控板）可移動畫面上的十字準星，按下 Select、Return 或 Space 即可選取點位，並在第二點鎖定前即時更新距離。

| 按鍵 | 效果 |
| --- | --- |
| `Cmd+Option+Shift+L`、`Ctrl+Alt+Shift+L` | 在加上 `-layout` 的建置中顯示或隱藏稽核覆蓋層 |
| 方向鍵、遙控器方向鍵／滑動 | 依格線步距移動尺規十字準星（預設為 8 pt） |
| `Shift` + 方向鍵 | 以 1 pt 為單位移動尺規十字準星 |
| Select、`Return`、`Space` | 在測量時選取十字準星所在的點位 |
| `Esc`、返回鍵、Menu | 結束測量 |

若你的應用程式繫結了相同的按鍵，將優先採用應用程式的繫結。在沒有實體按鍵的裝置（例如手機）上，請同時加上 `-devtools` 執行，並透過 DevTools 的 **Layout** 開關稽核層；隱藏它即可點擊被讀數遮住的控制項。原生後端與網頁會讀取呈現矩陣（presentation matrix），因此邊界、Pick 與稽核覆蓋層都會跟隨實際顯示的變換、動畫與捲動位置；watchOS 則跟隨其仿射繪圖。HarmonyOS 與小程式在渲染層回報幾何資訊前，會保留 Go 端追蹤器。重繪這些覆蓋層在每次更新都會消耗時間，因此測量效能時請關閉它們。

## 效能監測

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` 會在 macOS、iOS、Windows、Linux 與 Android 的應用程式畫面上顯示效能監視器。它會量測影格與提交次數，Android 也會額外顯示原生文字往返耗時。開發建置內含開發用戶端；若要進行真實效能測量，請加上 `-release`，以與 `nsgo build` 相同的方式建置。

## 語言與外觀

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` 會以指定語系啟動應用程式，亦可使用偽語系（參閱[在地化](localization.md)）。

## 代理程式

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation` 讓 AI 代理程式或端對端測試執行器能像真人一樣操作執行中的應用程式，目前已支援 macOS、Windows、Linux（`-target native`）、Android（`-target android`）、網頁（`-target web`）、在各開發者工具模擬器中的小程式（`-target miniprogram_wechat`、`miniprogram_qq`、`miniprogram_alipay` 或 `miniprogram_douyin`），以及 Apple 裝置與模擬器（`-target ios`、`tvos`、`watchos`、`ios_simulator`、`tvos_simulator` 或 `watchos_simulator`）：可列出視窗、檢視畫面結構摘要、點擊、輸入、傳送按鍵、捲動、拖曳、懸停、選取選單項目、回應警告框、等待元素出現及擷取螢幕截圖。代理程式透過 `go tool nsgo mcp` 經由 MCP 連線，該指令會沿用目前目錄執行中的 `dev -automation`，或在未啟動時以相同旗標（例如 `-target android`）啟動一個執行個體，並在用戶端離開時停止它：

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

其他用戶端也使用相同指令，在應用程式目錄中執行 `go` 並傳入參數 `tool nsgo mcp`。檢視可透過[測試](testing.md#a-test)所使用的選擇器命名，或使用快照印出的 `view:` 參照（一旦存檔重啟應用程式，該參照便會失效）。

代理程式會直接操作 `dev` 執行時應用程式的真實資料。只有 `dev -automation` 建置會包含此能力，且唯有本機持有 `.nsgo/automation.yaml`（僅限你自己可讀取）內權杖的工具才能連線存取。

### 應用程式自訂 MCP 工具

在 Go 應用程式或函式庫的引擎 goroutine 上呼叫 `automation.Register`。例如，以下函式能讓代理程式讀取目前顯示的場景 ID。

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

工具名稱在應用程式的計算圖中必須唯一，且不可與內建工具名稱重複。`nsgo mcp` 會持續列出該工具，直到對應的作用域被清除為止。Handler 會在引擎 goroutine 上執行，因此可以直接讀取或設定訊號。它接收一個 JSON 物件、驗證所需的參數，並回傳可編碼為 JSON 的值或錯誤。

在測試中，可在應用程式啟動時擷取作用域，於 `app.Post` 內呼叫 `automation.Call(cx, ctx, name, args)`，然後在檢查結果或檢視前呼叫 `app.Settle()`。此註冊機制僅在 `dev -automation` 與測試中有效，在發佈建置中則為無操作（no-op）。

## 所有建置共通的旗標

任何建置出的應用程式（包含發佈套件）在啟動時都接受 NSGo 自帶的旗標，這些旗標會覆蓋 `nsgo.yaml` 與應用程式碼的設定，且不會傳遞給應用程式自身的旗標解析：

| 旗標 | 效果 |
| --- | --- |
| `--nsgo-fps` | 在 macOS、iOS、Windows、Linux 與 Android 上顯示效能監視器 |
| `--nsgo-lang=zh-Hans` | 以指定語系啟動應用程式，亦可使用偽語系 |
| `--nsgo-appearance=dark` | 以 `light`、`dark` 或跟隨系統（`system`）的外觀啟動應用程式 |
| `--nsgo-os=ios` | 將網頁建置以 `macos`、`windows`、`linux`、`ios` 或 `android` 模式執行（`web` 即為其本身） |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

在網頁上，這些參數會放在頁面網址的查詢字串中：`?nsgo-fps&nsgo-lang=de`。

`--nsgo-os` 可在瀏覽器中預覽其他平台的版面配置：應用程式會讀取該作業系統與其裝置慣例（iOS 與 Android 為手機，其餘為桌面），且 `Env.Simulated` 會回傳 true。瀏覽器不具備該平台的原生元件：因此會以 `NavHeader` 與自行繪製的分頁列取代系統元件，沒有應用程式選單、原生彈出選單（`desktop.PopupMenu`）或視窗函式，且 `mobile.IsAvailable` 與 `desktop.IsAvailable` 皆為 false。自行繪製的選單（如 `ui.ContextMenu` 與 `ui.Menu`）仍可正常運作。快捷鍵則跟隨訪客實際使用的鍵盤。其他平台建置會忽略此旗標並記錄一則警告。
