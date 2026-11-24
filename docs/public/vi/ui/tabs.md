---
title: Thẻ
description: Danh sách thẻ, điều khiển phân đoạn và thanh thẻ của hệ thống.
group: Thành phần
order: 40
---

`ui.Tabs[T]` chọn một trong vài giá trị, dưới dạng điều khiển phân đoạn hoặc danh sách thẻ; ứng dụng hiển thị nội dung của giá trị được chọn bên cạnh nó. Một danh sách thẻ là một điểm dừng Tab: các phím mũi tên di chuyển qua các thẻ.

```go preview=tabs/basic
section := core.NewSignal(cx, "overview")
ui.Tabs[string]{
	Options: []ui.Option[string]{
		{Value: "overview", Label: "Overview"},
		{Value: "activity", Label: "Activity"},
		{Value: "settings", Label: "Settings"},
	},
	Value:    section.Get,
	OnChange: section.Set,
	Label:    "Sections",
}
```

## Biến thể và kích thước

`Variant` là `TabsSegmented` (mặc định, một núm nổi trong một rãnh lõm) hoặc `TabsUnderline` (một đường nhấn màu dưới thẻ được chọn). Kiểu gạch chân có thể đặt `IndicatorColor` và dùng `IndicatorFit: TabsIndicatorContent` để bám theo nhãn và biểu tượng thay vì toàn bộ thẻ; trong TSX chúng là `indicatorColor` và `indicatorFit="content"`. `Size` là `SizeDefault`, `SizeSmall` hoặc `SizeLarge`, còn `Equal` cho mọi thẻ cùng một độ rộng thay vì vừa với nhãn của nó.

```go preview=tabs/underline
ui.Tabs[string]{Variant: ui.TabsUnderline, Equal: true, Size: ui.SizeSmall, Options: tabs, Value: section.Get, OnChange: section.Set}
```

Một tùy chọn có thể mang một `Icon`, một hàm `Disabled`, một nút `Content` và một `TestID`. `Content` thay thế nhãn và kế thừa kiểu văn bản của thẻ.

Dùng `OptionsFunc` khi các thẻ bám theo một signal. NSGo cập nhật các hàng tại chỗ theo giá trị của chúng, giữ nguyên thẻ được chọn và tiêu điểm bàn phím khi giá trị đó vẫn còn được cung cấp; một giá trị biến mất sẽ để trạng thái không được chọn cho đến khi ứng dụng thay đổi nó.

## Thanh thẻ

`ui.TabShell` giữ các phần cấp cao nhất bên cạnh một thanh thẻ ở mọi độ rộng cửa sổ — mặc định ở dưới cùng, hoặc theo chiều dọc dọc theo cạnh đầu hoặc cạnh cuối khi `Edge` (hoặc `EdgeFunc`) là `mobile.TabBarLeading`, `mobile.TabBarTrailing` hoặc `mobile.TabBarAutomatic` (bám theo `cx.Env().VerticalBarEdge()`). Nó dùng thanh của hệ thống khi mọi thẻ đều cung cấp `SystemIcon` hoặc một `Icon` SVG, ở bất kỳ cạnh nào trên iOS và ở cạnh dưới trên Android; một `SystemIcon` đặt tên cho một SF Symbol trên iOS hoặc một drawable trên Android được ưu tiên, nếu không thì SVG được hiển thị. Ngược lại, nó tự vẽ thanh với các biểu tượng SVG được cung cấp. Một thanh chiếm chỗ riêng sẽ tự tránh vùng an toàn ở cạnh của nó, nên `core.EnvSafeArea` ở cạnh đó của một phần bên cạnh nó, kể cả một trang `base.Nav`, bằng 0; dưới thanh hệ thống nổi ở dưới cùng của iOS, hãy giữ nội dung tránh khỏi nó bằng `ui.TabBarInset`.

Đặt `ShellTab.TextFunc` cho tiêu đề thẻ bám theo ngôn ngữ hoặc trạng thái ứng dụng và `TabShell.LabelFunc` cho tên truy cập của thanh. Việc thay đổi tiêu đề giữ nguyên phần được chọn và thẻ hiện có miễn là biểu tượng của nó vẫn giữ nguyên.

`mobile.TabBar` là thanh thẻ của hệ thống trên iOS và Android, với `Items`, `Selected`, `OnChange`, và tùy chọn `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` hoặc `mobile.TabBarAutomatic`) / `EdgeFunc`. Mỗi `mobile.TabBarItem` nhận một `Title` và tên biểu tượng của nền tảng trong `Icon`; `SVG: lucide.House()` được hiển thị ở bất kỳ đâu mà `Icon` không đặt tên cho SF Symbol trên iOS hoặc drawable trên Android, được thu vừa vào 24 point và tô màu theo trạng thái chọn. Trong TSX, `svg` nhận một tài liệu SVG và `edge` nhận `"bottom"`, `"leading"`, `"trailing"` hoặc `"automatic"`. Ở những nơi khác `mobile.TabBar` không chiếm chỗ, vì vậy hãy hiển thị `ui.Tabs` ở đó.

`ui.SidebarShell` tự giữ những gì thanh bên, chân trang, tiêu đề, nội dung và bảng bên phải của nó chứa tránh khỏi vùng an toàn bên trái và bên phải, với nền của chúng trải tới tận các cạnh cửa sổ, nên `core.EnvSafeArea` ở các phía đó bằng 0 bên trong chúng, và `ui.NavHeader` cộng vùng an toàn bên cạnh vào phần đệm của nó. Nội dung của một `ui.NavPage` hoặc của một phần trong `TabShell` tự tránh vùng an toàn bằng `PaddingEnv`, để những gì cần trải tới tận cạnh, chẳng hạn một hình ảnh tràn viền, có thể làm vậy.

## Khả năng truy cập

Danh sách báo cáo `RoleTabList`, mỗi thẻ báo cáo `RoleTab`, và thẻ được chọn báo cáo trạng thái của nó. Danh sách là một điểm dừng Tab; Left và Up, Right và Down, Home và End di chuyển giữa các thẻ đang bật, và thẻ được chuyển tới báo cáo một thao tác nhấn. `Label` đặt tên cho danh sách.

## Nền tảng

iOS và Android hiển thị thanh thẻ của hệ thống với giao diện và cử chỉ riêng của chúng; macOS, Windows, Linux và web hiển thị `ui.Tabs`. Nội dung của ứng dụng giống nhau trên mọi nền tảng.

## Liên quan

- [Điều hướng](../navigation.md): các trang và thanh thẻ trong một ứng dụng.
- [Menu](menu.md): các lệnh thay vì các phần.
- [Hộp chọn](select.md): nhiều tùy chọn trong một danh sách.
- Các trang Tabs và NavStack của gallery.
- [Thành phần không kiểu dáng](../authoring/base.md#tabs): `base.Tabs`, cho các thẻ với giao diện của riêng bạn.
