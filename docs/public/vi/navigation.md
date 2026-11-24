---
title: Điều hướng
description: Ngăn xếp điều hướng gốc, header hệ thống, modal, menu và thanh tab.
group: Khái niệm cốt lõi
order: 56
---

## Một ngăn xếp

`base.NewNav` giữ một ngăn xếp các route cùng một kiểu: `Push` thêm một trang, `Pop` gỡ trang trên cùng, `PopToRoot` quay về trang đầu tiên và `Replace` thay thế trang trên cùng. `Stack` của nó hiển thị mỗi route thành một trang, giữ các trang bên dưới vẫn được gắn cùng trạng thái của chúng trong khi trang trên cùng hiển thị. `ui.NavPage` cung cấp cho mỗi trang nền của `ui` và một `ui.NavHeader` với tiêu đề, nút quay lại và các nút khác.

watchOS không có thanh điều hướng, thanh tab hay công tắc của hệ thống, nên ở đó `ui.NavPage` vẽ `ui.NavHeader` của nó phía trên trang, bao gồm cả nút quay lại, `ui.TabShell` vẽ thanh tab riêng và `ui.SwitchSystem` là công tắc riêng của `ui`.

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

Trên iOS và Android, nền tảng vẽ header của hệ thống từ tiêu đề của trang và các nút của header, cùng với nút quay lại và hiệu ứng chuyển trang của nó; trên macOS, Windows, Linux và web thì `ui` vẽ header.

Android dùng top app bar của Material 3. Một thanh thông thường cao 64 dp với tiêu đề 22 sp. `HeaderLargeTitle` tạo một thanh lớn 152 dp với tiêu đề 28 sp, thu gọn thành thanh thông thường khi scroll view ở đầu trang cuộn nội dung bên dưới nó, và mở rộng trở lại khi nội dung quay về đầu.

Đặt `HeaderTitleStyle` và `HeaderLargeTitleStyle` để tùy chỉnh phông chữ của tiêu đề gốc ở dạng thu gọn và mở rộng trên iOS và Android: `FontSize`, `FontWeight`, `FontFamily` và `FontStyle`. Các trường không được đặt giữ giá trị mặc định của từng nền tảng; `HeaderTitleStyleFunc` và `HeaderLargeTitleStyleFunc` bám theo các thay đổi. `HeaderStyle` vẫn cung cấp màu nền, màu tiêu đề và màu nhấn của nút. Trong TSX, đây là các đối tượng hoặc mảng style `headerTitleStyle` và `headerLargeTitleStyle`.

Một `HeaderTitle` tùy chỉnh chỉ thay thế tiêu đề thu gọn trên Android; `Title` vẫn là tiêu đề lớn ở dạng mở rộng. Đặt `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` để đặt `HeaderRight` cạnh tiêu đề lớn đó. Các điều khiển của nó di chuyển vào thanh công cụ được ghim khi tiêu đề thu gọn và vẫn bấm được trong suốt quá trình. TSX dùng `android={{ headerRightWithLargeTitle: true }}`. Dải tiêu đề lớn sẽ cao lên khi phông chữ cần thêm chỗ. Các bản build Android độc lập bật bố cục RTL gốc; một host nhúng phải đặt `android:supportsRtl="true"` trên application của nó để các header gốc theo đúng hướng của ứng dụng.

Cung cấp cho một trang `Header` riêng để thay thế `ui.NavHeader`. Trên iOS và Android, nó trở thành title view của header hệ thống, thay cho `HeaderTitle`, nằm cạnh `HeaderLeft`, `HeaderRight` của trang và nút quay lại; nó lấp đầy khoảng trống giữa hai đầu đó khi các view của nó giãn ra hoặc nhận chiều rộng theo phần trăm, chẳng hạn một trường tìm kiếm, và lấy kích thước theo nội dung trong các trường hợp khác. Ở những nơi khác, nó được vẽ phía trên nội dung. `ui.NavHeader.Scroll` vẫn kết nối thao tác cuộn của một trang iOS hoặc Android với hiệu ứng làm mờ của header.

