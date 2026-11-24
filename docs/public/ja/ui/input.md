---
title: 入力フィールド
description: 制御・非制御の両方に対応し、フォーカスとソフトウェアキーボードを備えたプラットフォーム標準のテキストフィールド。
group: コンポーネント
order: 33
---

`ui.Input` は、`ui` の見た目を持つプラットフォーム標準のテキストフィールドで、`Multiline` が指定されていない限り 1 行になります。他のすべてのスタイル付きフィールドと同様に `base.Input` のフィールドを備えています。`ui.Field` は同じフィールドですが独自のサイズを持たず、ポインターやフォーカスに追従しない枠内に配置されます。`Background` に `core.Transparent`、`BorderWidth` に `core.Zero` を指定すると枠が消え、プレーンテキストのように見えながら編集できるテキストになります。

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## 制御と非制御

`Value` を指定すると制御フィールドになり、アプリが値を保持して `OnChange` で更新します。`DefaultValue` を指定するとフィールド自身が値を保持します。制御フィールドは、値を `""` に設定することでクリアできます。

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## フィールド

`Multiline` はテキストを折り返し、スタイルで `Height` や `MinHeight` が指定されていない限り 3 行分の高さから始まってテキストに合わせて伸びます。`Masked` は文字を伏せ字にし、`Readonly` はテキストの選択やコピーを許可したまま編集を防ぎ、`Disabled` はフィールドを無効化し、`MaxLength` はユーザーが入力できる UTF-16 単位数の上限を設定します。`Placeholder` は `PlaceholderFunc` を使って関数として指定することもできます。

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` はソフトウェアキーボードの種類（`KeyboardEmail`、`KeyboardNumber`、`KeyboardURL` など）を選び、`ReturnKeyType` は Return キーのラベルを設定し、`SubmitBehavior` は Return キーで送信するか複数行の改行を挿入するかを決めます。`AutoCapitalize`、`NoAutoCorrect`、`AutoComplete` はシステムによる入力補助を調整します。

`InputSource` はフィールドにフォーカスがある間だけキーボードの入力ソースを切り替え、`KeepInputSource` が指定されていない限りフォーカスが外れるとユーザーの元の入力ソースに戻します。たとえば macOS で `"com.apple.keylayout.ABC"` を指定すると、コードやコマンド用のフィールドを英語入力に保てます。ID はプラットフォーム固有のもので、Windows では `"00000409"` のようなキーボードレイアウト ID、Linux では Fcitx 5 または IBus のインプットメソッドになります。有効化されていない入力ソースや、スマートフォンおよび Web では何も変わりません。

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` はフィールドの見た目に対して上書きマージされ、`StyleFunc` はさらにその上にマージされます。高さ、パディング、フォント、色など指定された項目だけが `ui` のものを置き換え、残りはそのまま維持されます。どちらにも `BorderColor` や `Hover` が設定されていない限り、枠線はポインターが乗ると暗くなり、フォーカス中はアクセントカラーになります。ゼロ値の `Padding` は `ui` のパディングを残しますが、明示的に設定される長さである `core.Zero` を使うと取り除くことができます（`Padding: core.All(core.Zero)`）。

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## フォーカスとイベント

`Ref` はフィールドにフォーカスを当てたり外したりします。`OnFocus` と `OnBlur` はフォーカスを、`OnSelectionChange` と `OnCaretMove` は選択範囲とキャレットを、`OnKeyPress` は入力された各キーを、`OnPaste` はペーストされた内容を、`OnCompositionChange` はインプットメソッドの変換状態を追跡します。

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

スマートフォンの画面下部はソフトウェアキーボードに隠れます。コンテンツの下側のパディングまたはマージンに `core.EnvKeyboard` を指定すると、コンテンツがキーボードの上に保たれ、キーボードに合わせてアニメーション移動します。`cx.DismissKeyboard()` はキーボードを閉じ、`cx.Env().KeyboardHeight()` はキーボードの高さを読み取ります。

## バリデーション

ルールはアプリ側が管理します。シグナルがエラーを保持し、`OnBlur` または `OnSubmit` でそれを設定し、フィールドの下のメッセージで表示します（[フォーム](../forms.md#バリデーション)）。

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## アクセシビリティ

フィールドは `RoleTextField`（または `Role` で検索ボックスを指定した場合は `RoleSearchField`）を報告し、`Label` を読み上げます。`Placeholder` はラベルではないため、目に見えるラベルがないフィールドには `Label` を指定してください。`Readonly` と `Disabled` も報告され、`OnSubmit` はキーボードのアクションになります。単一行、伏せ字（masked）、および複数行のフィールドは、macOS、iOS、Linux において現在の `Placeholder` をネイティブのアクセシビリティ用プレースホルダーまたはヒントとして公開し、複数行フィールドで描画されるプレースホルダーラベルのために 2 つ目のフォーカス停止点を作ることなく更新およびクリアします。

## プラットフォーム

iOS、Android、Windows、Linux では各プラットフォームのネイティブフィールドを使用し、Web では `<input>` または `<textarea>` を使用します。ソフトウェアキーボード、自動入力、インプットメソッド、テキストの方向はプラットフォームのものを使います。フォーカスと文字入力はすべてのターゲットで同じ方法でテストされています。

## 関連

- [フォーム](../forms.md): フィールド、バリデーション、選択肢、キーボード。
- [アクセシビリティ](../accessibility.md): ラベルとフォーカス。
- [レイアウト](../layout.md): コンテンツをキーボードの上に保つ。
- ギャラリーの Input と Form ページ。
