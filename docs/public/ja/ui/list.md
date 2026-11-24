---
title: リスト
description: 任意の数の行を表示領域分だけ構築し、選択とアクティベーションに対応するコンポーネント。
group: コンポーネント
order: 38
---

`ui.List` はネイティブのスクロールビュー上に行を描画し、画面付近の行だけを構築しながら、選択、アクティベーション、およびキーボード操作に対応します。

```go preview=list/basic height=280
ui.List{
	Count:         func() int { return len(items) },
	Key:           func(i int) any { return items[i].ID },
	EstimatedSize: 44,
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		item := items[row.Index()]
		return ui.Text(item.Title, 13, ui.Regular, colors.Text)
	},
}
```

## 行と識別子

`Count` は行数を指定し、`Row` は 1 行を構築します。`Key` は行に識別子を与えます。これにより、上方の行が追加・削除されても、残る行はそのセルと状態を保持します。行の追加、削除、並び替えが行われうる場合は、インデックスではなく ID を使用してください。インデックスはセルが別の行を表示するたびに追従するシグナルです。

```go
ui.List{Count: count, Key: func(i int) any { return items[i].ID }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
	return ui.Text(items[row.Index()].Title, 13, ui.Regular, colors.Text)
}}
```

`EstimatedSize` は行の想定される高さで、0 の場合は 50 ポイントになります。行ごとに異なる高さを持つこともでき、リストは構築した行を計測し、残りの行を推定します。

## レイアウト

`Header`、`Footer`、`Empty` は、行の前後や行が 0 件のときに表示するコンテンツを保持します。`Gap` は行の間隔を空け、`Columns` は行を横に並べて配置します。また、リストのパディングはコンテンツ側に適用されるため、スクロールバーは端にとどまります。リストには高さを指定するか、親要素いっぱいに広がるようにしてください。

## 選択とアクティベーション

`ui.List` は `ui.NewSelection(cx, multiple)` で作成した `Selection` を受け取ります。キー、ラベル、アクティベーション、およびキーボード操作がこれに連動します。Up、Down、Home、End でカーソルを移動し、Shift で選択範囲を拡張し、Mod+A ですべて選択し、複数選択では Space でトグルし、Enter またはダブルクリックで行をアクティベートします。

```go preview=list/selection height=320
selected := ui.NewSelection(cx, true)
ui.List{
	Count: count, Key: func(i int) any { return items[i].ID },
	Selection: selected,
	OnActivate: func(i int) { open(items[i]) },
	RowLabel:   func(i int) string { return items[i].Title },
	Row: func(cx *core.Scope, row core.ListRow) core.Node { return rowContent(cx, items[row.Index()]) },
}
```

`OnSelectionChange` は、押下、キー操作、または先行入力（typeahead）によって選択されている行が変更された後に実行され、アプリ自身が `Selection` を設定したときには実行されません。これにより、アプリが別の方法で開いたアイテムへ選択を移動させつつ、詳細ペインをユーザーの選択に追従させることができます。

`TapSelects` を指定すると、スマートフォン上でのタップがアクティベーションではなく選択になります。`RowLabel` は支援技術が読み上げる内容であり、先行入力が一致判定に使う文字列でもあります。

## 読み込みとスクロール

`OnLoadMore` は末尾が近づくと、その行数に対して 1 回実行されます。`Ref` は `ScrollToIndexAt`、`ScrollToEnd`、`ScrollToIndex` でスクロールします。内部の素のリストである `core.List` には、さらに低レベルなユースケース向けに `OnLoadMoreAtStart`、チャット向けの `Inverted`、`StickyHeaderIndices`、`MaintainVisibleContentPosition`、`Viewport`、`ViewabilityConfig` も備わっています（[リストとスクロール](../lists.md)）。

## アクセシビリティ

リストは `Label` で名前が付けられた 1 つの list または listbox として扱われ、各行は行内のテキストまたは `RowLabel` を名前として持ちます。選択機能を持つリストはカーソル位置の行を報告し、キーボードでの移動に合わせて読み上げます。スクリーンリーダーが表示領域外へ移動すると、リストにその位置へのスクロールを要求し、到達した行が構築されます。

## プラットフォーム

ネイティブのスクロールビューと仮想化は、すべてのターゲットで同じように動作します。行は再利用されるため、行の状態は `Key` に追従させる必要があります。通常は、構築時の状態を行の中ではなくアプリ側に保持するのが原則です。

## 関連項目

- [リストとスクロール](../lists.md): スティッキーヘッダーや追加読み込みを含む完全なガイド。
- [スクロール](scroll.md): 素のスクロールビュー。
- [DataTable](../lists.md#テーブル): 多数の行と列を持つテーブル。
- ギャラリーの List、Table、DataTable ページ。
