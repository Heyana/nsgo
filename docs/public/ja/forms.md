---
title: フォーム
description: テキストフィールド、フォーカス、ソフトウェアキーボード、スイッチ、ピッカー、バリデーション。
group: コアコンセプト
order: 57
---

## テキストフィールド

`ui.Input` はプラットフォーム標準のテキストフィールドを `ui` の見た目で提供し、`base.Input` のフィールドを備えています。`Value` を指定すると制御コンポーネントになり、アプリが値を保持して `OnChange` で更新します。`DefaultValue` を指定すると自身で値を保持します。

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` は文字を伏せ字にし、`Readonly` はテキストを選択・コピー可能なまま編集を防ぎます（ミニプログラムでは WeChat の単一行フィールドのみで対応し、WeChat の複数行フィールドと Alipay のフィールドはホスト側で編集を取り消せないため `Disabled` と同様に無効化されます）。`Disabled` はフィールドを無効化し、`Multiline` はテキストを折り返してスタイルでサイズが固定されていない限り内容に合わせて高さを伸ばします。複数行フィールドの各行とプレースホルダーはスタイルの `LineHeight` と同じ高さになり、`core.Text` と同様にフォントがその中に中央揃えされます。単一行フィールドは1行を高さの中央に揃えます。`KeyboardType` はソフトウェアキーボードの種類（`core.KeyboardEmail`、`core.KeyboardNumber`、`core.KeyboardURL` など）を選び、`AutoComplete` は自動入力のためにフィールドの内容をシステムに伝え、`MaxLength` はユーザーが入力できる UTF-16 単位数の上限を決めます。`AutoCapitalize` と `NoAutoCorrect` はキーボードの動作を調整します。

`OnSelectionChange` と `OnCaretMove` は選択範囲とキャレット位置を報告し、その位置にメニューを開く用途などに使えます。`Suggest`（`core.Suggest`）は `@` などのトリガーに続くクエリを追跡し、フィールドの隣の `ui.SuggestList` がキャレット位置に候補行を表示して上下キー、Return、タップで選択できるようにします（`chat.Composer` の `Suggest` によるメンション補完と同じ仕組みです）。`OnKeyPress` は各キー入力を、`OnPaste` はペーストされた内容を、`OnCompositionChange` はインプットメソッドの変換状態を追跡します。

iOS と Android では、`cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` を使うとフィールドが送信・フォーカス解除・改行挿入を行う前にソフトウェア Return キーを捕捉できます。`KeyAuto` と `KeyNotEditing` は `SubmitBehavior` に処理を委ねます。iOS では、未確定文字を確定する Return はインプットメソッド側に残ります。

## フォーカスとキーボード

`Ref.Focus()` は入力フィールドにフォーカスを移すか、デスクトップ・Web・Apple TV では有効な押下可能ビューまたは `Focusable` なビューにフォーカスを移します。`Ref.Blur()` は自身のビューのフォーカスのみを解除し、`cx.DismissKeyboard()` はソフトウェアキーボードを閉じます。`OnFocus` と `OnBlur` はフォーカスを追跡し、フォーカス可能な任意のビューでも `View.OnFocus`/`OnBlur` で同様に追跡できます。フォーカスされたビューが `Show` が閉じることや行の削除によってツリーから外れると、そのビューを削除したコミットの後（またはプラットフォームが次のフォーカスを先に報告する場合はそのビューの `OnFocus` の前）に `OnBlur` が一度だけ呼ばれ、ツリーに残る周囲のビューには `OnFocusOut` が呼ばれます。

`Ref` へのフォーカス、ブラー、スクロール、ズーム要求は、その要素がどの場所にも表示されていない `Holder` のコンテンツ内にある間は何もせず、要素がツリーから外れたときにキューに残っていた要求は破棄されます。そのコンテンツを再び表示しても要求は再生されないため、配置された後に再度呼び出してください。

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

スマートフォンの画面下部はソフトウェアキーボードに隠れます。下側のパディングまたはマージンとして `core.EnvKeyboard` を指定するとコンテンツがキーボードの上に保たれ、移動もキーボードに合わせてアニメーションします：

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

フォームを囲むスクロールビューでは、ドラッグ時にキーボードを閉じることや、キーボード表示中のタップの扱いを決められます：

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

コード内では `cx.Env().KeyboardHeight()` と `cx.Env().KeyboardDuration()` でキーボードのサイズとアニメーション時間を、`cx.Env().SafeArea()` で画面周囲のインセットを読み取れます。

## 選択コントロール

`ui.Switch` は即座に反映される設定切り替え、`ui.Checkbox` はオン・オフ・不確定状態、`ui.Radio` は `ui.RadioGroup` 内の選択肢の1つです。`ui.Select[T]` はトリガーの下に選択肢一覧を開くか、`Presentation` の指定に応じてシートとして開き、`ui.Tabs[T]` は複数の選択肢を並べて表示します。

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` と `base.NumberInput` は変化するアクセシブル名のために `LabelFunc` を受け付けます。カスタムの `Format` は編集中でない間は参照するシグナルに追従し、言語を切り替えても編集終了まで入力中の下書きが保持されます。`ui.NumberInput` は境界付きで数値をステップ増減・スクラブ入力でき、`ui.DatePicker` と `ui.TimeField` は日付と時刻を、`ui.ColorPicker` は色を、`ui.TagInput` はタグのリストを編集します。

