---
title: Widget
description: Hiển thị các view của ứng dụng trên Màn hình chính và Màn hình khóa của iPhone và iPad, và trên màn hình nền của máy Mac.
group: Tính năng nền tảng
order: 47
---

## Một loại widget

Widget hiển thị nhanh thông tin của ứng dụng dù ứng dụng có đang chạy hay không: trên Màn hình chính và Màn hình khóa của iPhone và iPad, trên màn hình nền và trong Trung tâm thông báo của máy Mac, và trong trình khởi chạy của Android. Một loại (kind) là một widget mà thư viện widget của hệ thống cung cấp, cùng với các họ kích thước mà nó có:

```yaml
widgets:
  - kind: counter
    name: Counter                 # shown in the system's gallery of widgets
    description: Shows the count.
    families: [small, medium, circular]  # also large, extra_large, rectangular and inline
# apple: { app_group: group.dev.example.hello }  # the shared App Group, group.<id> by default
```

Trên các nền tảng Apple, một widget extension vẽ widget, và `build` cùng `dev` biên dịch extension đó bằng trình biên dịch Swift của Xcode ([Xây dựng và đóng gói phát hành](building.md#widgets-and-live-activities)). Trên Android, runtime Java dựng sẵn vẽ cùng một timeline dưới dạng App Widget. Cả hai đều tiếp tục hiển thị nội dung đã xuất bản khi ứng dụng dừng.

Với bản build Mac App Store có sandbox, đặt `apple.app_group` và cung cấp các provisioning profile macOS qua `sign.macos_profile` và `sign.widgets_profile`. Cả ứng dụng và `<id>.widgets` phải thuộc cùng một team và được cấp quyền dùng nhóm đó. Timeline và hình ảnh nằm trong vùng chứa dùng chung của nhóm; các bản build macOS phân phối trực tiếp và bản phát triển vẫn giữ vị trí trong thư mục home như trước. [Ký cho cửa hàng](building.md#store-packages) mô tả các tùy chọn build.

`name` và `description` có thể đặt tên cho các thông điệp trong catalog của ứng dụng. Khi đó, thư viện widget và phần giữ chỗ dùng ngôn ngữ của người dùng, và quay về văn bản của catalog nguồn khi thiếu bản dịch. Một giá trị không phải là khóa catalog sẽ được hiển thị nguyên văn.

## Một timeline

`widget.Update` ghi những gì các widget của một loại hiển thị từ giờ trở đi, dưới dạng một timeline gồm các mục: hệ thống hiển thị mục có ngày đã qua gần nhất, rồi lần lượt từng mục sau đó vào ngày của nó, mà không cần ứng dụng. Một `Date` bằng giá trị zero nghĩa là bây giờ. Khi không có mục nào, các widget hiển thị tên và mô tả của loại. Trên các nền tảng Apple, một ứng dụng ở tiền cảnh có thể cập nhật widget thường xuyên tùy ý, còn ứng dụng ở nền thì vài chục lần mỗi ngày.

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` hiển thị cho một họ kích thước một view riêng thay cho `Entry.View`, `Entry.Background` lấp đầy phía sau widget, vượt qua các lề mà hệ thống giữ, và `Entry.URL` được mở cùng ứng dụng khi widget được chạm. `widget.Update` báo `widget.ErrNoWidgets` trong ứng dụng được build không có widget, `widget.ErrUnsupported` ở nơi khác ngoài macOS, iOS và Android, và `widget.Available` cho biết trường hợp nào trước khi ghi. Nó có thể chạy trên bất kỳ goroutine nào, kể cả trước `nsgo.Run`.

## View

Các view của widget không phải là phần tử của core: chúng bố cục như SwiftUI, một view có kích thước bằng nội dung của nó trừ khi `Style` định kích thước hoặc cho nó lấp đầy, và một vùng chứa căn giữa những gì nó chứa trừ khi `AlignItems` và `Justify` chỉ định khác. `Text`, `Symbol` và `Image`, từ dữ liệu byte hoặc từ một tệp mà `Update` đọc, hiển thị nội dung; `Column`, `Row`, `Stack`, `Spacer` và `Divider` sắp xếp nội dung; `Rectangle`, `Progress` và `Gauge` vẽ; còn `Date` hiển thị thời gian theo các định dạng của hệ thống, trong đó `Relative`, `Offset` và `Timer` tự tiếp tục đếm mà không cần một mục cho mỗi giây.

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` nhận khoảng cách, phần đệm, kích thước hoặc lấp đầy, căn chỉnh, nền với góc bo tròn, các kiểu văn bản của hệ thống hoặc cỡ chữ, độ đậm, thiết kế và biến thể phông, giới hạn số dòng, một `Link` mở cùng ứng dụng, và `Accented`, đưa một view vào nhóm các view mà hệ thống tô màu. `widget.Primary`, `Secondary` và `Accent` bám theo giao diện sáng/tối, còn `RGB` và `RGBA` nhận một màu khác trong chế độ tối bằng `Dark`.

## Nút

`widget.Button` chạy `widget.OnAction` từ iOS 17 và macOS 14, mà không mở ứng dụng. Các hệ thống cũ hơn chỉ hiển thị các phần tử con của nó. Đăng ký trình xử lý trong `main`, trước `nsgo.Run`: hệ thống có thể khởi động ứng dụng ở nền mà không có các view của nó. Một thao tác nhấn widget có `Activity == nil`; `Action` của nó là giá trị của nút.

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

Xử lý hành động và gọi `widget.Update` trước khi trình xử lý trả về, để việc tự động tải lại timeline của hệ thống thấy được kết quả. Trình xử lý chạy bên ngoài goroutine của các view; dùng `core.Scope.Post` để thay đổi view. Cơ chế này dùng cùng trình xử lý và intent như [Live Activities](live-activities.md), và chỉ cần `widgets` trong `nsgo.yaml`, không cần `live_activities`.

## Android

Thêm widget từ trình chọn widget của trình khởi chạy và thay đổi kích thước ở đó. Trình kết xuất chọn một họ kích thước được phép dựa trên giới hạn của widget: small khi dưới 240 point ở cả hai chiều, medium từ 240 theo chiều ngang, large từ 240 theo chiều dọc, và extra-large từ 500 theo chiều ngang; nếu họ đó không được liệt kê, nó dùng họ đầu tiên trong danh sách. `Entry.URL` và `Style.Link` mở ứng dụng qua một scheme mà ứng dụng khai báo trong `url_schemes`. Một liên kết trên view lồng bên trong được ưu tiên hơn liên kết của vùng chứa nó.

Các mục timeline trong tương lai dùng báo thức không chính xác (inexact alarm) của Android, mà hệ thống có thể trì hoãn khi ứng dụng ở nền. Các view hẹn giờ dùng chronometer gốc và tự chạy mà không cần ứng dụng; hiện chúng tôn trọng kích thước và màu, còn độ đậm phông, thiết kế và kiểu nghiêng tùy chỉnh vẫn là mục tiêu. Ngày tương đối được làm mới theo lịch của hệ thống. Trình kết xuất chung hỗ trợ văn bản, hình ảnh, bố cục, hình dạng, thanh, vòng và một tập ký hiệu thông dụng; hãy dùng `Image` cho hình minh họa phải trông giống nhau trên cả hai nền tảng. Tên SF Symbol tùy ý vẫn là một mục tiêu còn mở trên Android. Các widget provider của Android được tạo cho APK và AAB độc lập; các trình chủ nhúng không nhận được provider nào.

## Live Activities

Cùng một extension vẽ các Live Activities của ứng dụng trên Màn hình khóa và trong Dynamic Island ([Live Activities](live-activities.md)).

## Tiếp theo

- [Live Activities](live-activities.md): những gì ứng dụng theo dõi khi nó đang diễn ra.
- [Thông báo](notifications.md): thông báo và huy hiệu bên cạnh widget.
- [Xây dựng và đóng gói phát hành](building.md): nhóm ứng dụng mà extension dùng chung.
