---
title: Menu
description: Một menu mở từ nút kích hoạt, một menu ngữ cảnh và các mục của chúng.
group: Thành phần
order: 42
---

`ui.Menu` mở các `ui.MenuItem` từ một nút: bên cạnh nút, hoặc dưới dạng một sheet trượt lên từ dưới với `Presentation: base.PresentSheet`, bất kể thiết bị nào. `ui.ContextMenu` mở cùng các mục đó tại vị trí con trỏ.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

Menu tự mở và tự đóng; `Open` và `OnOpenChange` cho phép ứng dụng nắm giữ trạng thái đó. `Trigger` là giao diện và nội dung của nút, và menu nhận các lần nhấn của nó. Một popup không bao giờ vượt quá khoảng trống bên cạnh nút kích hoạt: nó mở ở phía vừa trọn vẹn và các mục của nó cuộn khi vượt quá khoảng trống đó, hoặc vượt quá `MaxHeight`.

## Mục

Một `ui.MenuItem` hiển thị `Text`, một `Icon` tùy chọn và một `Shortcut` (`"mod+c"`, được viết theo cách của nền tảng trong popup), và báo cáo `OnSelect`, đóng menu trừ khi có `NoCloseOnSelect`. `Disabled` tắt mục, `Checked` hiển thị dấu kiểm bên cạnh, và `Variant: ui.MenuItemDanger` đánh dấu một mục mang tính phá hủy. `ui.MenuLabel` làm tiêu đề cho một nhóm và `ui.MenuSeparator` phân tách các nhóm. Một `ui.Submenu` chứa thêm các mục và mở bên cạnh mục đó hoặc, trong một sheet, thay thế cho các mục xung quanh.

```go preview=menu/checked height=320
ui.Menu{Trigger: ui.Button{Text: "View", Variant: ui.ButtonGhost}, Children: []core.Node{
	ui.MenuLabel{Text: "Sort by"},
	ui.MenuItem{Text: "Name", Checked: func() bool { return sort.Get() == "name" }, OnSelect: func() { sort.Set("name") }},
	ui.MenuItem{Text: "Date", Checked: func() bool { return sort.Get() == "date" }, OnSelect: func() { sort.Set("date") }},
	ui.MenuSeparator{},
	ui.Submenu{Text: "Share", Children: []core.Node{
		ui.MenuItem{Text: "Copy link", OnSelect: copyLink},
		ui.MenuItem{Text: "Email", OnSelect: emailLink},
	}},
}}
```

## Menu ngữ cảnh

`ui.ContextMenu` mở các mục của nó khi nhấp chuột phụ, hoặc nhấn giữ trên màn hình cảm ứng, tại vị trí con trỏ trên `Trigger` của nó.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Hành vi

Escape, nút quay lại và một lần nhấn bên ngoài sẽ đóng menu. Menu là một popup được định vị bởi `Placement` và `Align`, hoặc là một sheet trượt lên từ dưới khi được trình bày theo cách đó. Trên máy tính để bàn, Down, Up, Home, End, Page Down và Page Up di chuyển qua các mục đang bật, bắt đầu từ mục mà con trỏ đang nằm trên, Right mở một menu con và Left đóng nó, còn Return thực hiện lựa chọn. Một menu được mở bằng bàn phím sẽ đặt tiêu điểm vào mục đầu tiên, và tiêu điểm quay lại nút kích hoạt khi menu đóng.

## Khả năng truy cập

Menu báo cáo `RoleMenu` và mỗi mục báo cáo `RoleMenuItem`. `Label` đặt tên cho nút kích hoạt khi nội dung của nó không làm được điều đó. Trình đọc màn hình đọc văn bản của một mục và cho biết mục đó có bị tắt hay không.

## Nền tảng

Nút kích hoạt và các mục là của ứng dụng; sheet, popup, tiêu điểm, Escape và nút quay lại là của nền tảng. Ứng dụng chọn popup hay sheet; NSGo không tự chọn cái nào theo thiết bị.

## Liên quan

- [Select](select.md): một giá trị từ một danh sách.
- [Dialog](dialog.md): một bề mặt modal.
- [Tabs](tabs.md): các phần được hiển thị cùng lúc.
- Các trang Menu, ContextMenu và Command trong gallery.
- [Thành phần không định kiểu](../authoring/base.md#menu): `base.Menu`, cho một menu có giao diện của riêng bạn.
