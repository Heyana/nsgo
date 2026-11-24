---
title: 測試
description: 以使用者操作應用程式的方式進行測試，無論是在無頭模式或原生視窗中。
group: 工具鏈
order: 21
---

## 撰寫測試

`nsgotest` 會在不開啟視窗的情況下啟動應用程式。測試會依文字或角色尋找控制項，並以使用者的方式操作它們：

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestPress(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("text:Press me")
	app.Find("text:Pressed 1 times")
}
```

當測試在目標平台上執行時，`TestMain` 會將主執行緒交給平台。

當套件需要涵蓋整個行程的設定與清理工作時，請將它們寫在 `MainFunc` 中的 `m.Run` 周圍；延遲執行的清理函式會在測試行程結束前執行：

```go
func TestMain(m *testing.M) {
	nsgotest.MainFunc(func() int {
		setup()
		defer teardown()
		return m.Run()
	})
}
```

`Main(m)` 是 `MainFunc(m.Run)` 的單行簡寫形式。目標平台會在各自的行程中執行每項測試，因此這些設定與清理作業在每個測試行程只會執行一次。

以函式啟動的應用程式 `nsgo.Run(func(app *core.Scope) {...})` 可使用 `nsgotest.StartApp(t, fn)` 進行測試；它會像 `Run` 一樣在應用程式自身的作用域中執行該函式一次，因此應用程式選單與結束處理常式都會保留。搭配桌面 `nsgotest.Environment` 時，應用程式的主檢視會顯示在 `nsgo.Run` 所開啟的主視窗中：包含 macOS，以及 Windows 與 Linux 上以函式啟動的應用程式。在該環境中 `desktop.CurrentWindow` 與 `OnClosing` 皆可運作；`app.CloseWindow()` 會送出視窗關閉按鈕的請求並回報視窗是否關閉，而單獨測試處理常式時，可直接呼叫它並讀取 `WindowClosing.Canceled()`（`OnQuitRequest` 則讀取 `QuitRequest.Canceled()`）。`app.RequestQuit(false)` 會送出使用者結束請求，而 `app.RequestQuit(true)` 則送出強制結束請求（例如登出）。兩者在無頭模式與 macOS、Windows、Linux 上皆可運作，並會將核准的請求計入 `app.Quits`。在 `app` 上執行的步驟皆作用於主視窗。`app.Opened` 會依照應用程式請求的設定（包含 `Transparent` 與 `SkipTaskbar` 等）列出應用程式開啟的視窗、面板與系統列檢視；`app.Window` 則以 `nsgotest.WindowRequest` 值列出主視窗的請求。可使用 `nsgotest.WindowSetConstraints` 與請求的 `Constraints` 來檢查尺寸限制。在無頭模式下，`app.Shares` 會以 `nsgotest.Share` 值記錄分享請求，其中包含檔案、文字與錨點。`app.DropFiles` 可指定放置區域內的任何元件：包覆該元件且距離最近、設有 `OnFileDrop` 的檢視會接收這些檔案。`app.OpenLinks("myapp://settings")` 會在無頭測試與 iOS、Android 目標平台上將應用程式自訂連結傳遞至 `desktop.OnOpenLinks` 或 `mobile.OnOpenLinks`。`app.ReceiveShare(mobile.SharedItem{Text: "https://example.com", MIME: "text/plain"})` 會在無頭測試與 iOS、Android 目標平台上將分享項目傳遞至 `mobile.OnShared`。檢視的 `Bounds()` 為其在視窗座標點中的框架範圍，與 `Ref.Bounds` 所回報的一致。圖片檢視的 `Image.Source` 代表該檢視顯示的來源，因此 `app.Find("id:video").Image.Source.Version` 與其 `Pixels.Rect` 可指出送達該檢視的像素影格與尺寸。在無頭模式下，`app.Press` 會像在目標平台上那樣切換系統開關（`ui.SwitchSystem`），而 `app.Emit("id:switch", "change", true)` 則可直接回報數值。`app.Navigate` 與 `app.PostMessage` 在網頁檢視已掛載但隱藏時仍可觸及，且該檢視會持續載入。`View.Web` 保存了網頁檢視來源、設定與導覽原則的 `nsgotest.WebConfig` 快照。`View.WebCommands` 則記錄應用程式傳送給網頁檢視的指令，可將其 `Command` 與 `nsgotest.WebLoad` 或 `nsgotest.WebSendInput` 等常數比對。

已掛載的圖片、輸入、強制回應（modal）、螢幕、捲動、視訊與子母畫面設定皆使用 `nsgotest` 組態記錄。`View.Sticky` 保存 `nsgotest.StickyCell` 值。當模型收到批次資料時，會複製圖片變體映射表與捲動清單。

使用 `app.Surfaces()` 可依開啟順序列出已開啟的視窗、面板與系統列檢視，且主視窗會排在第一位。`app.Surface("title:Settings")`、`app.Surface("id:settings-root")` 與 `app.Surface("index:1")` 可依目前標題、根檢視的測試 ID 或在該清單中的位置選取其中一個表面。選取器必須剛好只符合一個已開啟的表面。

在 macOS、Windows 與 Linux 目標平台上，`app.Surface("id:palette").WindowState()` 會在畫面穩定後於其 UI 執行緒上讀取實際原生視窗的 `Frame`、`Visible` 與 `Focused`。比對面板與原始視窗，即可確認顯示 `NoActivate` 面板時鍵盤焦點是否仍維持在原處。當無法取得原生位置時（例如在 Wayland 上），`Frame.Move` 為 false。無頭測試與其他目標平台會略過此步驟；它絕不會取代應用程式所請求的位置或焦點。

表面具備 `App` 的檢視查詢、輸入與手勢步驟、媒體回報、擷取、調整大小與等待方法：例如 `Exists`、`Texts`、`Enabled`、`Clear`、`Focus`、`Hover`、`RightClick`、`DropFiles`、`Resize` 與 `Settle`。它們會選取該視窗並沿用相同的 `App` 行為；等待與操作步驟會使整個應用程式進入穩定狀態。`Post` 會透過原始的 `App` 排入元件圖作業，並在分派前檢查所選表面是否仍然開啟。應用程式層級的時鐘、服務測試替身（fixture）、選單與警告對話框回覆，請維持在原始的 `App` 上操作。

`Config` 會以 `nsgotest.SurfaceConfig` 傳回開啟選項，該設定同時也會記錄於 `app.Opened` 中。`Screenshot`、`CapturePNG` 與 `Capture` 會在原生目標平台上擷取該表面。`Close` 會透過 `OnClosing` 請求關閉該視窗並回報是否已關閉。已關閉的控制代碼會使查詢、步驟與已排入的 `Post` 失敗，並從表面清單中移除；關閉父視窗同時也會關閉其子視窗。相同的步驟在無頭模式與 macOS、Windows、Linux 測試主機上皆可運作，並遵循各步驟既有的目標平台支援範圍。單一表面的目標平台僅會列出其主表面。

`app.Menu`、`app.DockMenu` 與 `app.PopupMenu` 會公開 `nsgotest.MenuItem` 快照，包含標籤、角色、勾選狀態與巢狀項目。使用對應的選擇步驟並傳入標籤路徑，即可觸發項目。

`app.ClickTray("index:0", core.MouseButtonLeft)` 或 `app.ClickTray("title:Tools", core.MouseButtonRight)` 會在無頭桌面測試中回報對啟用中系統列圖示的點擊。`ClickTrayIndex` 與 `ClickTrayTitle` 為便利形式。當測試需要模擬平台回報的圖示矩形範圍時，請在按鈕參數後傳入一個 `core.Bounds`。若未提供，則無法使用 `Tray.ClickBounds`。系統列選單隨後即可供 `ChoosePopup` 使用。

一旦指標從按下的位置移動超過 8 個點，`Drag` 便會取消指標底下的按壓狀態。網頁端亦套用相同規則，因此回到起始檢視的拖曳動作不會觸發 `OnPress`。`app.DragTo("id:source", "id:target")` 會透過一般的觸控回應器在兩個檢視的中心點之間執行拖曳；選取的表面同樣具備 `DragTo`，而 TSX 測試則使用 `await app.dragTo("id:source", "id:target")`。請確保兩個檢視皆為可見：該步驟在拖曳過程中不會捲動畫面，也不會直接呼叫放置處理常式。詳見[拖放操作](drag-drop.md)。

`app.KeyCommands()` 會列出在目前焦點與可見檢視狀態下可觸發的具標題鍵盤快捷鍵。無論在無頭模式或目標平台上，它都會回報按鍵、標題與群組。

`app.PressWith("id:row", core.Modifiers{Shift: true})` 會在單次按壓期間按住修飾鍵；`PressAtWith(selector, core.Point{X: 20, Y: 10}, mods)` 則會按壓檢視內的指定點位。`Shift`、`Control`、`Alt` 與 `Platform`（Apple 平台上為 Command，其餘平台為 Windows/Super 鍵）無論在無頭模式或透過原生目標平台輸入，都會傳遞至 `OnPressDown`、`OnPressUp` 與 `OnPress`。原生平台慣例依然適用：macOS 上的 Chrome 會將 Control 加上點擊轉換為右鍵內容選單。後續的 `Press` 或 `PressAt` 不會帶有修飾鍵。watchOS 因不具備鍵盤修飾鍵輸入，會拒絕帶修飾鍵的按壓操作。

目標平台步驟會將已捲動的控制項顯示在覆蓋視口邊緣的原生列上方，並遵循巢狀視口與捲動軸向。`PressAt` 使用指定檢視之視口中的點位。在無頭模式與目標平台模型中，指標鎖定、可見性與捲動限制皆會計入捲動內容的縮放比例，包含巢狀或小數倍縮放。檢視的 `Bounds()` 會保留其變換前的版面配置框架，與 `Ref.Bounds` 相同。

應用程式鎖定指標後，`app.PointerDelta(dx, dy)` 會將相對移動量傳送至鎖定中的檢視。無頭測試可透過 `nsgotest.Environment(core.Environment{CanLockPointer: true})` 啟用此功能。無論在無頭模式或目標平台測試中，按下 Escape 鍵皆會解除鎖定。

按壓操作可以直接指定文字欄位。無論在無頭模式或每個目標平台上，按壓後皆可立即讀取 `app.Find("id:field").Focused`，包含在被按壓檢視的處理常式將焦點移至該欄位的情況。

`app.SelectTab("id:tabs", 1)` 會選取已掛載 `mobile.TabBar` 的第二個項目。它在搭配 iOS 或 Android `Environment` 的無頭模式中可運作，並在 iOS 與 Android 目標平台上使用原生的分頁列。索引從零開始，且必須符合現有項目。手機導覽請使用此方法；`Emit` 僅會回報模型事件，且僅於無頭模式執行。

`app.Focus(selector)` 會像點擊般抵達文字欄位，並在桌面或網頁 `Environment` 或對應目標平台上透過 Tab 抵達可點擊或 `Focusable` 檢視，使接續的 `app.Key("Return")` 能按壓它。應用程式呼叫的 `Ref.Focus()` 在 Android、iPhone 與 iPad 上也能在不顯示軟體鍵盤的情況下聚焦已啟用的可點擊或 `Focusable` 檢視；在原生行動裝置測試中請使用 `app.Post(ref.Focus)` 並等待焦點完成。在行動目標平台上，焦點斷言會遵循原生的焦點回報，因此遭拒絕的請求不會通過無頭模型檢查。在桌面與網頁目標平台上，`app.Focus` 步驟對一般檢視仍使用 Tab 鍵。可點擊檢視是指具備 `OnPress`、`OnPressDown`、`OnPressUp` 或 `OnLongPress` 任一者的檢視，與各平台行為一致；對其按下 Return 或 Space 會傳送它所處理的按下、放開與點擊事件，因此僅有 `OnLongPress` 的檢視雖可取得焦點，但不會有按鍵能按壓它。

無論專案以哪個平台為目標，無頭執行預設皆使用手機慣用介面（phone idiom），因此 `cx.Env().Desktop()` 為 false 且懸停工具提示絕不會顯示。傳入 `nsgotest.Idiom(core.IdiomDesktop)` 即可測試桌面呈現方式，同時保留其餘環境設定（包含 `Environment` 選項的設定）。鍵盤焦點遵循環境的作業系統，而非其慣用介面。

在 macOS 目標平台測試中，測試視窗會忽略 AppKit 的螢幕尺寸限制，因此即使內容高度超過可見螢幕，`nsgotest.Size` 與 `Resize` 仍會維持所請求的內容尺寸。在 iOS 與 tvOS 上，`app.Resize(width, height)` 會調整測試宿主的 `UIWindow` 大小並等待原生版面配置完成；每項測試皆以所屬場景（scene）的尺寸開始。這可檢查不同視口尺寸下的內容，但不會調整 Split View、Slide Over 或幕前調度（Stage Manager）視窗。即時自動化會跟隨執行中場景的尺寸並拒絕 `Resize`。

在真實目標平台上，`nsgotest.Environment` 會固定外觀、語系、文字大小、減少動態效果、提高對比、定時觸覺回饋支援、散熱狀態、低耗電模式、登入啟動原因、前景應用程式與平台是否跟隨之，以及裝置名稱與型號。設定帶有 `DisplayID` 的 `Displays` 可固定顯示器清單與視窗目前的顯示器，包含其更新率。其餘屬性則維持目標平台自身的狀態：作業系統、慣用介面、安全區域與系統標題列高度。因此，帶有 `core.OSWindows` 的 `Environment` 並不會讓 `go tool nsgo test -target macos` 以 Windows 方式進行版面配置；若要測試該環境，請改在套用完整 `Environment` 的無頭模式下執行，或加上 `-target windows`。`nsgotest.Idiom` 同樣僅適用於無頭模式。`nsgotest.Size` 會在 macOS、Windows、Linux 與網頁上設定測試視窗的大小；Android、iOS、tvOS 與 watchOS 則維持裝置螢幕尺寸。在 `-target macos` 上，若未傳入 `Environment`，應用程式會如 `nsgo.Run` 般開啟視窗：根檢視或 `SetMainView` 設定的檢視會顯示在主視窗中，該處的 `desktop.CurrentWindow` 與 `OnClosing` 皆可運作，除非以 `nsgotest.Size` 指定大小，否則視窗會以 480 × 640 點開啟，並如同無頭模式般於其上執行步驟。透過關閉按鈕或 Close 項目的快捷鍵關閉視窗時，會詢問其 `OnClosing` 處理常式；一旦視窗關閉便不再有可操作的檢視，而應用程式在最後一個視窗關閉時所請求的結束作業，會計入 `app.Quits` 而非終止測試。

當沒有任何繫結攔截按鍵且沒有應用程式選單項目將其設為快捷鍵時，`app.Key("mod+a")` 會選取目前取得焦點欄位的文字，而 `mod+c`、`mod+x` 與 `mod+v` 則會搭配 `app.Clipboard` 進行複製、剪下與貼上。在 macOS、Windows 或 Linux `Environment` 中，若按鍵未被任何繫結、已聚焦檢視或欄位取走，且是某個已啟用應用程式選單項目的快捷鍵，便會觸發該項目，因此 `app.Key("mod+r")` 可測試選單快捷鍵是否送達該項目：已停用的項目不會接收任何按鍵；當欄位擁有焦點時，`NotInText` 項目不會接收任何按鍵，且不會攔截排在它之後項目的相同按鍵；而在 Linux 上，未搭配 Control、Alt 或徽標鍵的按鍵不會被任何項目接收。在無頭模式下，具標準角色且未設定 `OnSelect` 的項目，透過 `app.ChooseMenu` 或其快捷鍵選取時，會執行桌面平台對應的動作：Close 會透過 `OnClosing` 處理常式關閉主視窗；Quit 會詢問 `OnQuitRequest` 處理常式，且核准的請求會計入 `app.Quits`；Minimize、Zoom 與 Toggle Full Screen 會變更主視窗在環境中回報的狀態，應用程式的其他視窗則維持不變（`app.SetWindowEnvironment` 可變更任一視窗的狀態）；編輯角色則作用於取得焦點的欄位。任何其他角色，以及在欄位中執行 Undo 或 Redo，在無頭模式下會使該步驟失敗而非吞掉按鍵；在 `-target` 下此類項目會略過該測試。在無頭模式、桌面目標平台與網頁上，`KeyAlways` 繫結會優先於欄位取得 Return、Backspace 與全選；`KeyAuto` 與 `KeyNotEditing` 則將它們留給取得焦點的欄位。Android 與 iOS 測試宿主會優先將 Backspace 與全選交給 `KeyAlways` 繫結；Android 的 Control 快捷鍵隨後會送達原生編輯器，而 iOS 則會透過其軟體鍵盤路由未繫結的按鍵。iOS 上的軟體 Return 鍵目前尚未先將按鍵交給鍵盤對應表（[#1351](https://github.com/nsgo-dev/nsgo/issues/1351)）。已聚焦檢視的 `OnKeyDown` 會最先聽到 `app.Key`（包含單獨的修飾鍵如 `app.Key("shift")`），在無頭模式以及桌面與網頁目標平台上，被其消耗的按鍵不會觸及任何繫結。`app.Key` 是一次完整的按壓，因此該檢視的 `OnKeyUp` 隨後會聽到按鍵放開。`app.KeyDown` 與 `app.KeyUp` 會分別按下與放開單一按鍵，絕不構成組合鍵：`KeyDown` 按住的修飾鍵會附加至其後的每個按鍵直到執行 `KeyUp`，因此先 `app.KeyDown("shift")` 再 `app.Key("a")` 會按下 Shift+A，而對未按住的按鍵呼叫 `KeyUp` 會使該步驟失敗。測試結束時仍按住的按鍵屆時會被釋放（在 `-target` 下透過平台進行），讓聽到按下的檢視也聽到放開，且不會有任何按鍵殘留至下一項測試。`core.KeysDown` 與 `KeyHeld` 可讀取目前按住的按鍵。在 `-target` 下，watchOS 會略過包含這些操作的測試。`app.MouseDown`、`app.MouseDownAt`、`app.MouseDownWith`、`app.MouseDownAtWith` 及其對應的 `MouseUp` 方法可按下與放開五個滑鼠按鍵中的任一鍵（`core.MouseButtonLeft` 至 `core.MouseButtonForward`），並更新 `core.MouseButtonsDown` 與 `core.MouseButtonHeld`；`app.BlurWindow()` 則模擬視窗失去焦點並清除所有按住的按鍵與滑鼠按鍵。

`KeyDown` 與 `KeyUp` 同時也會驅動 Android 的原生硬體按鍵路徑與 HarmonyOS 的 Go 按鍵橋接。在 Android 與明確聚焦的 iPhone/iPad 檢視上，`OnKeyDown` 會在繫結與原生啟用之前執行；已消耗的按下事件仍會收到 `OnKeyUp`，放開事件會保留給原始檢視，且 `core.KeysDown` 會追蹤原生編輯器以外的按住按鍵。Android 的非同步消耗回應能維持 UI 反應流暢；失去焦點、卸載或超過兩秒未收到回應時，會清除待處理按鍵且不重播其原生動作。HarmonyOS 會追蹤回報的按鍵並可抑制 Go 繫結，但其宿主目前仍缺乏原生一般檢視焦點，且在從非同步橋接返回後無法抑制 ArkUI 動作。

`app.KeyCode("KeyW")` 會按壓實體按鍵位置，而 `KeyCodeDown` 與 `KeyCodeUp` 則分別按住與放開該位置。在無頭測試中，可設定 `core.Environment.KeyCodeNames`（例如 `map[string]string{"KeyW": "z"}`）來測試其他鍵盤配置；未指定的位置預設採用美式配置。在 `-target` 下，平台自身的按鍵事件會攜帶該位置（例如掃描碼或按鍵碼），因此右側修飾鍵或數字鍵盤的數字能與左側或主鍵盤數字列區分。在 macOS、Windows、Linux 與網頁上，平台目前的鍵盤配置會決定按鍵名稱，因此 AZERTY 配置下的 `KeyW` 為 `z`；iOS、Android 與 HarmonyOS 則回報美式配置在該位置對應的按鍵。若同時透過 `KeyDown` 與 `KeyCodeDown` 按住某個修飾鍵，該按鍵會維持按下狀態直到兩者皆釋放它為止。不支援按壓實體位置的目標平台（例如 watchOS）會略過該測試。

在 `-target` 下，`app.Select(selector, start, end)` 會移動原生欄位的選取範圍。偏移量以 UTF-16 單位計算，會裁剪至文字範圍內，且反轉的端點會重新排序。iOS 與 Android 使用其原生文字輸入 API；HarmonyOS 使用該欄位的控制器並等待其選取回呼。`OnSelectionChange` 會正常執行，且即使沒有該回呼，測試模型仍會讀取實際範圍。macOS、Windows、Linux 與網頁使用全選與編輯方向鍵，因此佔用這些按鍵的繫結或應用程式選單項目會從該步驟攔截它們。接續的輸入與原生剪貼簿編輯會作用於選取範圍；HarmonyOS 的測試輸入會取代其回報的原生範圍，但其測試宿主目前尚不支援驅動原生剪貼簿按鍵。

無頭模式的 Linux 與 Android 測試可在 `app.Installs` 中檢查傳遞給安裝程式的封裝檔路徑。設定 `app.InstallPackage` 可讓模擬的安裝程式傳回錯誤。

`app.DoublePress(selector)` 會在單一步驟中送出兩次按壓，讓平台計算連按兩下或連點兩下。`app.Pan` 使用目標平台的拖曳輸入，每個偏移量皆相對於起點。`app.Pinch` 在無頭模式下可運作，並會驅動 macOS、iOS 模擬器、Android 與網頁的平台手勢路徑。在 iOS 模擬器上，每個縮放比例會設定兩指之間的距離，當兩指超過 UIKit 的辨識門檻時便會開始回報。在 Android 上，兩指觸控會先展開到足以啟動原生辨識器的距離，接著依序移動至請求的縮放比例。在不支援雙指縮放輸入的目標平台上，執行到 `Pinch` 時會略過該測試的其餘部分，並回報哪個目標平台缺乏支援。

`app.Orientation` 保存目前作用中的 `Scope.RequestOrientation` 遮罩。當應用程式採用一般螢幕方向原則時，該值為零。

`app.NowPlaying()` 會傳回應用程式最後一次的系統媒體發佈資訊，或在清除後傳回 nil。`app.MediaCommand(core.MediaCommand{Kind: core.MediaSeek, Position: 30 * time.Second})` 會透過該發佈將指令傳送至其啟用中的處理常式。在無頭模式下請使用 `nsgotest.Environment(core.Environment{NowPlaying: true})`，或在 macOS 目標平台上執行。

## 無視窗繪圖

在 macOS 上，`nsgotest.PlatformTextEngine()` 可讓無頭測試使用 CoreText 度量。請透過 `nsgotest.TextEngine` 傳入；在其他平台上它會傳回 nil，而模型會沿用其近似度量。啟動應用程式前，請使用 `nsgotest.RegisterFont` 註冊字型位元組資料。

`nsgotest.Render(app.Find("id:score").Drawing, core.Size{Width: 600, Height: 800}, 2)` 會在不開啟視窗的情況下，將記錄的畫布重播並渲染為獨立擁有的 `*image.RGBA`。可使用 `image/png` 編碼或比對其像素。尺寸單位為點，縮放比例為每點的像素數，透明輸出上限為 16 Mi 像素。兩者皆必須為有限且大於零的數值。圖片會以 30 秒的期限同步載入；動態圖片會使用第一格影格，失敗時則會傳回錯誤。渲染前請先解析動畫插槽與檢視繪製。

無視窗渲染與字型註冊目前支援 macOS。其他目標平台會傳回 `nsgotest.ErrNoPlayer`；原生目標平台上的 `Screenshot` 仍可獨立使用。

`View.Style` 會記錄 `nsgotest.ViewStyle`，而 `View.Animated` 則記錄 `nsgotest.AnimatedValues`。`app.Animated(selector)` 會將繫結數值套用至記錄的基礎樣式。效果與自訂游標會保留其應用程式數值識別；`Radii`、`Rounded`、`Rim` 與 `Matrix` 則維持相同的幾何原則。

`app.Find(selector).Kind` 的型別為 `nsgotest.ViewKind`；當測試需要檢查已掛載檢視的型態時，可將它與 `nsgotest.KindText`、`KindInput`、`KindCanvas` 或 `KindGPU` 等常數比對。

無頭 GPU 測試可使用 `app.Surface("title:Inspector").GPUReadPixels("id:viewport")` 取得指定視窗的軟體 GPU 像素。`Surface.TriggerGPUDeviceLost` 與 `Surface.RestoreGPUDevice` 會在該視窗中模擬裝置遺失與復原，並採用該視窗的像素比例。即使不同視窗之間檢視 ID 重複，裝置與回讀回呼仍會保持獨立；`app.GPUFrames()` 會記錄整個應用程式的所有送出影格。這些輔助函式皆運作於無頭軟體 GPU 模型之上。

`nsgotest.UpdaterState(core.UpdaterState{...})` 會在無頭模式與原生目標平台的首次渲染前，預先植入更新器能力與偏好設定。設定 `app.Update` 即可提供每次檢查的結果；偏好設定變更會在不連線更新訂閱源（feed）的情況下進行模擬。

無頭測試可檢視 `app.EditMenu` 與 `app.EditMenuItems()`，透過 `app.ChooseEditAction(core.EditCopy)` 選擇提供的動作，並以 `app.CloseEditMenu()` 關閉選單。貼上動作會使用測試的剪貼簿。`app.LongPressAt(selector, point)` 會按住特定點位；無頭模式的 `app.Touch(selector, point).Hold(duration)` 會在手指抬起前觸發長按，讓後續的 `Move` 能夠測試選取滑動操作。選單記錄與動作選取皆為模型模擬行為；請使用原生驗收測試來驗證系統工具列。

`View.Props` 是包含識別資訊、無障礙名稱與狀態、動作，以及焦點與指標設定的 `nsgotest.ViewProps` 快照。`HelpText`、`HasAction`、`ActionList` 與 `FocusStop` 會公開其原則；標籤 ID、動作與漣漪設定皆複製自已掛載的數值。

## 小工具與即時動態

`nsgotest.Widgets(true)` 會記錄應用程式透過 `widget.Update` 發佈的內容。可使用 `app.Widgets()` 讀取每種類型的最新時間軸。未設定日期的項目會取得應用程式的牆上時間（wall time）。使用 `nsgotest.Widgets(false)` 可讓 `widget.Available()` 傳回 false，並使更新呼叫傳回 `widget.ErrNoWidgets`。

`nsgotest.LiveActivities(true)` 會記錄啟動、更新、提醒與結束事件。`app.LiveActivities()` 會依啟動順序傳回各項動態，包含其內容變更與任何最終內容。使用 `nsgotest.LiveActivities(false)` 可讓啟動呼叫傳回 `widget.ErrActivitiesDisabled`。這些選項會在測試期間接管對應的跨整個行程服務，包含在原生目標平台上亦同。呼叫這些服務的測試請依序執行。

無頭測試可透過 `app.ConnectGamepad(id, name)`、`app.SetGamepadInfo(id, family, vendorID, productID, canRumble)`、`app.DisconnectGamepad(id)`、`app.SetGamepadButton(id, btn, value)` 與 `app.SetGamepadAxis(id, axis, value)` 模擬遊戲控制器：連線、資訊更新與斷線會等待穩定並重新渲染讀取 `gamepad.Pads` 的作用域，而按鈕與類比軸更新則會在下一影格於 `gamepad.State` 生效。`app.GamepadRumbles()` 與 `app.LastGamepadRumble(id)` 用於檢查記錄的 `gamepad.RumbleEvent` 開始與停止震動指令。在 `-target` 下執行時會略過該測試。

`app.FindMounted("id:tab-bar").TabBadge(index)` 會在無頭模式與目標平台測試中傳回原生分頁的文字或圓點徽章。在自繪的 `ui.TabShell` 分頁上，`Props.Value.Text` 即為徽章所宣告的數值。

## 執行測試

```sh
go tool nsgo test
```

`test` 會為每項測試印出一行結果。`-run` 像 `go test` 一樣選取測試，包含諸如 `-run 'TestGallery/dark-home'` 的子測試。在目標平台上，`-timeout` 會設定每項測試的期限（預設為三分鐘；`0` 代表停用），結果會回報是否達到該期限。在模擬器或手機上，若測試的 `TestMain` 呼叫 `nsgotest.Main`，其期限會自測試宿主啟動時起算，因此應用程式啟動較慢不會計入測試期限內；啟動本身另有五分鐘的限制，超過時測試會因啟動逾時而失敗。若一項測試的所有子測試皆被略過，該測試會計為略過。`-junit report.xml` 會寫出保留各項略過原因的 JUnit 報告；`-trace` 會印出每個步驟的掛載項目與提交時間；`-bench` 則改為執行效能基準測試，每個結果印出一行。無頭執行會將 `--` 之後的旗標轉發給 `go test`；目標平台執行則接受下方列出的執行期旗標。即使無頭執行僅透過 `-- -c` 編譯測試，編譯器錯誤仍會顯示於主控台與 JUnit 報告中。

若傳入沒有 `nsgo.yaml` 的目錄（例如函式庫或模組根目錄），`test` 會涵蓋其下的每個套件；目錄也可如 Go 的套件模式那樣以 `/...` 結尾。它一次僅接受一個目錄，傳入第二個目錄時會因錯誤而停止：若要測試數個目錄，請傳入它們共同的上層目錄，或逐一執行。`go tool nsgo check` 亦以相同方式接收套件並進行靜態檢查（vet）。

測試會檢查響應式圖（reactive graph）的擁有權：請在應用程式的 goroutine 上存取訊號與作用域，並從背景工作人員呼叫 `cx.Post`。若在應用程式閒置時從其他 goroutine 呼叫，將會觸發帶有 `Scope.Post` 診斷訊息的 panic；其餘情況則可透過 `-race` 捕捉。引擎內部的 panic 仍會使測試失敗；在發佈的應用程式中，這會停止受影響的引擎並將其堆疊追蹤回報給宿主。開發建置同樣會檢查擁有權；發佈建置則省略此檢查；WebAssembly 的開發與測試建置因瀏覽器回呼可能會在瀏覽器管理的 goroutine 上恢復執行而省略 goroutine ID 斷言，其餘測試檢查仍會正常執行。

`app.Post(func() { state.Set(value) })` 會將作業排入應用程式的圖形 goroutine，並在下一個步驟或等待進入穩定狀態時執行：無頭模式中為測試 goroutine，目標平台上則為引擎 goroutine。可從測試或模擬的背景工作人員呼叫它，接著使用 `app.Settle()` 或等待函式來讀取產生的檢視。該函式可再排入更多作業並由同一個步驟執行；它不應執行任何操作步驟，也不可呼叫 `t.Fatal`，因為在目標平台上它並非於測試 goroutine 上執行。

無頭模式的 `cx.ReadAsset` 會讀取專案的 `resources/` 目錄。在 `-target` 下則會讀取為測試打包的資源。

接著每個步驟會以一次一影格的速度推進時鐘（最多 2 秒），直到會結束的動畫都已結束且每個樣式轉場都執行了 `OnTransitionEnd`——在目標平台上僅針對轉場，因為動畫由平台自行驅動：因此透過 `EndingStyle` 關閉的對話框或彈出氣泡框在 `app.Press` 返回時便已消失。若想觀察過程中的狀態，請在啟動時傳入 `nsgotest.ManualFrames()`；之後它們只會在 `app.Advance` 推進時鐘時跟著前進。

## 時間

無論在無頭模式或目標平台上，應用程式都運行在 `app.Clock` 上——這是一個從 2026-01-01 00:00 UTC 開始的 `core.ManualClock`，只會隨著測試推動而前進。`app.Advance(d)` 會將時鐘推進 `d`，在每個計時器到期時執行它，且在應用程式等待期間，如同真實顯示器般於每個影格間隔執行一次顯示影格。`app.FastForward(d)` 則會直接向前快轉，如同筆電闔上一段時間後再打開：途中到期的每個計時器會在結尾執行一次，顯示影格亦會執行一次。在無頭測試的模擬顯示更新率下，回呼同時也會遵循表面所設定的 `SetPreferredFrameRate`。

應用程式使用 `cx.Now()` 讀取此時鐘，並使用 `cx.WallTime()` 讀取顯示的日期，因此請使用它們來量測應用程式自身的時間（例如手勢速度或連點兩下的時間窗）。`time.Now()` 與 `time.Sleep` 跟隨系統時鐘，不會因 `Advance` 而前進：使用 `time.Now()` 計時的應用程式會錯過測試所推進的時間，而在測試中使用 `time.Sleep` 只會拖慢測試速度。若要等待來自其他 goroutine 的作業（例如回覆後的 `cx.Post`），請使用 `app.WaitFor`、`app.WaitForGone` 或 `app.WaitUntil`，它們的逾時計算的是真實時間。

在無頭模式下，這些等待函式會在每次檢查之間以一次一影格的速度推進時鐘。若要在等待期間讓時鐘保持靜止，請在啟動時傳入 `nsgotest.ManualFrames()`。在目標平台上，等待函式會在平台自行執行自身動畫的同時讓應用程式進入穩定狀態，僅當樣式轉場等待其 `OnTransitionEnd` 時才會在檢查之間推進時鐘，與操作步驟的行為相同。

依賴日期的畫面（例如顯示今日到期事項）可在啟動時傳入 `nsgotest.WallTime(t0)`：首次渲染時的 `cx.WallTime()` 即為 `t0`（含其時區），而計時器仍繼續依時鐘自身的時間運作。`nsgotest.Clock(c)` 會讓應用程式運行於測試自備的時鐘上，使外部程式碼能與之共用；當 `c.Advance` 推進該時鐘時，應用程式的計時器便會觸發。在目標平台上以 `nsgotest.Measure()` 啟動的測量測試，則會改為讓應用程式運行在系統時鐘上，如下所述。

若需要沒有對應操作步驟的無頭平台回報，請呼叫 `app.Report(nsgotest.Event{Kind: nsgotest.EventPress, ID: view.ID})`。該回報會複製其切片、導覽狀態與選用的 `nsgotest.TouchGeometry` 快照；`Data` 則保留測試的原生酬載。

## 在平台上執行

測試會在使用平台自身輸入的真實視窗中執行。測試探索會遵循目標平台的建置標籤，包含位於跨平台套件中僅限特定目標平台的測試檔案：

```sh
go tool nsgo test -target macos
```

在 macOS 上，測試視窗會忽略來自測試宿主外部的滑鼠、鍵盤與觸控板輸入，因此在測試套件執行期間使用電腦不會改變測試場景。測試本身的步驟依然正常運作。

測試視窗會在不啟用應用程式的情況下接收測試按鍵，讓您正在使用的應用程式得以保持在最前方。當另一個應用程式處於啟用狀態時，AppKit 可能會拒絕欄位的點擊事件，`Focus` 與 `Type` 會診斷出該拒絕情況。`go tool nsgo test -target macos -activate` 可讓測試應用程式在點擊欄位或按下按鍵前先啟用自身並取得鍵盤控制權。此模式需要已解鎖的桌面工作階段，且測試需匯入 `github.com/nsgo-dev/nsgo`（空白匯入即可）並從 `TestMain` 啟動 `nsgotest.Main` 或 `nsgotest.MainFunc`。

清理程序會關閉遺留開啟的原生內容選單，使其不會攔截下一個子測試的第一下點擊或按鍵。在 macOS 上，WindowServer 擷取會包含測試視窗中開啟的原生彈出選單，並裁剪至該視窗的邊界內；其他視窗則會被排除。

若要在原生 `WebView` 中傳送受信任事件，請檢查 `Env.CanSendInput` 並在應用程式的元件圖上送出輸入：`app.Post(func() { page.SendInput(core.WebClick(12, 16)) })`。請先等待頁面確認就緒再點擊，並先取得焦點再以 `core.WebText` 輸入文字。Linux 需要 X11 與 XTest；其測試宿主會接收這些腳本發出的事件，同時繼續拒絕實體輸入。

支援的目標平台包含 `macos`、`windows`、`linux`、`web`（於 Chrome、Chromium 或 Edge 中執行）、`ios`（於已連線的 iPhone 上執行，有多台裝置時以 `-device` 指定，並以 Xcode 上次使用的團隊或 `-team` 簽署）、`ios_simulator`、`tvos_simulator`（遙控器方向鍵、以 Return 作為選取鍵、Escape 作為 Menu 鍵）、`watchos_simulator` 與 `android`，或使用 `all`。每項測試皆在獨立的行程中執行。`app.Capture()` 會以 `image.Image` 傳回目前的像素，而 `app.CapturePNG()` 則傳回 PNG 位元組，兩者在無頭模式下皆會略過測試。在 macOS 上，這兩者擷取的是經過 WindowServer 合成後的視窗畫面，包含 Core Animation 的陰影、遮罩、濾鏡、動畫與插入游標；當無法擷取視窗時（例如視窗已最小化或位於其他 Space 中），檢視會改為透過 AppKit 自行繪製，不含上述效果，並在第一次發生時記錄於日誌中。NSGo 在編碼前會將兩種 macOS 擷取路徑都轉換為 sRGB。iOS 與 tvOS 亦以 sRGB 擷取，因此這些目標平台上的像素值不會受到顯示器是否採用 Display P3 的影響。Android 會擷取包含著色器表面在內的完整渲染視窗。`app.Screenshot("en/library")` 會將截圖儲存為 `.nsgo/test/screenshots/<target>/en/library.png`（在 `macos` 上為 `.nsgo/test/screenshots/macos/en/library.png`）；名稱可包含空白，但不可為絕對路徑或包含 `..`。`-screenshots dir` 可改將截圖儲存於 `dir` 目錄下，該路徑相對於工作目錄。若裝置日誌遺失或截斷了截圖的部分內容，測試會在寫入檔案前失敗；若截圖路徑已被先前的截圖佔用，該項測試亦會失敗。`-fps` 在 `macos`、`windows`、`linux` 與 `ios_simulator` 上會每秒記錄一次效能監測讀數，在 `web` 與 `android` 上則僅記錄引擎的讀數。`-env KEY=VALUE` 可設定測試環境的環境變數，手機測試沒有其他途徑取得這些變數。在模擬器或手機上，沒有 `nsgo.yaml` 之目錄的測試會以目錄名稱作為應用程式名稱進行安裝，其 ID 為 `dev.nsgo.` 加上目錄名稱的字母與數字：例如 `base/formula` 為 `dev.nsgo.formula`。在 `android` 與各模擬器上，測試會以應用程式 ID 加上 `.nsgotest` 安裝（App Clip 亦使用該 ID），並與已安裝的應用程式建置版本並存而非覆蓋它，並在測試結束後解除安裝。在 `ios` 上，測試會以佈建描述檔所指定的應用程式自身 ID 安裝，取代目前安裝在 iPhone 上的建置版本與其資料。

實體 iPhone 目標平台測試會保留應用程式的 stderr，包含測試宿主啟動後的 Go panic 診斷訊息。

在 `android` 與 `ios` 上，測試僅於 NSGo 的測試宿主中執行：Android 的執行時期會從測試行程的標準輸出讀取其協定內容，測試工具則透過宿主寫入的標記來取得測試輸出與結束代碼。任何要在這兩個平台上執行測試的套件，都必須在測試檔案中匯入 `github.com/nsgo-dev/nsgo`（空白匯入即可）以連結該宿主，並在 `TestMain` 中使用 `nsgotest.Main` 或 `nsgotest.MainFunc` 啟動它。在建置或安裝之前，`nsgo test` 會檢查宿主是否已連結且已宣告 `TestMain`。若該函式繞過宿主，Android 會回報無效的 stdout 協定並附上 `nsgotest.Main` 提示；在宿主啟動前，請將日誌與初始化輸出保留在 stderr。像 `ui/chat` 這類函式庫套件一旦在測試檔案中加入這兩者即可在手機上執行，同時原本即可在無頭模式或其他目標平台上執行。

未連結平台後端的套件會在目標平台的執行時期中以無頭模式執行。透過 `-target web`，函式庫控制項能以這種方式在瀏覽器內進行測試。儲存空間維持於記憶體中，且因瀏覽器沒有檔案系統，`DataDir`、`CacheDir` 與 `LogsDir` 皆為空字串。以檔案為基礎的 `nsgotest.MicrophoneInput` 模擬物件因需要真實 WAV 檔案，在該環境下會略過測試。

若 Android 測試在回報結果前發生 panic 或結束，會以該行程的結束狀態與已擷取輸出（包含 panic 堆疊）回報失敗。未啟動宿主便結束的 `TestMain` 同樣會以此方式失敗，即使結束狀態為 0 亦然。當 Android 目標平台測試逾時時，結果也會說明宿主是否曾印出啟動標記，並在執行器清除日誌前附上該項測試 `logcat` 輸出的結尾片段。

在 `web` 上，測試會在工具找到的無頭 Chrome、Chromium 或 Edge 中執行，或在 `NSGO_BROWSER` 指定的瀏覽器執行檔中執行。在 macOS 上，若已安裝 Chrome for Testing，工具會優先選用它。工具最多會等待 30 秒讓瀏覽器啟動，當指令被取消時會停止等待啟動，若瀏覽器結束或始終沒有回應則會回報瀏覽器的錯誤輸出。

每個操作步驟以及 `app.Settle()` 都會等待引擎待處理的提交完成，再計算 100 毫秒內無批次更新或有限動畫的空檔。即使應用程式持續變化，整個等待最多在五秒後停止；因此開啟較慢的第一個對話框無須在測試中額外加上 `Post` 或 sleep。

在 Android 上，工具會在建置前與每項測試前喚醒休眠的螢幕並解除無密碼的鎖定畫面。若裝置鎖定了 PIN 碼、密碼或圖形鎖，必須先解鎖才能繼續執行；錯誤訊息會指出該裝置名稱。暫時性的 `adb` 狀態讀取失敗會自動重試，而持續無法連線的裝置會立即終止該目標平台測試執行，而不會讓剩餘的每項測試逐一失敗。

若要進行測量而非檢查，請以 `nsgotest.Measure()` 啟動：應用程式會運行在系統時鐘上，`Advance` 會依實際傳入的時間長度等待，而步驟只會等待正在執行的動畫靜止。`app.Mark("name")` 可為接下來的測量讀數命名，而 `app.Mark("-")` 則結束該標記；無論是否加上 `-fps`，標記都會印出。

請將 NSGo 旗標放在目錄參數之前：`go tool nsgo test -target web ./app`，而非放在目錄之後。在無頭執行中，`--` 之後的旗標會傳遞給 `go test`。目標平台執行支援於該處傳入 `-short`、`-count`、`-cpu`、`-parallel`、`-failfast` 與 `-fullpath`，例如 `go tool nsgo test -target web ./app -- -short -count=3`；該處會明確拒絕其他 Go 測試旗標，包含 `-run` 與 `-timeout`，這兩者必須放在目錄之前。`-count=0` 不會執行任何測試。連結 NSGo 原生宿主的測試會針對每個 count 與 CPU 設定在全新的行程中重複執行，結果中會標示 `[run N]`，截圖置於 `run-N` 子目錄中，且每次呼叫各有獨立的逾時限制；函式庫則維持 Go 的同行程內重複執行方式。

## 媒體回報

`app.Play(selector, core.VideoInfo{Duration: time.Second, RateChangeable: true})` 會回報視訊載入完成，並附上其時長、原始尺寸與播放速率調整能力。將 `RateChangeable` 設為 false 可測試不提供倍速控制項的播放器。

在 `tui` 上，測試需要互動式終端機、已連結的 NSGo 後端與呼叫 `nsgotest.Main` 的 `TestMain`。每個行程開啟一個應用程式；其步驟會測試終端機編輯、焦點、滑鼠與按鍵路由。測試輸出會先暫存，直到終端機還原後才輸出。PNG 擷取會使用固定的可攜式字型將儲存格光柵化。[終端機目標](building.md#terminal)

## TSX 測試

TSX 應用程式中以 `nsgo/test` 撰寫的 `*.test.tsx` 檔案會隨著其 Go 測試一同執行。

`open("App.tsx", { clock: {} })` 會讓應用程式運行在手動時鐘上，並由 `app.clock` 推動它：`app.clock.runFor(ms)` 與 `app.wait(ms)` 類似，如同 `Advance` 般一次一影格推進，而 `app.clock.fastForward(ms)` 則如同 `FastForward` 般向前快轉。`clock: { time }` 會將 `Date.now()` 與 `new Date()` 讀取的時間起始於 `time`（可為 `Date` 或自 1970 年起的毫秒數），而 `app.clock.setSystemTime(time)` 可於稍後設定它；兩者皆不會推動計時器。未設定 `clock` 時，無頭模式應用程式的時間在 `waitFor` 與 `waitUntil` 等待期間仍會一次一影格地流逝，如同在裝置上一樣。

`open("App.tsx", { camera: true })` 會為測試環境提供相機，使 `Picker.canTakePhoto()` 解析為 `true`，並將 `Picker.takePhoto` 記錄於 `app.picks()` 中（帶有 `camera: true`），且由 `app.onPick` 回應：回傳 `PickFile` 或 `PickFile[]` 代表拍攝的照片，回傳 `null` 或 `[]` 代表取消，或拋出錯誤使 `Picker.takePhoto` 拒絕。未設定 `camera: true` 時，`Picker.canTakePhoto()` 為 `false` 且 `Picker.takePhoto()` 會拒絕。

若要在沒有測試檔案的情況下以腳本執行 TSX 應用程式，請將其進入點與以 `*.test.tsx` 測試主體格式撰寫的步驟（其中 `app` 代表該應用程式）從標準輸入傳入：

```sh
go tool nsgo test App.tsx < steps.ts
```
