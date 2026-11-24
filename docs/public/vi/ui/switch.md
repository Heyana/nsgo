---
title: Công tắc
description: Một thiết lập bật hoặc tắt, với ba kích thước.
group: Thành phần
order: 35
---

`ui.Switch` là một thiết lập có hiệu lực ngay lập tức. Nó không giữ giá trị: ứng dụng sở hữu giá trị đó và cập nhật nó trong `OnChange`.

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## Kích thước

`ui.Switch` nhận `Checked`, `OnChange`, `Label` và `Size` (`SizeDefault`, `SizeSmall` hoặc `SizeLarge`). Núm gạt trượt đi và rãnh đổi màu theo đồng hồ khung hình của nền tảng.

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

Công tắc dành cho một thiết lập áp dụng ngay lập tức. Với một lựa chọn bên trong biểu mẫu, hãy dùng [hộp kiểm](checkbox.md); với một trong nhiều tùy chọn, hãy dùng các `ui.Radio` trong một `ui.RadioGroup` hoặc một [hộp chọn](select.md).

## Khả năng truy cập

Công tắc báo cáo `RoleSwitch` và trạng thái bật hoặc tắt của nó. `Label` là những gì công nghệ hỗ trợ đọc, và văn bản bên cạnh công tắc thường là nhãn của nó. Phím Space hoặc thao tác nhấn sẽ bật/tắt công tắc khi nó có tiêu điểm bàn phím.

## Nền tảng

Điều khiển mặc định do ứng dụng vẽ, nên trông giống nhau ở mọi nơi. `Variant: ui.SwitchSystem` yêu cầu điều khiển của nền tảng: NSSwitch trên macOS, UISwitch trên iOS và công tắc 52 × 32 dp của Material 3 trên Android. Các nền tảng khác dùng giao diện mặc định. Thao tác nhấn, tiêu điểm và trình đọc màn hình là của nền tảng. Công tắc mặc định trên điện thoại có cùng kích thước với công tắc trên máy tính để bàn theo chủ ý thiết kế; `SizeSmall` dành cho các hàng dày đặc, không dành cho một loại thiết bị.

## Liên quan

- [Hộp kiểm](checkbox.md): một hoặc nhiều lựa chọn.
- [Biểu mẫu](../forms.md): các thiết lập trong một biểu mẫu.
- [Khả năng truy cập](../accessibility.md): nhãn và tiêu điểm.
- Trang Switch của gallery.
- [Thành phần không kiểu dáng](../authoring/base.md#switch): `base.Switch`, cho một công tắc với giao diện của riêng bạn.
