---
title: Từ ngữ, kiểm thử và xuất bản
description: Dịch nội dung hiển thị của thành phần, kiểm thử như người dùng thực thụ, và cung cấp cho ứng dụng Go lẫn TSX.
group: Viết thành phần
order: 74
---

## Từ ngữ

Thành phần tự hiển thị các từ ngữ của riêng mình — chẳng hạn tên nút bấm cho trình đọc màn hình — thông qua các danh mục (catalog) của gói thay vì chuỗi ký tự viết cứng. Hãy đặt một gói `locales/` trong thư viện cùng danh mục của ngôn ngữ nguồn và một tệp sinh mã nguồn:

```yaml
# locales/en.yaml
clear:
  message: Clear
  description: What screen readers say of the button that empties a chip input.
```

```go
// Package locales says the chips' words in the language the app shows.
package locales

//go:generate go tool nsgo i18n
```

`go generate ./locales` sinh ra một hàm cho mỗi thông điệp, và thành phần chỉ cần gọi hàm đó kèm scope của mình: `locales.Clear(cx)` sẽ đọc lên nội dung theo đúng ngôn ngữ ứng dụng đang hiển thị. Hãy commit tệp `catalog_gen.go` đã sinh để các ứng dụng có thể build chỉ với Go. Thêm một tệp cho mỗi ngôn ngữ bạn dịch sang (`zh-Hans.yaml`); ứng dụng có thể bổ sung các ngôn ngữ khác trong danh mục riêng của nó, dưới một khóa mang đường dẫn import của gói bạn, giống như với [từ ngữ riêng của NSGo](../localization.md#nsgos-own-words).

## Kiểm thử

`nsgotest` chạy thành phần đúng theo cách người dùng thao tác, ở chế độ không giao diện (headless). Hãy gắn thành phần vào một gốc nhỏ và tương tác theo vai trò và văn bản:

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestToggle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return chips.Toggle{On: on.Get, OnChange: on.Set, Label: "Bold"}
	})
	app.Press("role:switch Bold")
	if !app.Find("role:switch Bold").Props.Checked {
		t.Fatal("the toggle did not turn on")
	}
}
```

Từ thư mục gốc của thư viện, `go tool nsgo test` chạy toàn bộ các gói, và `-target macos` hoặc một nền tảng đích khác chạy chính các bài kiểm thử đó trong cửa sổ gốc. Hãy kiểm thử cả hai giao diện sáng/tối và một ngôn ngữ giả (`-lang en-XA`) để phát hiện sớm màu sắc hoặc từ ngữ viết cứng. [Kiểm thử](../testing.md) trình bày phần còn lại.

## Xuất bản

Một thư viện thành phần là một Go module: các ứng dụng `go get` thư viện đó và import các gói của nó. Hãy tách riêng dạng không kiểu dáng và dạng có kiểu dáng thành hai gói khác nhau khi cả hai cùng tồn tại, để một ứng dụng có giao diện riêng không phải kéo theo những kiểu dáng mà nó không sử dụng.

Trang [Thành phần](https://nsgo.dev/components/) trên website liệt kê các thư viện để mọi người cùng dùng; hãy thêm một tài liệu và ảnh chụp màn hình vào `docs/public/components/` để giới thiệu thư viện của bạn.

Ứng dụng TSX có thể gọi tới các thành phần Go sau khi chúng được đăng ký trong hàm `init`:

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

Khi đó `import { Toggle } from "example.com/chips"` sẽ nhận các trường cấu trúc dưới dạng props ([Ứng dụng TSX](../tsx.md#go-from-tsx)).