`NoPopOnBack` giữ lại một trang khi nút quay lại được nhấn, `OnBackPress` nhận thao tác nhấn đó và `NoPopOnSwipe` tắt thao tác vuốt để quay lại của iOS. `cx.OnBackPress(func() bool)` cung cấp cho ứng dụng một trình xử lý chạy trước ngăn xếp, dành cho các thay đổi chưa lưu. `OnDismiss` báo rằng người dùng đã pop trang, và ngăn xếp sẽ gỡ nó.

## Toàn màn hình nhập vai

Gọi `mobile.Immersive(cx, true)` để yêu cầu chế độ toàn màn hình cho cửa sổ và `mobile.Immersive(cx, false)` để trở về các cài đặt thanh hệ thống thông thường của nó. Với một trang hoặc hộp thoại, dùng `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` để yêu cầu chế độ này khi lớp đó đang hiển thị. Trong TSX, dùng `statusBar={{ immersive: true }}` trên một trang hoặc hộp thoại, hoặc `status_bar: { immersive: true }` trong `nsgo.yaml` làm mặc định cho cửa sổ.

Trên Android, thao tác vuốt từ cạnh sẽ hiện thanh trạng thái và thanh điều hướng trong chốc lát. Trên HarmonyOS, cửa sổ yêu cầu hệ thống ẩn thanh trạng thái, thanh điều hướng ba nút và chỉ báo điều hướng. Trên iOS, thanh trạng thái bị ẩn và UIKit quyết định khi nào ẩn hoặc hiện chỉ báo Home. `cx.Env().SafeArea()` tiếp tục báo cáo các khoảng lề mà hệ thống cung cấp, bao gồm cả phần khuyết của màn hình. Trên HarmonyOS với điều hướng ba nút ở chế độ ngang, hệ thống báo vùng của các nút nằm dọc cạnh dưới trong khi lại vẽ chúng dọc một cạnh bên, nên vùng an toàn có thể chừa chỗ ở phía dưới trong khi các nút che mất cạnh bên.

## Modal

