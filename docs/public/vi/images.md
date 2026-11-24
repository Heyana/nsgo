---
title: Hình ảnh và biểu tượng
description: Hiển thị hình ảnh từ URL, tài nguyên của ứng dụng, dữ liệu byte hoặc điểm ảnh do ứng dụng vẽ, và vẽ biểu tượng Lucide cùng SVG.
group: Khái niệm cốt lõi
order: 54
---

## Một hình ảnh

`core.Image` hiển thị một ảnh bitmap hoặc một SVG: một URL, một tệp, một đường dẫn `asset:` trong tài nguyên của ứng dụng, hoặc dữ liệu byte mà ứng dụng đang giữ. Nền tảng tải, giải mã và lưu đệm ảnh ngoài luồng chính, còn luồng chính vẽ kết quả.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` lấp đầy khung và cắt phần tràn ra; `core.Contain` thu toàn bộ ảnh vào khung, `core.Center` giữ kích thước tự nhiên của ảnh, còn `core.ScaleDown` giữ kích thước đó trừ khi ảnh lớn hơn khung. `BlurRadius` làm mờ ảnh theo đơn vị điểm, `TintColor` tô lại màu các điểm ảnh không trong suốt, và `ObjectPosition` đặt vị trí ảnh trong khung.

Ảnh bitmap khi co giãn mặc định dùng lấy mẫu mượt. Đặt `Sampling: core.ImageSamplingNearest` trong Go hoặc `sampling="nearest"` trong TSX để giữ cạnh sắc nét của pixel art và bộ đệm điểm ảnh. Các nền tảng Apple, Windows, Linux, Android và web hỗ trợ thiết lập này; HarmonyOS và mini program cũng áp dụng nó, dù kết quả trên thiết bị của chúng chưa được kiểm tra. Để có cạnh điểm ảnh chính xác trong trình duyệt dùng CSS `pixelated`, hãy co giãn theo một số nguyên điểm ảnh thiết bị cho mỗi điểm ảnh nguồn.

`Source` khai báo kích thước mà bố cục sử dụng: một chiều sẽ suy ra chiều còn lại, và ảnh không có kích thước sẽ là 0 x 0 cho đến khi style của nó đặt kích thước. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` hiển thị dữ liệu byte được nhúng bằng `go:embed`, và `Size` cung cấp kích thước nội tại cho một SVG.

Với [`desktop.WindowPointerPainted`](services.md), `HitTestAlpha: 0.5` cho phép con trỏ xuyên qua các điểm ảnh có độ mờ đục dưới một nửa trên macOS và Windows. TSX đặt cùng ngưỡng này bằng `hitTestAlpha`.

## Khung hình do ứng dụng vẽ

`Pixels` hiển thị một `*image.RGBA` do ứng dụng vẽ, như khung hình của trình phát video hay trình giả lập, mà không cần mã hóa, tải hay giải mã gì. Ghi mỗi khung hình vào cùng một bộ đệm và tăng `Version`: view sẽ hiển thị nó mà không có sự kiện tải hay hiệu ứng mờ dần, giữ nguyên kênh alpha, và trên macOS, iOS, tvOS, watchOS, Android, Windows và Linux sẽ hoán đổi tại chỗ mà không có khung hình trống ở giữa. Không có `Size`, nó đo các điểm ảnh của mình theo đơn vị điểm.

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // writes the next frame into frame.Pix
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo sao chép các điểm ảnh khi lần commit mang `Version` mới chạy, nên ứng dụng có thể ghi khung hình tiếp theo từ callback `RequestAnimationFrame` kế tiếp; một bộ giải mã chạy trên goroutine riêng sẽ ghi vào bộ đệm thứ hai và gửi yêu cầu hoán đổi. Cùng một `Version` sẽ không được vẽ lại. `cx.ImageSize` trả lời ngay cho một nguồn điểm ảnh bằng kích thước bộ đệm của nó. Một `core.Canvas` vẽ điểm ảnh bằng `DrawImage`, được sao chép theo cùng cách, trên mọi nền tảng; mini program yêu cầu canvas 2D của WeChat. `Images` của một `core.Shader` nhận điểm ảnh theo cùng cách, nhờ đó shader có thể lọc một khung hình do ứng dụng ghi, chẳng hạn màn hình của trình giả lập, trên mọi nền tảng vẽ được shader.

Các nền tảng Apple, Android, Windows, Linux và web cập nhật ảnh điểm ảnh trực tiếp, còn mini program WeChat dùng canvas 2D. HarmonyOS tạo một `PixelMap` cho mỗi phiên bản, đã được kiểm tra trên thiết bị, và các bản sao của nó đi qua một cầu nối chuỗi, nên các bộ đệm lớn bị ghi lại mỗi khung hình sẽ chạy dưới 60 fps khá nhiều trên nền tảng này ([bộ đệm điểm ảnh](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)).

