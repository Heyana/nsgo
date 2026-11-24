---
title: Khả năng truy cập
description: Nhãn, vai trò, giá trị, hành động và tiêu điểm cho trình đọc màn hình và bàn phím.
group: Khái niệm cốt lõi
order: 61
---

## Nội dung trình đọc màn hình đọc

`Label` của một view là nội dung mà công nghệ hỗ trợ sẽ đọc lên, `AccessibilityDescription` cung cấp thông tin bổ sung được đọc sau nhãn, `Hint` cho biết kết quả khi kích hoạt, và `Role` xác định bản chất của phần tử: `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable` và các vai trò khác. Theo mặc định, một `RoleStatus` là một vùng cập nhật động lịch sự (polite live region); một `RoleSearchField` là trường văn bản được thông báo như hộp tìm kiếm. Một hộp thoại sẽ được đọc kèm vai trò và nhãn khi mở ra, tiếp theo là mô tả của nó, giống như tiêu đề và mô tả của `ui.Dialog`. Văn bản không có `Label` sẽ được đọc theo chính nội dung của nó. `LabelFunc`, `HintFunc` và `AccessibilityDescriptionFunc` liên kết các đoạn văn bản trợ năng thay đổi động trên cùng một view, nhờ đó các điều khiển giữ nguyên tiêu điểm và trạng thái khi chuyển đổi ngôn ngữ. Tất cả các phần tử cốt lõi có nhãn đều nhận `LabelFunc`, còn `View` và `Text` nhận `HintFunc`.

Sử dụng `TextFunc` cho văn bản thay đổi động trên `ui.Button`, `ui.Checkbox`, `ui.MenuItem` và `ui.AlertAction`. Tên trợ năng của chúng sẽ tự động cập nhật theo văn bản đó. Nút bấm và hộp kiểm cũng hỗ trợ truyền riêng `LabelFunc`. Thanh trượt, thẻ (tab), công tắc, thanh tiến trình và nút kích hoạt cửa sổ bật lên đều nhận `LabelFunc` để thay đổi tên trợ năng. `ui.Option.LabelFunc` cung cấp văn bản cho các tùy chọn trong thẻ, hộp chọn và combobox. Trường nhập số, trường nhập mã, vòng tiến trình và cả hai biến thể của con quay tải (spinner) cũng nhận `LabelFunc`.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` cung cấp một khoảng giá trị hoặc một giá trị dạng văn bản: chẳng hạn giá trị nhỏ nhất, lớn nhất và hiện tại của thanh trượt, hoặc chuỗi văn bản như "3 items". `Accessible` gộp một view thành một phần tử duy nhất để tên và giá trên một thẻ được đọc liền mạch thay vì tách thành hai điểm dừng, và khi không có nhãn, view sẽ đọc văn bản của các phần tử con bên trong nó. `AccessibilityElementsHidden` ẩn các thành phần trang trí như biểu tượng mũi tên hay đường phân cách, trong khi `ImportantForAccessibility` ẩn toàn bộ cây con.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## Nhãn hiển thị

Gán `Ref` của điều khiển vào `LabelFor` trên một `core.Text` hoặc `core.View`. Khi nhấn vào nhãn, tiêu điểm sẽ chuyển đến điều khiển và kích hoạt hành động nhấn của nó, chẳng hạn như bật/tắt hộp kiểm. Văn bản đang hiển thị của nhãn sẽ tự động đặt tên cho điều khiển mà không cần lặp lại chuỗi `Label`, đồng thời kế thừa trạng thái `Disabled` và `Invalid` của điều khiển cho cả khả năng truy cập lẫn kiểu dáng. Bản thân nhãn không nhận vai trò nút bấm hay điểm dừng Tab.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` đặt tên cho một điều khiển từ nhiều phần tử hiển thị theo đúng thứ tự truyền vào, ưu tiên trước `Label`; thuộc tính này không chuyển tiếp sự kiện nhấn. Trong trường hợp khác, `Label` được ưu tiên hơn tên ngầm định từ `LabelFor` (vốn có thứ tự theo cấu trúc cây). Các thay đổi về văn bản, vị trí, trạng thái ẩn, việc gỡ bỏ và gắn lại đều tự động cập nhật mối liên kết này. Các tham chiếu bị thiếu, trùng lặp, vòng lặp hoặc đang bị ẩn sẽ bị bỏ qua; các liên kết chỉ có hiệu lực trong cùng một bề mặt và phạm vi giới hạn (confinement scope). Một nhãn không thể kích hoạt điều khiển nằm phía sau hộp thoại modal.

