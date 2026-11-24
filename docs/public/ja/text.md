---
title: テキスト
description: スパン、フォント、装飾、テキスト選択、ドキュメントを使ってテキストを表示します。
group: コアコンセプト
order: 53
---

## テキスト

`core.Text` は 1 つの段落であり、エンジン側でプラットフォーム独自のテキストエンジンによって組版され、メインスレッドで描画されます。サイズはフォントと幅によって決まり、ボックスの内側で折り返されます。

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` は固定の文字列で、`ContentFunc` はシグナルに追従するため、変化するテキストは関数内でシグナルを読み取ります。`MaxLines` は表示する行数を制限し、`EllipsizeMode` は収まらない部分を先頭・中央・末尾で省略し、`WhiteSpaceNoWrap` は段落を 1 行に保ちます。`AdjustsFontSizeToFit` は、`MinimumFontScale`（最小 0.1）を下限として、ボックスと行数制限に収まるまでテキストを縮小します。数式やその他のフォント相対のアタッチメントも一緒に縮小し、スペースに余裕ができれば元のサイズに戻りますが、明示的なポイントサイズやパディングは指定された値を維持します。

テキスト関連のフィールドは CSS と同様に継承されます。`FontSize`、`Color`、または `LineHeight` が設定されたビュー内のテキストは、自身で設定しない限りそれらの値を引き継ぎます。`TextAlign`、`TextTransform`、`LetterSpacing`、`TextDecoration`、`TextDecorationStyle`、`TextDecorationColor`、および `TextShadowColor`、`TextShadowOffset`、`TextShadowRadius` は行やグリフのスタイルを設定します。

## ランとリンク

`Spans` は、それぞれ独自のスタイルを持つ複数のラン（区間）からテキストを構成します。`OnPress` を持つスパンはリンクになり、それ以外のスパンを押したときはテキスト自身の `OnPress` が呼び出されます。

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` は読み取っている値が変化するとスパンを再生成し、差分のある部分だけが再準備されるため、コードエディタの行はハイライト結果が届くにつれて色分けを更新できます。スパンの `Background` と `CornerRadius` は各行の文字の背後にボックスを描画し、`Cursor` はテキスト本体の指定よりも優先してその上でのポインタの見た目を指定します。`Visibility: core.VisibilityHidden` はスパンをその場で非表示にします。単語分の領域は確保されたままになるため表示時に行が動かず、コピーやスクリーンリーダーからは除外されるため、単語を 1 つずつフェードインさせる用途などに適しています。

`Background` を持つスパンに `BackgroundInsetTop: core.Pct(62)` を設定すると、各行のボックスの下側 38% だけをハイライトできます。`BackgroundInsetBottom` は下端をトリミングします。どちらのインセットも、フォントのアセント（ascent）からディセント（descent）までのボックスの高さに対するポイント値またはパーセンテージで指定します。これらを指定してもテキストのレイアウトは変わりません。TSX では、ネストした `<Text>` のスタイルに `backgroundInsetTop: "62%"` と `backgroundInsetBottom` を設定します。角丸背景と上下のインセットは、HarmonyOS および 4 つのミニプログラムターゲットでも描画されます。

`Attachment` は、スパンのコンテンツの代わりにテキスト内へビューを配置します。アタッチメントは自身のサイズでレイアウトされ、周囲の単語と一緒に折り返され、独自のプレスイベントも受け取るため、アイコンや画像、小さなボタンを行内に配置できます。テキスト選択時は、代わりにそのスパンの `Content` がコピーされ、空の場合は除外されます。デフォルトでは、実際のフォントのメトリクスを使って最初のテキスト行が周囲のベースラインに揃います。`Em` キャンバスは独自のベースラインを提供し、ベースラインを持たないビューは下端で揃います。別の配置方法を選ぶには `Style.VerticalAlign` を設定してください。

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## フォント

`FontFamily` にはフォントファミリー名または CSS のフォールバックリスト（`Inter, "Noto Sans SC", sans-serif`）を指定でき、リスト内の前のファミリーにない文字に対して後続のファミリーが使われます。リスト内の各ファミリーにもウェイトとスタイルが適用されます。macOS と iOS では、行の高さとベースラインはその文字が実際に描画される書体から決まるため、すべて後続のファミリーで組まれた行はそのファミリーの高さとベースラインになります（ブラウザでは最初のファミリーの値が下限として保たれます）。`FontWeight`、`FontStyle`、`FontVariant`、`LineHeight`、`LineHeightMultiple`、`LetterSpacing`、`LetterSpacingEm` は書体と間隔を設定し、`core.FontMonospace` はプラットフォームの等幅フォントファミリーを表します。

