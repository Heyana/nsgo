---
title: Danh sách
description: Số hàng bất kỳ, chỉ dựng những hàng đang hiển thị, với lựa chọn và kích hoạt.
group: Thành phần
order: 38
---

`ui.List` hiển thị các hàng trên một scroll view gốc và chỉ dựng những hàng gần màn hình, với lựa chọn, kích hoạt và bàn phím.

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## Hàng và định danh

`Count` cho biết có bao nhiêu hàng và `Row` dựng một hàng. `Key` cấp định danh cho một hàng: các hàng còn lại giữ nguyên ô và trạng thái của chúng khi các hàng phía trên được thêm vào hoặc bị bỏ đi. Hãy dùng một ID thay vì chỉ số khi các hàng có thể được thêm, xóa hoặc sắp xếp lại; chỉ số là một signal đi theo ô khi ô đó hiển thị một hàng khác.

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` là chiều cao dự kiến của một hàng, là 50 point khi bằng 0. Các hàng có thể có chiều cao khác nhau; danh sách đo những hàng nó dựng và ước lượng phần còn lại.

## Bố cục

`Header`, `Footer` và `Empty` chứa nội dung xung quanh các hàng và khi không có hàng nào. `Gap` ngăn cách các hàng, `Columns` xếp chúng cạnh nhau, và phần đệm của danh sách là của nội dung, nên thanh cuộn vẫn nằm ở mép. Hãy cấp cho danh sách một chiều cao hoặc để nó lấp đầy phần tử cha.

## Lựa chọn và kích hoạt

`ui.List` nhận một `Selection`, được tạo bằng `ui.NewSelection(cx, multiple)`. Khóa, nhãn, kích hoạt và bàn phím bám theo nó: Up, Down, Home và End di chuyển con trỏ, Shift mở rộng vùng chọn, Mod+A chọn tất cả, Space bật/tắt trong lựa chọn nhiều mục, và Enter hoặc nhấp đúp kích hoạt một hàng.

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange` chạy sau khi một lần nhấn, một phím hoặc tìm kiếm khi gõ (typeahead) thay đổi những hàng được chọn, và không chạy khi chính ứng dụng đặt `Selection`, nhờ đó một khung chi tiết có thể bám theo lựa chọn của người dùng trong khi ứng dụng di chuyển vùng chọn tới một mục được mở theo cách khác.

`TapSelects` khiến một lần chạm trên điện thoại chọn hàng thay vì kích hoạt nó. `RowLabel` là nội dung mà công nghệ hỗ trợ đọc và nội dung mà typeahead so khớp.

## Tải thêm và cuộn

`OnLoadMore` chạy khi gần tới cuối, một lần cho mỗi số lượng hàng. `Ref` cuộn bằng `ScrollToIndexAt`, `ScrollToEnd` và `ScrollToIndex`. `core.List`, danh sách cơ bản bên dưới nó, còn có `OnLoadMoreAtStart`, `Inverted` cho giao diện trò chuyện, `StickyHeaderIndices`, `MaintainVisibleContentPosition`, `Viewport` và `ViewabilityConfig` cho các trường hợp cấp thấp ([Danh sách và cuộn](../lists.md)).

## Khả năng truy cập

Danh sách là một list hoặc listbox duy nhất, được đặt tên bởi `Label`, với văn bản của một hàng hoặc `RowLabel` làm tên của hàng đó. Một danh sách có lựa chọn báo cáo hàng của con trỏ và thông báo hàng đó khi bàn phím di chuyển. Một trình đọc màn hình di chuyển ra ngoài vùng hiển thị sẽ yêu cầu danh sách cuộn tới đó, việc này dựng các hàng mà nó tới được.

## Nền tảng

Scroll view gốc và cơ chế ảo hóa là như nhau trên mọi mục tiêu. Các hàng được tái sử dụng, nên trạng thái của một hàng phải bám theo `Key` của nó; quy tắc thông thường là giữ trạng thái dựng trong ứng dụng thay vì trong một hàng.

## Liên quan

- [Danh sách và cuộn](../lists.md): hướng dẫn đầy đủ, bao gồm tiêu đề cố định và tải thêm.
- [Scroll](scroll.md): một scroll view đơn giản.
- [DataTable](../lists.md#tables): một bảng có nhiều hàng và cột.
- Các trang List, Table và DataTable trong gallery.
