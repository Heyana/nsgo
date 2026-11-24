---
title: ダイアログ
description: モーダルサーフェス、シート、およびアラート。
group: コンポーネント
order: 41
---

`ui.Dialog` は暗くしたアプリの上に表示されるモーダルサーフェスで、タッチスクリーンでは下から現れるシートになります。`ui.Sheet` は下から表示されるパネルで、`ui.AlertDialog` はユーザーに確認を求めます。

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## 開閉

ダイアログが開いているかどうかはアプリ側で保持します。`OnOpenChange` はダイアログが要求するすべての変更を受け取り、`base.Reason` によって外側の押下と Escape キーや戻る操作を区別できます。`OnClose` はダイアログが閉じた後に実行されます。`NoCloseOnEscape` と `NoCloseOnBackdropPress` を指定すると、それぞれ Escape キーと外側の押下で閉じなくなります。

サーフェスの上部には `Title` と `Description`（表示中に変化する場合は `TitleFunc` と `DescriptionFunc`）が表示され、`Footer` にはボタンを配置します。`Bare` を指定するとサーフェス全体を `Children` に委ねてアプリ独自のレイアウトのダイアログにでき、デスクトップでは `Style` と `StyleFunc` で指定された角丸に合わせてサーフェスの縁の内側でクリップされ、`Inset` によってデスクトップ上のウィンドウ端からの最小距離が設定されます。

タッチスクリーンのシートでは、`Bare` を指定するとデフォルトのグラバーとコンテンツのパディングが取り除かれ、`Children` がパネル全体を満たすようになります。`NoGrabber` はグラバーのみを非表示にし、`ContentStyle` と `ContentStyleFunc` はコンテンツラッパーを、`TitleStyle` と `TitleStyleFunc` はタイトルをカスタマイズします。`Leading` と `Trailing` は、タイトルを中央に配置しつつタイトル行の両端にキャンセルや完了ボタンなどのノードを配置します。`StartingStyle` と `EndingStyle` はシートのトランジションをカスタマイズします。独自のタイミングが必要な場合は、それらの中に `Transition` を含めてください。

`Style` はサーフェスの見た目に対して上書きマージされます。`StyleFunc` はその後にマージされ、ダイアログが開いている間および再度開かれたときに、外観設定を含め読み取ったシグナルに追従します。たとえば、`colors := ui.ColorsOf(cx)` として `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` とするとテーマに追従します。どちらのフィールドも `ui.AlertDialog` でも機能します。ゼロ値の色を指定すると既存の色がそのまま残るため、色をクリアするには `core.RGBA(0xFFFFFF, 0)` のようなゼロ値ではない透明色を使用してください。

## シート

`ui.Sheet` は暗くしたバックドロップの上に表示される下部のパネルです。バックドロップの押下、Escape キー、または戻る操作で `OnClose` が呼ばれます。

アクセシブルなダイアログ名には `Label` または `LabelFunc` を、テストでパネルを見つけるには `TestID` を設定します。`LabelFunc` は表示中のコンテンツを置き換えることなくシグナルに追従します。`CloseOnSwipe: true` を設定すると、下方向へのスワイプが閉じるしきい値を超えたときにも `OnClose` が呼ばれるため、そのコールバック内で `Open` を更新して閉じてください。

`Style` または `StyleFunc` を設定すると、グラバーとセーフエリアの処理を保ったままサーフェスの背景、角丸の半径、パディングをカスタマイズできます。`StyleFunc` は読み取ったシグナルに追従します。`CornerRadii` を伴わない `CornerRadius` は、デフォルトの上部のみの角丸の代わりにすべての角を丸くします。

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## アラート

`ui.AlertDialog` はユーザーに確認や了承を求めます。各アクションを実行すると閉じますが、外側の押下では閉じません。

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

`Role: core.AlertCancel` を持つアクションは Escape キーまたは戻る操作で実行され、続いて `OnOpenChange(false, base.ReasonCancel)` が呼ばれます。`Default: true` を設定すると、デスクトップ、Web、および Apple TV でアラートが開いたときにそのアクションにキーボードフォーカスが当たり、Return キーで選択できるようになります。アクションが押されると `OnPress` が実行され、次に `OnOpenChange(false, base.ReasonSelect)` が呼ばれます。キーボードフォーカスが他のアクションやフィールドにない間は Return キーでデフォルトアクションが実行されますが、別のアクションにフォーカスが移っている場合は Return キーでそのアクションが押され、アラートの上に別のダイアログがある場合はそのダイアログが Return キーを受け取ります。一度アクションが実行されると、`OnOpenChange` が何をしようともアラートは閉じ、`Open` が一度 false になって再び true になるまで他のアクションは実行されません。複数のアクションに同じマーカーが付いている場合は、最初のものが優先されます。マーカーがない場合、Escape キーはアクションを実行せずに閉じ、Return キーにはデフォルトアクションがありません。`Role: core.AlertDestructive` はアクションの性質を表し、見た目は `Variant` または `Color` で選択します。

システム標準のアラートやアクションシートには `cx.ShowAlert` を使用してください（[プラットフォームサービス](../services.md#アラートとクリップボード)）。

## 振る舞い

モーダルは背後にあるアプリへの操作をブロックし、キーボードフォーカスを取得し、無効にされていない限り Escape キーと戻る操作で閉じます。タッチスクリーンでは、ダイアログは下方向へのドラッグに追従し、しきい値を超えると閉じるシートになります。

## アクセシビリティ

デスクトップとタッチスクリーンにおいて、`ui.Dialog` は `RoleDialog`（または明示的に指定された `Role`）を、`ui.AlertDialog` は `RoleAlertDialog` を報告します。どちらもタイトル（タイトルだけでは不十分な場合は `Label`）を読み上げ、続いて説明を読み上げます。ダイアログが開くとフォーカスはその内部に移動し、閉じると元の場所に戻ります。バックドロップにはフォーカスが止まりません。フッター内のボタンはそれぞれのロールとラベルを保持します。

## プラットフォーム

Android とタッチスクリーンではダイアログは下から現れるシートになり、デスクトップではバックドロップを伴う中央配置のサーフェスになります。内容は同じで、表示形式だけが変わります。

## 関連

- [ナビゲーション](../navigation.md): ページ、タブ、モーダルの組み合わせ。
- [メニュー](menu.md): アンカー付きコマンドとコンテキストメニュー。
- [アクセシビリティ](../accessibility.md): フォーカスとダイアログのロール。
- ギャラリーの Dialog、Sheet、AlertDialog ページ。
- [スタイルなしコンポーネント](../authoring/base.md#dialog): iOS のプレゼンテーションや独自のモーションを備えた `base.Dialog` と `base.Sheet`。
