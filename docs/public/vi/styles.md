---
title: Kiểu dáng và chủ đề
description: Kiểu dáng có kiểu, lớp Tailwind, kiểu dáng theo trạng thái, cùng chủ đề sáng và tối.
group: Khái niệm cốt lõi
order: 52
---

## Một kiểu dáng

`core.Style` là một giá trị Go duy nhất cho bố cục, diện mạo và văn bản của một hộp. Giá trị không (zero value) của mỗi trường là chưa đặt, nên các kiểu dáng có thể kết hợp với nhau: `Merge` chỉ thay thế những gì kiểu dáng sau đặt, và một phần tử con kế thừa các trường văn bản mà các phần tử cha của nó đã đặt. Để đặt một độ dài về không trên một kiểu dáng đã cho nó giá trị, hãy dùng `core.Zero` trong `Padding`, `Margin`, `Borders`, `BorderWidth`, `CornerRadius` hoặc một khoảng cách (gap), giống như `core.Transparent` làm với màu: `Padding: core.All(core.Zero)`.

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

Kiểu dáng hỗ trợ màu, góc, viền, đường viền ngoài (outline), đổ bóng, dải chuyển màu, bộ lọc, làm mờ phông nền (backdrop blur), mặt nạ, chế độ hòa trộn, biến đổi 3D và chuyển tiếp của CSS. `core.RGB`, `core.RGBA` và `core.ParseColor` tạo màu, `core.LinearGradient` và các hàm cùng loại tạo dải chuyển màu, `core.Ring` tạo vòng (ring) của Tailwind, và `core.AllCorners` cho mỗi góc bán kính riêng. `BorderCurveContinuous` bo các góc liền mạch vào cạnh như iOS. Đổ bóng và đường viền ngoài của một hộp nằm ngoài vùng cắt `Overflow` của nó, trừ những trường hợp [Bố cục](layout.md#vị-trí) nêu. Bộ lọc phông nền thay đổi những gì nằm phía sau một view giống như `backdrop-filter` của CSS: một `Blur` làm mờ nó theo bán kính, còn các bộ lọc màu thay đổi màu sắc của nó. iOS vẽ chất liệu hệ thống (material) mà `View.IOS.BackdropMaterial` chỉ định cho một hiệu ứng làm mờ không có bộ lọc màu; một hiệu ứng làm mờ chỉ định chất liệu khác `core.BackdropUltraThin` sẽ vẽ chất liệu đó trên iOS và chất liệu gần nhất của AppKit trên macOS, bỏ qua các bộ lọc màu. Linux và Android không vẽ bộ lọc phông nền. `Style.GlassEffect` (`core.GlassRegular` hoặc `core.GlassClear`, `glassEffect` trong TSX) vẽ Liquid Glass thay cho làm mờ phông nền trên macOS 26 và iOS 26 (và shader kính của NSGo trên Windows; hãy cho view kính một hiệu ứng làm mờ để dùng thay thế ở các nền tảng khác). Đặt `Style.GlassInteractive` (`glassInteractive` trong TSX) bật phản hồi nhấn tương tác của hệ thống cho Liquid Glass trên macOS 27 (`NSGlassEffectView.effectIsInteractive`) và iOS 26 (`UIGlassEffect.isInteractive`) trong khi vẫn giữ `OnPress` và các phần tử con có thể nhấn hoạt động.

`Transform` di chuyển vùng nhận con trỏ cùng với view: thao tác nhấn, di chuột (hover) và cử chỉ bắt đầu tại nơi view được vẽ, và vị trí con trỏ được báo cáo sẽ hoàn tác phép biến đổi. Một phần tử con đã biến đổi nằm ngoài phần tử cha vẫn có thể tương tác được trừ khi một vùng cắt hoặc khung cuộn che nó đi. Mini program hiện chưa hoàn tác các keyframe mà lớp hiển thị của chúng chạy ([lộ trình](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## Trạng thái

Diện mạo của một view có thể thay đổi theo trạng thái của nó. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled` và các trạng thái `aria-*` là các giá trị `*Style` được gộp lên phần còn lại khi trạng thái đó còn hiệu lực. Ứng dụng đặt `Selected`, `Checked`, `Disabled` và `Expanded` thông qua hàm, còn nền tảng báo cáo thao tác nhấn và tiêu điểm.

`Active` và `group-active:` theo dõi một view khi nó đang được nhấn, điều này cần một trình xử lý nhấn hoặc một `Responder` giữ lượt chạm. Tạo kiểu cho một view thông thường bằng `Active`, `active:` hoặc `group` không khiến nó có thể nhấn được hay trở thành điểm dừng Tab của bàn phím; hãy đặt `Focusable` khi bản thân nó cần nhận tiêu điểm.

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

Một bộ phận của thành phần có thể nhận các trạng thái của điều khiển bao quanh mà không tự báo cáo chúng: `PartStates` cung cấp `Checked`, `Selected`, `Disabled` và các trạng thái còn lại, nhờ đó dấu tích của hộp kiểm cũng đi theo hộp kiểm đối với trình đọc màn hình.

## Lớp

`Class` trên `View`, `Text`, `Span`, `Input`, `Image`, `Scroll` và `Canvas` nhận các lớp Tailwind CSS 4, và mỗi bộ phận của một thành phần `base` đều có một lớp bên cạnh kiểu dáng của nó. Các tiện ích (utility), biến thể (variant) và giá trị tùy ý mang ý nghĩa giống như trong Tailwind, trên chủ đề và bảng màu mặc định của nó.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

Một lớp được gộp bên dưới kiểu dáng, nên các trường của kiểu dáng được ưu tiên. Lớp có thể theo dõi những gì thay đổi bằng `ClassFunc`, và một biến thể trạng thái (`hover:bg-zinc-100`, `checked:opacity-100`) trở thành trạng thái của kiểu dáng. `group` và `group-hover:` tạo kiểu cho các phần tử con dựa trên trạng thái của phần tử cha, `first:` và `last:` theo vị trí của một phần tử con, còn `aria-checked:`, `data-open:` và `focus-within:` theo các trạng thái mà một điều khiển thể hiện. `pointer-fine:` và `pointer-coarse:` tạo kiểu cho một cây con dành cho chuột hoặc màn hình cảm ứng, khớp với đặc tính media `pointer` của CSS thông qua `cx.Env().Pointer()` (truy vấn `pointer` của trình duyệt trên web, fine trên máy tính để bàn và Mac, coarse trên điện thoại, máy tính bảng và đồng hồ, và không phải cả hai trên TV), và kết hợp với các biến thể khác như `dark:` và `hover:` để ứng dụng chọn kích thước cho cảm ứng hay con trỏ một cách tường minh thay vì `core` tự thay đổi ngầm.

Đánh dấu một điều khiển bằng `peer` để tạo kiểu cho các phần tử anh em đứng sau dựa trên trạng thái của nó, ví dụ một hộp kiểm với `Class: "peer"` theo sau là một nhãn với `Class: "peer-checked:text-accent peer-disabled:opacity-50"`. Một peer chỉ tạo kiểu cho các phần tử anh em đứng sau nó dưới cùng một phần tử cha, kể cả `Show` và `Each`, không bao giờ cho các phần tử anh em đứng trước hay các view bên trong nó. Dùng `peer/field` với `peer-checked/field:` để đặt tên cho một nhóm, và `peer-data-invalid:` cho trạng thái dữ liệu boolean `invalid` của một điều khiển. Các dấu đánh dấu và trạng thái cập nhật theo cơ chế phản ứng (reactive). Đánh dấu một view không khiến nó trở thành điểm dừng Tab hay nhận thao tác nhấn, nên `peer-active:` chỉ theo dõi một peer tự nhận thao tác nhấn, chẳng hạn một nút bấm.

`has-[>svg]:px-3` thay đổi phần đệm của phần tử cha khi nó có một biểu tượng là con trực tiếp; `[&_svg]:size-4 [&_svg]:shrink-0` định kích thước các biểu tượng con cháu của nó. `svg` chọn `core.Icon` và `SVGView`, kể cả thông qua các vùng có điều kiện. Các bộ chọn này cập nhật khi phần tử con, lớp hoặc trạng thái thay đổi. `hover:[&_svg]` theo trạng thái hover của phần tử cha; `[&_svg]:hover` theo trạng thái hover của từng biểu tượng. `active` của một bộ chọn chỉ theo dõi các view nhận thao tác nhấn, chẳng hạn một nút bấm, giống như `peer-active:`. Các bộ chọn khớp cạnh tranh theo độ đặc hiệu (specificity) trước thứ tự biến thể; kiểu dáng nội tuyến được ưu tiên trừ khi lớp được đánh dấu important.

Bộ chọn khớp với các phần tử NSGo trên mọi nền tảng đích. Chúng hỗ trợ danh sách, các tổ hợp con (`>`), con cháu (`_`), anh em liền kề (`+`) và anh em theo sau (`~`), lớp, sự có mặt/bằng nhau của `data-*`, vai trò, các trạng thái trợ năng boolean được hỗ trợ và `:is`, `:where`, `:not`, `:has` cùng vị trí phần tử con. Các kiểu là `view`/`div`, `text`/`span` (toàn bộ nút văn bản), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview` và `native`. Dùng một `&` cấp cao nhất trong mỗi phương án của bộ chọn tùy ý; nếu không có nó, bộ chọn sẽ kiểm tra chính phần tử đó. Các bộ chọn chỉ có trên trình duyệt, phần tử giả (pseudo-element), at-rule, ký tự thoát và `&` tường minh bên trong một hàm đều không được nhận dạng; các bộ chọn này không chạm tới các view hiện thực của backend hay từng đoạn văn bản riêng lẻ.

Các biến thể `peer-*` trong `Span.Class` hoặc `Screen.HeaderClass` không có phần tử anh em nào để theo dõi. Chúng bị bỏ qua và được ghi log một lần cho mỗi lớp và ngữ cảnh; hãy đặt biến thể peer của một span lên `Text` chứa nó.

Trường `Tailwind` của chủ đề mở rộng chủ đề mặc định với màu, điểm ngắt (breakpoint) và tiện ích, và một màu mà bảng màu không có sẽ chỉ định một vai trò `Palette` (`bg-accent`, `text-text-muted/70`) hoặc một màu của chủ đề, được đọc từ chủ đề của nút theo giao diện mà nó hiển thị.

## Chủ đề

Một `core.Theme` là một diện mạo cho cả hai giao diện: một `Palette` cho sáng và một cho tối, một bán kính góc và các họ phông chữ. Các vai trò của bảng màu là `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus` và `Scrollbar`; `Colors` chứa các vai trò mà một thư viện bổ sung, chẳng hạn `inset` và `raised`. `core.DefaultTheme()`, chủ đề mà ứng dụng hiển thị cho đến khi đặt chủ đề khác, là chủ đề trung tính với màu nhấn xanh dương và các vai trò của `ui`, còn `ui.WarmDuskTheme()` là một bảng màu ấm.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` cấp một chủ đề cho phạm vi (scope) và mọi thứ được dựng bên dưới nó, nhờ đó một thanh bên hoặc một bảng điều khiển có thể có chủ đề riêng; một chủ đề nil sẽ lấy chủ đề ở cấp trên. `core.ParseTheme` đọc một tệp chủ đề YAML hoặc JSON, hoặc các biến CSS của một tệp chủ đề shadcn/ui, với màu theo vai trò, một bán kính và các họ phông chữ. `core.DefaultTheme`, chủ đề mà ứng dụng hiển thị cho đến khi đặt một chủ đề, và `ui.WarmDuskTheme` trả về các chủ đề sẵn sàng để chỉnh sửa.

`cx.SetAppearance` buộc toàn bộ ứng dụng ở chế độ sáng hoặc tối, `cx.SetSubtreeAppearance` làm điều đó cho một cây con, và `core.AppearanceSystem` quay lại theo hệ thống. `cx.Dark()` cho biết một cây con hiển thị giao diện nào, và `cx.Palette()` cung cấp các màu của nó với các giá trị ghi đè của ứng dụng chủ nhúng đã được áp dụng.

Khi không có phần tử tổ tiên nào đặt `Style.Color`, văn bản và các biểu tượng `currentColor` dùng `Palette.Text` của cây con, cập nhật tại chỗ theo các thay đổi của chủ đề, giao diện và token của ứng dụng chủ. Một màu tường minh, kể cả `core.Transparent`, vẫn tiếp tục được kế thừa.

## Màu trong thành phần

Một thành phần đọc màu của nó tại nơi nó được dựng và trong các `StyleFunc` của nó, nhờ đó thay đổi chủ đề hoặc giao diện được áp dụng tại chỗ mà không cần dựng lại thành phần. `ui.ColorsOf(cx)` đặt tên cho các vai trò mà `ui` dùng, `ui.TintsOf(cx)` cung cấp các lớp phủ màu trong mờ của chúng, và `ui.FinOf(cx)` cung cấp các lớp hoàn thiện nhiều tầng của `ui`.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` chạy lại khi những gì nó đọc thay đổi, nên `cx.Dark()` và `cx.Palette()` bên trong nó sẽ theo chủ đề. Điều tương tự áp dụng cho `ClassFunc`, `StyleFunc` của một span và các trường `StyleFunc` của một thành phần.

## Con trỏ

`Style.Cursor` đặt một con trỏ chuột có tên (`core.CursorText`, `core.CursorColResize`, v.v.); hãy để các phần tử có thể nhấn ở `core.CursorAuto` để chúng giữ mũi tên của nền tảng. Đối với công cụ vẽ, trò chơi hoặc con trỏ canvas tùy chỉnh trên máy tính để bàn và web, `Style.CursorImage` nhận một `*core.CustomCursor` với một `Image` 1x, một `Image2x` HiDPI tùy chọn (được chọn tự động khi `PixelRatio >= 1.5`), một `Hotspot` tính bằng điểm logic được giới hạn trong phạm vi ảnh, và một con trỏ có tên `Fallback`. Kiểm tra `cx.Env().SupportsCustomCursor()` để biết nền tảng đích hiện tại có hiển thị ảnh con trỏ tùy chỉnh hay không; các nền tảng cảm ứng sẽ bỏ qua chúng.

## Tiếp theo

- [Bố cục](layout.md): vị trí đặt các hộp.
- [Văn bản](text.md): phông chữ, span và trang trí.
- [Hoạt ảnh](animation.md): chuyển tiếp giữa các kiểu dáng.
