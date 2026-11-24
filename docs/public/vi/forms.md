---
title: Biểu mẫu
description: Trường nhập văn bản, tiêu điểm, bàn phím ảo, công tắc, bộ chọn và kiểm tra hợp lệ.
group: Khái niệm cốt lõi
order: 57
---

## Trường nhập văn bản

`ui.Input` là trường nhập văn bản gốc của nền tảng theo giao diện của `ui`, với các trường của `base.Input`. Khi truyền `Value`, nó là thành phần được kiểm soát (controlled): ứng dụng giữ giá trị và cập nhật nó trong `OnChange`; khi truyền `DefaultValue`, nó tự giữ giá trị riêng.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` ẩn các ký tự, `Readonly` ngăn chỉnh sửa trong khi vẫn cho phép bôi đen và sao chép văn bản (trong mini program chỉ hoạt động trên trường một dòng của WeChat: trường nhiều dòng của WeChat và các trường của Alipay sẽ tắt hẳn giống `Disabled`, vì các môi trường chủ đó không thể hoàn tác thao tác sửa), `Disabled` vô hiệu hóa trường, và `Multiline` cho phép xuống dòng và tự giãn chiều cao theo nội dung trừ khi kiểu dáng ấn định kích thước. Các dòng của một trường nhiều dòng và phần chữ gợi ý (placeholder) của nó cao bằng thuộc tính `LineHeight` trong kiểu dáng, với phông chữ căn giữa trong mỗi dòng như ở `core.Text`; một trường một dòng giữ dòng chữ của nó căn giữa theo chiều cao của trường. `KeyboardType` chọn loại bàn phím ảo (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL` và các loại khác), `AutoComplete` cho hệ thống biết trường chứa thông tin gì để tự động điền, và `MaxLength` giới hạn số đơn vị UTF-16 mà người dùng nhập vào. `AutoCapitalize` và `NoAutoCorrect` tinh chỉnh hành vi của bàn phím.

`OnSelectionChange` và `OnCaretMove` báo cáo vùng chọn và vị trí dấu nháy (caret), phục vụ cho việc mở trình đơn ngay tại đó. `Suggest`, một `core.Suggest`, theo dõi truy vấn sau một ký tự kích hoạt như `@`, và một `ui.SuggestList` bên cạnh trường sẽ gợi ý các hàng ngay tại dấu nháy để chọn bằng phím Lên, Xuống, Return hoặc nhấn chọn, tương tự cách `Suggest` của `chat.Composer` hoạt động cho thao tác nhắc tên (`mention`). `OnKeyPress` lắng nghe từng phím bấm, `OnPaste` nhận nội dung người dùng dán vào, và `OnCompositionChange` theo dõi bộ gõ.

Trên iOS và Android, `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` chặn phím Return trên bàn phím ảo trước khi trường gửi biểu mẫu, mất tiêu điểm hoặc chèn dòng mới. `KeyAuto` và `KeyNotEditing` nhường quyền xử lý lại cho `SubmitBehavior`; trên iOS, phím Return dùng để xác nhận văn bản đang gõ dở (marked text) vẫn do bộ gõ xử lý.

## Tiêu điểm và bàn phím

`Ref.Focus()` đặt tiêu điểm vào một trường nhập liệu hoặc — trên máy tính để bàn, web và Apple TV — một view có thể nhấn đang bật hoặc có `Focusable`. `Ref.Blur()` chỉ bỏ tiêu điểm khỏi chính view của nó, còn `cx.DismissKeyboard()` đóng bàn phím ảo. `OnFocus` và `OnBlur` theo dõi tiêu điểm, và `View.OnFocus`/`OnBlur` làm điều tương tự cho mọi view có thể nhận tiêu điểm. Một view đang giữ tiêu điểm khi rời khỏi cây giao diện — chẳng hạn khi `Show` đóng lại hoặc hàng của nó bị xóa — sẽ nhận `OnBlur` một lần sau commit đã gỡ bỏ nó (hoặc trước `OnFocus` của view kế tiếp nếu nền tảng báo tiêu điểm mới trước), còn các view xung quanh vẫn ở lại sẽ nhận `OnFocusOut`.

Các yêu cầu focus, blur, cuộn và phóng to/thu nhỏ của một `Ref` sẽ không làm gì khi phần tử của nó đang nằm trong nội dung của một `Holder` không được hiển thị ở nơi nào, và những yêu cầu đang nằm trong hàng đợi khi phần tử rời khỏi cây sẽ bị hủy bỏ. Việc hiển thị lại nội dung đó không phát lại các yêu cầu cũ: hãy gọi lại một lần nữa khi phần tử đã được đặt vào cây.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Bàn phím ảo che mất phần dưới màn hình điện thoại. Dùng `core.EnvKeyboard` làm phần đệm dưới hoặc lề dưới sẽ giữ nội dung nằm phía trên bàn phím, và chuyển động của nó khớp nhịp hoạt ảnh với bàn phím:

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

