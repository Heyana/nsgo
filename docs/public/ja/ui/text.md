---
title: テキスト
description: 段落、スパン、リンク、フォント、ドキュメント。
group: コンポーネント
order: 32
---

`core.Text` はプラットフォームのテキストエンジンによって組版される 1 つの段落です。`ui.Text` と `ui.Textf` は `ui` の書体で 1 行を表示するためのヘルパーですが、要素自体は依然として `core.Text` であるため `Style` や `Spans` も受け取ります。

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## コンテンツ

`Content` は固定テキストで、`ContentFunc` はシグナルに追従します。`MaxLines` は行数を制限し、`EllipsizeMode` は入りきらない部分を先頭・中央・末尾のいずれかで省略します。`Style.WhiteSpaceNoWrap` は段落を 1 行に保ちます。`TextAlign`、`TextTransform`、`LineHeight`、`LetterSpacing` で段落のスタイルを設定し、`OnTextLayout` はレイアウト後の行情報を報告します。

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## スパンとリンク

`Spans` はそれぞれ独自のスタイルを持つテキストの並び（ラン）を作成します。`OnPress` を持つスパンはリンクとなり、その `Background` と `CornerRadius` は各行の文字の背後に描画されます。`SpansFunc` は参照している値が変化したときにランを再生成するため、ハイライトされたコードや動的に変化する文に適しています。

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` はスパンのコンテンツの代わりにアイコン、画像、小さなボタンなどのビューを行内に配置します。自身のサイズでレイアウトされ、単語と一緒に折り返され、独自の押下イベントを受け取ります。

## フォントとスタイル

`FontFamily` はフォントファミリー名または CSS のフォントリストを指定します。`FontSize`、`FontWeight`、`FontStyle`、`FontVariant`、`LineHeight`、`LetterSpacing`、`TextDecoration`、および `TextShadowColor` とそのオフセット・半径でテキストのスタイルを設定します。`core.FontMonospace` はプラットフォームの等幅フォントファミリーです。

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

パッケージ化されたフォントは最初のビューの前に `nsgo.Fonts` で登録するか、アプリの実行中に `cx.RegisterFont` で書体を登録します。フォントが利用可能になる前に準備されたテキストは再度準備されます。

## ドキュメント

`base/textview.TextView` は見出し、段落、リスト、表、コード、脚注、アラート、画像、数式、リンクといった Markdown や HTML を同じプリミティブで表示します。`textview.Parse` は Markdown を、`textview.ParseHTML` は HTML を読み込み、`Options.Streaming` は受信途中のドキュメントを修復します。

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` は `TextView` が表示しているテキストを検索し、一致箇所をハイライト表示します。複数のビューに `textview.ProvideSearch` で同じ検索オブジェクトを渡し、`Open`、`SetQuery`、`Next`、`Previous` を呼び出します。`Count` と `Selected` が結果を報告します。検索パーツのスタイルは `textview.PartSearch` と `textview.PartSearchSelected` で設定します。

## 選択とアクセシビリティ

`SelectableOn` を使うと、ユーザーがテキストを選択してコピーできるようになります。`core.View` に `Selectable` を設定すると、配下のテキストがドキュメント順にまとめて選択できるようになります。`RoleHeader` などの `Role` はテキストが何であるかを支援技術に伝え、文言がそのままラベルにならない場合は `Label` で置き換えます。

## プラットフォーム

プラットフォームのテキストエンジンがテキストをレイアウトするため、改行位置、フォント、インプットメソッドはそのプラットフォームに従います。CJK の禁則処理や改行、絵文字、右から左（RTL）のテキストもシステムに従います。アプリは `cx.Env().FontScale()` で文字サイズを読み取ることができ、`NoFontScaling` が設定されていない限りテキストはその倍率に追従します。

## 関連

- [テキスト](../text.md)：テキスト、スパン、フォント、ドキュメントの完全なガイド。
- [画像とアイコン](../images.md)：インライン画像とアイコン。
- [アクセシビリティ](../accessibility.md)：見出し、ラベル、選択。
- ギャラリーの Text および TextView ページ。
