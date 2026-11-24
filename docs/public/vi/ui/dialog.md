---
title: Hộp thoại
description: Một bề mặt modal, một sheet và một hộp cảnh báo.
group: Thành phần
order: 41
---

`ui.Dialog` là một bề mặt modal nằm trên ứng dụng bị làm mờ, và là một sheet trượt lên từ dưới trên màn hình cảm ứng. `ui.Sheet` là một bảng trượt lên từ dưới, còn `ui.AlertDialog` yêu cầu người dùng xác nhận.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Mở và đóng

Ứng dụng giữ trạng thái hộp thoại có đang mở hay không. `OnOpenChange` nhận mọi thay đổi mà hộp thoại yêu cầu, kèm một `base.Reason` phân biệt một lần nhấn bên ngoài với Escape hoặc nút quay lại; `OnClose` chạy một lần khi hộp thoại đã đóng. `NoCloseOnEscape` và `NoCloseOnBackdropPress` ngăn Escape và một lần nhấn bên ngoài đóng hộp thoại.

`Title` và `Description` đứng đầu bề mặt, `TitleFunc` và `DescriptionFunc` dùng khi chúng thay đổi trong lúc hộp thoại đang mở, và `Footer` chứa các nút của nó. `Bare` giao toàn bộ bề mặt cho `Children`, dành cho hộp thoại có bố cục riêng của ứng dụng; trên máy tính để bàn, nội dung được cắt bên trong viền bề mặt theo các góc mà `Style` và `StyleFunc` đặt cho nó, và `Inset` đặt khoảng cách tối thiểu của nó tới các cạnh cửa sổ trên máy tính để bàn.

Trên các sheet cảm ứng, `Bare` giờ đây bỏ thanh kéo (grabber) và phần đệm nội dung mặc định để `Children` lấp đầy bảng. `NoGrabber` chỉ ẩn thanh kéo, `ContentStyle` và `ContentStyleFunc` tùy chỉnh lớp bao nội dung, và `TitleStyle` và `TitleStyleFunc` tùy chỉnh tiêu đề. `Leading` và `Trailing` đặt các node, chẳng hạn như nút Cancel và Done, ở hai đầu hàng tiêu đề, với tiêu đề được căn giữa giữa chúng. `StartingStyle` và `EndingStyle` tùy chỉnh hiệu ứng chuyển tiếp của sheet; hãy thêm một `Transition` vào chúng khi bạn cần thời gian tùy chỉnh.

`Style` được gộp đè lên giao diện của bề mặt. `StyleFunc` được gộp sau nó và bám theo các signal mà nó đọc, bao gồm cả chế độ giao diện, trong lúc hộp thoại đang mở và khi được mở lại. Ví dụ, `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` bám theo chủ đề, trong đó `colors := ui.ColorsOf(cx)`. Cả hai trường cũng hoạt động trên `ui.AlertDialog`. Một màu bằng không giữ nguyên màu hiện có; hãy dùng một màu trong suốt khác không như `core.RGBA(0xFFFFFF, 0)` để xóa nó.

## Sheet

`ui.Sheet` là một bảng ở phía dưới, nằm trên một lớp nền bị làm mờ; một lần nhấn vào lớp nền, Escape hoặc nút quay lại sẽ gọi `OnClose`.

Đặt `Label` hoặc `LabelFunc` làm tên hộp thoại trợ năng của nó, và `TestID` để tìm bảng trong kiểm thử. `LabelFunc` bám theo các signal của nó mà không thay thế nội dung đang mở. `CloseOnSwipe: true` cũng gọi `OnClose` sau khi một cú vuốt xuống vượt qua ngưỡng đóng; hãy cập nhật `Open` trong callback đó để đóng nó.

Đặt `Style` hoặc `StyleFunc` để tùy chỉnh nền, bán kính góc và phần đệm của bề mặt trong khi vẫn giữ thanh kéo và việc xử lý vùng an toàn. `StyleFunc` bám theo các signal mà nó đọc. Một `CornerRadius` không kèm `CornerRadii` sẽ bo tròn mọi góc thay cho phần trên bo tròn mặc định.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Hộp cảnh báo

`ui.AlertDialog` yêu cầu người dùng xác nhận hoặc ghi nhận một điều gì đó. Mỗi hành động sẽ đóng nó, còn một lần nhấn bên ngoài thì không.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

Một hành động có `Role: core.AlertCancel` chạy khi nhấn Escape hoặc nút quay lại, tiếp theo là `OnOpenChange(false, base.ReasonCancel)`. `Default: true` trao tiêu điểm bàn phím cho một hành động khi hộp cảnh báo mở trên máy tính để bàn, web và Apple TV, và khiến Return chọn nó; một lần nhấn hành động sẽ chạy `OnPress`, rồi `OnOpenChange(false, base.ReasonSelect)`. Return chạy hành động mặc định khi tiêu điểm bàn phím không nằm trên hành động hay trường nào khác: khi tiêu điểm đã chuyển sang một hành động khác, Return nhấn hành động đó, và một hộp thoại nằm trên hộp cảnh báo sẽ giữ phím Return. Khi một hành động đã chạy, hộp cảnh báo sẽ đóng, bất kể `OnOpenChange` làm gì, và không chạy hành động nào khác cho tới khi `Open` chuyển sang false rồi true trở lại. Nếu nhiều hành động mang cùng một dấu đánh dấu, hành động đầu tiên được ưu tiên. Khi không có dấu đánh dấu, Escape đóng hộp cảnh báo mà không chạy hành động nào và Return không có hành động mặc định. `Role: core.AlertDestructive` mô tả một hành động; `Variant` hoặc `Color` chọn giao diện của nó.

Để dùng hộp cảnh báo hoặc action sheet của chính hệ thống, hãy dùng `cx.ShowAlert` ([Dịch vụ nền tảng](../services.md#alerts-and-the-clipboard)).

## Hành vi

Một modal chặn ứng dụng phía sau nó, nhận tiêu điểm bàn phím và đóng khi nhấn Escape và nút quay lại, trừ khi được yêu cầu không làm vậy. Trên màn hình cảm ứng, hộp thoại là một sheet đi theo thao tác kéo xuống và đóng lại khi vượt qua một ngưỡng.

## Khả năng truy cập

`ui.Dialog` báo cáo `RoleDialog`, hoặc `Role` được chỉ định rõ, còn `ui.AlertDialog` báo cáo `RoleAlertDialog` trên máy tính để bàn và màn hình cảm ứng. Cả hai đọc tiêu đề của chúng, hoặc `Label` khi tiêu đề chưa đủ, tiếp theo là mô tả. Tiêu điểm di chuyển vào hộp thoại khi nó mở và quay trở lại khi nó đóng; lớp nền không phải là một điểm dừng. Các nút ở chân hộp thoại giữ nguyên vai trò và nhãn của chúng.

## Nền tảng

Trên Android và màn hình cảm ứng, hộp thoại là một sheet trượt lên từ dưới; trên máy tính để bàn, nó là một bề mặt được căn giữa kèm lớp nền. Nội dung là như nhau; chỉ có cách trình bày thay đổi.

## Liên quan

- [Điều hướng](../navigation.md): trang, thẻ và modal kết hợp với nhau.
- [Menu](menu.md): các lệnh được neo và menu ngữ cảnh.
- [Khả năng truy cập](../accessibility.md): tiêu điểm và vai trò hộp thoại.
- Các trang Dialog, Sheet và AlertDialog trong gallery.
- [Thành phần không định kiểu](../authoring/base.md#dialog): `base.Dialog` và `base.Sheet`, với cách trình bày của iOS và chuyển động của riêng bạn.
