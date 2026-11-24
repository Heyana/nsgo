---
title: Quyền
description: Kiểm tra và xin các quyền của hệ thống, mở phần cài đặt của chúng và hướng dẫn người dùng đến System Settings trên macOS.
group: Tính năng nền tảng
order: 49
---

## Kiểm tra và xin quyền

Hệ thống giữ lại một số việc mà ứng dụng có thể làm để người dùng tự cấp: micrô, camera, danh bạ, lịch, lời nhắc, ảnh, Bluetooth, thư viện phương tiện, vị trí và thông báo trên điện thoại và máy Mac, chuyển động và vị trí ở chế độ nền trên điện thoại, và trên macOS còn có Accessibility, Screen Recording, Input Monitoring, Full Disk Access, App Management, Developer Tools và Automation đối với một ứng dụng khác ([vị trí và chuyển động](services.md#location-and-motion), [thông báo](notifications.md)). `core.Permissions()` liệt kê chúng dưới dạng các khóa `core.Permission`. `cx.PermissionStatus` trả lời ứng dụng có một quyền hay không mà không hỏi, còn `cx.RequestPermission` hỏi bằng lời nhắc của hệ thống ở nơi có lời nhắc và chưa hỏi lần nào. Hãy xin quyền khi người dùng làm một việc cần đến quyền đó. Face ID và Touch ID không nằm trong số này: `cx.BiometricAvailability` cho biết về chúng và `cx.AuthenticateBiometric` hỏi bằng lời nhắc riêng của chúng ([sinh trắc học](services.md#biometrics)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

Trên macOS, Automation được cấp theo từng ứng dụng đích: truyền `core.PermissionAutomationOf("com.apple.finder")` để kiểm tra hoặc xin quyền đối với một ứng dụng đích đang chạy theo bundle ID của nó, hoặc `core.PermissionAutomation` để mở ngăn Automation trong System Settings. Một ứng dụng đích không đang chạy sẽ trả lời `PermissionUnknown`.

| Trạng thái | `String()` | Ý nghĩa |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Chưa hỏi: một yêu cầu sẽ hiển thị lời nhắc của hệ thống |
| `PermissionGranted` | `granted` | Ứng dụng có quyền |
| `PermissionLimited` | `limited` | Truy cập một phần, như ảnh hoặc danh bạ được chọn |
| `PermissionDenied` | `denied` | Bị từ chối; chỉ cài đặt của hệ thống mới thay đổi được |
| `PermissionUnknown` | `unknown` | Bị giữ lại, nhưng hệ thống hoặc nền tảng không cho biết |
| `PermissionNotGated` | `not-gated` | Không có gì ngăn ứng dụng, như trên Linux |
| `PermissionUnavailable` | `unavailable` | Nền tảng không có thứ đó |

`Allowed` là true với granted, limited và not gated. macOS trả lời Accessibility, Screen Recording và Input Monitoring là denied cho đến khi người dùng bật chúng, vì việc xin quyền chỉ đưa ứng dụng vào danh sách trong ngăn của chúng, và chỉ báo việc cấp Screen Recording khi ứng dụng khởi động lại.

## Mô tả mục đích sử dụng

Trên macOS và iOS, việc xin quyền cần lý do của quyền đó trong Info.plist của ứng dụng, nơi `apple.info` của `nsgo.yaml` lưu giữ; nếu thiếu, yêu cầu sẽ trả lời unavailable và ghi log khóa đó, vì hệ thống sẽ kết thúc ứng dụng. Một thông điệp trong [catalog](localization.md) của ứng dụng có cùng khóa, như `NSCameraUsageDescription: Scans the codes you point it at.` trong `locales/en.yaml`, sẽ dịch lý do, và hệ thống khi đó hỏi bằng ngôn ngữ của người dùng; ngôn ngữ nào không dịch nó sẽ dùng lý do của ngôn ngữ nguồn.

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

Trên Android, mỗi quyền phải được liệt kê trong `android.permissions` của `nsgo.yaml`; quyền nào bị bỏ ra khỏi manifest sẽ trả lời `PermissionUnavailable`. Dưới `nsgo dev`, macOS tính những gì ứng dụng làm cho terminal đã khởi động nó, nên chính terminal là ứng dụng được hỏi và được liệt kê; hãy build ứng dụng để kiểm thử các quyền của chính nó.

## Mở phần cài đặt

`cx.OpenPermissionSettings` mở trang cấp một quyền và trả lời có trang nào được mở hay không: Privacy & Security tại ngăn của quyền đó (bao gồm Automation, Contacts, Photos và Location Services) hoặc Notifications trên macOS, trang của ứng dụng trong Settings (hoặc trang Notifications của nó từ iOS 16) trên iOS, trang của ứng dụng trong Settings trên Android, và trang micrô, camera hoặc thông báo trên Windows.

Với `PermissionLocation` và `PermissionBackgroundLocation`, macOS mở Location Services; `PermissionMotion` mở Motion & Fitness, dù macOS không cung cấp mẫu dữ liệu chuyển động. iOS và Android mở trang của ứng dụng cho cả ba. Mở phần cài đặt không xin cũng không cấp quyền.

## Hướng dẫn người dùng

`desktop.GuidePermission` mở trang đó và chờ người dùng cấp quyền. Trên macOS, một bảng điều khiển đi theo cửa sổ của System Settings mà không chiếm tiêu điểm và, với những ngăn có danh sách nhận ứng dụng được kéo vào, giữ sẵn ứng dụng để người dùng kéo vào đó. Nó kết thúc, và hàm của nó nhận được trạng thái, khi ứng dụng có quyền, người dùng đóng bảng điều khiển hoặc System Settings, `stop` được gọi hoặc thành phần biến mất; một hướng dẫn mới sẽ kết thúc hướng dẫn trước đó.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

Ứng dụng được giữ sẵn là bundle của nó, nếu không thì là tệp thực thi của nó; `GuideOptions.App` chỉ định một ứng dụng khác và `Name` là tên bảng điều khiển gọi nó. Windows mở trang đó và chờ Settings đóng lại. Ở những nơi khác, hàm nhận được trạng thái ngay lập tức.

## Nền tảng

| Nền tảng | Những gì nó cho biết và hỏi |
| --- | --- |
| macOS | Mọi quyền (Automation theo từng ứng dụng đích đang chạy qua `PermissionAutomationOf`, cùng với thông báo cho ứng dụng dạng bundle); Automation thuần, App Management, Developer Tools và Media & Apple Music là unknown; chuyển động là unavailable |
| iOS | Micrô, camera, danh bạ, lịch, lời nhắc, ảnh (với `PermissionLimited`), Bluetooth, thư viện phương tiện, vị trí, chuyển động và thông báo |
| Windows | Micrô, camera và thông báo, những quyền mà nó không bao giờ hiển thị lời nhắc; phần còn lại không bị kiểm soát (not gated) |
| Linux | Không bị kiểm soát bên ngoài sandbox; thông báo phản ánh việc máy chủ thông báo có phản hồi hay không; trong Flatpak và Snap, camera thông qua desktop portal và micrô thông qua portal và PipeWire |
| Web | Micrô và camera (unknown trên Firefox), thông báo, vị trí (không gồm vị trí nền) và chuyển động trên trình duyệt di động |
| Android | Micrô, camera, lịch, Bluetooth và thư viện phương tiện, ở nơi `android.permissions` khai báo chúng |
| Apple Watch, mini program | Thông báo, vị trí và chuyển động thông qua các dịch vụ của chúng; phần còn lại do hệ thống giữ hiện là unknown |

Quyền vị trí và chuyển động tuân theo [cảm biến của từng nền tảng](services.md#location-and-motion), và `PermissionNotifications` tuân theo [thông báo](notifications.md). Fitness dùng `PermissionActivityRecognition` (`ACTIVITY_RECOGNITION` trên Android, `NSMotionUsageDescription` trên Apple), bao gồm cả áp suất trên Apple; chuyển động thô trên Android vẫn tách riêng. Trên Safari di động, hãy xin `PermissionMotion` từ một trình xử lý lần nhấn.

## Kiểm thử

`nsgotest` trả lời từ mô hình của nó: `app.Permissions` lưu trạng thái của từng quyền, ban đầu là chưa hỏi, và một yêu cầu đối với quyền chưa hỏi sẽ đặt nó thành giá trị của `app.PermissionAnswers` cho quyền đó, nếu không thì là granted. `PermissionNotifications` luôn đồng bộ với `app.NotificationPermission` và `app.NotificationAnswer`. `app.PermissionSettings` liệt kê các trang cài đặt đã mở và `app.Guide` là hướng dẫn đang hiển thị, mà `app.GrantPermission` và `app.CloseGuide` sẽ kết thúc.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Tiếp theo

- [Thông báo](notifications.md): quyền riêng của chúng, huy hiệu và lần nhấn.
- [Build và phát hành](building.md): các entitlement và capability mà một gói khai báo.
