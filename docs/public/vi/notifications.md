---
title: Thông báo
description: Xin quyền hiển thị thông báo, hiển thị, lên lịch và đóng chúng, đếm chúng và hiển thị tiến độ của một tác vụ trên biểu tượng của ứng dụng, đồng thời nhận các lần nhấn.
group: Tính năng nền tảng
order: 45
---

## Quyền

`cx.RequestNotificationPermission` xin người dùng cho phép ứng dụng hiển thị thông báo, với lời nhắc của hệ thống ở lần đầu tiên, và trả lời bằng một `core.NotificationPermission`: `NotificationsGranted`, `NotificationsDenied`, hoặc `NotificationsUnavailable` ở nơi nền tảng hoặc bản build không hiển thị thông báo nào. Hãy xin quyền khi người dùng làm một việc cần đến thông báo: iOS và Android chỉ hỏi một lần, còn trình duyệt từ chối lời nhắc không do người dùng gây ra. `cx.NotificationPermission` đọc câu trả lời mà không hỏi.

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## Hiển thị một thông báo

`cx.ShowNotification` hiển thị một thông báo, kể cả khi ứng dụng đang ở phía trước, và thay thế thông báo có cùng `ID`; `Silent` bỏ âm thanh và rung của hệ thống. Nó trả lời `done` cho biết hệ thống có nhận thông báo hay không, là false nếu không có quyền, và trả về `core.ErrNotification`, không gọi `done`, cho thông báo không có ID hoặc tiêu đề. `cx.CloseNotification` gỡ thông báo mà một ID chỉ định.

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` gỡ mọi thông báo ứng dụng đã hiển thị và hủy những thông báo nó đã lên lịch. Trên macOS, iOS, watchOS, Windows và Android, điều đó bao gồm cả những thông báo mà các lần chạy trước để lại trong Notification Center, Action Center hoặc ngăn thông báo; trên Linux và web, nó chỉ đến được những thông báo mà lần chạy này đã hiển thị hoặc lên lịch. Hãy gọi nó khi ứng dụng lên phía trước, lúc cửa sổ của nó hiển thị điều mà các thông báo đã báo:

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## Lên lịch một thông báo

Một thông báo có `At` muộn hơn hiện tại sẽ được lên lịch, và gọi lại `cx.ShowNotification` với `ID` của nó sẽ dời nó, còn `cx.CloseNotification` hủy nó trước khi hiển thị. `Repeat` hiển thị nó mỗi ngày vào giờ trong ngày của `At`, hoặc mỗi tuần vào cả thứ trong tuần của nó, bắt đầu từ thời điểm như vậy kế tiếp, theo múi giờ của thiết bị. `cx.Env().CanScheduleNotifications()` cho biết nền tảng có lên lịch thông báo hay không: các ứng dụng macOS, iOS và watchOS trong một bundle, Android và Windows lên lịch chúng một cách gốc dù ứng dụng có đang chạy hay không (Windows nối tiếp các lần lặp hằng ngày và hằng tuần khi ứng dụng chạy và thiết lập lại chúng khi khởi chạy), còn Linux và web (nơi có `Notification`) giữ chúng trong khi ứng dụng hoặc trang đang chạy. Ở những nơi khác (tvOS, nơi trung tâm thông báo chỉ hiển thị huy hiệu, và mini program) `done` trả lời false.

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

Android có thể hiển thị thông báo đã lên lịch muộn vài phút so với giờ của nó khi điện thoại ở chế độ ngủ (doze); một ứng dụng cần đúng từng phút, như đồng hồ báo thức, liệt kê `android.permission.SCHEDULE_EXACT_ALARM` trong `android.permissions`, quyền mà người dùng cấp trong cài đặt hệ thống từ Android 12, hoặc `USE_EXACT_ALARM`, quyền mà Google Play chỉ cho phép ứng dụng báo thức và lịch dùng. Thông báo của một ứng dụng Android bị buộc dừng (force-stop) sẽ chờ đến lần khởi động tiếp theo của nó. Để kiểm thử việc gửi thông báo sau khi tiến trình chết, hãy chờ `ShowNotification` trả lời true, rồi kết thúc các tiến trình của nó mà không buộc dừng gói.

## Lần nhấn

`cx.OnNotificationPress` chạy hàm của nó với `ID` của thông báo mà người dùng đã nhấn, trong khi thành phần đặt nó còn tồn tại, và đưa ứng dụng lên phía trước. Một lần nhấn đã khởi động ứng dụng sẽ đến trình xử lý được đăng ký đầu tiên. Hãy giữ trong `ID` thứ mà lần nhấn mở ra (`"chat:42"`), vì một lần nhấn có thể khởi động lại ứng dụng từ đầu. Trên Windows, điều đó xảy ra với ứng dụng mà trình cài đặt của nó đã đưa vào menu Start; nếu không, một lần chạy chỉ nghe thấy các lần nhấn trong khi nó đang chạy.

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Một ứng dụng Android xin quyền từ Android 13 trở đi liệt kê `android.permission.POST_NOTIFICATIONS` trong `android.permissions` của `nsgo.yaml`.

## Huy hiệu

`cx.SetBadge` đặt một con số lên biểu tượng của ứng dụng: trên Dock ở macOS, trên Màn hình chính ở iOS (cần quyền thông báo), trên nút thanh tác vụ ở Windows, trên các trình khởi chạy Linux tương thích Unity và trên ứng dụng web đã cài đặt. Con số bằng 0 hoặc nhỏ hơn sẽ xóa nó. Một ứng dụng macOS không cần quyền cho huy hiệu Dock của mình cho đến khi nó kiểm tra hoặc xin quyền thông báo hoặc hiển thị một thông báo; từ đó trở đi, trong lần chạy đó và các lần sau, Dock chỉ hiển thị huy hiệu khi người dùng cho phép huy hiệu của ứng dụng trong System Settings, điều mà `cx.RequestNotificationPermission` xin cùng với các cảnh báo. Các trình khởi chạy không có giao diện huy hiệu Linux và Android không có con số nào để đặt; trình khởi chạy Android đánh dấu ứng dụng có thông báo đang hiển thị.

```go
cx.SetBadge(len(unread.Get()))
```

## Tiến độ tác vụ

`cx.SetTaskProgress` hiển thị một tác vụ dài đã đi được bao xa trên biểu tượng của ứng dụng: một thanh trên ô Dock ở macOS, trên nút thanh tác vụ ở Windows và trên các trình khởi chạy Linux tương thích Unity. Nó không cần quyền và hiển thị bên cạnh huy hiệu. `TaskProgressNormal` hiển thị `Value`, một phân số từ 0 đến 1; `TaskProgressPaused` và `TaskProgressError` tô màu nó trên Windows và macOS; `TaskProgressIndeterminate` chuyển động trên Windows và là một thanh đứng yên, đầy một nửa trên macOS và Linux; `TaskProgress` giá trị zero sẽ xóa nó. Trên Windows, nó hiển thị trên nút của cửa sổ thuộc scope, và tiến độ được đặt ở nơi không có cửa sổ nào có nút riêng, như scope của chính ứng dụng hoặc một bảng khay hệ thống, sẽ hiển thị trên mọi cửa sổ của ứng dụng không tự đặt tiến độ. iOS, Android và web không hiển thị gì.

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## Tiếp theo

- [Quyền](permissions.md): các quyền khác của hệ thống, như micrô và Accessibility.
- [Phản hồi xúc giác](haptics.md): phản hồi mà thiết bị phát ra dưới ngón tay người dùng.
- [Dịch vụ nền tảng](services.md): cảnh báo, bảng nhớ tạm, tệp, lưu trữ và mạng.
- [Live Activities](live-activities.md): những gì ứng dụng theo dõi, trên Màn hình khóa và trong Dynamic Island.
- [Build và phát hành](building.md): các quyền mà một gói cần.
