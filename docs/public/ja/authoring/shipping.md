---
title: 文言・テスト・公開
description: コンポーネントが発する文言を翻訳し、ユーザーと同じようにテストし、Go および TSX アプリ向けに公開する。
group: コンポーネントを書く
order: 74
---

## 文言

ボタンのスクリーンリーダー向け名称など、コンポーネント自身が発する文言は、文字列リテラルではなくそのパッケージのカタログ経由で出力します。ライブラリ内にソース言語のカタログを置いた `locales/` パッケージと、コードを生成するファイルを用意します。

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

`go generate ./locales` を実行するとメッセージごとに関数が生成され、コンポーネントからスコープを渡して `locales.Clear(cx)` を呼ぶことで、アプリが表示している言語で文言を出力できます。アプリが Go のみでビルドできるよう、生成された `catalog_gen.go` をコミットしてください。翻訳先の言語ごとにファイル（`zh-Hans.yaml` など）を追加します。未収録の言語については、[NSGo 自身の文言](../localization.md#nsgos-own-words)と同様に、アプリ側が自身のカタログ内でパッケージのインポートパスをキーとして翻訳を提供できます。

## テスト

`nsgotest` は、ユーザーと同じ操作方法でコンポーネントをヘッドレス実行します。小さなルートにマウントし、ロールとテキストを指定して操作します。

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

ライブラリのルートで `go tool nsgo test` を実行するとすべてのパッケージがテストされ、`-target macos` などのターゲットを指定すると同じテストをネイティブウィンドウ内で実行できます。ハードコードされた色や文言を検出するため、両方の外観モードおよび擬似ロケール（`-lang en-XA`）でもテストしてください。詳細は[テスト](../testing.md)で解説しています。

## 公開

コンポーネントライブラリは Go モジュールであり、アプリは `go get` で取得してパッケージをインポートします。独自の見た目を持つアプリが使わないスタイルをインポートせずに済むよう、スタイルなし形式とスタイル付き形式の両方を提供する場合は別々のパッケージに分けてください。

サイトの [Components](https://nsgo.dev/components/) ページでは他の開発者が利用できるライブラリを一覧紹介しています。自分のライブラリを掲載するには、`docs/public/components/` にドキュメントとスクリーンショットを追加してください。

`init` 関数内で登録すると、TSX アプリからも Go コンポーネントを利用できるようになります。

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

これにより、`import { Toggle } from "example.com/chips"` と記述し、フィールドを props として渡せるようになります（[TSX アプリ](../tsx.md#go-from-tsx)）。
