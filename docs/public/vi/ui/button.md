---
title: Button
description: Nút nhấn, các biến thể và kích thước của nó, và nút biểu tượng.
group: Thành phần
order: 31
---

`ui.Button` là một nút nhấn mang giao diện của `ui`, với các biến thể và kích thước mà một cửa sổ cần. `OnPress` chạy khi một lần nhấn hoàn tất.

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## Biến thể và kích thước

`ui.Button.Variant` là `ButtonPrimary` (mặc định), `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`, `ButtonTinted` hoặc `ButtonDanger`. Tinted và danger dùng màu nhấn hoặc màu nguy hiểm của chủ đề trên một nền tô nhạt. Trên các idiom máy tính để bàn và TV, `Size` là `SizeDefault` (28 point), `SizeSmall` (24) hoặc `SizeLarge` (32). Trên các idiom cảm ứng, cùng các kích thước đó là 44, 40 và 48 point, nên có thể dùng nút mà không cần tạo kiểu lại.

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` tô nút bằng một màu của ứng dụng thay vì màu của biến thể, còn `Tint` tô biểu tượng của nó. `Flex` cho nút một phần chia trong một hàng, còn `Style` bố trí nó: có thể khiến nút lấp đầy vùng chứa, đặt kích thước theo ý ứng dụng, thêm margin hoặc bo góc; `TestID` đặt tên cho nó để kiểm thử.

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

Nút chỉ có biểu tượng là `ui.IconButton`, với `Variant` là `IconButtonGhost`, `IconButtonRaised` hoặc `IconButtonFilled` (một hình tròn phẳng mang màu inset của chủ đề hoặc màu `Fill`), một `Size` tính bằng point và một `Label` cho công nghệ hỗ trợ. Trên máy tính để bàn, nhãn còn hiển thị dưới dạng tooltip khi con trỏ dừng trên nút; `Tooltip` hiển thị nội dung khác ở đó, và một `Tooltip` trả về `""` sẽ không hiển thị gì. Một `ui.Button` có `Icon` mà không có `Text` cũng là hình vuông, và cần `Label`.

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## Nội dung và trạng thái

`ui.Button` hiển thị `Text`, một `Icon` tùy chọn ở đầu, hoặc cả hai. Dùng `TextStyle` để tùy chỉnh cỡ chữ, độ đậm hoặc các kiểu văn bản khác của nhãn.

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` nhận một hàm, nên nút bám theo một signal: nút bị vô hiệu hóa không nhận lần nhấn lẫn tiêu điểm bàn phím, và giao diện của nó là trạng thái vô hiệu hóa, không phải một màu do ứng dụng chọn.

`Busy` dành cho một hành động vẫn đang chạy. Khi nó trả về true, một biểu tượng đang tải mang màu chữ của nút hiển thị đè lên nội dung, nên nó vẫn hiện rõ trên màu nhấn của nút primary; các lần nhấn không có tác dụng, và công nghệ hỗ trợ được báo rằng nút đang bận. Mặc định, nội dung vẫn giữ chỗ của nó bên dưới biểu tượng đang tải. Nút vẫn giữ tiêu điểm bàn phím. `BusyLabel` hoặc `BusyLabelFunc` hiển thị một thông điệp bên cạnh biểu tượng đang tải và báo nó làm tên truy cập của nút trong khi nút đang bận. Nút giữ ít nhất chiều rộng lúc nghỉ của nó.

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` và `ui.ProgressRing` nhận một `Tint` riêng cho bất kỳ bề mặt nào khác mà màu nhấn của chúng có thể bị chìm mất.

## Liên kết

`ui.Link{Text: "Docs", Href: "/docs"}` mở một đích đến với vai trò liên kết. Văn bản và gạch chân khi hover của nó dùng màu nhấn, trừ khi `Style.Color` đặt một màu khác; `Style.Hover.Color` đặt màu khi hover.

## Khả năng truy cập

Nút báo vai trò `RoleButton` và dùng văn bản của nó làm tên. `Label` thay thế văn bản đối với công nghệ hỗ trợ. Một nút chỉ có biểu tượng mà không có `Label` thì không đọc được, nên hãy đặt tên cho nó.

Return và Space nhấn một nút đang có tiêu điểm trên máy tính để bàn, web và Apple TV; trình đọc màn hình của điện thoại nhấn nó như một cú click.

## Nền tảng

Nút là một view được vẽ giống nhau ở mọi nơi, không phải nút của nền tảng; các lần nhấn, tiêu điểm và khả năng truy cập đến từ nền tảng. Các idiom cảm ứng dùng vùng bố cục lớn hơn như đã mô tả ở trên.

## Liên quan

- [Biểu mẫu](../forms.md): nút trong biểu mẫu và kiểm tra hợp lệ.
- [Kiểu dáng và chủ đề](../styles.md): chủ đề mà nút lấy màu từ đó.
- [Khả năng truy cập](../accessibility.md): vai trò, nhãn và tiêu điểm.
- Trang Button của bộ sưu tập.
- [Thành phần không kiểu dáng](../authoring/base.md#button): `base.Button`, cho một nút mang giao diện riêng của bạn.
