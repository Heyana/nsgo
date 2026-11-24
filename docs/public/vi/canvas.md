---
title: Canvas và shader
description: Vẽ đồ họa 2D trên engine và tô phủ một view bằng fragment shader trên GPU.
group: Khái niệm cốt lõi
order: 59
---

## Một canvas

`core.Canvas` ghi lại các lệnh vẽ 2D trên engine và giao cho nền tảng thực thi chúng, tương tự như thẻ `<canvas>` của trang web. `Draw` tự động chạy lại khi có signal mà nó đọc, kích thước của canvas, hoặc các phông chữ khả dụng thay đổi, và nền tảng sẽ vẽ bản ghi đó trên luồng chính. Một phông chữ được nạp qua `cx.RegisterFont` sẽ kích hoạt `Draw` chạy lại ngay khi sẵn sàng, cập nhật cả phần văn bản lẫn hình học dựa trên `TextMetrics`.

Hãy dùng `Em` hoặc `EmFunc` cho canvas có kích thước tính theo đơn vị em của phông chữ kế thừa. Với nội dung có chiều cao phụ thuộc vào chiều rộng, `MeasureEm(width, fontSize)` trả về một `EmBox` tại chiều rộng khả dụng tính bằng point (`core.Auto` khi không giới hạn) và cỡ chữ đã phân giải. Các giá trị mà nó đọc đều được theo dõi, và hàm này cũng chạy trong pha bố cục nên tuyệt đối không ghi signal bên trong nó. `Ref.ContentBox()` theo dõi khung bố cục nằm bên trong phần đệm (padding) và viền của một view đã gắn trên cây, theo hệ tọa độ của chính nó.

```go
core.Canvas{
	Style: core.Style{Height: core.Pt(220)},
	Draw: func(d *core.Drawing, size core.Size) {
		d.SetFillColor(core.RGB(0xf1f5f9)).FillRect(0, 0, float32(size.Width), float32(size.Height))
		d.SetStrokeColor(core.RGB(0x007aff)).SetLineWidth(2).SetLineCap(core.CapRound)
		d.BeginPath().MoveTo(24, 160).LineTo(90, 80).LineTo(160, 120).LineTo(230, 40).Stroke()
	},
}
```

Bản ghi tuân theo mô hình `CanvasRenderingContext2D` trên web: `Save` và `Restore`, các phép biến đổi, đường vẽ (path) gồm đoạn thẳng, đường cong, hình chữ nhật và cung tròn, tô màu và nét vẽ với màu đơn sắc hoặc dải chuyển sắc, cắt vùng (clipping), nét đứt, đổ bóng, văn bản và hình ảnh. `core.ParsePath` tạo một `core.Path` từ dữ liệu đường vẽ SVG, còn `FillFromView` và `StrokeFromView` lấy các kiểu `Fill` và `Stroke` kế thừa của view canvas, nhờ đó biểu đồ tự động đi theo chủ đề của ứng dụng.

Trên web, các canvas nằm trong `core.Scroll` đang phóng to hoặc có `Style.Transform` / `AnimatedStyle` co giãn sẽ phát lại ở tỉ lệ tích lũy ngay khi chuyển động dừng lại, tối đa gấp 4 lần tỉ lệ điểm ảnh của màn hình. Một canvas có bitmap sau khi phóng vượt quá 4096 × 4096 pixel sẽ chỉ vẽ phần đang hiển thị và tự cập nhật khi cuộn hoặc kéo, giữ cho nét vẽ và ký tự luôn sắc nét mà không cần chạy lại `Draw`. Apple, Android, Windows và Linux cũng giữ cho canvas và văn bản khi phóng to luôn sắc nét dưới cả thao tác thu phóng của khung cuộn lẫn phép biến đổi kiểu dáng.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## Văn bản, hình ảnh và các phần

`MeasureText`, `TextMetrics` và `SetTextAlign` bố cục văn bản bằng bộ máy văn bản của chính nền tảng, nên các nhãn sử dụng cùng hệ thống phông chữ với phần còn lại của ứng dụng. `DrawImage`, `DrawImageIn` và `DrawImageFrom` vẽ ảnh bitmap; `DrawSVG` và `DrawSVGIn` vẽ các tài liệu `*core.SVG` đã phân tích (`core.ParseSVG`) theo kích thước gốc hoặc căn vừa vào một hình chữ nhật — với các phần tử `<text>` và `<tspan>` được đo và kết xuất bởi chính bộ máy văn bản của nền tảng cùng `currentColor` lấy từ màu tô của bản vẽ — và `OnPress` có thể kiểm tra bản vẽ gần nhất bằng `IsPointInPath` và `IsPointInStroke`. Các backend gốc lưu đệm 64 nguồn ảnh canvas gần nhất chưa được giữ cố định; hãy gọi `release := cx.KeepImage(src)` để ghim các bảng sprite hoặc texture dùng lâu dài khỏi giới hạn loại bỏ bộ đệm cho đến khi `release()` chạy hoặc `cx` bị giải phóng.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

