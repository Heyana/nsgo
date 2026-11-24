---
title: セレクト
description: リストから 1 つの値を、ポップアップまたはシートとして選択するコンポーネント。
group: コンポーネント
order: 36
---

`ui.Select[T]` は、任意の比較可能な型（comparable type）の値のリストから 1 つを選択します。リストはすべてのデバイスでトリガーの下に開きますが、`Presentation: base.PresentSheet` を指定すると、スマートフォンアプリでよく使われるような下からのシートとして代わりに開きます。

```go preview=select/basic height=280
size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options: []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:   size.Get,
	OnChange: size.Set,
	Label:   "Size",
}
```

## 選択肢と検索

`Options` は `ui.Option[T]` のスライスで、それぞれに `Value`、`Label`、および任意の `Group`、`Icon`、`Disabled` 関数、またはアプリ独自の `Content` を持ちます。`Select` や `Combobox` では、空でない同じ `Group` を持つ隣接した選択肢がその見出しの下に表示され、矢印キーや文字入力ではスキップされ、スクリーンリーダーでは見出しとして読み上げられます。選択肢がシグナルに追従する場合は `OptionsFunc` を使用してください。これにより、開いているリストがフィールドの値とフォーカスを保ったままその場で更新されます。`Placeholder` は選択肢が選ばれていない間に表示されます。`Searchable` は入力によってリストを絞り込み、`OnClear` はクリアボタンを表示し、`Disabled` は操作を無効化します。

```go preview=select/searchable height=400
ui.Select[string]{
	Options:  languages,
	Value:    language.Get,
	OnChange: language.Set,
	Searchable: true,
	SearchPlaceholder: "Search languages",
	OnClear:  func() { language.Set("") },
	Placeholder: "Choose a language",
}
```

`Style` はセレクトのレイアウトを指定し、幅が指定されていない場合は 200 ポイント幅になります。その高さ、パディング、書体は [Input](input.md) と同様にフィールドに反映されるため、`SizeSmall` のボタンと高さを揃えることができます。

```go
ui.Select[string]{Options: sizes, Value: size.Get, OnChange: size.Set, Label: "Size",
	Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
```

## 振る舞い

フォーカスされたトリガーから Up、Down、Return、Space キーでリストを開きます。矢印キーでハイライトを移動し、先行入力（typeahead）で項目のタイトルに一致する位置へジャンプします。項目を選ぶとリストが閉じてその値が報告されます。Escape キー、戻る操作、または外側への押下は変更を行わずにリストを閉じます。`Empty` は検索結果が見つからないときに表示されるテキストです。

## アクセシビリティ

`Label` は支援技術向けにフィールドの名前を付けます。シグナルの変化に合わせてその名前をその場で更新するには `LabelFunc` を使用してください。ラベルが空の場合は `Placeholder` が使われます。トリガーとリストはセレクトのロールを持ち、選択中の項目が報告されます。キーボードでは Tab キーでトリガーに到達し、Return または Space キーで開きます。

## プラットフォーム

macOS、Windows、Linux では、リストはトリガーの横に表示されるポップアップになります。iOS と Android ではシートになり、選択された行がフィールド上に表示され続けます。どのターゲットでも同じ選択肢と値が機能します。

## 関連項目

- [フォーム](../forms.md): フォーム内のセレクト。
- [メニュー](menu.md): 値ではなくコマンドを扱います。
- [タブ](tabs.md): 少数の選択肢を一度に表示します。
- ギャラリーの Select および Combobox ページ。
- [スタイルなしコンポーネント](../authoring/base.md#select): グループと独自のトリガーを備えた `base.Select`。
