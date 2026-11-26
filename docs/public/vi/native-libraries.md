---
title: Thư viện gốc
description: Phát hành kèm các thư viện dùng chung dựng sẵn và framework của Apple cùng một Go package thông qua các bước đóng gói, giấy phép và ký số thông thường của NSGo, và nêu các gói Linux mà chúng cần.
group: Bộ công cụ
order: 23
---

Một Go package có thể phát hành kèm các thư viện dùng chung mà nó nạp qua purego bằng cách đặt `nsgo-natives.yaml` trong thư mục package của nó, và từ phiên bản 2 có thể kèm cả framework cho iOS và tvOS, cũng như nêu các gói Linux mà nó cần. Khi một ứng dụng import package đó, NSGo chọn các khai báo của nó cho target, xác minh các tệp và đưa chúng vào trước khi ký số và tạo trình cài đặt. Ứng dụng không cần cấu hình thêm hay lệnh đóng gói nào khác.

Các bản build ứng dụng desktop và bài kiểm thử trên target tắt cgo cho mã của ứng dụng. Mã gốc phải dùng purego và một thư viện được khai báo trong `nsgo-natives.yaml`. NSGo từ chối các package của ứng dụng có chứa tệp cgo trước khi bản build bắt đầu.

## Khai báo thư viện

```yaml
version: 2
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

Phiên bản 2 hỗ trợ các khóa `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`, `ios`, `ios_simulator`, `tvos` và `tvos_simulator`. Phiên bản 1 chỉ hỗ trợ sáu khóa desktop; một tệp chỉ được đọc cho các khóa của phiên bản của nó, nên các bản build iOS và tvOS không đóng gói gì từ một tệp phiên bản 1. Hãy khai báo mọi target mà Go package được chọn; `[]` nói rõ rằng target đó không cần thư viện đóng gói kèm. Ngoài ra, Go build tag có thể loại package ra khỏi các target mà nó không hỗ trợ. `macos_universal` dùng cả hai mục macOS, còn một target terminal dùng các mục của desktop tương ứng: `tui_macos_arm64` dùng `macos_arm64`.

Mỗi thư viện có:

- `name`: tên tệp khi được cài đặt, kết thúc bằng `.dylib` trên macOS, `.dll` trên Windows, hoặc `.so` hay một `.so.*` có số phiên bản trên Linux. Trên iOS và tvOS, đây là tên của một framework, `<Name>.framework`, trong đó `Name` gồm chữ cái ASCII, chữ số, `.` và `-`, bắt đầu và kết thúc bằng một chữ cái hoặc chữ số.
- Đúng một trong hai: `path`, một tệp tương đối với thư mục package này, hoặc `url`, một URL HTTP(S) trỏ tới chính thư viện. Phiên bản 2 chỉ chấp nhận URL https, và việc tải xuống thất bại khi một lần chuyển hướng rời khỏi https. Hãy phát hành các thư viện đã giải nén thay vì URL của tệp nén, và với một framework thì chỉ phát hành tệp nhị phân của nó. URL không được chứa tên người dùng và mật khẩu nhúng.
- `sha256`: SHA-256 của tệp gốc, bắt buộc với cả hai nguồn. Ví dụ, `shasum -a 256 lib/macos-arm64/libexample.dylib` in ra giá trị này trên macOS.
- `licenses`: danh sách đường dẫn không rỗng tới các tệp giấy phép và thông báo gốc, tương đối với thư mục package này. Các văn bản này được phát hành trong `THIRD_PARTY_NOTICES.txt` và có trong `core.ThirdPartyNotices()` ở các bản build release. [Chính sách giấy phép](building.md#thông-báo-giấy-phép) hiện có của ứng dụng, bao gồm các ngoại lệ chính xác theo module, áp dụng độc lập cho giấy phép của thư viện gốc.

Đường dẫn cục bộ và liên kết tượng trưng (symbolic link) phải nằm bên trong package khai báo. NSGo từ chối các trường không xác định, checksum không khớp, tệp dành cho hệ điều hành hoặc kiến trúc khác, và các tên cài đặt bị trùng nhau. Hai package có thể dùng chung một tệp cùng tên với cùng checksum. Mỗi tệp giới hạn ở 1 GiB.

## Đóng gói và nạp

| Target | Thư mục thư viện |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows, Linux và các target terminal | Cạnh tệp thực thi |
| iOS và tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

Package nạp thư viện của nó từ thư mục đó, dựa trên vị trí của tệp thực thi thay vì thư mục làm việc. Hãy build sẵn các phụ thuộc để các thư viện đi kèm của chúng được nạp từ thư mục mà chúng được phát hành trong đó. Khai báo từng tệp đi kèm cần thiết và từng tên tệp mà trình nạp cần.

NSGo kiểm tra lại các tệp tải về trong cache trước khi dùng lại và điều phối các bản build chạy đồng thời thông qua một khóa tệp. Cache nội dung nằm trong thư mục cache người dùng của hệ điều hành, ở `nsgo/natives`. Các tệp cục bộ được xác minh khi chúng được sao chép.

Một gói macOS universal yêu cầu cùng tên thư viện cho cả hai kiến trúc. NSGo ghép các lát (slice) bằng `lipo`; một thư viện vốn đã là universal của nhà phát hành có thể dùng cho mục nào cũng được. Các thư viện được ký trước ứng dụng, sau đó đi qua các bước tạo disk image, ký số và notarization thông thường của ứng dụng. Việc ký số trên Windows bao gồm cả các DLL đã khai báo, kể cả khi `build.installers: false`. Trình cài đặt Linux bao gồm cùng các tệp và thông báo như thư mục chứa tệp thực thi. Trên Mac, các target terminal ký ad hoc các thư viện macOS; trên hệ thống khác chúng đóng gói thư viện như được phát hành, thư viện universal giữ nguyên, và từ chối thư viện arm64 chưa được ký.

Môi trường phát triển, `-run` và các bài kiểm thử trên target dùng cùng bước chọn package và sao chép, trên desktop cũng như trên iOS và tvOS. Các khai báo được chọn theo target, tag và overlay Go được sinh ra của bản build, bao gồm cả các thay thế cục bộ (local replacement) và package được vendor. Android, HarmonyOS, watchOS, web và mini program không đóng gói gì từ tệp này.

## Framework cho iOS và tvOS

NSGo kiểm tra tệp nhị phân của từng framework và dừng bản build kèm lý do, trừ khi nó:

- là một thư viện động Mach-O arm64 được build cho nền tảng của target, thiết bị thật hoặc trình giả lập, như `LC_BUILD_VERSION` của nó cho biết; một tệp universal đóng góp lát arm64 của nó; arm64e bị từ chối;
- yêu cầu hệ điều hành tối thiểu không mới hơn của ứng dụng, tức `apple.info.MinimumOSVersion`, hoặc nếu không có thì iOS 15.0 và tvOS 17.0 ([Xây dựng và đóng gói phát hành](building.md#nsgoyaml));
- có tên cài đặt `@rpath/<Name>.framework/<Name>`;
- chỉ nạp qua `@rpath` những framework mà các package của ứng dụng khai báo cho cùng target.

NSGo ghi `Info.plist` của framework, với `CFBundleIdentifier` là ID của ứng dụng, một dấu chấm và `Name` viết thường, kèm nền tảng và hệ điều hành tối thiểu của ứng dụng. Các bản build cho trình giả lập ký ad hoc framework. Bản build cho thiết bị để việc đó cho bước ký của ứng dụng: `sign.profile` ký framework bằng danh tính của ứng dụng trước chính ứng dụng, giống như Xcode làm với `nsgo build -run` và `nsgo dev` trên thiết bị. Framework có nạp các framework đi kèm cần đường dẫn chạy `@loader_path/..` hoặc `@executable_path/Frameworks` để tìm thấy chúng trong `Frameworks`, vì tệp thực thi của ứng dụng không có đường dẫn nào, và không nạp gì khác qua `@rpath` trừ khi có đường dẫn chạy của hệ thống như `/usr/lib/swift`. Framework có bundle ID trùng, không phân biệt hoa thường, với một bundle khác trong ứng dụng, chẳng hạn `StoreKit.framework` bên cạnh `NSGoStoreKit.framework` của NSGo, sẽ dừng bản build.

`nsgo build -embed` dừng lại và nêu tên package khi ứng dụng liên kết một package có khai báo framework cho iOS: XCFramework dành cho ứng dụng chủ hiện chưa thể mang chúng theo.

## Gói Linux

Từ phiên bản 2, `linux` nêu, cho từng kiến trúc Linux, các gói hệ thống mà gói Debian và gói Arch của một ứng dụng liên kết Go package này phụ thuộc vào và khuyến nghị:

```yaml
version: 2
libraries:
  # ...
