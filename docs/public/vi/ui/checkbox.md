---
title: Hộp kiểm
description: Một hộp kiểm, ở trạng thái được chọn, không được chọn hoặc không xác định.
group: Thành phần
order: 34
---

`ui.Checkbox` là một hộp kiểm theo giao diện của `ui`, kèm một nhãn bên cạnh. Nó không giữ giá trị: ứng dụng cho biết hộp có được chọn hay không và cập nhật giá trị đó trong `OnChange`.

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## Nội dung

`ui.Checkbox` hiển thị `Text` hoặc `Children` của ứng dụng bên cạnh ô hộp. `Label` thay thế cả hai cho công nghệ hỗ trợ, còn `Hint` cho biết hộp kiểm dùng để làm gì.

## Trạng thái

`Indeterminate` không phải được chọn cũng không phải không được chọn, dành cho ô "chọn tất cả" khi chỉ một phần được chọn; khi đó một lần nhấn sẽ yêu cầu trạng thái được chọn. `Disabled` chặn các lần nhấn và tiêu điểm.

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## Phím bổ trợ khi nhấn

Dùng `OnChangeEvent` thay cho `OnChange` khi một danh sách cần chọn theo dải bằng Shift kèm nhấn:

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

Khi được đặt, callback này thay thế `OnChange`; ứng dụng vẫn tự cập nhật giá trị. Kích hoạt bằng bàn phím trên máy tính để bàn và web giữ lại các phím bổ trợ sau các phím tắt của ứng dụng; trên Windows, Alt cùng Space hoặc Return sẽ đi tới menu cửa sổ. `onChangeEvent` của TSX nhận `{ checked, press }`.

## Khả năng truy cập

Hộp kiểm báo cáo `RoleCheckbox` cùng trạng thái được chọn hoặc không xác định của nó. Tên của nó là `Text`, `Label` hoặc văn bản của các phần tử con. Phím Space bật/tắt nó khi nó có tiêu điểm bàn phím, và trình đọc màn hình sẽ thông báo trạng thái mới.

## Nền tảng

Ô hộp do ứng dụng vẽ, không phải điều khiển của nền tảng, nên trông giống nhau ở mọi nơi. Thao tác nhấn, vòng tiêu điểm và trình đọc màn hình là của nền tảng. Nó được kiểm thử theo vai trò và nhãn trên mọi mục tiêu.

## Liên quan

- [Switch](switch.md): một thiết lập có hiệu lực ngay lập tức.
- [Biểu mẫu](../forms.md): các lựa chọn trong một biểu mẫu.
- [List](list.md): hộp kiểm trong các hàng.
- Trang Checkbox trong gallery.
- [Thành phần không định kiểu](../authoring/base.md#checkbox): `base.Checkbox`, cho một ô hộp có giao diện của riêng bạn.
