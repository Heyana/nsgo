---
title: "例：Todo アプリ"
description: シグナル、リスト、テストを使いながら、小さな Todo アプリを順を追って構築します。
group: はじめに
order: 11
---

このチュートリアルでは、項目の追加、チェックによる完了、削除ができる小さな Todo アプリを作成します。所要時間は約 10 分で、[セットアップ](../getting-started.md)が完了していることを前提としています。

## アプリを作成する

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Todo アプリを構築する

生成されたアプリはカウンターです。`app.go` 全体を次のアプリに置き換えます：

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

ファイルを保存するとアプリが再起動します。タスクを入力して **Add** を押し、チェックを入れてみてください。

## 仕組み

**状態はシグナルで管理します。** `items`、`draft`、`nextID` はシグナルです。シグナルは値を保持し、それを読み取る箇所を追跡します。`items` が変化すると、それを読み取っているテキストと行だけが更新されます。`core.NewSignal` は `==` で値を比較するため、スライスには等価比較関数を指定しない `core.NewSignalFunc` を使用します。

**更新ではスライスを置き換えます。** `items.Update` は新しいスライスを返すため、リストの識別情報と行のキーによって画面上に残るものが決まります。`nextID` は各 Todo に ID を割り当て、`core.List` の `Key` がそれを使用します。安定したキーがないと、行は別の項目に対してセルを再利用してしまいます。

**画面はボックスのツリーです。** `Root` は、パディング、ギャップ、セーフエリアのインセットを持つ `core.View` を返します。`FlexGrow: 1` と `FlexShrink: 1` によりウィンドウいっぱいに広がりつつそれ以上は大きくならないため、長いリストがウィンドウの端を超えて引き伸ばされるのではなく、その中でスクロールするようになります。入力行には `Direction: core.Row` を設定しており、それ以外はすべてデフォルトの縦方向（column）です。

**入力は制御されています。** `draft.Get` がフィールドの値となり、`draft.Set` が変更を受け取ります。Return キーが押されると `OnSubmit` がタスクを追加し、`add` が下書きをクリアするため、フィールドは自動的に空になります。

**リストは表示される分だけを構築します。** `core.List` は `Count` で行数を取得し、画面に近づいた行だけを構築します。項目がない間は `Empty` が表示され、各行のチェックボックスと削除ボタンはそれぞれの Todo の ID を保持しているため、リストが変化した後でも押したときに正しい項目を参照できます。

**見た目は `ui` が提供します。** `ui.Text`、`ui.Input`、`ui.Button`、`ui.Checkbox`、`ui.IconButton` はテーマから色を取得し、ライトモードとダークモードに自動的に追従します。`ui.ColorsOf(cx)` はアプリが独自に描画する要素に色を提供し、`StyleFunc` はテーマの変更にその場で追従します。

## テストする

生成されたテストはカウンターのボタンを押す内容になっているため、`app_test.go` を Todo アプリのテストに置き換えます：

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

このテストはアプリをヘッドレスで起動し、テキストやテスト ID で要素を見つけて、ユーザーと同じように操作します。次のコマンドで実行します：

```sh
go tool nsgo test
```

ターゲット上でテストを実行する際、`TestMain` はプラットフォームにメインスレッドを渡し、`go tool nsgo test -target ios_simulator` はネイティブ入力を使ってシミュレータ上で同じテストを実行します。

## 出荷する

```sh
go tool nsgo build
```

`build` は `nsgo.yaml` に記載された各ターゲット向けのリリース用パッケージを `dist/<name>-<version>-<target>` に出力します（`.app`、`.ipa`、`.apk`、Windows や Linux のディレクトリ、Web サイトなど）。配布に必要なもの一式も `dist/releases` にまとめられます。ディスクイメージ、セットアッププログラム、Debian および Arch パッケージ、IPA、APK、Web サイト、ミニプログラム、GitHub リリースからアプリをインストールするスクリプトなどが含まれます。[ビルドと配信](../building.md)では、インストーラー、署名と公証、公開、アイコン、ウィジェット、App Clip、App Store について解説しています。

## 次のステップ

- [例：メモアプリ](notes.md) と [例：人気の GitHub リポジトリ](repos.md)：さらに 2 つのアプリを順を追って作成します。
- [コンポーネントと状態](../components.md)：要素、コンポーネント、シグナル、リージョンをより詳しく解説します。
- [レイアウト](../layout.md) と [スタイルとテーマ](../styles.md)：画面の構造と見た目について解説します。
- [リストとスクロール](../lists.md)：長いリスト、スティッキーヘッダー、追加読み込み。
- [テスト](../testing.md)：コントロールの検索、押下、スクリーンショットの撮影。
- [埋め込み](../embedding.md)：別のアプリの中に組み込む方法。
