---
title: Thành phần và trạng thái
description: Phần tử, thành phần, signal và kiểu dáng.
group: Khái niệm cốt lõi
order: 50
---

## Phần tử

Màn hình là một cây các phần tử, chính là các giá trị struct trong Go. `core` chứa các phần tử cơ bản: `View`, `Text`, `Image`, `Scroll`, `Input` và nhiều loại khác. `ui` chứa các điều khiển mà ứng dụng sử dụng, như `Button`, `Select`, `DatePicker` và `EmojiPicker`, với giao diện tuân theo chủ đề (theme). (`base` giữ chính các điều khiển đó nhưng chưa có giao diện, dành cho [tác giả viết thành phần](authoring/index.md).)

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` hiển thị nội dung web; một `WebSource` rỗng sẽ tải `about:blank` và báo cáo các sự kiện tải và điều hướng như bình thường. Đặt `SourceFunc` thành một nguồn rỗng hoặc gọi `Ref.Load(core.WebSource{})` để xóa trang trước đó. Đặt `InjectedJavaScriptBeforeContentFunc` khi đoạn script chạy trước các script của chính trang bám theo một signal: khi signal thay đổi, view sẽ tải lại nguồn với script mới, thay thế trang đang hiển thị — kể cả trang mà người dùng hoặc `Ref.Load` đã điều hướng tới. Đặt `UserAgentSuffix` (TSX là `userAgentSuffix`) để nối thêm một chuỗi, chẳng hạn như `Version/26.0 Safari/605.1.15`, vào sau user agent mặc định của nền tảng hoặc sau `UserAgent` khi giá trị đó được đặt; trên macOS và iOS, hậu tố nối vào user agent của nền tảng chỉ có hiệu lực khi view được tạo lần đầu, còn HarmonyOS và web vẫn giữ user agent riêng của chúng. Gắn một `core.Ref` để gọi `Reload`, `GoBack` và `GoForward`. Trên các nền tảng gốc, các lệnh này giữ cho HTML và nguồn dữ liệu của chính ứng dụng được miễn khỏi `core.AccessWebSchemes`; các trang tạo ra điều hướng mới vẫn tuân theo chính sách truy cập.

Trên tvOS, `cx.Env().HasWebView()` trả về `false`. Việc gắn một `WebView` sẽ báo một `*core.WebError` qua `OnError` mà không tải WebKit; hãy cung cấp nội dung ứng dụng phù hợp cho nền tảng này.

Trên macOS, iOS, Windows và Linux, `OnNavigationStateChange` cũng báo cáo các thay đổi về URL, lịch sử và tiêu đề trong cùng một tài liệu.

Đặt `WebView.Media` thành `core.WebMicrophone`, `core.WebCamera` hoặc cả hai để cho phép các yêu cầu `getUserMedia` của trang. Khai báo mô tả mục đích sử dụng trong `apple.info` và `RECORD_AUDIO` hoặc `CAMERA` trong `android.permissions`; hệ thống vẫn sẽ hỏi người dùng. Trên Android, một yêu cầu đang chờ sẽ bị từ chối nếu ứng dụng bỏ bớt thiết bị đã yêu cầu, trang rời đi hoặc view đóng lại trong khi hộp thoại hệ thống đang mở. Trang phải yêu cầu quyền truy cập lại sau những thay đổi như vậy.

`WebView.Muted` và `MutedFunc` (TSX là `muted`) tắt toàn bộ âm thanh của trang trên Windows và Linux, bao gồm Web Audio và các frame nhúng. macOS 12 và iOS 15 trở lên sẽ tạm dừng phát đa phương tiện khi bị tắt tiếng. Các môi trường chủ khác giữ nguyên chế độ phát bình thường. Các cập nhật được áp dụng cho trang hiện tại và duy trì qua các lần điều hướng.

Trong TSX, khi `injectedJavaScriptBeforeContentLoaded` thay đổi, nguồn sẽ được tải lại với script mới trong cùng một web view, tương tự như `InjectedJavaScriptBeforeContentFunc`; khi `injectedJavaScript` thay đổi, một web view mới được tạo ra và tải lại nguồn từ đầu, tương tự như khi thay đổi `userAgent`, `userAgentSuffix` hoặc `mobile`. Trên HarmonyOS, việc thay đổi bất kỳ script tiêm nào trong hai loại đều tạo ra web view mới và bắt đầu một lịch sử điều hướng gốc mới. Trên Windows và HarmonyOS, mỗi script tiêm chạy trong frame chính bên trong một khối (block), nên các khai báo `let`, `const` và `class` ở cấp cao nhất vẫn mang tính cục bộ bên trong nó; hãy gán vào `window` nếu muốn trang nhìn thấy biến toàn cục.

Đặt `Mobile` (TSX là `mobile`) để hiển thị trang theo giao diện điện thoại trên Windows, macOS và Linux, dành cho những trang như YouTube Shorts vốn chỉ chuyển video bằng thao tác vuốt dọc trên điện thoại: trang sẽ nhận user agent của điện thoại (trừ khi `UserAgent` đã được đặt) cùng màn hình cảm ứng không có hover, và thao tác kéo chuột hoặc cuộn bánh xe/trackpad lên phần không cuộn được sẽ truyền tới trang như một cú vuốt ngón tay. Windows mô phỏng điện thoại giống chế độ thiết bị của Chrome. macOS và Linux thực hiện bằng script, nên stylesheet của trang vẫn thấy chuột và các sự kiện cảm ứng là không đáng tin cậy (untrusted), điều mà một số trang có thể bỏ qua. iOS, Android, HarmonyOS, web và mini program hiển thị trang như nguyên bản.

Trên macOS, iOS, Windows và Linux, `OnOpenWindow` nhận các yêu cầu mở cửa sổ mới từ trang web. View sẽ không tự tạo cửa sổ bật lên (popup). Hãy dùng `URL` và `UserInitiated` của yêu cầu để quyết định điều hướng ngay trên view đó bằng `Ref.Load`, mở URL ở nơi khác hay bỏ qua. Một view bị hạn chế sẽ gửi các yêu cầu bị từ chối tới `OnError`.

Đặt `OnDownload` để lưu các tệp mà các trang trong web view tải xuống trên macOS, iOS, Linux, Windows và Android. Các tệp tải xuống được lưu vào `downloads/<id>/<file name>` bên trong thư mục dữ liệu của ứng dụng và báo cáo khi bắt đầu, khi hoàn tất kèm số byte, hoặc khi thất bại; những view không có `OnDownload` hoặc bị hạn chế sẽ hủy lượt tải xuống.

`core.Video` phát một tệp hoặc URL bằng trình phát của nền tảng. Trên macOS, iOS và tvOS, đặt `Video.AudioSource` thành một tệp hoặc URL riêng chứa âm thanh cho một đoạn video hữu hạn. Âm thanh của trình phát sẽ lấy từ nguồn đó. Hình ảnh và âm thanh bắt đầu cùng lúc, và `Ref.Seek` di chuyển cả hai trên cùng một dòng thời gian. Video quyết định thời lượng phát, và âm thanh ngắn hơn sẽ im lặng khi kết thúc.

Một đoạn clip trong suốt hiển thị phần phía sau nó dưới dạng HEVC kèm kênh alpha trong tệp `.mov` trên macOS (iOS chưa được kiểm chứng), và dưới dạng VP9 kèm kênh alpha trong WebM trên Linux với plug-in `codecalpha` của GStreamer (`gstreamer1.0-plugins-bad`, mà một gói Linux có thể khuyến nghị cài đặt trong `nsgo.yaml`). Trên macOS, iOS, Windows và Linux, một clip cục bộ không thể giữ lại kênh alpha (WebM trên các nền tảng Apple, HEVC trên Linux, và mọi định dạng trên Windows cho đến khi đầu ra alpha của Media Foundation được kiểm chứng trên máy Windows) sẽ làm `OnError` báo lỗi thay vì hiển thị video trong một khung đục: lỗi là một `*core.MediaError` có `Reason` bằng `core.MediaErrorAlphaUnsupported`, để ứng dụng có thể tự giải mã clip đó. Android, web và các URL từ xa trên mọi nền tảng sẽ phát các clip như vậy theo đúng đầu ra của bộ giải mã, vốn có thể là nền đục.

`OnPause` và `OnPlay` bám theo các điều khiển tích hợp sẵn trên Apple, Android, Windows, Linux và web. `OnPause` trả về `VideoPauseControls` khi tạm dừng từ bộ điều khiển. `Paused` được áp dụng khi mở một nguồn phát hoặc khi giá trị của nó thay đổi. Thay đổi âm lượng, tốc độ phát và bố cục vẫn giữ nguyên trạng thái tạm dừng hoặc đang phát gốc của hệ thống, kể cả khi người dùng bấm phát từ bộ điều khiển trong lúc `Paused` vẫn là `true`. Hãy gắn `PausedFunc` với trạng thái của bạn và cập nhật nó trong các callback đó nếu ứng dụng tự hiển thị nút phát riêng.

`Rate` đặt tốc độ phát và `RateFunc` có thể thay đổi tốc độ trong khi đang phát. Dùng `VideoInfo.RateChangeable` từ `OnLoad` để hiển thị bộ điều khiển tốc độ. Linux có thể đổi tốc độ cho các video cho phép tua khi đã cài đặt GstPlay và bộ lọc `scaletempo` của GStreamer. Trình phát dự phòng chỉ dùng tốc độ 1×.

Đặt `Video.Headers` để gửi các HTTP request header như `Referer`, `User-Agent` hoặc `Cookie` trên Android và cho các tệp HTTP trực tiếp như MP4 hoặc MOV trên Linux, macOS, iOS và tvOS.

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

Video sao chép map này khi được gắn vào cây và dùng lại chính các header đó nếu `SourceFunc` cung cấp một URL mới. Trên các nền tảng Apple, hãy dùng nguồn có độ dài byte xác định và hỗ trợ range request để tua được. Trình phát sẽ gửi các header đã cấu hình qua các chuyển hướng cùng nguồn gốc (same-origin redirect). Các header tương tự cũng áp dụng cho tệp HTTP `AudioSource` trực tiếp trên các nền tảng Apple. Các URL danh sách phát kết thúc bằng `.m3u` hoặc `.m3u8` dùng trình tải gốc của hệ thống.

Trên Linux, hãy cài đặt GstPlay cùng các plug-in `scaletempo` và `souphttpsrc` của GStreamer. Hãy dùng URL đích cuối cùng của media nếu bạn đặt `User-Agent` hoặc `souphttpsrc` của bạn thiếu thuộc tính `location-trusted`. Với thuộc tính đó và user agent mặc định, GStreamer giữ lại các header của ứng dụng qua các chuyển hướng cùng nguồn gốc. Header của ứng dụng áp dụng cho tệp nguồn, còn các phân đoạn media thích ứng dùng request riêng của chúng.

## Thành phần

Một thành phần là một hàm nhận vào một scope và trả về một phần tử. Hàm chạy một lần; mọi thay đổi về sau được điều khiển bởi signal.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

Một thành phần nhận props là một kiểu dữ liệu có phương thức `Render`:

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` có thể bỏ nút đóng bằng `NoClose`, thêm một node `Trailing`, và đóng toast thông qua `OnPress`. `Toasts.NoClose` ẩn nút đóng trên toàn bộ chồng toast.

