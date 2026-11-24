---
title: Bố cục
description: Flexbox, grid, kích thước và định vị, với vùng an toàn và bàn phím được tính vào bố cục.
group: Khái niệm cốt lõi
order: 51
---

## Khung

Mọi phần tử là một khung được engine bố trí. Các giá trị mặc định giống React Native: flexbox theo cột, các phần tử con giữ kích thước trừ khi chúng giãn ra hoặc co lại, các mục được kéo giãn theo trục chéo, và kích thước bao gồm cả padding và viền.

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`, `Row`, `ColumnReverse` và `RowReverse` đặt trục chính; `Wrap` và `WrapReverse` cho phép các phần tử con tràn sang nhiều dòng. `JustifyContent` sắp xếp chúng dọc theo trục đó, còn `AlignItems`, hoặc `AlignSelf` trên một phần tử con, sắp xếp chúng theo trục chéo. `Gap`, `RowGap` và `ColumnGap` tạo khoảng cách giữa chúng, và padding cùng margin của `style` dùng `core.All`, `core.Symmetric` hoặc `core.Edges`.

## Kích thước

`Width`, `Height`, `MinWidth`, `MaxWidth` và `FlexBasis` nhận `core.Pt` cho đơn vị điểm hoặc `core.Pct` cho phần trăm của phần tử cha. `FlexGrow` chia sẻ không gian trống và `FlexShrink` cho phép một khung nhường lại một phần; một khung không có cả hai sẽ giữ nguyên kích thước. `AspectRatio` là chiều rộng chia cho chiều cao. `AutoMargin` chiếm không gian trống ở các cạnh mà nó chỉ định, nên một cạnh sẽ đẩy khung về phía cạnh còn lại.

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

Nội dung của một khung quyết định kích thước của nó cho đến khi chiều rộng hoặc chiều cao nói khác. `core.MinContent`, `core.MaxContent` và `core.FitContent` nhận các kích thước mà CSS đặt tên: chiều rộng min-content của một văn bản là từ dài nhất của nó. Một văn bản trong một hàng chỉ co lại và xuống dòng khi có `FlexShrink: 1`; thêm `FlexGrow: 1` khi nó cần chiếm phần không gian còn lại. Giá trị tối thiểu chưa đặt vốn đã là 0, như trong React Native, nên `MinWidth: core.Pt(0)` không thay đổi gì và một văn bản đang co lại sẽ ngắt giữa các từ khi hàng hẹp hơn từ dài nhất của nó. `MinWidth: core.AutoMinimum` dừng nó ở từ dài nhất, giống như giá trị tối thiểu tự động của CSS.

Một màn hình lấp đầy cửa sổ dùng `FlexGrow: 1` và `FlexShrink: 1`, và mỗi khung nằm giữa nó với một danh sách hay scroll view cần cuộn cũng vậy. Chỉ với `FlexGrow`, một khung sẽ giãn theo nội dung của nó, nên một danh sách dài sẽ kéo giãn nó vượt quá cửa sổ thay vì cuộn.

## Grid

`DisplayGrid` bố trí các phần tử con theo CSS grid. `GridTemplateColumns` và `GridTemplateRows` nhận cú pháp track của CSS, còn `GridRow` và `GridColumn` đặt một mục theo đường kẻ hoặc span.

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` tạo ra nhiều cột nhất có thể vừa. `GridFlow` chọn thứ tự mà grid được lấp đầy, còn `JustifyItems`, `JustifySelf` và `AlignItems` đặt các mục trong ô của chúng.

## Vị trí

