---
title: Tham khảo thành phần
description: Các thành phần dùng nhiều nhất, biến thể, trạng thái và khả năng truy cập của chúng.
group: Thành phần
order: 30
---

Đây là các thành phần của `ui`, mang giao diện của NSGo và tuân theo chủ đề. Phần tham khảo này bắt đầu với những thành phần mà một ứng dụng dùng đến đầu tiên; mỗi trang bắt đầu bằng ví dụ ngắn nhất, sau đó trình bày các bộ phận, trạng thái và khả năng truy cập của thành phần. Bộ sưu tập (gallery) bao gồm mọi thành phần, kể cả những thành phần chưa có trang ở đây.

[Bộ sưu tập](https://nsgo.dev/gallery/) chạy bản demo của mọi thành phần ngay trong trình duyệt.

## Hành động

- [Button](ui/button.md): nút nhấn, dạng thường hoặc dùng màu nhấn (accent), và nút biểu tượng.

## Văn bản và nhập liệu

- [Text](ui/text.md): đoạn văn, span, liên kết, phông chữ và văn bản tài liệu.
- [Input](ui/input.md): trường văn bản của nền tảng, có kiểm soát hoặc không.
- [Checkbox](ui/checkbox.md): hộp kiểm, ở trạng thái đã chọn, chưa chọn hoặc không xác định.
- [Switch](ui/switch.md): một thiết lập bật hoặc tắt.
- [Select](ui/select.md): một giá trị từ danh sách, dưới dạng popup hoặc sheet.
- [Slider](ui/slider.md): một giá trị hoặc một khoảng dọc theo một rãnh trượt.

## Tập hợp

- [List](ui/list.md): số hàng bất kỳ, chỉ được dựng khi hiển thị, có chọn và kích hoạt.
- [Scroll](ui/scroll.md): view cuộn của nền tảng, có phân trang, căn khớp (snapping), thu phóng và làm mới.

## Điều hướng

- [Tabs](ui/tabs.md): danh sách tab, điều khiển phân đoạn (segmented control) và thanh tab của hệ thống.

## Lớp phủ

- [Dialog](ui/dialog.md): bề mặt modal, sheet và hộp cảnh báo của hệ thống.
- [Menu](ui/menu.md): menu mở từ một phần tử kích hoạt, menu ngữ cảnh và các mục của chúng.

## Thêm nữa

Bộ sưu tập bao gồm phần còn lại: bảng, biểu đồ, trình soạn thảo mã, văn bản định dạng, terminal, dock, bảng lệnh (command palette), bộ chọn màu, ngày và emoji (`ui.EmojiPicker`), thông báo nhanh (toast) và nhiều hơn nữa. Mỗi thành phần đều có bản demo ở đó.

`base/terminal.View` mở các đường dẫn cục bộ đang tồn tại bằng Command+click (Control+click trên Windows và Linux), giống như cách nó mở URL. Đường dẫn tương đối dùng thư mục làm việc OSC 7 của shell; một shell cục bộ chưa báo thư mục nào sẽ dùng `terminal.Options.Dir`, hoặc thư mục home khi chưa đặt giá trị này. Hãy đặt tên tệp chứa dấu cách trong dấu ngoặc kép. Các authority thư mục từ xa và các kết nối không có báo cáo thư mục cục bộ sẽ không được phân giải theo tệp cục bộ. Tệp và thư mục dùng `Scope.OpenFile`; liên kết URL tiếp tục đi qua `Scope.OpenURL` và trình xử lý URL của ứng dụng chủ nhúng.

`base/terminal.View` mặc định từ chối việc chương trình đọc và ghi bảng nhớ tạm. Chỉ đặt `AllowClipboardRead` và `AllowClipboardWrite` khi ứng dụng cho phép; `AccessClipboard` của view yêu cầu vẫn được áp dụng. Các prop tương tự cũng có trên `Terminal` của TSX và có thể thay đổi mà không cần khởi động lại phiên của nó. Ứng dụng tự cung cấp giao diện xin sự đồng ý. Thao tác dán chủ động của người dùng cung cấp riêng một bản chụp văn bản/PNG cho các chương trình dùng giao thức bảng nhớ tạm của Kitty (mode 5522); mật khẩu dùng một lần của nó chỉ đọc được bản chụp đó. Các chương trình không dùng mode này nhận văn bản bracketed thông thường. Khả năng có ảnh trong bảng nhớ tạm tùy thuộc vào dịch vụ bảng nhớ tạm của nền tảng; vùng chọn chính (primary selection) của X11 không được cung cấp.

## Tiếp theo

- [Thành phần và trạng thái](components.md): cách một thành phần trả về các phần tử và cách signal cập nhật chúng.
- [Kiểu dáng và chủ đề](styles.md): chủ đề mà các thành phần lấy màu từ đó.
- [Thành phần không kiểu dáng](authoring/base.md): cùng các điều khiển đó nhưng chưa có giao diện, để bạn tự tạo giao diện riêng.
- [Khả năng truy cập](accessibility.md): nhãn, vai trò và giá trị cho mọi điều khiển.