linux:
  linux_x64:
    depends:
      deb: [libva2, libva-drm2, libpulse0]
      arch: [libva, libpulse]
    recommends:
      deb: [va-driver-all]
      arch: ["intel-media-driver: hardware decoding on Intel GPUs"]
  linux_arm64:
    depends:
      deb: [libavcodec61, libavformat61, libpulse0]
      arch: [ffmpeg, libpulse]
```

- `linux` là tùy chọn. Các khóa của nó là `linux_x64` và `linux_arm64`, nên một package đóng gói kèm thư viện trên một kiến trúc và nạp thư viện của hệ thống trên kiến trúc kia có thể nêu những gì mỗi kiến trúc cần.
- `depends` và `recommends` đều nhận các danh sách `deb` và `arch`, giống `linux.depends` và `linux.recommends` của ứng dụng trong [nsgo.yaml](building.md#nsgoyaml): mỗi mục `deb` được viết theo cách dpkg viết, chẳng hạn `libsoup-3.0-0 | libsoup3` hay `libavcodec61 (>= 7:7.1)`, và mỗi mục `arch` theo cách pacman viết, chẳng hạn `ffmpeg>=7` hay `libpulse: audio output`.
- Các trường `Depends` và `Recommends` của `.deb` cùng các dòng `depend` và `optdepend` của gói Arch liệt kê các gói của chính NSGo, rồi của ứng dụng, rồi của từng Go package mà bản build liên kết, theo thứ tự Go liệt kê chúng, bỏ qua các mục đã có và khuyến nghị của một Go package mà một mục `depends` đã nêu. Một Go package bị target hoặc build tag loại ra không thêm gì.
- Một mục rỗng hoặc chứa ký tự xuống dòng sẽ dừng bản build và nêu tên Go package đó, tương tự `linux` trong tệp phiên bản 1 hoặc một khóa khác hai khóa này.
- AppImage và Flatpak của bản build kho ứng dụng không lấy gì từ `linux`.

## Giữ lại tệp khi vendor

Thao tác vendor của Go cần dữ liệu nhúng để giữ lại các tệp nằm bên dưới thư mục package. Một package có thể đánh dấu manifest và các giấy phép của nó để được đưa vào mà không cần dùng tới nội dung byte của chúng lúc chạy:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Hãy đưa cả các đường dẫn thư viện cục bộ vào chỉ thị embed đó khi dùng `path`, ví dụ `lib/*`. Với tải xuống qua URL, Go package chỉ cần manifest và các tệp giấy phép.
