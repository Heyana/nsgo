---
title: パーツと状態
description: アプリが所有する状態を保持せず、すべてのパーツにスタイルを設定し、支援技術にコンポーネントの種類を伝える。
group: コンポーネントを書く
order: 71
---

スタイルなしコンポーネントは、コントロールの振る舞いを決定し、見た目は利用者に委ねます。このページでは、`base` が各コントロールを構築するのと同じ方法で、オン／オフ切り替え用のピル型コントロール `Toggle` を構築します。

## 制御された値

コントロールはアプリが所有する値を自身で保持しません。アプリが値を関数として渡し、コールバックからその値を更新します。コンポーネントは表示する箇所でその関数を読み取るため、値が変化したときはそのパーツだけが更新されます。

```go
// Toggle is a pill that is on or off. Its parts are the pill and a dot
// inside it, then its children, the label.
type Toggle struct {
	On       func() bool
	Disabled func() bool
	// OnChange receives the value a press asks for, the opposite of On's.
	OnChange func(on bool)

	Class        string
	Style        core.Style
	StyleFunc    func() core.Style
	DotClass     string
	DotStyle     core.Style
	DotStyleFunc func() core.Style
	Children     []core.Node
	Label        string
}
```

アプリは値をシグナルで保持します：`chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`。

## パーツ

コンポーネントが描画する各パーツは、そのパーツ名にちなんだ `Class`、`Style`、`StyleFunc`（`DotClass`、`DotStyle`、`DotStyleFunc`）を受け取ります。`Style` は固定スタイル、`StyleFunc` は読み取ったシグナルに追従するスタイルであり、クラスはその両方の下敷きとして適用されるため、アプリ側は好みの記法を使えます。

パーツは自身の状態を報告するのではなく、`PartStates` を通じてコントロール全体の状態を受け取ります。これにより、支援技術には 1 つのコントロールとして見せながら、`checked:` クラスや `Checked` スタイルをパーツに適用できます。

```go
func (t Toggle) Render(*core.Scope) core.Node {
	on := func() bool { return t.On != nil && t.On() }
	off := func() bool { return t.Disabled != nil && t.Disabled() }
	shows := func() core.PartStates { return core.PartStates{Checked: on(), Disabled: off()} }
	dot := core.View{Class: t.DotClass, Style: t.DotStyle, StyleFunc: t.DotStyleFunc, PartStates: shows}
	return core.View{
		Class: t.Class, Style: t.Style, StyleFunc: t.StyleFunc,
		Role: core.RoleSwitch, Label: t.Label, Checked: on, Disabled: t.Disabled,
		OnPress: func(core.PressEvent) {
			if t.OnChange != nil {
				t.OnChange(!on())
			}
		},
		Children: append([]core.Node{dot}, t.Children...),
	}
}
```

アプリはクラスだけで次のようにスタイル設定できます。

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

同様に `PartStates.Invalid` は、別の無効コントロールとして報告することなく、`aria-invalid:` クラスや `Style.AriaInvalid`（内部の `group-aria-invalid:` スタイルを含む）を装飾パーツに適用します。支援技術にバリデーション状態を伝えるために、`Invalid` は実際のコントロールまたはネイティブ入力要素に設定してください。

## アクセシビリティ

コントロールのルート要素がその種類を宣言します。具体的には `Role`、テキストだけでは伝わらない場合の `Label`、操作結果を示す `Hint`、そして該当する `Checked`、`Selected`、`Disabled`、`Invalid`、`Expanded` を設定します。内部のパーツ自体は沈黙させます。タブリストのように矢印キーで項目間を移動するコントロールは、1 つの Tab 停止点にします。ロールとアクションの一覧は[アクセシビリティ](../accessibility.md)を参照してください。

## そのまま引き継ぐべきプロパティ

ルートが独自のビューであるコンポーネントでは、そのビューの `OnLayout`、`Tooltip`、`TestID` を受け取ってそのまま渡すことで、アプリ側が通常のビューと同じように計測・補足説明・テストを行えるようにします。コールバックは省略可能にします。`OnChange` が `nil` の場合は、値を変更せずに表示だけ行うコントロールとして動作させます。
