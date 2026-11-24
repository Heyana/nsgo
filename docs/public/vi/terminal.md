---
title: Terminal
description: Chạy một chương trình hoặc hiển thị một phiên từ xa trong lưới terminal.
group: Khái niệm cốt lõi
order: 63
---

`base/terminal` cung cấp một phiên (`Terminal`) và `View` của nó. Máy tính để bàn chạy các chương trình cục bộ; iOS và trình duyệt dùng `Options.Conn`, một `io.ReadWriteCloser` được kết nối với một chương trình từ xa. Ứng dụng sở hữu phiên và đóng nó khi hoàn tất.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` chọn kiểu con trỏ mặc định là khối, thanh dọc hoặc gạch dưới. `CursorBlink` bật nhấp nháy mặc định của nó; giá trị không giữ con trỏ đứng yên. Các chương trình ghi đè hình dạng và nhấp nháy thông qua DECSCUSR hoặc DEC mode 12. Con trỏ khi không có tiêu điểm sẽ rỗng và đứng yên; khi đang soạn thảo bằng bộ gõ (composition) và khi giảm chuyển động, con trỏ cũng đứng yên. Đồng hồ hoạt ảnh của nền tảng điều khiển việc nhấp nháy mà engine không cần vẽ lại văn bản của terminal.

`View.Font`, `Theme` và `DarkTheme` đặt phông chữ và bảng màu. Lưới tự vẽ các ký tự vẽ hộp và ký tự khối Unicode cùng các dấu phân cách Powerline U+E0B0–U+E0B7, căn các ô liền kề theo pixel thiết bị. Văn bản khác dùng phông chữ của nền tảng và các phông dự phòng của chúng.

`MinimumContrast` nâng độ tương phản của văn bản thông thường so với nền thực tế của ô, bao gồm màu đảo ngược, văn bản mờ và vùng chọn. Các giá trị 1–21 dùng tỷ lệ tương phản của WCAG; giá trị không giữ nguyên màu. Đồ họa và emoji giữ nguyên màu của chúng. Với `Transparent`, độ tương phản vẫn dùng nền đã cấu hình của terminal.

`Padding` chừa khoảng trống bên ngoài lưới, tính bằng điểm. `PaddingColor` chọn nền của khoảng trống đó:

| Giá trị | Hành vi |
| --- | --- |
| `PaddingBackground` | Nền terminal, bao gồm các thay đổi qua OSC 11 |
| `PaddingExtend` | Nền của ô gần nhất; trên màn hình chính, không mở rộng ở cạnh trên/dưới bên cạnh các hàng dấu nhắc, hàng Powerline hoặc hàng chứa một ô có nền mặc định |
| `PaddingExtendAlways` | Mở rộng mọi cạnh, không có các ngoại lệ trên |

Ứng dụng TSX import `tsx/terminal` để nhận tác dụng phụ đăng ký `Terminal` trong `nsgo/base`. Các prop là `cursorBlink`, `minimumContrast` và `paddingColor`; prop cuối nhận `background`, `extend` và `extend-always`. Các prop về giao diện có thể thay đổi trong khi phiên vẫn tiếp tục.

## Trong trình duyệt

Ứng dụng nhúng mô-đun WebAssembly libghostty-vt đã ghim phiên bản và chạy cùng view terminal và luồng nhập liệu. Việc build ứng dụng không cần Zig, không cần cài thư viện gốc hay tải xuống lúc chạy.

Chuyển WebSocket của trình duyệt thành `io.ReadWriteCloser`: gửi byte nhị phân và nối hoặc tách các thông điệp đến thành một luồng. Ứng dụng và máy chủ của nó chịu trách nhiệm xác thực, kết nối lại và mọi thông điệp điều khiển/thay đổi kích thước; NSGo tiêu thụ các byte của terminal. [Fixture kiểm thử chấp nhận trên trình duyệt](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go) cho thấy một bộ chuyển đổi luồng byte hoàn chỉnh có xử lý đóng kết nối.

## Hành động và thu phóng

`View.Keybinds` ghi đè các mặc định bằng `Keybind{Key, Action, Amount}`. Các phím dùng cùng tên như `Scope.OnKey`, chẳng hạn `mod+=` hoặc `ctrl+shift+c`. Các mục sau được ưu tiên. Các phím tắt chạy khi ô nhập của terminal này có tiêu điểm và không có bộ gõ nào đang soạn thảo.

Các hành động là `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize` và `Ignore`. Các hành động về cửa sổ và thẻ thuộc về bảng phím tắt của ứng dụng. `Ignore` tiêu thụ một phím mà không gửi nó tới chương trình.

Theo mặc định, `mod+=` và `mod+shift+=` tăng phông chữ thêm một điểm, `mod+-` giảm nó, và `mod+0` đặt lại về `View.Font.Size` (13 khi chưa đặt). `Amount` đặt bước tăng/giảm, là một khi bằng không, hoặc kích thước cho `SetFontSize`. Kích thước được giới hạn trong 1–255 điểm; các yêu cầu không hữu hạn bị bỏ qua. Thu phóng giữ nguyên phiên và view bên ngoài, cập nhật số hàng và cột của lưới, và thay đổi kích thước của chương trình.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` được theo dõi cho các điều khiển hiển thị nó; `Ref.SetFontSize(size)` và `Ref.Perform(action, amount)` chạy trên engine của ứng dụng, giống như các trình xử lý sự kiện. `Perform` báo cáo một hành động có khả dụng hay không. Một ref chưa được gắn trả về kích thước bằng không và từ chối các hành động.

