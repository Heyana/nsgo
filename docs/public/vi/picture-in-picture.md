---
title: Hình trong hình
description: Phát video ở chế độ Picture in Picture của iOS, tvOS, macOS, Android và trình duyệt, hoặc hiển thị một cây tùy chỉnh trên iOS.
group: Tính năng nền tảng
order: 45
---

Trên iOS, hãy khai báo phát nền trước khi dùng Picture in Picture; macOS không cần chế độ nền:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

Với một video, hãy bật `PictureInPicture`, giữ một `core.Ref`, rồi bắt đầu hoặc dừng nó từ một điều khiển của ứng dụng. Theo dõi callback trạng thái để bật điều khiển đó khi `Possible` trở thành true:

```go
ref := &core.Ref{}
state := core.NewSignal(cx, core.PictureInPictureState{})
video := core.Video{
    Source: "asset:clip.mp4", Ref: ref,
    PictureInPicture: true, AutoPictureInPicture: true,
    OnPictureInPicture: state.Set,
}
start := ui.Button{
    Text: "Picture in Picture", OnPress: ref.StartPictureInPicture,
    Disabled: func() bool { return !state.Get().Possible },
}
```

`AutoPictureInPicture` chọn tự động vào PiP khi một ứng dụng iOS chuyển xuống nền; tvOS và macOS bỏ qua tùy chọn này. `Controls` vẫn hiển thị các điều khiển phát lại gốc; nút PiP của ứng dụng dùng cùng một phiên được quản lý cho một video có điều khiển. `OnPictureInPictureRestore` cho phép ứng dụng hiện trang trình phát của nó khi người dùng quay lại. Hãy giữ video được gắn (mounted) trong khi PiP đang hoạt động: xóa scope của nó sẽ dừng phát lại và PiP.

Trên Android, activity đã đóng gói vốn đã khai báo hỗ trợ PiP. Một video đi vào cửa sổ PiP của activity, phần còn lại của giao diện ứng dụng bị ẩn, và giữ nguyên trình phát cùng vị trí phát khi quay lại. `StopPictureInPicture` mở rộng lại chính activity đó. Các điều khiển phát/tạm dừng của cửa sổ báo `OnPlay` và `OnPause`, với `VideoPauseControls`; `Ref.Seek` dùng cùng trình phát đó. Việc tự động vào PiP cần thiết lập tường minh và một video đã được chuẩn bị, đang phát. Một lần bắt đầu vẫn có thể thất bại vì trạng thái hiện tại của hệ thống; hãy xử lý `Error` ngay cả khi `Possible` đã là true.

Một host Android nhúng phải khai báo `android:supportsPictureInPicture="true"` và xử lý các thay đổi cấu hình `orientation|screenSize|screenLayout|smallestScreenSize`. Chuyển tiếp `onPictureInPictureModeChanged` và `onUserLeaveHint` của activity tới các phương thức cùng tên trên `NsgoView`; trên Android 15+, cũng chuyển tiếp `PictureInPictureUiState.isTransitioningToPip()` tới `NsgoView.onPictureInPictureTransition`. Một host hoặc thiết bị không hỗ trợ sẽ báo `Supported: false`.

Trên web, hãy bắt đầu từ một lần nhấn của người dùng trong trình duyệt có API PiP video tiêu chuẩn. `Supported` cũng phản ánh Permissions Policy của trang, và `Possible` cần metadata của video đã được tải. Các điều khiển PiP riêng của trình duyệt báo cùng trạng thái hoạt động. Dừng, thay thế hoặc xóa nguồn, hoặc gỡ video sẽ kết thúc phiên của nó; một lần bắt đầu đang chờ không thể mở lại nguồn trước đó. PiP trên web chỉ hiển thị video, không có nội dung NSGo tùy chỉnh, không tự động vào khi chuyển xuống nền và không có `OnPictureInPictureRestore`. Trình duyệt không hỗ trợ sẽ báo `Supported: false`.

