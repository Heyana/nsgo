---
title: Kéo và thả
description: Truyền giá trị ứng dụng giữa các view trong cùng một cửa sổ.
group: Khái niệm cốt lõi
order: 57
---

`base/dragdrop` kết nối một nguồn (source) và một đích (target) trong cùng một `Area`, nằm trong một cửa sổ. Ứng dụng giữ nguyên các view, kiểu dáng, vùng chọn và hành động xử lý giá trị của mình. Theo mặc định, một nguồn bắt đầu kéo sau 4 điểm dịch chuyển, giữ nguyên thao tác nhấn thông thường.

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` chạy một lần khi thao tác kéo bắt đầu. Hãy trả về một slice để mang nhiều mục đang chọn đi cùng nhau; nếu đó là dữ liệu có thể thay đổi (mutable), hãy sao chép nó ở đây để giữ nguyên bản chụp tại thời điểm kéo. Giá trị `Data` bằng `nil` sẽ tắt khả năng kéo. `Threshold` đặt khoảng dịch chuyển cần thiết. `Activation: dragdrop.AfterLongPress` chờ `View.DelayLongPress` (mặc định 500 ms) trước, sau đó mới tính dịch chuyển; hãy chọn chế độ này một cách tường minh ở những nơi thao tác vuốt cần ưu tiên cuộn trước.

`View` giữ nguyên các prop thông thường của nó, bao gồm hàm xử lý nhấn và khả năng truy cập. Nguồn tự quản lý `View.Responder`, nên hãy để trống trường này. Các phần tử con bên trong nguồn có thể chứa một nút bấm với hành động bình thường của nó. `Preview` đi theo con trỏ mà không chiếm quyền nhận đầu vào hay gắn lại (remount) nguồn; nếu không có `Preview`, nhãn của nguồn sẽ được hiển thị. Hãy đặt nhãn hoặc preview cho nguồn. Các `Area` lồng nhau hoạt động độc lập với nhau.

Các đích cần có `OnDrop` để tiếp nhận giá trị. Đích nằm sâu nhất bên trong nhận cơ hội đầu tiên; khi `Accept` từ chối giá trị, một đích cha chấp nhận nó có thể nhận thay. `OnEnter`, `OnMove` và `OnLeave` có thể cập nhật diện mạo của đích. Mỗi sự kiện mang theo `Data`, `Position` tính trong đích và `PagePosition` tính trên bề mặt (surface). Việc vô hiệu hóa hoặc gỡ bỏ đích đang hoạt động sẽ xóa trạng thái đã đi vào của nó; khi thả tay, điều kiện chấp nhận được kiểm tra lại một lần nữa. Một đích nằm sau view khác, bị cắt khuất, bị vô hiệu hóa hoặc nằm dưới hộp thoại modal sẽ không thể nhận thả.

`OnStart` và `OnEnd` mô tả phiên kéo của nguồn. `OnEnd` nhận `Result{Data, Dropped, Target}`. Phím Escape, hệ thống hủy chạm gốc, vô hiệu hóa hoặc gỡ bỏ nguồn, hay thả tay bên ngoài một đích chấp nhận sẽ kết thúc phiên mà không thả thành công. Vị trí đích sử dụng hình học của lần commit gần nhất; các hoạt ảnh gốc diễn ra giữa các commit không được lấy mẫu. Cơ chế này truyền giá trị bên trong cửa sổ; tệp từ hệ thống vẫn dùng `OnFileDrop`.

Hãy luôn duy trì một trình đơn, lệnh bàn phím hoặc hành động trợ năng thực hiện cùng một thao tác đó. Kéo và thả là lối tắt tới hành động ấy. Các bài kiểm thử dùng `app.DragTo("id:poster", "id:playlist")` hoặc `DragTo` của một bề mặt đã chọn; bài kiểm thử TSX dùng `await app.dragTo("id:poster", "id:playlist")`. Cả hai view phải nằm gọn sẵn trong cửa sổ hiển thị; bước kiểm thử gửi một chuỗi sự kiện chạm thông qua responder mà không giả lập sự kiện thả hay cuộn trong khi kéo. Xem [Kiểm thử](testing.md).
