---
title: Dịch vụ nền tảng
description: Hộp cảnh báo, bảng nhớ tạm, tệp, ảnh, chia sẻ, lưu trữ, bí mật, nhận dạng giọng nói, đọc văn bản, mạng, đăng nhập, mua hàng trong ứng dụng và vòng đời của ứng dụng.
group: Tính năng nền tảng
order: 44
---

## Hộp cảnh báo và bảng nhớ tạm

`cx.ShowAlert` hiển thị hộp cảnh báo hoặc bảng hành động (action sheet) của nền tảng. Các `Actions` của nó được trả về theo chỉ số, và một vai trò sẽ vẽ nút hủy và nút phá hủy theo cách của nền tảng. Trên macOS, hộp cảnh báo và bộ chọn tệp dùng một sheet khi cửa sổ của chúng đang hiển thị. Một cửa sổ bị ẩn, thu nhỏ hoặc nằm ngoài màn hình, hay một ứng dụng khay hệ thống không có cửa sổ, sẽ mở hộp thoại riêng biệt. Đóng cửa sổ đã yêu cầu sẽ hủy các hộp thoại của nó.

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` ghi văn bản và `SetClipboardImage` ghi một hình ảnh. `ReadClipboard`, `ReadClipboardImage` và `ReadClipboardFiles` trả lời bất đồng bộ, còn `HasClipboardText` và `HasClipboardImage` đọc xem bảng nhớ tạm đang chứa gì mà không hiện thông báo dán. `core.FileFromURL` mở các URL mà `ReadClipboardFiles` và `View.OnFileDrop` cung cấp thành một `core.File`: URL `file:`, và trên web là URL `blob:`.

tvOS không có bảng nhớ tạm công khai. Việc ghi vào bảng nhớ tạm bị bỏ qua, việc đọc không trả về nội dung và việc kiểm tra sự hiện diện trả về false.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

Trên macOS và Windows, `ReadClipboardItems` cho một ảnh chụp nhanh của mọi biểu diễn dạng byte có sẵn, tối đa 48 MiB. Mỗi mục mang các giá trị `ClipboardRepresentation` có thứ tự, với `Type` gốc và `Data` thô của nó, kể cả những định dạng chỉ báo hiện diện có dữ liệu rỗng. `ClipboardFormats` liệt kê tên các định dạng gốc mà không sao chép nội dung của chúng. Để khôi phục một ảnh chụp nhanh, hãy truyền nó cho `WriteClipboardItems` cùng một hàm gọi lại tùy chọn nhận lỗi ghi. Một slice rỗng sẽ xóa sạch bảng nhớ tạm. So sánh bằng nhau các giá trị từ `ClipboardChangeCount` để phát hiện một lần sao chép mới, kể cả khi văn bản được sao chép giống hệt.

macOS giữ ranh giới giữa các mục và tên kiểu của pasteboard. Windows dùng một mục duy nhất và giữ nguyên các tên Win32 dựa trên byte như `CF_HDROP` cùng các định dạng đã đăng ký như `Rich Text Format`. Khi ghi, nó gộp các mục có định dạng khác nhau và dùng một byte 0 cho nội dung rỗng. Các handle GDI, định dạng owner-display và ID định dạng số riêng tư bị bỏ qua. Trên các nền tảng khác, hàm gọi lại đọc và ghi nhận `CodeUnavailable` còn hàm gọi lại đếm nhận `-1`.

## Mở liên kết và tệp

`cx.OpenURL` mở một URL trong trình duyệt hoặc ứng dụng của hệ thống, và `core.CheckURL` cho biết những gì nó từ chối: không có scheme, dài hơn 16 KiB, có khoảng trắng, địa chỉ web không có host, và `javascript:`, `data:`, `blob:`, `file:`, `about:` cùng `asset:`.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` mở một tệp mà ứng dụng có trong ứng dụng gắn với kiểu của tệp đó, và `cx.Share` hiển thị bảng chia sẻ của hệ thống cho tệp, văn bản hoặc cả hai trên macOS, iOS, Android, Windows, Linux (thông qua desktop portal hoặc trình quản lý tệp) và web (`navigator.share`). Một `core.SharedFile` là một đường dẫn trên đĩa — hoặc, với `Share` trên web và ở bất cứ đâu tệp được giữ trong bộ nhớ, là các byte `Data` của nó — cùng tên và kiểu mà ứng dụng kia nhìn thấy. Trên máy tính để bàn, `desktop.RevealFile` hiển thị một tệp hoặc thư mục được chọn trong Finder, Explorer hoặc trình quản lý tệp, và `desktop.TrashFile` chuyển nó vào Thùng rác hoặc Recycle Bin; nó không bao giờ xóa vĩnh viễn, mà trả lời `core.ErrNoTrash` và để nguyên mục đó khi ổ đĩa không có thùng rác hoặc thùng rác không giữ được nó, chẳng hạn một tệp lớn hơn Recycle Bin.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` nhận văn bản và tệp cho [`android.share_types`](building.md#incoming-shares-on-android) hoặc [`apple.share_types`](building.md#incoming-shares-on-ios). Nội dung chia sẻ trên Android có thể khởi động ứng dụng; iOS xếp chúng vào hàng đợi trong một App Group cho đến khi ứng dụng hoạt động hoặc người dùng mở nó lần tiếp theo. Các nội dung chia sẻ đang chờ sẽ đợi trình xử lý đầu tiên. iOS giữ lô hộp thư đến cho đến khi hàm gọi lại trả về; việc chuyển giao diễn ra ít nhất một lần qua mỗi lần tiến trình thoát. Văn bản đến trong `Text`, còn tệp đến dưới dạng bản sao bộ nhớ đệm có thể đọc với `Path`, `Name` và `MIME`. Hãy di chuyển hoặc sao chép tệp sang bộ lưu trữ lâu dài nếu nó cần tồn tại lâu hơn bộ nhớ đệm.

`desktop.LookUp` hiển thị bảng từ điển của hệ thống cho một đoạn văn bản, như thao tác chạm ba ngón trên macOS, được vẽ phía trên một hộp trong một view: văn bản và hộp của một vùng chọn cùng `Ref` của view `Selectable` chứa nó. `desktop.CanLookUp` cho biết nó có làm được không, trong một cửa sổ trên macOS; Windows, Linux và web không có bảng như vậy. Trong kiểm thử, `nsgotest.App.LookUps` liệt kê các bảng đã hiển thị khi `nsgotest.Environment` đặt `CanLookUp`.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` yêu cầu xem trước tệp, bắt đầu tại một chỉ số, và cập nhật các tệp khi được gọi lại trong lúc bảng đang mở. `desktop.CanQuickLook` báo cáo khả năng sử dụng trong cửa sổ riêng của ứng dụng trên macOS và Windows, và trên Linux khi phiên làm việc cung cấp dịch vụ D-Bus đang chạy hoặc có thể kích hoạt của GNOME Sushi. Windows dùng trình xử lý xem trước đã đăng ký cho kiểu tệp; các định dạng được hỗ trợ phụ thuộc vào các trình xử lý hoặc trình xem trước đã cài đặt. Kết quả true nghĩa là nền tảng đã được yêu cầu, không có nghĩa là nó đã hiển thị tệp.

Phím Space hoặc Escape đóng bảng. Các phím mũi tên chọn tệp trong yêu cầu trên Windows và trong các phiên bản Sushi báo cáo vùng chọn và quyền sở hữu trên X11. Trên Wayland, Linux hiện mở một bản xem trước không có cửa sổ cha; tính năng tự đóng theo chủ sở hữu và chọn bằng phím mũi tên không khả dụng. Các trình xem trước dành riêng cho KDE và web không được hỗ trợ.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Biểu tượng menu gốc

`desktop.MenuItem.Icon` hiển thị một biểu tượng bên cạnh một mục trong menu ứng dụng, Dock, khay hệ thống và menu bật lên trên macOS, Windows và Linux. Một `desktop.MenuIcon` nhận một biểu tượng vector đã phân tích (`SVG`, chẳng hạn `lucide.Folder()`), các byte hình ảnh đã mã hóa (`Data`, PNG, JPEG, GIF, WebP hoặc SVG) hoặc một `core.ImageSource` cục bộ/asset/pixel (`Source`). Đặt `Template` sẽ tô màu một hình ảnh đơn sắc bằng màu chữ của menu, theo giao diện sáng và tối (và trạng thái tô sáng trên macOS); các biểu tượng `SVG` đơn sắc mặc định được coi là template.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

Biểu tượng được tải đồng bộ từ nguồn cục bộ; URL từ xa và hình ảnh không hợp lệ sẽ để mục đó không có biểu tượng. Nhãn menu, phím tắt và chỉ báo check/radio giữ nguyên hành vi gốc của chúng.

Trong kiểm thử, `nsgotest.MenuItem.Icon` ghi lại biểu tượng của mỗi mục trong `app.Menu`, `app.DockMenu` và `app.PopupMenu`.

## Ảnh và tệp

`cx.PickImages` hiển thị bộ chọn ảnh của hệ thống và trả lời bằng các `core.PickedFile`, mà ứng dụng đọc được byte, tên, kiểu và kích thước pixel của chúng. `cx.TakePhoto` hiển thị máy ảnh ở nơi nền tảng có; `cx.Env().CanTakePhoto` cho biết nền tảng có hay không. Trên macOS, Windows và Linux, nó hiển thị bản xem trước camera của trình quét cùng nút Take Photo. `cx.ScanCode` mở bản xem trước camera và trả lời bằng mã đầu tiên nó đọc được, là mã QR trừ khi `ScanOptions.Kinds` yêu cầu loại khác, hoặc nil khi người dùng đóng nó; `cx.Env().CanScanCode` cho biết nền tảng có trình quét hay không, trên các thiết bị macOS, iOS và Android có camera, máy Windows có camera, máy tính Linux có Camera portal và GStreamer `pipewiresrc`, và các trình duyệt có `getUserMedia` và `BarcodeDetector`. macOS trình bày một sheet khi cửa sổ yêu cầu đang hiển thị, Windows và Linux là một hộp modal phía trên cửa sổ, còn web là một lớp phủ trên trang. Đặt `NSCameraUsageDescription` trong `apple.info` cho macOS và iOS, và `CAMERA` trong `android.permissions` cho Android. Android, Windows và Linux đọc được mọi loại trừ PDF417. Hãy coi văn bản của mã là đầu vào không đáng tin cậy.

Trên tvOS, `PickImages` trả lời không có hình ảnh nào và `TakePhoto` trả về `core.ErrNoCamera`; NSGo không cung cấp bộ chọn Continuity Camera.

Trong kiểm thử, `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto` và `nsgotest.App.ScanCode` nhận các bản ghi `nsgotest.ImagePick` cho việc chọn và chụp, các bản ghi `nsgotest.CodeScan` cho việc quét, và trả lời thay cho giao diện hệ thống. Các hook này hoạt động ở chế độ headless và trên các host `nsgo test -target` được hỗ trợ. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` cung cấp camera cho một target không có.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` chọn tệp thuộc bất kỳ kiểu nào, `cx.PickDirectories` chọn thư mục dưới dạng một `fs.FS`, và `cx.SaveFile` lưu một tệp hoặc các byte qua hộp thoại lưu của hệ thống. Cả hai bộ chọn đều nhận một `Title`, một nhãn `Button` và một thư mục `Start`, và `PickFilesOptions.TypesName` đặt tên cho menu kiểu, để ứng dụng có thể hiển thị chúng bằng ngôn ngữ của mình; đây là các gợi ý, mà macOS, Windows và Linux tuân theo, iOS chỉ tuân theo với `Start`, còn Android và web hoàn toàn không. `cx.PickScreenColor` chọn một màu ở bất kỳ đâu trên màn hình khi `cx.Env().CanPickScreenColor()` là true: macOS, Windows, Linux có desktop Screenshot portal và các trình duyệt có `EyeDropper`.

Đặt `core.SaveOptions.Start` thành một thư mục tuyệt đối đã tồn tại để mở hộp thoại lưu tại đó trên macOS, Windows, Linux và iOS. Với gợi ý rỗng hoặc không dùng được, hoặc trên các target khác, hệ thống sẽ chọn thư mục.

Đặt `core.SaveOptions.Accept` để kiểm tra đích đã chọn trước khi ghi. Nó chạy trên goroutine của engine. Trả về một lỗi sẽ để nguyên đích đó. Trên macOS, Windows và trong bộ chọn GTK chạy trong tiến trình trên Linux, hộp thoại hiển thị lý do và cho phép người dùng chọn đường dẫn khác. Hủy sau một lần từ chối sẽ báo `saved == false`, đường dẫn bị từ chối và lỗi. Trên Android và trong desktop portal của Linux, một lần từ chối sẽ hoàn tất việc lưu với kết quả đó. Android cho phép người dùng chọn một thư mục và cung cấp một tree URI với tên tệp được yêu cầu làm tham số truy vấn `name`. Nó tạo tài liệu sau khi được chấp thuận và báo URI của tài liệu mới khi thành công. Nền tảng sẽ bỏ việc ghi nếu bước kiểm tra không trả lời trong vòng năm giây. iOS và HarmonyOS báo lỗi cho các lần lưu cần kiểm tra. Web không có đường dẫn đích và dùng luồng lưu thông thường của nó.

