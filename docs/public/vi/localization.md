---
title: Bản địa hóa
description: Dịch các thông điệp của ứng dụng và bám theo ngôn ngữ mà ứng dụng hiển thị.
group: Khái niệm cốt lõi
order: 62
---

## Thông điệp

`locales/` nằm cạnh `nsgo.yaml` chứa các thông điệp của ứng dụng, mỗi ngôn ngữ một tệp YAML (`en.yaml`, `de.yaml`) theo định dạng ICU MessageFormat:

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

Một thông điệp là một chuỗi, hoặc là `message` kèm `description` và `placeholders` dành cho người dịch.

Package import `locales/`. `dev`, `build` và `test` kiểm tra các catalog và viết một hàm Go cho mỗi thông điệp bên cạnh chúng, chẳng hạn `locales.Greeting(cx, "Ada")`, hàm này nói thông điệp bằng ngôn ngữ mà ứng dụng đang hiển thị và bám theo khi ngôn ngữ đó thay đổi; `go tool nsgo i18n` chỉ viết các hàm này. Thông điệp định dạng số và ngày tháng (`{d, date, medium}`), và `appName` đặt tên ứng dụng bằng từng ngôn ngữ trong các gói cài đặt của nó, giống như một thông điệp có khóa `apple.info`, chẳng hạn `NSCameraUsageDescription`, dịch mục Info.plist tương ứng ([quyền truy cập](permissions.md#usage-descriptions)).

## Ngôn ngữ

`cx.SetLocale("de")` chọn ngôn ngữ, và bố cục chạy theo hướng viết của ngôn ngữ đó. Trong một ứng dụng độc lập, những gì do chính hệ thống hiển thị, như menu chọn văn bản, hộp thoại tệp và các widget của GTK, cũng theo ngôn ngữ bị ép buộc (ngay lập tức trên Android 13+, Windows và Linux, và từ lần khởi chạy tiếp theo trên macOS và iOS); `cx.SetLocale("")` quay lại bám theo ngôn ngữ của người dùng.

```sh
go tool nsgo dev -lang en-XA
```

chạy ứng dụng trong một ngôn ngữ giả (pseudo locale) để làm lộ ra văn bản chưa dịch và văn bản bị cắt. Bất kỳ bản build nào cũng nhận tùy chọn này khi khởi động: `--nsgo-lang=en-XA`.

## Tuần, giờ và đơn vị đo

`i18n.Regional(cx)` cho biết cách người dùng đọc tuần, giờ và đơn vị đo, độc lập với ngôn ngữ: ngày bắt đầu tuần, định dạng giờ 12 hoặc 24 giờ, hệ đo lường và đơn vị nhiệt độ, theo cài đặt của họ trên nền tảng, nếu không thì theo khu vực của họ. Lịch và bộ chọn ngày bắt đầu tuần từ ngày đó và các trường thời gian hiển thị theo định dạng giờ đó, trừ khi ứng dụng đặt `FirstDayOfWeek` hoặc `HourCycle`, và giờ mà các thông điệp trong catalog viết ra cũng theo định dạng giờ của người dùng.

## Văn bản riêng của NSGo

Các thành phần của NSGo hiển thị văn bản của chúng bằng những ngôn ngữ mà NSGo đã được dịch sang. Với ngôn ngữ khác, một khóa đặt tên theo một package, chẳng hạn `github.com/nsgo-dev/nsgo/base/locales`, cung cấp các thông điệp của package đó bằng ngôn ngữ của tệp. Menu của các nền tảng, cùng các vai trò và trạng thái mà trình đọc màn hình của Android và Windows thông báo ở những chỗ hệ thống không có từ riêng, chẳng hạn một thẻ (tab) hay một hộp tìm kiếm, sẽ dùng văn bản của `github.com/nsgo-dev/nsgo/desktop/locales`.
