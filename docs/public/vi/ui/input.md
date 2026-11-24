---
title: Trường nhập
description: Trường văn bản của nền tảng, có kiểm soát hoặc không, với tiêu điểm và bàn phím ảo.
group: Thành phần
order: 33
---

`ui.Input` là trường văn bản của nền tảng theo giao diện của `ui`, một dòng trừ khi có `Multiline`. Nó có các trường của `base.Input`, như mọi trường đã định kiểu khác; `ui.Field` là cùng trường đó nhưng không có kích thước riêng, nằm trong một viền không thay đổi theo con trỏ hay tiêu điểm; `core.Transparent` trong `Background` và `core.Zero` trong `BorderWidth` của nó sẽ tắt khung hộp, dành cho văn bản trông như văn bản thường nhưng vẫn chỉnh sửa được.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## Có kiểm soát và không kiểm soát

Với `Value`, trường là có kiểm soát: ứng dụng giữ giá trị và cập nhật nó trong `OnChange`. Với `DefaultValue`, trường tự giữ giá trị của riêng nó. Một trường có kiểm soát có thể được xóa bằng cách đặt giá trị của nó thành `""`.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## Các trường

`Multiline` ngắt dòng, ban đầu cao ba dòng và tăng chiều cao theo văn bản trừ khi kiểu dáng của nó đặt `Height` hoặc `MinHeight`. `Masked` ẩn các ký tự, `Readonly` chặn chỉnh sửa trong khi văn bản vẫn có thể được chọn và sao chép, `Disabled` tắt trường, và `MaxLength` giới hạn số đơn vị UTF-16 mà người dùng nhập. `Placeholder` có thể là một hàm với `PlaceholderFunc`.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` chọn một bàn phím ảo (`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL` và các loại khác), `ReturnKeyType` đặt nhãn cho phím return của nó, và `SubmitBehavior` cho biết Return sẽ gửi hay ngắt dòng trong trường nhiều dòng. `AutoCapitalize`, `NoAutoCorrect` và `AutoComplete` điều chỉnh các trợ giúp của hệ thống.

`InputSource` chọn một nguồn nhập bàn phím khi trường có tiêu điểm và khôi phục nguồn của người dùng khi trường mất tiêu điểm, trừ khi có `KeepInputSource`: `"com.apple.keylayout.ABC"` giữ một trường dành cho mã hoặc lệnh ở tiếng Anh trên macOS. ID là của nền tảng, là một ID bố cục bàn phím như `"00000409"` trên Windows và một bộ gõ Fcitx 5 hoặc IBus trên Linux; một nguồn chưa được bật, điện thoại và web không thay đổi gì.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` được gộp đè lên giao diện của trường, và `StyleFunc` đè lên đó: những gì chúng đặt, chẳng hạn như chiều cao, phần đệm, phông chữ hoặc một màu, sẽ thay thế của `ui`, phần còn lại giữ nguyên. Viền tối đi khi con trỏ di qua và nhận màu nhấn khi có tiêu điểm, miễn là không cái nào đặt `BorderColor` hoặc `Hover`. Một `Padding` bằng không giữ phần đệm của `ui`; `core.Zero`, một độ dài được đặt rõ, sẽ bỏ nó đi (`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## Tiêu điểm và sự kiện

`Ref` đặt và bỏ tiêu điểm cho trường. `OnFocus` và `OnBlur` theo dõi tiêu điểm, `OnSelectionChange` và `OnCaretMove` theo dõi vùng chọn và con nháy, `OnKeyPress` theo dõi từng phím được gõ, `OnPaste` những gì được dán và `OnCompositionChange` một bộ gõ đang soạn chữ.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Bàn phím ảo che phần dưới của điện thoại. Đặt `core.EnvKeyboard` vào phần đệm hoặc lề dưới của nội dung để nội dung luôn nằm trên bàn phím; chuyển động được tạo hiệu ứng cùng với bàn phím. `cx.DismissKeyboard()` đóng bàn phím, và `cx.Env().KeyboardHeight()` đọc chiều cao của nó.

## Kiểm tra hợp lệ

Ứng dụng nắm giữ các quy tắc. Một signal giữ lỗi, `OnBlur` hoặc `OnSubmit` đặt nó, và một thông báo dưới trường hiển thị nó ([Biểu mẫu](../forms.md#kiểm-tra-hợp-lệ)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## Khả năng truy cập

Trường báo cáo `RoleTextField`, hoặc `RoleSearchField` cho hộp tìm kiếm khi `Role` của nó chỉ định như vậy, và đọc `Label` của nó; `Placeholder` không phải là nhãn, nên hãy cấp `Label` cho trường không có nhãn hiển thị. `Readonly` và `Disabled` được báo cáo, và `OnSubmit` là hành động của bàn phím. Các trường một dòng, che ký tự và nhiều dòng cung cấp `Placeholder` hiện tại làm placeholder hoặc gợi ý trợ năng gốc trên macOS, iOS và Linux, cập nhật và xóa mà không tạo thêm điểm dừng tiêu điểm thứ hai cho nhãn placeholder được vẽ của trường nhiều dòng.

## Nền tảng

iOS, Android, Windows và Linux dùng các trường gốc của chúng; web dùng một `<input>` hoặc `<textarea>`. Bàn phím ảo, tự động điền, bộ gõ và hướng văn bản là của nền tảng. Tiêu điểm và thao tác gõ được kiểm thử theo cùng một cách trên mọi mục tiêu.

## Liên quan

- [Biểu mẫu](../forms.md): trường, kiểm tra hợp lệ, lựa chọn và bàn phím.
- [Khả năng truy cập](../accessibility.md): nhãn và tiêu điểm.
- [Bố cục](../layout.md): giữ nội dung nằm trên bàn phím.
- Các trang Input và Form trong gallery.