Trong kiểm thử headless, `App.PickFiles`, `App.PickDirectories` và `App.SaveFile` nhận các bản ghi `nsgotest.FilePick`, `nsgotest.DirectoryPick` và `nsgotest.FileSave`. Các bản ghi tương tự có trong `app.FilePicks`, `app.DirectoryPicks` và `app.Saves`.

### Tài nguyên đóng gói

Đặt tệp dưới thư mục `resources/` của ứng dụng và dùng `cx.ReadAsset("asset:maps/level.bin", done)` để đọc chúng từ Go. Hàm gọi lại nhận `([]byte, error)` trên goroutine của engine sau khi đọc xong và bị bỏ qua nếu chủ sở hữu của nó biến mất hoặc chạy lại. Dùng `errors.Is(err, core.ErrAssetNotFound)` để nhận biết tệp bị thiếu. Việc đọc tải toàn bộ tệp vào bộ nhớ.

## Kéo âm thanh PCM

Dùng `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` để phát PCM Float32 xen kẽ mà ứng dụng của bạn giải mã hoặc tổng hợp. `Start` bắt đầu kéo mẫu, `Pause` dừng việc kéo và scope sở hữu sẽ đóng đầu ra khi nó kết thúc. macOS, iOS, Android, Windows, Linux và web hỗ trợ luồng này. Các target khác trả về `audio.ErrUnsupported`.

`Render` trả về số khung mà nó đã điền. Nó chạy trên luồng âm thanh và phải tránh chặn, cấp phát bộ nhớ và truy cập đồ thị. Một lần điền thiếu sẽ phát im lặng cho phần còn lại và tăng `Underruns`. `Position` trả về số khung đơn điệu và thời gian của host tại khung đó. Dùng `Flush` để bỏ các khung đang chờ và `Latency` để đọc độ trễ bộ đệm. Thay đổi tuyến âm thanh và gián đoạn sẽ tạm dừng đầu ra và gọi `OnInterrupt` trên engine. Ứng dụng có thể tiếp tục nó bằng `Start`.

## Lưu trữ