`ui.Toasts.Style` và `StyleFunc` tùy chỉnh mọi bề mặt toast, bao gồm nền, viền, bo góc và phần đệm. Kiểu dáng của chồng toast được gộp đè lên theme, theo sau là `ToastContent.Style` và `StyleFunc` của từng toast. Mỗi `StyleFunc` chạy sau `Style` tương ứng của nó và cập nhật khi các signal bên trong thay đổi. Thuộc tính `Color` trong các kiểu dáng này tô màu cho tiêu đề, và tô cho phần mô tả với độ mờ giảm bớt, giúp chữ luôn dễ đọc trên nền tối hoặc nền có màu. Các pha xuất hiện, biến mất và vuốt vẫn áp dụng kiểu dáng chuyển động của riêng chúng. Trong TSX, đặt prop `style` trên `Toasts` và truyền `style` hoặc `styleFunc` vào `show` cho từng toast.

## Signal

Một signal nắm giữ một giá trị. Đọc nó bên trong một hàm mà phần tử gọi, chẳng hạn `ContentFunc`, sẽ đăng ký theo dõi phần màn hình đó, và chỉ phần đó cập nhật khi giá trị thay đổi.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

Một signal chứa cả một slice sẽ cập nhật mọi hàng đọc đến nó. `core.NewCollection` lưu các mục theo khóa, mỗi mục nằm trong một signal riêng, nhờ vậy mỗi hàng chỉ theo dõi mục của riêng mình và thay đổi một mục chỉ chạy lại đúng một hàng:

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

