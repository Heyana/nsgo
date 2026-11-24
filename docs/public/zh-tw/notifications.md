---
title: 通知
description: 請求顯示通知的權限、顯示、排程與關閉通知、計算數量與在應用程式圖示上顯示任務進度，以及接收點擊事件。
group: 平台功能
order: 45
---

## 權限

`cx.RequestNotificationPermission` 會向使用者請求允許應用程式顯示通知，在首次呼叫時顯示系統提示視窗，並回傳 `core.NotificationPermission`：`NotificationsGranted`、`NotificationsDenied`，或在該平台或建置不提供通知功能時回傳 `NotificationsUnavailable`。請在使用者執行需要通知的操作時再提出請求：iOS 與 Android 僅會提示詢問一次，而瀏覽器會拒絕並非由使用者操作觸發的請求提示。`cx.NotificationPermission` 則會在不提示使用者的情況下直接讀取目前結果。

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## 顯示通知

`cx.ShowNotification` 會顯示一則通知（即使應用程式正位於前景也會顯示），並取代具有相同 `ID` 的既有通知；設定 `Silent` 則會省略系統提示音與震動。它會透過 `done` 回傳系統是否已接收該通知（若無權限則為 false）；若通知缺少 ID 或標題，則會回傳 `core.ErrNotification` 且不會呼叫 `done`。`cx.CloseNotification` 會移除指定 ID 的通知。

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` 會移除應用程式所顯示的所有通知，並取消已排程的通知。在 macOS、iOS、watchOS、Windows 與 Android 上，這涵蓋先前執行期間留在通知中心、重要訊息中心或通知面板中的通知；在 Linux 與網頁上則僅限於本次執行所顯示或排程的通知。當應用程式回到前景且視窗已呈現通知內容時，可以呼叫此函式：

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## 排程通知

若通知的 `At` 晚於目前時間，該通知便會進入排程；再次以相同 `ID` 呼叫 `cx.ShowNotification` 可調整排程時間，而在顯示前呼叫 `cx.CloseNotification` 則可取消該排程。設定 `Repeat` 會依據裝置時區，從下一個符合的時間點開始，於每日的 `At` 相同時刻，或每週的相同星期與時刻重複顯示。`cx.Env().CanScheduleNotifications()` 會回報平台是否支援通知排程：封裝為應用程式套件（bundle）的 macOS、iOS 與 watchOS 應用程式，以及 Android 與 Windows 皆支援原生排程，無論應用程式是否正在執行（Windows 會在應用程式執行期間串接每日與每週重複排程，並在啟動時重新設定排程）；Linux 與網頁（在 `Notification` 可用的情況下）則僅在應用程式或頁面執行期間保留排程。在其他平台（僅在通知中心顯示標記的 tvOS，以及小程式）上，`done` 會回傳 false。

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

在手機進入休眠（Doze）模式時，Android 可能會比排程時間延遲數分鐘才顯示通知；若應用程式需要精準到分鐘的觸發（例如鬧鐘），請在 `android.permissions` 中列出 `android.permission.SCHEDULE_EXACT_ALARM`（自 Android 12 起需由使用者在系統設定中授予），或使用 `USE_EXACT_ALARM`（Google Play 僅允許鬧鐘與行事曆應用程式使用）。被強制停止的 Android 應用程式，其通知會延後至下次啟動時才顯示。若要測試行程終止後的通知送達，請先等待 `ShowNotification` 回傳 true，再終止其行程（但不要強制停止整個套件）。

## 點擊事件

只要設定監聽的元件仍處於存活狀態，`cx.OnNotificationPress` 就會在使用者點擊通知時以該通知的 `ID` 執行其回呼函式，並將應用程式帶至前景。若該次點擊啟動了應用程式，事件會送達第一個註冊的處理常式。由於點擊通知可能會重新啟動應用程式，請將點擊後應開啟的目標資訊保留在 `ID` 中（例如 `"chat:42"`）。在 Windows 上，只有透過安裝程式將捷徑放入開始功能表的應用程式才能以此方式重新啟動；以其他方式執行的應用程式則僅能在執行期間接收點擊事件。

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

自 Android 13 起，需要請求通知權限的 Android 應用程式須在 `nsgo.yaml` 的 `android.permissions` 中列出 `android.permission.POST_NOTIFICATIONS`。

## 標記

`cx.SetBadge` 會在應用程式圖示上顯示計數標記：適用於 macOS 的 Dock、iOS 的主畫面（需要通知權限）、Windows 的工作列按鈕、相容於 Unity 的 Linux 啟動器，以及已安裝的網頁應用程式。傳入 0 或更小的數值即可清除標記。macOS 應用程式在尚未檢查或請求通知權限，也尚未顯示通知之前，設定 Dock 標記並不需要權限；但一旦執行過上述任一操作，在該次與後續的執行中，Dock 只會在使用者於「系統設定」中允許該應用程式顯示標記時才會顯示（呼叫 `cx.RequestNotificationPermission` 時會連同提示一併請求此權限）。不具備 Linux 標記介面的啟動器以及 Android 無法直接設定數字；Android 啟動器會在應用程式有通知顯示時自動加上標記。

```go
cx.SetBadge(len(unread.Get()))
```

## 任務進度

`cx.SetTaskProgress` 會在應用程式圖示上顯示長時間任務的進度：在 macOS 上顯示於 Dock 圖示上方的進度列、在 Windows 上顯示於工作列按鈕，亦支援相容於 Unity 的 Linux 啟動器。此功能不需任何權限，且可與標記同時顯示。`TaskProgressNormal` 會顯示介於 0 到 1 之間的小數 `Value`；`TaskProgressPaused` 與 `TaskProgressError` 會在 Windows 與 macOS 上為進度列著色；`TaskProgressIndeterminate` 在 Windows 上會呈現動態進度，在 macOS 與 Linux 上則顯示為靜止的半滿進度列；傳入零值 `TaskProgress` 即可清除進度顯示。在 Windows 上，進度會顯示於該作用域所屬視窗的工作列按鈕上；若設定於不具備獨立按鈕的作用域（例如應用程式本身的作用域或系統匣面板），則會顯示在所有未自行設定進度的應用程式視窗上。iOS、Android 與網頁平台不會顯示任何內容。

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## 下一步

- [權限](permissions.md)：系統的其他權限，例如麥克風與輔助使用。
- [觸覺回饋](haptics.md)：裝置在使用者指尖下播放的觸覺回饋。
- [平台服務](services.md)：警告對話框、剪貼簿、檔案、儲存空間與網路連線。
- [即時動態](live-activities.md)：應用程式在鎖定畫面與動態島上持續追蹤的動態資訊。
- [建置與發佈](building.md)：封裝檔所需的各項權限設定。
