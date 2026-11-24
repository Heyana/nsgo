---
title: スタイルなしコンポーネント
description: パーツごとにスタイルを設定したり、スタイル付きライブラリを構築したりするための、見た目を持たない NSGo 標準コントロール。
group: コンポーネントを書く
order: 73
---

`base` には `ui` が持つすべてのコントロールが含まれており、振る舞い・状態・アクセシビリティを備えつつ、見た目は一切持ちません。各パーツは `Class`、`Style`、`StyleFunc`（[パーツと状態](parts.md)）を受け取るため、独自のデザインを持つライブラリやアプリでは `ui` の代わりにこれらを使うか、`ui` のようにこれらをラップします（[見た目とテーマ](looks.md)）。このページでは[コンポーネントリファレンス](../ui.md)のコントロールを扱い、ギャラリーではそれぞれの `ui` 形式と並べて表示しています。

`Button`、`Checkbox`、`Select`、`Slider`、`NumberInput` などのコントロールは、バリデーションシグナルに追従させるための `Invalid: rejected.Get` を受け取ります。この状態は `Class: "aria-invalid:border-red-500"` や `Style.AriaInvalid` でスタイル設定します。コントロールを無効化することなく、アクセシビリティとスタイルが更新されます。複合フィールドは内部の実際の入力欄や装飾パーツへこの状態を引き継ぎ、`ColorPicker` は自身の各コントロールと共有します。

ビュー間でアプリケーションの値をドラッグする場合は、[`base/dragdrop`](../drag-drop.md) を使用してください。ドラッグ＆ドロップ領域がソースとターゲットを接続し、アプリ側でその内容、プレビュー、外観を提供します。

## Button

`base.Button` は見た目を `Style` から、内容を `Children` から受け取ります。完了した押下では `OnPress` が実行され、独自のフィードバックのために押下の開始と終了を追う `OnPressDown` と `OnPressUp` もあります。

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`、`Busy`、`Selected`、`Expanded` は状態をアプリ、スタイル、支援技術へ伝えます。実行中のアクションには `Busy`（`aria-busy:` でスタイル設定）、トグルやリストの現在項目には `Selected`、ディスクロージャーには `Expanded` を使用します。行やリスト内で別の方法でアクセスできるボタンを Tab 停止点から外すには `NoFocus` を指定します。

## Checkbox

`base.Checkbox` はボックス、インジケーター、およびその中のマークのスタイルを受け取り、アプリが所有する値に基づいて表示します。

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

インジケーターは `PartStates` を通じてボックスから状態を受け取るため、2 つ目のフォーカス停止点を作ることなく `checked:` クラスでスタイル設定できます。

修飾キーを扱う場合は `OnChangeEvent func(checked bool, press core.PressEvent)` を設定します。これは `OnChange` を置き換え、値の管理をアプリに委ねます。`press.Modifiers.Shift` を使えばリストで範囲選択を行えます。デスクトップ、Web、Android、iOS/tvOS での Space/Return によるアクティベーションでは、アプリのショートカットが先にキーを処理しない限り修飾キーの状態が保持されます。Windows では Alt と Space または Return の組み合わせはウィンドウメニューへ送られます。TSX では `onChangeEvent` が `{ checked, press }` を受け取ります。

## Switch

`base.Switch` はトラック、その内部のサム、そしてラベルとしての小要素で構成されます。押下されると、現在の `Checked` とは逆の要求値を報告します。

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

トラックとサムはそれぞれ独自の `StyleFunc` を受け取り、状態スタイルは `Checked` に追従します。

## Slider

`base.Slider` は `base.SliderValue`（`base.SingleValue(v)` または `base.RangeValue(low, high)`）を受け取ります。トラック、インジケーター、レンジ、サムを個別にスタイル設定できます。

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`、`Max`、`Step` はリアクティブなゲッターです。それらのシグナルを変更すると、ドラッグ中であってもマウント済みのスライダーとそのアクセシブルな範囲が更新されます。`Axis` で垂直スライダーになり、`Inverted` を設定すると `base.Progress` と同様に最小値が逆側の端に置かれます。`Marks` はインジケーター上に独自の目盛りを配置し、`OnChangeEnd` はドラッグ終了時の値を報告します。

## Select

`base.Select` は `base.SelectItem` と `*string` 型の値を受け取るため、「未選択」は `nil` になります。トリガー、ポップアップ、各行、およびグループをスタイル設定できます。

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