`Relative` là mặc định và giữ một khung trong luồng; `Top`, `Right`, `Bottom` và `Left` dịch chuyển nó. `Absolute` đưa nó ra khỏi luồng và đặt nó trong phần tử tổ tiên được định vị gần nhất. `Fixed` đặt nó trong view gốc hoặc modal của nó, bất kể thứ gì cuộn, còn `Sticky` ghim nó bên trong phần tử cha trong scroll view gần nhất: `Top` tính từ đỉnh, hoặc `Bottom` tính từ đáy khi `Top` là auto (`Left` và `Right` theo chiều ngang). Một footer với `Position: core.Sticky, Bottom: core.Pt(0)` nằm ở đáy khi nội dung dài cuộn và đi theo sau nội dung ngắn. `ZIndex` xếp chồng các khung giữa các phần tử cùng cấp.

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` giữ một khung trong luồng và bỏ qua các giá trị inset của nó, nên các phần tử con định vị tuyệt đối sẽ tìm một tổ tiên phía trên nó. `Overflow` cắt các phần tử con theo một khung, và `DisplayNone` đưa một khung ra khỏi bố cục.

Bóng đổ bên ngoài và đường viền ngoài (outline) của chính khung nằm bên ngoài vùng cắt của nó, như trong CSS, trên Android, Windows, Linux, web và Apple, ở đó áp dụng cho mọi loại view, kể cả các góc có bán kính khác nhau và nội dung cuộn được phóng to. Cho đến khi điều này được kiểm tra trên một iPhone thật, `Bare` của `ui.Dialog` vẫn đặt vùng cắt của nó trên một view bên trong có cùng các góc, thụt vào theo độ dày viền, giúp giữ chúng trên mọi nền tảng đích:

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## Hướng và môi trường

`LayoutDirection` bố trí một cây con từ trái sang phải hoặc từ phải sang trái, ghi đè hướng của ứng dụng: `DirectionRTL` và `DirectionLTR` đặt hướng, còn mặc định là kế thừa. Trái và phải trong margin, padding, viền và inset được hoán đổi theo hướng, nên chúng là điểm đầu và điểm cuối như trong React Native. `PhysicalEdges` và `PhysicalBorders` giữ các cạnh được đặt tên ở đúng vị trí của chúng. `WritingDirection` sắp xếp thứ tự các đoạn văn bản có nhiều hướng, và `WritingAuto` lấy hướng từ chữ cái có hướng mạnh đầu tiên.

`PaddingEnv` và `MarginEnv` thêm giá trị môi trường vào một cạnh. `core.EnvSafeArea` là inset của vùng an toàn ở cạnh đó và `core.EnvKeyboard` là chiều cao của bàn phím ảo, nhờ đó nội dung luôn nằm trên bàn phím trong khi chuyển động của nó được tạo hoạt ảnh cùng bàn phím.

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` chỉ định mọi cạnh, còn `cx.Env().SafeArea()` và `cx.Env().KeyboardHeight()` đọc các giá trị đó trong mã.

Trên điện thoại gập hoặc điện thoại hai màn hình, `cx.Env().Geometry()` liệt kê các vùng mà nếp gập hoặc bản lề chia cửa sổ ra, mỗi vùng kèm phần vùng an toàn nằm trong nó, cùng các nếp gập giữa chúng; `State` và `Angle` của nó cho biết thiết bị mở rộng đến mức nào (`core.HingeClosed`, `HingeHalfOpen` hoặc `HingeOpen`) ngay cả khi không có nếp gập nào cắt qua cửa sổ, như trên màn hình ngoài hoặc một màn hình được mở phẳng. Các thiết bị khác báo một vùng và `HingeUnknown`. Trên iOS 27.1, `cx.Env().VerticalBarEdge()` báo cạnh mà hệ thống chọn cho thanh dọc (`core.VerticalBarLeading`, `core.VerticalBarTrailing` hoặc `core.VerticalBarUnspecified`), mà `mobile.TabBar` và `ui.TabShell` tuân theo khi `Edge` là `mobile.TabBarAutomatic`. Không có gì được bố trí ngầm theo hình học: ứng dụng tự chọn các ngăn của mình.

## Class đáp ứng

`Class` nhận các class của Tailwind CSS: `flex`, `flex-col`, `flex-wrap`, `items-center`, `justify-between`, `gap-3`, `p-4`, `w-full`, `grid`, `grid-cols-3`, `col-span-2` và các class khác. Các breakpoint tuân theo cửa sổ, còn giao diện hiển thị, hướng và chế độ giảm chuyển động tuân theo môi trường:

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`, `md:`, `lg:`, `xl:` và `2xl:` áp dụng từ chiều rộng 40, 48, 64, 80 và 96 rem; `max-md:` áp dụng dưới một mốc. `portrait:`, `dark:`, `rtl:`, `motion-reduce:`, `contrast-more:`, `pointer-fine:`, `pointer-coarse:`, `ios:`, `android:` và `web:` tuân theo những gì môi trường cho biết. Các class được hợp nhất bên dưới một `Style`, và class của một trạng thái (`hover:`) trở thành style trạng thái, nên có thể kết hợp cả hai.

## Tiếp theo

- [Kiểu dáng và chủ đề](styles.md): màu sắc, góc bo, bóng đổ và giao diện của một cây con.
- [Văn bản](text.md): cách văn bản lấy kích thước và xuống dòng.
- [Danh sách và cuộn](lists.md): nội dung cuộn được và các hàng được tái sử dụng.