`Language` はテキストの BCP 47 言語タグであり、上記のフィールドと同様にテキスト、スパン、入力フィールドへ継承されます。漢字はその言語の字形で描画されるため、システムの優先言語が中国語であっても `ja` を指定した `直` は日本の字形で表示され、韓国語（`ko`）では CSS の `word-break: keep-all` と同様に単語単位で改行されます。設定されていないテキストや入力フィールドはアプリの表示言語を使い、言語が切り替わるとそれに追従します。サブツリー全体に適用する場合はビューに、1 つのランに適用する場合は `Span` に設定してください。

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

アプリのフォントは `resources/` 配下に同梱して最初のビューの前に登録するか、アプリの実行中にフォントのバイト列を登録します。

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // TrueType または OpenType フォント
```

フォントの読み込み前にレイアウトされたテキストは再準備され、キャンバスは `Draw` を再実行して、読み込まれたフォントでグリフ描画とテキスト計測が行われるようにします。`cx.Env().FontScale()` はユーザーが設定した文字サイズ倍率であり、`NoFontScaling` が設定されていない限りテキストはこれに追従します。`MaxFontSizeMultiplier` でその上限を指定できます。

## 選択と計測

`SelectableOn` を設定するとユーザーがテキストを選択してコピーできるようになり、`View` に `Selectable` を設定すると、Web ページのようにその内部のテキストをドキュメント順でまとめて選択できるようになります。`SelectionColor` は選択ハイライトの色を指定します。`OnTextSelection` はユーザーが選択範囲を動かすたびに `core.TextSelection` を受け取ります。これにはプレーンな `Text`、`Markdown`、`HTML`（範囲内の `TextView` やスタイル付きテキストの解析済みインライン・ブロック構造を、部分的なスパンやリンクも含めて保持）、テキストごとの `Ranges`、および選択範囲が触れているテキストのビュー内での `Bounds` が含まれており、Markdown や HTML としてのコピー、単語リスト作成、選択された単語の翻訳などに使えます。`desktop.LookUp` はそれらをシステムの辞書で表示します（[サービス](services.md#リンクとファイルを開く)）。`cx.MeasureText` は指定したフォントで文字列のサイズを計測するため、テキストに合わせた列幅の決定に使えます。これを呼び出すリアクティブ関数は利用可能なフォントが変わると再評価され、`OnTextLayout` は各レイアウト後に計測された行を報告します。

## ドキュメント

`base/textview` は同じプリミティブを使って Markdown や HTML（見出し、段落、リスト、表、コード、脚注、アラート、画像、数式、リンク）を表示します。

```go
textview.TextView{
	Markdown: "## NSGo\n\nNative views from **Go**.",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

数式（`$…$`、`$$…$$`）は、アプリが `base/formula` の `RenderMath: formula.RenderMath` を渡すまでは TeX ソースのまま表示されます。渡すと周囲のテキストのサイズと色で組版され、アプリの言語で読み上げられ、TeX としてコピーできるようになります。フォントを含めてデスクトップアプリでは約 6.5 MB、Web アプリでは gzip 圧縮後で約 3 MB サイズが増えます。`formula.View` はドキュメント以外の場所でも TeX または Typst の数式を表示でき、その `OnHit` コールバックは数式エディタ向けに Untie のソース箇所、最も近いキャレット位置、および重なっている診断情報を報告します。watchOS でも数式は同じキャンバス経路を使い、リンクすると 1.4 MB のフォントを含めて実行ファイルが約 6.1 MB 増えます。watch シミュレータでの実行では、そのフォントの読み込みとパース（所有するバイト列のコピーを含む）に約 3.9 MB の保持 Go ヒープが計測されました。シミュレータでの計測は実機のメモリ上限や利用可能な余裕（ヘッドルーム）を示すものではないため、実際の値をアプリを動かす watch 実機で計測してください。設定されたフォントでカバーできないテキストラン（`\text{}` や Typst 文字列）には、CJK を含む各文字体系用のシステムフォントが使われますが、明示的な `formula.Fonts.Fallbacks` が優先されます。

少しずつ届くソースには、`formula.View` に `SourceFunc` を渡して `Streaming: true` を設定するか、`StreamingFunc` をアプリの受信状態にバインドします。これにより、有効な部分のレイアウトを続けながら回復可能なエラー表示やプレースホルダー描画が非表示になります。受信が完了したらこのフラグをクリアし、残っているエラーを表示してください。`TextView` で使う場合は、渡された `Math.TeX` と `Math.Display` を使ってその `formula.View` を返す `RenderMath` 関数を指定します。その `StreamingFunc` でも同じ受信状態を読み取れます。

ディスプレイ数式はテキストのフォントサイズを保ったまま、パディングとボーダーの内側のブロック幅に合わせて安全な関係演算子や演算子の位置で折り返されます。リサイズや文字サイズの変更に応じて行も再配置されます。分割できない部分は引き続き横スクロールし、インライン数式は本来のサイズを維持します。

`mermaid` コードブロックは、アプリが `base/diagram` の `RenderDiagram: diagram.RenderDiagram` を渡すまではソースコードを表示します。渡すと、アプリの外観に合わせてライトまたはダークで、プラットフォームのフォントを使い、limu によってネイティブの図（フローチャート、シーケンス図、クラス図、ステート図、ER 図、ガントチャート、円グラフ、マインドマップ、タイムラインなど）として描画されます。リンクを持つ図の要素は押すとアプリの URL サービスを通じてリンクを開き、ポインタを上に置くと Mermaid のツールチップが表示されます。テキストの利用可能な幅に合わせてレイアウトされ、横幅の広いフローチャートはラベルのサイズを保ったまま縦向きに配置され、それでも収まらない図は横スクロールします。コピー時には元のソースが取得され、ストリーミング中のブロックは完了するまでコードとして表示されます。`diagram.View` はドキュメント以外の場所にも図を表示でき、コンテナ幅の変化に追従します。`Icons` は `"fa:fa-car"` のような Mermaid のアイコン名を `icons/lucide` を含む `*core.SVG` ドキュメントにマップし、該当名がない場合でも行内に空白は残りません。画像ノードは共有キャンバス画像ローダーを通じて HTTP(S)、ファイル、または `asset:` ソースを読み込み、`constraint: "on"` が設定されている場合はアスペクト比を保って中央に収め、そうでない場合は引き伸ばします。Mermaid は読み込み前に画像アドレスをフィルタリングします（`data:` URL の拒否を含む）。テキストビューでアイコンを渡すには、`RenderDiagram` に `diagram.View{Source: code.Code, Icons: icons}` を返すコールバックを設定してください。

図に `ui` パレットを適用するには、`RenderDiagram` コールバックから `ThemeFunc` 付きの `diagram.View` を返します。この関数はキャンバスの描画時に色を読み取るため、パレットやサブツリーの外観が変更されると既存のビューも再配色されます。ゼロ値のままの色には図のデフォルト色が使われます。

```go
func drawDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ThemeFunc: func() *diagram.Theme {
		return &diagram.Theme{
			Text: colors.Text(), Surface: colors.Surface(), Border: colors.Border(),
			Line: colors.Muted(), Accent: colors.Accent(), Critical: colors.Danger(),
		}
	}}
}
// ui.TextView{Markdown: source, RenderDiagram: drawDiagram}
```

`textview.Parse` は Markdown を、`textview.ParseHTML` は HTML を、アプリから直接構築することもできるブロック列へと解析します。`Options.Streaming` は断片的に届くドキュメントを修復し、末尾で閉じられていない構文を閉じ、リストマーカー単体のように後続の文字によって意味が決まる最終行を保留することで、応答がマーカーをそのままテキストとして一瞬表示してしまうのを防ぎます。また、`StreamingCaret` はストリーミングで生成される応答の末尾にキャレットを描画します。

`textview.Search` は、`Search` フィールドまたは上位の `textview.ProvideSearch` を通じて参加したビュー全体を横断してテキストを検索します。`SetQuery`、`Open`、`Close` で検索バーから操作でき、`Count` と `Selected` で一致数と現在位置を取得し、`Next` と `Previous` で読み順に沿って一致箇所を前後に移動します。各ビューは一致箇所に `PartSearch` を、選択中の一致箇所に `PartSearchSelected` を付与するため、アプリのスタイルで配色し、画面内にスクロールさせつつ、ユーザーのテキスト選択状態は維持できます。

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## 次のステップ

- [画像とアイコン](images.md): テキスト内にも配置できる画像と SVG。
- [スタイルとテーマ](styles.md): 継承される色とテキストフィールド。
- [レイアウト](layout.md): テキストが幅を決める仕組み。

## リッチテキスト編集

`ui/richeditor.View` はテーマに沿った見た目で `base/richeditor.Session` を表示し、`base/richeditor.View` はアプリが提供するスタイルでその振る舞いを提供します。iOS と Android では長押しやダブルタップで単語を選択でき、選択ハンドルをドラッグしてブロックをまたいで範囲を広げられます。ネイティブの編集メニューでは「コピー」と「すべて選択」が利用でき、セッションが編集可能な場合は選択状態とクリップボードの内容に応じて「カット」と「ペースト」も利用できます。`Session.SetEditable` は開いているメニューも更新します。ハンドルのつまみは、ベースビューの `Styles.SelectionHandle` または `Classes.SelectionHandle` でスタイル設定できます。

カスタムテキストコントロールの場合は、`core.Input` にフォーカスを当てて、サーフェスポイント単位の矩形範囲と `core.EditCut`、`EditCopy`、`EditPaste`、`EditSelectAll` のビットマスクを指定して `Ref.OpenEditMenu(bounds, actions)` を呼び出します。iOS と Android では、`Input.OnEditAction` がカット、コピー、すべて選択を受け取り、ペーストには `Input.OnPaste` が使われます。メニューを閉じるには `Ref.CloseEditMenu` を呼び出してください。
