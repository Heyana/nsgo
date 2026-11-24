---
title: Giới thiệu
description: NSGo là gì, chạy trên những nền tảng nào và hoạt động ra sao.
order: 1
---

NSGo là một framework giao diện gốc đa nền tảng được viết bằng Go. Bạn viết ứng dụng một lần, bằng Go hoặc TSX, và nó chạy trên iOS cùng App Clip của nó, tvOS, watchOS, Android, macOS, Windows, Linux và web, được vẽ bằng chính các view của từng nền tảng. Ứng dụng Go cũng chạy được trong mini program của WeChat, QQ, Alipay và Douyin ở mức thử nghiệm: các nền tảng này giới hạn ứng dụng ở 2 MB, và các ứng dụng lớn hơn sẽ vượt quá. HarmonyOS chạy ứng dụng Go dưới dạng bản xem trước, được vẽ bằng ArkUI.

## Những gì bạn viết

Một ứng dụng là một package main của Go. Các thành phần của nó trả về phần tử: view, văn bản, hình ảnh, ô nhập liệu và các điều khiển trong `ui`. Trạng thái nằm trong signal, và chỉ những gì đọc một signal mới cập nhật khi signal đó thay đổi.

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

Cùng một ứng dụng có thể được viết bằng TSX và tạo kiểu bằng các class Tailwind ([Ứng dụng TSX](tsx.md)).

## Cách hoạt động

Một engine Go sở hữu ứng dụng: đồ thị phản ứng, kiểu dáng, bố cục flexbox và grid, cùng việc chuẩn bị văn bản. Mỗi lần cập nhật tạo ra một lô thay đổi, và một backend mỏng áp dụng nó lên các view gốc của nền tảng. Hoạt ảnh chạy trên đồng hồ khung hình của nền tảng, nên không có gì phải commit mỗi khung hình.

| Nền tảng | Vẽ bằng |
| --- | --- |
| iOS | UIKit và CoreText |
| Android | Android Views |
| macOS | AppKit và CoreText |
| Windows | Direct2D và DirectWrite |
| Linux | GTK 4, Pango và Cairo |
| Web | WebAssembly và các phần tử DOM |

Mọi nền tảng đích đều build được trên máy Mac có Go; iOS cần thêm Xcode. Công cụ `nsgo` tạo, chạy, kiểm thử và đóng gói ứng dụng.

## Trạng thái

NSGo còn ở giai đoạn đầu: 0.1.0 là bản phát hành đầu tiên, API của nó chưa ổn định, và những gì còn thiếu nằm trong [lộ trình](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Nó chuyển thiết kế của [Native Shell](https://github.com/longbridge/nsh) sang Go.

## Cách đọc tài liệu

Mỗi trang trả lời một câu hỏi. Một ví dụ mã là một đoạn của một thành phần, trừ khi trang nói khác: `cx` là `*core.Scope` mà thành phần nhận được. Ứng dụng dùng các thành phần của `ui`, vốn có giao diện sẵn; `base` chứa cùng các điều khiển đó nhưng không có giao diện, dành cho tác giả viết thành phần.

- **Bắt đầu**: [cài đặt](getting-started.md), sau đó là các ứng dụng ví dụ được xây dựng từng bước, [ứng dụng việc cần làm](tutorials/todo.md), [ứng dụng ghi chú](tutorials/notes.md) và [các kho GitHub phổ biến](tutorials/repos.md).
- **Bộ công cụ**: [chạy và gỡ lỗi](debugging.md), [kiểm thử](testing.md), [build và phát hành](building.md) và [nhúng](embedding.md).
- **Thành phần**: [các thành phần dùng nhiều nhất](ui.md), cùng biến thể, trạng thái và khả năng truy cập của chúng.
- **Khái niệm cốt lõi**: [thành phần và trạng thái](components.md), [bố cục](layout.md), [kiểu dáng](styles.md), [văn bản](text.md), [danh sách](lists.md), [điều hướng](navigation.md), [biểu mẫu](forms.md), [bản địa hóa](localization.md), [TSX](tsx.md) và các mục khác.
- **Tính năng nền tảng**: [dịch vụ nền tảng](services.md), [thông báo](notifications.md), [phản hồi xúc giác](haptics.md), [widget](widgets.md), [Live Activities](live-activities.md) và [quyền truy cập](permissions.md).
- **Viết thành phần**: [thành phần cho ứng dụng của người khác](authoring/index.md), không có kiểu dáng và có kiểu dáng.
- **Đóng góp**: [build NSGo từ mã nguồn](contributing.md), tìm hiểu cấu trúc của nó và gửi một thay đổi.
- [Bản demo trực tiếp](https://nsgo.dev/gallery/): mọi thành phần, ngay trong trình duyệt của bạn.
