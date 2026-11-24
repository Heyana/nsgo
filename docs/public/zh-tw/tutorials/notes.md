---
title: "範例：筆記應用程式"
description: 建置一個具備導覽堆疊、多行編輯器，以及將筆記儲存在裝置上的筆記應用程式。
group: 快速開始
order: 12
---

本教學將建置一個筆記應用程式：包含筆記清單、編輯單則筆記的頁面，以及在應用程式重新啟動後依然保留的筆記資料。本教學假設你已完成[環境設定](../getting-started.md)。

## 建立應用程式

```sh
mkdir notes && cd notes
go mod init example.com/notes
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## 撰寫應用程式

將 `app.go` 的全部內容替換為：

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

按下 **+** 新增筆記並輸入內容；第一行文字會成為它在清單中的標題。結束應用程式並重新啟動後，筆記依然會保留在原地。

## 運作原理

**頁面堆疊。** `base.NewNav(cx, 0)` 會維護一個路由堆疊，這裡使用筆記 ID 作為路由，並以 `0` 代表筆記清單。`nav.Stack` 會為每個路由建構頁面並保持下方頁面處於掛載狀態，因此在開啟筆記時，清單仍會保留其捲動位置。`Push` 用於開啟筆記，而 `Pop` 則返回上一頁。

**附帶標題列的頁面。** `ui.NavPage` 會為頁面提供 `ui` 的背景與包含標題、返回按鈕及操作按鈕的 `ui.NavHeader`。在 iOS 與 Android 上，系統會繪製該標題列並附帶原生的返回手勢與轉場效果；在其他平台上則由 `ui` 自行繪製。

**裝置上的儲存空間。** `cx.Storage()` 是應用程式的鍵值儲存空間。`Root` 會在啟動時讀取一次筆記，而 `save` 會在筆記發生變更時將其以 JSON 格式寫入。儲存空間保存的是字串；應用程式寫入的檔案則應放在 `cx.DataDir()` 中。

**單一真實資料來源。** 每個頁面都讀取同一個 `notes` 訊號。編輯器的 `Value` 讀取筆記、`OnChange` 儲存筆記，標題列的 `TitleFunc` 與清單的列項目也都跟隨同一個訊號，因此會在你輸入時即時更新。

## 執行測試

替換 `app_test.go` 的內容。`nsgotest.Storage` 會在啟動應用程式時預先寫入儲存空間資料，模擬應用程式先前已執行過的情境：

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

## 下一步

- [範例：熱門 GitHub 儲存庫](repos.md)：從網路擷取資料而不阻塞畫面。
- [導覽](../navigation.md)：強制回應視窗、選單與分頁列。
- [平台服務](../services.md)：檔案、分享、剪貼簿與更多功能。