`ui.OtpInput` はワンタイムコードをマス目に入力します。これと `base.OtpInput` は、フォーカスされたフィールドや入力コードを置き換えることなく `LabelFunc` を受け付けます。`Size` はデフォルトの幅と高さ（デスクトップと TV では 36 ポイント、タッチデバイスでは 44 ポイント）を設定します。`CellWidth` と `CellHeight` で縦横それぞれ個別に上書きできます。`TextStyle` はセルの文字スタイルを、`CornerRadius` は角丸を、`CaretHeight` は行高から算出されるキャレット高さを上書きします。

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## グループ化された行

`ui.FormSection` は行をカード上に配置し、上にタイトル、下にフッターを表示します。カードの背景・ボーダー・角丸には `CardStyle` を設定し、`CardStyleFunc` はその上にマージされて読み取るシグナルに追従します。`DividerInset` には `core.Each` で作られた行を含む各子行が渡されます。インセットと `true` を返すと区切り線のインセットを上書きし、`false` を返すと自動のタイトル揃え（他のノードでは 12 ポイント）を維持します。インセットを 0 にすると全幅の区切り線になります。

`ui.FormRow` はタイトルの前に `Leading` またはアイコンタイルを配置します。`LeadingWidth` を設定すると独自ノード用のスペースを確保し、その下の区切り線をタイトルに揃えられます（このスロットのデフォルトは 24 ポイントです）。`TileSize` を設定するとアイコンタイル、グリフ、角丸が連動してスケーリングされます。`TitleStyle` と `DetailStyle` はタイトルと詳細テキストのタイポグラフィを上書きします。`TitleMaxLines` はタイトルの最大行数を制限し（0 は無制限）、`MinHeight` は行の最小高さをポイント単位で上書きし、`Below` はタイトルと詳細のテキスト列の下にノードを配置します。`MinHeight` が 0 のときはデフォルト（40 ポイント、タッチデバイスでは 48 ポイント）になります。

単一選択のグループにするには、セクションを `base.RadioGroup` で囲み、各行に `Role: core.RoleRadio`、`Checked`、`OnPress` を設定します。行は `base.Radio` を使ってチェック状態と選択状態のアクセシビリティ、1つの Tab ストップ、矢印キーによる選択を提供し、すでにチェック済みの行を押しても何もしません。選択された値はアプリが管理します。

`ui.FormSection.TitleFunc` と `FooterFunc`、および `ui.FormRow.TitleFunc`、`DetailFunc`、`LabelFunc` は、行を再構築せずに文言だけを更新します。詳細、セクションタイトル、フッターが空になるとその任意テキストは取り除かれ、行ラベルが空の場合は現在のタイトルが使われます。検索可能な `ui.Select` では `SearchPlaceholderFunc` と `EmptyFunc` をバインドしても、開いている検索状態や選択状態が維持されます。

## バリデーション

ルールはアプリ側で管理します。シグナルでエラー文字列を保持し、`OnBlur` や `OnSubmit` でそれを設定し、エラーがある間だけ `core.Show` でフィールドの下にメッセージを表示します。

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` は表示されるメッセージをアニメーションさせ、`OnSubmit` と `ReturnKeyNext` の組み合わせでキーボードのフォーカスを次のフィールドへ移動させます。

## 次のステップ

- [ナビゲーション](navigation.md)：フォームをどこに配置するか。
- [テキスト](text.md)：フィールドの文字をどうスタイル付けするか。
- [アクセシビリティ](accessibility.md)：スクリーンリーダーが必要とするラベルと値。
