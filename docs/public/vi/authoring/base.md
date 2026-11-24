---
title: Thành phần không kiểu dáng
description: Các điều khiển riêng của NSGo chưa gắn giao diện, dùng để tạo kiểu từng phần hoặc làm nền xây dựng thư viện có kiểu dáng.
group: Viết thành phần
order: 73
---

`base` chứa mọi điều khiển mà `ui` có, với đầy đủ hành vi, trạng thái và khả năng truy cập nhưng không có giao diện. Mỗi phần đều nhận `Class`, `Style` và `StyleFunc` ([Các phần và trạng thái](parts.md)), nhờ vậy một thư viện hoặc ứng dụng có thiết kế riêng có thể dùng trực tiếp chúng thay cho `ui`, hoặc bọc chúng lại như cách `ui` làm ([Giao diện và chủ đề](looks.md)). Trang này đề cập đến các điều khiển trong [tài liệu tham khảo thành phần](../ui.md); trang trưng bày (gallery) hiển thị từng điều khiển bên cạnh phiên bản `ui` của nó.

Các điều khiển như `Button`, `Checkbox`, `Select`, `Slider` và `NumberInput` nhận `Invalid: rejected.Get` để bám theo một signal kiểm tra tính hợp lệ. Hãy tạo kiểu cho trạng thái đó bằng `Class: "aria-invalid:border-red-500"` hoặc `Style.AriaInvalid`. Thuộc tính này cập nhật khả năng truy cập và kiểu dáng mà không vô hiệu hóa điều khiển. Các trường kết hợp sẽ truyền nó xuống trường nhập liệu thật và các phần trang trí; `ColorPicker` chia sẻ trạng thái này với các điều khiển con của nó.

Đối với các giá trị ứng dụng được kéo giữa các view, hãy sử dụng [`base/dragdrop`](../drag-drop.md): một khu vực kết nối các nguồn và đích trong khi ứng dụng tự cung cấp nội dung, bản xem trước và diện mạo của chúng.

## Button

`base.Button` lấy diện mạo từ `Style` và nội dung từ `Children`. `OnPress` chạy khi hoàn tất một lần nhấn; `OnPressDown` và `OnPressUp` theo dõi lúc bắt đầu và kết thúc lần nhấn để bạn tự tạo phản hồi.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected` và `Expanded` báo cáo trạng thái cho ứng dụng, kiểu dáng và công nghệ hỗ trợ: một hành động đang chạy dùng `Busy` (tạo kiểu với `aria-busy:`), một nút bật/tắt hoặc mục đang chọn trong danh sách dùng `Selected`, một phần tử đóng mở dùng `Expanded`. `NoFocus` loại nút bấm khỏi các điểm dừng Tab khi hàng hoặc danh sách đã điều hướng tới nó bằng cách khác.

## Checkbox

`base.Checkbox` nhận một kiểu cho ô vuông, một kiểu cho bộ chỉ báo và dấu tích của bạn bên trong nó, hiển thị dựa trên giá trị mà ứng dụng nắm giữ.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

Bộ chỉ báo lấy trạng thái từ ô vuông thông qua `PartStates`, nên các lớp `checked:` tạo kiểu được cho nó mà không biến nó thành một điểm dừng thứ hai.

Để nhận các phím bổ trợ, hãy thiết lập `OnChangeEvent func(checked bool, press core.PressEvent)`. Nó thay thế `OnChange` và giao quyền quyết định giá trị cho ứng dụng; `press.Modifiers.Shift` giúp một danh sách chọn theo dải. Thao tác kích hoạt bằng Space/Return trên máy tính để bàn, web, Android và iOS/tvOS vẫn giữ nguyên các phím bổ trợ trừ khi có phím tắt của ứng dụng xử lý phím đó trước; trên Windows, tổ hợp Alt cùng Space hoặc Return sẽ mở menu cửa sổ thay thế. Trong TSX, `onChangeEvent` nhận `{ checked, press }`.

## Switch

`base.Switch` gồm một rãnh trượt, một nút gạt bên trong và các phần tử con đóng vai trò nhãn. Một lần nhấn gửi về giá trị mới mà người dùng yêu cầu, ngược lại với `Checked`.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

Rãnh trượt và nút gạt nhận `StyleFunc` riêng, và kiểu trạng thái của chúng bám theo `Checked`.

## Slider

`base.Slider` nhận một `base.SliderValue`: `base.SingleValue(v)` hoặc `base.RangeValue(low, high)`. Nó tạo kiểu tách biệt cho rãnh trượt, thanh chỉ báo, vùng khoảng giá trị và các nút kéo.

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max` và `Step` là các hàm getter phản ứng: khi signal của chúng thay đổi, thanh trượt đang gắn trên cây và dải giá trị trợ năng của nó sẽ cập nhật theo, kể cả trong lúc đang kéo. `Axis` chuyển thanh trượt sang chiều dọc và `Inverted` đặt giá trị nhỏ nhất ở đầu bên kia, tương tự như `base.Progress`; `Marks` đặt các vạch chia của riêng bạn trên thanh chỉ báo, và `OnChangeEnd` báo cáo giá trị sau khi kết thúc thao tác kéo.

## Select

