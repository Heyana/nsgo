---
title: アクセシビリティ
description: スクリーンリーダーやキーボード向けのラベル、ロール、値、アクション、フォーカス。
group: コアコンセプト
order: 61
---

## スクリーンリーダーが読み上げる内容

ビューの `Label` は支援技術が読み上げる名称、`AccessibilityDescription` はラベルの後に読み上げられる補足説明、`Hint` は操作したときに何が起こるかの説明、`Role` はその要素が何であるか（`core.RoleButton`、`RoleLink`、`RoleHeader`、`RoleImage`、`RoleCheckbox`、`RoleSwitch`、`RoleRadio`、`RoleTab`、`RoleTextField`、`RoleSearchField`、`RoleStatus`、`RoleMenu`、`RoleMenuItem`、`RoleDialog`、`RoleAlertDialog`、`RoleAlert`、`RoleAdjustable`、`RoleProgressBar`、`RoleTable` など）を表します。ステータスはデフォルトで polite なライブリージョンになり、検索フィールドは検索ボックスとして読み上げられるテキストフィールドになります。ダイアログは開いたときにロールとラベルが読み上げられ、その後に `ui.Dialog` のタイトルと説明と同様に説明が読み上げられます。ラベルのないテキストはその内容が読み上げられます。`LabelFunc`、`HintFunc`、`AccessibilityDescriptionFunc` は同じビュー上で動的に変化するアクセシビリティテキストをバインドするため、言語切り替え時にもコントロールのフォーカスと状態が維持されます。ラベルを持つコア要素はすべて `LabelFunc` を受け取り、`View` と `Text` は `HintFunc` を受け取ります。

`ui.Button`、`ui.Checkbox`、`ui.MenuItem`、`ui.AlertAction` でテキストを動的に変更する場合は `TextFunc` を使用します。これらのアクセシブルな名前はそのテキストに追従します。ボタンとチェックボックスは別途 `LabelFunc` も受け取ります。スライダー、タブ、スイッチ、プログレスバー、ポップアップトリガーは、変化するアクセシブルな名前に対して `LabelFunc` を受け取ります。`ui.Option.LabelFunc` はタブ、セレクト、コンボボックスにおける選択肢のテキストを提供します。数値入力、コード入力、プログレスリング、および両方のスピナーバリアントも `LabelFunc` を受け取ります。

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` は範囲やテキスト形式の値を提供します（スライダーの最小値・最大値・現在値、または「3 items」のようなテキストなど）。`Accessible` はビューを 1 つの要素としてまとめるため、カードの名称と価格を 2 つのフォーカス停止点ではなくまとめて読み上げることができ、ラベルがない場合は子孫のテキストを読み上げます。`AccessibilityElementsHidden` はシェブロンや区切り線などの装飾要素を隠し、`ImportantForAccessibility` はサブツリー全体を隠します。

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## 表示ラベル

コントロールの `Ref` を `core.Text` または `core.View` の `LabelFor` に指定します。ラベルを押すとそのコントロールにフォーカスが移り、チェックボックスの切り替えなどの押下アクションが実行されます。現在の表示テキストが `Label` 文字列を繰り返すことなくコントロールの名前となり、アクセシビリティとスタイルの両面でコントロールの `Disabled` および `Invalid` 状態に追従します。ラベル自体にはボタンロールや Tab 停止点は追加されません。

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` は、複数の表示要素を指定された順序で組み合わせてコントロールの名前とし、`Label` より優先されます（押下イベントは転送しません）。それ以外の場合は、ツリー順に従う暗黙の `LabelFor` による名前よりも `Label` が優先されます。テキストの更新、移動、非表示、削除、再マウントに応じて関連付けも更新されます。欠落している参照、重複した参照、循環参照、非表示の参照は無視され、関連付けは同一サーフェスおよび閉じ込めスコープ内に留まります。ラベルからモーダル背後のコントロールを操作することはできません。

`base` と `ui` のボタン、チェックボックス、ラジオ、スイッチ、セレクト、数値入力は `Ref` と `LabelledBy` を内部のコントロールに渡します。スライダーは `ThumbRef` と `StartThumbRef` を公開しています。TSX では、`ref={node => field.set(node)}` でセットしたノードシグナルとともに `labelFor={field()}` と `labelledBy={[label()]}` を使用することで、フィールドより前に配置されたラベルでもフィールドのマウント時に更新されるようになります。

## アクション

