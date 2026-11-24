---
title: 平台服務
description: 警告對話框、剪貼簿、檔案、照片、分享、儲存空間、機密資料、語音辨識、文字朗讀、網路連線、登入、App 內購買與應用程式的生命週期。
group: 平台功能
order: 44
---

## 警告對話框與剪貼簿

`cx.ShowAlert` 會顯示平台的警告對話框或動作表單。其 `Actions` 會以索引回傳，而角色（role）會讓取消與破壞性按鈕以平台自己的方式繪製。在 macOS 上，當視窗可見時，警告對話框與檔案選擇器會以工作表（sheet）呈現。若視窗已隱藏、最小化或位於螢幕外，或是沒有視窗的系統匣應用程式，對話框則會改為獨立開啟。關閉發出請求的視窗會取消其對話框。

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` 寫入文字，`SetClipboardImage` 寫入圖片。`ReadClipboard`、`ReadClipboardImage` 與 `ReadClipboardFiles` 以非同步方式回應，而 `HasClipboardText` 與 `HasClipboardImage` 可讀取剪貼簿目前的內容類型，且不會觸發貼上通知。`core.FileFromURL` 會將 `ReadClipboardFiles` 與 `View.OnFileDrop` 提供的 URL 開啟為 `core.File`：包含 `file:` URL，以及在網頁上的 `blob:` URL。

tvOS 沒有公開的貼上板。寫入剪貼簿會被忽略，讀取不會回傳任何內容，而存在檢查一律回傳 false。

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

在 macOS 與 Windows 上，`ReadClipboardItems` 會提供所有可用位元組表示形式的快照，上限為 48 MiB。每個項目都帶有依序排列的 `ClipboardRepresentation` 值，包含原生的 `Type` 與其原始 `Data`，也包含資料為空、僅表示存在的格式。`ClipboardFormats` 會列出原生格式名稱，而不複製其內容。若要還原快照，請將它傳給 `WriteClipboardItems`，並可附上一個接收寫入錯誤的回呼函式。傳入空切片會清空剪貼簿。比較 `ClipboardChangeCount` 回傳的值是否相等，即可偵測到新的複製動作，即使複製的文字完全相同也一樣。

macOS 會保留項目邊界與貼上板類型名稱。Windows 使用單一項目，並保留以位元組為基礎的 Win32 名稱（如 `CF_HDROP`）與已註冊的格式（如 `Rich Text Format`）。寫入時它會合併具有不同格式的項目，並以一個零位元組表示空內容。GDI 控制代碼、擁有者顯示格式與私有的數字格式 ID 會被略過。在其他平台上，讀取與寫入的回呼函式會收到 `CodeUnavailable`，而計數回呼函式會收到 `-1`。

## 開啟連結與檔案

`cx.OpenURL` 會在系統的瀏覽器或應用程式中開啟 URL，而 `core.CheckURL` 會說明它拒絕哪些 URL：沒有 scheme、超過 16 KiB、含有空白、沒有主機的網址，以及 `javascript:`、`data:`、`blob:`、`file:`、`about:` 與 `asset:`。

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` 會以與檔案類型關聯的應用程式開啟應用程式所擁有的檔案，而 `cx.Share` 會為檔案、文字或兩者顯示系統的分享表單，支援 macOS、iOS、Android、Windows、Linux（透過桌面 portal 或檔案管理員）與網頁（`navigator.share`）。`core.SharedFile` 是磁碟上的一個路徑——或者對於網頁上的 `Share`，以及任何檔案保存在記憶體中的情況，則是其 `Data` 位元組——並帶有另一個應用程式看到的名稱與類型。在桌面平台上，`desktop.RevealFile` 會在 Finder、檔案總管或檔案管理員中顯示並選取一個檔案或資料夾，而 `desktop.TrashFile` 會將它移到垃圾桶或資源回收筒；它絕不會永久刪除，當磁碟區沒有垃圾桶，或垃圾桶無法保留該項目（例如檔案大於資源回收筒的容量）時，它會回應 `core.ErrNoTrash` 並讓項目留在原處。

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` 會接收針對 [`android.share_types`](building.md#incoming-shares-on-android) 或 [`apple.share_types`](building.md#incoming-shares-on-ios) 分享而來的文字與檔案。Android 的分享可以啟動應用程式；iOS 則會將它們排入 App Group 中，直到應用程式處於作用中或使用者下次開啟它。待處理的分享會等待第一個處理常式。iOS 會保留收件匣批次，直到回呼函式返回；跨越行程結束時，傳遞保證至少一次。文字會放在 `Text` 中，檔案則以可讀取的快取副本送達，並帶有 `Path`、`Name` 與 `MIME`。若檔案需要在快取之外長久保存，請將它移動或複製到永久儲存空間。

`desktop.LookUp` 會為一段文字顯示系統的字典面板，就像在 macOS 上用三指輕點一樣，繪製在檢視中的某個方框上方：即選取範圍的文字與方框，以及其 `Selectable` 檢視的 `Ref`。`desktop.CanLookUp` 會說明是否可以使用，僅限 macOS 上的視窗；Windows、Linux 與網頁沒有這樣的面板。在測試中，`nsgotest.App.LookUps` 會列出在 `nsgotest.Environment` 設定了 `CanLookUp` 時所顯示的面板。

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` 會請求檔案預覽，從指定索引開始，並在面板開啟期間再次呼叫時更新檔案。`desktop.CanQuickLook` 會回報在 macOS 與 Windows 上應用程式自己的視窗中是否可用，在 Linux 上則是當工作階段提供 GNOME Sushi 正在執行或可啟動的 D-Bus 服務時可用。Windows 使用該檔案類型已註冊的預覽處理常式；支援的格式取決於已安裝的處理常式或預覽程式。回傳 true 表示已向平台提出請求，而不代表平台已成功呈現檔案。

按空白鍵或 Escape 會關閉面板。在 Windows 上，以及在 X11 上會回報選取狀態與擁有者的 Sushi 版本中，方向鍵可在請求的檔案間選取。在 Wayland 上，Linux 目前會開啟沒有父視窗的預覽；無法自動隨擁有者關閉，也無法以方向鍵選取。KDE 專屬的預覽程式與網頁不受支援。

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## 原生選單圖示

`desktop.MenuItem.Icon` 會在 macOS、Windows 與 Linux 的應用程式選單、Dock 選單、系統匣選單與彈出選單中，於項目旁顯示圖示。`desktop.MenuIcon` 接受已解析的向量圖示（`SVG`，例如 `lucide.Folder()`）、編碼後的圖片位元組（`Data`，PNG、JPEG、GIF、WebP 或 SVG），或是本機／資源／像素的 `core.ImageSource`（`Source`）。設定 `Template` 會以選單的文字顏色為單色圖片著色，並跟隨淺色與深色外觀（在 macOS 上還包含反白狀態）；單色的 `SVG` 圖示預設會被視為範本。

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

圖示會從本機來源同步載入；遠端 URL 與無效的圖片會讓項目不顯示圖示。選單標籤、快捷鍵與核取／單選指示器會保留其原生行為。

在測試中，`nsgotest.MenuItem.Icon` 會在 `app.Menu`、`app.DockMenu` 與 `app.PopupMenu` 中記錄每個項目的圖示。

## 照片與檔案

`cx.PickImages` 會顯示系統的照片選擇器，並以 `core.PickedFile` 回應，應用程式可讀取其位元組、名稱、類型與像素尺寸。`cx.TakePhoto` 會在平台具備相機時顯示相機；`cx.Env().CanTakePhoto` 會說明是否具備。在 macOS、Windows 與 Linux 上，它會顯示掃描器的相機預覽與「拍照」按鈕。`cx.ScanCode` 會開啟相機預覽，並以它讀到的第一個條碼回應——除非 `ScanOptions.Kinds` 要求其他類型，否則為 QR 碼——或在使用者關閉時回應 nil；`cx.Env().CanScanCode` 會說明平台是否有掃描器，支援具備相機的 macOS、iOS 與 Android 裝置、具備相機的 Windows 電腦、具備 Camera portal 與 GStreamer `pipewiresrc` 的 Linux 桌面，以及具備 `getUserMedia` 與 `BarcodeDetector` 的瀏覽器。當發出請求的視窗可見時，macOS 會呈現工作表，Windows 與 Linux 會在視窗上方呈現強制回應視窗，而網頁則在頁面上方呈現覆蓋層。在 macOS 與 iOS 上請將 `NSCameraUsageDescription` 放入 `apple.info`，在 Android 上則將 `CAMERA` 放入 `android.permissions`。Android、Windows 與 Linux 可讀取 PDF417 以外的所有類型。請將條碼的文字視為不可信任的輸入。

在 tvOS 上，`PickImages` 會以沒有圖片的結果回應，而 `TakePhoto` 會回傳 `core.ErrNoCamera`；NSGo 不提供接續互通相機（Continuity Camera）選擇器。

