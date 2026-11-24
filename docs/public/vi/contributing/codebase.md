---
title: Cấu trúc mã nguồn
description: Nơi từng phần của NSGo được đặt và các tài liệu giải thích lý do.
group: Đóng góp
order: 81
---

NSGo là một Go module duy nhất. Ứng dụng nhập các gói của nó từ tầng trên; backend của từng nền tảng nằm ở tầng dưới và tiếp nhận các thay đổi dưới dạng các view gốc để tạo mới, cập nhật và gỡ bỏ.

## Các gói

| Gói | Nội dung |
| --- | --- |
| `nsgo` (thư mục gốc) | `nsgo.Run` và `nsgo.NewView`, chọn backend theo các build tag |
| `core` | API cho ứng dụng: phần tử, kiểu dáng, lớp, chủ đề, signal và dịch vụ; chuẩn bị bố cục và văn bản |
| `base` | Các thành phần chưa tạo kiểu, mỗi thành phần một tệp, và các thành phần phức tạp dạng gói (`base/dock`, `base/editor`) |
| `ui` | Các thành phần của `base` đã được tạo kiểu, cùng các gói như `ui/chat` |
| `mobile`, `desktop` | Những tính năng chỉ điện thoại hoặc chỉ máy tính để bàn mới có |
| `tsx` | Ứng dụng TSX, trình đóng gói và trình biên dịch của chúng, cùng các module mà chúng nhập |
| `i18n`, `*/locales` | Các danh mục thông điệp và mã nguồn được sinh ra từ chúng |
| `nsgotest` | Các bài kiểm thử mô phỏng thao tác như người dùng, ở chế độ headless hoặc trên nền tảng thật |
| `internal/engine`, `internal/driver`, `internal/layout` | Goroutine của engine, giao thức gắn kết (mount protocol) và bố cục |
| `internal/platform/<target>` | Một backend: view gốc, văn bản và nhập liệu cho một nền tảng đích |
| `cmd/nsgo` | Công cụ `nsgo` |
| `examples/` | Ứng dụng gallery và các ứng dụng ví dụ khác |

Các gói chỉ nhập theo chiều từ trên xuống dưới; `internal/moduletest/layering_test.go` kiểm tra đồ thị phụ thuộc này trên mọi nền tảng đích.

## Tài liệu thiết kế

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) trình bày bản đồ các gói, các luồng thực thi và luồng dữ liệu từ một commit cho tới khi hiển thị lên màn hình, đồng thời liệt kê các chủ đề trong `docs/architecture/`: mỗi phần một tài liệu riêng, như bố cục, văn bản, cuộn trang, chủ đề hoặc công cụ CLI. Hãy đọc tài liệu của phần bạn định sửa trước khi thay đổi; nó ghi lại cách phần đó hoạt động và những điểm khác biệt so với [Native Shell](https://github.com/longbridge/nsh), dự án mà NSGo chuyển thể thiết kế.

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) chứa các quy tắc mà mọi thay đổi phải tuân theo, áp dụng cho cả con người lẫn coding agent. Một vài quy tắc định hình hầu hết các thay đổi:

- Mỗi API có một nơi ở duy nhất: `core` cho phần tử và môi trường, `base` cho điều khiển, `ui` cho diện mạo của chúng, `mobile` và `desktop` cho từng loại thiết bị tương ứng.
- `base` không có diện mạo riêng; một thành phần trong `ui` là phiên bản `base` có gắn diện mạo.
- Không có gì tự động thay đổi ngầm theo thiết bị: không có điều khiển lớn hơn trên máy tính để bàn hay nhỏ hơn trên điện thoại trong `core` hoặc `base`.
- Dùng view gốc ở mọi nơi, đồng nhất trên mọi nền tảng: những gì một nền tảng có sẵn nguyên bản, các nền tảng khác sẽ mô phỏng theo.
- Không viết C thủ công, và không cần gì ngoài macOS, Xcode và Go để build mọi nền tảng đích trừ trình giả lập Android x86_64.

Những phần còn thiếu theo từng nền tảng được liệt kê trong [lộ trình phát triển](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md).
