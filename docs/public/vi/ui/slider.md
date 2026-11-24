---
title: Thanh trượt
description: Một giá trị hoặc một khoảng giá trị dọc theo rãnh trượt, điều khiển bằng bàn phím và con trỏ.
group: Thành phần
order: 37
---

`ui.Slider` chọn một giá trị dọc theo rãnh trượt, bằng thao tác kéo, theo bước và bằng bàn phím. Một giá trị là một núm trượt duy nhất; `High` tạo thành một khoảng gồm hai núm.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## Một khoảng giá trị

Đặt `High` và `OnHighChange` để chọn hai giá trị. `Value` là đầu thấp. `LowLabel` và `HighLabel` đặt tên cho các núm trượt dành cho công nghệ hỗ trợ.

```go preview=slider/range
price := core.NewSignal(cx, 20.0)
ceiling := core.NewSignal(cx, 80.0)
ui.Slider{
	Value: price.Get, OnChange: price.Set,
	High: ceiling.Get, OnHighChange: ceiling.Set,
	Min: 0, Max: 100, Step: 1,
	Label: "Price", LowLabel: "Minimum", HighLabel: "Maximum",
}
```

## Bước và giá trị

`Step` làm giá trị bắt vào các nấc; bước bằng 0 cho phép mọi giá trị. `ValueText` thay đổi những gì công nghệ hỗ trợ đọc làm giá trị, còn `Disabled` vô hiệu hóa thanh trượt.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

Dùng `MinFunc`, `MaxFunc` và `StepFunc` khi khoảng giá trị hoặc bước thay đổi, chẳng hạn khi một video báo cáo thời lượng của nó. Chúng ghi đè các trường tĩnh tương ứng và cập nhật thanh trượt đã được gắn, giữ nguyên tiêu điểm và mọi thao tác kéo đang diễn ra. Một `MaxFunc` tường minh trả về 0 cho phép khoảng giá trị bị thu gọn cho đến khi có thời lượng; giá trị của ứng dụng được giữ nguyên và hiển thị trong giới hạn hiện tại.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

Trong TSX, `min={minimum()}`, `max={duration()}` và `step={step()}` bám theo signal của chúng ngay tại chỗ; `max={0}` thu gọn khoảng giá trị, giống như một `MaxFunc` trả về 0.

## Hướng

`Axis: base.Vertical` dựng đứng thanh trượt, với giá trị nhỏ nhất ở dưới cùng; hãy cho nó một chiều cao trong `Style`. `Inverted` đặt giá trị nhỏ nhất ở đầu kia: phía trên của thanh trượt dọc, phía cuối của thanh trượt ngang. Các phím mũi tên dọc theo rãnh trượt tuân theo hướng này. `ui.Progress` nhận cùng `Axis` và `Inverted`.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## Hành vi

Thao tác kéo bám theo con trỏ và báo `OnChange` trong khi di chuyển. Các phím mũi tên di chuyển núm trượt đang có tiêu điểm một bước, Home và End đưa về hai đầu, còn Page Up và Page Down di chuyển xa hơn. `ui.Slider` đánh dấu các bước bên dưới rãnh trượt khi `Step` lớn hơn 0, tối đa 60 vạch.

## Khả năng truy cập

Thanh trượt báo cáo `RoleAdjustable`, `Label` và giá trị của nó; một khoảng giá trị có cả hai núm trượt với nhãn riêng. `ValueText` được đọc thay cho con số. Bàn phím và các thao tác tăng, giảm của trình đọc màn hình làm thay đổi giá trị.

## Nền tảng

Rãnh trượt do ứng dụng vẽ và có cùng kích thước ở mọi nơi. Thao tác nhấn, tiêu điểm và trình đọc màn hình là của nền tảng; thao tác kéo bằng cảm ứng hoạt động như kéo bằng con trỏ với cùng bước.

## Liên quan

- [Biểu mẫu](../forms.md): thanh trượt trong một biểu mẫu.
- [Khả năng truy cập](../accessibility.md): giá trị và vai trò có thể điều chỉnh.
- Các trang Slider, RangeSlider và Rating của gallery.
- [Thành phần không kiểu dáng](../authoring/base.md#slider): `base.Slider`, dạng dọc và với các vạch của riêng bạn.