在測試中，`nsgotest.App.PickImages`、`nsgotest.App.TakePhoto` 與 `nsgotest.App.ScanCode` 會接收挑選與拍攝的 `nsgotest.ImagePick` 紀錄，以及掃描的 `nsgotest.CodeScan` 紀錄，並代替系統 UI 回應。這些掛鉤可在無頭模式下運作，也可在支援的 `nsgo test -target` 主機上運作。`nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` 可讓沒有相機的目標擁有相機。

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` 可挑選任何類型的檔案，`cx.PickDirectories` 可將資料夾挑選為 `fs.FS`，而 `cx.SaveFile` 會透過系統的儲存對話框儲存檔案或位元組。這兩個選擇器都接受 `Title`、`Button` 標籤與 `Start` 資料夾，而 `PickFilesOptions.TypesName` 用於命名類型選單，讓應用程式能以自己的語言顯示它們；這些只是提示，macOS、Windows 與 Linux 會遵循，iOS 只遵循 `Start`，Android 與網頁則完全不遵循。`cx.PickScreenColor` 可在 `cx.Env().CanPickScreenColor()` 為 true 的地方挑選螢幕上任意位置的顏色：macOS、Windows、具備桌面 Screenshot portal 的 Linux，以及具備 `EyeDropper` 的瀏覽器。

將 `core.SaveOptions.Start` 設為一個已存在的絕對路徑資料夾，即可在 macOS、Windows、Linux 與 iOS 上讓儲存對話框從該處開啟。若提示為空或無法使用，或是在其他目標上，則由系統選擇資料夾。

設定 `core.SaveOptions.Accept` 可在寫入前驗證所選的目的地。它會在引擎的 goroutine 上執行。回傳錯誤會讓目的地維持不變。在 macOS、Windows 以及 Linux 的同行程 GTK 選擇器中，對話框會顯示原因並讓使用者選擇其他路徑。在拒絕後取消會回報 `saved == false`、被拒絕的路徑與該錯誤。在 Android 與 Linux 桌面 portal 中，拒絕會以該結果完成儲存。Android 讓使用者選擇資料夾，並提供一個 tree URI，以請求的檔名作為其 `name` 查詢參數。它會在核准後建立文件，並在成功時回報新文件的 URI。若檢查未在五秒內回應，平台會放棄寫入。iOS 與 HarmonyOS 對需要檢查的儲存會回報錯誤。網頁沒有目的地路徑，會使用其一般的儲存流程。

在無頭測試中，`App.PickFiles`、`App.PickDirectories` 與 `App.SaveFile` 會接收 `nsgotest.FilePick`、`nsgotest.DirectoryPick` 與 `nsgotest.FileSave` 紀錄。相同的紀錄也存放在 `app.FilePicks`、`app.DirectoryPicks` 與 `app.Saves` 中。

### 內建資源

將檔案放在應用程式的 `resources/` 目錄下，並使用 `cx.ReadAsset("asset:maps/level.bin", done)` 從 Go 讀取。回呼函式會在讀取完成後於引擎的 goroutine 上收到 `([]byte, error)`，若其擁有者已消失或再次執行，則會被捨棄。使用 `errors.Is(err, core.ErrAssetNotFound)` 來辨識不存在的檔案。讀取會將整個檔案載入記憶體。

## 拉取式 PCM 音訊

使用 `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` 播放應用程式自行解碼或合成的交錯式 Float32 PCM。`Start` 開始拉取樣本，`Pause` 停止拉取，而擁有它的作用域在結束時會關閉輸出。macOS、iOS、Android、Windows、Linux 與網頁支援此串流。其他目標會回傳 `audio.ErrUnsupported`。

`Render` 會回傳它填入的影格數。它在音訊執行緒上執行，必須避免阻塞、記憶體配置與存取圖（graph）。填入不足時，其餘部分會播放靜音並遞增 `Underruns`。`Position` 會回傳單調遞增的影格計數，以及該影格的主機時間。使用 `Flush` 捨棄待處理的影格，使用 `Latency` 讀取緩衝區延遲。路由變更與中斷會暫停輸出，並在引擎上呼叫 `OnInterrupt`。應用程式可以用 `Start` 恢復播放。

## 儲存空間

`cx.Storage()` 是應用程式的鍵值儲存空間，保存在裝置上，並由應用程式的所有圖共用。讀取為同步操作，在計算中讀取會使該計算訂閱其變更；超過配額（`core.StorageQuota`，預設 5 MiB）時，`SetItem` 會回傳 `core.StorageFullError`。它是一個使用者的任何程式都能讀取的一般檔案，因此請將權杖與密碼改存在[機密資料](#secrets)中。

`cx.Store()` 會回傳底層的存放區，供以 `core.RootOptions.Storage` 建立另一個根的主機使用，例如在自己的 goroutine 上執行外掛程式的主機：兩個根此時會共用應用程式的項目以及保存在裝置上的內容。它是整個存放區，而不是 `cx.ProvideStorage` 提供給非應用程式自行撰寫之程式碼的分割區。

自行提供持久化機制的主機會以 `core.StorageBackend` 呼叫 `core.NewStore(backend)`：`Load() (map[string]string, error)` 會載入一次，而 `Write(core.StorageChange)` 會依序接收變更。請依 `change.Op`（`core.StorageSet`、`core.StorageRemove`、`core.StorageClear`）分支處理，並使用 `change.Key` 與 `change.Value`。若儲存空間只保存在記憶體中，請傳入 `nil`。

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` 會列出鍵，`RemoveItem` 刪除一個鍵，而 `Clear` 會清空儲存空間。`cx.DataDir()` 與 `cx.CacheDir()` 提供應用程式的資料夾，用於存放鍵值儲存空間容納不下的檔案；`cx.LogsDir()` 提供平台用於記錄檔的目錄，而 `Scope.ProvideStorage` 可給予子樹一個專屬的分割區。

