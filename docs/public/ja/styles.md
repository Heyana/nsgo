---
title: スタイルとテーマ
description: 型付きスタイル、Tailwind クラス、状態スタイル、そしてライトテーマとダークテーマ。
group: コアコンセプト
order: 52
---

## スタイル

`core.Style` は、ボックスのレイアウト、見た目、テキストをまとめて表す 1 つの Go の値です。各フィールドのゼロ値は「未設定」を意味するため、スタイルは合成できます。`Merge` は後から指定されたスタイルで設定されたフィールドのみを置き換え、子要素は親要素が設定したテキスト関連のフィールドを継承します。長さを設定しているスタイルに対して長さを 0（なし）に上書きするには、色に対する `core.Transparent` と同様に、`Padding`、`Margin`、`Borders`、`BorderWidth`、`CornerRadius`、またはギャップに `core.Zero` を指定します（例: `Padding: core.All(core.Zero)`）。

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

スタイルには CSS の色、角丸、ボーダー、アウトライン、シャドウ、グラデーション、フィルター、背景ブラー（backdrop blur）、マスク、ブレンドモード、3D トランスフォーム、トランジションを指定できます。`core.RGB`、`core.RGBA`、`core.ParseColor` で色を作成し、`core.LinearGradient` とその関連関数でグラデーションを、`core.Ring` で Tailwind のリングを、`core.AllCorners` で各角の半径を設定できます。`BorderCurveContinuous` は iOS のように角を辺へ滑らかにつなげて丸めます。ボックスのシャドウとアウトラインは、[レイアウト](layout.md#位置指定)で説明されている例外を除き、`Overflow` によるクリップの外側に留まります。背景フィルター（backdrop filter）は CSS の `backdrop-filter` と同様にビューの背後にあるものを変化させ、`Blur` は半径に応じて背後をぼかし、カラーフィルターは色を変更します。カラーフィルターなしのブラーに対して、iOS は `View.IOS.BackdropMaterial` で指定されたシステムマテリアルを描画します。`core.BackdropUltraThin` 以外のマテリアルを指定したブラーは、iOS ではそのマテリアルを描画し、macOS では AppKit で最も近いマテリアルを描画しますが、カラーフィルターは適用されません。Linux と Android では背景フィルターは描画されません。`Style.GlassEffect`（`core.GlassRegular` または `core.GlassClear`、TSX では `glassEffect`）は、macOS 26 および iOS 26 で背景ブラーの代わりに Liquid Glass を描画します（Windows では NSGo のガラスシェーダーで描画されます。他のプラットフォームでのフォールバック用にガラスビューにはブラーも指定してください）。`Style.GlassInteractive`（TSX では `glassInteractive`）を設定すると、`OnPress` や押下可能な子要素の動作を維持したまま、macOS 27（`NSGlassEffectView.effectIsInteractive`）および iOS 26（`UIGlassEffect.isInteractive`）で Liquid Glass に対するシステムのインタラクティブな押下レスポンスが有効になります。

`Transform` はビューとともにポインターのターゲットも移動させます。押下、ホバー、ジェスチャーは描画されている位置から開始され、報告されるポインター位置はトランスフォームを元に戻した座標になります。親要素の外にはみ出して配置されたトランスフォーム後の子要素も、クリップやスクロールビューポートによって隠されていない限り到達可能です。ミニプログラムでは、レンダーレイヤーが実行するキーフレームの座標変換の打ち消しはまだサポートされていません（[ロードマップ](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)）。

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## 状態

ビューの見た目は状態に応じて変えることができます。`Hover`、`Focus`、`FocusWithin`、`Active`、`Selected`、`Checked`、`Disabled`、および `aria-*` の各状態は `*Style` 値であり、その状態が維持されている間は他のスタイルの上にマージされます。アプリは `Selected`、`Checked`、`Disabled`、`Expanded` を関数を通じて設定し、プラットフォームは押下とフォーカスを報告します。

`Active` と `group-active:` はビューが押されている間追従しますが、そのためには押下ハンドラーまたはタッチを保持する `Responder` が必要です。通常のビューに `Active`、`active:`、`group` でスタイルを設定しても、押下可能になったりキーボードの Tab 移動先になったりはしません。ビュー自体がフォーカスを受け取るべき場合は `Focusable` を設定してください。

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

コンポーネントの一部は、自身で状態を報告することなく周囲のコントロールの状態を引き継ぐことができます。`PartStates` は `Checked`、`Selected`、`Disabled` などを提供するため、チェックボックスのマークはスクリーンリーダーに対してもチェックボックスの状態に追従します。

## クラス

`View`、`Text`、`Span`、`Input`、`Image`、`Scroll`、`Canvas` の `Class` は Tailwind CSS 4 のクラスを受け付け、`base` コンポーネントのすべてのパーツにはスタイルと並んでクラスが用意されています。ユーティリティ、バリアント、任意値（arbitrary value）は、デフォルトのテーマとパレット上で Tailwind における意味と同じ意味を持ちます。

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

クラスはスタイルの下位にマージされるため、スタイルのフィールドが優先されます。クラスは `ClassFunc` を使って変化に追従させることができ、状態バリアント（`hover:bg-zinc-100`、`checked:opacity-100`）はスタイルの状態になります。`group` と `group-hover:` は親要素の状態から子要素のスタイルを設定し、`first:` と `last:` は子要素の位置に追従し、`aria-checked:`、`data-open:`、`focus-within:` はコントロールが示す状態に追従します。`pointer-fine:` と `pointer-coarse:` は、`cx.Env().Pointer()`（Web 上ではブラウザーの `pointer` クエリ、デスクトップと Mac では fine、スマートフォン・タブレット・ウォッチでは coarse、TV ではそのどちらでもない）を介して CSS の `pointer` メディア特性に一致させながらマウスやタッチスクリーン向けにサブツリーをスタイリングします。また `dark:` や `hover:` などの他のバリアントと組み合わせることができ、`core` が暗黙的に変更するのではなく、アプリがタッチ向けまたはポインター向けのサイズ設定を明示的に選択できます。

コントロールに `peer` を付けると、その状態に基づいて後続の兄弟要素のスタイルを設定できます。たとえば、`Class: "peer"` を持つチェックボックスの後に `Class: "peer-checked:text-accent peer-disabled:opacity-50"` を持つラベルを配置できます。peer は同じ親要素配下の後続の兄弟要素（`Show` や `Each` 内も含む）のみをスタイリングし、前の兄弟要素や自身内部のビューには適用されません。特定の組に名前を付けるには `peer/field` と `peer-checked/field:` を使用し、コントロールの真偽値データ状態 `invalid` には `peer-data-invalid:` を使用します。マーカーと状態はリアクティブに更新されます。ビューにマーカーを付けても Tab 移動先になったり押下を受け取ったりするようにはならないため、`peer-active:` はボタンのようにそれ自体が押下を受け取る peer にのみ追従します。

`has-[>svg]:px-3` は、直接の子要素としてアイコンを持つ場合に親要素のパディングを変更します。`[&_svg]:size-4 [&_svg]:shrink-0` は子孫アイコンのサイズを設定します。`svg` は条件付き領域（conditional region）を介する場合も含めて `core.Icon` と `SVGView` を選択します。これらのセレクターは、子要素・クラス・状態が変化すると更新されます。`hover:[&_svg]` は親要素のホバーに追従し、`[&_svg]:hover` は各アイコンのホバーに追従します。セレクターの `active` は `peer-active:` と同様に、ボタンなど押下を受け取るビューにのみ追従します。一致したセレクターはバリアント順よりも先に詳細度（specificity）で競合し、クラスに important が付かない限りインラインスタイルが優先されます。

セレクターはすべてのターゲット上で NSGo の要素に一致します。セレクターリスト、子結合子（`>`）、子孫結合子（`_`）、隣接兄弟結合子（`+`）、後続兄弟結合子（`~`）、クラス、`data-*` の存在・等価判定、ロール、サポートされている真偽値のアクセシビリティ状態、さらに `:is`、`:where`、`:not`、`:has`、および子要素の位置指定をサポートしています。要素型は `view`/`div`、`text`/`span`（テキストノード全体）、`scroll`、`input`、`textarea`、`image`/`img`、`canvas`、`svg`、`video`、`webview`、`native` です。任意セレクターの選択肢ごとにトップレベルの `&` を 1 つ使用してください。これがない場合、セレクターはその要素自身をテストします。ブラウザー専用のセレクター、擬似要素、アットルール（at-rule）、エスケープ、および関数内の明示的な `&` は未知の指定として扱われます。これらのセレクターは、バックエンド実装ビューや個々のテキストスパンには到達しません。

`Span.Class` や `Screen.HeaderClass` 内の `peer-*` バリアントには追従すべき兄弟要素がありません。これらは無視され、クラスとコンテキストごとに 1 回ログに記録されます。スパンの peer バリアントは、代わりにそれを含む `Text` に設定してください。

テーマの `Tailwind` フィールドはデフォルトテーマを色、ブレークポイント、ユーティリティで拡張し、パレットにない色を指定すると、ノードの現在の外観におけるテーマから読み取られる `Palette` ロール（`bg-accent`、`text-text-muted/70`）またはテーマカラーを参照します。

## テーマ

`core.Theme` はライトとダークの両方の外観をまとめた 1 つの見た目を表し、ライト用とダーク用の各 `Palette`、角丸の半径、フォントファミリーを持ちます。パレットのロールは `Background`、`Surface`、`Text`、`TextMuted`、`Border`、`Accent`、`AccentText`、`Danger`、`Warning`、`Success`、`Selection`、`Focus`、`Scrollbar` です。`Colors` には `inset` や `raised` など、ライブラリが追加するロールが格納されます。別のテーマを設定するまでアプリが表示する `core.DefaultTheme()` は青いアクセントと `ui` のロールを持つニュートラルなテーマであり、`ui.WarmDuskTheme()` は温かみのあるパレットです。

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` はそのスコープとその配下で構築されるすべての要素にテーマを設定するため、サイドバーやパネルに独自のテーマを持たせることができます。nil テーマを渡すと上位のテーマを引き継ぎます。`core.ParseTheme` は YAML または JSON のテーマファイル、あるいは shadcn/ui テーマファイルの CSS 変数からロールカラー、角丸の半径、フォントファミリーを読み込みます。テーマを設定するまでアプリが表示する `core.DefaultTheme` と `ui.WarmDuskTheme` は、そのまま変更可能なテーマを返します。

`cx.SetAppearance` はアプリ全体をライトまたはダークに強制し、`cx.SetSubtreeAppearance` はサブツリーに対してそれを行います。`core.AppearanceSystem` を指定すると再びシステム設定に従います。`cx.Dark()` はサブツリーがどちらの外観を表示しているかを返し、`cx.Palette()` は埋め込みホストによる上書きが適用されたパレットの色を返します。

どの祖先要素も `Style.Color` を設定していない場合、テキストと `currentColor` アイコンはサブツリーの `Palette.Text` を使用し、テーマ、外観、ホストトークンの変更にその場で追従します。`core.Transparent` を含む明示的な色が指定されている場合は、その色の継承が維持されます。

## コンポーネント内の色

コンポーネントは構築される場所とその `StyleFunc` 内で色を読み取るため、コンポーネントを再構築することなくテーマや外観の変更がその場で反映されます。`ui.ColorsOf(cx)` は `ui` が使用するロール名を提供し、`ui.TintsOf(cx)` はそれらの半透明の塗りつぶしを、`ui.FinOf(cx)` は `ui` のレイヤー化された仕上げ（layered finish）を提供します。

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` は内部で読み取った値が変化すると再実行されるため、その中にある `cx.Dark()` や `cx.Palette()` もテーマに追従します。これは `ClassFunc`、スパンの `StyleFunc`、およびコンポーネントの `StyleFunc` フィールドでも同様です。

## カーソル

`Style.Cursor` は名前付きのポインターカーソル（`core.CursorText`、`core.CursorColResize` など）を設定します。押下可能な要素はプラットフォームの矢印を維持するため `core.CursorAuto` のままにしてください。デスクトップや Web 上の描画ツール、ゲーム、カスタムキャンバスポインター向けには、`Style.CursorImage` に `*core.CustomCursor` を指定できます。これには 1x の `Image`、任意の HiDPI 用 `Image2x`（`PixelRatio >= 1.5` で自動選択）、画像境界内にクランプされる論理ポイント単位の `Hotspot`、および名前付きカーソルの `Fallback` を設定します。現在のターゲットがカスタムカーソル画像を表示するかどうかを確認するには `cx.Env().SupportsCustomCursor()` を使用してください。タッチプラットフォームでは無視されます。

## 次のステップ

- [レイアウト](layout.md): ボックスをどこに配置するか。
- [テキスト](text.md): フォント、スパン、装飾。
- [アニメーション](animation.md): スタイル間のトランジション。
