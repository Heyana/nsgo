---
title: Hoạt ảnh
description: Chuyển tiếp, giá trị hoạt ảnh, hoạt ảnh bố cục và các mẫu chuyển động chạy trên đồng hồ khung hình của nền tảng.
group: Khái niệm cốt lõi
order: 58
---

## Giá trị hoạt ảnh

Một hoạt ảnh chạy trên luồng chính của nền tảng, được điều nhịp theo đồng hồ khung hình riêng của nền tảng đó, nên không tốn chi phí cho engine trên mỗi khung hình. `AnimatedValue` là một con số do nền tảng di chuyển; `Style.Animated` gắn độ mờ, phép tịnh tiến, tỉ lệ, góc xoay, màu nền, màu viền, đổ bóng và màu chữ của một view vào giá trị này.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring` và `Decay` dịch chuyển một giá trị, còn `Sequence`, `Parallel`, `Stagger`, `Delay` và `Loop` kết hợp chúng lại. `cx.StartAnimation` trả về một `AnimationRun` có phương thức `Stop` để dừng lượt chạy tại vị trí hiện tại và `Reset` để đưa các giá trị về ban đầu. Một lượt chạy bắt đầu từ vị trí hiện tại của giá trị, và một hoạt ảnh mới tác động lên cùng giá trị sẽ kết thúc hoạt ảnh cũ.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

Các nút dẫn xuất tự động bám theo một giá trị: `core.Interpolate` ánh xạ các con số hoặc màu sắc với hàm làm mượt và ngoại suy riêng, còn `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp` và `SnappingDiffClamp` tính toán các giá trị khác từ đó. `ui.Lerp` là hàm nội suy rút gọn.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` chạy một hàm một lần vào khung hình hiển thị tiếp theo, và `Scope.Animate` chia bước theo một khoảng thời gian trên đồng hồ đó. Cả hai đều chạy trên goroutine của đồ thị nên có thể gán giá trị cho các signal. Hàm nhận vào thời gian của khung hình — chính là thời gian hiển thị thực tế của màn hình trên macOS, iOS, Android, web, Linux, Windows, HarmonyOS và watchOS (trên HarmonyOS khi máy build có OpenHarmony SDK; ứng dụng đóng gói thiếu SDK sẽ dùng đồng hồ của engine) — nhờ vậy một vòng lặp dịch chuyển đối tượng theo khoảng thời gian giữa hai khung hình sẽ bám sát nhịp hiển thị; một bài kiểm thử `nsgotest` sẽ truyền thời gian của đồng hồ kiểm thử.

Thiết lập tốc độ gọi lại của cửa sổ bằng `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})`. Truyền `Minimum` và `Maximum` để cấp một dải tần cho liên kết hiển thị, chẳng hạn `{Minimum: 80, Maximum: 120, Preferred: 120}` trên màn hình ProMotion. Hãy gọi hàm này từ chính scope của cửa sổ. Thiết lập áp dụng cho `RequestAnimationFrame` và `Animate` cho đến khi thay đổi lại. Truyền một dải giá trị bằng 0 để khôi phục nhịp mặc định của màn hình. Thời gian truyền vào callback vẫn là thời gian hiển thị thực tế. Màn hình và hệ điều hành sẽ chọn tần số quét khả dụng, và NSGo có thể bỏ qua một số khung hình hiển thị để điều tiết nhịp callback.


`Scope.Tween` tăng giảm một con số theo từng bước trên đồng hồ đó cho những thứ mà giá trị hoạt ảnh không chạm tới được, chẳng hạn như con số đếm tăng dần trong văn bản: nó gọi một hàm với từng giá trị từ đầu đến cuối giống như cách `Timing` dịch chuyển một giá trị, và gọi thêm một lần với giá trị cuối. Một bước `nsgotest` khởi chạy nó sẽ đợi đến giá trị cuối giống như với hoạt ảnh, và khi bật chế độ giảm chuyển động, nó trả về giá trị cuối ngay lập tức. `Animated.tween` của TSX cũng hoạt động tương tự.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## Chuyển tiếp

