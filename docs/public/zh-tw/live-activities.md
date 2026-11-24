---
title: 即時動態
description: 在鎖定畫面與動態島上即時追蹤進行中的事件，並於 Android 上以 Live Updates 顯示。
group: 平台功能
order: 48
---

## 啟用設定

即時動態（Live Activity）用於即時追蹤正在進行的事件，例如外送進度或計時器：在 iOS 16.1 以上的 iPhone 上會顯示於鎖定畫面與動態島中，在 Android 16 上則會以 Live Update 的形式顯示於通知面板頂端與狀態列膠囊標籤（chip）中；較舊版本的 Android 則會顯示持續性通知。請在 `nsgo.yaml` 中啟用此功能：

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## 啟動、更新與結束

`widget.StartActivity` 會啟動一個指定類型的即時動態（`kind` 用於命名追蹤的對象，寫法與小工具類型相同），並顯示一個 `widget.ActivityContent`。`Activity.Update` 會替換顯示的內容，`Activity.Alert` 除更新內容外還會發出提醒通知使用者，而 `Activity.End` 則會結束該動態（若傳入非 nil 的最終內容則會一併更新），並依照 `Dismissal` 設定決定何時從鎖定畫面移除。`widget.Activities` 會列出應用程式目前執行中的所有即時動態（包含先前執行期間啟動的動態），而 `widget.ActivitiesEnabled` 則回報使用者與平台是否允許此功能。

```go
a, err := widget.StartActivity("delivery", widget.ActivityContent{
	Title: "Your order", Body: "On its way", Symbol: "box.truck.fill",
	Timer: arrival, ShowProgress: true, Progress: 0.4,
	Actions: []widget.ActivityAction{{ID: "pause", Title: "Pause", Symbol: "pause.fill"}},
})
if err != nil {
	return
}
a.Update(widget.ActivityContent{Title: "Your order", Body: "Arrived", Status: "Now"})
a.End(nil, widget.DismissAt(time.Now().Add(2*time.Hour)))
```

在未建置此功能的應用程式中，`StartActivity` 會回報 `widget.ErrNoActivities`；在 iOS 16.1 與 Android 以外的平台上會回報 `widget.ErrActivitiesUnsupported`；當使用者關閉該權限時則會回報 `widget.ErrActivitiesDisabled`。一旦即時動態結束後，`Update` 與 `End` 會回報 `widget.ErrActivityEnded`。單一即時動態的內容大小上限為 4 KB，這是 ActivityKit 與 APNs 所規定的限制。

## 顯示內容

兩個平台皆可顯示標題與內文、狀態或由系統自動計時的計時器、進度條、最多三個操作按鈕以及強調色。iOS 會透過小工具擴充功能來繪製畫面，並接受 `widget` 的檢視來取代鎖定畫面及動態島各個區域的預設內容：包含 `LockScreen`、展開後的 `Leading`、`Trailing`、`Center` 與 `Bottom`、精簡動態島中的 `CompactLeading` 與 `CompactTrailing`，以及多個動態同時存在時的 `Minimal`。由於 Live Updates 不支援自訂應用程式檢視，Android 僅會搭配應用程式圖示顯示這些資料欄位。

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` 指定輕觸即時動態時開啟應用程式的連結，`Stale` 指定 iOS 何時將內容標示為過時，而 `Background` 則用於填滿鎖定畫面上即時動態的背景。

## 按下操作

`widget.OnAction` 會執行操作按鈕或 `widget.Button` 被按下時的處理函式，並傳入對應的即時動態與操作 ID（若是[小工具按鈕](widgets.md#按鈕)，則 `Activity` 為 nil）。請在 `main` 中、呼叫 `nsgo.Run` 之前設定此處理函式：在 iOS 上，按下按鈕時系統會在背景啟動應用程式而不載入其檢視，並保留該即時動態直到處理函式返回為止，因此處理函式可直接於該處更新動態；在 Android 上，按下事件會透過執行階段接收器直接送達執行中的 Go 行程，無須開啟其 Activity。操作按鈕需要發佈它們的引擎保持執行；該引擎結束後，過時的操作將不會有任何作用，但輕觸通知本身仍會開啟應用程式。該處理函式執行於系統的 goroutine 上，而非應用程式檢視的 goroutine，若要操作檢視狀態，需透過 `core.Scope` 的 `Post` 派發。

```go
func main() {
	widget.OnAction(func(p widget.ActivityPress) {
		if p.Action == "pause" {
			p.Activity.Update(paused)
		}
	})
	nsgo.Run(Root)
}
```

在 iOS 17 及以上版本中，即時動態內的檢視也可以觸發操作：`widget.Button{Action: "pause"}` 會執行相同的處理函式而無須開啟應用程式，且該處原本輕觸僅會開啟應用程式，改用按鈕後只會顯示其子項目並執行動作。

## 推播更新

應用程式在背景時，僅於仍在執行期間能自行更新即時動態，因此若需追蹤遠端發生的狀態變化，請在啟動時傳入 `widget.ActivityOptions{Push: true}`，將 `widget.OnPushToken` 收到的權杖傳送至伺服器，並由伺服器發送 `content-state` 為 `widget.ActivityState` 所產生內容的 APNs `liveactivity` 推播更新（推播中不可包含圖片）。自 iOS 17.2 起，應用程式也會取得一組權杖，可搭配 `widget.ActivityAttributesType` 屬性類型以及指定類型與 ID 的屬性資料，透過推播直接啟動即時動態。使用推播功能需要在應用程式的描述檔中啟用推播通知授權（entitlement）。

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## 限制

系統會在八小時後自動結束即時動態。`widget.DismissDefault` 會在 iOS 上將已結束的動態保留於鎖定畫面最長四小時、在 Android 上則立即移除；`widget.DismissNow` 會立即移除，而 `widget.DismissAt` 則可指定於四小時內的特定時間移除。Android 本身需要 `POST_NOTIFICATIONS` 與 `POST_PROMOTED_NOTIFICATIONS` 權限：請先呼叫 `cx.RequestNotificationPermission` 向使用者請求權限（[通知](notifications.md)）。Gallery 中的 Widgets 示範項目與 `examples/widgets` 均包含可執行的範例。

## 下一步

- [小工具](widgets.md)：由同一個擴充功能繪製的主畫面與鎖定畫面時間軸小工具。
- [通知](notifications.md)：Android 必須先向使用者請求的通知權限。
- [建置與發佈](building.md)：小工具擴充功能與所需的簽署授權設定。
