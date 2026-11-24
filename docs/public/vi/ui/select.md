---
title: Hộp chọn
description: Một giá trị từ một danh sách, dưới dạng popup hoặc sheet.
group: Thành phần
order: 36
---

`ui.Select[T]` chọn một trong danh sách các giá trị thuộc bất kỳ kiểu so sánh được nào. Danh sách mở ra bên dưới nút kích hoạt trên mọi thiết bị; `Presentation: base.PresentSheet` thay vào đó mở nó dưới dạng một sheet trượt lên từ dưới, như một ứng dụng điện thoại có thể muốn.

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## Tùy chọn và tìm kiếm

`Options` là các `ui.Option[T]`, mỗi tùy chọn có một `Value`, một `Label`, và tùy ý một `Group`, `Icon`, hàm `Disabled` hoặc `Content` riêng của ứng dụng. Các tùy chọn liền kề thuộc cùng một `Group` không rỗng được hiển thị dưới một tiêu đề của nhóm đó, trong `Select` hoặc `Combobox`; tiêu đề này được các phím mũi tên và thao tác gõ bỏ qua, và được trình đọc màn hình đọc như một heading. Dùng `OptionsFunc` khi các tùy chọn bám theo một signal, để danh sách đang mở cập nhật tại chỗ và giữ nguyên giá trị và tiêu điểm của trường. `Placeholder` hiển thị khi chưa có tùy chọn nào được chọn. `Searchable` lọc danh sách bằng cách gõ, `OnClear` hiển thị một nút xóa, và `Disabled` vô hiệu hóa nó.

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` bố trí hộp chọn, rộng 200 point trừ khi nó đặt một chiều rộng; chiều cao, phần đệm và kiểu chữ của nó được áp dụng tới trường, giống như với một [Input](input.md), nhờ đó hộp chọn thẳng hàng với một nút `SizeSmall`:

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## Hành vi

Up, Down, Return và Space mở danh sách từ nút kích hoạt đang có tiêu điểm; các phím mũi tên di chuyển vùng tô sáng và typeahead nhảy theo tiêu đề của các mục. Chọn một mục sẽ đóng danh sách và báo cáo giá trị của nó. Escape, nút quay lại và một lần nhấn bên ngoài đóng danh sách mà không thay đổi gì. `Empty` là văn bản hiển thị khi tìm kiếm không thấy kết quả nào.

## Khả năng truy cập

`Label` đặt tên cho trường dành cho công nghệ hỗ trợ. Dùng `LabelFunc` để cập nhật tên đó tại chỗ khi các signal của nó thay đổi. Một nhãn rỗng sẽ dùng `Placeholder`. Nút kích hoạt và danh sách mang các vai trò của hộp chọn, và mục được chọn sẽ được báo cáo. Bàn phím tới nút kích hoạt bằng Tab và mở nó bằng Return hoặc Space.

## Nền tảng

Trên macOS, Windows và Linux, danh sách là một popup bên cạnh nút kích hoạt. Trên iOS và Android, nó là một sheet, và hàng được chọn vẫn hiển thị trên trường. Cùng các tùy chọn và giá trị hoạt động trên mọi mục tiêu.

## Liên quan

- [Biểu mẫu](../forms.md): hộp chọn trong một biểu mẫu.
- [Menu](menu.md): các lệnh thay vì một giá trị.
- [Tabs](tabs.md): một vài tùy chọn được hiển thị cùng lúc.
- Các trang Select và Combobox trong gallery.
- [Thành phần không định kiểu](../authoring/base.md#select): `base.Select`, với các nhóm và một nút kích hoạt của riêng bạn.
