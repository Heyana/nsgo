---
title: Các phần và trạng thái
description: Không giữ trạng thái thuộc sở hữu của ứng dụng, tạo kiểu cho từng phần và cho công nghệ hỗ trợ biết bản chất của thành phần.
group: Viết thành phần
order: 71
---

Một thành phần không kiểu dáng quyết định cách điều khiển hoạt động và giao lại toàn quyền hiển thị cho người sử dụng. Trang này xây dựng `Toggle`, một nút hình viên thuốc có trạng thái bật hoặc tắt, theo đúng cách `base` xây dựng các điều khiển của mình.

## Giá trị được kiểm soát

Một điều khiển không tự giữ giá trị thuộc quyền sở hữu của ứng dụng: ứng dụng truyền giá trị vào dưới dạng hàm và cập nhật lại từ hàm gọi lại (callback). Thành phần chỉ đọc giá trị đó tại đúng nơi nó hiển thị, nhờ vậy chỉ phần đó cập nhật lại khi giá trị thay đổi.

```go
// Toggle is a pill that is on or off. Its parts are the pill and a dot
// inside it, then its children, the label.
type Toggle struct {
	On       func() bool
	Disabled func() bool
	// OnChange receives the value a press asks for, the opposite of On's.
	OnChange func(on bool)

	Class        string
	Style        core.Style
	StyleFunc    func() core.Style
	DotClass     string
	DotStyle     core.Style
	DotStyleFunc func() core.Style
	Children     []core.Node
	Label        string
}
```

Ứng dụng lưu giữ giá trị trong một signal: `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## Các phần

Mỗi phần mà thành phần vẽ ra đều nhận `Class`, `Style` và `StyleFunc`, được đặt tên theo phần đó (`DotClass`, `DotStyle`, `DotStyleFunc`). `Style` là tĩnh, `StyleFunc` tự động cập nhật theo các signal mà nó đọc, và `Class` nằm bên dưới cả hai, nhờ vậy ứng dụng có thể dùng bất kỳ cách nào họ quen viết.

Một phần nhận trạng thái của điều khiển thông qua `PartStates` thay vì tự báo cáo trạng thái riêng, nhờ đó các lớp `checked:` và kiểu `Checked` áp dụng được cho nó trong khi công nghệ hỗ trợ vẫn chỉ thấy một điều khiển duy nhất:

```go
func (t Toggle) Render(*core.Scope) core.Node {
	on := func() bool { return t.On != nil && t.On() }
	off := func() bool { return t.Disabled != nil && t.Disabled() }
	shows := func() core.PartStates { return core.PartStates{Checked: on(), Disabled: off()} }
	dot := core.View{Class: t.DotClass, Style: t.DotStyle, StyleFunc: t.DotStyleFunc, PartStates: shows}
	return core.View{
		Class: t.Class, Style: t.Style, StyleFunc: t.StyleFunc,
		Role: core.RoleSwitch, Label: t.Label, Checked: on, Disabled: t.Disabled,
		OnPress: func(core.PressEvent) {
			if t.OnChange != nil {
				t.OnChange(!on())
			}
		},
		Children: append([]core.Node{dot}, t.Children...),
	}
}
```

Ứng dụng có thể tạo kiểu cho nó chỉ bằng các lớp:

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

Tương tự, `PartStates.Invalid` áp dụng các lớp `aria-invalid:` và `Style.AriaInvalid` cho một phần trang trí, bao gồm cả kiểu `group-aria-invalid:` bên trong nó, mà không báo cáo thêm một điều khiển không hợp lệ nữa. Hãy đặt `Invalid` lên chính điều khiển thật hoặc trường nhập liệu gốc để công nghệ hỗ trợ nhận được trạng thái hợp lệ của nó.

## Khả năng truy cập

Phần tử gốc của điều khiển cho biết nó là gì: một `Role`, một `Label` khi văn bản bên trong chưa diễn đạt đủ, một `Hint` mô tả tác dụng khi kích hoạt, cùng các trạng thái `Checked`, `Selected`, `Disabled`, `Invalid` hoặc `Expanded` tùy theo ngữ cảnh. Các phần con bên trong giữ im lặng. Một điều khiển cho phép di chuyển giữa các mục bằng phím mũi tên chỉ tính là một điểm dừng Tab, giống như danh sách thẻ. [Khả năng truy cập](../accessibility.md) liệt kê đầy đủ các vai trò và hành động.

## Những thuộc tính cần truyền tiếp

Một thành phần có gốc là view riêng của chính nó nên nhận `OnLayout`, `Tooltip` và `TestID` của view đó rồi truyền tiếp xuống, để các ứng dụng có thể đo đạc, chú giải và kiểm thử nó như mọi view bình thường. Các hàm gọi lại nên để tùy chọn: một `OnChange` bằng nil tạo ra điều khiển chỉ hiển thị giá trị mà không làm thay đổi nó.