Một `Scroll` bọc quanh biểu mẫu có thể tự ẩn bàn phím khi kéo và quyết định hành động khi chạm lúc bàn phím đang mở:

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` và `cx.Env().KeyboardDuration()` đọc kích thước và thời lượng chuyển động của bàn phím trong mã nguồn, còn `cx.Env().SafeArea()` đọc các khoảng lề an toàn quanh màn hình.

## Lựa chọn

`ui.Switch` là thiết lập có hiệu lực ngay lập tức, `ui.Checkbox` có trạng thái đã chọn, chưa chọn hoặc không xác định (indeterminate), và `ui.Radio` là một mục trong `ui.RadioGroup`. `ui.Select[T]` mở danh sách tùy chọn bên dưới nút kích hoạt, hoặc dưới dạng sheet khi `Presentation` yêu cầu, còn `ui.Tabs[T]` hiển thị một vài lựa chọn cùng lúc.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` và `base.NumberInput` nhận `LabelFunc` cho tên trợ năng thay đổi động. Một hàm `Format` tùy chỉnh bám theo các signal mà nó đọc trong khi trường không ở trạng thái đang sửa; khi chuyển đổi ngôn ngữ, bản nháp đang gõ dở vẫn được giữ nguyên cho tới khi kết thúc chỉnh sửa. `ui.NumberInput` tăng giảm theo bước và trượt chỉnh một con số trong giới hạn, `ui.DatePicker` và `ui.TimeField` chỉnh sửa ngày và giờ, `ui.ColorPicker` chỉnh sửa màu sắc, và `ui.TagInput` chỉnh sửa danh sách thẻ (tag).

`ui.OtpInput` nhận mã xác thực một lần vào từng ô. Thành phần này cùng `base.OtpInput` nhận `LabelFunc` mà không làm thay thế trường đang giữ tiêu điểm hay mã bên trong. `Size` đặt chiều cao và chiều rộng mặc định: 36 điểm trên máy tính để bàn và TV, 44 điểm trên thiết bị cảm ứng. `CellWidth` và `CellHeight` ghi đè độc lập từng chiều. `TextStyle` ghi đè kiểu chữ trong ô, `CornerRadius` thay đổi độ bo góc, và `CaretHeight` ghi đè chiều cao suy ra từ `LineHeight`.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## Nhóm các hàng

`ui.FormSection` đặt các hàng của nó lên một thẻ (card), với tiêu đề ở trên và chân trang ở dưới. Đặt `CardStyle` để tùy chỉnh nền, viền hoặc bo góc của thẻ. `CardStyleFunc` gộp đè lên trên đó và bám theo các signal nó đọc. `DividerInset` nhận từng hàng con, kể cả các hàng tạo bởi `core.Each`: trả về khoảng thụt lề và `true` để ghi đè đường phân cách của hàng đó, hoặc `false` để giữ căn lề tự động theo tiêu đề (12 điểm cho các node khác). Khoảng thụt lề bằng 0 sẽ tạo đường phân cách kéo hết chiều rộng.

`ui.FormRow` đặt `Leading` hoặc một ô biểu tượng (icon tile) trước tiêu đề. Đặt `LeadingWidth` để chừa chỗ cho node của bạn và căn thẳng đường phân cách bên dưới nó với tiêu đề. Vị trí này mặc định rộng 24 điểm. Đặt `TileSize` để co giãn đồng bộ ô biểu tượng, ký hiệu (glyph) và bo góc. `TitleStyle` và `DetailStyle` ghi đè kiểu chữ của tiêu đề và chi tiết. `TitleMaxLines` giới hạn số dòng của tiêu đề (0 là không giới hạn), `MinHeight` ghi đè chiều cao tối thiểu của hàng theo đơn vị điểm, và `Below` đặt một node nằm dưới tiêu đề và chi tiết trong cùng cột văn bản của chúng. `MinHeight` bằng 0 giữ mức mặc định: 40 điểm, hoặc 48 điểm trên thiết bị cảm ứng.

Đối với một nhóm chọn một giá trị duy nhất, hãy bọc section trong `base.RadioGroup` và đặt `Role: core.RoleRadio`, `Checked` và `OnPress` cho từng hàng. Hàng sẽ dùng `base.Radio` để lấy trạng thái trợ năng đã chọn và được đánh dấu, gom thành một điểm dừng Tab duy nhất và hỗ trợ chọn bằng phím mũi tên; nhấn vào một hàng đã chọn sẵn sẽ không làm gì. Ứng dụng tự quản lý giá trị được chọn.

`ui.FormSection.TitleFunc` và `FooterFunc`, cùng `ui.FormRow.TitleFunc`, `DetailFunc` và `LabelFunc`, cập nhật nội dung chữ mà không dựng lại hàng. Tiêu đề section, chân trang hoặc phần chi tiết rỗng sẽ gỡ bỏ văn bản tùy chọn đó; nhãn hàng rỗng sẽ dùng tiêu đề hiện tại của hàng. Một `ui.Select` có tìm kiếm liên kết `SearchPlaceholderFunc` và `EmptyFunc` mà vẫn giữ nguyên khung tìm kiếm đang mở và lựa chọn hiện tại.

## Kiểm tra hợp lệ

Ứng dụng tự quản lý các quy tắc: một signal giữ thông báo lỗi, `OnBlur` hoặc `OnSubmit` gán giá trị cho nó, và `core.Show` thêm thông báo dưới trường nhập liệu khi có lỗi.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` tạo hoạt ảnh cho thông báo lỗi khi xuất hiện, và `OnSubmit` kết hợp với `ReturnKeyNext` chuyển bàn phím sang trường tiếp theo.

## Tiếp theo

- [Điều hướng](navigation.md): nơi biểu mẫu sinh sống.
- [Văn bản](text.md): cách tạo kiểu chữ cho trường nhập liệu.
- [Khả năng truy cập](accessibility.md): nhãn và giá trị mà trình đọc màn hình cần.
