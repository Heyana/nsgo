---
title: So sánh với các framework khác
description: Vị trí của NSGo bên cạnh NSH, MyGo và các UI framework khác, cùng những đánh đổi của nó.
order: 2
---

Vị trí của NSGo giữa các UI framework bắt nguồn từ ba lựa chọn: ứng dụng được viết bằng Go (hoặc TSX ở tầng trên), mỗi nền tảng vẽ giao diện bằng các view gốc của chính nó, và một codebase duy nhất bao phủ điện thoại, máy tính để bàn và web. Các framework khác đưa ra những lựa chọn khác nhau, và mỗi lựa chọn đều có cái giá của nó.

## Tổng quan

| Framework | Ngôn ngữ | Vẽ bằng | Nền tảng đích |
| --- | --- | --- | --- |
| NSGo | Go, TSX | View gốc của từng nền tảng | iOS, App Clips, tvOS, watchOS, Android, macOS, Windows, Linux, web; mini program ở mức thử nghiệm |
| [NSH](https://nsh.dev) | Rust, TSX | View gốc của từng nền tảng | iOS, Android, macOS, Windows, Linux, web, ứng dụng GPUI, terminal; HarmonyOS và WeChat mini program thông qua các nền tảng cộng đồng |
| [MyGo](https://github.com/egoist/mygo) | Go, trang web | Trình kết xuất GPU riêng hoặc webview hệ thống | macOS, Windows, Linux, web |
| [Fyne](https://fyne.io), [Gio](https://gioui.org) | Go | Trình kết xuất riêng | Máy tính để bàn, điện thoại, web |
| [Wails](https://wails.io) | Go, trang web | Webview hệ thống | Máy tính để bàn |
| [Flutter](https://flutter.dev) | Dart | Trình kết xuất riêng | Điện thoại, máy tính để bàn, web |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | Trình kết xuất riêng; Compose gốc trên Android | Điện thoại, máy tính để bàn, web |
| [React Native](https://reactnative.dev) | JavaScript, TSX | View gốc của từng nền tảng | iOS, Android; các nền tảng khác thông qua nền tảng cộng đồng |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | Điều khiển gốc của từng nền tảng | iOS, Android, macOS, Windows |
| [Avalonia](https://avaloniaui.net) | C# | Trình kết xuất riêng | Máy tính để bàn, điện thoại, web |
| [Qt](https://www.qt.io) | C++, QML | Trình kết xuất riêng | Máy tính để bàn, điện thoại, thiết bị nhúng |
| [Electron](https://www.electronjs.org), [Tauri](https://tauri.app) | Trang web; Rust cho Tauri | Chromium đi kèm; webview hệ thống cho Tauri | Máy tính để bàn; thêm điện thoại cho Tauri |

## Điểm mạnh

- **View gốc (Native views).** Văn bản, bộ gõ, chọn văn bản, cuộn trang, khả năng truy cập và trình đơn đều là thành phần gốc của nền tảng, nên ứng dụng hoạt động đúng như kỳ vọng của người dùng ở đó — trong khi các framework tự vẽ lại phải tự xây dựng từng thứ một. Những gì chỉ một nền tảng có, chẳng hạn Liquid Glass trên iOS, các nền tảng khác sẽ mô phỏng theo cùng một cách thay vì bỏ qua.
- **Cập nhật chi tiết theo hạt mịn.** Thành phần chỉ chạy một lần và các signal chỉ cập nhật đúng phần nào đọc chúng, không cần virtual DOM hay hàm view chạy lại mỗi khung hình để so sánh khác biệt (diff); hoạt ảnh chạy theo đồng hồ khung hình của nền tảng.
- **Gói cài đặt nhỏ nhất và khởi động nhanh nhất.** Trong các [bài đo hiệu năng](https://nsgo.dev/benchmarks/), cùng một ứng dụng đếm số đóng gói ra khoảng 8 MB trên macOS, Windows và Linux, nhỏ hơn MyGo, GPUI và NSH (33 đến 45 MB đối với NSH); ứng dụng cũng khởi động nhanh nhất và tốn ít bộ nhớ hơn các framework tự vẽ bằng GPU như MyGo và GPUI.
- **Thuần Go.** Engine, bố cục và các backend đều viết bằng Go, không có C, không cgo và không cần engine JavaScript đối với ứng dụng Go.
- **Build mọi nền tảng từ một máy Mac.** Một máy Mac có sẵn Go và Xcode có thể biên dịch chéo cho mọi nền tảng đích: không cần NDK, Android SDK, JDK, GTK hay chuỗi công cụ Windows.
- **Tạo mọi tệp phát hành chỉ với một lệnh.** `nsgo build` xuất ra tệp ảnh đĩa đã ký và công chứng (notarized) cho macOS, trình cài đặt đã ký cho Windows, gói deb, Arch và AppImage, gói cho Mac App Store, Microsoft Store, Google Play và Flathub, cùng các script cài đặt từ bản phát hành GitHub ([đóng gói và phát hành](building.md)).
- **Cơ chế cập nhật riêng của từng nền tảng.** Trên macOS có Sparkle và trên Windows có WinSparkle để tải về, xác minh và cài đặt phiên bản tiếp theo; trên Linux và Android ứng dụng tìm thấy bản mới và mở trang phát hành; trên iOS thì App Store đảm nhận. Ứng dụng được cài từ kho ứng dụng sẽ do chính kho đó cập nhật ([cập nhật](building.md#updates)).
- **Mọi nền tảng.** Điện thoại, máy tính bảng, TV, đồng hồ, máy tính để bàn và web dùng chung một engine và một API, cùng với iOS App Clips, widget trên iPhone và Mac, và ở mức thử nghiệm là các mini program WeChat, QQ, Alipay và Douyin.
- **Nhúng vào ứng dụng có sẵn.** Một ứng dụng NSGo có thể đặt như một view trong ứng dụng iOS, Android, Go desktop hoặc web hiện có, giúp chuyển đổi dần từng màn hình một ([nhúng ứng dụng](embedding.md)).
- **Vòng lặp phát triển nhanh.** Lưu một tệp Go sẽ tự động build lại và khởi động lại ứng dụng; thay đổi TSX cập nhật ngay vào ứng dụng đang chạy mà vẫn giữ nguyên trạng thái; lỗi build và panic khi kết xuất hiển thị trực tiếp lên trên ứng dụng ([chạy và gỡ lỗi](debugging.md)).
- **Một bộ kiểm thử dùng cho mọi nơi.** Các bài kiểm thử tìm và thao tác với điều khiển theo văn bản hoặc vai trò như người dùng thật, chạy nhanh không cần cửa sổ, hoặc giữ nguyên không đổi để chạy trong cửa sổ thật trên từng nền tảng đích với đầu vào gốc của nền tảng đó và lưu ảnh chụp màn hình ([kiểm thử](testing.md)).
- **Hỗ trợ bản địa hóa tích hợp sẵn.** Từ ngữ của ứng dụng và các thành phần nằm trong các danh mục (catalog), và các ngôn ngữ giả (pseudo-locale) giúp phát hiện nội dung chưa dịch, bị cắt chữ hoặc chưa sẵn sàng cho bố cục phải-sang-trái ([bản địa hóa](localization.md)).
- **Viết shader bằng Go.** `core.Shader` chạy các GPU shader được viết bằng cú pháp Go hoặc WGSL trên mọi nền tảng ([canvas](canvas.md#shaders)).
- **Dùng TSX khi muốn.** Các màn hình có thể viết bằng TSX cùng các lớp Tailwind, chạy trên cùng một engine với các thành phần Go.

## Điểm yếu

- **Còn non trẻ.** 0.1.0 là bản phát hành đầu tiên và API chưa ổn định; [lộ trình phát triển](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) liệt kê những gì còn thiếu, chẳng hạn như giới hạn heap cho TSX được tải lúc chạy.
- **Hệ sinh thái nhỏ.** Chưa có nhiều thành phần bên thứ ba, ví dụ hay câu trả lời trên mạng so với Flutter, React Native hay Qt.
- **Khác biệt giữa các nền tảng.** Dùng view gốc đồng nghĩa với việc có những khác biệt nhỏ giữa các nền tảng, nên ứng dụng cần được kiểm tra trên từng nền tảng. Một framework tự vẽ giao diện sẽ trông giống hệt nhau ở mọi nơi.
- **Cầu nối riêng.** Trên Android, chương trình Go chạy song song với Java runtime và giao tiếp qua pipe, còn trên Linux người dùng cần GTK 4.14 trở lên.
- **TSX chạy qua trình thông dịch.** TSX chạy trong goja — một trình thông dịch JavaScript viết bằng Go — nên chậm hơn các engine mà React Native hay Electron sử dụng.

## NSH

NSGo chuyển thể thiết kế của NSH: kiến trúc, các backend, tên gọi API và các thành phần chưa tạo kiểu trong `base` đều đến từ NSH ([trạng thái](index.md#status)).

- **Chọn NSGo** để viết ứng dụng bằng Go thay vì Rust hoặc TSX, để build mọi nền tảng đích từ một máy Mac với Go và Xcode, hoặc để tận dụng những gì NSGo bổ sung thêm: GPU shader viết bằng Go, biểu tượng vector, terminal, khả năng ứng dụng tự mở cửa sổ riêng, panel và biểu tượng khay hệ thống, cùng các thành phần đã tạo kiểu trong `ui`.
- **Chọn NSH** nếu bạn muốn một dự án trưởng thành hơn, TSX chạy trên QuickJS có bytecode và giới hạn heap, backend GPUI, hoặc hỗ trợ Android 32-bit.

## MyGo

Cả hai đều xây dựng giao diện bằng Go không dùng cgo, nhưng MyGo tự vẽ giao diện gốc lên GPU và dựng lại từ hàm view ở mỗi khung hình, còn NSGo chỉ gửi những phần thay đổi xuống cho các view gốc của hệ thống.

- **Chọn MyGo** cho ứng dụng máy tính để bàn cần hiển thị giống nhau trên mọi hệ điều hành máy tính, các cửa sổ trang web gọi hàm có định kiểu vào Go, cùng các API và công cụ phát hành cho máy tính để bàn: biểu tượng khay hệ thống, liên kết tệp, trình cài đặt, công chứng (notarization) và cập nhật delta có chữ ký.
- **Chọn NSGo** để vươn tới cả điện thoại, sử dụng cơ chế văn bản, nhập liệu và khả năng truy cập gốc của từng nền tảng, hoặc giữ trạng thái trong các signal thay vì vẽ lại toàn bộ từ dữ liệu ứng dụng.

## Các bộ công cụ Go khác

- **Fyne và Gio** tự vẽ mọi thứ giống như MyGo, và cả hai đều hỗ trợ điện thoại lẫn web. Fyne cung cấp bộ widget đầy đủ với giao diện riêng; Gio theo mô hình immediate mode, ở mức thấp và linh hoạt. Cả hai đều đã có phiên bản phát hành và người dùng; nhưng không công cụ nào trông hay mang lại cảm giác gốc của hệ điều hành.
- **Wails** đặt giao diện web vào webview hệ thống với Go ở phía sau: mọi thư viện web đều hoạt động, nhưng giao diện thực chất là một trang web và chỉ dành cho máy tính để bàn.

## Framework bằng ngôn ngữ khác

- **Flutter và Compose Multiplatform** tự vẽ từng điểm ảnh: giống hệt nhau trên mọi nền tảng và đã trưởng thành với hệ sinh thái lớn, nhưng văn bản, cuộn trang và nhập liệu phải làm lại thay vì dùng đồ gốc, và bạn cần học Dart hoặc Kotlin.
- **React Native** chia sẻ cùng mô hình với NSGo là dùng lõi phản ứng (reactive core) điều khiển các view gốc, và sở hữu hệ sinh thái lớn nhất trong nhóm này. Framework này tập trung vào iOS và Android, chạy logic ứng dụng bằng JavaScript; còn NSGo chạy bằng Go và coi máy tính để bàn lẫn web là nền tảng hạng nhất.
- **.NET MAUI** bọc các điều khiển của từng nền tảng bằng C#, nhưng không hỗ trợ Linux hay web. **Avalonia** tự vẽ giao diện theo phong cách gần với WPF, chạy trên mọi máy tính để bàn, điện thoại và web.
- **Qt** đã trưởng thành, nhanh và chạy được trên thiết bị nhúng, nhưng tự vẽ widget riêng, yêu cầu C++ hoặc QML, và được cấp phép theo LGPL hoặc giấy phép thương mại.
- **Electron và Tauri** hiển thị trang web: dùng trọn vẹn công cụ và thư viện web, nhưng đánh đổi bằng mức tiêu tốn bộ nhớ của trình duyệt (đặc biệt là Electron) và một giao diện mang cảm giác của trang web.

## Lựa chọn

Hãy chọn NSGo cho một ứng dụng cần mang lại cảm giác gốc trên điện thoại, máy tính để bàn và web từ một codebase Go duy nhất, khi bạn chấp nhận được việc dự án còn đang ở giai đoạn đầu. Đối với một ứng dụng phải phát hành gấp trên một API đã ổn định, hãy chọn một framework trưởng thành hơn trong bảng trên: Flutter hoặc React Native cho điện thoại; MyGo, Wails hoặc Tauri nếu chỉ làm cho máy tính để bàn.
