---
title: Live Activities
description: Theo dõi một sự việc khi nó đang diễn ra trên Màn hình khóa và trong Dynamic Island, và dưới dạng Live Updates trên Android.
group: Tính năng nền tảng
order: 48
---

## Bật tính năng

Một Live Activity theo dõi một sự việc khi nó đang diễn ra, như một đơn giao hàng hay một bộ hẹn giờ: trên Màn hình khóa và trong Dynamic Island của iPhone từ iOS 16.1, và dưới dạng Live Update trên Android 16, ở đầu ngăn thông báo và trong một chip của thanh trạng thái; các phiên bản Android cũ hơn hiển thị một thông báo đang diễn ra (ongoing notification). Bật tính năng này trong `nsgo.yaml`:

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## Bắt đầu, cập nhật và kết thúc

`widget.StartActivity` bắt đầu một activity thuộc một loại (kind), tên loại cho biết nó theo dõi điều gì và được viết giống như các loại widget, hiển thị một `widget.ActivityContent`. `Activity.Update` thay thế nội dung nó hiển thị, `Activity.Alert` cũng làm vậy và đồng thời cảnh báo người dùng, còn `Activity.End` kết thúc nó, với nội dung cuối cùng nếu khác nil, và rời khỏi Màn hình khóa theo những gì `Dismissal` quy định. `widget.Activities` liệt kê các activity đang chạy của ứng dụng, bao gồm cả những activity từ các lần chạy trước của ứng dụng, và `widget.ActivitiesEnabled` cho biết người dùng và nền tảng có cho phép chúng hay không.

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

`StartActivity` báo `widget.ErrNoActivities` trong một ứng dụng được build không kèm tính năng này, `widget.ErrActivitiesUnsupported` trên các nền tảng khác ngoài iOS 16.1 và Android, và `widget.ErrActivitiesDisabled` khi người dùng đã tắt chúng; `Update` và `End` báo `widget.ErrActivityEnded` khi activity đã kết thúc. Nội dung của một activity tối đa là 4 KB, đúng như giới hạn mà ActivityKit và APNs chấp nhận.

## Nội dung hiển thị

Cả hai nền tảng đều hiển thị tiêu đề và nội dung, trạng thái hoặc một bộ đếm giờ do hệ thống tự chạy, một thanh tiến trình, tối đa ba hành động và màu nhấn. iOS vẽ chúng bằng widget extension, và nhận các view của `widget` thay cho chúng trên Màn hình khóa và ở từng phần của Dynamic Island: `LockScreen`, `Leading`, `Trailing`, `Center` và `Bottom` khi mở rộng, `CompactLeading` và `CompactTrailing` trong island thu gọn, và `Minimal` khi nhiều activity cùng chia sẻ nó. Android chỉ hiển thị các trường dữ liệu, kèm biểu tượng của ứng dụng, vì Live Updates không nhận view của ứng dụng.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` mở cùng ứng dụng khi người dùng chạm vào activity, `Stale` là thời điểm iOS hiển thị nội dung như đã lỗi thời, và `Background` tô nền activity trên Màn hình khóa.

## Thao tác nhấn

`widget.OnAction` chạy trình xử lý cho thao tác nhấn vào một hành động hoặc một `widget.Button`, cùng với activity và ID của hành động (`Activity` là nil đối với một [nút widget](widgets.md#buttons)). Hãy đặt nó trong `main`, trước `nsgo.Run`: trên iOS, hệ thống khởi động ứng dụng ở chế độ nền cho một thao tác nhấn, không kèm các view của nó, và giữ activity cho đến khi trình xử lý trả về, nên trình xử lý cập nhật activity ngay tại đó; trên Android, một thao tác nhấn đến được tiến trình Go đang chạy thông qua runtime receiver mà không mở Activity của nó. Các hành động cần engine đã đăng chúng vẫn còn đang chạy; một hành động cũ sẽ không làm gì sau khi engine đó kết thúc. Bản thân thông báo vẫn mở ứng dụng khi được chạm vào. Trình xử lý chạy trên một goroutine của hệ thống, không phải của các view của ứng dụng; nó tiếp cận các view đó thông qua `Post` của một `core.Scope`.

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

Một view của activity cũng làm được điều này, trên iOS 17 trở lên: `widget.Button{Action: "pause"}` chạy cùng trình xử lý đó mà không mở ứng dụng, và chỉ hiển thị các phần tử con của nó ở những nơi mà thao tác nhấn chỉ mở ứng dụng.

## Cập nhật qua push

Một ứng dụng ở chế độ nền chỉ cập nhật activity của nó khi đang chạy, vì vậy ứng dụng theo dõi những sự việc diễn ra ở nơi khác sẽ bắt đầu activity với `widget.ActivityOptions{Push: true}`, gửi token mà `widget.OnPushToken` nhận được lên máy chủ của mình, và để máy chủ đẩy các bản cập nhật APNs `liveactivity` có `content-state` là kết quả của `widget.ActivityState` cho nội dung đó: không có hình ảnh nào được đưa vào một push. Từ iOS 17.2, ứng dụng cũng nhận được một token để bắt đầu activity qua push, với kiểu thuộc tính `widget.ActivityAttributesType` và các thuộc tính gồm loại và id. Push cần entitlement thông báo đẩy trong profile của ứng dụng.

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## Giới hạn

Hệ thống kết thúc một activity sau tám giờ. `widget.DismissDefault` giữ một activity đã kết thúc trên Màn hình khóa tối đa bốn giờ trên iOS và gỡ nó ngay lập tức trên Android, `widget.DismissNow` gỡ nó ngay lập tức và `widget.DismissAt` gỡ vào một thời điểm trong vòng bốn giờ. Android tự yêu cầu `POST_NOTIFICATIONS` và `POST_PROMOTED_NOTIFICATIONS`: hãy hỏi người dùng bằng `cx.RequestNotificationPermission` trước ([Thông báo](notifications.md)). Bản demo Widgets của gallery và `examples/widgets` có chạy một activity.

## Tiếp theo

- [Widget](widgets.md): các dòng thời gian của Màn hình chính và Màn hình khóa, được vẽ bởi cùng một extension.
- [Thông báo](notifications.md): quyền mà Android yêu cầu trước.
- [Build và phát hành](building.md): extension và các entitlement mà nó cần.
