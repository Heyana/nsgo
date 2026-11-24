---
title: チェックボックス
description: オン・オフ・不確定状態を表すチェックボックス。
group: コンポーネント
order: 34
---

`ui.Checkbox` は、`ui` の見た目で横にラベルが付いたチェックボックスです。値を自身では保持せず、チェックされているかどうかはアプリ側で指定し、`OnChange` で更新します。

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## コンテンツ

`ui.Checkbox` は、ボックスの横に `Text` またはアプリ側の `Children` を表示します。`Label` は支援技術向けにその両方を置き換え、`Hint` は何のためのものかを説明します。

## 状態

`Indeterminate` はオンでもオフでもない不確定状態を表し、一部のみが選択されているときの「すべて選択」などに使われます。この状態で押下すると、オン（チェック済み）への変更が要求されます。`Disabled` は押下とフォーカスを無効にします。

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## 押下時の修飾キー

リストで Shift キーを押しながらの範囲選択が必要な場合は、`OnChange` の代わりに `OnChangeEvent` を使用します：

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

このコールバックが設定されている場合、`OnChange` の代わりに実行されます。値の更新は引き続きアプリ側で行います。デスクトップおよび Web でのキーボード操作によるアクティベーションでは、アプリのショートカットで消費されなかった修飾キーが保持されます。Windows では、Alt と Space または Return の組み合わせはウィンドウメニューへ送られます。TSX の `onChangeEvent` は `{ checked, press }` を受け取ります。

## アクセシビリティ

チェックボックスは `RoleCheckbox` とチェック済みまたは不確定の状態を報告します。名前には `Text`、`Label`、または子要素のテキストが使われます。キーボードフォーカスがあるときに Space キーを押すと切り替わり、スクリーンリーダーが新しい状態を読み上げます。

## プラットフォーム

ボックスはプラットフォームのコントロールではなくアプリ側で描画されるため、どの環境でも同じ見た目になります。押下処理、フォーカスリング、スクリーンリーダーはプラットフォームのものを使用します。すべてのターゲットでロールとラベルによってテストされています。

## 関連

- [スイッチ](switch.md): 即座に反映される設定。
- [フォーム](../forms.md): フォーム内の選択肢。
- [リスト](list.md): 行内のチェックボックス。
- ギャラリーの Checkbox ページ。
- [スタイルなしコンポーネント](../authoring/base.md#checkbox): 独自の見た目のボックスを作るための `base.Checkbox`。
