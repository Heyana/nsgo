---
title: "Ví dụ: Ứng dụng ghi chú"
description: Xây dựng một ứng dụng ghi chú với ngăn xếp điều hướng, trình soạn thảo nhiều dòng và các ghi chú được lưu trên thiết bị.
group: Bắt đầu
order: 12
---

Hướng dẫn này xây dựng một ứng dụng ghi chú: một danh sách ghi chú, một trang để chỉnh sửa từng ghi chú, và các ghi chú vẫn còn nguyên khi ứng dụng khởi động lại. Hướng dẫn giả định bạn đã hoàn tất phần [cài đặt](../getting-started.md).

## Tạo ứng dụng

```sh
mkdir notes && cd notes
go mod init example.com/notes
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Viết ứng dụng

Thay toàn bộ nội dung `app.go`:

```go program height=480
package main

import (
	"encoding/json"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type note struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// title is a note's first line.
func (n note) title() string {
	line, _, _ := strings.Cut(strings.TrimSpace(n.Text), "\n")
	if line == "" {
		return "New note"
	}
	return line
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	store := cx.Storage()
	var saved []note
	if data, ok := store.GetItem("notes"); ok {
		json.Unmarshal([]byte(data), &saved)
	}
	notes := core.NewSignalFunc(cx, saved, nil)
	save := func(list []note) {
		notes.Set(list)
		data, _ := json.Marshal(list)
		store.SetItem("notes", string(data))
	}

	// A route is the ID of the note it edits, and 0 the list.
	nav := base.NewNav(cx, 0)

	find := func(id int) note {
		for _, n := range notes.Get() {
			if n.ID == id {
				return n
			}
		}
		return note{ID: id}
	}
	add := func() {
		id := 1
		for _, n := range notes.Get() {
			id = max(id, n.ID+1)
		}
		save(append([]note{{ID: id}}, notes.Get()...))
		nav.Push(id)
	}
	edit := func(id int, text string) {
		list := append([]note(nil), notes.Get()...)
		for i := range list {
			if list[i].ID == id {
				list[i].Text = text
			}
		}
		save(list)
	}
	remove := func(id int) {
		var list []note
		for _, n := range notes.Get() {
			if n.ID != id {
				list = append(list, n)
			}
		}
		save(list)
		nav.Pop(1)
	}

	return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, id int, _ func() int) base.NavPage {
		if id == 0 {
			return listPage(cx, notes.Get, nav.Push, add)
		}
		return ui.NavPage(cx, base.NavPage{
			TitleFunc: func() string { return find(id).title() },
			Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(16)}, Children: []core.Node{
				ui.Input{
					Multiline:   true,
					Value:       func() string { return find(id).Text },
					OnChange:    func(text string) { edit(id, text) },
					Placeholder: "Write something",
					TestID:      "editor",
					Style:       core.Style{FlexGrow: 1, FlexShrink: 1},
				},
			}}},
		}, ui.NavHeader{
			OnBack:    func() { nav.Pop(1) },
			BackLabel: "Notes",
			Trailing: []core.Node{ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost,
				TestID: "delete", OnPress: func() { remove(id) }}},
		})
	})
}

func listPage(cx *core.Scope, notes func() []note, open func(int), add func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	return ui.NavPage(cx, base.NavPage{
		Title: "Notes",
		Children: []core.Node{core.List{
			Count:         func() int { return len(notes()) },
			Key:           func(i int) any { return notes()[i].ID },
			EstimatedSize: 48,
			Style:         core.Style{FlexGrow: 1},
			Empty: core.View{Style: core.Style{Padding: core.All(16)}, Children: []core.Node{
				ui.Text("No notes yet.", 13, ui.Regular, colors.Muted),
			}},
			Row: func(cx *core.Scope, row core.ListRow) core.Node {
				n := notes()[row.Index()]
				return core.View{
					Role:    core.RoleButton,
					OnPress: func(core.PressEvent) { open(n.ID) },
					Style:   core.Style{Padding: core.Symmetric(12, 16), Borders: core.Edges{Bottom: 1}},
					StyleFunc: func() core.Style {
						return core.Style{BorderColor: colors.Border()}
					},
					Children: []core.Node{ui.Text(n.title(), 15, ui.Regular, colors.Text)},
				}
			},
		}},
	}, ui.NavHeader{
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Plus, Label: "New note", Variant: ui.IconButtonGhost,
			TestID: "new", OnPress: add}},
	})
}
```

Nhấn **+** để thêm một ghi chú rồi gõ nội dung; dòng đầu tiên sẽ trở thành tiêu đề của ghi chú trong danh sách. Thoát ứng dụng rồi khởi động lại: các ghi chú vẫn còn đó.

## Cách hoạt động

**Một ngăn xếp các trang.** `base.NewNav(cx, 0)` giữ một ngăn xếp các route, ở đây là ID của ghi chú, với `0` dành cho danh sách. `nav.Stack` dựng một trang cho mỗi route và giữ các trang bên dưới vẫn được gắn, nên danh sách giữ nguyên vị trí cuộn trong khi một ghi chú đang mở. `Push` mở một ghi chú và `Pop` quay lại.

**Các trang có thanh tiêu đề.** `ui.NavPage` cho trang nền của `ui` và một `ui.NavHeader` với tiêu đề, nút quay lại và các nút của nó. Trên iOS và Android, hệ thống vẽ thanh tiêu đề, với cử chỉ quay lại và hiệu ứng chuyển trang riêng; ở các nền tảng khác, `ui` tự vẽ nó.

**Lưu trữ trên thiết bị.** `cx.Storage()` là kho lưu trữ khóa-giá trị của ứng dụng. `Root` đọc các ghi chú một lần khi khởi động, và `save` ghi chúng dưới dạng JSON mỗi khi chúng thay đổi. Kho lưu trữ chứa chuỗi; các tệp mà ứng dụng ghi ra được đặt trong `cx.DataDir()`.

**Một nguồn dữ liệu duy nhất.** Mọi trang đều đọc cùng một signal `notes`. `Value` của trình soạn thảo đọc ghi chú, `OnChange` lưu nó, còn `TitleFunc` của thanh tiêu đề và hàng trong danh sách bám theo cùng signal đó, nên chúng cập nhật ngay khi bạn gõ.

## Kiểm thử

Thay `app_test.go`. `nsgotest.Storage` khởi động ứng dụng với kho lưu trữ đã có sẵn dữ liệu, như thể ứng dụng đã từng chạy trước đó:

```go
package main

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestWriteANote(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:No notes yet.")
	app.Press("id:new")
	app.Type("id:editor", "Groceries\nMilk")
	app.WaitFor("role:header Groceries", time.Second)
	if data, _ := app.Storage().GetItem("notes"); !strings.Contains(data, "Milk") {
		t.Fatalf("saved %q", data)
	}
}

func TestSavedNotes(t *testing.T) {
	app := nsgotest.Start(t, Root, nsgotest.Storage(map[string]string{
		"notes": `[{"id":1,"text":"Ideas\nA notes app"}]`,
	}))
	app.Press("text:Ideas")
	app.Press("id:delete")
	app.WaitFor("text:No notes yet.", time.Second)
}
```

```sh
go tool nsgo test
```

## Tiếp theo

- [Ví dụ: Các kho GitHub phổ biến](repos.md): tải dữ liệu từ mạng mà không chặn màn hình.
- [Điều hướng](../navigation.md): modal, menu và thanh tab.
- [Dịch vụ nền tảng](../services.md): tệp, chia sẻ, bảng nhớ tạm và nhiều hơn nữa.
