---
title: Danh sách và cuộn
description: Các scroll view với cử chỉ gốc, và danh sách tái sử dụng hàng cho bất kỳ số lượng mục nào.
group: Khái niệm cốt lõi
order: 55
---

## Một scroll view

`core.Scroll` là scroll view riêng của nền tảng, nên các thao tác vuốt nhanh (fling), hiệu ứng nảy, thanh cuộn, căn chốt (snapping) và cuộn bằng cảm ứng đều là của hệ thống gốc. Các phần tử con của nó được sắp xếp theo cột, hoặc theo hàng với `Horizontal`, dài bao nhiêu tùy nhu cầu.

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

Scroll view lấp đầy vùng chứa của nó và phần padding được đặt vào nội dung bên trong, nhờ vậy thanh cuộn luôn nằm sát cạnh. Nó sẽ co lại để vừa với phần tử cha trừ khi style của nó quy định khác. Đặt `NoShrink` khi nó phải giữ kích thước yêu cầu trong lúc một phần tử anh em co lại. `HideIndicator` ẩn chỉ báo cuộn của nền tảng, còn `Bar` vẽ thanh cuộn riêng của ứng dụng.

`Behavior` và `BehaviorFunc` chọn cách nó cuộn: `Paging`, `SnapToInterval` và `SnapToOffsets` cùng với `SnapToAlignment`, một `DecelerationRate`, `NoBounces`, `ContentInset` và các quy tắc cho bàn phím. `MinZoomScale` và `MaxZoomScale` cho phép phóng to bằng thao tác chụm, `OnRefresh` và `Refreshing` cho phép kéo xuống để làm mới, và `OffsetValues` bám theo độ lệch cuộn trên luồng chính cho các hiệu ứng di chuyển cùng nó.

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` báo cáo độ lệch cuộn nhiều nhất một lần mỗi lượt, hoặc một lần mỗi `ScrollEventThrottle`. `OnScrollDragStart`, `OnScrollDragEnd`, `OnMomentumStart`, `OnMomentumEnd` và `OnScrollEnd` đánh dấu điểm đầu và cuối các cử chỉ của người dùng. `Ref` cuộn từ mã bằng `ScrollTo`, `ScrollToEnd`, `ZoomTo` và `ScrollToIndex` đối với danh sách.

## Một danh sách

`core.List` hiển thị dữ liệu dài trên một scroll view gốc trong khi chỉ tạo các hàng ở gần màn hình. `Count` cho biết có bao nhiêu hàng và `Row` dựng một hàng; `Key` gán cho mỗi hàng một định danh, để các hàng còn lại giữ nguyên ô (cell) của chúng khi có hàng được thêm vào hoặc xóa đi phía trên.

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` chạy một lần cho mỗi ô được tái sử dụng và cập nhật `r.Index()` khi ô đó được dùng lại cho một hàng khác, vì vậy hãy đọc `r.Index()` và các signal của mục bên trong các hàm phản ứng (`ContentFunc`, `StyleFunc`, `NewMemo`) thay vì giữ lại một giá trị chỉ đọc một lần khi dựng hàng. Các ô nhàn rỗi được tạm cất, nên các phép tính của chúng sẽ chờ cho đến khi ô được gắn lại với một hàng đang hiển thị.

`Header`, `Footer`, `Empty` và `Separator` chứa nội dung xung quanh và giữa các hàng. `NumColumns` đặt các hàng cạnh nhau, `Gap` và `ColumnGap` tạo khoảng cách giữa chúng, và `Horizontal` cuộn theo chiều ngang. `StickyHeaderIndices` ghim các hàng đó vào đầu vùng hiển thị cho đến khi hàng tiếp theo đẩy chúng ra, và `StickyHeaderOffset` chừa chỗ cho một header nằm phía trên chúng.

`OnViewableItemsChange` báo cáo các hàng đang hiển thị theo các ngưỡng của `ViewabilityConfig`, và `OnLoadMore` chạy khi vùng hiển thị tiến gần đến cuối, một lần cho mỗi số lượng hàng. `OnLoadMoreAtStart` làm điều tương tự ở phần đầu, và `InitialScrollIndex` mở danh sách tại một hàng. `Alignment: core.ListAlignEnd` đặt danh sách nằm ở cuối, giống như một ứng dụng trò chuyện với các hàng theo thứ tự đọc: nó mở ở hàng mới nhất, bám theo các hàng mới khi người dùng đang ở cuối, và giữ một cuộc trò chuyện ngắn ở phía dưới. `Inverted` thay vào đó đặt hàng 0 ở cuối, và `MaintainVisibleContentPosition` bám theo phần cuối của một danh sách đang dài ra khi người dùng ở gần đó.

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` cuộn một hàng tới giữa vùng hiển thị, và `list.ScrollToEnd(true)` cuộn tới cuối. `list.ListPosition()` đọc vị trí bắt đầu của vùng hiển thị, gồm một hàng và khoảng cách đã vào trong hàng đó, để giữ vị trí đang đọc: `list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` đưa một danh sách được tạo mới trở lại đúng chỗ đó. Một danh sách cũng có thể nằm trong cùng một trang với nội dung khác: đặt `Viewport: core.ListViewportParent`, và scroll view bao quanh sẽ cuộn danh sách cùng với phần còn lại.

## Bảng

`base/datatable` hiển thị một bảng hàng triệu dòng với các hàng và cột ảo hóa, header cố định, các cột ghim ở bên trái (`Fixed`) hoặc bên phải (`FixedRight`) trong khi các cột khác cuộn bên dưới chúng, sắp xếp, chọn một hoặc nhiều hàng, thay đổi kích thước và di chuyển cột, cùng các ô do chính ứng dụng định nghĩa. `MultiSelect` hỗ trợ bật/tắt bằng Command hoặc Control, chọn dải bằng Shift và mở rộng vùng chọn bằng bàn phím khi giữ Shift, trong khi `SelectedRows` và `OnSelectedRowsChange` cho phép ứng dụng kiểm soát các hàng được chọn. `CellTooltip` gắn tooltip cho một ô, đồng thời mô tả ô đó cho trình đọc màn hình, với kiểu dáng từ `Styles.CellTooltip`; những gì `RenderCell` vẽ không nhận con trỏ, nên một `ui.Tooltip` bên trong ô sẽ không bao giờ hiện ra. `base.Table` là một bảng đơn giản chứa các hàng của ứng dụng dành cho dữ liệu nhỏ hơn, và `base/plot` vẽ biểu đồ.

Dùng `DataTable.LabelFunc` cho tên lưới thay đổi theo ngôn ngữ hoặc trạng thái ứng dụng. `ColumnGroup.LabelFunc` cập nhật tiêu đề của nhóm cột trong khi nhóm vẫn còn trong vùng hiển thị.

## Tiếp theo

- [Hoạt ảnh](animation.md): các hiệu ứng bám theo một scroll view.
- [Văn bản](text.md): các hàng văn bản tự xuống dòng.
- [Điều hướng](navigation.md): các màn hình có nội dung cuộn.