Ảnh bitmap khi co giãn mặc định sử dụng lấy mẫu làm mượt. Với đồ họa pixel art, hãy gọi `d.SetImageSmoothing(false)` trước `DrawImage`, `DrawImageFrom` hoặc `DrawAtlas`, hoặc đặt `ctx.imageSmoothingEnabled = false` trong TSX. `Save` và `Restore` lưu giữ thiết lập này cùng với các trạng thái vẽ khác. Phép lấy mẫu láng giềng gần (nearest-neighbor) đã sẵn sàng trên các nền tảng Apple, Windows, Linux, Android và web; HarmonyOS và mini program cũng áp dụng nó, dù đầu ra chưa được kiểm chứng trên thiết bị thật.

`d.DrawVertices(image, vertices)` vẽ một lưới có phủ texture. Cứ mỗi ba giá trị `core.CanvasVertex` liên tiếp tạo thành một tam giác. Đặt `Position` theo tọa độ point của canvas và `TexCoord` theo tọa độ point của ảnh, tương tự như `DrawImageFrom`. `Color` của mỗi đỉnh nhân với texture và nội suy trên toàn tam giác; giá trị rỗng (zero value) giữ nguyên màu texture gốc. Để biến dạng lưới, hãy cập nhật lại tọa độ các điểm và ghi lại bản vẽ. Ảnh truyền vào có thể là bất kỳ `ImageSource` nào, bao gồm bộ đệm điểm ảnh hoặc `Source()` của một `OffscreenImage`; canvas trên Android và mini program hiện chưa hỗ trợ vẽ ảnh offscreen. `SetImageSmoothing(false)` cũng áp dụng cho lưới đỉnh. Nền tảng web yêu cầu ảnh nhúng nội tuyến hoặc cùng nguồn (same-origin) đối với lưới tùy ý. Mini program vẽ lưới thông qua trình phát của web, vốn chưa được kiểm chứng trong WeChat DevTools hoặc trên thiết bị thật. HarmonyOS cắt từng tam giác và vẽ ảnh qua phép biến đổi afin; giống như `DrawImage`, nó vẽ được ảnh mã hóa và bộ đệm điểm ảnh nhưng chưa vẽ được ảnh offscreen, đồng thời áp dụng độ mờ alpha trung bình của mỗi tam giác nhưng chưa hỗ trợ nhuộm màu (color tint). Nó chỉ che được đường ghép giữa các tam giác không trong suốt, và chưa được kiểm chứng trên thiết bị thật.

`DrawTintedImage`, `DrawTintedImageIn` và `DrawTintedImageFrom` nhân các kênh sRGB và alpha của ảnh với một màu trước khi hòa trộn. Với chế độ nguồn đè (source-over) mặc định, các điểm ảnh trong suốt vẫn để lộ nền phía sau. Trong TSX, hãy dùng `drawTintedImage(image, tint, ...args)` với cùng các dạng tham số tọa độ như `drawImage`. Trên web, một nguồn ảnh khác nguồn (cross-origin) phải cho phép yêu cầu CORS ẩn danh thì mới nhuộm màu được từng điểm ảnh. HarmonyOS hỗ trợ nhuộm màu ảnh từ byte, tài nguyên ứng dụng và bộ đệm điểm ảnh nhưng chưa kiểm chứng trên thiết bị thật; ảnh offscreen chưa vẽ được trên nền tảng này.

Dùng `d.DrawAtlas(source, rects, xforms, alphas)` để vẽ nhiều sprite từ cùng một ảnh theo thứ tự. Mỗi `core.Bounds` chọn một ô trong hệ tọa độ point của chính ảnh đó. Giá trị `core.RSXform` tương ứng ánh xạ góc trên-trái của ô tới `TX, TY`, với `SCos` và `SSin` bằng tỉ lệ nhân với cos và sin của góc xoay. Ví dụ, `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` vẽ một ô 32 point ở kích thước một nửa tại vị trí (40, 20). Truyền `nil` để giữ nguyên độ mờ đầy đủ của sprite, hoặc truyền các hệ số nhân với alpha vẽ hiện tại.

