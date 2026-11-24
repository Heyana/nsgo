---
title: Văn bản
description: Đoạn văn, span, liên kết, phông chữ và tài liệu.
group: Thành phần
order: 32
---

`core.Text` là một đoạn văn được dàn trang bởi bộ máy văn bản của nền tảng. `ui.Text` và `ui.Textf` là các hàm hỗ trợ cho một dòng theo kiểu chữ của `ui`; phần tử vẫn là một `core.Text`, nên nó cũng nhận `Style` và `Spans`.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## Nội dung

`Content` là cố định còn `ContentFunc` bám theo một signal. `MaxLines` giới hạn số dòng và `EllipsizeMode` cắt phần còn lại ở đầu, giữa hoặc cuối; `Style.WhiteSpaceNoWrap` giữ một đoạn văn trên một dòng. `TextAlign`, `TextTransform`, `LineHeight` và `LetterSpacing` định kiểu cho đoạn văn, và `OnTextLayout` báo cáo các dòng của nó sau khi bố cục.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## Span và liên kết

`Spans` tạo một văn bản gồm các đoạn chạy, mỗi đoạn có kiểu riêng. Một span có `OnPress` là một liên kết, và `Background` cùng `CornerRadius` của nó được vẽ phía sau các ký tự của nó trên mọi dòng. `SpansFunc` cung cấp lại các đoạn chạy khi những gì nó đọc thay đổi, dành cho mã được tô sáng hoặc một câu thay đổi.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` đặt một view vào trong dòng, thay cho nội dung của span: một biểu tượng, một hình ảnh hoặc một nút nhỏ. Nó được bố cục theo kích thước riêng, xuống dòng cùng các từ và tự nhận thao tác nhấn của nó.

## Phông chữ và kiểu

`FontFamily` đặt tên cho một họ phông hoặc một danh sách CSS. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration` và `TextShadowColor` cùng độ lệch và bán kính của nó định kiểu cho văn bản. `core.FontMonospace` là họ phông có độ rộng cố định của nền tảng.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

Đăng ký phông chữ đóng gói kèm bằng `nsgo.Fonts` trước view đầu tiên, hoặc đăng ký một kiểu chữ trong khi ứng dụng chạy bằng `cx.RegisterFont`. Một văn bản đã được chuẩn bị trước khi phông chữ tới sẽ được chuẩn bị lại.

## Tài liệu

`base/textview.TextView` hiển thị Markdown hoặc HTML bằng cùng các thành phần cơ bản: tiêu đề, đoạn văn, danh sách, bảng, mã, chú thích cuối trang, cảnh báo, hình ảnh, công thức và liên kết. `textview.Parse` đọc Markdown và `textview.ParseHTML` đọc HTML, còn `Options.Streaming` sửa một tài liệu vẫn đang được truyền tới.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` tìm kiếm trong văn bản mà một `TextView` hiển thị và tô sáng các kết quả khớp. Cho nhiều view dùng chung một tìm kiếm bằng `textview.ProvideSearch`, rồi gọi `Open`, `SetQuery`, `Next` và `Previous`; `Count` và `Selected` báo cáo kết quả. Các phần của tìm kiếm được định kiểu bằng `textview.PartSearch` và `textview.PartSearchSelected`.

## Lựa chọn và khả năng truy cập

`SelectableOn` cho phép người dùng chọn và sao chép một văn bản. Một `core.View` có `Selectable` làm cho các văn bản của nó được chọn cùng nhau theo thứ tự tài liệu. Một `Role` như `RoleHeader` cho công nghệ hỗ trợ biết văn bản đó là gì, và `Label` thay thế nó khi các từ không phải là nhãn.

## Nền tảng

Bộ máy văn bản của nền tảng bố cục văn bản, nên ngắt dòng, phông chữ và phương thức nhập đều tuân theo nó. Ngắt dòng CJK, emoji và văn bản từ phải sang trái tuân theo hệ thống. Ứng dụng có thể đọc cỡ chữ bằng `cx.Env().FontScale()`, và văn bản bám theo nó trừ khi đặt `NoFontScaling`.

## Liên quan

- [Văn bản](../text.md): hướng dẫn đầy đủ về văn bản, span, phông chữ và tài liệu.
- [Hình ảnh và biểu tượng](../images.md): hình ảnh và biểu tượng nội dòng.
- [Khả năng truy cập](../accessibility.md): tiêu đề, nhãn và lựa chọn.
- Các trang Text và TextView của gallery.