Các loại trạng thái khác cũng được xây dựng trên signal theo cách tương tự:

- `core.NewSelector` cho biết một khóa có đang được chọn hay không, và chỉ chạy lại các hàng mà vùng chọn rời đi và đi tới.
- `core.NewHistory` lưu trữ một giá trị kèm khả năng hoàn tác (undo) và làm lại (redo), với các signal `CanUndo` và `CanRedo` dành cho trình đơn Edit.
- `core.NewPersisted` lưu một giá trị định kiểu trong bộ nhớ ứng dụng dưới dạng JSON có phiên bản, hỗ trợ di trú dữ liệu do các phiên bản cũ ghi ra.
- `core.From` theo dõi một nguồn bên ngoài đồ thị, chẳng hạn như một kênh đăng ký gửi dữ liệu từ một goroutine khác.

## Kiểu dáng

`core.Style` tuân theo CSS: flexbox và grid, phần đệm (padding) và lề (margin), bo góc, viền, đổ bóng, dải chuyển màu (gradient), phép biến đổi (transform) và các trạng thái như `Active`. Phần tử cũng nhận các lớp Tailwind thông qua `Class`:

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

Các chủ đề (theme) quy định diện mạo cho các thành phần trong `ui` ở cả chế độ sáng và tối.

## Ứng dụng máy tính để bàn