Trên iOS, `mobile.PictureInPicture` nhận `Children` với bố cục riêng, mặc định là 320 × 180 point. Nó không chiếm không gian nào trong ứng dụng. `Ref` của nó có cùng các phương thức bắt đầu và dừng, còn `OnState`, `OnRestore`, `OnPlay`, `OnPause` và `OnSkip` chuyển các hành động của hệ thống. Liên kết `PausedFunc` và `PositionFunc` với trạng thái phát lại mà các callback đó cập nhật; `Duration: 0` mô tả nội dung trực tiếp. Một ứng dụng bị tạm ngưng sẽ giữ khung hình được kết xuất cuối cùng.

Đặt một `core.Video` vào cây tùy chỉnh để ghép các khung hình đã giải mã của nó với phụ đề do ứng dụng vẽ hoặc các lớp phủ khác. Bắt đầu phiên bằng `Ref` bên ngoài. Dùng chung một signal tạm dừng với video, cấp `PositionFunc` của phiên tùy chỉnh từ `Video.OnProgress`, và chuyển tiếp `OnSkip` tới `Ref.Seek` của video. Âm thanh của video phát qua trình phát riêng của nó.

Cây tùy chỉnh cũng kết xuất văn bản, view, hình ảnh và Canvas. WebView, shader Metal và hiệu ứng nền (backdrop) không khả dụng trong các khung hình được chụp. Ảnh động và các chuyển tiếp layer gốc hiển thị trạng thái mô hình của chúng. Hãy dùng các giá trị hoạt ảnh tường minh cho chuyển động. Quy trình duyệt của App Store có thể từ chối những cách dùng không liên quan đến video.

`Supported: false` phân biệt một nền tảng không khả dụng với một phiên được hỗ trợ nhưng chưa sẵn sàng hoặc có `Error` giải thích một lỗi. Video hỗ trợ iOS và tvOS khi AVKit báo thiết bị hỗ trợ PiP, macOS, Android và các trình duyệt web có API tiêu chuẩn; cây tùy chỉnh chỉ hỗ trợ iOS. Trên iOS và tvOS, phiên âm thanh dùng chung ở trong danh mục playback khi PiP được bật, kể cả khi tạm dừng, và trộn với âm thanh của ứng dụng khác khi `AudioSession` của video là `AudioSessionMixWithOthers`.

Trong các bài kiểm thử headless, `nsgotest.PictureInPictureSupport(supported, possible)` đặt những gì hệ thống cung cấp dưới một `nsgotest.Environment` iOS, macOS hoặc Android, các trường `PiPState`, `PiPStarts` và `PiPStops` của view ghi lại phiên, `app.ReportPictureInPicture` báo một trạng thái như hệ thống sẽ báo, và `app.PictureInPictureControl` gửi `play`, `pause`, `skip` và `restore` của cửa sổ. Cửa sổ PiP của video phát và tạm dừng trình phát riêng của video, điều mà `Video.OnPlay` và `Video.OnPause` báo lại, với `VideoPauseControls` cho lần tạm dừng; `app.PlayVideo` và `app.PauseVideo` báo chúng trong bài kiểm thử.

Chạy `go tool nsgo dev -target ios examples/pip` cho cả hai hướng, hoặc `-target macos` hay `-target android` cho PiP video. Các bài kiểm thử gốc trên Mac xác minh việc vào, thoát và các lệnh lặp lại có và không có điều khiển gốc, cùng với việc thay thế một nguồn có điều khiển đang hoạt động. Việc nghiệm thu trên iPhone, bao gồm toàn màn hình của video có điều khiển và tự động vào PiP, vẫn đang chờ; phần hiện thực hiện có các bài kiểm thử delegate/khung hình gốc và độ bao phủ ứng dụng headless.

Với video trên tvOS, chạy `go tool nsgo dev -target tvos_simulator examples/pip`; NSGo liên kết trình phát với một phiên Now Playing gốc. Trình mô phỏng tvOS 27 tải được video có và không có điều khiển gốc nhưng AVKit báo PiP không được hỗ trợ ở đó. Việc vào, dừng và khôi phục PiP thực tế vẫn cần được xác minh trên một Apple TV thật.

Bài kiểm thử hồi quy cho target web dùng một clip cục bộ đã ghi sẵn và các lần nhấn nút ứng dụng đáng tin cậy để xác minh phiên PiP thực tế của trình duyệt qua các bước vào, thoát, thay thế nguồn và xóa trên Chromium. Các engine trình duyệt khác vẫn cần nghiệm thu.
