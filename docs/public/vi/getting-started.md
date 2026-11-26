---
title: Cài đặt
description: Cài đặt những gì NSGo cần, tạo một ứng dụng và chạy nó trên mọi nền tảng đích.
group: Bắt đầu
order: 10
---

## Yêu cầu

- [Go](https://go.dev/dl/) 1.27.3 trở lên.
- Để build cho iOS, tvOS và watchOS, cần một máy Mac có Xcode. Mọi nền tảng đích khác đều build được từ macOS chỉ với Go; riêng trình giả lập Android x86_64 cần thêm Android NDK.
- Để chạy trên iPhone, cần một Apple ID trong mục Accounts của Xcode và iPhone đã được tin cậy, với Developer Mode được bật.
- Để chạy trên Android, cần `adb` từ platform-tools của Android SDK.
- Để chạy trên Windows, chỉ cần Go, và Go cũng build được các gói cho Windows và web.
- Trên Linux, cần GTK 4.14 trở lên, Pango và Cairo. Ảnh từ mạng dùng libsoup 3, web view dùng WebKitGTK 6.0, còn âm thanh và video dùng các plug-in GStreamer. Trên các hệ thống có giới hạn user namespace của AppArmor, [WebKitGTK cần một profile bubblewrap](debugging.md#web-view-trên-linux).

## Tạo một ứng dụng

Tạo một module, thêm NSGo và công cụ của nó, rồi để công cụ viết ra một ứng dụng:

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` thêm `nsgo` vào module một lần, nhờ đó `go tool nsgo` chạy được nó mà không cần cài đặt gì. `init` tạo ra:

| Tệp | Nội dung |
| --- | --- |
| `app.go` | Ứng dụng, một bộ đếm: một thành phần `Root` và `main`, hàm gọi `nsgo.Run(Root)` |
| `app_test.go` | Một bài kiểm thử nhấn vào nút |
| `nsgo.yaml` | Tên, id, phiên bản và các nền tảng đích của ứng dụng |

`go tool nsgo init -template tsx` tạo một ứng dụng TSX thay thế, còn `-template mixed` tạo một ứng dụng Go mà điểm vào TSX của nó dùng các thành phần Go của chính ứng dụng.

Thêm thư viện bằng `nsgo add <module>` thay cho `go get`: nó cho thấy `go.mod` và `go.sum` sẽ nhận gì trước khi ghi, và không bao giờ đưa NSGo của dự án ra khỏi các bản phát hành ([Thêm thư viện](building.md#thêm-thư-viện)).

### Mẫu từ module Go

Truyền đường dẫn module của mẫu kèm phiên bản tùy chọn. Nếu bỏ qua phiên bản, nó sẽ được phân giải thành phiên bản mới nhất mà module proxy của Go tìm thấy.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

Xuất bản bộ khởi đầu bên trong thư mục `template/` của module. Bao gồm `nsgo.yaml` và `main.go`. Công cụ render cả hai tệp này, cùng mọi tệp có đuôi `.tmpl`, bằng `text/template` của Go, và bỏ `.tmpl` khỏi tên tệp đầu ra. Các tệp khác, kể cả tài nguyên nhị phân, được sao chép nguyên vẹn.

Mẫu nhận `.Name` (tên hiển thị của ứng dụng), `.ID` (ID ứng dụng được sinh ra), `.Slug` (tên viết thường, nối bằng dấu gạch ngang) và `.Module` (giá trị của `-module`, hoặc slug). Hãy kèm theo một `go.mod.tmpl` để khai báo module và các phụ thuộc của ứng dụng được sinh ra, vì Go loại trừ các tệp `go.mod` lồng nhau khỏi bản tải module. Nếu không có tệp này, `-module` sẽ viết ra cùng một `go.mod` tối giản như với mẫu dựng sẵn.

## Chạy ứng dụng

```sh
go tool nsgo dev
```

`dev` build ứng dụng cho máy này và mở nó. Mỗi lần lưu, nó build lại và khởi động lại ứng dụng, còn thay đổi chỉ ở TSX sẽ cập nhật ứng dụng đang chạy tại chỗ. Một lần build thất bại sẽ hiển thị đè lên ứng dụng, kèm các nút để sao chép lỗi và build lại.

Chạy ở nơi khác bằng `-target`:

```sh
go tool nsgo dev -target web                # in the browser
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator
```

`go tool nsgo doctor` liệt kê mọi nền tảng đích, cho biết máy này có build và chạy được nền tảng đó hay không, và còn thiếu gì với những nền tảng chưa làm được, chẳng hạn Xcode, `adb`, một thiết bị đã kết nối hoặc GTK.

[Chạy và gỡ lỗi](debugging.md) trình bày về thiết bị, theo dõi cập nhật, trình giám sát hiệu năng và các ngôn ngữ khác.

## Với một coding agent

Skill của NSGo dạy các coding agent, như Claude Code, Codex và Cursor, cách viết, kiểm thử và phát hành một ứng dụng NSGo. Thêm nó vào thư mục của ứng dụng, hoặc dùng `-g` cho mọi dự án:

```sh
bunx skills add nsgo-dev/nsgo
```

Các agent tự đọc tài liệu sẽ bắt đầu từ <https://nsgo.dev/llms.txt>, nơi liệt kê mọi trang dưới dạng Markdown tại `https://nsgo.dev/docs/<page>.md`; <https://nsgo.dev/llms-full.txt> chứa tất cả các trang. Agent cũng có thể sử dụng ứng dụng đang chạy ([Chạy và gỡ lỗi](debugging.md#agent-tự-động-hóa)).

## Tiếp theo

- [Ví dụ: Ứng dụng việc cần làm](tutorials/todo.md): trạng thái, một danh sách và một bài kiểm thử, trong khoảng mười phút.
- [Ví dụ: Ứng dụng ghi chú](tutorials/notes.md): một ngăn xếp điều hướng và các ghi chú được lưu trên thiết bị.
- [Ví dụ: Các kho GitHub phổ biến](tutorials/repos.md): tải dữ liệu từ mạng, kèm xử lý lỗi và thử lại.
