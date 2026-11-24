---
title: Thực hiện thay đổi
description: "Những gì một pull request gửi lên NSGo cần có: từ việc cập nhật đồng bộ trên mọi backend đến kiểm thử, tài liệu và một dòng trong changelog."
group: Đóng góp
order: 82
---

## Trước khi bắt đầu

Hãy mở một issue hoặc một thảo luận (discussion) cho bất kỳ thay đổi nào lớn hơn một bản sửa lỗi nhỏ, để thiết kế được thống nhất trước khi viết mã. Những câu hỏi mà thiết kế còn bỏ ngỏ được liệt kê trong [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions); câu trả lời sẽ được chốt tại đó trước tiên.

## Mọi backend

Một bản sửa lỗi hay một tính năng ở một backend thường cũng cần có ở các backend còn lại. Hãy thực hiện cùng một thay đổi đó trên mọi nền tảng đích cần đến nó, hoặc ghi nhận phần còn thiếu vào [lộ trình phát triển](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Các thành phần cần có diện mạo và hành vi giống nhau trên mọi nền tảng.

## Kiểm tra

Hãy dùng công cụ `nsgo` thay vì gọi trực tiếp `go vet`, `go test` hoặc `go build`:

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

Thêm bài kiểm thử cho những gì bạn thay đổi: dùng `nsgotest` cho những gì người dùng nhìn thấy, và bài kiểm thử riêng của từng gói cho phần còn lại. Chạy gallery trên các nền tảng bạn đã chỉnh sửa ([Thiết lập môi trường phát triển](../contributing.md#the-gallery)).

Định dạng các tệp Go bằng `gofmt` từ đúng phiên bản Go trong `go.mod`; `gofmt -l .` liệt kê các tệp cần định dạng lại. CI kiểm tra mọi tệp Go đang được theo dõi và sẽ báo lỗi kèm tên các tệp chưa đúng định dạng.

## Pull request

Đặt tiêu đề theo chuẩn Conventional Commit, chẳng hạn `fix(linux): keep the caret after a paste`: khi squash merge, tiêu đề này sẽ trở thành dòng subject của commit, và bước kiểm tra sẽ thất bại cho đến khi tiêu đề hợp lệ. Biểu mẫu PR sẽ yêu cầu liệt kê các nền tảng đích bị ảnh hưởng, ảnh chụp trước và sau đối với mọi thay đổi hiển thị, cùng các bước kiểm tra bạn đã chạy.

## Tài liệu và changelog

- Cập nhật tài liệu có thông tin thực tế thay đổi: một chủ đề trong `docs/architecture/` về cách NSGo vận hành bên trong, hoặc một trang trong `docs/public/` về cách ứng dụng và thành phần sử dụng nó ([Tài liệu và trang web](docs.md)).
- Giữ `README.md` và `README.zh-CN.md` tương đương nhau; mọi thứ khác viết bằng tiếng Anh, kèm các trang tiếng Trung trong `docs/public/zh/`.
- Ghi nhận một thay đổi mà người dùng có thể thấy thành một dòng duy nhất, có kèm tham chiếu như `(#12 by @you)`, trong một tệp riêng đặt ở `changelog.d/` dưới đúng tiêu đề loại thay đổi như `### Fixed`, thay vì sửa trực tiếp `CHANGELOG.md` — tệp này sẽ được tự động tổng hợp từ các tệp trên khi phát hành ([định dạng](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)).
