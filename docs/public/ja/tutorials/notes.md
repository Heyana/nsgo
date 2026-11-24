---
title: "例：メモアプリ"
description: ナビゲーションスタック、複数行エディタ、デバイスに保存されるメモを備えたメモアプリを構築します。
group: はじめに
order: 12
---

このチュートリアルでは、メモの一覧、メモを編集するページ、そしてアプリを再起動しても残るメモを備えたメモアプリを作成します。[セットアップ](../getting-started.md)が完了していることを前提としています。

## アプリを作成する

```sh
mkdir notes && cd notes
go mod init example.com/notes
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## アプリを書く

`app.go` 全体を次のコードで置き換えます：

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

**+** を押してメモを追加し、入力してみてください。1 行目が一覧のタイトルになります。アプリを終了して再起動しても、メモはそのまま残っています。

## 仕組み

**ページのスタック。** `base.NewNav(cx, 0)` はルートのスタックを保持します。ここではメモの ID を使い、一覧を `0` としています。`nav.Stack` は各ルートに対してページを構築し、その下にあるページもマウントしたまま維持するため、メモを開いている間も一覧のスクロール位置が保たれます。`Push` でメモを開き、`Pop` で戻ります。

**ヘッダー付きのページ。** `ui.NavPage` はページに `ui` の背景と、タイトル・戻るボタン・各種ボタンを備えた `ui.NavHeader` を与えます。iOS と Android ではシステムが独自の戻るジェスチャーやトランジションとともにヘッダーを描画し、それ以外の環境では `ui` が描画します。

**デバイス上のストレージ。** `cx.Storage()` はアプリのキー・バリューストレージです。`Root` は起動時にメモを一度読み込み、`save` はメモが変更されるたびに JSON として書き込みます。ストレージは文字列を保持します。アプリが書き込むファイルは `cx.DataDir()` に配置します。

**単一の情報源。** すべてのページが同じ `notes` シグナルを参照します。エディタの `Value` がメモを読み取り、`OnChange` が保存し、ヘッダーの `TitleFunc` と一覧の行も同じシグナルを追従するため、入力に合わせてリアルタイムに更新されます。

## テストする

`app_test.go` を置き換えます。`nsgotest.Storage` は、まるで以前実行されたかのようにストレージに値が書き込まれた状態でアプリを起動します：

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

## 次のステップ

- [例：人気の GitHub リポジトリ](repos.md): 画面をブロックせずにネットワークからデータを取得します。
- [ナビゲーション](../navigation.md): モーダル、メニュー、タブバー。
- [プラットフォームサービス](../services.md): ファイル、共有、クリップボードなど。