## Menu ngữ cảnh

Một cú nhấp phụ mở một `base.ContextMenu` chưa có kiểu dáng với các mục Sao chép, Dán, Chọn tất cả và Xóa đã được bản địa hóa. Sao chép yêu cầu có vùng chọn. Xóa loại bỏ nội dung phía trên dấu nhắc và bị vô hiệu hóa trên màn hình thay thế. Màn hình cảm ứng dùng cách hiển thị bằng nhấn giữ của menu ngữ cảnh. Tạo kiểu cho bề mặt bằng `MenuClass`/`MenuStyle` và cho các mục bằng `MenuItemClass`/`MenuItemStyle`.

Đặt `RightClickAction` thành `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste` (sao chép vùng chọn, nếu không thì dán), hoặc `RightClickIgnore`. Ignore để lại sự kiện cho một menu ứng dụng bao quanh; hãy dùng nó khi cung cấp một `ui.ContextMenu` có kiểu dáng gọi các hành động của ref.

Import `tsx/terminal` để đăng ký `<Terminal>` trong `nsgo/base`. Các prop phản ứng `keybinds` và `rightClickAction` của nó cập nhật view trong khi vẫn giữ phiên. Giá trị hành động dùng tên của Ghostty như `increase_font_size` và `copy_to_clipboard`; các giá trị nhấp chuột phải là `context-menu`, `paste`, `copy`, `copy-or-paste` và `ignore`. Các prop tạo kiểu cho menu dùng tên Go ở dạng lower camel case.

## Báo cáo chuột

Các chương trình bật theo dõi chuột sẽ nhận các thao tác nhấn, nhả, kéo và di chuột được yêu cầu, do libghostty-vt mã hóa theo giao thức mà chúng đã chọn. Shift bỏ qua việc bắt nhấp và kéo để dùng cho vùng chọn. `View.MouseReporting` là một `func() bool` phản ứng tùy chọn, mặc định là true khi nil; trả về false để giữ việc chọn bằng chuột và cuộn lịch sử gốc của nền tảng ngay cả khi một chương trình yêu cầu bắt chuột. Cú nhấp chuột phải của chương trình được ưu tiên hơn một menu ngữ cảnh bao quanh khi báo cáo đang được bật.

Trên màn hình thay thế, khi chế độ cuộn thay thế được bật và không có yêu cầu theo dõi chuột, thao tác cuộn bánh xe theo chiều dọc trở thành phím con trỏ, theo chế độ phím con trỏ thường hoặc chế độ ứng dụng. `MouseScrollMultiplier{Precision: 1, Discrete: 3}` đặt số lần báo cáo mặc định cho bàn di chuột và bánh xe cuộn; giá trị không dùng các mặc định đó, còn giá trị dương được giới hạn trong 0.01–10,000. Đầu vào lẻ được cộng dồn cho đến khi đủ một bước hoàn chỉnh. Lịch sử cuộn thông thường giữ cách cuộn và quán tính gốc của nền tảng.

Import `tsx/terminal` để đăng ký `<Terminal>` trong `nsgo/base`. Prop `mouseReporting` của nó mặc định là true; `mouseScrollPrecision` và `mouseScrollDiscrete` đặt các hệ số nhân.