當設定了 `app_clip` 與明確的 `apple.app_group` 時，iOS App Clip 與其完整應用程式會共用這些資料與儲存位置；它們的快取則保持分開。啟用群組不會遷移既有的沙盒資料。[App Clip 儲存空間](building.md#app-clips)說明了設定與簽署方式。

在 HarmonyOS 上，`os.ReadFile`、`os.WriteFile` 與 `os.Rename` 等 Go 檔案操作可在這些目錄與 `os.TempDir()` 下運作。相對路徑從 files 目錄開始。建議性檔案鎖與符號連結無法使用。

### 當機報告

原生發行版建置會將 Go 當機追蹤保存在 `cx.LogsDir()/crashes` 下。Apple 會記錄未捕捉的 Objective-C 例外，Android 會記錄未捕捉的 Java 例外，而 Windows 會記錄到達其頂層篩選器的 SEH 例外。在下次啟動時以 `cx.CrashReports()` 列出待處理的報告。每份報告都會標明產生它的版本、建置編號與平台。呼叫 `Open()` 讀取診斷文字，關閉讀取器，然後在應用程式處理完報告後呼叫 `Acknowledge()` 將其刪除。請在 goroutine 上執行這些檔案操作。

## 磁碟空間

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` 會以非同步方式查詢包含 `cx.DataDir()` 的磁碟區，並在引擎 goroutine 上呼叫回呼函式。`Total` 是系統回報的容量，`Available` 是應用程式可用於使用者要求之操作的空間，而 `Opportunistic` 是它可用於快取與預先擷取的空間。三者皆以位元組為單位。在系統會回報該數值的平台上，`Available` 包含系統會依需求回收的空間。只有單一可用空間數值的平台會在兩個欄位中都回傳該數值。在網頁上，`Total` 是瀏覽器為該來源估算的儲存配額，而 `Available` 是未使用的部分。當服務無法使用時，它會回傳 `core.ErrDiskSpaceUnavailable`。

Apple 建置要求在 `nsgo.yaml` 的 `apple.privacy.accessed_apis` 中宣告 `NSPrivacyAccessedAPICategoryDiskSpace`。宣告 `E174.1` 以便在寫入前檢查空間，或在空間不足時釋放檔案，且該變更須能被使用者察覺。宣告 `85F4.1` 以便向使用者顯示可用空間。應用程式必須遵守 Apple 對使用與分享這些讀數的限制。未宣告該類別的建置會回傳 `core.ErrNotDeclared`。[Apple 核准的理由](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

使用 `nsgotest.DiskSpace(space, err)` 設定測試的回應。

## 雲端儲存空間

`cloud` 套件可在裝置間同步小型鍵值項目（`cloud.Get`、`cloud.Set`、`cloud.Delete`、`cloud.Keys`），以及較大的檔案或存檔快照（`cloud.ReadFile`、`cloud.WriteFile`、`cloud.DeleteFile`、`cloud.ListFiles`）：在 macOS 與 iOS 上使用 iCloud Key-Value Store 與 ubiquity 文件容器，在 Android 上使用 Google Play 遊戲服務的 Saved Games。

每項操作都會在引擎 goroutine 上傳遞其回呼函式，若 `cx` 已被釋放則略過。`cx.Env().CanUseCloudStorage()` 會回報雲端儲存空間是否可用；當使用者已登出、缺少權利（entitlement），或平台沒有原生雲端後端（Windows、Linux 與網頁）時，每次呼叫都會回傳 `cloud.ErrUnavailable`，絕不會默默捨棄寫入。

```go
if cx.Env().CanUseCloudStorage() {
	cloud.WriteFile(cx, "saves/slot1.sav", saveBytes, func(err error) {
		if err != nil {
			status.Set(err.Error())
		}
	})
}

cloud.WatchChanges(cx, func(ev cloud.ChangeEvent) {
	reload(ev.Keys, ev.Files)
})

cloud.OnConflict(cx, func(c cloud.Conflict) {
	_ = c.Resolve(mergeSaves(c.Local, c.Remote))
})
```

`cloud.WatchChanges` 與 `cloud.OnConflict` 會在引擎 goroutine 上傳遞來自外部多裝置的變更與版本衝突，並在 `cx` 被釋放時自動清理。讀取有衝突的檔案會回傳 `*cloud.ConflictError`（符合 `errors.Is(err, cloud.ErrConflict)`）。

若要在 `nsgo build` 中啟用 Apple iCloud 權利，請在 `nsgo.yaml` 中加入 `cloud:`（或 `icloud:`）區段：

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

簽署描述檔必須授予你所要求的 iCloud 功能。在 macOS 上，只有 Mac App Store 建置（`sign.macos_profile`）會帶有 iCloud，因為 macOS 只在有佈建描述檔時才會授予；Developer ID 建置會將其略過並發出警告。在 Android 上，雲端儲存空間需要應用程式中包含 Play Games Services v2 SDK，以及已登入的玩家。

在 Steam 等桌面商店上，Steam Auto-Cloud 會直接同步 `cx.DataDir()`，無需任何額外程式碼——只要持續將本機存檔寫入該處，讓 Steam 同步該目錄即可。

在測試中，`nsgotest` 會為每個應用程式提供一個隔離的記憶體內雲端存放區：以 `nsgotest.CloudAvailable` 或 `app.SetCloudAvailable` 切換可用性，以 `app.SimulateCloudChange` 模擬外部多裝置更新，並以 `app.SimulateCloudConflict` 測試衝突解決。

## 機密資料

`cx.Secrets()` 是應用程式的安全存放區，用於存放重新整理權杖、API 金鑰與密碼：依帳戶 `Get`、`Set` 與 `Delete` 一個字串，並由平台加以保護。不存在的帳戶會讀為 `("", false, nil)`，而在平台沒有安全存放區的地方（例如網頁），每次呼叫都會回傳 `core.ErrSecretsUnavailable`；NSGo 絕不會退回使用一般儲存空間。系統可能會讓呼叫等待，例如鑰匙圈詢問使用者時，因此請在引擎的 goroutine 之外呼叫它們，並以 `cx.Post` 將結果傳回。

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| 平台 | 機密資料的保存位置 |
| --- | --- |
| macOS | 登入鑰匙圈中的一般密碼：`kSecAttrService` 為服務，`kSecAttrAccount` 為帳戶 |
| iOS、tvOS、watchOS | 鑰匙圈中的一般密碼，方式相同 |
| Windows | 認證管理員中的一般認證，其目標為 `service/account`，最多 2560 位元組 |
| Linux | Secret Service（GNOME Keyring、KWallet）中帶有 `service` 與 `account` 屬性的項目；若工作階段未執行任何 Secret Service 則無法使用 |
| Android | 以 Android Keystore 的 AES 金鑰加密，存放在應用程式的私有檔案中，不納入備份；讀取時也會退回讀取 `expo-secure-store` 加密存入其 `SecureStore` `SharedPreferences` 的值 |
| 微信小程式 | 平台加密儲存空間（`wx.setStorage` / `wx.getStorage` 搭配 `encrypt: true`，基礎函式庫 2.21.3 以上）；較舊的基礎函式庫無法使用 |
| 網頁、其他小程式 | 無法使用（`core.ErrSecretsUnavailable`）：瀏覽器沒有受作業系統保護的用戶端機密存放區（請將網頁工作階段保存在由伺服器設定的 `HttpOnly` cookie 中），而 QQ、支付寶與抖音小程式缺少加密儲存空間 |

服務預設為應用程式的 ID（`core.App().ID`）。從其他框架遷移過來的應用程式，可藉由指定該框架寫入之項目的名稱，讓使用者保持登入：`Service`，以及在 Apple 平台上的 `AccessGroup` 與 `Accessible`。在 Android 上，將 `Service` 設為 `expo-secure-store` 的 `keychainService`（預設為 `"key_v1"`），即可讀取它加密存入 `SharedPreferences` 的值；寫入或刪除會清除已遷移的項目。在 iOS 上，`core.SecretAfterFirstUnlock` 讓背景工作能在手機鎖定時讀取項目，而 `ThisDeviceOnly` 的存取性設定會讓項目不納入備份。在 macOS 上，`AccessGroup` 會將項目移至資料保護鑰匙圈，這需要由團隊簽署、且其 `keychain-access-groups` 權利列出該群組的建置，並在 `nsgo.yaml` 的 `macos.entitlements` 下為 Mac App Store 與 Developer ID 建置進行設定。

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` 會將應用程式的機密資料保存在記憶體中的 `App.Secrets`，而 `nsgotest.Secrets` 可讓測試一開始就帶有一些機密資料，因此測試絕不會碰到機器的鑰匙圈。TSX 套件透過 `secrets` 能力存取存放區，並使用其專屬的服務。

## 網路連線

`cx.Env().Network()` 會回報系統是否有可用的路由（`Online`），以及系統是否將該路由視為計量或昂貴的連線（`Expensive`）。繫結會像追蹤其他環境值一樣追蹤它。`cx.OnNetworkChange(func(core.Network))` 會在其作用域存續期間，針對每次變更在引擎的 goroutine 上呼叫處理常式。系統的路由並不代表某個特定伺服器是否會回應。無法回報連線狀態的目標預設為上線，而未知的費用預設為 false。

透過 `nsgotest.Environment` 或 `App.SetEnvironment` 設定 `core.Environment{Network: &core.Network{Online: false}}`，即可測試離線行為。

Go 本身的 `net/http` 可在每個目標上發出請求，並使用系統的 TLS 與代理伺服器設定。信號屬於引擎的 goroutine，因此來自其他 goroutine 的回應需透過 `cx.Post` 傳給它們。

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` 會在圖的 goroutine 上執行其函式，在那裡可以設定信號。在 Android 上，資訊清單會要求網際網路權限，而圖片下載會自行快取到磁碟上。

`core.NewResource` 對從來源載入的值也做同樣的事。它的擷取函式會在自己的 goroutine 上執行，並在來源變更時重新載入：

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`、`items.Err()` 與 `items.Loading()` 是繫結會追蹤的信號。新的查詢會透過 `ctx` 取消仍在執行中的請求，因此較晚抵達的回應絕不會取代較新的回應。`items.Refetch()` 會重試，並在載入期間保留上一份清單，而 `items.Mutate` 可立即設定一個值，用於樂觀更新。

`core.Suspense` 會在其內容所讀取的資源載入第一個值期間顯示後備內容，因此畫面不需要自己的載入旗標：

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

內容在等待期間會保持掛載但隱藏，因此值一抵達就能立即顯示。重新擷取時會保持顯示上一個值。

### 探索區域網路服務

在 macOS、iOS、Android、Windows 與 Linux 上，`cx.AdvertiseService` 會發布一個監聽中的連接埠與 TXT 屬性。`cx.BrowseServices` 會回報已解析的服務，以及其主機、連接埠與 TXT 屬性。`Host` 可能是 DNS 主機名稱或 IP 位址。當服務離開時，回呼函式會再次收到它，並設定 `Removed`。請檢查 `cx.Env().CanDiscoverServices()` 以確認平台支援。兩個呼叫都會回傳一個停止函式，並在其作用域被釋放時結束。平台錯誤會以非同步方式在引擎 goroutine 上回報。Linux 使用 Avahi，若該常駐程式無法使用則會回報錯誤。

若要在 Apple 平台上使用 Bonjour，請在 `nsgo.yaml` 中宣告服務類型並說明區域網路存取的用途。系統會管理權限提示並記住使用者的選擇。

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android 使用 `NsdManager`。Android 12 及更早版本，以及 T Extensions 7 之前的 Android 13，需要 `CHANGE_WIFI_MULTICAST_STATE` 才能進行 Wi-Fi 探索。獨立建置會宣告該權限。若你將 NSGo 嵌入 Android 主機中，請為這些裝置將它加入主機的資訊清單。在沒有 Android 原始 TXT API（API 37 或 T Extensions 22）的裝置上，發布的 TXT 值必須是有效的 UTF-8。瀏覽時會保留原始 TXT 值。若 Android 主機以 API 37 或更高版本為目標，則必須在 Android 17 或更新版本上宣告並請求 `ACCESS_LOCAL_NETWORK`。

在無頭測試中，使用 `nsgotest.LocalServices` 提供瀏覽結果。

## 正在播放

使用 `cx.SetNowPlaying(core.NowPlaying{...})` 在 macOS 的控制中心與選單列中顯示應用程式目前的媒體。提供 `Title`、`Artist`、`Album`，以及可選的 `Artwork`（型別為 `*core.ImageSource`）。`Duration` 為零表示直播串流。提供目前的 `Position` 與播放 `Rate`（暫停時為零，正常速度時為一）。當曲目、位置或速率變更時再次發布。系統會在兩次呼叫之間推進位置。

在 `Commands` 中列出支援的傳輸控制，例如 `core.MediaPlay`、`core.MediaPause`、`core.MediaTogglePlayPause` 與 `core.MediaSeek`。以 `cx.OnMediaCommand` 處理請求。回呼函式會收到命令的 `Kind`，若為搜尋（seek）則還會收到請求的 `Position`。最新的存續處理常式會接收命令。應用程式播放結束時，請呼叫 `cx.ClearNowPlaying()`。當應用程式手動發布時，macOS 上受控制的影片會暫停自己的發布。請檢查 `cx.Env().CanShowNowPlaying()` 以確認平台支援。

## 背景下載

在獨立的 iOS 與 macOS 應用程式以及 Android 上使用 `cx.Download` 與 `cx.OnDownloadUpdate`。嵌入式 Apple 檢視沒有傳輸主機。Android 應用程式需在 `nsgo.yaml` 中啟用 `android.background_downloads: true`。請在應用程式可見時，由使用者操作啟動下載。使用絕對路徑作為目的地，且其父目錄必須已存在並可供應用程式寫入。

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

傳輸使用 GET，並接受可選的請求標頭。除非 `CertificatePEM` 釘選了伺服器的葉憑證，否則它使用一般的 HTTPS 信任存放區。釘選同時要求主機名稱相符且憑證有效。請使用最終的 URL，因為服務要求該 URL 回傳成功的回應。

在 Apple 上，位於公開主機名稱的自簽憑證需要在 `apple.info` 中為該主機設定 App Transport Security 例外。傳輸仍會檢查其精確的憑證釘選、主機名稱與有效性。本機 NAS 連線還需要說明存取用途的 `NSLocalNetworkUsageDescription`，而未限定網域或 `.local` 的主機則使用 `NSAllowsLocalNetworking`。[Apple 的 ATS 設定](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Android 前景服務在活動離開畫面後仍會繼續執行。它會發布進度通知，但受 Android 通知權限的限制。進度以位元組數表示，在得知總量前會將 `Total` 設為 -1。成功時會以不可分割的方式取代目的地檔案。`task.Pause()` 會保留部分檔案，當伺服器提供驗證器時 `task.Resume()` 會使用 HTTP 範圍請求，而 `task.Cancel()` 會移除部分檔案。

iOS 使用系統的背景 URLSession，在應用程式暫停時仍會繼續傳輸。應用程式會在啟動時重新連接其工作階段，並在確認背景完成前發布檔案。macOS 的傳輸在行程內執行。Apple 平台對暫停的下載使用原生的續傳資料，並將請求中繼資料保存在應用程式的私有儲存空間，直到最終結果儲存為止。無法續傳的伺服器會讓傳輸從頭開始。

在應用程式啟動時再次註冊 `OnDownloadUpdate`，即可看到進行中的工作與已儲存的結果。處理完最終結果後使用 `p.Task.Forget()`。Android 可能會停止服務，例如其前景傳輸時間到期時。應用程式隨後會收到 `DownloadInterrupted`。在 Windows、Linux、tvOS、watchOS 與網頁上，`Download` 會回傳 `ErrDownloadUnavailable`。

## 語音辨識與生命週期

`cx.StartSpeech` 會在 `cx.Env().CanRecognizeSpeech()` 表示可用的地方（macOS、iOS、Android、Windows，以及提供 `SpeechRecognition` 或 `webkitSpeechRecognition` 的瀏覽器）使用系統的辨識器辨識語音，並隨著轉錄文字變化而回應。在 Apple 平台上請在 `apple.info` 中宣告 `NSMicrophoneUsageDescription` 與 `NSSpeechRecognitionUsageDescription`；在 Android 上請將 `RECORD_AUDIO` 加入 `android.permissions`。在無頭測試中，辨識進行期間 `app.Speech` 會持有一個 `nsgotest.SpeechSession`。使用 `app.HearSpeech` 與 `app.EndSpeech` 傳遞結果。

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` 會說明應用程式處於作用中、非作用中或背景狀態：在桌面平台上指的是 `cx` 所在的視窗，因此最小化視窗中的內容可以暫停，而另一個視窗仍在顯示；在應用程式自己的作用域中則是指其所有視窗整體。`desktop.Minimized(cx)` 會說明該視窗是否已最小化，而 `desktop.FrontmostApp(cx)` 會說明桌面上位於最前方的是哪個應用程式（包含應用程式本身），以及其視窗是否處於全螢幕，就像桌面寵物或時間追蹤器會追蹤它一樣；`desktop.CanReadFrontmostApp(cx)` 會說明是否能追蹤它，支援 macOS、Windows、X11 與支援的 Wayland 合成器。`cx.OnQuit` 會在應用程式結束時進行清理，無論以何種方式結束，並給予其工作 200 ms。`desktop.OnQuitRequest` 可以取消或延後桌面的「結束」選單項目或 `desktop.RequestQuit(cx)`。在 Windows 與 Linux 上，當關閉視窗會結束應用程式時，它也會在 `OnClosing` 之後執行。`desktop.RequestRestart(cx)` 會在重新啟動前詢問相同的處理常式；`desktop.Restart(cx)` 仍為立即執行。Linux 與 Windows 上的更新程式重新啟動也會詢問，因此儲存作業可以取消或延後它們。當系統回報登出、重新開機或關機時，`q.Forced()` 為 true，應用程式會繼續執行 `OnQuit`。在詢問使用者做決定前，請先檢查 `Forced()`。`cx.SetAppearance` 會強制使用淺色或深色，`cx.SetLocale` 選擇語言，`cx.OnKey` 繫結鍵盤快捷鍵——當其擁有者顯示的內容被隱藏或位於對話框之下時，該快捷鍵會暫停作用，除非 `core.KeyOptions{OverModals: true}` 讓它保持作用；它會在應用程式選單之前取得按鍵，除非 `YieldsToMenu` 將按鍵讓給焦點所在處、採用相同快捷鍵的已啟用選單項目（當文字欄位或網頁檢視擁有焦點時，`MenuNotEditing` 項目會把按鍵留給該繫結，並讓後續具有相同快捷鍵的項目拿不到該按鍵；而 Linux 的選單不接受未按住 Control、Alt 或標誌鍵的按鍵，因此在那裡繫結會保有這類按鍵）；它在 `cx` 所在的視窗中作用，除非 `AllSurfaces` 將它繫結到應用程式的每個視窗與面板（包括之後開啟的），作為應用程式全域快捷鍵，此時當其擁有者在繫結所在的視窗中顯示的內容被隱藏時，它會在所有視窗中暫停作用；它還會將焦點控制項使用的按鍵留給該控制項——按鈕的 Return 與空白鍵，以及滑桿、分頁列或單選群組的方向鍵——除非以 `core.KeyAlways` 或 `core.KeyNotEditing` 繫結；`cx.OnOpenFiles` 會接收在 macOS、Windows、Linux 與 iOS 上開啟的文件。iOS 會將一般檔案匯入應用程式擁有的暫存空間；若要保留它們，請將它們移至 `cx.DataDir()`。在 macOS 上，關閉提供應用程式選單的視窗時，會還原最近一個仍存在之視窗的選單，若已無視窗則還原為預設選單。

在 Wayland 上，追蹤最前方應用程式需要合成器的 wlr foreign-toplevel 協定第 2 版或更新版本；其他合成器會回報 `CanReadFrontmostApp` 為 false。其 ID 為 Wayland 應用程式 ID，名稱在可取得時來自桌面項目（desktop entry），而 PID 為 0。封裝的 Windows 應用程式使用其 AUMID，而非 frame host 路徑。macOS 遵循 AppKit 的全螢幕呈現狀態，而非依視窗大小推斷。

應用程式選單中的編輯角色會在其 `OnSelect` 後備處理之前，先交由具有焦點的編輯器處理。在 macOS 上，「還原」與「重做」會依循該編輯器的歷史紀錄，而「剪下」會依循其選取範圍，即使應用程式處於非作用中也一樣；沒有編輯器時，後備處理會保持啟用。

在 iOS 上，給 `cx.OnKeyWith` 一個 `core.KeyOptions.Title` 函式，即可在繫結能夠觸發時於系統的鍵盤選單中提供該快捷鍵。`Group` 會在 iPadOS 26 及更新版本中為其選單命名。這些函式可以讀取應用程式的目錄，讓快捷鍵標題跟隨語言變化。在測試中使用 `app.KeyCommands()` 檢查目前的清單。

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` 是使用者為裝置取的名稱，而 `cx.Env().DeviceModel()` 是其硬體型號，例如 `Mac15,7`、`iPhone15,2` 或 `Google Pixel 8`，讓應用程式能分辨兩台已配對的裝置。macOS、Android、Windows 與 Linux 兩者皆提供；iOS 提供型號，而名稱只提供給擁有 Apple 使用者指定裝置名稱權利的應用程式。watchOS 提供其硬體識別碼，HarmonyOS 提供其市場名稱或產品型號，小程式則提供平台的品牌與型號；這些目標會將使用者指定的名稱留空。網頁兩者皆不提供。測試可透過 `nsgotest.Environment` 設定它們。

`cx.Env().ThermalState()` 在 Apple 平台與 Android API 29 或更新版本上會回報 `core.ThermalNominal`、`ThermalFair`、`ThermalSerious` 或 `ThermalCritical`。`cx.Env().LowPowerMode()` 在 Apple 平台、Android、Windows 以及具備 power-profiles-daemon 的 Linux 工作階段上會回報 `core.LowPowerOn` 或 `LowPowerOff`。在系統未提供數值的地方，讀取會回傳 `ThermalUnknown` 與 `LowPowerUnknown`。每項讀取都會各自追蹤變更，讓應用程式能在執行時調整其工作量。在測試中，透過 `nsgotest.Environment` 或 `app.SetEnvironment` 在 `core.Environment` 上設定 `ThermalState` 與 `LowPowerMode`。

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` 在 macOS、Windows 與 X11 上可讀取指標位置，即使指標位於應用程式視窗之外也可以。每次呼叫取一次樣本。該點使用與 `desktop.Monitors(cx)` 和 `Window.ShowAt` 相同的螢幕座標。在 Wayland 與其他不支援的目標上，或作用域缺少 `core.AccessCursorPoint` 時，`available` 為 false。

`desktop.SetWindowConstraints` 會限制內容大小，並控制調整大小、最小化與最大化；開啟視窗時設定 `WindowOptions.Constraints`，或對主視窗使用 `nsgo.WindowConstraints`。Linux 在 X11 上會套用非固定的最大值，並在 X11 與 Wayland 上從 GTK 原生標題列移除被禁止的最小化／最大化按鈕。Wayland 目前僅支援以固定大小作為上限。

`desktop.SetWindowLevel` 會讓視窗浮在其他視窗之上，而 `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` 或 `WindowOptions.Workspaces` 會讓它保持在每個工作區中，就像桌面寵物、HUD 或計時器那樣：在 macOS 上會出現在每個 Space 中，並位於其他應用程式的全螢幕空間之上，且不納入 Cmd-` 的切換；在 Linux 上則會出現在 X11 視窗管理員的每個虛擬桌面中。Windows 與 Wayland 會讓它留在開啟時所在的工作區，而 `desktop.CanSetWindowWorkspaces` 會說明它是否生效：在 macOS 上以及在支援 `_NET_WM_DESKTOP` 的 X11 視窗管理員下為 true。主視窗可藉由從其內容中呼叫 `desktop.SetWindowWorkspaces` 而出現在每個工作區。在測試中，`nsgotest.App.Window` 會列出這些請求，而 `nsgotest.Environment` 可設定 `CanSetWindowWorkspaces`。

`desktop.RegisterHotKey` 會繫結全域快捷鍵，無論哪個應用程式位於最前方都能作用，支援 macOS、Linux，以及以 `nsgo.Run(App)` 執行的 Windows 應用程式。其控制代碼的 `State` 會說明系統是否接受：在系統回應前為 `HotKeyPending`，之後為 `HotKeyRegistered`；當另一個註冊已佔用該組合鍵時為 `HotKeyTaken`；對於不支援的按鍵或沒有全域快捷鍵的平台則為 `HotKeyUnsupported`，讓快捷鍵錄製器能要求使用者換一個組合鍵。在 macOS 上，只有當應用程式本身已持有該組合鍵，或另一個應用程式以獨佔方式註冊了它時，才算被佔用。在 Wayland 上，合成器會請使用者繫結該快捷鍵：使用者繫結後即為已註冊，若使用者或合成器略過它則為已佔用。在測試中，`nsgotest.HotKeyTaken` 與 `nsgotest.HotKeyUnsupported` 可讓組合鍵產生對應的回應。

使用 `desktop.TitleBarHidden` 時，Windows 與 Linux 會在單獨按下並放開 Alt 或按下 F10 時，以彈出選單的形式提供應用程式選單。Alt 搭配頂層標籤的第一個字母或數字（依標籤所用的文字系統）會開啟該子選單或選擇該命令。應用程式的繫結會保有其按鍵；Shift+F10 仍是內容選單鍵，其他 Alt 組合鍵則保留給系統。Escape 會關閉彈出選單並還原鍵盤焦點。

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

在 macOS、Windows 與 Linux 上調整桌面視窗大小時，會在繪製前最多等待 16 ms 以取得新的版面配置。請讓反應式計算與事件處理常式保持快速，好讓版面配置趕上這個期限；將緩慢的 I/O 放在 goroutine 上執行，並透過 `cx.Post` 傳回結果。忙碌中的應用程式會繼續調整大小，並在趕上進度時顯示其版面配置。

當 `WindowFrame.Move` 為 true 時，`desktop.SetWindowFrame` 會移動視窗，使用以主螢幕左上角為原點、以點為單位的 `X` 與 `Y`。未設定的尺寸會維持目前大小。請檢查 `desktop.CanPlaceWindows`：macOS、Windows 與 X11 上的 Linux 支援定位；一般的 Wayland 頂層視窗會保留合成器所決定的位置。`desktop.Frame` 在這些桌面平台上會追蹤原生視窗的位置。

在 macOS 與 Windows 上，`WindowFrame{Animated: true}` 會讓可見的面板平滑移動到新的框架，包括停靠面板的大小。隱藏的面板與減少動態效果時會立即套用新框架。一般的 macOS 視窗使用 AppKit 的動畫。

將 `desktop.WindowOptions.Parent` 設為應用程式中一個已開啟的視窗，即可在父視窗關閉時一併關閉兩者。在桌面平台上，子視窗會保持在父視窗之上，並在父視窗隱藏或最小化時跟隨；被明確隱藏的子視窗在父視窗恢復時仍保持隱藏。在 macOS 上，當未設定 `Frame` 時，它也會置中於父視窗上方，並跟隨父視窗進入全螢幕。

`WindowOptions.NoShadow` 與 `desktop.SetWindowShadow(cx, false)` 也會移除隱藏標題列（自行繪製外框）的 Windows 視窗的陰影；使用系統標題列的視窗會保留其外框與陰影，由視窗管理員裝飾的 Linux 視窗也是如此，在那裡 `NoShadow` 只會清除 GTK 自身裝飾的陰影。

`desktop.SetWindowPointerEvents` 會讓指標穿過視窗，到達其下方任何應用程式的視窗：`WindowPointerNone` 會讓整個視窗都穿透，就像抬頭顯示器那樣；`WindowPointerPainted` 則讓沒有檢視繪製內容或接收指標的地方穿透，就像桌面寵物周圍一樣，而寵物本身仍會接收按壓、拖曳與右鍵點擊；系統繪製的標題列視為未繪製任何內容。`WindowOptions.PointerEvents` 與 `PanelOptions.PointerEvents` 會以此方式開啟視窗。`Desktop.WindowDrag` 區域即使位於可按壓的祖先內，仍能拖曳視窗；較近的可按壓或編輯控制項則會保有自己的輸入，而不會拖曳視窗。不接收指標的視窗不會收到任何按壓，因此應用程式需透過快捷鍵、系統匣圖示或另一個視窗將它恢復。由 `Access` 持有的程式碼需要 `core.AccessOverlay`。

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

在 macOS 與 Windows 上，繪製視窗中的圖片可將 `Image.HitTestAlpha`（TSX 為 `hitTestAlpha`）設為大於 0 且不超過 1 的值。例如，0.5 會讓至少半不透明的圖片像素接收指標。此門檻值也適用於圖片的 `OnPress`。檢視的背景或邊框仍算作已繪製。零表示使用整個圖片檢視，Linux 上的圖片也是如此。

在 macOS 的繪製視窗中，`Video.HitTestAlpha`（TSX 為 `hitTestAlpha`）會對解碼後的影片影格套用相同的門檻值。它在 `Controls` 關閉且影片軌未經變形時有效，例如作為桌面寵物、帶有 alpha 的 HEVC。零、無法取得影格的情況以及其他桌面主機，則使用影片檢視的邊界。

`desktop.NewTray` 在 macOS 與 Windows 上可透過 `TrayOptions.View` 繪製即時圖示。Windows 會將檢視的透明呈現結果調整為通知區域的圖示大小，並在檢視變更或圖片載入完成時更新；其動作請使用 `TrayOptions.OnClick`、`Panel` 與 `Menu`。Linux 目前顯示 PNG 圖示；繪製其系統匣檢視仍是待實現的目標。

`desktop.OpenPanel` 會開啟這類快捷鍵或系統匣圖示所顯示的面板：一個位於其他視窗之上的無邊框視窗。如同 `desktop.OpenWindow`、`desktop.NewTray`、`mobile.OpenOverlay`、`audio.NewPlayer`、`audio.LoadSound` 與 `core.NewHolder`，它直接屬於你傳入的 `cx`，而非正在執行之處理常式的擁有者，因此從按鈕的 `OnPress` 傳入應用程式的作用域，可讓視窗、面板或播放器在該按鈕的作用域消失後繼續存在；反應式基本元件（`Effect`、`Cleanup`、`AfterFunc`、`OnKey`）則屬於正在執行的擁有者，而 `cx.Run` 會切換該擁有者。彈出卡片會在設定了 `Transparent` 的面板上繪製自己，該面板沒有自己的背景、外框或陰影，因此只會顯示卡片的圓角與 `BoxShadow`，並在面板內為陰影保留空間；`FitContent` 會在面板首次顯示前及內容變更時，將面板大小調整為其內容大小，上限為 `Width` 與 `Height`。`desktop.Transparent` 會說明面板目前是否沒有繪製任何自己的內容，就像透明視窗一樣，而 `desktop.CanMakeWindowsTransparent` 只說明視窗是否可以透明：在其為 false 的地方（例如沒有合成器的 Linux），請為內容加上背景。

在 macOS 上，未設定 `Transparent` 的面板在任一尺寸小於 24 點時會使用直角，並在正常大小時恢復圓角。

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` 在 macOS 上會於其圖片或繪製的檢視旁顯示 `TrayOptions.Title`。若要自訂彈出視窗，請在 `OnClick` 或 `Menu` 中讀取 `tray.ClickBounds()`，並將可用的矩形傳給 `panel.ShowAt(bounds, core.PlaceBottom)`。它使用與 `Env.Displays` 相同的螢幕點座標，並包含標題。在首次點擊之前、移除之後、在 Linux 上，以及 Windows shell 無法提供時，邊界皆無法取得。每次點擊都會取代先前的邊界，因此無法取得的結果絕不會重複使用舊的矩形。設定 `TrayOptions.Panel` 則會保留內建的點擊切換行為。

`PanelOptions.Dock` 會透過 `Show`、`Focus`、`Toggle` 與內容大小調整，讓面板沿著顯示器可用邊緣保持置中。將 `Edge` 設為 `core.PlaceTop`、`PlaceBottom`、`PlaceLeft` 或 `PlaceRight`，將 `Display` 設為 `Env.Displays` 的 ID（留空表示主要顯示器），並將 `Margin` 設為與該邊緣的距離（以點為單位）。停靠的面板無法使用 `ShowAt` 或 `MoveTo`；`SetWindowFrame` 只會變更其大小。系統匣圖示會在停靠位置顯示它，而不是在圖示下方。

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` 會與任意視窗定位分開回報停靠支援。macOS、Windows 與 X11 支援停靠；Wayland 需要可選的執行階段函式庫 `libgtk4-layer-shell.so.0`，以及實作 `zwlr_layer_shell_v1` 的合成器。NSGo 會在 GTK 之前載入該函式庫；已經載入 Wayland 的嵌入主機必須在啟動主機前預先載入 gtk4-layer-shell。缺少函式庫、協定或所選的顯示器時，面板會保持隱藏並呼叫 `OnPlacementError`。Wayland 停靠使用頂層（top layer），且不保留工作區空間：`NoActivate` 會讓鍵盤留在目前的視窗，而沒有設定它的面板會在隱藏前取得獨佔的鍵盤焦點。一般的 Wayland 視窗則保留合成器平常的定位方式。

快捷鍵錄製器讓使用者選擇這樣的按鍵。`Focusable` 檢視的 `OnKeyDown` 會在其擁有焦點期間、早於應用程式的繫結接收每個按下的按鍵，並附帶所按住的修飾鍵，包括單獨按下的修飾鍵；回傳 `true` 會讓該按鍵不傳給繫結，因此錄製 Command+S 不會觸發儲存。`KeyEvent.Shortcut` 會以 `OnKey` 與 `RegisterHotKey` 接受的格式寫出該次按壓，每個修飾鍵以名稱表示（"ctrl+shift+k"、"alt+space"，Command+Shift+K 則為 "shift+meta+k"），單獨的修飾鍵則為 ""。其 `OnKeyUp` 會接收這些按鍵放開的事件，即使 `OnKeyDown` 已攔截它或焦點已移開也一樣，並附帶仍按住的修飾鍵。兩者皆可在 macOS、Windows、Linux、網頁（瀏覽器會保留它不交給頁面的快捷鍵）、Apple TV 與 `nsgotest` 中運作；Android 會以非同步方式將實體鍵盤事件傳給具有焦點的檢視，因此其 `OnKeyDown` 的回傳值無法攔截按鍵。iPhone 與 iPad 支援輪詢實體按鍵。應用程式以全域方式註冊的快捷鍵會比檢視先取得其按鍵，因此錄製器在錄製期間應取消註冊它。

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

遊戲或畫布則改為輪詢按鍵。`core.KeysDown(cx)` 會列出視窗上按住的按鍵，而 `core.KeyHeld(cx, "left")` 會說明某個按鍵是否按住：這些是會被追蹤的讀取，`RequestAnimationFrame` 迴圈也會讀取它們。第一次讀取會讓平台從那時起回報每個按鍵，因此已經按住的按鍵在再次按下前會維持未知；在欄位或網頁檢視中輸入的按鍵，或在輸入法組字期間的按鍵，絕不會被視為按住，而在視窗失去焦點時仍按住的按鍵會在那時被視為放開。它們在 `OnKeyUp` 可運作的地方都能運作，也能在連接實體鍵盤的 iPhone 與 iPad 上運作。

對於滑鼠按鈕，`View.OnMouseDown` 與 `View.OnMouseUp` 會在 macOS、Windows、Linux、網頁與 `nsgotest` 上，為每個按鈕（`core.MouseButtonLeft`、`core.MouseButtonRight`、`core.MouseButtonMiddle`、`core.MouseButtonBack`、`core.MouseButtonForward`）傳遞 `core.MouseButtonEvent`，且不會讓該檢視成為鍵盤焦點停駐點。`core.MouseButtonsDown(cx)` 與 `core.MouseButtonHeld(cx, core.MouseButtonRight)` 是對視窗上按住之按鈕的追蹤讀取，並會在視窗失去焦點時自動清除。

對於與實體位置相關的控制，請讀取 `KeyEvent.Code`、`core.KeyCodesDown(cx)` 或 `core.KeyCodeHeld(cx, "KeyW")`。代碼使用 `KeyboardEvent.code` 的名稱：在 AZERTY 上，QWERTY 的 W 位置為 `Code: "KeyW"` 與 `Key: "z"`。`core.KeyCodeLabel(cx, "KeyW")` 會提供目前鍵盤配置在提示中顯示的標籤。在 iOS、Android、HarmonyOS 以及沒有配置對應表的瀏覽器上，它會使用該位置上次回報的按鍵名稱，在看到按鍵之前回傳空字串。由於 ArkUI 不回報掃描碼，HarmonyOS 會以 InputKit 的 `KeyCode` 同時命名位置與按鍵，因此在那裡按鍵名稱尚不會跟隨非 QWERTY 的鍵盤配置。

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## 保持螢幕喚醒

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` 會讓螢幕保持開啟，直到回傳的釋放函式執行或目前的作用域被釋放為止。在 Android 上，此保持僅在應用程式可見時生效。在網頁上，它在具備 Screen Wake Lock 支援的安全環境中運作，且頁面在重新可見時會再次請求鎖定。Safari 可能要求在核准第一次請求前先有一次按壓或按鍵。僅針對電腦的保持在這兩個平台上沒有效果。

## 指標鎖定

在遊戲攝影機，或需要在螢幕邊緣繼續的拖曳中，從持有該 ref 之檢視上的按壓呼叫 `ref.LockPointer()`。指標會停留在原處並隱藏，而該檢視會在 `OnPointerDelta(dx, dy)` 中接收相對移動量。正值表示向右與向下移動。

呼叫回傳的函式即可釋放鎖定。按下 Escape、視窗失去焦點，或隱藏、移除該檢視也會釋放鎖定。請檢查 `cx.Env().CanLockPointer()`，以確認該繪圖表面是否支援。網頁需要使用者手勢，例如一次按壓。Wayland 上的 Linux 需要支援指標約束與相對移動的合成器。

## 音效限制

在載入音效前使用 `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` 變更引擎的限制。也可以設定 `MaxEncodedBytes`、`MaxDecodedBytes` 與 `MaxPlaybacks`。零值欄位會使用預設值：每個音效編碼後 8 MiB 與 5 秒、64 個來源、所有音效合計解碼後 16 MiB，以及每個輸出 32 個重疊播放。既有的音效會保持載入，並計入新的限制。受限的作用域會在引擎預算內保留預設限制：已簽署 TSX 套件的 `nsgo/audio` 音效無論 Go 應用程式設定為何都會受此限制，而應用程式自己的 TSX 則採用 Go 應用程式的限制。無論限制為何，Android 的 SoundPool 最多同時播放 32 個音效，且每個音效最多只解碼 1 MiB。

## 全螢幕

`desktop.SetFullScreen(cx, true, displayID)` 在 macOS、Windows 與 Linux 上會填滿由 `cx.Env().Displays()` 指定的顯示器。省略 ID 則使用視窗目前所在的顯示器。未知的 ID 會讓視窗維持原位。重複相同的請求不會有任何效果。`desktop.SetFullScreen(cx, false)` 會離開全螢幕並還原視窗先前的框架。在網頁上，請從按壓處理常式進入全螢幕。由瀏覽器選擇顯示器，而 `cx.Env().FullScreen()` 會追蹤進入與離開，包括按下 Esc。

## 顯示器

`cx.Env().Display()` 會回傳視窗目前的 `core.DisplayInfo`，並在視窗移到另一個顯示器時更新繫結。`cx.Env().Displays()` 會列出平台回報的顯示器。`RefreshRate` 與 `MaxRefreshRate` 以 Hz 為單位，無法取得時為零。iOS 回報面板的最大值。HarmonyOS 回報視窗所在的顯示器、其目前的更新率與其支援的最高更新率。網頁回報目前的螢幕，並以 `RefreshRateEstimated` 標記其影格回呼的量測值。在測試中，透過 `nsgotest.Environment` 設定 `Displays` 與 `DisplayID`。

## 文字朗讀

`cx.Speak` 會在 `cx.Env().CanSpeak()` 表示平台支援朗讀的地方，以系統的語音用某種語言大聲讀出文字：macOS、iOS、tvOS、Android、Windows、網頁，以及具備 speech-dispatcher 的 Linux。一次只朗讀一段話語：另一個 `Speak` 會中斷正在朗讀的那段，`cx.StopSpeaking` 以及發出朗讀之作用域的結束也會如此。`done` 會收到一次結束方式的通知：完成、停止，或以 `core.SpeakError` 失敗，例如沒有已安裝的語音能說該語言時的 `core.SpeakLanguageUnavailable`。

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` 與 `Pitch` 的範圍為 0.5 到 2，1 為該語音本身的值。`cx.Voices("ja", ...)` 會列出為某種語言安裝的語音，讓應用程式能請使用者安裝缺少的語音，或讓他們挑選一個，並將其 `ID` 放入 `core.Voice`。瀏覽器只有在使用者於頁面上按過某個東西後才會朗讀，在此之前會以 `core.SpeakNotAllowed` 失敗。TSX 套件的 Go 元件只有在具備 `text_to_speech` 能力時才能朗讀。

## 循環播放音訊

設定 `audio.Voice.Loop` 可重複播放音效，直到 `Stop` 或 `Close`。對於串流音樂，請設定 `audio.PlayerOptions.LoopStart` 並呼叫 `SetLoop(true)`。第一輪包含前奏。之後的每一輪從該偏移位置開始。偏移為零時會重複整首曲目。

在 macOS 14 與 iOS 17 或更新版本上，有限長度的曲目會使用原生佇列，在目前這一輪結束前準備好下一次重複。較早的 Apple 版本會在目前這一輪結束時跳轉到循環起點，中間會有短暫的間隙。在 Linux 與網頁上，循環播放器會將最多 32 MiB、5 分鐘的曲目解碼至記憶體中（約每分鐘 23 MB），並無間隙地重複播放。在 Linux 上，更大或更長的曲目會透過其自己的解碼器串流播放，同樣無間隙地重複；在網頁上則會在結尾時跳回，中間有短暫的間隙。在 Windows 與 Android 上，設有起點的循環會在結尾時跳回該起點，中間有短暫的間隙。在網頁上，速率不為 1 的循環播放器也會在結尾時跳回，因為瀏覽器的無間隙播放會讓音高隨速率改變。在網頁上，循環播放器與音效一樣，只有在使用者於頁面上按過某個東西後才會播放，在此之前會以 `NotAllowed` 失敗。

## 生物辨識

`cx.BiometricAvailability` 會回報是否已登錄 Face ID、Touch ID 或裝置指紋；Android 不會說明登錄的是哪一種，因此在那裡當有登錄指紋時會回報指紋，否則當裝置具備臉部辨識硬體時會回報臉部。`cx.AuthenticateBiometric` 會以一個不可為空的理由請系統確認使用者身分，並回報成功、失敗、取消或無法使用。它不會替應用程式選擇密碼後備方案。

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

使用 Face ID 的應用程式需在 `apple.info` 下宣告 `NSFaceIDUsageDescription`；Android 應用程式需在 `android.permissions` 下列出 `USE_BIOMETRIC`。

## 登入

`cx.AuthSession` 會在系統的驗證工作階段中，而非網頁檢視中，讓使用者透過 OAuth 提供者登入：在 macOS、iOS 與 watchOS 上使用 `ASWebAuthenticationSession`，在 Android 上使用 Custom Tab 或瀏覽器，在 Windows、Linux 以及未設定 scheme 的 Mac 上使用預設瀏覽器，在網頁上則使用彈出視窗。在網頁上，重新導向頁面也會透過同源的 `BroadcastChannel` 送出其回呼，因此即使提供者的 `Cross-Origin-Opener-Policy` 切斷了彈出視窗與頁面的關聯，登入仍能完成。這樣的彈出視窗對頁面而言會被視為已關閉，因此在彈出視窗顯示過提供者的頁面後將其關閉，工作階段要到 5 分鐘後才會以取消結束；請為工作階段提供取消按鈕。協定由應用程式自己維護：它產生 PKCE 驗證碼與 state，為所提供的重新導向建立授權 URL，並在交換授權碼前檢查回呼。取消並不是失敗：`errors.Is(err, core.AuthCancelled)` 表示使用者關閉了工作階段，或回傳的 `cancel` 結束了它。

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

設定了 scheme 時，重新導向為 `com.example.app:/oauth2redirect`（Windows、Linux 與 Android 需要在 `nsgo.yaml` 的 `url_schemes` 中加入它，作業系統才會將回呼交給執行中的應用程式；macOS、iOS 與 watchOS 的系統工作階段無需該宣告即可攔截），而在桌面平台的迴路位址上則為 `http://127.0.0.1:<port>/oauth2redirect`，因此請向提供者註冊兩者，通常是各自作為獨立的用戶端。若只使用迴路位址，請將 `Scheme` 留空；或將其設為 `https` 並搭配 `Host` 使用應用程式的關聯網域，iOS 17.4、macOS 14.4 與 watchOS 10.4 可攔截此類回呼。watchOS 需要 scheme 或關聯的 HTTPS 網域；它沒有迴路位址後備方案，且 NSGo 既有的 watchOS 26 最低版本要求仍然適用。關閉的瀏覽器分頁不會通知迴路位址工作階段，因此請為它設定 `Timeout` 或提供取消按鈕。`Ephemeral` 會要求 macOS、iOS 與 watchOS 不共用瀏覽器的 cookie，並在 Android 上於瀏覽器支援時請求私密的 Custom Tab；請勿假設 Android 上的 cookie 是隔離的。tvOS 與小程式目前會回應 `core.AuthUnavailable`。在 TSX 中，`AuthSession.start(url, options)` 需要 `auth_session` 能力，並提供一個工作階段，其 `result` 會以回呼解析，取消後則為 null。

Android 會在一般的 `OnOpenLinks` 之前取用相符的回呼，而在沒有回呼的情況下返回會讓工作階段以取消結束。其回呼必須使用應用程式擁有的私有 scheme；空的 scheme、HTTPS 回呼或未宣告的 scheme 會回傳 `core.AuthUnavailable`，在登入工作階段功能推出前建置的 Android 執行階段也是如此。

瀏覽器工作階段只會取用與其相符的回呼連結；不相關與較晚抵達的連結仍會送到 `OnOpenLinks`。為同一個回呼 URI 啟動另一個瀏覽器工作階段會取消較早的那個；不同的 URI 可以在不同視窗中等待。

請使用傳給授權 URL 函式的確切重新導向。網頁與桌面迴路位址的重新導向會在函式收到之前，先將 Unicode 路徑編碼並解析點區段；迴路位址的 `Path` 是解碼後的文字，因此字面上的 `%` 也會被跳脫。

小程式則改用 `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)`。回呼函式會收到一個 `core.MiniProgramCredential`，其中包含 `Platform`（`MiniProgramWeChat`、`MiniProgramQQ`、`MiniProgramDouyin` 或 `MiniProgramAlipay`）以及一個非空、短效的 `Code`。請將它傳送到你的伺服器，以進行宿主平台的授權碼交換；應用程式密鑰請保存在伺服器上。這對應 `wx.login`／`qq.login`／`tt.login`，或支付寶的基本 `auth_base` 授權，不會請求個人資料。它不會開啟 URL，也不會執行 OAuth URL 建構函式。

`cx.Env().CanSignInMiniProgram()` 會回報宿主 API 是否可用。其他目標會回應 `core.MiniProgramSignInUnavailable`；沒有 `core.AccessMiniProgramSignIn` 的程式碼會得到 `AccessDenied`。抖音預設會以靜默方式請求：未登入的宿主會回傳 `MiniProgramSignInNotSignedIn`，而 `MiniProgramSignInOptions{Prompt: true}` 允許顯示其宿主帳號登入視窗。關閉該視窗會回傳 `MiniProgramSignInCancelled`。匿名的裝置代碼絕不算作已登入的使用者。測試可設定 `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` 與 `app.MiniProgramSignIn`；`app.MiniProgramSignIns` 會記錄請求。TSX 不會在小程式中執行。

若應用程式的網頁檢視應以使用者在其他地方（例如在他們自己的瀏覽器中）已登入的身分登入，可以使用 `cx.SetWebCookies` 寫入該登入的 cookie，包括頁面腳本無法設定的 HttpOnly cookie，然後再載入頁面：done 會在存放區保存這些 cookie 後執行一次。該存放區是應用程式所有網頁檢視共用的永久存放區，因此帶有 `Expires` 的 cookie 會比應用程式存續得更久；無痕檢視看不到它們，但 Android 除外，因為它為每個應用程式只保留一個存放區。HarmonyOS、網頁、tvOS 與 watchOS 會回應 `Unavailable`。

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

若要移除一個 cookie，請以相同的名稱、網域與路徑再次寫入它，並將 `Expires` 設為過去的時間；到期時間為零則會產生工作階段 cookie。在載入應觀察到該移除結果的頁面之前，請等待回呼。

反過來，`cx.WebCookies` 會列出存放區中的 cookie，包括 `document.cookie` 無法讀取的 HttpOnly cookie，以及它們的網域、路徑、到期時間與旗標，無論該網站的頁面是否正在顯示：`core.WebCookieFilter` 可挑選對某個 `URL` 發出請求時會送出的 cookie，或某個 `Domain` 及其下所有主機的 cookie，零值篩選器則會挑選全部。以使用者的登入身分從 Go 呼叫網站，或顯示使用者已登入哪些網站的應用程式，就是以這種方式讀取它們。`cx.ClearWebsiteData` 會讓使用者登出所有網站：它會移除存放區的 cookie、本機與工作階段儲存空間、IndexedDB、service worker 與快取，並在它們全部清除後執行一次 done。Android 的 `CookieManager` 只會回應某個 URL 會送出的名稱與值，因此在那裡只有 `URL` 篩選器能列出 cookie，並以其主機填入 `Domain`、其他欄位留空，而 `Domain` 篩選器或零值篩選器會回應 `Unavailable`；在那裡清除時會保留 HTTP 快取。HarmonyOS、網頁、tvOS 與 watchOS 對兩者皆回應 `Unavailable`。

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## 使用 Apple 登入

`cx.SignInWithApple` 會在 macOS、iOS 與 tvOS 上，於 `cx.Env().CanSignInWithApple()` 為 true 時，從應用程式自己的按鈕顯示系統的「使用 Apple 登入」表單。它會回應一次，提供認證資料或錯誤：使用者關閉表單時為 `core.AppleSignInCancelled`，這並不算失敗；否則為 `AppleSignInFailed`，或在其他平台上為 `AppleSignInUnavailable`。Apple 只在使用者第一次登入該應用程式時提供電子郵件與姓名，因此請在那時保存它們。`Nonce` 會以其 SHA-256 的小寫十六進位形式放入請求中，並保存在身分權杖的 `nonce` 宣告裡：伺服器會對它提供給應用程式的 nonce 進行雜湊並比對。`cx.AppleCredentialState` 會告知使用者之後是否已撤銷它。在無頭測試中，`app.AppleSignIn` 會接收一個 `nsgotest.AppleSignInRequest`，並提供認證資料或錯誤。

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

應用程式必須以「使用 Apple 登入」權利簽署，當 `nsgo.yaml` 要求時，`build` 會加入它；沒有它，表單就會失敗：

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode 會為 `nsgo dev` 的裝置建置註冊此功能；`sign.profile` 或 `sign.macos_profile` 中的佈建描述檔必須來自已開啟「使用 Apple 登入」的 App ID。macOS 只在有描述檔時才會授予它，因此磁碟映像檔或在 Mac 上以 `nsgo dev` 執行的應用程式無法登入，模擬器也不行，因為其建置不宣告任何開發者權利。

## App 內購買

`cx.Products`、`cx.Purchase` 以及與之並列的呼叫，會在 macOS、iOS 與 tvOS 上透過 App Store 進行銷售，前提是 `cx.Env().CanPurchase()` 為 true，且建置的 `nsgo.yaml` 要求使用 StoreKit：

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

請顯示商店提供的價格 `DisplayPrice`，絕不要顯示應用程式自訂的價格：商店會自行為每個店面定價。購買會回應一次，提供交易或錯誤：使用者關閉表單時為 `core.PurchaseCancelled`；等待核准期間（例如「購買前詢問」）為 `core.PurchasePending`；否則為附帶商店說明文字的 `core.PurchaseFailed`，或在其他地方為 `PurchasesUnavailable`。請將交易的 `Signed`（商店的 JWS）傳送到應用程式的伺服器進行驗證，交付所購買的內容，然後完成該交易；商店會在每次啟動時將未完成的交易再次交給應用程式。

`cx.ManageSubscriptions` 會開啟 Apple 的訂閱管理介面（在 iOS 上為系統表單，在 macOS 與 tvOS 上為 App Store 的訂閱頁面）。`cx.RequestRefund` 會為一筆交易發起退款請求，並回應 `core.RefundSuccess` 或 `core.RefundCancelled`。

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

對於已完成的交易，包括 `CurrentEntitlements` 回傳的交易，`FinishTransaction` 會回傳 `PurchaseNotFound`；已完成的非消耗性項目仍歸使用者所有。

請在應用程式啟動時註冊 `OnPurchaseUpdate`：在任何處理常式出現之前抵達的更新會等待第一個處理常式。`cx.CurrentEntitlements` 會回應使用者目前擁有的項目，以便在啟動時解鎖它們，而「回復購買」按鈕會呼叫 `cx.SyncPurchases`，這可能會要求使用者登入。`AccountToken` 是應用程式自己的 UUID，會由交易與 App Store 的伺服器通知攜帶。macOS 只在 Mac App Store 建置中進行銷售，App Clip 則不銷售任何東西；Android、watchOS、Windows、Linux、網頁與小程式目前會回應 `PurchasesUnavailable`。請以 `nsgotest` 測試購買，其 `Products`、`PurchaseAnswer`、`UpdatePurchase` 與 `UnfinishedTransactions` 會代替商店。`app.PurchaseRequests` 會將商店操作記錄為 `nsgotest.PurchaseRequest` 值。在 TSX 中，`Purchases` 模組需要 `purchases` 能力：`purchase` 會以 `{ status: "purchased", transaction }`、`"pending"` 或 `"cancelled"` 解析，而 `on("update", listener)` 會接收更新。

## 介面方向

在全螢幕播放器的作用域中呼叫 `cx.RequestOrientation(core.OrientationLandscape)`，即可要求 iOS、Android、HarmonyOS 或瀏覽器切換為橫向。作用域仍存續的最新請求會生效。當該作用域結束時，會恢復先前的請求，或讓應用程式回到其平常的方向政策。

在 iOS 16 及更新版本上，此請求可以立即旋轉介面。在較舊的 iOS 版本上，它會限制下一次系統旋轉。已呈現之強制回應視窗的 `IOS.SupportedOrientations` 優先。在 Android 與 HarmonyOS 上，系統無法表示的直向與橫向方向組合會被縮減為一個允許的方向，並優先採用目前已顯示的方向。Android 在大螢幕或多視窗模式下可能會忽略方向請求。

在網頁應用程式中，請從開啟播放器作用域的按壓處理常式呼叫 `desktop.ToggleFullScreen(cx)`。瀏覽器會因該次按壓進入全螢幕，然後 NSGo 會套用播放器的方向請求。離開全螢幕會釋放鎖定。瀏覽器主控台會回報不支援的操作與被拒絕的請求。

`nsgotest.App.Orientation` 會記錄最新請求的遮罩，零表示平常的方向政策。

## 商店評分

`cx.RequestReview()` 會在 macOS 與 iOS 上向 App Store、在 Android 上向 Google Play 請求 App 內評分提示。`cx.Env().CanRequestReview()` 會回報該服務是否可用。是否以及何時顯示提示由商店決定，且不會回報任何結果。請使用開發建置來測試 Apple 的提示。TestFlight 建置會抑制它。

獨立的 Android 套件包含 Play Review SDK、其對話框活動與 Play 商店可見性查詢；建置它們不需要 Android SDK 或 JDK。沒有啟用 Play 商店的裝置會回報該服務無法使用。請使用[內部應用程式分享](https://developer.android.com/guide/playcore/in-app-review/test)測試已發佈的套件。

嵌入 NSGo 時，請將 `com.google.android.play:review:2.0.2` 加入 Gradle 主機。在主機的資訊清單中，於 `<queries>` 下加入 `<package android:name="com.android.vending" />`，讓應用程式能看到 Play 商店。若主機會縮減其程式碼，請以下列規則保留 Play Review 與 Tasks API，因為 NSGo 是依名稱載入它們的。

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

在測試中，請在 `nsgotest.Environment` 中設定 `CanRequestReview`，並檢查 `nsgotest.App.ReviewRequests`。

## 系統邊緣手勢

在螢幕邊緣具有觸控操作的遊戲，可在這些操作啟用時呼叫 `mobile.DeferSystemGestures`。使用回傳的函式可提早釋放請求。當其作用域被清理時，請求也會結束。同一視窗中來自多個作用域的請求會合併。

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS 使用實體螢幕邊緣。Android 10 及更新版本使用以繪圖表面點為單位、從左上角量測的矩形。請讓矩形只圍繞需要它們的操作控制項。Android 會保留其主畫面手勢，並限制應用程式可從「返回」手勢中排除的區域大小。`nsgotest.App.SystemGestures` 會提供測試其主要繪圖表面上目前合併後的請求。對於其他視窗，請使用 `Surface.SystemGestures()`。

## Android 上的覆蓋層

`mobile.OpenOverlay` 會在 Android 上將應用程式的一個檢視顯示在其他應用程式之上，這是一棵共用應用程式信號的獨立樹，並由前景服務讓應用程式在其顯示期間持續執行。應用程式需在 `android.permissions` 下列出 `SYSTEM_ALERT_WINDOW`，並由使用者在系統設定中授予，`mobile.RequestOverlayPermission` 會開啟該設定；沒有該權限時，覆蓋層會立即結束，而 `OnEnd` 會得知原因。它會接收觸控，但會將按鍵與其周圍的觸控留給位於前方的應用程式，除非它是 `Focusable` 或 `Passive`；它可從其內容透過 `desktop.SetWindowFrame` 移動。其他平台不會開啟任何覆蓋層。

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## 定位與動作

`cx.Locate` 取得一次定位，`cx.WatchLocation` 串流定位結果，而 `cx.WatchMotion` 串流加速度計、陀螺儀、磁力計與融合後的裝置動作樣本。每個呼叫都會回傳一個 `*core.SensorStream`，其 `Stop` 會取消它，啟動它的作用域結束時也會如此；回呼函式在引擎 goroutine 上執行。權限為 `core.PermissionLocation`、`PermissionBackgroundLocation` 與 `PermissionMotion`（[權限](permissions.md)）：啟動串流絕不會詢問使用者，而缺少權限的串流會以 `core.SensorNotAllowed` 失敗。

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate` 會等待新的定位結果，預設為 30 秒（`Timeout`）；只有 `MaxAge` 能讓它採用快取的結果，並以 `Cached` 與 `Age` 標明。`Approximate` 表示使用者只允許大略位置的定位結果；Android 最多每十分鐘才更新一次這類結果，因此在那裡請為 `Locate` 設定 `MaxAge`。座標使用 WGS 84；提供者缺少的量測值其 `Valid` 為 false，而 `Altitude` 為橢球高，`MSLAltitude` 則為平均海平面以上的高度。動作使用裝置的自然軸向，加速度單位為 m/s²（面朝上平放時 z 軸為 +9.81），旋轉單位為 rad/s，磁場單位為 µT，預設每 20 ms 一次，範圍限制在 10 ms 至 1 s。負載過重時，串流會為每個感測器保留最新的樣本，並在 `Dropped` 中計算被取代的樣本數。

串流在應用程式非作用中時會繼續，在背景時會暫停，並以 `Gap` 標記其後的第一個樣本；待處理的 `Locate` 會保留其剩餘的逾時時間。`LocationOptions.Background` 會在 `PermissionBackgroundLocation` 下讓定位串流在背景繼續進行：Apple 應用程式會加入 `location` 背景模式，而在 Android 上，前景服務會顯示帶有 `NotificationTitle` 與 `NotificationBody` 的通知，並在活動結束後讓應用程式繼續執行。系統終止的行程不會被任何機制重新啟動。

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS 與 watchOS 提供定位與動作，macOS 提供定位，tvOS 一次提供一個定位結果，而 Android 則透過框架本身的提供者提供兩者。Linux 從系統匯流排上的 GeoClue 2 取得定位結果與串流，其代理程式會在應用程式的第一個請求開始時詢問使用者；由於無法在不提示的情況下查詢 GeoClue，在應用程式於該次執行中詢問之前，權限會讀為尚未詢問，且背景定位無法使用。Windows 從 WinRT 的 `Geolocator` 取得它們；當應用程式的定位功能關閉時，權限讀為已拒絕；當應用程式在該次執行中詢問並獲准後，讀為已授予；否則讀為尚未詢問，而背景定位在那裡同樣無法使用。安全環境下的瀏覽器會提供地理定位結果，在行動裝置上還會提供加速度計、陀螺儀與融合動作讀數。在 Safari 上，請從按壓處理常式呼叫 `cx.RequestPermission(core.PermissionMotion, done)`。沒有 `navigator.geolocation` 時，瀏覽器會回報定位無法使用；在桌面平台上以及對於磁力計串流，則回報動作無法使用。在 TSX 中，`Sensors` 提供 `permission`、`requestPermission`、`locate`、`watchLocation` 與 `watchMotion`，受套件能力 `location`、`backgroundLocation` 與 `motion` 管控，時間以毫秒為單位。

## 音訊

`audio` 套件可在 `cx.Env().CanPlayAudio()` 為 true 的地方播放一次解碼至記憶體的短音效（`audio.LoadSound`）與串流播放的長音訊（`audio.NewPlayer`）；在其他地方，呼叫會以 `audio.Unavailable` 失敗。

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` 可為每次播放設定 `Volume`（0 到 1，0 視為 1）、`Rate`（0.5 到 2，0 視為 1）與 `Pan`（-1 為完全偏左，1 為完全偏右，0 為置中）；超出這些範圍的值會被限制在範圍內，而 NaN 會採用預設值。`Sound.Stop` 會停止該音效的所有播放，而 `Close` 會釋放它，擁有它的作用域在被釋放時也會如此。`Player` 支援 `Play`、`Preload`、`Pause`、`Seek`、`SetVolume`、`SetRate`、`SetLoop` 與 `Close`。目前 macOS、iOS、Windows、Linux、Android 與網頁都能播放音訊。Apple、Windows 與瀏覽器後端支援 WAV、AAC 與 MP3。Linux 使用 GStreamer 編解碼器，其串流播放器以 1× 速度播放。Android 會在播放器的檢視離開畫面時暫停播放器，並將每個解碼後的音效限制為 1 MiB。tvOS、watchOS、HarmonyOS 與小程式仍回報 `CanPlayAudio` 為 false。

瀏覽器要求使用者手勢才能發出聲音播放。請預先載入短音效，然後在點擊、輕點或按鍵之後呼叫 `Sound.Play`。在手勢之前播放的音效是無聲的。若瀏覽器封鎖播放，播放器的 `Play` 回呼函式會收到 `audio.NotAllowed`。請從使用者手勢中重試。

`Sound.Play` 會回傳一個 `audio.Playback`，在 macOS、iOS、Windows、Linux 與網頁上，它可以獨立設定自己的音量與速率或停止。`Ended()` 是反應式的，當該次播放完成、停止、因語音數上限被取代或無法開始時會變為 true。沒有個別控制的後端會回傳一個已結束的控制代碼。

在 iOS 上，函式庫自有、在 `audio` 與 `Video` 之外播放的音訊引擎（例如 `AudioQueue`），會在播放期間以 `audio.HoldSession(core.AudioSessionPlayback)` 持有應用程式唯一的音訊工作階段，並在停止時呼叫回傳的釋放函式。它採用與播放器的 `AudioSession` 相同的層級，與應用程式的影片、播放器與錄音並列，因此 `AudioSessionPlayback` 與 `AudioSessionMixWithOthers` 在響鈴／靜音切換開關設為靜音時仍會繼續播放。在其他地方，它不會持有任何東西。

使用 `audio.Voice{Muted: true}` 以靜音方式開始一個音效，使用 `playback.SetVolume(0)` 讓進行中的音效靜音。對於長音訊，`audio.NewPlayer` 會串流一個來源，而 `player.SetMuted(true)` 會在播放繼續的同時讓輸出靜音。取消靜音會還原播放器設定的音量。`Voice.Volume` 與 `Player.SetVolume` 以零表示預設的最大音量。

在 TSX 中，`nsgo/audio` 提供 `supported()` 與 `loadSound(uri)`，受 `audio` 套件能力管控。`loadSound` 會以一個音效解析，其 `play({ volume, rate, pan, loop, muted })` 會回傳一個具有 `setVolume`、`setRate`、`stop` 與反應式 `ended` 的播放；在那裡，音量 0 即為靜音。被拒絕的載入會以一個 `Error` 拒絕，其 `reason` 為音訊錯誤的名稱（例如 `quota-exceeded`），而其 `code` 在缺少能力時為 `ERR_ACCESS_DENIED`，超過限制時為 `ERR_NSH_QUOTA_EXCEEDED`，在無法播放音訊的地方則為 `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM`。音效只有在 `close()` 或其檢視消失時才會被釋放；在沙盒中，每次載入都會計入 `Limits.Sounds`（預設 256），直到載入失敗或其音效被關閉為止。

## 遊戲控制器

`gamepad` 套件會以 W3C Standard Gamepad 配置（`ButtonSouth`／`ButtonA` 至 `ButtonHome`，`AxisLeftX`／`AxisLeftY`／`AxisRightX`／`AxisRightY`）讀取已連接的遊戲控制器。`cx.Env().SupportsGamepads()` 會回報目標是否支援控制器（macOS、iOS、Windows、Linux、Android，以及具備 Gamepad API 的瀏覽器）。

`gamepad.Pads(cx)` 是一個會被追蹤的反應式讀取，當控制器連接或中斷時會重新呈現作用域，但按鈕或搖桿變化時不會重新呈現。每個 `gamepad.Pad` 都提供其 `Family`（`FamilyXbox`、`FamilyPlayStation`、`FamilyNintendo` 或 `FamilyGeneric`，也可透過 `gamepad.DetectFamily` 分類）、平台有回報時的 USB `VendorID` 與 `ProductID`，以及表示是否支援觸覺馬達的 `CanRumble`。在 `RequestAnimationFrame` 迴圈中，`gamepad.State(cx, id)` 會回傳控制器的按鈕（`[0, 1]`，`>= 0.5` 時 `Pad.Pressed` 為 true）與軸（`[-1, 1]`，負值為左／上，正值為右／下），每個顯示影格擷取一次快照。

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` 會驅動低頻（重／左）與高頻（輕／右）震動馬達，強度限制在 `[0, 1]` 範圍內，並取代該控制器上任何進行中的震動；它會在 `duration` 結束後、應用程式進入背景或失去焦點時（`cx.Env().Lifecycle() != core.LifecycleActive`）、`cx` 被釋放時，或控制器中斷連接時自動停止。`gamepad.StopRumble(cx, id)` 可提早停止震動；`CanRumble == false` 的控制器會順利地忽略震動呼叫。

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

呼叫 `gamepad.EnableFocusNavigation(cx)` 即可選擇使用控制器在原生焦點停駐點之間導覽（方向鍵與左搖桿移動焦點，`ButtonSouth` 啟用具有焦點的控制項，`ButtonEast` 觸發返回）。進行中的遊戲畫面檢視會呼叫 `gamepad.CaptureInput(cx)`，在其掛載期間暫停焦點導覽。在 TSX 中，`nsgo/gamepad` 提供 `supported()`、`pads()`、`state(id)`、`Button` 與 `Axis`，受 `gamepad` 套件能力管控。

## 音訊時序

使用 `audio.Now(cx)` 作為定時播放的時鐘。當 `audio.CanSchedule(cx)` 為 true 時，將 `audio.Voice.StartAt` 設為未來的時間。針對目前的輸出路由呼叫 `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })`。macOS、iOS、Linux 與網頁支援定時開始；在 Windows 與 Android 上，`CanSchedule` 為 false，設定了 `StartAt` 的播放會立即開始。回呼函式的 `known` 值會說明路由是否回報了延遲，Windows 與 Android 不會回報。

## 遊戲服務

`gameservices` 套件提供一個與提供者無關的 API，可在 `cx.Env().CanUseGameServices()` 為 true 的地方使用玩家登入、成就、排行榜，以及原生的 Game Center／Play 遊戲服務覆蓋層。每個回呼函式都在引擎 goroutine 上執行，因此可以直接設定信號而不會阻塞影格。

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement` 會將進度限制在 `0..100`（`100` 會解鎖該成就）。`gameservices.LoadScores` 會讀取 `Global` 或 `Friends` 在 `AllTime`、`Week` 或 `Today` 期間的排名 `gameservices.ScoreEntry` 列，而 `gameservices.ShowAchievements` 與 `gameservices.ShowLeaderboard` 會開啟平台的原生覆蓋層。macOS、iOS 與 tvOS 透過 purego 存取 GameKit；Android 將透過一個尚未納入執行階段的 Java 執行階段掛鉤存取 Play 遊戲服務，因此目前回報 `CanUseGameServices() == false`；Windows、Linux、網頁、watchOS 與小程式回報 `CanUseGameServices() == false`，並回應 `gameservices.ErrUnsupported`。

在測試中，使用 `nsgotest.Environment(core.Environment{CanUseGameServices: true})` 啟用遊戲服務，並以 `App.SetGameServicesPlayer`、`App.SetGameServicesFriends`、`App.SeedLeaderboardScores`、`App.SetGameServicesError`、`App.GameServicesAchievements`、`App.GameServicesScores`、`App.GameServicesShownAchievements` 與 `App.GameServicesShownLeaderboards` 操作每個應用程式隔離的模擬實作。

## 下一步

- [通知](notifications.md)：權限、角標與按壓。
- [表單](forms.md)：文字欄位與鍵盤。
- [嵌入](embedding.md)：主機應用程式提供給其嵌入應用程式的服務。
- [建置與發佈](building.md)：套件所需的權限與檔案。

### 體適能、區域與限時動作

`cx.WatchSteps`、`cx.WatchActivity` 與 `cx.WatchPressure` 會回傳具作用域的串流。在使用步數／活動之前，以及在 Apple 上使用氣壓之前，請先請求 `core.PermissionActivityRecognition`。Apple 透過 Core Motion 提供這三者；Android 提供計步器與氣壓，並回報活動分類無法使用。計數屬於樣本的 `Since` 時間範圍；恢復後的時間範圍會帶有 `Gap`。氣壓以帕斯卡為單位，並可選擇性提供以公尺為單位的相對高度。缺少的距離、樓層與高度其 `Valid` 為 false。

`cx.WatchGeofences` 接受 `core.GeofenceOptions`，其中包含一到二十個不重複的 `GeofenceRegion` 圓形區域（WGS 84 座標與以公尺為單位的半徑）。它會為每個區域回報 `GeofenceInside`、`GeofenceOutside` 或 `GeofenceUnknown`。`Background` 需要背景定位存取權／宣告，以及 Apple 的「永遠」授權；Android 還需要通知標題。停止或釋放會移除其註冊；任何訂閱都不會在行程終止後存續。

`cx.RequestPreciseLocation("route", done)` 會明確請求精確位置授權，並回應一個布林值，絕不回應位置。請在 `apple.info` 中宣告 `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`。Android 需要大略與精確兩種權限。回應 false 時，大略位置仍可能維持授權。

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` 會請求一個有限時長的即時串流。它需要 `AccessMotion` 與 `AccessBackgroundMotion`；TSX 套件需宣告 `motion` 與 `backgroundMotion`。iOS 可能會提早使其執行租約到期。Android 應用程式需設定 `android.background_motion: true` 並宣告 `FOREGROUND_SERVICE`；即使請求的時長更長，非黏性服務也會在三分鐘內停止。時長到期或作業系統使其到期時會回報 `SensorTimedOut`。停止／釋放會結束提供者與租約；終止後不會再記錄任何資料。其他目標會回報無法使用。

TSX 的 `Sensors` 對應這六個方法，並以毫秒為單位。無頭測試可透過 `FeedSteps`、`FeedActivity`、`FeedPressure` 與 `FeedGeofence` 提供資料，並設定 `PreciseLocationAnswer` 以回應精確度請求。實體的體適能、區域喚醒與背景暫停行為仍需在裝置上驗收。