`ui.Dialog` hiển thị một bề mặt phía trên ứng dụng đã được làm tối, dưới dạng một sheet từ dưới lên trên màn hình cảm ứng, và `ui.Sheet` hiển thị một bảng từ dưới lên. Mỗi loại đều đóng khi nhấn Escape và khi nhấn vào phần nền phía sau, trừ khi được chỉ định khác.

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` yêu cầu người dùng xác nhận, và `cx.ShowAlert` hiển thị hộp cảnh báo hoặc action sheet riêng của hệ thống ([Dialog](ui/dialog.md)).

## Menu

`ui.Menu` mở các `ui.MenuItem` từ một nút: ngay cạnh nút, hoặc dưới dạng một sheet từ dưới lên khi `Presentation` của nó yêu cầu, trên mọi thiết bị. `ui.ContextMenu` làm điều tương tự cho cú nhấp chuột phụ hoặc nhấn giữ, còn `ui.Popover`, `ui.HoverCard` và `ui.Tooltip` hiển thị các nội dung neo khác.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Thẻ

`ui.Tabs[T]` là một điều khiển phân đoạn (segmented control) hoặc một danh sách thẻ, với chỉ báo trượt tới tùy chọn được chọn; `TabStyle` được gộp đè lên style của mỗi thẻ, `Selected` của nó gộp đè lên style của thẻ được chọn, và `IndicatorStyle` gộp đè lên style của chỉ báo. Các thẻ kiểu gạch chân có thể đặt `IndicatorColor` và làm đường gạch vừa với nội dung bằng `IndicatorFit: TabsIndicatorContent`. `ui.Sidebar` chứa các phần và hàng cho thanh bên trên desktop, và `ui.TabShell` giữ các phần cấp cao nhất bên cạnh một thanh tab (mặc định ở phía dưới, hoặc theo chiều dọc ở cạnh đầu hoặc cạnh cuối với `Edge` / `EdgeFunc`, bao gồm `mobile.TabBarAutomatic` để bám theo `cx.Env().VerticalBarEdge()`) ở mọi chiều rộng cửa sổ. Nó dùng thanh của hệ thống khi mọi thẻ đều có `SystemIcon` hoặc một `Icon` SVG, ở bất kỳ cạnh nào trên iOS và ở cạnh dưới trên Android, hiển thị SVG ở những chỗ `SystemIcon` không đặt tên một SF Symbol trên iOS hay một drawable trên Android; nếu không, nó tự vẽ thanh với các biểu tượng SVG. Đặt `BarHidden` khi một trang chi tiết được push đang hiển thị để ẩn thanh và khôi phục vùng an toàn của phần đó; `TabBarInset` trở thành 0 khi thanh bị ẩn hoặc được đặt theo chiều dọc.

`mobile.TabBar` là thanh tab của hệ thống trên iOS và Android, với giao diện Liquid Glass hoặc Material; nó nhận `Items` gồm các `mobile.TabBarItem{Title, Icon, SVG}`, chỉ số được chọn, `OnChange`, và một `Edge` tùy chọn (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing`, hoặc `mobile.TabBarAutomatic`, giá trị này bám theo `cx.Env().VerticalBarEdge()` trên iOS 27.1 và quay về cạnh dưới ở những nơi khác) hoặc `EdgeFunc` phản ứng. Các vai trò bảng màu của Android `tabBarBackground`, `tabBarIdle`, `tabBarSelected` và `tabBarIndicator` lần lượt đặt màu nền của thanh, màu biểu tượng và nhãn ở trạng thái nghỉ, màu biểu tượng và nhãn được chọn, và viên thuốc (pill) đánh dấu mục được chọn. Bảng màu tùy chỉnh lần lượt quay về `Surface`, `TextMuted`, `Accent` và `accentContainer`, với viên thuốc màu nhấn 12% khi `accentContainer` không được đặt. Bảng màu mặc định giữ màu nền và màu trạng thái nghỉ của Material, với thẻ được chọn được tô theo màu nhấn. Ở những nơi khác nó không chiếm chỗ, nên ứng dụng hiển thị `ui.Tabs` ở đó. `mobile.IsAvailable(cx)` cho biết thanh có hiển thị hay không. `Icon` đặt tên một biểu tượng của nền tảng; `SVG`, chẳng hạn `lucide.House()`, hiển thị trên iOS và Android ở những chỗ `Icon` không đặt tên một SF Symbol hay drawable, và được thanh tô màu giống như một biểu tượng hệ thống.

Đặt `TabBarItem.Badge` thành `mobile.TabBadge{Text: "3"}` để hiển thị văn bản hoặc `mobile.TabBadge{Dot: true}` để hiển thị một chấm. Cập nhật nó thông qua `ItemsFunc`, và trả về giá trị zero để xóa nó. `ui.ShellTab.Badge` nhận `func() mobile.TabBadge` cho cả thanh hệ thống lẫn thanh tự vẽ. Văn bản được ưu tiên khi cả hai trường đều được đặt. UIKit hiển thị một badge gốc trống cho dạng chấm. Công nghệ hỗ trợ đọc văn bản của badge hoặc từ đã được bản địa hóa cho nội dung mới. TSX nhận `badge: { text: "3" }` hoặc `badge: { dot: true }` trên một mục gốc, và một hàm trả về giá trị đó trên một mục `TabShell`.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Tiếp theo

- [Biểu mẫu](forms.md): những gì một trang của ngăn xếp chứa.
- [Hoạt ảnh](animation.md): cách các trang và modal di chuyển.
- [Bố cục](layout.md): vùng an toàn xung quanh một header.
