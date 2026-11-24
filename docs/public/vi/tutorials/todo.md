---
title: "Ví dụ: Ứng dụng việc cần làm"
description: Xây dựng từng bước một ứng dụng việc cần làm nhỏ, với signal, một danh sách và một bài kiểm thử.
group: Bắt đầu
order: 11
---

Hướng dẫn này xây dựng một ứng dụng việc cần làm nhỏ: thêm mục, đánh dấu hoàn thành và xóa chúng. Mất khoảng mười phút và giả định bạn đã hoàn tất phần [cài đặt](../getting-started.md).

## Tạo ứng dụng

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Xây dựng ứng dụng việc cần làm

Ứng dụng được tạo sẵn là một bộ đếm. Thay toàn bộ nội dung `app.go` bằng ứng dụng này:

```go program height=480
package main

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type todo struct {
	id   int
	text string
	done bool
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	items := core.NewSignalFunc(cx, []todo{}, nil)
	draft := core.NewSignal(cx, "")
	nextID := core.NewSignal(cx, 1)
	colors := ui.ColorsOf(cx)

	add := func() {
		text := strings.TrimSpace(draft.Get())
		if text == "" {
			return
		}
		id := nextID.Get()
		nextID.Set(id + 1)
		items.Update(func(list []todo) []todo {
			return append(list, todo{id: id, text: text})
		})
		draft.Set("")
	}
	setDone := func(id int, done bool) {
		items.Update(func(list []todo) []todo {
			next := append([]todo(nil), list...)
			for i := range next {
				if next[i].id == id {
					next[i].done = done
				}
			}
			return next
		})
	}
	remove := func(id int) {
		items.Update(func(list []todo) []todo {
			out := list[:0:0]
			for _, it := range list {
				if it.id != id {
					out = append(out, it)
				}
			}
			return out
		})
	}
	left := func() int {
		n := 0
		for _, it := range items.Get() {
			if !it.done {
				n++
			}
		}
		return n
	}

	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(24), PaddingEnv: core.EnvAll(core.EnvSafeArea), Gap: 16},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Background()}
		},
		Children: []core.Node{
			ui.Text("Todos", 28, ui.Bold, colors.Text),
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "What needs doing?",
					OnSubmit: func(string) { add() }, TestID: "draft", Style: core.Style{FlexGrow: 1}},
				ui.Button{Text: "Add", Icon: lucide.Plus, OnPress: add, TestID: "add"},
			}},
			ui.Textf(func() string { return fmt.Sprintf("%d left", left()) }, 13, ui.Regular, colors.Muted),
			core.List{
				Count:         func() int { return len(items.Get()) },
				Key:           func(i int) any { return items.Get()[i].id },
				EstimatedSize: 44,
				Style:         core.Style{FlexGrow: 1},
				Empty:         ui.Text("Nothing yet. Add one above.", 13, ui.Regular, colors.Muted),
				Row: func(cx *core.Scope, row core.ListRow) core.Node {
					index := row.Index()
					it := items.Get()[index]
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(8, 0)},
						Children: []core.Node{
							ui.Checkbox{
								Checked: func() bool {
									for _, x := range items.Get() {
										if x.id == it.id {
											return x.done
										}
									}
									return false
								},
								OnChange: func(done bool) { setDone(it.id, done) },
								Text:     it.text,
								TestID:   fmt.Sprintf("todo-%d", it.id),
								Style:    core.Style{FlexGrow: 1, FlexShrink: 1},
							},
							ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost,
								TestID: fmt.Sprintf("delete-%d", it.id), OnPress: func() { remove(it.id) }},
						}}
				},
			},
		},
	}
}
```

Lưu tệp và ứng dụng sẽ khởi động lại. Gõ một việc cần làm, nhấn **Add** và đánh dấu hoàn thành.

## Cách hoạt động

**Trạng thái là signal.** `items`, `draft` và `nextID` là các signal. Một signal giữ một giá trị và theo dõi những gì đọc nó: khi `items` thay đổi, chỉ phần văn bản và các hàng đọc nó mới được cập nhật. `core.NewSignal` so sánh giá trị bằng `==`, nên một slice dùng `core.NewSignalFunc` không kèm hàm so sánh bằng.

**Cập nhật thay thế slice.** `items.Update` trả về một slice mới, nên danh tính của danh sách và khóa của các hàng quyết định những gì còn lại trên màn hình. `nextID` cấp cho mỗi việc cần làm một ID, và `Key` của `core.List` dùng ID đó; nếu không có khóa ổn định, các hàng sẽ dùng lại ô của chúng cho những mục khác.