`Transition` tạo hoạt ảnh cho kiểu dáng hoặc lớp (class) của một view khi chúng thay đổi, thực thi trực tiếp trên nền tảng mà không tốn công sức của engine trên từng khung hình. `Properties` quy định những thuộc tính nào dịch chuyển; giá trị 0 có nghĩa là mọi thuộc tính ngoại trừ khung vị trí/kích thước (frame).

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` dịch chuyển và thay đổi kích thước một view từ vị trí đang hiển thị khi kiểu dáng của chính nó gán cho nó một khung mới. Ngược lại, hoạt ảnh bố cục tạo hoạt ảnh cho toàn bộ một commit, giống như `LayoutAnimation` của React Native: `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` tạo hoạt ảnh cho các thao tác di chuyển, chèn và xóa của đợt cập nhật tiếp theo, với các mẫu sẵn có cho làm mượt (easing), tuyến tính và lò xo (spring). Một chuyển tiếp `LayoutSpring` nhận `Response` (chu kỳ tự nhiên không tắt dần của lò xo) và `DampingRatio` (`< 1` tắt dần dưới hạn, `1` tắt dần tới hạn, `> 1` tắt dần quá hạn), chạy cho đến khi ổn định thay vì cố định thời lượng, đồng thời mang theo vị trí và vận tốc hiện tại sang đích mới nếu bị đổi hướng giữa chừng.

```go
cx.AnimateLayout(core.LayoutAnimation{
	Update: &core.LayoutTransition{
		Curve:        core.LayoutSpring,
		Response:     280 * time.Millisecond,
		DampingRatio: 0.85,
	},
})
open.Set(!open.Get())
```

`View.OnTransitionEnd` chạy một lần sau khi thay đổi đã hoàn tất quá trình chuyển tiếp, bất kể có thuộc tính nào dịch chuyển hay không. Chế độ giảm chuyển động sẽ tắt các chuyển tiếp và hoạt ảnh bố cục, vì vậy hãy tôn trọng cài đặt của nền tảng bằng cách đảm bảo trạng thái cuối luôn đạt tới được.

## Chuyển động

`core.Motion` dịch chuyển đồng thời độ mờ, phép tịnh tiến, tỉ lệ và góc xoay của một view qua các khung hình khóa (keyframe), dựa trên một `MotionSpec` của ứng dụng hoặc một `MotionPreset` có sẵn: xuất hiện, biến mất, thu hút chú ý và lặp lại. Gắn kiểu dáng của nó sẽ đặt view ở trạng thái nghỉ; `Play` chạy một spec, còn `Exit` cho phép view rời đi cùng chuyển động của nó bên trong một `core.Presence`.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` giữ lại một nhánh cho đến khi mọi chuyển động thoát đã đăng ký của nó hoàn tất, nhờ vậy một trang tính (sheet) hay thông báo nổi (toast) luôn rời đi trọn vẹn với hiệu ứng của mình dù bất kỳ điều gì gỡ bỏ nó. `core.PresenceOf` áp dụng hành vi tương tự với một khóa so sánh được và hàm nội dung, còn `MaxLeaving` giới hạn số lượng nhánh cũ đang rời đi khi giá trị thay đổi nhanh. `MotionPresets()` liệt kê mọi mẫu có sẵn và `Kind()` cho biết mẫu đó dùng để xuất hiện, biến mất, thu hút chú ý hay lặp.

## Khung hình khóa

`KeyframeAnimation` lặp lại các khung hình khóa trên nền tảng: độ mờ, tỉ lệ, góc xoay và tịnh tiến theo chiều dọc tại mỗi mốc thời gian, kèm theo thời lượng, độ trễ, hàm làm mượt và số lần lặp. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse` và `KeyframesBounce` chính là các hiệu ứng của Tailwind.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` dịch chuyển một giá trị hoạt ảnh qua các điểm dừng giống như Web Animations, với các mốc offset, hàm làm mượt cho từng đoạn, độ trễ, số lần lặp và hướng chạy. Cử chỉ cũng có thể điều khiển các giá trị: `View.PanValues`, `PinchValue` và `RotationValue` dịch chuyển chúng trên luồng chính tính từ vị trí lúc cử chỉ bắt đầu.

`core.PanEvent` bao gồm `Position` trong view và `PagePosition` trong trang ở mọi pha kéo (pan), nên các công cụ kéo thả không cần thêm một sự kiện nhấn riêng biệt để lấy lại tọa độ ngón tay.

## Tiếp theo

- [Kiểu dáng và giao diện](styles.md): các thuộc tính mà chuyển tiếp dịch chuyển.
- [Danh sách và cuộn](lists.md): các giá trị bám theo khung cuộn.
- [Bố cục](layout.md): bản chất của các khung được tạo hoạt ảnh.
