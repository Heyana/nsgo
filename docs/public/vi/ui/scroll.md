---
title: Cuộn
description: Scroll view của nền tảng, với phân trang, bám điểm dừng, thu phóng và làm mới.
group: Thành phần
order: 39
---

`core.Scroll` là scroll view của chính nền tảng. Các phần tử con của nó được xếp thành một cột, hoặc một hàng với `Horizontal`, và nó lấp đầy vùng chứa trong khi nội dung cuộn.

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Phần đệm đặt trên nội dung, không phải trên scroll view, để thanh cuộn vẫn nằm sát mép. Scroll view co lại cho vừa với phần tử cha trừ khi kiểu dáng của nó quy định khác. Đặt `NoShrink` khi nó phải giữ kích thước được yêu cầu trong lúc một phần tử anh em co lại.

## Hành vi

`Behavior` chọn cách nó cuộn: `Paging`, `SnapToInterval` hoặc `SnapToOffsets` cùng `SnapToAlignment`, một `DecelerationRate`, `NoBounces`, `ContentInset`, và các quy tắc của bàn phím.

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` ẩn thanh cuộn của nền tảng và `Bar` vẽ thanh cuộn riêng của ứng dụng. `MinZoomScale` và `MaxZoomScale` cho phép thao tác chụm để thu phóng; `NoPinchZoom` bỏ qua chúng. `CenterContent` căn giữa nội dung nhỏ hơn khung nhìn.

## Làm mới và sự kiện

`OnRefresh` khiến view có thể làm mới; hãy đặt `Refreshing` trong lúc công việc đang chạy. `PullValue` theo dõi mức nội dung bị kéo, dành cho chỉ báo riêng của ứng dụng, và `Behavior.RefreshIndicator` đặt chiều cao của nó.

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` báo cáo vị trí cuộn tối đa một lần mỗi lượt, hoặc một lần mỗi `ScrollEventThrottle`; `Stale` đánh dấu một báo cáo có từ trước khi nền tảng áp dụng một lần cuộn từ mã. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` và `OnScrollEnd` đánh dấu đầu và cuối các cử chỉ của người dùng.

`OffsetValues` theo dõi vị trí cuộn trên luồng chính, dành cho một tiêu đề thu gọn mà không cần commit mỗi khung hình. `Ref` cuộn từ mã bằng `ScrollTo`, `ScrollToEnd` và `ZoomTo`, và ref của bất kỳ view nào bên trong có thể đưa view đó vào vùng nhìn thấy bằng `Reveal`, cuộn mọi scroll view bao quanh nó:

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

`RevealBox` thay vào đó đưa một hộp bên trong view vào vùng nhìn thấy, chẳng hạn hộp của một từ lấy từ `TextRangeBoxes` của một đoạn văn bản, và `Margin` giữ khoảng trống ở các mép của scroll view ngoài cùng cho một thanh nằm đè lên nó.

Một view mà bàn phím chuyển tiêu điểm tới sẽ tự cuộn vào vùng nhìn thấy.

## Bố cục

`Horizontal` cuộn theo chiều ngang. `Behavior.ContentInset` đặt khoảng trống quanh nội dung, cuộn cùng với nội dung. Với một trang lấp đầy vùng an toàn, hãy dùng `PaddingEnv` trên nội dung, không phải trên scroll view.

## Khả năng truy cập

`Label` đặt tên cho vùng cuộn và `Role` cho biết nó là gì, chẳng hạn một list box. Các cử chỉ cuộn, cuộn bằng bàn phím và cuộn bằng trình đọc màn hình của nền tảng đều được giữ nguyên; `Busy` báo cáo một view vẫn đang tải.

## Nền tảng

Mọi backend đều dùng scroll view của nền tảng, nên các thao tác vuốt nhanh, hiệu ứng nảy, thanh cuộn và cuộn cảm ứng đều là gốc. Hiệu ứng ở mép khác nhau: macOS và iOS co giãn đàn hồi, Android kéo giãn, Windows và Linux dừng lại, còn web theo trình duyệt. `EdgeEffects` và `EdgeBar` thêm các thanh đè lên nội dung ở nơi ứng dụng muốn. Trên điện thoại và máy tính bảng iOS và Android, các scroll view dọc và ngang có nội dung tràn sẽ hiển thị một tay cầm cuộn có thể kéo bằng cảm ứng dọc theo trục đang hoạt động, cho phép tua qua toàn bộ phạm vi nội dung; các scroll view ngắn vừa khít mà không tràn, view đang thu phóng, `Behavior.NoScroll`, `HideIndicator` và các view dùng `Bar` tùy chỉnh hoặc `base.SetScrollbarTheme` sẽ ẩn nó. Trong kiểm thử, `nsgotest.App.ScrollHandle` báo cáo giới hạn khung nhìn của nó và `DragAt` hoặc `Touch` kéo nó trực tiếp. Trên Windows, chỉ báo dạng lớp phủ xuất hiện khi cuộn bằng bánh xe, cảm ứng, kéo thanh cuộn và quán tính của chúng, không xuất hiện với `ScrollTo`, `ScrollBy`, reveal, inset hay hoạt ảnh do ứng dụng thực hiện.

## Liên quan

- [Danh sách và cuộn](../lists.md): hướng dẫn đầy đủ về cuộn và danh sách.
- [List](list.md): các hàng được tái sử dụng.
- [Bố cục](../layout.md): phần đệm và vùng an toàn.
- Các trang Scroll và Lists trong gallery.