`Drawing.Node` đặt tên cho một phần của bản vẽ — như một cột của biểu đồ hay một vùng trên bản đồ — kèm khung bao, vai trò, nhãn và trạng thái chọn. Công nghệ hỗ trợ đọc các phần đó như các phần tử con của canvas, còn bài kiểm thử tìm chúng theo nhãn, văn bản hoặc test ID; thao tác nhấn vào một phần tương ứng với nhấn vào tâm của nó.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## Ảnh vẽ ngoại tuyến

`cx.NewOffscreenImage(size, scale, draw)` kết xuất một `core.Drawing` một lần vào ảnh ngoại tuyến (offscreen image) được giữ lại để các khung hình sau vẽ bằng `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` hoặc `core.Image`. Truyền `scale <= 0` sẽ sử dụng tỉ lệ điểm ảnh hiện tại của màn hình. Bản vẽ chỉ được kết xuất lại khi gọi `Invalidate()` hoặc `Resize(size, scale)` (hoặc khi đăng ký thêm phông chữ mới), và tự động được giải phóng khi `cx` bị hủy hoặc khi gọi `Release()`.

```go
tiles := cx.NewOffscreenImage(core.Size{Width: 512, Height: 512}, 0, func(d *core.Drawing) {
	// Drawn once into the offscreen image until tiles.Invalidate() is called.
	for y := range 16 {
		for x := range 16 {
			d.SetFillColor(tileColor(x, y)).FillRect(float32(x*32), float32(y*32), 32, 32)
		}
	}
})

core.Canvas{
	Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)},
	Draw: func(d *core.Drawing, _ core.Size) {
		// Reuses the cached surface in a single DrawImage operation per frame.
		d.DrawImage(tiles.Source(), -cameraX.Get(), -cameraY.Get())
	},
}
```

## Tham số hoạt ảnh

`Drawing.Bind` gắn một tham số của thao tác vẽ vừa gọi vào một nút hoạt ảnh, và nền tảng chỉ vẽ lại đúng phần đó theo đồng hồ khung hình của mình mà không cần commit mới. `core.ArgX`, `ArgY`, `ArgR`, `ArgA` và `ArgColor` định danh các tham số; macOS, iOS và Linux chỉ vẽ lại vùng bị thay đổi, trong khi Windows ghi lại bản vẽ còn Android và web vẽ lại toàn bộ canvas.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## Shader

`core.Shader` tô phủ khung của nó bằng một fragment shader mà GPU thực thi cho từng điểm ảnh. `core.CompileShader` nhận một chương trình viết bằng cú pháp Go có hàm `Fragment`, còn `core.CompileWGSL` nhận mã WGSL. `Uniforms` thiết lập các biến xuất khẩu của chương trình, và `Images` cung cấp tối đa bốn texture cho nó.

```go
var program = core.MustCompileShader(`
package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	v := 0.5 + 0.5*sin(pos.x/40)
	return Tint * vec4(v, v, v, 1)
}
`)

core.Shader{
	Program:  program,
	Uniforms: core.Uniforms{"Tint": core.RGB(0x007aff)},
	Style:    core.Style{Height: core.Pt(220), CornerRadius: 16},
}
```

Một shader viết bằng Go nhận tâm điểm ảnh ở `dst` và cùng điểm đó theo đơn vị point ở `pos`, đồng thời có sẵn các hàm dựng sẵn của GLSL; `rectSize`, `pixelScale`, `image0At` đến `image3At` cùng kích thước của chúng là các bổ sung riêng của NSGo. Một chương trình WGSL viết `fn shade(uv: vec2f) -> vec4f` và đọc `nsh.time` cùng `nsh.size`; `Paused` tạm dừng đồng hồ của nó. Nền tảng không có luồng GPU cho shader sẽ không vẽ gì tại đó.

`core.ShaderEffect` vẽ các phần tử con của nó thông qua WGSL và `core.ScreenEffect` phủ toàn bộ bề mặt. `core.ShaderTransition` và `core.ScreenTransition` lấy mẫu hình ảnh trước và sau khi `Value` thay đổi. `core.TransitionShader` đảm nhiệm việc này cho điều hướng và hộp thoại. Trên Android, việc chụp view cho các hiệu ứng đòi hỏi Android 10 trở lên và OpenGL ES 3.0 có hỗ trợ external texture. Cả `core.Canvas` và `core.Shader` đều nhận `Label`, và các phần tử con của chúng được vẽ đè lên phía trên.

## Tiếp theo

- [Hình ảnh và biểu tượng](images.md): các bức ảnh mà canvas vẽ.
- [Hoạt ảnh](animation.md): các giá trị mà canvas gắn kết.
- [Khả năng truy cập](accessibility.md): các phần mà canvas hiển thị cho trình hỗ trợ.
