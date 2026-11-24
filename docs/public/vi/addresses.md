---
title: Địa chỉ ứng dụng
description: Đọc và cập nhật địa chỉ của ứng dụng web thông qua lịch sử trình duyệt.
group: Khái niệm cốt lõi
order: 59
---

`cx.AppURL()` đọc địa chỉ của ứng dụng dưới dạng một giá trị được theo dõi. Trên web, URL trang ban đầu đã sẵn sàng trước khi thành phần gốc kết xuất, nhờ đó việc tải lại trang hoặc mở liên kết chia sẻ có thể chọn đúng view tương ứng. Ứng dụng tự quyết định cách ánh xạ URL sang các view của mình.

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // Show the error in the app's usual way.
            }
        }},
    }}
}
```

`SetAppURL` cập nhật địa chỉ được theo dõi ngay lập tức mà không tải lại trang hay điều hướng ngăn xếp. Theo mặc định, nó thay thế mục lịch sử trình duyệt hiện tại; `HistoryPush` yêu cầu thêm một mục mới. Nếu có thao tác điều hướng hoặc đẩy modal diễn ra trong cùng một commit, thao tác đó sẽ cung cấp mục lịch sử thay thế để một hành động không tạo ra hai bước quay lại. Nhiều lần ghi trong cùng một commit sẽ giữ lại địa chỉ cuối cùng và bảo toàn mọi yêu cầu đẩy lịch sử mới.

Địa chỉ tương đối được phân giải dựa trên địa chỉ hiện tại hoặc thẻ base HTML được chỉ định rõ. Địa chỉ trên trình duyệt phải giữ nguyên cùng nguồn (origin) với trang. Địa chỉ không hợp lệ, khoảng trắng, ký tự điều khiển, dấu gạch chéo ngược (`\`) và địa chỉ vượt quá 16 KiB sẽ trả về lỗi mà không làm thay đổi địa chỉ hiện tại.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` lắng nghe các thao tác duyệt lịch sử trình duyệt và chỉnh sửa phần hash. `change.Reason` có giá trị là `"back"`, `"forward"` hoặc `"address"`; hàm trả về dùng để hủy đăng ký, và việc giải phóng scope cũng tự động gỡ trình lắng nghe. Hãy đọc `AppURL` để lấy địa chỉ ban đầu. Các lần ghi từ chính ứng dụng sẽ cập nhật các liên kết dữ liệu nhưng không phát thông báo thay đổi này. Một lần duyệt lịch sử giữ nguyên địa chỉ cũng không phát thay đổi.

Nút quay lại (back) của trình duyệt vẫn đóng modal hoặc pop một màn hình điều hướng. Nếu ứng dụng từ chối thao tác quay lại, mục lịch sử hiện tại cùng địa chỉ của nó sẽ được khôi phục. Trình xử lý đóng modal có thể tự đặt địa chỉ thay thế của riêng mình. Thao tác pop bằng mã nguồn sẽ cập nhật địa chỉ sau khi quá trình duyệt lịch sử bất đồng bộ hoàn tất, và các lần ghi phát sinh trong quá trình đó sẽ đợi cho đến khi hoàn tất. Việc quay lại vượt quá các mục của trình chạy này sẽ thoát khỏi ứng dụng, bao gồm cả các mục được giữ lại từ lần tải trước.

Đối với các đường dẫn như `/films/42`, hãy cấu hình thư mục triển khai ứng dụng và thiết lập máy chủ trả về tệp `index.html` của ứng dụng cho các tuyến đường ứng dụng:

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base` ghi thẻ HTML base để các tập lệnh, bảng kiểu, Wasm và tài nguyên vẫn tải được đúng khi mở trực tiếp một địa chỉ sâu. Nếu không thiết lập, các đường dẫn tài nguyên tương đối hiện có sẽ được giữ nguyên; các địa chỉ dạng truy vấn (query) hoặc hash vẫn dùng bình thường mà không cần thay đổi. Quy tắc dự phòng của máy chủ phải giữ nguyên các yêu cầu tới tệp tài nguyên thật. Một Chính sách Bảo mật Nội dung (Content Security Policy) nghiêm ngặt phải cho phép thẻ base cùng nguồn này (`base-uri 'self'`).

Một ứng dụng nhúng bên trong trang khác sẽ lưu giữ địa chỉ của nó ở nội bộ và không đụng đến lịch sử của trang chứa. Một iframe cùng nguồn có thể chủ động bật tính năng này bằng thuộc tính `data-nsgo-history`. Các cây con bị giới hạn (confined subtrees) không thể đọc hoặc thay đổi địa chỉ của ứng dụng.

Trên các nền tảng đích khác, `AppURL` lưu giữ liên kết cuối cùng được mở bằng ứng dụng hoặc giá trị do `SetAppURL` lưu lại; việc thiết lập giá trị này không làm thay đổi cửa sổ gốc hay trình duyệt hệ thống. Các mini program cũng lưu giữ giá trị này mà không làm thay đổi tuyến đường của ứng dụng chủ.

TSX cũng dùng chung trạng thái này:

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
