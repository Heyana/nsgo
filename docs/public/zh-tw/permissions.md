---
title: 權限
description: 檢查並請求系統權限、開啟對應設定頁面，以及在 macOS 上引導使用者前往「系統設定」。
group: 平台功能
order: 49
---

## 檢查與請求

系統會將應用程式的部分功能保留給使用者決定是否授權：在手機與 Mac 上包含麥克風、相機、聯絡人、行事曆、提醒事項、照片、藍牙、媒體資料庫、定位與通知；在手機上還包含動作與背景定位；在 macOS 上更包含輔助使用、螢幕錄製、輸入監控、完全取用磁碟、App 管理、開發者工具，以及對其他應用程式的自動化控制（參閱[定位與動作](services.md#location-and-motion)、[通知](notifications.md)）。`core.Permissions()` 會將這些項目列為 `core.Permission` 鍵值。`cx.PermissionStatus` 可在不詢問使用者的情況下回報應用程式是否已取得指定權限，而 `cx.RequestPermission` 則會在系統支援提示且尚未詢問過時跳出系統請求視窗。請在使用者執行需要該權限的操作時再提出請求。Face ID 與 Touch ID 不屬於此列：請改用 `cx.BiometricAvailability` 查詢可用性，並透過 `cx.AuthenticateBiometric` 以其專屬提示進行驗證（參閱[生物辨識](services.md#biometrics)）。

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

在 macOS 上，「自動化」權限是針對個別目標應用程式授予的：傳入 `core.PermissionAutomationOf("com.apple.finder")` 可依套件識別碼（bundle ID）檢查或請求正在執行中的目標應用程式權限，而傳入 `core.PermissionAutomation` 則可開啟「系統設定」中的「自動化」面板。若目標應用程式目前未執行，則會回傳 `PermissionUnknown`。

| 狀態 | `String()` | 意義 |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | 尚未詢問：提出請求時會顯示系統提示視窗 |
| `PermissionGranted` | `granted` | 應用程式已取得此權限 |
| `PermissionLimited` | `limited` | 部分存取權，例如僅限所選照片或聯絡人 |
| `PermissionDenied` | `denied` | 已被拒絕；僅能透過系統設定更改 |
| `PermissionUnknown` | `unknown` | 受系統管控，但系統或平台未回報其狀態 |
| `PermissionNotGated` | `not-gated` | 沒有任何限制阻擋應用程式使用，例如在 Linux 上 |
| `PermissionUnavailable` | `unavailable` | 該平台不具備此功能或權限 |

對於 granted、limited 與 not gated 狀態，`Allowed` 皆為 true。在 macOS 上，輔助使用、螢幕錄製與輸入監控在使用者手動開啟之前都會回傳 denied，因為呼叫請求只會將應用程式加入對應的設定面板列表中；且螢幕錄製權限在應用程式重新啟動後才會回報為已授權。

## 用途說明

在 macOS 與 iOS 上，請求權限必須在應用程式的 Info.plist 中提供用途說明，這部分資訊存放於 `nsgo.yaml` 的 `apple.info` 中；若缺少該說明，請求會直接回傳 unavailable 並記錄該鍵值，因為未提供時系統會強制終止應用程式。在應用程式的[翻譯目錄](localization.md)中加入相同鍵值的訊息（例如在 `locales/en.yaml` 中設定 `NSCameraUsageDescription: Scans the codes you point it at.`）即可翻譯該說明文字，讓系統以使用者的語言提出請求；若某種語言未提供翻譯，則會顯示來源語言的說明文字。

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # macOS Automation
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+; BLUETOOTH on Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+; READ_EXTERNAL_STORAGE on Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

在 Android 上，每一項權限都必須列於 `nsgo.yaml` 的 `android.permissions` 中；未列在清單（manifest）中的權限會回傳 `PermissionUnavailable`。在 `nsgo dev` 模式下執行時，macOS 會將應用程式的操作歸屬於啟動它的終端機，因此系統會向該終端機請求權限並將其列於設定清單中；請直接建置應用程式來測試應用程式自身的權限設定。

## 開啟設定頁面

`cx.OpenPermissionSettings` 會開啟用於授予該權限的設定頁面，並回傳是否成功開啟：在 macOS 上會開啟「隱私權與安全性」下對應權限的面板（包含自動化、聯絡人、照片與定位服務）或「通知」頁面；在 iOS 上會開啟「設定」中的應用程式頁面（自 iOS 16 起則開啟其「通知」頁面）；在 Android 上會開啟「設定」中的應用程式頁面；在 Windows 上則會開啟麥克風、相機或通知的設定頁面。

對於 `PermissionLocation` 與 `PermissionBackgroundLocation`，macOS 會開啟「定位服務」；對於 `PermissionMotion`，即使 macOS 不提供動作感測資料，仍會開啟「動作與健身」。iOS 與 Android 對這三項權限皆會開啟應用程式自身的設定頁面。開啟設定頁面本身既不會提出權限請求，也不會直接授予權限。

## 引導使用者

`desktop.GuidePermission` 會開啟上述設定頁面並等待使用者授予權限。在 macOS 上，會有一個浮動面板在不搶走視窗焦點的情況下跟隨「系統設定」視窗；若該面板的清單支援透過拖曳加入應用程式，面板上也會顯示應用程式圖示供使用者直接拖曳過去。當應用程式取得權限、使用者關閉面板或「系統設定」、呼叫了 `stop` 函式，或是所屬元件遭到卸載時，引導流程便會結束，並透過回呼函式通知最新狀態；啟動新的引導流程也會結束前一個引導。

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

面板所提供拖曳的項目預設為應用程式封裝檔（若無則為執行檔）；`GuideOptions.App` 可指定其他應用程式路徑，而 `Name` 則決定面板上顯示的名稱。Windows 會開啟設定頁面並等待「設定」關閉。在其他平台上，回呼函式會立即收到目前的權限狀態。

## 各平台支援

| 平台 | 回報與請求範圍 |
| --- | --- |
| macOS | 所有權限（透過 `PermissionAutomationOf` 針對執行中的目標應用程式處理自動化權限，封裝後的應用程式另支援通知權限）；未指定目標的自動化、App 管理、開發者工具與「媒體與 Apple Music」回傳 unknown；動作權限為 unavailable |
| iOS | 麥克風、相機、聯絡人、行事曆、提醒事項、照片（支援 `PermissionLimited`）、藍牙、媒體資料庫、定位、動作與通知 |
| Windows | 麥克風、相機與通知（不會主動跳出提示視窗）；其餘項目皆為未受管制（not gated） |
| Linux | 沙盒外不受管制；通知權限反映通知伺服器是否有回應；在 Flatpak 與 Snap 環境中，相機透過桌面入口（desktop portal）處理，麥克風則透過桌面入口與 PipeWire 處理 |
| Web | 麥克風與相機（在 Firefox 中為 unknown）、通知、定位（不含背景定位），以及行動裝置瀏覽器上的動作權限 |
| Android | 麥克風、相機、行事曆、藍牙與媒體資料庫（前提是已在 `android.permissions` 中宣告） |
| Apple Watch、小程式 | 透過各自的服務處理通知、定位與動作；其餘受系統管控的權限目前皆回傳 unknown |

定位與動作權限遵循各[平台的感測器](services.md#location-and-motion)支援情況，而 `PermissionNotifications` 則遵循[通知](notifications.md)的規範。體適能活動辨識使用 `PermissionActivityRecognition`（在 Android 上為 `ACTIVITY_RECOGNITION`，在 Apple 平台上為 `NSMotionUsageDescription`），包含 Apple 的氣壓感測資料；Android 的原始動作感測資料則仍分開處理。在行動版 Safari 上，請於點擊事件處理常式中請求 `PermissionMotion`。

## 測試

`nsgotest` 會依據其測試模型回傳結果：`app.Permissions` 保存各項權限狀態，初始值皆為尚未詢問；請求尚未詢問的權限時，會將其設為 `app.PermissionAnswers` 中對應的設定值，若未設定則預設為已授權。`PermissionNotifications` 會與 `app.NotificationPermission` 及 `app.NotificationAnswer` 保持同步。`app.PermissionSettings` 會記錄已開啟的設定頁面，而 `app.Guide` 則記錄目前顯示中的引導面板，並可透過 `app.GrantPermission` 與 `app.CloseGuide` 結束。

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## 下一步

- [通知](notifications.md)：通知專屬權限、圖示標記與點擊事件。
- [建置與發佈](building.md)：封裝檔所宣告的授權（entitlements）與功能（capabilities）。