## Chụp một view

Gọi `cx.Capture(&ref, done)` với `core.Ref` của một view đã được gắn, hoặc `cx.Capture(nil, done)` cho bề mặt hiện tại. Callback nhận dữ liệu byte PNG theo tỉ lệ điểm ảnh của bề mặt hoặc một lỗi trên goroutine của engine. Dùng `core.ImageSource{Data: png}` để hiển thị kết quả. Để chia sẻ, hãy ghi dữ liệu byte vào một tệp `.png` trong thư mục cache của ứng dụng và truyền đường dẫn của nó trong `core.ShareOptions.Files`. [Chia sẻ tệp](services.md#mở-liên-kết-và-tệp)

Việc chụp lấy các điểm ảnh hiển thị của bề mặt trong phạm vi khung của view. Tính năng này khả dụng trên macOS, iOS, tvOS, Windows, Linux, Android và web. Android chụp cửa sổ của activity. Chụp trên web vẽ DOM của ứng dụng vào một canvas, bao gồm cả đầu ra shader của NSGo, và bỏ qua các frame nhúng, các ảnh không đọc được và một số hiệu ứng CSS.

## Tài nguyên và tỉ lệ

Các tệp trong `resources/` được phát hành cùng ứng dụng. `core.Src("asset:images/cover.png")` tải một tệp, và `Scales: core.ScalesOf(1, 2, 3)` chọn `cover.png`, `cover@2x.png` hoặc `cover@3x.png` theo tỉ lệ điểm ảnh của màn hình.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` hiển thị một nguồn trong khi ảnh đang tải, và `FadeDuration` làm ảnh đã tải hiện dần lên trên nó; 0 để nền tảng tự chọn, còn một nano giây sẽ hiển thị ngay lập tức. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError` và `OnLoadEnd` theo dõi quá trình tải. `cx.ImageSize` đọc kích thước theo điểm ảnh của một nguồn mà không hiển thị nó, `cx.PrefetchImage` tải trước một ảnh vào cache, `cx.SetImageCacheLimit` đặt ngân sách byte cho ảnh đã mã hóa trong bộ nhớ (mặc định `core.DefaultImageCacheLimit`, 32 MiB; giá trị âm bị kẹp về 0), và `cx.KeepImage` ghim một atlas hoặc texture để không bị loại khỏi cache cho đến khi `release` được trả về chạy hoặc scope của nó bị hủy. Giới hạn áp dụng cho từng cache, không tính các ảnh đã ghim, các lượt tải đang diễn ra và lượt vẽ hiện tại; nó không giới hạn điểm ảnh đã giải mã, bộ nhớ tiến trình hay cache của trình duyệt. Giá trị 0 tắt việc giữ lại các ảnh không được ghim. Các backend gốc giữ ảnh từ xa trong một cache đĩa riêng.

## Ảnh động và ảnh vector

Ảnh động GIF và WebP phát trong view, mỗi khung hình theo độ trễ riêng của nó, và lặp lại theo quy định của tệp. Tài liệu SVG được tải như ảnh và luôn sắc nét ở mọi kích thước; một SVG một màu có thể được tô màu bằng `TintColor`.

## Biểu tượng

`core.Icon` vẽ một SVG bằng màu chữ mà nó kế thừa, cao bằng cỡ chữ trừ khi style của nó đặt kích thước. Khung của nó giữ tỉ lệ khung hình của SVG, nên một khung vuông sẽ không kéo giãn nó.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` có một hàm truy cập cho mỗi biểu tượng Lucide, đặt tên theo PascalCase (`lucide.ArrowUpRight`). Với công nghệ hỗ trợ, một biểu tượng chỉ là phần trang trí trừ khi `Label` đặt tên cho nó. `ui.Icon` cũng vậy nhưng với các giá trị mặc định của `ui`, còn `core.SVG` và `core.SVGView` vẽ các đường do chính ứng dụng dựng.

## Ảnh có kiểu dáng

`ui.Image` là một hình ảnh trong một khung bo góc, có placeholder khi đang tải và một biểu tượng hình ảnh mờ khi tải thất bại. Nó giữ tỉ lệ khung hình của nguồn, hoặc 3:2 nếu không có, trừ khi style của nó đặt kích thước cả hai chiều.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## Tiếp theo

- [Văn bản](text.md): hình ảnh bên trong một dòng, dưới dạng span.
- [Kiểu dáng và chủ đề](styles.md): góc bo, bóng đổ và màu tô.
- [Danh sách và cuộn](lists.md): các hàng hình ảnh được tái sử dụng.
