---
title: Button
description: プッシュボタン、そのバリアントとサイズ、およびアイコンボタン。
group: コンポーネント
order: 31
---

`ui.Button` は `ui` の見た目を持つプッシュボタンで、ウィンドウが必要とするバリアントとサイズを備えています。完了した押下に対して `OnPress` が実行されます。

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## バリアントとサイズ

`ui.Button.Variant` には `ButtonPrimary`（デフォルト）、`ButtonSecondary`、`ButtonOutline`、`ButtonGhost`、`ButtonTinted`、`ButtonDanger` を指定できます。`ButtonTinted` と `ButtonDanger` は、淡い塗りつぶしの上にテーマのアクセントカラーまたは危険色を使用します。デスクトップや TV などのイディオムでは、`Size` は `SizeDefault`（28 ポイント）、`SizeSmall`（24）、`SizeLarge`（32）になります。タッチ操作のイディオムでは、スタイルを書き直さなくてもそのまま使えるように、同じサイズがそれぞれ 44、40、48 ポイントになります。

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` はバリアントの色ではなくアプリ独自の塗りつぶし色を指定し、`Tint` はアイコンの色を指定します。`Flex` は行内での割合を与え、`Style` はレイアウトを設定します（コンテナいっぱいに広げる、アプリ独自のサイズを設定する、マージンを追加する、角を丸くするなど）。`TestID` はテスト用の識別名を設定します。

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

アイコンのみのボタンには `ui.IconButton` があり、`Variant` に `IconButtonGhost`、`IconButtonRaised`、`IconButtonFilled` を指定できます。これはテーマのインセットカラーまたは `Fill` で塗られたフラットな円形で、ポイント単位の `Size` と支援技術向けの `Label` を持ちます。デスクトップでは、ポインターをボタン上に重ねるとラベルがツールチップとしても表示されます。そこに別の文言を出したい場合は `Tooltip` を指定し、`""` を返すようにするとツールチップは表示されません。`Text` なしで `Icon` を指定した `ui.Button` も正方形になり、同様に `Label` が必要です。

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## 内容と状態

`ui.Button` は `Text`、任意の先頭 `Icon`、またはその両方を表示します。ラベルのフォントサイズ、ウェイト、その他のテキストスタイルをカスタマイズするには `TextStyle` を使用します。

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` は関数を受け取るため、ボタンはシグナルに追従します。無効化されたボタンは押下もキーボードフォーカスも受け付けず、見た目はアプリが選んだ色ではなく無効状態のものになります。

`Busy` は処理がまだ実行中であることを表すためのものです。`true` を返している間は、ボタンのテキスト色のスピナーがコンテンツの上に表示されるため、プライマリボタンのアクセントカラーの上でも視認でき、押下は無効になり、支援技術にはビジー状態であることが伝わります。デフォルトでは、スピナーの下でもコンテンツ分のスペースが維持されます。ボタンはキーボードフォーカスを保持します。`BusyLabel` または `BusyLabelFunc` を指定すると、スピナーの横にメッセージが表示され、ビジー状態の間にボタンのアクセシブルな名前として報告されます。ボタンは少なくとも通常時の幅を維持します。

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` と `ui.ProgressRing` には、アクセントカラーが見えなくなるようなサーフェス上で使うための独自の `Tint` があります。

## リンク

`ui.Link{Text: "Docs", Href: "/docs"}` はリンクロール付きで遷移先を開きます。テキストとホバー時の下線にはアクセントカラーが使われますが、`Style.Color` で別の色を設定でき、`Style.Hover.Color` でホバー時の色を設定できます。

## アクセシビリティ

ボタンは `RoleButton` とそのテキストを自身の名前として報告します。`Label` を指定すると、支援技術に対してテキストの代わりにその名前が使われます。アイコンのみのボタンで `Label` がないと読み上げができなくなるため、必ず名前を付けてください。

デスクトップ、Web、Apple TV では、フォーカスされたボタンを Return キーまたは Space キーで押せます。スマートフォンのスクリーンリーダーではクリックとして押下されます。

## プラットフォーム

このボタンはプラットフォーム固有のボタンではなく、どこでも同じように描画されるビューです。押下、フォーカス、アクセシビリティはプラットフォームから提供されます。タッチイディオムでは、前述の通りより大きなレイアウトターゲットが使用されます。

## 関連項目

- [フォーム](../forms.md): フォーム内のボタンとバリデーション。
- [スタイルとテーマ](../styles.md): ボタンが色を取得するテーマ。
- [アクセシビリティ](../accessibility.md): ロール、ラベル、フォーカス。
- ギャラリーの Button ページ。
- [スタイルなしコンポーネント](../authoring/base.md#button): 独自の見た目のボタンを作るための `base.Button`。
