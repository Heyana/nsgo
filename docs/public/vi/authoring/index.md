---
title: Viết thành phần
description: Xây dựng các thành phần để người khác đưa vào ứng dụng của họ, giống như cách base và ui của NSGo được tạo ra.
group: Viết thành phần
order: 70
---

Loạt trang này dành cho tác giả của các thành phần: thư viện điều khiển, hệ thống thiết kế hoặc một view phức tạp mà các ứng dụng nhập về dưới dạng gói Go. Tài liệu giả định bạn đã nắm [Thành phần và trạng thái](../components.md) và [Kiểu dáng và giao diện](../styles.md).

## Một thành phần

Một thành phần là một kiểu dữ liệu có phương thức `Render`, giống như trong ứng dụng. Các trường của nó là những gì ứng dụng truyền vào; bất kỳ trường nào ứng dụng không gán sẽ nhận giá trị rỗng (zero value), vì vậy giá trị rỗng của mỗi trường nên là trường hợp thông dụng nhất.

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

Ứng dụng sử dụng nó như bất kỳ phần tử nào khác: `chips.Chip{Text: "New"}`.

## Hành vi và giao diện

NSGo lưu giữ mỗi điều khiển ở hai dạng. `base` nắm giữ hành vi, trạng thái và khả năng truy cập nhưng không có giao diện, đồng thời nhận mọi kiểu dáng từ ứng dụng theo từng phần; `ui` khoác giao diện lên dạng `base` để hành vi chỉ nằm ở một nơi duy nhất. Một thư viện có thể đi theo cách phân tách tương tự:

- Một thành phần **không kiểu dáng** nắm quyền xử lý thao tác nhấn, bàn phím, tiêu điểm và nội dung trình đọc màn hình nghe được, đồng thời mở ra kiểu dáng của từng phần, giống như `base.Switch`. Các ứng dụng và thư viện khác tự khoác giao diện của họ lên nó ([Các phần và trạng thái](parts.md)).
- Một thành phần **có kiểu dáng** bọc lấy một thành phần không kiểu dáng và chỉ chọn các kiểu dáng từ chủ đề đang áp dụng, giống như `ui.Switch` ([Giao diện và chủ đề](looks.md)).

Một thành phần không có hành vi nào đáng chia sẻ, chẳng hạn như một huy hiệu (badge), có thể được viết thẳng dưới dạng có kiểu dáng.

## Một gói

- Giữ mỗi thành phần trong một tệp riêng, đặt tên theo thành phần đó (`chip.go`), cùng các kiểu chỉ mình nó dùng; những gì nhiều thành phần dùng chung sẽ có tệp riêng.
- Đặt các thành phần phức tạp vào một gói riêng, với API bỏ bớt tên thành phần lặp lại, giống như `dock.Area` của `base/dock`.
- Đặt các từ ngữ mà thành phần hiển thị vào thư mục `locales/` của gói ([Từ ngữ, kiểm thử và xuất bản](shipping.md)).
- Không tự động thay đổi ngầm theo thiết bị: kích thước hay hình dạng thay đổi giữa điện thoại và máy tính để bàn phải là một trường để ứng dụng chủ động thiết lập.

## Tiếp theo

- [Các phần và trạng thái](parts.md): giá trị được kiểm soát, các phần tạo kiểu và khả năng truy cập.
- [Giao diện và chủ đề](looks.md): màu sắc từ chủ đề, các biến thể và kích thước.
- [Thành phần không kiểu dáng](base.md): các điều khiển riêng của NSGo chưa gắn giao diện để bạn tạo kiểu hoặc xây dựng tiếp.
- [Từ ngữ, kiểm thử và xuất bản](shipping.md): bản dịch, kiểm thử và TSX.