同じ `Group` を持つ隣接アイテムはヘッダーを共有し、先行入力（typeahead）はアイテムのタイトルに一致し、`SearchField` で独自の検索フィールドを描画でき、`Cleanable` でクリアボタンを表示します。`OnCancel` は Escape キー、戻る操作、および外側への押下を受け取ります。

## Tabs

`base.Tabs` は `base.Tab` を保持し、`IndicatorStyle` からインジケーターを描画します。どのタブが選択されているかはアプリ側で保持します。`base.Tab.OnPress` は選択済みのタブも含めて押下されるたびに実行されます。

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` はバックドロップ、レイヤー、ポップアップのスタイル、`Animation` または `TransitionShader`、そして子要素を受け取ります。`Open`、`DefaultOpen`、`OnOpenChange` で開閉を制御し、`OnOpen`、`OnClose`、`OnCancel` で結果を報告します。`OnConfirm` はデフォルトアクションです。キーボードフォーカスが Return キーを自身で処理するボタンやフィールドにない限り、また別のモーダルがダイアログを覆っていない限り、Return キーで `OnConfirm` が実行され、その後ダイアログは `ReasonSelect` で閉じます。`Role` と `AccessibilityDescription` はスクリーンリーダーが読み上げる内容で、確認ダイアログには `RoleAlertDialog` を使用します。`NoCloseOnSwipe` は iOS のスワイプで閉じる動作を無効にし、`Presentation` は iOS のプレゼンテーション方法を選択し、`StatusBar` は表示中のステータスバーを変更します。

`base.Sheet` は任意の端に配置されるパネルで、バックドロップとコンテンツを持ちます。`CloseOnSwipe` を指定するとボトムシートが下方向へのドラッグに追従し、しきい値を超えると閉じます。`base.Dialog` と同様に `OnConfirm`、`Role`、`AccessibilityDescription` を受け取ります。`base.ShowAlertDialog` はロールに応じたボタンを持つシステムのアラートまたはアクションシートを表示します。

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` は独自のトリガーから `base.MenuItem` を開き、`base.ContextMenu` はポインター位置で開きます。アイテムは `OnSelect` を報告し、`NoCloseOnSelect` がない限りメニューを閉じます。`Checked` を指定するとチェック可能なアイテムになり、`Indicator` は内容の前にマークを描画し、`Role` は支援技術に伝える役割（メニュー項目、チェックボックス項目、ラジオ項目）を選択します。`base.Submenu` にはさらにアイテムを入れられます。

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

デスクトップでは `Placement`、`Align`、`Offset`、`Margin` でポップアップの位置を決め、`PopupStyle` と `BackdropStyle` でスタイルを設定します。

## EmojiPicker

`base.EmojiPicker` は、アプリの言語（または明示的な `Languages` / `LanguagesFunc`）でローカライズされた CLDR 名、キーワード、ショートコードによってすべての Unicode 絵文字を検索します。カテゴリータブ、オプションの `Recent` エントリー、肌の色（スキントーン）の選択、キーボードナビゲーション、仮想化されたグリッドを備えています。すべてのデフォルトラベルとカテゴリー名は `base/locales` から取得され、絵文字データセットは `EmojiPicker` またはそのヘルパー（`base.SearchEmojis`、`base.EmojisInCategory`、`base.EmojiByString`）を使用するアプリにのみリンクされます。

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` と `DefaultCategory` で表示するブラウズグループを選択します。カテゴリータブとスキントーンの選択肢はそれぞれ 1 つの Tab 停止点となり、矢印キーで移動できます。`EmojiPickerRef` は `Focus`、`Query`、`SetQuery`、`SetCategory`、`MatchedCount` を提供し、スタイル付きピッカーも同じクエリ状態を使用します。`LabelFunc`、`SearchLabelFunc`、`PlaceholderFunc` を使うと表示中の文言をその場で更新できます。`Emoji.WithSkinTone` はペア絵文字の双方に選択されたスキントーンを適用し、デフォルトトーンに戻すとニュートラルな形式に復元します。絵文字そのもので検索すると、その正確なシーケンスが保持されます。

Lucide カテゴリーアイコン、クリア可能な検索フィールド、強調表示された絵文字プレビュー、スキントーンスウォッチを備えたテーマ対応ピッカーを使用するには、`ui.EmojiPicker` を使用してください。