`nsgo.Run(Root)` hiển thị một view gốc duy nhất. Thay vào đó, một ứng dụng máy tính để bàn có thể tự mở những gì nó hiển thị: `nsgo.Run(App)` chạy `func App(app *core.Scope)` một lần, và hàm này mở các cửa sổ, panel, biểu tượng khay hệ thống và phím tắt toàn cục qua `desktop`, tất cả dùng chung các signal của nó. Cơ chế này hoạt động trên macOS, Windows và Linux; trên macOS, `examples/tray` không hiện trên Dock mà chỉ nằm trên thanh menu. `nsgo.Run(App, nsgo.StartHidden())` mở cửa sổ chính mà không hiển thị nó, để một ứng dụng khay hệ thống hoặc ứng dụng được hệ thống khởi chạy lúc đăng nhập (`desktop.LaunchedAtLogin`) không bị nháy cửa sổ lên; ứng dụng hiển thị cửa sổ khi cần bằng `app.MainWindow().Show()`. Điện thoại và web hiển thị ứng dụng ngay khi khởi chạy.

`desktop.SetWindowLevel` giữ một cửa sổ nằm trên các cửa sổ khác — như một đồng hồ đếm giờ hay thú cưng màn hình — hoặc nằm dưới chúng, còn `desktop.SetWindowFrame` di chuyển và đổi kích thước cửa sổ. `desktop.CanSetWindowLevel` và `desktop.CanPlaceWindows` cho biết các lệnh này có hiệu lực trong scope của một cửa sổ hay không: cả hai đều có hiệu lực trên macOS, Windows và Linux chạy X11, và đều không có hiệu lực với cửa sổ toplevel thông thường trên Wayland, nơi compositor tự quyết định thứ tự xếp chồng và vị trí.

Một cửa sổ mở với `desktop.WindowOptions{Transparent: true}`, hoặc cửa sổ chính với `nsgo.WindowTransparent()`, sẽ không có nền hay khung viền, để phần màn hình desktop lộ ra ở những chỗ ứng dụng không vẽ gì — như một thú cưng màn hình hay widget; một view có `Desktop.WindowDrag` dùng để kéo di chuyển cửa sổ đó. `NoShadow` bỏ phần bóng đổ của hệ thống và `SkipTaskbar` giữ cửa sổ không hiện trên thanh tác vụ Windows, Alt+Tab cũng như danh sách tác vụ X11; `desktop.SetWindowTransparent`, `SetWindowShadow` và `SetSkipTaskbar` thay đổi các tùy chọn này về sau. Windows chỉ thay đổi độ trong suốt ở cửa sổ được mở ở chế độ trong suốt (loại này không có bóng đổ), còn Wayland không có gợi ý nào để ẩn cửa sổ khỏi danh sách tác vụ. `desktop.CanMakeWindowsTransparent` và `desktop.CanSkipTaskbar` cho biết trong scope của một cửa sổ xem các thiết lập này có hiệu lực hay không: độ trong suốt cần DirectComposition trên Windows và cần compositor trên Linux (X11 không có compositor sẽ không hỗ trợ), và trên macOS thì `desktop.SetDockVisible` giúp ẩn ứng dụng khỏi Dock.

## Terminal

Nhập `base/terminal` và tạo một phiên làm việc bằng `terminal.New(terminal.Options{Conn: conn})` cho luồng kết nối từ xa, hoặc đặt `Command` cho chương trình cục bộ trên macOS, Linux hoặc Windows. Kết xuất `terminal.View{Terminal: session}` và gọi `Close` khi phiên kết thúc. `Options.ImageLimit` giới hạn bộ nhớ đồ họa Kitty tính bằng byte: bằng 0 sẽ dùng 64 MiB, giá trị âm sẽ tắt hình ảnh. Các chương trình có thể gửi ảnh RGB, RGBA hoặc PNG cùng vị trí đặt bằng ký tự giữ chỗ Unicode.

Gọi `Search(text)` để tô sáng các kết quả khớp trên màn hình và phần cuộn ngược (scrollback), `SearchNext` và `SearchPrevious` để chọn chúng, và `EndSearch` để xóa phần tô sáng. `Options.OnSearch` báo cáo `SearchStatus`; `Theme.Search`, `SearchText`, `SearchSelected` và `SearchSelectedText` đặt màu cho các kết quả khớp. Các shell phát mã đánh dấu OSC 133 hỗ trợ giữ Alt/Option và nhấp chuột trong vùng lệnh đang nhập để di chuyển con trỏ. NSGo không tự cài đặt các script tích hợp shell.
