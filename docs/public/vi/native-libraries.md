---
title: Thư viện gốc
description: Phát hành kèm các thư viện dùng chung dựng sẵn cùng một Go package thông qua các bước đóng gói, giấy phép và ký số thông thường của NSGo.
group: Bộ công cụ
order: 23
---

Một Go package có thể phát hành kèm các thư viện dùng chung mà nó nạp qua purego bằng cách đặt `nsgo-natives.yaml` trong thư mục package của nó. Khi một ứng dụng import package đó, NSGo chọn các khai báo của nó cho target, xác minh các tệp và đưa chúng vào trước khi ký số và tạo trình cài đặt. Ứng dụng không cần cấu hình thêm hay lệnh đóng gói nào khác.

## Khai báo thư viện

```yaml
version: 1
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
```

Các khóa được hỗ trợ là `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64` và `linux_x64`. Hãy khai báo mọi target mà Go package được chọn; `[]` nói rõ rằng target đó không cần thư viện đóng gói kèm. Ngoài ra, Go build tag có thể loại package ra khỏi các target mà nó không hỗ trợ. `macos_universal` dùng cả hai mục macOS.

Mỗi thư viện có:

- `name`: tên tệp khi được cài đặt, kết thúc bằng `.dylib` trên macOS, `.dll` trên Windows, hoặc `.so` hay một `.so.*` có số phiên bản trên Linux.
- Đúng một trong hai: `path`, một tệp tương đối với thư mục package này, hoặc `url`, một URL HTTP(S) trỏ tới chính thư viện. Hãy phát hành các thư viện đã giải nén thay vì URL của tệp nén. URL không được chứa tên người dùng và mật khẩu nhúng.
- `sha256`: SHA-256 của tệp gốc, bắt buộc với cả hai nguồn. Ví dụ, `shasum -a 256 lib/macos-arm64/libexample.dylib` in ra giá trị này trên macOS.
- `licenses`: danh sách đường dẫn không rỗng tới các tệp giấy phép và thông báo gốc, tương đối với thư mục package này. Các văn bản này được phát hành trong `THIRD_PARTY_NOTICES.txt` và có trong `core.ThirdPartyNotices()` ở các bản build release. [Chính sách giấy phép](building.md#thông-báo-giấy-phép) hiện có của ứng dụng, bao gồm các ngoại lệ chính xác theo module, áp dụng độc lập cho giấy phép của thư viện gốc.

Đường dẫn cục bộ và liên kết tượng trưng (symbolic link) phải nằm bên trong package khai báo. NSGo từ chối các trường không xác định, checksum không khớp, tệp dành cho hệ điều hành hoặc kiến trúc khác, và các tên cài đặt bị trùng nhau. Hai package có thể dùng chung một tệp cùng tên với cùng checksum. Mỗi tệp giới hạn ở 1 GiB.

## Đóng gói và nạp

| Target | Thư mục thư viện |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows và Linux | Cạnh tệp thực thi |

Package nạp thư viện của nó từ thư mục đó, dựa trên vị trí của tệp thực thi thay vì thư mục làm việc. Hãy build sẵn các phụ thuộc để các thư viện đi kèm của chúng được nạp từ thư mục mà chúng được phát hành trong đó. Khai báo từng tệp đi kèm cần thiết và từng tên tệp mà trình nạp cần.

NSGo kiểm tra lại các tệp tải về trong cache trước khi dùng lại và điều phối các bản build chạy đồng thời thông qua một khóa tệp. Cache nội dung nằm trong thư mục cache người dùng của hệ điều hành, ở `nsgo/natives`. Các tệp cục bộ được xác minh khi chúng được sao chép.

Một gói macOS universal yêu cầu cùng tên thư viện cho cả hai kiến trúc. NSGo ghép các lát (slice) bằng `lipo`; một thư viện vốn đã là universal của nhà phát hành có thể dùng cho mục nào cũng được. Các thư viện được ký trước ứng dụng, sau đó đi qua các bước tạo disk image, ký số và notarization thông thường của ứng dụng. Việc ký số trên Windows bao gồm cả các DLL đã khai báo, kể cả khi `build.installers: false`. Trình cài đặt Linux bao gồm cùng các tệp và thông báo như thư mục chứa tệp thực thi.

Các bài kiểm thử target desktop gốc và môi trường phát triển dùng cùng bước chọn package và sao chép. Các khai báo được chọn theo target, tag và overlay Go được sinh ra của bản build, bao gồm cả các thay thế cục bộ (local replacement) và package được vendor. Hiện tại các target khác chưa đóng gói thư viện nào từ tệp này.

## Giữ lại tệp khi vendor

Thao tác vendor của Go cần dữ liệu nhúng để giữ lại các tệp nằm bên dưới thư mục package. Một package có thể đánh dấu manifest và các giấy phép của nó để được đưa vào mà không cần dùng tới nội dung byte của chúng lúc chạy:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Hãy đưa cả các đường dẫn thư viện cục bộ vào chỉ thị embed đó khi dùng `path`, ví dụ `lib/*`. Với tải xuống qua URL, Go package chỉ cần manifest và các tệp giấy phép.