**Màn hình là một cây các hộp.** `Root` trả về một `core.View` có padding, khoảng cách (gap) và phần lề của vùng an toàn. `FlexGrow: 1` và `FlexShrink: 1` khiến nó lấp đầy cửa sổ và không hơn, nên một danh sách dài sẽ cuộn bên trong nó thay vì kéo giãn nó ra ngoài mép cửa sổ. Hàng nhập liệu đặt `Direction: core.Row`; mọi thứ khác là cột mặc định.

**Ô nhập liệu được kiểm soát.** `draft.Get` là giá trị của trường và `draft.Set` nhận mỗi thay đổi. `OnSubmit` thêm một việc khi nhấn Return, và `add` xóa bản nháp, nên trường tự làm trống.

**Danh sách chỉ dựng những gì hiển thị.** `core.List` yêu cầu `Count` hàng và chỉ dựng một hàng khi nó tiến gần tới màn hình. `Empty` hiển thị khi chưa có mục nào, và hộp kiểm cùng nút xóa của mỗi hàng mang ID của việc cần làm tương ứng, nên một lần nhấn vẫn tìm đúng mục sau khi danh sách thay đổi.

**Giao diện là của `ui`.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox` và `ui.IconButton` lấy màu từ chủ đề và tự động theo chế độ sáng và tối. `ui.ColorsOf(cx)` cung cấp màu cho những phần mà ứng dụng tự vẽ, và một `StyleFunc` bám theo việc đổi chủ đề ngay tại chỗ.

## Kiểm thử

Bài kiểm thử được tạo sẵn nhấn nút của bộ đếm, nên hãy thay `app_test.go` bằng một bài kiểm thử cho ứng dụng việc cần làm:

```go
package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestAddCompleteAndDelete(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:Nothing yet. Add one above.")
	app.Type("id:draft", "Buy milk")
	app.Press("id:add")
	app.WaitFor("text:Buy milk", time.Second)
	app.Press("id:todo-1")
	app.WaitFor("text:0 left", time.Second)
	app.Press("id:delete-1")
	app.WaitFor("text:Nothing yet. Add one above.", time.Second)
}
```

Bài kiểm thử khởi động ứng dụng ở chế độ headless, tìm các phần tử theo văn bản hoặc test ID của chúng, và thao tác như một người dùng. Chạy nó bằng:

```sh
go tool nsgo test
```

`TestMain` trao luồng chính cho nền tảng khi các bài kiểm thử chạy trên một nền tảng đích, và `go tool nsgo test -target ios_simulator` chạy cùng bài kiểm thử đó trong trình giả lập với cơ chế nhập liệu gốc của nó.

## Phát hành

```sh
go tool nsgo build
```

`build` ghi một gói phát hành cho mỗi nền tảng đích mà `nsgo.yaml` liệt kê, vào `dist/<name>-<version>-<target>`: một `.app`, một `.ipa`, một `.apk`, các thư mục của Windows và Linux, hoặc một trang web. Mọi thứ để phân phối cũng được đặt vào `dist/releases`: ảnh đĩa, chương trình cài đặt, các gói Debian và Arch, IPA, APK, trang web, mini program, và các script cài ứng dụng từ các bản phát hành GitHub của bạn. [Xây dựng và đóng gói phát hành](../building.md) trình bày về trình cài đặt, ký và công chứng (notarization), xuất bản, biểu tượng, widget, App Clip và App Store.

## Tiếp theo

- [Ví dụ: Ứng dụng ghi chú](notes.md) và [Ví dụ: Các kho GitHub phổ biến](repos.md): thêm hai ứng dụng nữa, từng bước một.
- [Thành phần và trạng thái](../components.md): phần tử, thành phần, signal và vùng (region) một cách sâu hơn.
- [Bố cục](../layout.md) và [Kiểu dáng và chủ đề](../styles.md): cấu trúc và giao diện của màn hình.
- [Danh sách và cuộn](../lists.md): danh sách dài, tiêu đề dính và tải thêm.
- [Kiểm thử](../testing.md): tìm điều khiển, nhấn chúng và chụp ảnh màn hình.
- [Nhúng ứng dụng](../embedding.md): đặt ứng dụng vào bên trong một ứng dụng khác.