`AccessibilityActions` は、スクリーンリーダーが要素に対して提供するアクションを列挙します。標準アクション（`core.ActionActivate`、`ActionIncrement`、`ActionDecrement`、`ActionEscape`、`ActionMagicTap`）やアプリ独自のアクションを指定でき、それぞれ `Name` と `Label` を持ちます。`OnAccessibilityAction` がその名前を受け取ります。要素をマウントしたまま利用可能なアクションを変更したり、ラベルを翻訳したりするには、`core.View` または `core.Input` で `AccessibilityActionsFunc` を使用します。

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` は、複数項目の選択を許可するグリッド、リストボックス、またはツリーであることを示します。`base.DataTable` は複数行選択時にこれを設定し、Web、Windows、Linux で対応するアクセシビリティフラグが公開されます。

`Expanded` はディスクロージャーやメニューボタンが開いているかどうか、`Busy` はコンテンツが読み込み中であること、`Invalid` はアプリがフィールドの内容やコントロールの値を拒否したこと（`aria-invalid:` でスタイル設定可能）を示し、`Checked`、`Selected`、`Pressed` はコントロールの状態を報告します。コントロールのパーツは `PartStates` を通じてその状態を受け取れるため、インジケーターは自身のフォーカス停止点を作ることなくチェックボックスの状態に追従できます。

`SelectionItem` は、現在の `Selected` 値とは独立して、ビューが選択可能なコレクション内のアイテムであることを示します。`base.DataTable` は行またはセルを選択可能にするときにそれらへ設定し、`base.Tree` はすべての行に設定します。Windows では SelectionItem パターンが保持され、Web では `aria-selected="true"` または `"false"` が出力されます。選択機能のないテーブルでは、行にいずれのマーカーも付きません。

## 動的な変更

`LiveRegion` を設定すると、ビューのラベルや内部テキストが変化した際にスクリーンリーダーが読み上げます。`core.LiveRegionPolite` はユーザーがアイドル状態になるまで待機し、`LiveRegionAssertive` は即座に割り込んで読み上げます。`cx.Announce` は 1 つのメッセージを 1 回だけ読み上げます。

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## フォーカスとキーボード

ツールチップは、トリガーがフォーカスを受け取る前にその内容を説明します。`View.Tooltip`、`base.Tooltip`、`ui.Tooltip` は、コントロールの名前と重複しない限り、かつアプリが `AccessibilityDescription` を指定していない、またはツールチップが `Disabled` でない場合に、現在のテキストをコントロールのアクセシビリティ説明として使用します。ラッパーはその説明を配下のコントロールに渡します。バブルとその子要素はスクリーンリーダーから隠され、アラートとして通知されることはありません。したがって、`ui.IconButton` のデフォルトツールチップがそのラベルと重複して読み上げられることはありません。

`base.DataTable` は名前付きグリッドをキーボードおよびアクセシビリティのフォーカス停止点として公開し、ポインターのヒットレイヤーは支援技術から隠されます。

デスクトップ、Web、Apple TV では、押下を受け取るビューはテキストフィールドと並んで Tab 停止点になります。`NoFocus` を指定すると停止点から除外され、`Focusable` を指定すると押下を受け取らないビューでも停止点になります。`Style.FocusVisible` はキーボード操作でフォーカスが移動したときだけフォーカスリングを描画し（`focus-visible:`）、`Style.Focus` はすべてのフォーカスに、`Style.FocusWithin` はビュー内部のフォーカスに反応します。`OnFocus` と `OnBlur` はフォーカス変化を受け取り（`OnBlur` はフォーカス中のビューがツリーから離れるときにも呼ばれます）、`Ref.Focus()` と `Ref.SetAccessibilityFocus()` でフォーカスを移動できます。`core.FocusedRef(cx)` は現在フォーカスを持つビューへの Ref であり、キー操作から開く検索バーのようにトリガーを持たない要素が、閉じるときに元のフォーカスへ戻すために使います。Return キーと Space キーはフォーカス中のビューを押し、矢印キーはタブリスト、ラジオグループ、メニュー内を移動します。キーボードでビューにフォーカスが移動すると、そのビューの `Tooltip` は即座に表示され、フォーカスが外れるか Escape が押されるまで表示されたままになります。ポップアップの `OnOpen` はコンテンツが配置された後に実行されるため、その中で `Focus` を呼ぶと非モーダルダイアログと同様にフォーカスを内部へ移動できます。ポップオーバーのコンテンツは Tab 順序でトリガーの直後に続き、ポップアップが閉じるとフォーカスはトリガーに戻ります。

Android、iPhone、iPad では、ハードウェアキーボード向けに `Ref.Focus()` とボタンの `AutoFocus` によって、有効な押下可能ビューまたは `Focusable` なビューにフォーカスを当てることができます。コントロールをタッチしてもそのフォーカスは移動しません。Space と Return はキーマップ処理の後にフォーカス中の押下可能ビューをアクティブ化し、すべての押下フェーズで Control、Alt、Shift、およびプラットフォームキーの修飾状態を保持します。`NoFocus`、無効化または非表示のコントロール、モーダル背後のコントロールはそのフォーカスを受け取れません。ビューにフォーカスを当ててもソフトウェアキーボードは開かず、ネイティブのテキストフィールドは従来の編集動作を保ちます。watchOS とミニプログラムには物理キーボードによるアクティベーション経路はありません。

`cx.Env().ScreenReader()` はスクリーンリーダーが動作中かどうか、`cx.Env().ReduceMotion()` はユーザーが視差効果・モーションの軽減を求めているか、`cx.Env().IncreaseContrast()` はコントラストの上昇を求めているか（macOS・iOS・watchOS の「コントラストを上げる」、Windows のコントラストテーマ、GNOME のハイコントラスト、GTK 4.20 のコントラスト設定またはハイコントラスト GTK テーマ、Android 14 の標準以上のコントラスト設定、Web の `prefers-contrast: more` または `forced-colors: active`）、`cx.Env().FontScale()` は文字サイズの倍率、`cx.Env().RTL()` はレイアウト方向を返します。`NoFontScaling` が設定されていない限りテキストは文字サイズ設定に追従し、右から左（RTL）のレイアウトでは論理エッジを使用していれば物理エッジが自動的に反転されます。

## 次のステップ

- [スタイルとテーマ](styles.md): フォーカス状態と無効状態。
- [フォーム](forms.md): フィールドのラベルと値。
- [キャンバスとシェーダー](canvas.md): 描画のパーツを公開する。