Các nút bấm, hộp kiểm, nút chọn đơn, công tắc, hộp chọn và trường nhập số trong `base` và `ui` đều truyền `Ref` và `LabelledBy` xuống điều khiển gốc; thanh trượt cung cấp `ThumbRef` và `StartThumbRef`. Trong TSX, hãy dùng `labelFor={field()}` và `labelledBy={[label()]}` với các signal nút được gán qua `ref={node => field.set(node)}`, để nhãn đứng trước trường nhập liệu tự cập nhật khi trường đó được gắn vào cây.

## Hành động

`AccessibilityActions` liệt kê những hành động mà trình đọc màn hình cung cấp trên một phần tử: gồm các hành động chuẩn (`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`) và hành động riêng của ứng dụng, mỗi hành động gồm một `Name` và một `Label`. `OnAccessibilityAction` sẽ nhận về tên hành động đó. Hãy dùng `AccessibilityActionsFunc` trên `core.View` hoặc `core.Input` để thay đổi danh sách hành động khả dụng hoặc dịch nhãn của chúng trong khi phần tử vẫn đang được gắn trên cây.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` đánh dấu một lưới, hộp danh sách (listbox) hoặc cây cho phép chọn nhiều mục cùng lúc. `base.DataTable` thiết lập thuộc tính này cho chế độ chọn nhiều hàng; nền tảng web, Windows và Linux sẽ xuất ra cờ trợ năng tương ứng.

`Expanded` cho biết một phần tử đóng mở hoặc nút menu đang mở hay đóng, `Busy` cho biết nội dung đang tải, `Invalid` cho biết ứng dụng đã từ chối nội dung trường nhập hoặc giá trị của điều khiển (được tạo kiểu qua `aria-invalid:`), còn `Checked`, `Selected` và `Pressed` báo cáo trạng thái của điều khiển. Các phần con của một điều khiển có thể nhận trạng thái của nó thông qua `PartStates`, nhờ đó bộ chỉ báo đi theo hộp kiểm của nó mà không tạo thêm điểm dừng riêng.

`SelectionItem` đánh dấu một view là một mục trong tập hợp có thể chọn, độc lập với giá trị `Selected` hiện tại của nó. `base.DataTable` gắn thuộc tính này lên các hàng hoặc ô khi bảng cho phép chọn chúng, và `base.Tree` gắn lên mọi hàng; Windows giữ pattern SelectionItem còn web ghi `aria-selected="true"` hoặc `"false"`. Một bảng không hỗ trợ chọn sẽ không gắn các dấu hiệu này lên hàng.

## Thay đổi động trực tiếp

`LiveRegion` khiến trình đọc màn hình tự động thông báo nhãn và văn bản bên trong một view khi chúng thay đổi: `core.LiveRegionPolite` đợi đến khi người dùng rảnh, còn `LiveRegionAssertive` ngắt ngang ngay lập tức. `cx.Announce` phát một thông báo đơn lẻ một lần.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## Tiêu điểm và bàn phím

Chú giải công cụ (tooltip) mô tả nút kích hoạt trước khi nút đó nhận tiêu điểm. `View.Tooltip`, `base.Tooltip` và `ui.Tooltip` dùng văn bản hiện tại của chúng làm mô tả trợ năng cho điều khiển, trừ khi văn bản đó lặp lại tên của điều khiển, ứng dụng đã cung cấp `AccessibilityDescription`, hoặc tooltip đang ở trạng thái `Disabled`. Một lớp bọc (wrapper) sẽ truyền mô tả này xuống các điều khiển bên trong nó. Bong bóng tooltip và các phần tử con của nó được ẩn khỏi trình đọc màn hình và không bao giờ phát thông báo cảnh báo; nhờ đó tooltip mặc định của `ui.IconButton` không lặp lại nhãn của nút.

`base.DataTable` hiển thị lưới có tên của mình làm điểm dừng tiêu điểm cho bàn phím và công nghệ hỗ trợ; lớp nhận con trỏ được ẩn khỏi công nghệ hỗ trợ.

Trên máy tính để bàn, web và Apple TV, các view nhận sự kiện nhấn đều là điểm dừng Tab bên cạnh các trường văn bản; `NoFocus` loại bỏ một view khỏi thứ tự Tab và `Focusable` biến một view không có sự kiện nhấn thành điểm dừng. `Style.FocusVisible` chỉ vẽ vòng tiêu điểm khi bàn phím di chuyển tiêu điểm đến (`focus-visible:`), `Style.Focus` áp dụng cho mọi trạng thái có tiêu điểm và `Style.FocusWithin` áp dụng khi tiêu điểm nằm bên trong view; `OnFocus` và `OnBlur` lắng nghe sự kiện đó (trong đó `OnBlur` cũng chạy khi view đang giữ tiêu điểm rời khỏi cây), còn `Ref.Focus()` và `Ref.SetAccessibilityFocus()` giúp di chuyển tiêu điểm bằng mã; `core.FocusedRef(cx)` trả về tham chiếu đến view hiện đang giữ tiêu điểm, hữu ích cho những thành phần mở ra không thông qua nút kích hoạt (như thanh tìm kiếm mở bằng phím tắt) để trả lại tiêu điểm khi đóng. Phím Return và Space nhấn vào view đang có tiêu điểm, còn các phím mũi tên di chuyển qua danh sách thẻ, nhóm radio và menu. `Tooltip` của một view sẽ hiện ngay khi bàn phím di chuyển tiêu điểm tới nó, cho đến khi tiêu điểm rời đi hoặc người dùng nhấn Escape. Hàm `OnOpen` của cửa sổ bật lên chạy ngay sau khi nội dung của nó đã sẵn sàng, vì vậy gọi `Focus` tại đó sẽ đưa tiêu điểm vào bên trong như cách một hộp thoại không khóa nền nên làm; nội dung của một popover đứng ngay sau nút kích hoạt của nó trong thứ tự Tab, và tiêu điểm quay trở lại nút kích hoạt khi cửa sổ bật lên đóng lại.

Trên Android, iPhone và iPad, `Ref.Focus()` và `AutoFocus` của nút bấm có thể đặt tiêu điểm vào một view đang bật có thể nhấn hoặc có `Focusable` để sử dụng với bàn phím cứng. Việc chạm tay vào điều khiển không di chuyển tiêu điểm này. Phím Space và Return kích hoạt phần tử có thể nhấn đang giữ tiêu điểm sau khi qua keymap, đồng thời bảo toàn các phím bổ trợ Control, Alt, Shift và phím hệ thống trong mọi pha nhấn. `NoFocus`, các điều khiển bị vô hiệu hóa hoặc bị ẩn, và các điều khiển nằm sau modal không thể nhận tiêu điểm này. Đặt tiêu điểm lên một view không làm bật bàn phím ảo; các trường nhập liệu gốc vẫn giữ hành vi chỉnh sửa của chúng. watchOS và các mini program không có luồng kích hoạt bằng bàn phím vật lý.

`cx.Env().ScreenReader()` cho biết trình đọc màn hình có đang chạy hay không, `cx.Env().ReduceMotion()` cho biết người dùng có yêu cầu giảm chuyển động hay không, `cx.Env().IncreaseContrast()` cho biết người dùng có yêu cầu tăng độ tương phản hay không (Increase Contrast trên macOS, iOS và watchOS, giao diện tương phản trên Windows, High Contrast của GNOME, thiết lập tương phản của GTK 4.20 hoặc giao diện GTK tương phản cao, thiết lập tương phản trên mức chuẩn của Android 14, `prefers-contrast: more` hoặc `forced-colors: active` trên web), `cx.Env().FontScale()` trả về hệ số cỡ chữ, và `cx.Env().RTL()` cho biết hướng bố cục. Văn bản tự co giãn theo cỡ chữ hệ thống trừ khi bật `NoFontScaling`, và bố cục từ phải sang trái sẽ tự động đảo ngược các cạnh vật lý khi bạn sử dụng các cạnh logic.

## Tiếp theo

- [Kiểu dáng và giao diện](styles.md): các trạng thái tiêu điểm và vô hiệu hóa.
- [Biểu mẫu](forms.md): nhãn và giá trị cho các trường nhập liệu.
- [Canvas và shader](canvas.md): hiển thị các phần tử của bản vẽ cho trình hỗ trợ.