`cx.Storage()` là bộ lưu trữ khóa-giá trị của ứng dụng, được giữ trên thiết bị và dùng chung bởi mọi đồ thị của ứng dụng. Việc đọc là đồng bộ, và đọc bên trong một phép tính sẽ đăng ký phép tính đó theo dõi thay đổi; `SetItem` trả về một `core.StorageFullError` khi vượt hạn mức (`core.StorageQuota`, mặc định 5 MiB). Đây là một tệp thông thường mà bất kỳ chương trình nào của người dùng cũng đọc được, vì vậy hãy giữ token và mật khẩu trong [bí mật](#secrets) thay vào đó.

`cx.Store()` trả về kho lưu trữ bên dưới, dành cho một host tạo một root khác bằng `core.RootOptions.Storage`, chẳng hạn host chạy một plugin trên goroutine riêng: khi đó cả hai root dùng chung các mục của ứng dụng và những gì được giữ trên thiết bị. Đó là toàn bộ kho, không phải phân vùng mà `cx.ProvideStorage` cấp cho mã không do ứng dụng viết.

Một host tự cung cấp cơ chế lưu trữ bền vững sẽ gọi `core.NewStore(backend)` với một `core.StorageBackend`: `Load() (map[string]string, error)` tải một lần, và `Write(core.StorageChange)` nhận các thay đổi theo thứ tự. Rẽ nhánh theo `change.Op` (`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`) và dùng `change.Key` cùng `change.Value`. Truyền `nil` cho bộ lưu trữ chỉ giữ trong bộ nhớ.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` liệt kê các khóa, `RemoveItem` xóa một khóa và `Clear` làm trống bộ lưu trữ. `cx.DataDir()` và `cx.CacheDir()` cho các thư mục của ứng dụng dành cho những tệp mà bộ lưu trữ khóa-giá trị quá nhỏ để chứa; `cx.LogsDir()` cho thư mục của nền tảng dành cho tệp nhật ký, và một `Scope.ProvideStorage` cấp cho một cây con một phân vùng riêng.

Một iOS App Clip và ứng dụng đầy đủ của nó dùng chung các vị trí dữ liệu và lưu trữ này khi `app_clip` và `apple.app_group` tường minh được cấu hình; bộ nhớ đệm của chúng vẫn tách biệt. Bật nhóm không di chuyển dữ liệu sandbox hiện có. [Lưu trữ App Clip](building.md#app-clips) trình bày cấu hình và ký.

Trên HarmonyOS, các thao tác tệp của Go như `os.ReadFile`, `os.WriteFile` và `os.Rename` hoạt động trong các thư mục này và `os.TempDir()`. Đường dẫn tương đối bắt đầu từ thư mục files. Khóa tệp tư vấn (advisory lock) và liên kết tượng trưng không khả dụng.

### Các thư mục của người dùng

`desktop.UserDir(cx, desktop.DownloadsDir)` trả về đường dẫn của một trong các thư mục của người dùng: `DocumentsDir`, `DownloadsDir`, `DesktopDir`, `PicturesDir`, `MusicDir` hoặc `VideosDir`. Hàm này hỏi hệ thống mỗi lần gọi, nên thư mục mà người dùng đã di chuyển vẫn được theo dõi: các thư mục trong thư mục chính (home) trên macOS (`Movies` cho video), các known folder trên Windows, kể cả của OneDrive, và các thư mục người dùng XDG trên Linux. Thư mục có thể chưa tồn tại. Điện thoại, web và HarmonyOS không có những thư mục như vậy mà ứng dụng truy cập được qua đường dẫn, Linux cũng vậy đối với thư mục mà `user-dirs.dirs` bỏ qua: `UserDir` trả về `core.ErrUserDirUnavailable`, và khi đó ứng dụng hỏi người dùng bằng `cx.PickDirectories` hoặc `cx.SaveFile`.

```go
dir, err := desktop.UserDir(cx, desktop.DownloadsDir)
if err != nil {
	dir = cx.DataDir()
}
```

Bài kiểm thử không bao giờ nhận các thư mục thật của người dùng: mỗi thư mục là một thư mục trống bên trong thư mục tạm của bài kiểm thử, và `nsgotest.UserDirs(map[core.StandardDir]string{…})` cố định chúng, thư mục nào bị bỏ qua thì không khả dụng.

### Báo cáo sự cố

Các bản dựng phát hành gốc giữ vết sự cố của Go trong `cx.LogsDir()/crashes`. Apple ghi lại các ngoại lệ Objective-C không được bắt, Android ghi lại các ngoại lệ Java không được bắt, và Windows ghi lại các ngoại lệ SEH đi tới bộ lọc cấp cao nhất của nó. Liệt kê các báo cáo đang chờ ở lần khởi chạy tiếp theo bằng `cx.CrashReports()`. Mỗi báo cáo xác định phiên bản, bản dựng và nền tảng đã tạo ra nó. Gọi `Open()` để đọc văn bản chẩn đoán, đóng reader, rồi gọi `Acknowledge()` để xóa báo cáo khi ứng dụng đã xử lý xong. Hãy chạy các thao tác tệp này trên một goroutine.

## Dung lượng đĩa

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` truy vấn bất đồng bộ ổ đĩa chứa `cx.DataDir()` và gọi lại trên goroutine của engine. `Total` là dung lượng do hệ thống báo cáo, `Available` là phần ứng dụng có thể dùng cho một thao tác do người dùng yêu cầu, và `Opportunistic` là phần nó có thể dùng cho bộ nhớ đệm và tải trước. Cả ba đều tính bằng byte. `Available` bao gồm dung lượng mà hệ thống thu hồi theo nhu cầu ở nơi nó báo cáo con số đó. Các nền tảng chỉ có một con số dung lượng trống sẽ trả về nó ở cả hai trường. Trên web, `Total` là hạn mức lưu trữ ước tính của trình duyệt cho origin và `Available` là phần chưa dùng. Khi dịch vụ không khả dụng, nó trả về `core.ErrDiskSpaceUnavailable`.

Các bản dựng Apple yêu cầu `apple.privacy.accessed_apis` trong `nsgo.yaml` khai báo `NSPrivacyAccessedAPICategoryDiskSpace`. Khai báo `E174.1` để kiểm tra dung lượng trước khi ghi hoặc để giải phóng tệp khi dung lượng thấp, với một thay đổi mà người dùng có thể quan sát. Khai báo `85F4.1` để hiển thị dung lượng khả dụng cho người dùng. Ứng dụng phải tuân theo các hạn chế của Apple về việc dùng và chia sẻ các số đo này. Một bản dựng không có danh mục này trả về `core.ErrNotDeclared`. [Các lý do được Apple chấp thuận](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

Đặt câu trả lời cho một bài kiểm thử bằng `nsgotest.DiskSpace(space, err)`.

## Lưu trữ đám mây

Gói `cloud` đồng bộ các mục khóa-giá trị nhỏ (`cloud.Get`, `cloud.Set`, `cloud.Delete`, `cloud.Keys`) và các tệp lớn hơn hoặc ảnh chụp bản lưu (`cloud.ReadFile`, `cloud.WriteFile`, `cloud.DeleteFile`, `cloud.ListFiles`) giữa các thiết bị: iCloud Key-Value Store và các container tài liệu ubiquity trên macOS và iOS, và Google Play Games Saved Games trên Android.

Mọi thao tác đều gửi hàm gọi lại trên goroutine của engine và bỏ qua nó nếu `cx` đã bị hủy. `cx.Env().CanUseCloudStorage()` báo cáo lưu trữ đám mây có khả dụng hay không; khi người dùng đã đăng xuất, thiếu entitlement, hoặc nền tảng không có backend đám mây gốc (Windows, Linux và web), mọi lời gọi đều trả về `cloud.ErrUnavailable` và không bao giờ âm thầm bỏ qua việc ghi.

```go
if cx.Env().CanUseCloudStorage() {
	cloud.WriteFile(cx, "saves/slot1.sav", saveBytes, func(err error) {
		if err != nil {
			status.Set(err.Error())
		}
	})
}

cloud.WatchChanges(cx, func(ev cloud.ChangeEvent) {
	reload(ev.Keys, ev.Files)
})

cloud.OnConflict(cx, func(c cloud.Conflict) {
	_ = c.Resolve(mergeSaves(c.Local, c.Remote))
})
```

`cloud.WatchChanges` và `cloud.OnConflict` gửi các thay đổi từ bên ngoài trên nhiều thiết bị và các xung đột phiên bản trên goroutine của engine, và tự động dọn dẹp khi `cx` bị hủy. Đọc một tệp bị xung đột trả về một `*cloud.ConflictError` (khớp với `errors.Is(err, cloud.ErrConflict)`).

Để bật entitlement iCloud của Apple trong `nsgo build`, thêm một mục `cloud:` (hoặc `icloud:`) vào `nsgo.yaml`:

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

Hồ sơ ký phải cấp các khả năng iCloud mà bạn yêu cầu. Trên macOS, chỉ các bản dựng Mac App Store (`sign.macos_profile`) mới mang iCloud, vì macOS chỉ cấp nó kèm một provisioning profile; một bản dựng Developer ID sẽ bỏ nó ra kèm một cảnh báo. Trên Android, lưu trữ đám mây cần Play Games Services v2 SDK trong ứng dụng và một người chơi đã đăng nhập.

Trên các cửa hàng máy tính như Steam, Steam Auto-Cloud đồng bộ trực tiếp `cx.DataDir()` mà không cần thêm dòng mã nào — cứ tiếp tục ghi các tệp lưu cục bộ ở đó và để Steam đồng bộ thư mục.

Trong kiểm thử, `nsgotest` cấp cho mỗi ứng dụng một kho đám mây trong bộ nhớ được cô lập: bật tắt khả dụng bằng `nsgotest.CloudAvailable` hoặc `app.SetCloudAvailable`, mô phỏng các cập nhật từ bên ngoài trên nhiều thiết bị bằng `app.SimulateCloudChange`, và kiểm thử việc giải quyết xung đột bằng `app.SimulateCloudConflict`.

## Bí mật

`cx.Secrets()` là kho bảo mật của ứng dụng, dành cho refresh token, khóa API và mật khẩu: `Get`, `Set` và `Delete` một chuỗi theo tài khoản, được nền tảng bảo vệ. Một tài khoản không tồn tại được đọc là `("", false, nil)`, và ở nơi nền tảng không có kho bảo mật, như trên web, mọi lời gọi đều trả về `core.ErrSecretsUnavailable`; NSGo không bao giờ lùi về bộ lưu trữ thông thường. Hệ thống có thể khiến một lời gọi phải chờ, như khi keychain hỏi người dùng, vì vậy hãy gọi chúng ngoài goroutine của engine và trả kết quả về bằng `cx.Post`.

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| Nền tảng | Nơi giữ bí mật |
| --- | --- |
| macOS | Một generic password của login keychain: `kSecAttrService` là service, `kSecAttrAccount` là account |
| iOS, tvOS, watchOS | Một generic password của keychain, theo cùng cách |
| Windows | Một generic credential của Credential Manager có target là `service/account`, tối đa 2560 byte |
| Linux | Một mục của Secret Service (GNOME Keyring, KWallet) với các thuộc tính `service` và `account`; không khả dụng ở nơi phiên làm việc không chạy dịch vụ nào |
| Android | Được mã hóa bằng một khóa AES của Android Keystore, trong các tệp riêng của ứng dụng, nằm ngoài bản sao lưu; việc đọc cũng lùi về các giá trị mà `expo-secure-store` đã mã hóa vào `SharedPreferences` `SecureStore` của nó |
| Mini program WeChat | Bộ lưu trữ được nền tảng mã hóa (`wx.setStorage` / `wx.getStorage` với `encrypt: true`, base library 2.21.3+); không khả dụng trên các base library cũ hơn |
| Web, các mini program khác | Không khả dụng (`core.ErrSecretsUnavailable`): trình duyệt không có kho bí mật phía client được hệ điều hành bảo vệ (hãy giữ phiên web trong cookie `HttpOnly` do máy chủ đặt), và các mini program QQ, Alipay và Douyin thiếu bộ lưu trữ mã hóa |

Theo mặc định, service là ID của ứng dụng (`core.App().ID`). Một ứng dụng chuyển từ framework khác sang sẽ giữ người dùng ở trạng thái đăng nhập bằng cách đặt tên cho các mục mà framework đó đã ghi: `Service`, và trên các nền tảng Apple là `AccessGroup` và `Accessible`. Trên Android, đặt `Service` thành `keychainService` của `expo-secure-store` (mặc định là `"key_v1"`) để đọc các giá trị mà nó đã mã hóa vào `SharedPreferences`; việc ghi hoặc xóa sẽ xóa mục đã di chuyển. Trên iOS, `core.SecretAfterFirstUnlock` cho phép công việc nền đọc một mục khi điện thoại đang khóa, và một mức truy cập `ThisDeviceOnly` giữ nó ngoài bản sao lưu. Trên macOS, một `AccessGroup` chuyển các mục sang data protection keychain, vốn cần một bản dựng được ký bởi một team có entitlement `keychain-access-groups` liệt kê nhóm đó, được cấu hình trong `macos.entitlements` của `nsgo.yaml` cho cả bản dựng Mac App Store lẫn Developer ID.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` giữ bí mật của ứng dụng trong bộ nhớ, trong `App.Secrets`, và `nsgotest.Secrets` khởi đầu một bài kiểm thử với một số bí mật, nhờ đó bài kiểm thử không bao giờ chạm vào keychain của máy. Một gói TSX truy cập kho với capability `secrets`, dưới một service riêng của nó.

## Mạng

`cx.Env().Network()` báo cáo hệ thống có một tuyến dùng được hay không (`Online`) và hệ thống có coi tuyến đó là tính phí theo dung lượng hoặc đắt đỏ hay không (`Expensive`). Các binding theo dõi nó như bất kỳ giá trị môi trường nào khác. `cx.OnNetworkChange(func(core.Network))` gọi trình xử lý trên goroutine của engine cho mỗi thay đổi trong khi scope của nó còn tồn tại. Tuyến của hệ thống không nói gì về việc một máy chủ cụ thể có phản hồi hay không. Một target không báo cáo được kết nối mặc định là online, và chi phí không xác định mặc định là false.

Đặt `core.Environment{Network: &core.Network{Online: false}}` qua `nsgotest.Environment` hoặc `App.SetEnvironment` để kiểm thử hành vi ngoại tuyến.

`net/http` của chính Go thực hiện yêu cầu trên mọi target, với các thiết lập TLS và proxy của hệ thống. Signal thuộc về goroutine của engine, vì vậy một phản hồi từ goroutine khác đến được chúng thông qua `cx.Post`.

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` chạy hàm của nó trên goroutine của đồ thị, nơi signal có thể được đặt. Trên Android, manifest yêu cầu quyền internet, và các bản tải xuống hình ảnh tự lưu vào bộ nhớ đệm trên đĩa.

`core.NewResource` làm điều tương tự cho một giá trị được tải từ một nguồn. Hàm tải của nó chạy trên một goroutine riêng và tải lại khi nguồn thay đổi:

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`, `items.Err()` và `items.Loading()` là các signal mà binding theo dõi. Một truy vấn mới sẽ hủy yêu cầu vẫn đang chạy thông qua `ctx`, nên một câu trả lời đến muộn không bao giờ thay thế một câu trả lời mới hơn. `items.Refetch()` thử lại, giữ danh sách cuối cùng trong khi tải, và `items.Mutate` đặt ngay một giá trị cho một cập nhật lạc quan.

`core.Suspense` hiển thị nội dung thay thế trong khi các resource mà nội dung của nó đọc đang tải giá trị đầu tiên, nhờ đó một màn hình không cần cờ đang tải riêng:

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

Nội dung vẫn được gắn kết nhưng bị ẩn trong khi chờ, nên nó hiển thị ngay khi các giá trị đến. Một lần tải lại giữ nó hiển thị với giá trị cuối cùng.

### Khám phá dịch vụ cục bộ

Trên macOS, iOS, Android, Windows và Linux, `cx.AdvertiseService` công bố một cổng đang lắng nghe và các thuộc tính TXT. `cx.BrowseServices` báo cáo các dịch vụ đã phân giải cùng host, cổng và thuộc tính TXT của chúng. `Host` có thể là tên máy DNS hoặc địa chỉ IP. Khi một dịch vụ rời đi, hàm gọi lại nhận lại nó với `Removed` được đặt. Kiểm tra `cx.Env().CanDiscoverServices()` để biết nền tảng có hỗ trợ không. Cả hai lời gọi đều trả về một hàm dừng và kết thúc khi scope của chúng bị hủy. Lỗi nền tảng được báo cáo bất đồng bộ trên goroutine của engine. Linux dùng Avahi và báo lỗi nếu daemon không khả dụng.

Với Bonjour trên các nền tảng Apple, hãy khai báo các kiểu dịch vụ và giải thích việc truy cập mạng cục bộ trong `nsgo.yaml`. Hệ thống quản lý lời nhắc cấp quyền và ghi nhớ lựa chọn của người dùng.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android dùng `NsdManager`. Android 12 trở về trước và Android 13 trước T Extensions 7 cần `CHANGE_WIFI_MULTICAST_STATE` để khám phá qua Wi-Fi. Các bản dựng độc lập khai báo quyền đó. Nếu bạn nhúng NSGo vào một host Android, hãy thêm nó vào manifest của host cho các thiết bị đó. Các giá trị TXT được công bố phải là UTF-8 hợp lệ trên các thiết bị không có API TXT thô của Android (API 37 hoặc T Extensions 22). Việc duyệt giữ nguyên các giá trị TXT thô. Nếu một host Android nhắm tới API 37 trở lên, nó phải khai báo và yêu cầu `ACCESS_LOCAL_NETWORK` trên Android 17 trở lên.

Dùng `nsgotest.LocalServices` để cung cấp kết quả duyệt trong một bài kiểm thử headless.

## Đang phát

Dùng `cx.SetNowPlaying(core.NowPlaying{...})` để hiển thị nội dung đa phương tiện hiện tại của ứng dụng trong Control Center và thanh menu của macOS. Cung cấp `Title`, `Artist`, `Album` và `Artwork` tùy chọn dưới dạng một `*core.ImageSource`. `Duration` bằng 0 nghĩa là một luồng trực tiếp. Cung cấp `Position` hiện tại và `Rate` phát (0 khi tạm dừng, 1 ở tốc độ bình thường). Công bố lại khi bản nhạc, vị trí hoặc tốc độ thay đổi. Hệ thống tự tăng vị trí giữa các lần gọi.

Liệt kê các điều khiển phát được hỗ trợ trong `Commands`, chẳng hạn `core.MediaPlay`, `core.MediaPause`, `core.MediaTogglePlayPause` và `core.MediaSeek`. Xử lý các yêu cầu bằng `cx.OnMediaCommand`. Hàm gọi lại nhận `Kind` của lệnh và, với lệnh tua, `Position` được yêu cầu. Trình xử lý còn sống mới nhất sẽ nhận lệnh. Gọi `cx.ClearNowPlaying()` khi ứng dụng phát xong. Trong khi ứng dụng công bố thủ công, các video có điều khiển trên macOS tạm ngừng việc tự công bố của chúng. Kiểm tra `cx.Env().CanShowNowPlaying()` để biết nền tảng có hỗ trợ không.

## Tải xuống nền

Dùng `cx.Download` và `cx.OnDownloadUpdate` trong các ứng dụng iOS và macOS độc lập, và trên Android. Các view Apple được nhúng không có host truyền tải. Ứng dụng Android bật `android.background_downloads: true` trong `nsgo.yaml`. Bắt đầu một lần tải xuống từ một hành động của người dùng trong khi ứng dụng đang hiển thị. Dùng một đích tuyệt đối có thư mục cha đã tồn tại mà ứng dụng có thể ghi.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

Việc truyền tải dùng GET và chấp nhận các header yêu cầu tùy chọn. Nó dùng kho tin cậy HTTPS thông thường trừ khi `CertificatePEM` ghim chứng chỉ lá của máy chủ. Việc ghim cũng yêu cầu tên máy khớp và chứng chỉ hợp lệ. Hãy dùng URL cuối cùng vì dịch vụ yêu cầu một phản hồi thành công từ URL đó.

Trên Apple, một chứng chỉ tự ký tại một tên máy công khai cần một ngoại lệ App Transport Security cho host đó trong `apple.info`. Việc truyền tải vẫn kiểm tra chính xác chứng chỉ được ghim, tên máy và tính hợp lệ. Một kết nối NAS cục bộ cũng cần một `NSLocalNetworkUsageDescription` giải thích việc truy cập, và các host không đủ điều kiện hoặc `.local` dùng `NSAllowsLocalNetworking`. [Các thiết lập ATS của Apple](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Dịch vụ tiền cảnh (foreground service) của Android tiếp tục chạy sau khi activity rời khỏi màn hình. Nó đăng một thông báo tiến độ, tùy thuộc vào quyền thông báo của Android. Tiến độ dùng số byte và đặt `Total` thành -1 cho đến khi biết. Khi thành công, đích được thay thế một cách nguyên tử. `task.Pause()` giữ tệp tải dở, `task.Resume()` dùng một HTTP range khi máy chủ cung cấp validator, và `task.Cancel()` xóa tệp tải dở.

iOS dùng một URLSession nền của hệ thống, tiếp tục truyền tải trong khi ứng dụng bị tạm ngưng. Ứng dụng kết nối lại phiên của mình khi khởi chạy và công bố tệp trước khi xác nhận hoàn tất nền. Trên macOS, việc truyền tải chạy trong tiến trình. Các nền tảng Apple dùng dữ liệu tiếp tục gốc cho các bản tải xuống bị tạm dừng và giữ siêu dữ liệu yêu cầu trong bộ lưu trữ riêng của ứng dụng cho đến khi một kết quả cuối cùng được lưu. Một máy chủ không thể tiếp tục sẽ khiến việc truyền tải bắt đầu lại.

Đăng ký lại `OnDownloadUpdate` khi ứng dụng khởi động để thấy các tác vụ đang hoạt động và các kết quả đã lưu. Dùng `p.Task.Forget()` sau khi xử lý một kết quả cuối cùng. Android có thể dừng dịch vụ, chẳng hạn khi thời gian truyền tải tiền cảnh hết hạn. Khi đó ứng dụng nhận `DownloadInterrupted`. Trên Windows, Linux, tvOS, watchOS và web, `Download` trả về `ErrDownloadUnavailable`.

## Nhận dạng giọng nói và vòng đời

`cx.StartSpeech` nhận dạng giọng nói bằng trình nhận dạng của hệ thống ở nơi `cx.Env().CanRecognizeSpeech()` cho biết nó tồn tại (macOS, iOS, Android, Windows và các trình duyệt cung cấp `SpeechRecognition` hoặc `webkitSpeechRecognition`), và trả lời bằng bản ghi khi nó thay đổi. Trên các nền tảng Apple, hãy khai báo `NSMicrophoneUsageDescription` và `NSSpeechRecognitionUsageDescription` trong `apple.info`; trên Android, thêm `RECORD_AUDIO` vào `android.permissions`. Trong một bài kiểm thử headless, `app.Speech` giữ một `nsgotest.SpeechSession` trong khi việc nhận dạng đang hoạt động. Dùng `app.HearSpeech` và `app.EndSpeech` để gửi kết quả.

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` cho biết ứng dụng đang hoạt động, không hoạt động hay ở nền: trên máy tính để bàn là cửa sổ chứa `cx`, nên nội dung của một cửa sổ bị thu nhỏ có thể tạm dừng trong khi một cửa sổ khác đang hiển thị, và trong scope của chính ứng dụng là tất cả các cửa sổ của nó gộp lại. `desktop.Minimized(cx)` cho biết cửa sổ đó có bị thu nhỏ không, và `desktop.FrontmostApp(cx)` cho biết ứng dụng nào đang ở phía trước trên máy tính, kể cả chính ứng dụng, và cửa sổ của nó có ở chế độ toàn màn hình không, như một thú cưng trên màn hình hay một trình theo dõi thời gian theo dõi nó; `desktop.CanReadFrontmostApp(cx)` cho biết nó có được theo dõi không, trên macOS, Windows, X11 và các compositor Wayland được hỗ trợ. `cx.OnQuit` dọn dẹp khi ứng dụng thoát, dù thoát theo cách nào, với 200 ms cho công việc của nó. `desktop.OnQuitRequest` có thể hủy hoặc hoãn một mục menu Quit trên máy tính hoặc `desktop.RequestQuit(cx)`. Trên Windows và Linux, nó cũng chạy sau `OnClosing` khi việc đóng một cửa sổ sẽ kết thúc ứng dụng. `desktop.RequestRestart(cx)` hỏi các trình xử lý tương tự trước khi khởi động lại; `desktop.Restart(cx)` vẫn diễn ra ngay lập tức. Các lần khởi chạy lại do trình cập nhật trên Linux và Windows cũng hỏi, nên việc lưu có thể hủy hoặc hoãn chúng. Khi hệ thống báo đăng xuất, khởi động lại hoặc tắt máy, `q.Forced()` là true và ứng dụng tiếp tục qua `OnQuit`. Kiểm tra `Forced()` trước khi hỏi người dùng đưa ra quyết định. `cx.SetAppearance` ép giao diện sáng hoặc tối, `cx.SetLocale` chọn ngôn ngữ, `cx.OnKey` gắn một phím tắt bàn phím, phím tắt này nghỉ khi những gì chủ sở hữu của nó hiển thị bị ẩn hoặc nằm dưới một hộp thoại trừ khi `core.KeyOptions{OverModals: true}` giữ nó, nó nhận phím trước menu ứng dụng trừ khi `YieldsToMenu` nhường phím cho một mục menu đang bật dùng cùng phím tắt ở nơi có tiêu điểm (khi một trường văn bản hoặc web view có tiêu điểm, một mục `MenuNotEditing` để lại phím cho binding và giữ phím khỏi các mục sau có cùng phím tắt, còn menu của Linux không nhận phím nào được nhấn mà không có Control, Alt hoặc phím logo, nên binding giữ phím như vậy ở đó), nó hoạt động trong cửa sổ chứa `cx` trừ khi `AllSurfaces` gắn nó vào mọi cửa sổ và bảng của ứng dụng, kể cả những cái được mở sau, cho một phím tắt toàn ứng dụng, phím tắt này nghỉ trong tất cả chúng khi những gì chủ sở hữu của nó hiển thị bị ẩn trong cửa sổ mà nó được gắn vào, và nó để lại cho một điều khiển đang có tiêu điểm những phím mà điều khiển đó dùng, Return và Space cho một nút, các phím mũi tên cho một thanh trượt, thanh thẻ hoặc nhóm radio, trừ khi được gắn với `core.KeyAlways` hoặc `core.KeyNotEditing`; `cx.OnOpenFiles` nhận các tài liệu được mở trên macOS, Windows, Linux và iOS. iOS nhập các tệp thông thường vào bộ lưu trữ tạm thời do ứng dụng sở hữu; hãy di chuyển chúng sang `cx.DataDir()` để giữ lại. Trên macOS, đóng cửa sổ đã cung cấp menu ứng dụng sẽ khôi phục menu của cửa sổ còn tồn tại gần đây nhất, hoặc menu mặc định khi không còn cửa sổ nào.

Trên Wayland, việc theo dõi ứng dụng ở phía trước cần phiên bản 2 trở lên của giao thức wlr foreign-toplevel của compositor; các compositor khác báo `CanReadFrontmostApp` là false. ID của nó là ID ứng dụng Wayland, tên của nó lấy từ desktop entry khi có, và PID của nó là 0. Các ứng dụng Windows đóng gói dùng AUMID thay vì đường dẫn frame-host. macOS theo trạng thái trình bày toàn màn hình của AppKit thay vì suy ra từ kích thước cửa sổ.

Một vai trò chỉnh sửa trong menu ứng dụng nhận trình soạn thảo đang có tiêu điểm trước khi dùng `OnSelect` dự phòng của nó. Trên macOS, Undo và Redo theo lịch sử của trình soạn thảo đó, và Cut theo vùng chọn của nó, kể cả khi ứng dụng không hoạt động; khi không có trình soạn thảo, mục dự phòng vẫn được bật.

Trên iOS, hãy cung cấp cho `cx.OnKeyWith` một hàm `core.KeyOptions.Title` để đưa phím tắt vào menu bàn phím của hệ thống trong khi binding có thể kích hoạt. `Group` đặt tên cho menu của nó trên iPadOS 26 trở lên. Các hàm này có thể đọc catalog của ứng dụng để tiêu đề phím tắt theo ngôn ngữ. Dùng `app.KeyCommands()` trong kiểm thử để xem danh sách hiện tại.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` là tên mà người dùng đặt cho thiết bị, và `cx.Env().DeviceModel()` là mẫu phần cứng của nó, chẳng hạn `Mac15,7`, `iPhone15,2` hoặc `Google Pixel 8`, để ứng dụng có thể phân biệt hai thiết bị đã ghép đôi. macOS, Android, Windows và Linux cung cấp cả hai; iOS cung cấp mẫu, và chỉ cung cấp tên cho ứng dụng có entitlement tên thiết bị do người dùng đặt của Apple. watchOS cung cấp mã định danh phần cứng, HarmonyOS cung cấp tên thương mại hoặc mẫu sản phẩm, và mini program cung cấp thương hiệu và mẫu của nền tảng; các target đó để trống tên do người dùng đặt. Web không cung cấp cả hai. Các bài kiểm thử đặt chúng bằng `nsgotest.Environment`.

`cx.Env().ThermalState()` báo cáo `core.ThermalNominal`, `ThermalFair`, `ThermalSerious` hoặc `ThermalCritical` trên các nền tảng Apple và Android API 29 trở lên. `cx.Env().LowPowerMode()` báo cáo `core.LowPowerOn` hoặc `LowPowerOff` trên các nền tảng Apple, Android, Windows và các phiên Linux có power-profiles-daemon. Ở nơi hệ thống không cung cấp giá trị, các lần đọc trả về `ThermalUnknown` và `LowPowerUnknown`. Mỗi lần đọc theo dõi thay đổi độc lập, nên ứng dụng có thể điều chỉnh khối lượng công việc trong khi chạy. Trong kiểm thử, đặt `ThermalState` và `LowPowerMode` trên `core.Environment` qua `nsgotest.Environment` hoặc `app.SetEnvironment`.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` đọc vị trí con trỏ kể cả bên ngoài các cửa sổ của ứng dụng trên macOS, Windows và X11. Nó lấy một mẫu cho mỗi lần gọi. Điểm này dùng cùng hệ tọa độ màn hình với `desktop.Monitors(cx)` và `Window.ShowAt`. Trên Wayland và các target không được hỗ trợ khác, hoặc nếu scope thiếu `core.AccessCursorPoint`, `available` là false.

`desktop.SetWindowConstraints` giới hạn kích thước nội dung và điều khiển việc thay đổi kích thước, thu nhỏ và phóng to; đặt `WindowOptions.Constraints` khi mở một cửa sổ hoặc dùng `nsgo.WindowConstraints` cho cửa sổ chính. Linux áp dụng giới hạn tối đa không cố định trên X11 và gỡ các nút thu nhỏ/phóng to bị cấm khỏi thanh tiêu đề gốc của GTK trên X11 và Wayland. Wayland hiện chỉ hỗ trợ kích thước cố định cho giới hạn trên.

`desktop.SetWindowLevel` đặt một cửa sổ nổi phía trên các cửa sổ khác, và `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` hoặc `WindowOptions.Workspaces` giữ nó trên mọi không gian làm việc, như một thú cưng trên màn hình, một HUD hay một đồng hồ hẹn giờ: trên macOS là trên mọi Space và phía trên các không gian toàn màn hình của ứng dụng khác, ngoài Cmd-`, và trên Linux là trên mọi màn hình ảo của một trình quản lý cửa sổ X11. Windows và Wayland để nó ở không gian nơi nó được mở, và `desktop.CanSetWindowWorkspaces` cho biết nó có tác dụng không: true trên macOS và dưới một trình quản lý cửa sổ X11 hỗ trợ `_NET_WM_DESKTOP`. Một cửa sổ chính được đặt trên mọi không gian làm việc bằng cách gọi `desktop.SetWindowWorkspaces` từ nội dung của nó. Trong kiểm thử, `nsgotest.App.Window` liệt kê các yêu cầu, và `nsgotest.Environment` đặt `CanSetWindowWorkspaces`.

`desktop.RegisterHotKey` gắn một phím tắt toàn cục, hoạt động khi bất kỳ ứng dụng nào ở phía trước, trên macOS, trên Linux và trong một ứng dụng Windows chạy bằng `nsgo.Run(App)`. `State` của handle cho biết hệ thống có nhận nó không: `HotKeyPending` cho đến khi hệ thống trả lời, sau đó là `HotKeyRegistered`, `HotKeyTaken` khi một đăng ký khác giữ tổ hợp đó, hoặc `HotKeyUnsupported` cho một phím hoặc một nền tảng không có phím tắt toàn cục, để một trình ghi phím tắt có thể yêu cầu một tổ hợp khác. Trên macOS, một tổ hợp chỉ được coi là đã bị chiếm khi ứng dụng đã giữ nó hoặc một ứng dụng khác đã đăng ký nó độc quyền. Trên Wayland, compositor yêu cầu người dùng gắn phím tắt: nó được đăng ký khi họ đã gắn, và bị coi là đã chiếm khi họ hoặc compositor bỏ qua nó. Trong kiểm thử, `nsgotest.HotKeyTaken` và `nsgotest.HotKeyUnsupported` khiến các tổ hợp trả lời như vậy.

Với `desktop.TitleBarHidden`, Windows và Linux cung cấp menu ứng dụng dưới dạng menu bật lên khi Alt được nhấn và thả một mình hoặc F10. Alt cùng chữ cái hoặc chữ số đầu tiên của một nhãn cấp cao nhất sẽ mở menu con đó hoặc chọn lệnh đó, theo hệ chữ viết của nhãn. Các binding của ứng dụng giữ phím của chúng; Shift+F10 vẫn là phím menu ngữ cảnh, và các tổ hợp Alt khác thuộc về hệ thống. Escape đóng menu bật lên và khôi phục tiêu điểm bàn phím.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

Khi thay đổi kích thước trên máy tính macOS, Windows và Linux, hệ thống chờ tối đa 16 ms để có bố cục mới trước khi vẽ. Hãy giữ các phép tính phản ứng và trình xử lý sự kiện nhanh để bố cục kịp thời hạn đó; thực hiện I/O chậm trên một goroutine và gửi kết quả qua `cx.Post`. Một ứng dụng bận vẫn tiếp tục thay đổi kích thước và hiển thị bố cục của nó khi bắt kịp.

`desktop.SetWindowFrame` di chuyển một cửa sổ khi `WindowFrame.Move` là true, dùng `X` và `Y` tính bằng point từ góc trên bên trái của màn hình chính. Các kích thước không được đặt giữ nguyên giá trị hiện tại. Kiểm tra `desktop.CanPlaceWindows`: macOS, Windows và Linux trên X11 hỗ trợ định vị; một toplevel Wayland thông thường giữ vị trí của compositor. `desktop.Frame` theo vị trí của cửa sổ gốc trên các máy tính này.

Trên macOS và Windows, `WindowFrame{Animated: true}` chuyển mượt một bảng đang hiển thị sang khung mới, kể cả kích thước của một bảng được neo. Các bảng bị ẩn và chế độ giảm chuyển động áp dụng khung mới ngay lập tức. Các cửa sổ macOS thông thường dùng hoạt ảnh của AppKit.

Đặt `desktop.WindowOptions.Parent` thành một cửa sổ đang mở của ứng dụng để đóng cả hai khi cửa sổ cha đóng. Trên máy tính, cửa sổ con ở phía trên cửa sổ cha và theo nó khi nó ẩn hoặc thu nhỏ; một cửa sổ con bị ẩn tường minh vẫn ẩn khi cửa sổ cha quay lại. Trên macOS, nó cũng được căn giữa trên cửa sổ cha khi `Frame` không được đặt và theo cửa sổ cha vào chế độ toàn màn hình.

`WindowOptions.NoShadow` và `desktop.SetWindowShadow(cx, false)` cũng bỏ bóng của một cửa sổ Windows có thanh tiêu đề bị ẩn, vốn tự vẽ phần khung của mình; một cửa sổ có thanh tiêu đề của hệ thống giữ khung và bóng của nó, cũng như một cửa sổ Linux được trình quản lý cửa sổ trang trí, nơi `NoShadow` chỉ xóa bóng của phần trang trí riêng của GTK.

`desktop.SetWindowPointerEvents` cho con trỏ xuyên qua một cửa sổ tới các cửa sổ bên dưới, của bất kỳ ứng dụng nào: `WindowPointerNone` ở mọi nơi, như một màn hình hiển thị thông tin (HUD), và `WindowPointerPainted` ở bất cứ nơi nào không có view nào vẽ gì hoặc nghe con trỏ, như xung quanh một thú cưng trên màn hình vẫn nhận nhấn, kéo và nhấp chuột phải trên chính nó; thanh tiêu đề do hệ thống vẽ được tính là không vẽ gì. `WindowOptions.PointerEvents` và `PanelOptions.PointerEvents` mở một cửa sổ theo cách đó. Một vùng `Desktop.WindowDrag` vẫn kéo được bên trong một tổ tiên có thể nhấn; một điều khiển có thể nhấn hoặc chỉnh sửa ở gần hơn giữ đầu vào của riêng nó thay vì kéo cửa sổ. Một cửa sổ không nhận con trỏ sẽ không nghe được lần nhấn nào, vì vậy ứng dụng trả lại nó từ một phím tắt, một biểu tượng khay hoặc một cửa sổ khác. Mã do một `Access` giữ cần `core.AccessOverlay`.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

Trên macOS và Windows, một hình ảnh trong một cửa sổ painted có thể đặt `Image.HitTestAlpha` (TSX `hitTestAlpha`) thành một giá trị lớn hơn 0 và tối đa là 1. Ví dụ, 0.5 nhận con trỏ trên các pixel hình ảnh có độ mờ đục ít nhất một nửa. Ngưỡng này cũng áp dụng cho `OnPress` của hình ảnh. Nền hoặc viền của view vẫn được tính là đã vẽ. Giá trị 0 dùng toàn bộ view hình ảnh, cũng như các hình ảnh trên Linux.

Trong một cửa sổ painted trên macOS, `Video.HitTestAlpha` (TSX `hitTestAlpha`) áp dụng cùng ngưỡng cho các khung video đã giải mã. Nó hoạt động khi `Controls` tắt và với một track video không biến đổi, chẳng hạn HEVC có alpha cho một thú cưng trên màn hình. Giá trị 0, một khung không khả dụng và các host máy tính khác dùng biên của view video.

`desktop.NewTray` có thể vẽ một biểu tượng động bằng `TrayOptions.View` trên macOS và Windows. Windows khớp bản kết xuất trong suốt của view với kích thước biểu tượng của vùng thông báo và cập nhật nó khi view thay đổi hoặc một hình ảnh tải xong; dùng `TrayOptions.OnClick`, `Panel` và `Menu` cho các hành động của nó. Linux hiện hiển thị biểu tượng PNG; vẽ view khay của nó vẫn là một mục tiêu.

`desktop.OpenPanel` mở một bảng mà một phím tắt như vậy hoặc một biểu tượng khay hiển thị: một cửa sổ không viền nằm phía trên các cửa sổ khác. Giống như `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` và `core.NewHolder`, nó thuộc trực tiếp về `cx` bạn truyền vào chứ không phải chủ sở hữu của trình xử lý đang chạy, nên truyền scope của ứng dụng từ `OnPress` của một nút sẽ giữ cửa sổ, bảng hoặc trình phát tồn tại sau khi scope của nút đó biến mất; các primitive phản ứng (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) thì ngược lại thuộc về chủ sở hữu đang chạy, mà `cx.Run` chuyển đổi. Một thẻ bật lên tự vẽ trên một bảng với `Transparent`, vốn không có nền, khung hay bóng riêng, nên các góc bo tròn và `BoxShadow` của thẻ là tất cả những gì hiển thị, với chỗ cho bóng bên trong bảng; `FitContent` đặt kích thước bảng theo nội dung của nó, tối đa `Width` và `Height`, trước khi nó hiển thị lần đầu và khi nội dung thay đổi. `desktop.Transparent` cho biết hiện tại bảng có không tự vẽ gì không, như với một cửa sổ trong suốt, trong khi `desktop.CanMakeWindowsTransparent` chỉ cho biết một cửa sổ có thể trong suốt hay không: ở nơi nó là false, như trên Linux không có compositor, hãy cho nội dung một nền.

Trên macOS, một bảng không có `Transparent` dùng góc vuông khi một trong hai chiều nhỏ hơn 24 point và khôi phục góc bo tròn ở kích thước bình thường.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` hiển thị `TrayOptions.Title` bên cạnh hình ảnh hoặc view được vẽ của nó trên macOS. Với một cửa sổ bật lên tùy chỉnh, hãy đọc `tray.ClickBounds()` bên trong `OnClick` hoặc `Menu` và truyền hình chữ nhật khả dụng cho `panel.ShowAt(bounds, core.PlaceBottom)`. Nó dùng cùng các point màn hình như `Env.Displays` và bao gồm cả tiêu đề. Biên không khả dụng trước lần nhấp đầu tiên, sau khi gỡ bỏ, trên Linux và khi shell của Windows không cung cấp được. Mỗi lần nhấp thay thế biên trước đó, nên một kết quả không khả dụng không bao giờ dùng lại một hình chữ nhật cũ. Đặt `TrayOptions.Panel` sẽ giữ hành vi nhấp-để-bật-tắt có sẵn thay vào đó.

`PanelOptions.Dock` giữ một bảng ở giữa dọc theo cạnh khả dụng của một màn hình qua `Show`, `Focus`, `Toggle` và việc thay đổi kích thước nội dung. Đặt `Edge` thành `core.PlaceTop`, `PlaceBottom`, `PlaceLeft` hoặc `PlaceRight`, `Display` thành một ID của `Env.Displays` (để trống cho màn hình chính), và `Margin` thành khoảng cách từ cạnh đó tính bằng point. Một bảng được neo không thể dùng `ShowAt` hoặc `MoveTo`; `SetWindowFrame` chỉ thay đổi kích thước của nó. Một biểu tượng khay hiển thị nó tại vị trí neo thay vì dưới biểu tượng.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` báo cáo khả năng neo riêng biệt với khả năng đặt cửa sổ tùy ý. macOS, Windows và X11 hỗ trợ nó; Wayland cần thư viện runtime tùy chọn `libgtk4-layer-shell.so.0` và một compositor triển khai `zwlr_layer_shell_v1`. NSGo tải thư viện đó trước GTK; một host nhúng đã tải Wayland phải tải trước gtk4-layer-shell trước khi khởi động host. Thiếu thư viện, giao thức hoặc màn hình được chọn sẽ để bảng ẩn và gọi `OnPlacementError`. Một bảng neo trên Wayland dùng lớp trên cùng mà không dành riêng không gian làm việc: `NoActivate` để bàn phím lại cho cửa sổ hiện tại, còn một bảng không có nó sẽ nhận tiêu điểm bàn phím độc quyền cho đến khi bị ẩn. Các cửa sổ Wayland thông thường giữ vị trí thông thường do compositor đặt.

Một trình ghi phím tắt cho phép người dùng chọn một phím như vậy. `OnKeyDown` của một view `Focusable` nghe mỗi phím được nhấn xuống trong khi nó có tiêu điểm, trước các binding của ứng dụng, cùng các phím bổ trợ đang giữ, kể cả một phím bổ trợ được nhấn một mình; trả về `true` sẽ giữ phím khỏi các binding, nên việc ghi Command+S không lưu. `KeyEvent.Shortcut` viết lần nhấn theo dạng mà `OnKey` và `RegisterHotKey` nhận, mỗi phím bổ trợ theo tên ("ctrl+shift+k", "alt+space", "shift+meta+k" cho Command+Shift+K), và "" cho một phím bổ trợ đơn lẻ. `OnKeyUp` của nó nghe mỗi phím đó được nhả ra, kể cả sau khi `OnKeyDown` đã tiêu thụ nó hoặc tiêu điểm đã chuyển đi, cùng các phím bổ trợ vẫn đang giữ. Cả hai hoạt động trên macOS, Windows, Linux, web (nơi trình duyệt giữ các phím tắt mà nó không trao cho trang nào), Apple TV và trong `nsgotest`; Android gửi sự kiện bàn phím phần cứng tới các view có tiêu điểm một cách bất đồng bộ, nên giá trị trả về của `OnKeyDown` không thể tiêu thụ phím. iPhone và iPad hỗ trợ thăm dò phím phần cứng. Một phím tắt mà ứng dụng đã đăng ký toàn cục sẽ nhận phím trước view, nên một trình ghi sẽ hủy đăng ký nó trong khi ghi.

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

Một trò chơi hoặc một canvas thì thăm dò các phím. `core.KeysDown(cx)` liệt kê các phím đang được giữ trên cửa sổ và `core.KeyHeld(cx, "left")` cho biết một phím có đang được giữ không: đây là các lần đọc được theo dõi, mà một vòng lặp `RequestAnimationFrame` cũng đọc. Lần đọc đầu tiên khiến nền tảng báo cáo mọi phím từ đó trở đi, nên một phím đã được giữ từ trước vẫn không được biết cho đến khi được nhấn lại; các phím được gõ vào một trường hoặc một web view, hoặc trong khi một bộ gõ đang soạn, không bao giờ được coi là đang giữ, và một phím vẫn đang giữ khi cửa sổ mất tiêu điểm sẽ được coi là nhả ra lúc đó. Chúng hoạt động ở nơi `OnKeyUp` hoạt động và trên iPhone và iPad có bàn phím phần cứng.

Với các nút chuột, `View.OnMouseDown` và `View.OnMouseUp` gửi một `core.MouseButtonEvent` cho mọi nút (`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`) trên macOS, Windows, Linux, web và `nsgotest`, mà không biến view thành một điểm dừng tiêu điểm bàn phím. `core.MouseButtonsDown(cx)` và `core.MouseButtonHeld(cx, core.MouseButtonRight)` là các lần đọc được theo dõi của các nút đang giữ trên cửa sổ, tự động xóa khi cửa sổ mất tiêu điểm.

Với một điều khiển gắn với vị trí vật lý, hãy đọc `KeyEvent.Code`, `core.KeyCodesDown(cx)` hoặc `core.KeyCodeHeld(cx, "KeyW")`. Mã dùng các tên của `KeyboardEvent.code`: trên AZERTY, vị trí W của QWERTY có `Code: "KeyW"` và `Key: "z"`. `core.KeyCodeLabel(cx, "KeyW")` cho nhãn hiển thị của bố cục đang dùng để làm lời nhắc. Trên iOS, Android, HarmonyOS và các trình duyệt không có bản đồ bố cục, nó dùng tên phím cuối cùng được báo cáo tại vị trí đó và trả về chuỗi rỗng cho đến khi thấy một phím. HarmonyOS đặt tên cả vị trí lẫn phím từ `KeyCode` của InputKit, vì ArkUI không báo cáo scan code, nên ở đó tên phím chưa theo một bố cục không phải QWERTY.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## Giữ màn hình luôn sáng

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` giữ màn hình luôn sáng cho đến khi hàm giải phóng được trả về chạy hoặc scope hiện tại bị hủy. Trên Android, việc giữ áp dụng trong khi ứng dụng đang hiển thị. Trên web, nó hoạt động trong một ngữ cảnh bảo mật có hỗ trợ Screen Wake Lock, và trang yêu cầu khóa lại khi nó hiển thị trở lại. Safari có thể yêu cầu một lần nhấn hoặc một phím trước khi chấp thuận yêu cầu đầu tiên. Các yêu cầu chỉ giữ máy tính không có tác dụng trên hai nền tảng này.

## Khóa con trỏ

Trong camera của một trò chơi hoặc một thao tác kéo cần tiếp tục ở mép màn hình, hãy gọi `ref.LockPointer()` từ một lần nhấn trên view giữ ref đó. Con trỏ ở yên một chỗ và bị ẩn trong khi view nhận chuyển động tương đối trong `OnPointerDelta(dx, dy)`. Giá trị dương di chuyển sang phải và xuống dưới.

Gọi hàm được trả về để giải phóng khóa. Escape, cửa sổ mất tiêu điểm, hoặc ẩn hay gỡ view cũng giải phóng nó. Kiểm tra `cx.Env().CanLockPointer()` để xem surface có hỗ trợ không. Web yêu cầu một cử chỉ của người dùng như một lần nhấn. Linux trên Wayland yêu cầu một compositor hỗ trợ pointer constraints và relative motion.

## Giới hạn âm thanh

Dùng `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` trước khi tải âm thanh để thay đổi giới hạn của engine. `MaxEncodedBytes`, `MaxDecodedBytes` và `MaxPlaybacks` cũng có thể được đặt. Các trường bằng 0 dùng giá trị mặc định là 8 MiB đã mã hóa và 5 giây cho mỗi âm thanh, 64 nguồn, 16 MiB đã giải mã trên tất cả âm thanh, và 32 lượt phát chồng nhau cho mỗi đầu ra. Các âm thanh hiện có vẫn được giữ và tính vào giới hạn mới. Các scope bị hạn chế giữ giới hạn mặc định trong phạm vi ngân sách của engine: âm thanh `nsgo/audio` của một gói TSX đã ký chạm tới chúng bất kể ứng dụng Go đã đặt gì, trong khi TSX riêng của ứng dụng nhận giới hạn của ứng dụng Go. SoundPool của Android phát tối đa 32 âm thanh cùng lúc và giải mã tối đa 1 MiB của mỗi âm thanh, bất kể giới hạn là gì.

## Toàn màn hình

`desktop.SetFullScreen(cx, true, displayID)` lấp đầy màn hình được đặt tên bởi `cx.Env().Displays()` trên macOS, Windows và Linux. Bỏ ID để dùng màn hình hiện tại của cửa sổ. Một ID không xác định để cửa sổ ở nguyên chỗ. Lặp lại cùng một yêu cầu không có tác dụng. `desktop.SetFullScreen(cx, false)` thoát toàn màn hình và khôi phục khung trước đó của cửa sổ. Trên web, hãy vào toàn màn hình từ một trình xử lý nhấn. Trình duyệt chọn màn hình, và `cx.Env().FullScreen()` theo việc vào và thoát, kể cả Esc.

## Màn hình

`cx.Env().Display()` trả về `core.DisplayInfo` hiện tại của cửa sổ và cập nhật các binding khi cửa sổ chuyển sang màn hình khác. `cx.Env().Displays()` liệt kê các màn hình mà nền tảng báo cáo. `RefreshRate` và `MaxRefreshRate` tính bằng Hz, bằng 0 khi tần số không khả dụng. iOS báo cáo tần số tối đa của tấm nền. HarmonyOS báo cáo màn hình của cửa sổ, tần số hiện tại và tần số cao nhất được hỗ trợ. Web báo cáo màn hình hiện tại và đánh dấu số đo từ frame callback của nó bằng `RefreshRateEstimated`. Trong kiểm thử, đặt `Displays` và `DisplayID` qua `nsgotest.Environment`.

## Đọc văn bản

`cx.Speak` đọc to văn bản bằng giọng của hệ thống cho một ngôn ngữ ở nơi `cx.Env().CanSpeak()` cho biết nền tảng có thể đọc: macOS, iOS, tvOS, Android, Windows, web và Linux có speech-dispatcher. Mỗi lần chỉ đọc một câu: một `Speak` khác sẽ cắt câu đang đọc, và `cx.StopSpeaking` cùng việc kết thúc scope đã đọc nó cũng vậy. `done` nghe một lần về cách nó kết thúc: hoàn tất, bị dừng, hoặc thất bại với một `core.SpeakError`, chẳng hạn `core.SpeakLanguageUnavailable` khi không có giọng nào được cài đặt nói ngôn ngữ đó.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` và `Pitch` chạy từ 0.5 đến 2, với 1 là giá trị riêng của giọng. `cx.Voices("ja", ...)` liệt kê các giọng được cài đặt cho một ngôn ngữ, để ứng dụng có thể yêu cầu người dùng cài một giọng còn thiếu hoặc để họ chọn một giọng, mà `ID` của nó được đặt vào `core.Voice`. Trình duyệt chỉ đọc sau khi người dùng đã nhấn vào thứ gì đó trên trang, và thất bại với `core.SpeakNotAllowed` trước đó. Các thành phần Go của một gói TSX chỉ đọc được khi có capability `text_to_speech`.

## Phát âm thanh lặp lại

Đặt `audio.Voice.Loop` để lặp lại một lượt phát âm thanh cho đến khi `Stop` hoặc `Close`. Với nhạc phát trực tuyến, đặt `audio.PlayerOptions.LoopStart` và gọi `SetLoop(true)`. Lượt đầu tiên bao gồm phần mở đầu. Các lượt sau bắt đầu tại vị trí bù. Vị trí bù bằng 0 lặp lại toàn bộ bản nhạc.

Trên macOS 14 và iOS 17 trở lên, một bản nhạc hữu hạn dùng hàng đợi gốc để chuẩn bị mỗi lần lặp trước khi lần hiện tại kết thúc. Các phiên bản Apple cũ hơn tua về điểm bắt đầu vòng lặp khi lượt hiện tại kết thúc, với một khoảng ngắt ngắn. Trên Linux và web, một trình phát lặp giải mã một bản nhạc tối đa 32 MiB và 5 phút vào bộ nhớ, khoảng 23 MB mỗi phút, và lặp lại nó không có khoảng ngắt. Trên Linux, một bản nhạc lớn hơn hoặc dài hơn được phát trực tuyến qua bộ giải mã riêng và cũng lặp lại không có khoảng ngắt; trên web nó tua lại ở cuối, với một khoảng ngắt ngắn. Trên Windows và Android, một vòng lặp có điểm bắt đầu sẽ tua về đó ở cuối, với một khoảng ngắt ngắn. Trên web, một trình phát lặp ở tốc độ khác 1 cũng tua lại ở cuối, vì phát liền mạch của trình duyệt sẽ làm lệch cao độ theo tốc độ. Trên web, một trình phát lặp, giống như một âm thanh, chỉ phát sau khi người dùng đã nhấn vào thứ gì đó trên trang, và thất bại với `NotAllowed` trước đó.

## Sinh trắc học

`cx.BiometricAvailability` báo cáo Face ID, Touch ID hoặc vân tay của thiết bị đã được đăng ký hay chưa; Android không cho biết loại nào đã được đăng ký, nên ở đó nó báo vân tay khi có vân tay được đăng ký và nếu không thì báo khuôn mặt khi thiết bị có phần cứng nhận diện khuôn mặt. `cx.AuthenticateBiometric` yêu cầu hệ thống xác nhận người dùng kèm một lý do, lý do này không được để trống, và báo cáo thành công, thất bại, bị hủy hoặc không khả dụng. Nó không chọn phương án dự phòng bằng mật mã cho ứng dụng.

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

Ứng dụng dùng Face ID khai báo `NSFaceIDUsageDescription` trong `apple.info`; ứng dụng Android liệt kê `USE_BIOMETRIC` trong `android.permissions`.

## Đăng nhập

`cx.AuthSession` đăng nhập người dùng với một nhà cung cấp OAuth trong phiên xác thực của hệ thống thay vì một web view: `ASWebAuthenticationSession` trên macOS, iOS và watchOS, một Custom Tab hoặc trình duyệt trên Android, trình duyệt mặc định trên Windows, Linux và một máy Mac không có scheme, và một cửa sổ bật lên trên web. Trên web, trang chuyển hướng cũng gửi callback của nó qua một `BroadcastChannel` cùng origin, nên việc đăng nhập hoàn tất khi `Cross-Origin-Opener-Policy` của nhà cung cấp cắt cửa sổ bật lên khỏi trang. Một cửa sổ bật lên như vậy được trang coi là đã đóng, nên việc đóng một cửa sổ bật lên sau khi nó đã hiển thị trang của nhà cung cấp chỉ kết thúc phiên ở trạng thái bị hủy sau 5 phút; hãy cung cấp cho phiên một nút hủy. Ứng dụng giữ giao thức: nó tạo PKCE verifier và state, dựng URL ủy quyền cho redirect mà nó được cung cấp, và kiểm tra callback trước khi đổi code của nó. Hủy không phải là thất bại: `errors.Is(err, core.AuthCancelled)` cho biết người dùng đã đóng phiên, hoặc hàm `cancel` được trả về đã kết thúc nó.

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

Redirect là `com.example.app:/oauth2redirect` khi một scheme được đặt (Windows, Linux và Android cần nó trong `url_schemes` của `nsgo.yaml` để hệ điều hành chuyển callback cho ứng dụng đang chạy; phiên hệ thống của macOS, iOS và watchOS bắt được nó mà không cần khai báo đó), và `http://127.0.0.1:<port>/oauth2redirect` trên loopback của máy tính, vì vậy hãy đăng ký cả hai với nhà cung cấp, thường dưới dạng các client riêng biệt. Để trống `Scheme` để chỉ dùng loopback, hoặc đặt nó thành `https` cùng `Host` cho tên miền liên kết của ứng dụng, mà iOS 17.4, macOS 14.4 và watchOS 10.4 bắt được. watchOS yêu cầu một scheme hoặc tên miền HTTPS liên kết; nó không có phương án dự phòng loopback, và mức tối thiểu watchOS 26 hiện có của NSGo vẫn áp dụng. Một tab trình duyệt bị đóng không báo gì cho một phiên loopback, nên hãy cho nó một `Timeout` hoặc một nút hủy. `Ephemeral` yêu cầu macOS, iOS và watchOS không chia sẻ cookie của trình duyệt, và yêu cầu một Custom Tab riêng tư trên Android ở nơi trình duyệt hỗ trợ; đừng giả định cookie được cô lập trên Android. tvOS và mini program hiện trả lời `core.AuthUnavailable`. Trong TSX, `AuthSession.start(url, options)` cần capability `auth_session` và cung cấp một phiên có `result` resolve với callback, hoặc null khi bị hủy.

Android tiêu thụ một callback khớp trước `OnOpenLinks` thông thường, và việc quay lại mà không có callback sẽ kết thúc phiên ở trạng thái bị hủy. Callback của nó phải dùng một scheme riêng do ứng dụng sở hữu; một scheme rỗng, callback HTTPS hoặc scheme chưa khai báo trả về `core.AuthUnavailable`, cũng như một runtime Android được dựng trước khi có phiên đăng nhập.

Một phiên trình duyệt chỉ tiêu thụ liên kết callback khớp với nó; các liên kết không liên quan và đến muộn vẫn tới được `OnOpenLinks`. Bắt đầu một phiên trình duyệt khác cho cùng URI callback sẽ hủy phiên trước đó; các URI khác nhau có thể chờ trong các cửa sổ khác nhau.

Hãy dùng chính xác redirect được truyền vào hàm URL ủy quyền. Các redirect web và loopback trên máy tính mã hóa các đường dẫn Unicode và phân giải các đoạn dấu chấm trước khi hàm nhận chúng; một `Path` loopback là văn bản đã giải mã, nên một ký tự `%` nguyên văn cũng được thoát.

Mini program dùng `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)` thay vào đó. Hàm gọi lại nhận một `core.MiniProgramCredential` với `Platform` của nó (`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin` hoặc `MiniProgramAlipay`) và một `Code` không rỗng, tồn tại ngắn. Gửi nó tới máy chủ của bạn để đổi code với host; giữ app secret ở đó. Đây là `wx.login`/`qq.login`/`tt.login`, hoặc ủy quyền cơ bản `auth_base` của Alipay, mà không yêu cầu thông tin hồ sơ. Nó không mở URL hay chạy hàm dựng URL OAuth.

`cx.Env().CanSignInMiniProgram()` báo cáo API của host có khả dụng không. Các target khác trả lời `core.MiniProgramSignInUnavailable`; mã không có `core.AccessMiniProgramSignIn` nhận `AccessDenied`. Douyin mặc định hỏi một cách im lặng: một host chưa đăng nhập trả về `MiniProgramSignInNotSignedIn`, và `MiniProgramSignInOptions{Prompt: true}` cho phép cửa sổ đăng nhập tài khoản host của nó. Đóng cửa sổ đó trả về `MiniProgramSignInCancelled`. Một mã thiết bị ẩn danh không bao giờ được tính là người dùng đã đăng nhập. Các bài kiểm thử đặt `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` và `app.MiniProgramSignIn`; `app.MiniProgramSignIns` ghi lại các yêu cầu. TSX không chạy trong mini program.

Một ứng dụng mà các web view của nó cần được đăng nhập như người dùng đã đăng nhập ở nơi khác, chẳng hạn trong trình duyệt của chính họ, sẽ ghi cookie của lần đăng nhập đó bằng `cx.SetWebCookies`, kể cả cookie HttpOnly mà script của trang không đặt được, rồi tải các trang: done chạy khi kho đã giữ chúng. Kho này là kho bền vững mà mọi web view của ứng dụng dùng chung, nên cookie có `Expires` tồn tại lâu hơn ứng dụng; các view Incognito không thấy chúng, trừ trên Android, nơi giữ một kho cho mỗi ứng dụng. HarmonyOS, web, tvOS và watchOS trả lời `Unavailable`.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

Để xóa một cookie, hãy ghi lại nó với cùng tên, tên miền và đường dẫn và `Expires` ở quá khứ; thời hạn bằng 0 tạo ra một cookie phiên. Hãy chờ hàm gọi lại trước khi tải một trang cần thấy việc xóa đó.

Theo chiều ngược lại, `cx.WebCookies` liệt kê các cookie của kho, kể cả cookie HttpOnly cùng tên miền, đường dẫn, thời hạn và các cờ của chúng, những thứ mà `document.cookie` không đọc được, dù có hay không có trang nào của trang web đang hiển thị: một `core.WebCookieFilter` chọn những cookie mà một yêu cầu tới một `URL` sẽ gửi, hoặc những cookie của một `Domain` và mọi host bên dưới nó, còn bộ lọc rỗng thì chọn tất cả. Một ứng dụng gọi trang web từ Go bằng thông tin đăng nhập của người dùng, hoặc hiển thị nơi người dùng đã đăng nhập, sẽ đọc chúng theo cách này. `cx.ClearWebsiteData` đăng xuất người dùng khỏi mọi trang web: nó xóa cookie của kho, local storage và session storage, IndexedDB, service worker và bộ nhớ đệm, và done chạy khi chúng đã bị xóa. `CookieManager` của Android chỉ trả lời tên và giá trị mà một URL sẽ gửi, nên ở đó chỉ bộ lọc `URL` liệt kê được, điền `Domain` bằng host của nó và để trống các trường khác, còn bộ lọc `Domain` hoặc bộ lọc rỗng trả lời `Unavailable`; việc xóa ở đó để lại bộ nhớ đệm HTTP. HarmonyOS, web, tvOS và watchOS trả lời `Unavailable` cho cả hai.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Sign in with Apple

`cx.SignInWithApple` hiển thị sheet Sign in with Apple của hệ thống từ nút riêng của ứng dụng trên macOS, iOS và tvOS, ở nơi `cx.Env().CanSignInWithApple()` là true. Nó trả lời một lần bằng thông tin xác thực, hoặc bằng một lỗi: `core.AppleSignInCancelled` khi người dùng đóng sheet, vốn không phải là thất bại, nếu không thì `AppleSignInFailed` hoặc, trên các nền tảng khác, `AppleSignInUnavailable`. Apple chỉ cung cấp email và tên trong lần đăng nhập đầu tiên của người dùng vào ứng dụng, vì vậy hãy giữ chúng lúc đó. `Nonce` được đưa vào yêu cầu dưới dạng SHA-256 của nó ở hệ hex chữ thường, mà claim `nonce` của identity token chứa: máy chủ băm nonce mà nó đã cấp cho ứng dụng và so sánh. `cx.AppleCredentialState` cho biết người dùng đã thu hồi nó kể từ đó chưa. Trong kiểm thử headless, `app.AppleSignIn` nhận một `nsgotest.AppleSignInRequest` và cung cấp thông tin xác thực hoặc lỗi.

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

Ứng dụng phải được ký với entitlement Sign in with Apple, mà `build` thêm vào khi `nsgo.yaml` yêu cầu; nếu thiếu, sheet sẽ thất bại:

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode đăng ký khả năng này cho các bản dựng thiết bị từ `nsgo dev`; một provisioning profile trong `sign.profile` hoặc `sign.macos_profile` phải đến từ một App ID đã bật Sign in with Apple. macOS chỉ cấp nó kèm một profile, nên một disk image hoặc một lần chạy `nsgo dev` trên máy Mac không thể đăng nhập, và trình giả lập cũng vậy, vì các bản dựng của nó không yêu cầu entitlement nhà phát triển nào.

## Mua hàng trong ứng dụng

`cx.Products`, `cx.Purchase` và các lời gọi bên cạnh chúng bán hàng thông qua App Store trên macOS, iOS và tvOS, ở nơi `cx.Env().CanPurchase()` là true, trong một bản dựng có `nsgo.yaml` yêu cầu StoreKit:

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

Hãy hiển thị giá mà cửa hàng cung cấp, `DisplayPrice`, không bao giờ dùng giá riêng của ứng dụng: cửa hàng tự định giá cho từng storefront. Một giao dịch mua trả lời một lần, bằng giao dịch hoặc một lỗi: `core.PurchaseCancelled` khi người dùng đóng sheet, `core.PurchasePending` trong khi chờ phê duyệt, như với Ask to Buy, và nếu không thì `core.PurchaseFailed`, kèm lời của cửa hàng, hoặc `PurchasesUnavailable` ở nơi khác. Gửi `Signed` của giao dịch, tức JWS của cửa hàng, tới máy chủ của ứng dụng để xác minh, giao những gì đã mua, rồi hoàn tất nó; cửa hàng sẽ trao lại một giao dịch chưa hoàn tất cho ứng dụng ở mỗi lần khởi chạy.

`cx.ManageSubscriptions` mở giao diện quản lý gói đăng ký của Apple (sheet hệ thống trên iOS, trang gói đăng ký của App Store trên macOS và tvOS). `cx.RequestRefund` bắt đầu một yêu cầu hoàn tiền cho một giao dịch và trả lời `core.RefundSuccess` hoặc `core.RefundCancelled`.

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction` trả về `PurchaseNotFound` cho một giao dịch đã hoàn tất, kể cả một giao dịch được trả về bởi `CurrentEntitlements`; một mặt hàng không tiêu hao đã hoàn tất vẫn thuộc quyền sở hữu.

Đăng ký `OnPurchaseUpdate` khi ứng dụng khởi động: các cập nhật đến trước bất kỳ trình xử lý nào sẽ chờ trình xử lý đầu tiên. `cx.CurrentEntitlements` trả lời những gì người dùng hiện sở hữu, để mở khóa chúng khi khởi chạy, và một nút Restore Purchases gọi `cx.SyncPurchases`, có thể yêu cầu người dùng đăng nhập. `AccountToken` là một UUID riêng của ứng dụng mà giao dịch và các thông báo máy chủ của App Store mang theo. macOS chỉ bán trong một bản dựng Mac App Store, và một App Clip không bán gì; Android, watchOS, Windows, Linux, web và mini program hiện trả lời `PurchasesUnavailable`. Kiểm thử mua hàng bằng `nsgotest`, với `Products`, `PurchaseAnswer`, `UpdatePurchase` và `UnfinishedTransactions` đóng vai cửa hàng. `app.PurchaseRequests` ghi lại các thao tác của cửa hàng dưới dạng các giá trị `nsgotest.PurchaseRequest`. Trong TSX, module `Purchases` cần capability `purchases`: `purchase` resolve với `{ status: "purchased", transaction }`, `"pending"` hoặc `"cancelled"`, và `on("update", listener)` nghe các cập nhật.

## Hướng giao diện

Gọi `cx.RequestOrientation(core.OrientationLandscape)` trong scope của một trình phát toàn màn hình để yêu cầu iOS, Android, HarmonyOS hoặc trình duyệt chuyển sang hướng ngang. Yêu cầu mới nhất có scope còn sống sẽ có hiệu lực. Khi scope đó kết thúc, yêu cầu trước đó được khôi phục hoặc ứng dụng quay về chính sách hướng thông thường.

Trên iOS 16 trở lên, yêu cầu có thể xoay giao diện ngay lập tức. Trên các phiên bản iOS cũ hơn, nó hạn chế lần xoay tiếp theo của hệ thống. `IOS.SupportedOrientations` của một modal đang được trình bày được ưu tiên. Trên Android và HarmonyOS, một tổ hợp hướng dọc và ngang mà hệ thống không biểu diễn được sẽ bị rút gọn thành một hướng được phép, ưu tiên hướng đang hiển thị. Android có thể bỏ qua yêu cầu hướng ở chế độ màn hình lớn hoặc đa cửa sổ.

Trong một ứng dụng web, gọi `desktop.ToggleFullScreen(cx)` từ một trình xử lý nhấn mở scope của trình phát. Trình duyệt vào toàn màn hình từ lần nhấn, rồi NSGo áp dụng yêu cầu hướng của trình phát. Thoát toàn màn hình sẽ giải phóng khóa. Bảng điều khiển của trình duyệt báo cáo các thao tác không được hỗ trợ và các lần từ chối.

`nsgotest.App.Orientation` ghi lại mặt nạ được yêu cầu gần nhất, bằng 0 cho chính sách hướng thông thường.

## Đánh giá trên cửa hàng

`cx.RequestReview()` yêu cầu một lời nhắc đánh giá trong ứng dụng từ App Store trên macOS và iOS, hoặc Google Play trên Android. `cx.Env().CanRequestReview()` báo cáo dịch vụ có khả dụng không. Cửa hàng quyết định có hiển thị lời nhắc hay không và khi nào, và không báo cáo kết quả. Dùng một bản dựng phát triển để kiểm thử lời nhắc của Apple. Các bản dựng TestFlight chặn nó.

Các gói Android độc lập bao gồm Play Review SDK, activity hộp thoại của nó và truy vấn hiển thị Play Store; việc dựng chúng không cần Android SDK hay JDK. Một thiết bị không có Play Store được bật báo cáo dịch vụ là không khả dụng. Kiểm thử một gói đã phát hành bằng [Internal App Sharing](https://developer.android.com/guide/playcore/in-app-review/test).

Khi nhúng NSGo, thêm `com.google.android.play:review:2.0.2` vào host Gradle. Trong manifest của host, thêm `<package android:name="com.android.vending" />` dưới `<queries>` để ứng dụng có thể thấy Play Store. Nếu host thu gọn mã, hãy giữ lại các API Play Review và Tasks bằng các quy tắc dưới đây, vì NSGo tải chúng theo tên.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

Trong kiểm thử, đặt `CanRequestReview` trong `nsgotest.Environment` và kiểm tra `nsgotest.App.ReviewRequests`.

## Cử chỉ cạnh hệ thống

Một trò chơi có điều khiển cảm ứng ở mép màn hình có thể gọi `mobile.DeferSystemGestures` trong khi các điều khiển đang hoạt động. Dùng hàm được trả về để giải phóng yêu cầu sớm. Nó cũng kết thúc khi scope của nó được dọn dẹp. Các yêu cầu từ nhiều scope trong cùng một cửa sổ được kết hợp.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS dùng các cạnh màn hình vật lý. Android 10 trở lên dùng các hình chữ nhật tính bằng point của surface đo từ góc trên bên trái. Hãy giữ các hình chữ nhật quanh những điều khiển cần chúng. Android dành riêng cử chỉ về màn hình chính và giới hạn vùng mà ứng dụng có thể loại trừ khỏi cử chỉ Quay lại. `nsgotest.App.SystemGestures` cung cấp cho kiểm thử yêu cầu kết hợp hiện tại trên surface chính của chúng. Dùng `Surface.SystemGestures()` cho một cửa sổ khác.

## Lớp phủ trên Android

`mobile.OpenOverlay` hiển thị một view của ứng dụng phía trên các ứng dụng khác trên Android, một cây riêng dùng chung các signal của ứng dụng, và một dịch vụ tiền cảnh giữ ứng dụng chạy trong khi nó hiển thị. Ứng dụng liệt kê `SYSTEM_ALERT_WINDOW` trong `android.permissions`, và người dùng cấp quyền đó trong cài đặt hệ thống, mà `mobile.RequestOverlayPermission` mở ra; nếu không có quyền, lớp phủ kết thúc ngay và `OnEnd` nghe lý do. Nó nhận các lần chạm nhưng để lại các phím và các lần chạm xung quanh nó cho ứng dụng ở phía trước, trừ khi nó là `Focusable` hoặc `Passive`, và di chuyển bằng `desktop.SetWindowFrame` từ nội dung của nó. Các nền tảng khác không mở lớp phủ nào.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## Vị trí và chuyển động

`cx.Locate` lấy một lần định vị, `cx.WatchLocation` truyền liên tục các lần định vị và `cx.WatchMotion` truyền liên tục các mẫu gia tốc kế, con quay hồi chuyển, từ kế và chuyển động thiết bị tổng hợp. Mỗi hàm trả về một `*core.SensorStream` mà `Stop` của nó hủy luồng, cũng như việc scope đã khởi động nó kết thúc; các hàm gọi lại chạy trên goroutine của engine. Các quyền là `core.PermissionLocation`, `PermissionBackgroundLocation` và `PermissionMotion` ([quyền](permissions.md)): bắt đầu một luồng không bao giờ hỏi quyền, và một luồng không có quyền của nó sẽ thất bại với `core.SensorNotAllowed`.

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate` chờ một lần định vị mới, mặc định 30 giây (`Timeout`); chỉ `MaxAge` mới cho phép nó nhận một lần định vị đã lưu trong bộ nhớ đệm, và lần định vị đó cho biết điều này bằng `Cached` và `Age`. `Approximate` cho biết một lần định vị mà người dùng chỉ cho phép ở mức xấp xỉ; Android làm mới lần định vị như vậy tối đa mười phút một lần, nên hãy cho `Locate` một `MaxAge` cho nó ở đó. Tọa độ theo WGS 84; một phép đo mà nhà cung cấp thiếu có `Valid` là false, và `Altitude` là độ cao ellipsoid trong khi `MSLAltitude` là độ cao trên mực nước biển trung bình. Chuyển động dùng các trục tự nhiên của thiết bị, gia tốc tính bằng m/s² (+9.81 trên trục z khi nằm ngửa), tốc độ quay tính bằng rad/s và từ trường tính bằng µT, mặc định mỗi 20 ms, giới hạn trong khoảng 10 ms–1 s. Khi tải cao, một luồng giữ mẫu mới nhất cho mỗi cảm biến và đếm các mẫu bị thay thế trong `Dropped`.

Các luồng tiếp tục khi ứng dụng không hoạt động và tạm dừng ở nền, đánh dấu mẫu đầu tiên sau đó bằng `Gap`; một `Locate` đang chờ giữ phần thời gian chờ còn lại của nó. `LocationOptions.Background` giữ một luồng vị trí tiếp tục ở nền dưới `PermissionBackgroundLocation`: ứng dụng Apple thêm chế độ nền `location`, và trên Android một dịch vụ tiền cảnh hiển thị thông báo với `NotificationTitle` và `NotificationBody` và giữ ứng dụng chạy sau khi activity của nó kết thúc. Không có gì khởi động lại một tiến trình mà hệ thống đã kết thúc.

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS và watchOS cung cấp vị trí và chuyển động, macOS cung cấp vị trí, tvOS từng lần định vị một, và Android cung cấp cả hai qua các nhà cung cấp riêng của framework. Linux lấy các lần định vị và luồng từ GeoClue 2 trên system bus, mà agent của nó hỏi người dùng khi yêu cầu đầu tiên của ứng dụng bắt đầu; vì GeoClue không thể được hỏi mà không hiện lời nhắc, quyền được đọc là chưa hỏi cho đến khi ứng dụng đã hỏi trong lần chạy đó, và vị trí nền không khả dụng. Windows lấy chúng từ `Geolocator` của WinRT; quyền được đọc là bị từ chối khi vị trí bị tắt cho ứng dụng, là được cấp khi ứng dụng đã hỏi trong lần chạy đó và được cho phép, và nếu không thì là chưa hỏi, và vị trí nền cũng không khả dụng ở đó. Một trình duyệt bảo mật cung cấp các lần định vị địa lý và, trên thiết bị di động, các số đo gia tốc kế, con quay hồi chuyển và chuyển động tổng hợp. Gọi `cx.RequestPermission(core.PermissionMotion, done)` từ một trình xử lý nhấn trên Safari. Trình duyệt báo vị trí không khả dụng khi không có `navigator.geolocation`, chuyển động không khả dụng trên máy tính và cho các luồng từ kế. Trong TSX, `Sensors` có `permission`, `requestPermission`, `locate`, `watchLocation` và `watchMotion`, dưới các capability của gói là `location`, `backgroundLocation` và `motion`, tính bằng mili giây.

## Âm thanh

Gói `audio` phát các âm thanh ngắn được giải mã một lần vào bộ nhớ (`audio.LoadSound`) và âm thanh dài được phát trực tuyến (`audio.NewPlayer`) ở nơi `cx.Env().CanPlayAudio()` là true; ở nơi khác, các lời gọi thất bại với `audio.Unavailable`.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` đặt `Volume` cho mỗi lượt phát (0 đến 1, 0 được coi là 1), `Rate` (0.5 đến 2, 0 được coi là 1) và `Pan` (-1 hoàn toàn bên trái đến 1 hoàn toàn bên phải, 0 ở giữa); các giá trị ngoài phạm vi đó bị kẹp lại và NaN nhận giá trị mặc định. `Sound.Stop` dừng mọi lượt phát của âm thanh và `Close` giải phóng nó, như scope sở hữu của nó làm khi bị hủy. `Player` hỗ trợ `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop` và `Close`. macOS, iOS, Windows, Linux, Android và web hiện phát được âm thanh. Các backend Apple, Windows và trình duyệt hỗ trợ WAV, AAC và MP3. Linux dùng các codec GStreamer, và trình phát trực tuyến của nó phát ở tốc độ 1×. Android tạm dừng một trình phát khi view của nó rời khỏi màn hình và giới hạn mỗi âm thanh đã giải mã ở 1 MiB. tvOS, watchOS, HarmonyOS và mini program vẫn báo `CanPlayAudio` là false.

Trình duyệt yêu cầu một cử chỉ của người dùng để phát có tiếng. Hãy tải trước các âm thanh ngắn, rồi gọi `Sound.Play` sau một lần nhấp, chạm hoặc nhấn phím. Một âm thanh được phát trước cử chỉ sẽ im lặng. Hàm gọi lại `Play` của một trình phát nhận `audio.NotAllowed` nếu trình duyệt chặn việc phát. Hãy thử lại từ một cử chỉ của người dùng.

`Sound.Play` trả về một `audio.Playback`, mà trên macOS, iOS, Windows, Linux và web có thể đặt âm lượng và tốc độ riêng hoặc dừng độc lập. `Ended()` có tính phản ứng và trở thành true khi lượt phát đó kết thúc, bị dừng, bị thay thế do giới hạn số giọng, hoặc không thể bắt đầu. Các backend không có điều khiển riêng lẻ trả về một handle đã kết thúc.

Trên iOS, một audio engine riêng của một thư viện phát bên ngoài `audio` và `Video`, chẳng hạn một `AudioQueue`, giữ phiên âm thanh duy nhất của ứng dụng bằng `audio.HoldSession(core.AudioSessionPlayback)` trong khi nó phát và gọi hàm giải phóng được trả về khi nó dừng. Nó nhận các mức mà `AudioSession` của một trình phát nhận, bên cạnh video, trình phát và bản ghi âm của ứng dụng, nên `AudioSessionPlayback` và `AudioSessionMixWithOthers` tiếp tục phát khi công tắc Ring/Silent ở chế độ im lặng. Ở nơi khác, nó không giữ gì.

Dùng `audio.Voice{Muted: true}` để bắt đầu một hiệu ứng ở trạng thái im lặng và `playback.SetVolume(0)` để làm im lặng một hiệu ứng đang phát. Với âm thanh dài, `audio.NewPlayer` phát trực tuyến một nguồn và `player.SetMuted(true)` làm im lặng đầu ra trong khi việc phát vẫn tiếp tục. Bỏ tắt tiếng sẽ khôi phục âm lượng đã cấu hình của trình phát. `Voice.Volume` và `Player.SetVolume` dùng 0 cho âm lượng tối đa mặc định.

Trong TSX, `nsgo/audio` cung cấp `supported()` và `loadSound(uri)`, được kiểm soát bởi capability `audio` của gói. `loadSound` resolve với một âm thanh mà `play({ volume, rate, pan, loop, muted })` của nó trả về một lượt phát có `setVolume`, `setRate`, `stop` và một `ended` có tính phản ứng; ở đó, âm lượng 0 là im lặng. Một lần tải bị từ chối sẽ reject với một `Error` có `reason` là tên của lỗi âm thanh, chẳng hạn `quota-exceeded`, và có `code` là `ERR_ACCESS_DENIED` khi thiếu capability, `ERR_NSH_QUOTA_EXCEEDED` khi vượt giới hạn và `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM` ở nơi không có gì phát được âm thanh. Một âm thanh chỉ được giải phóng bởi `close()` hoặc khi view của nó biến mất; trong một sandbox, mỗi lần tải được tính vào `Limits.Sounds` (mặc định 256) cho đến khi nó thất bại hoặc âm thanh của nó đóng lại.

## Tay cầm chơi game

Gói `gamepad` đọc các tay cầm chơi game đã kết nối theo bố cục W3C Standard Gamepad (`ButtonSouth`/`ButtonA` đến `ButtonHome`, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` báo cáo target có hỗ trợ tay cầm không (macOS, iOS, Windows, Linux, Android và các trình duyệt có Gamepad API).

`gamepad.Pads(cx)` là một lần đọc phản ứng được theo dõi, kết xuất lại các scope khi tay cầm kết nối hoặc ngắt kết nối mà không kết xuất lại khi nút hoặc cần analog thay đổi. Mỗi `gamepad.Pad` cung cấp `Family` của nó (`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo` hoặc `FamilyGeneric`, cũng được phân loại qua `gamepad.DetectFamily`), `VendorID` và `ProductID` USB ở nơi nền tảng báo cáo chúng, và `CanRumble` cho khả năng hỗ trợ động cơ rung. Bên trong một vòng lặp `RequestAnimationFrame`, `gamepad.State(cx, id)` trả về các nút của tay cầm (`[0, 1]`, với `Pad.Pressed` là true khi `>= 0.5`) và các trục (`[-1, 1]`, âm là trái/lên và dương là phải/xuống) được chụp nhanh một lần mỗi khung hình hiển thị.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` điều khiển các động cơ rung tần số thấp (nặng/trái) và tần số cao (nhẹ/phải) với cường độ được kẹp trong `[0, 1]`, thay thế mọi lần rung đang chạy trên tay cầm đó và tự động dừng sau `duration`, khi ứng dụng chuyển sang nền hoặc mất tiêu điểm (`cx.Env().Lifecycle() != core.LifecycleActive`), khi `cx` bị hủy, hoặc khi tay cầm ngắt kết nối. `gamepad.StopRumble(cx, id)` dừng rung sớm; các tay cầm có `CanRumble == false` bỏ qua các lời gọi rung một cách gọn gàng.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

Gọi `gamepad.EnableFocusNavigation(cx)` để chọn dùng việc điều hướng giữa các điểm dừng tiêu điểm gốc bằng tay cầm (D-pad và cần analog trái di chuyển tiêu điểm, `ButtonSouth` kích hoạt điều khiển đang có tiêu điểm và `ButtonEast` kích hoạt quay lại). Các view gameplay đang hoạt động gọi `gamepad.CaptureInput(cx)` để tạm dừng điều hướng tiêu điểm trong khi chúng được gắn kết. Trong TSX, `nsgo/gamepad` cung cấp `supported()`, `pads()`, `state(id)`, `Button` và `Axis`, được kiểm soát bởi capability `gamepad` của gói.

## Định thời âm thanh

Dùng `audio.Now(cx)` làm đồng hồ cho việc phát có định thời. Đặt `audio.Voice.StartAt` thành một thời điểm trong tương lai khi `audio.CanSchedule(cx)` là true. Gọi `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` cho tuyến đầu ra đang hoạt động. macOS, iOS, Linux và web hỗ trợ bắt đầu có định thời; trên Windows và Android, `CanSchedule` là false và một `StartAt` sẽ phát ngay lập tức. Giá trị `known` của hàm gọi lại cho biết tuyến có báo cáo độ trễ hay không, điều mà Windows và Android không làm.

## Dịch vụ trò chơi

Gói `gameservices` cung cấp một API trung lập với nhà cung cấp cho việc đăng nhập người chơi, thành tích, bảng xếp hạng và các lớp phủ gốc Game Center / Play Games ở nơi `cx.Env().CanUseGameServices()` là true. Mọi hàm gọi lại đều chạy trên goroutine của engine nên có thể đặt signal trực tiếp mà không chặn các khung hình.

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement` kẹp tiến độ trong `0..100` (`100` mở khóa thành tích). `gameservices.LoadScores` đọc các hàng `gameservices.ScoreEntry` đã xếp hạng cho `Global` hoặc `Friends` trong khoảng `AllTime`, `Week` hoặc `Today`, và `gameservices.ShowAchievements` cùng `gameservices.ShowLeaderboard` mở các lớp phủ gốc của nền tảng. macOS, iOS và tvOS truy cập GameKit qua purego; Android sẽ truy cập Play Games Services qua một hook runtime Java chưa có trong runtime, nên hiện nó báo `CanUseGameServices() == false`; Windows, Linux, web, watchOS và mini program báo `CanUseGameServices() == false` và trả lời `gameservices.ErrUnsupported`.

Trong kiểm thử, bật dịch vụ trò chơi bằng `nsgotest.Environment(core.Environment{CanUseGameServices: true})` và điều khiển bản giả lập cô lập của mỗi ứng dụng bằng `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements` và `App.GameServicesShownLeaderboards`.

## Tiếp theo

- [Thông báo](notifications.md): quyền, huy hiệu và các lần nhấn.
- [Biểu mẫu](forms.md): trường văn bản và bàn phím.
- [Nhúng](embedding.md): các dịch vụ mà một ứng dụng host cung cấp cho ứng dụng được nhúng của nó.
- [Dựng và phát hành](building.md): các quyền và tệp mà một gói cần.

### Thể dục, vùng địa lý và chuyển động có giới hạn

`cx.WatchSteps`, `cx.WatchActivity` và `cx.WatchPressure` trả về các luồng có scope. Hãy yêu cầu `core.PermissionActivityRecognition` trước khi dùng bước chân/hoạt động, và trước khi dùng áp suất trên Apple. Apple cung cấp cả ba qua Core Motion; Android cung cấp bộ đếm bước chân và áp suất, và báo cáo việc phân loại hoạt động là không khả dụng. Các số đếm thuộc về cửa sổ `Since` của mẫu; các cửa sổ được tiếp tục mang `Gap`. Áp suất tính bằng pascal, với độ cao tương đối tùy chọn tính bằng mét. Khoảng cách, số tầng và độ cao bị thiếu có `Valid` là false.

`cx.WatchGeofences` nhận `core.GeofenceOptions` với từ một đến hai mươi vòng tròn `GeofenceRegion` riêng biệt (tọa độ WGS 84 và bán kính tính bằng mét). Nó báo cáo `GeofenceInside`, `GeofenceOutside` hoặc `GeofenceUnknown` cho mỗi vùng. `Background` yêu cầu quyền truy cập/khai báo vị trí nền và ủy quyền Always của Apple; Android cũng cần một tiêu đề thông báo. Dừng hoặc hủy sẽ gỡ các đăng ký của nó; không có đăng ký nào tồn tại sau khi tiến trình chết.

`cx.RequestPreciseLocation("route", done)` yêu cầu tường minh ủy quyền vị trí chính xác và trả lời bằng một giá trị boolean, không bao giờ là một vị trí. Trong `apple.info`, khai báo `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`. Android cần cả quyền coarse lẫn fine. Một câu trả lời false có thể vẫn để vị trí xấp xỉ được ủy quyền.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` yêu cầu một luồng trực tiếp hữu hạn. Nó cần `AccessMotion` và `AccessBackgroundMotion`; các gói TSX khai báo `motion` và `backgroundMotion`. iOS có thể khiến hợp đồng thực thi của nó hết hạn sớm hơn. Ứng dụng Android đặt `android.background_motion: true` và khai báo `FOREGROUND_SERVICE`; dịch vụ không cố định (nonsticky) dừng trong vòng ba phút ngay cả khi thời lượng được yêu cầu dài hơn. Hết thời lượng hoặc hệ điều hành thu hồi sẽ báo `SensorTimedOut`. Dừng/hủy sẽ kết thúc nhà cung cấp và hợp đồng thực thi; không có gì được ghi lại sau khi kết thúc. Các target khác báo không khả dụng.

`Sensors` trong TSX phản chiếu sáu phương thức này và dùng mili giây. Kiểm thử headless cấp dữ liệu qua `FeedSteps`, `FeedActivity`, `FeedPressure` và `FeedGeofence`, và đặt `PreciseLocationAnswer` cho các yêu cầu độ chính xác. Hành vi thể dục thực tế, đánh thức theo vùng và chạy nền khi bị tạm ngưng vẫn cần được nghiệm thu trên thiết bị.