`base.Select` nhận danh sách `base.SelectItem` và một giá trị `*string`, nên trạng thái “chưa chọn” là `nil`. Bạn tự tạo kiểu cho nút kích hoạt, cửa sổ bật lên, các hàng và các nhóm của nó.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Các mục liền kề có cùng `Group` dùng chung một tiêu đề nhóm, tính năng gõ tìm nhanh (typeahead) khớp theo tiêu đề của mục, `SearchField` vẽ trường tìm kiếm của riêng bạn, và `Cleanable` hiển thị nút xóa lựa chọn. `OnCancel` lắng nghe phím Escape, nút quay lại và cú nhấn ra bên ngoài.

## Tabs

`base.Tabs` chứa các `base.Tab` và vẽ thanh chỉ báo từ `IndicatorStyle`; ứng dụng tự quản lý thẻ nào đang được chọn. `base.Tab.OnPress` chạy trên mọi lần nhấn, bao gồm cả thẻ đang được chọn.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` nhận các kiểu cho lớp nền mờ (backdrop), lớp phủ và hộp thoại bật lên, cùng một `Animation` hoặc `TransitionShader`, và các phần tử con của bạn. `Open`, `DefaultOpen` và `OnOpenChange` điều khiển trạng thái đóng mở; `OnOpen`, `OnClose` và `OnCancel` báo cáo điều gì đã xảy ra. `OnConfirm` là hành động mặc định: phím Return sẽ chạy nó, trừ khi tiêu điểm bàn phím đang nằm trên một nút bấm hay trường nhập liệu tự xử lý phím Return hoặc có một modal khác đang đè lên hộp thoại, sau đó hộp thoại đóng lại với `ReasonSelect`. `Role` và `AccessibilityDescription` là nội dung trình đọc màn hình sẽ thông báo, dùng `RoleAlertDialog` cho hộp thoại xác nhận. `NoCloseOnSwipe` ngăn thao tác vuốt của iOS đóng hộp thoại, `Presentation` chọn kiểu trình bày trên iOS, và `StatusBar` thay đổi thanh trạng thái trong khi hộp thoại mở.

`base.Sheet` là một bảng trượt ở bất kỳ cạnh nào, kèm lớp nền mờ và nội dung của bạn; khi bật `CloseOnSwipe`, bảng trượt ở cạnh dưới sẽ bám theo thao tác kéo xuống và đóng lại khi vượt quá ngưỡng. Nó nhận `OnConfirm`, `Role` và `AccessibilityDescription` giống như `base.Dialog`. `base.ShowAlertDialog` hiển thị hộp thoại cảnh báo hoặc bảng hành động hệ thống với các nút phân theo vai trò:

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` mở các `base.MenuItem` từ nút kích hoạt của riêng bạn, còn `base.ContextMenu` mở tại vị trí con trỏ. Một mục báo về `OnSelect` và đóng menu trừ khi bật `NoCloseOnSelect`; `Checked` biến nó thành mục có thể đánh dấu, `Indicator` vẽ dấu đánh dấu của bạn trước nội dung, và `Role` chọn cách công nghệ hỗ trợ đọc nó: mục menu, hộp kiểm hoặc mục radio. `base.Submenu` chứa thêm các mục con bên trong.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset` và `Margin` định vị cửa sổ bật lên trên máy tính để bàn, còn `PopupStyle` và `BackdropStyle` tạo kiểu cho nó.

## EmojiPicker

`base.EmojiPicker` tìm kiếm toàn bộ biểu tượng cảm xúc Unicode theo tên CLDR đã bản địa hóa, từ khóa và mã ngắn trong các ngôn ngữ của ứng dụng (hoặc chỉ định qua `Languages` / `LanguagesFunc`), đi kèm thẻ danh mục, các mục `Recent` tùy chọn, bộ chọn tông màu da, điều hướng bàn phím và lưới ảo hóa. Mọi nhãn mặc định và tên danh mục đều lấy từ `base/locales`, và bộ dữ liệu emoji chỉ được liên kết vào những ứng dụng có sử dụng `EmojiPicker` hoặc các hàm hỗ trợ của nó (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`).

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` và `DefaultCategory` chọn nhóm hiển thị để duyệt. Các thẻ danh mục và các lựa chọn tông màu da đều là một điểm dừng Tab duy nhất với điều hướng bằng phím mũi tên. `EmojiPickerRef` cung cấp `Focus`, `Query`, `SetQuery`, `SetCategory` và `MatchedCount`; bộ chọn có kiểu dáng cũng sử dụng cùng trạng thái truy vấn này. `LabelFunc`, `SearchLabelFunc` và `PlaceholderFunc` cập nhật từ ngữ ngay tại chỗ. `Emoji.WithSkinTone` áp dụng tông màu đã chọn cho cả hai người trong một cặp, và tông mặc định khôi phục dạng trung tính; việc tìm kiếm bằng chính một emoji sẽ giữ nguyên chuỗi ký tự chính xác của nó.

Hãy dùng `ui.EmojiPicker` cho bộ chọn đã gắn chủ đề với các biểu tượng danh mục Lucide, trường tìm kiếm có nút xóa nhanh, ô xem trước emoji được tô sáng và bảng chọn tông màu da.
