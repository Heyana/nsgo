---
title: Phản hồi xúc giác
description: Phát phản hồi xúc giác định sẵn hoặc một chuỗi xung theo thời gian.
group: Tính năng nền tảng
order: 46
---

## Phát một phản hồi

`mobile.PlayHaptic` phát phản hồi dưới ngón tay người dùng trên iOS và Android, và không làm gì ở nơi khác, vì vậy hãy kết hợp nó với những gì người dùng nhìn thấy. `HapticSelection` đánh dấu một giá trị thay đổi, như bước của một bộ chọn, một phân đoạn hay một thanh trượt; từ `HapticLight` đến `HapticRigid` đánh dấu các phần tử va chạm, theo độ nặng và độ đàn hồi của chúng; còn `HapticSuccess`, `HapticWarning` và `HapticError` đánh dấu một tác vụ kết thúc với kết quả tương ứng.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## Phát một mẫu

`cx.Env().CanPlayHapticPattern()` cho biết phản hồi xúc giác theo thời gian có khả dụng hay không. Một mẫu chứa các xung với thời điểm bắt đầu `At` tính tương đối so với mẫu. `Duration` bằng 0 tạo ra một xung tức thời, còn thời lượng dương duy trì xung tối đa 30 giây. `Intensity` và `Sharpness` nằm trong khoảng từ 0 đến 1. Một mẫu mới sẽ thay thế mẫu trước đó. Truyền một mẫu rỗng để dừng phát. Mẫu sẽ dừng khi ứng dụng chuyển xuống nền.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS phát các sự kiện qua Core Haptics trên phần cứng được hỗ trợ. Android tuân theo thời gian và dùng cường độ khi bộ rung hỗ trợ điều khiển biên độ. Các trình duyệt Android có `navigator.vibrate` tuân theo thời gian bật và tắt sau một tương tác của người dùng. Android và web phát xung tức thời dưới dạng một xung 20 ms, còn độ sắc nét chỉ khả dụng trên iOS. Trên nền tảng không hỗ trợ, lời gọi không có tác dụng.

Trong kiểm thử, đặt `HapticPatterns: true` qua `nsgotest.Environment` và đọc `app.HapticPatterns`. Lịch sử có một mục nil khi ứng dụng hủy phát.

## Khi nào

Hãy phát phản hồi ngay khi cử chỉ diễn ra, chứ không phải khi kết quả trả về: phản hồi chọn khi một giá trị đi qua, phản hồi va chạm khi một view tiếp đất, phản hồi kết quả khi một tác vụ kết thúc. Phát một phản hồi cho thay đổi do người dùng gây ra, và không phát cho những gì ứng dụng tự làm.

## Tiếp theo

- [Thông báo](notifications.md): các thông báo và huy hiệu mang cùng tin tức.
- [Dịch vụ nền tảng](services.md): những gì còn lại mà hệ thống cung cấp.
- [Khả năng truy cập](accessibility.md): lời thông báo mà người dùng không cảm nhận được rung sẽ nghe thấy.
