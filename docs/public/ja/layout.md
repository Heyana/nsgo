---
title: レイアウト
description: フレックスボックス、グリッド、サイズと位置指定、さらにレイアウト内のセーフエリアとキーボードの扱い。
group: コアコンセプト
order: 51
---

## ボックス

すべての要素はエンジンによってレイアウトされるボックスです。デフォルトは React Native と同じで、縦方向（column）のフレックスボックス、伸長・縮小の指定がない限りサイズを維持する子要素、交差軸方向に引き伸ばされるアイテム、そしてパディングとボーダーを含むサイズ計算になっています。

```go
core.View{
	Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.All(16), AlignItems: core.AlignCenter},
	Children: []core.Node{
		core.Text{Content: "One"},
		core.Text{Content: "Two"},
	},
}
```

`Column`、`Row`、`ColumnReverse`、`RowReverse` で主軸を設定し、`Wrap` と `WrapReverse` で子要素を複数行に折り返せます。`JustifyContent` は主軸に沿って子要素を配置し、`AlignItems`（または個々の子要素の `AlignSelf`）は交差軸に沿って配置します。要素間の間隔には `Gap`、`RowGap`、`ColumnGap` を指定し、`style` のパディングとマージンには `core.All`、`core.Symmetric`、`core.Edges` を使います。

## サイズ

`Width`、`Height`、`MinWidth`、`MaxWidth`、`FlexBasis` には、ポイント単位の `core.Pt` または親要素に対する割合の `core.Pct` を指定します。`FlexGrow` は余白を分配し、`FlexShrink` はボックスがスペースを譲って縮小できるようにします。どちらも指定しないボックスは元のサイズを保ちます。`AspectRatio` は幅を高さで割った比率です。`AutoMargin` は指定した辺の余白をすべて吸収するため、片側に指定するとボックスを反対側へ押し寄せます。

```go
core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
	core.View{Style: core.Style{Width: core.Pt(220), FlexShrink: 0}},
	core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}},
}}
```

幅や高さが別途指定されない限り、ボックスのサイズはコンテンツによって決まります。`core.MinContent`、`core.MaxContent`、`core.FitContent` は CSS と同名のサイズ指定で、たとえばテキストの min-content 幅は最も幅の広い単語の幅になります。行（row）内のテキストは `FlexShrink: 1` を指定したときのみ縮小して折り返し、残りのスペースを埋める必要がある場合は `FlexGrow: 1` も追加します。最小値が未設定のときは React Native と同様にすでに 0 であるため、`MinWidth: core.Pt(0)` を指定しても何も変わらず、行の幅が最も長い単語より狭い場合、縮小中のテキストは単語の途中でも改行されます。`MinWidth: core.AutoMinimum` を指定すると、CSS の自動最小サイズ（automatic minimum）と同様に、最も幅の広い単語の幅で縮小が止まります。

ウィンドウ全体を満たす画面には `FlexGrow: 1` と `FlexShrink: 1` を指定します。また、その画面とスクロールさせるリストやスクロールビューの間にある各ボックスにも同様に指定します。`FlexGrow` だけを指定すると、ボックスはコンテンツに合わせて伸びてしまうため、長いリストがスクロールせずにウィンドウを超えて引き伸ばされてしまいます。

## グリッド

`DisplayGrid` は子要素を CSS グリッドでレイアウトします。`GridTemplateColumns` と `GridTemplateRows` には CSS のトラック構文を指定し、`GridRow` と `GridColumn` はライン番号またはスパンによってアイテムを配置します。

```go
core.View{Style: core.Style{
	Display: core.DisplayGrid,
	GridTemplateColumns: "repeat(3, 1fr)",
	GridAutoRows: "44px",
	Gap: 12,
}, Children: []core.Node{
	core.View{Style: core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}}},
}}
```

`repeat(auto-fill, minmax(160px, 1fr))` を使うと、収まるだけの列数が自動的に作られます。`GridFlow` はグリッドを埋める順序を選択し、`JustifyItems`、`JustifySelf`、`AlignItems` はセル内でのアイテムの位置を決めます。

## 位置指定

デフォルトは `Relative` で、ボックスを通常のフロー内に保ったまま `Top`、`Right`、`Bottom`、`Left` でオフセットします。`Absolute` はボックスをフローから外し、最も近い位置指定済み（positioned）の先祖を基準に配置します。`Fixed` は何がスクロールするかに関わらずルートビューまたはそのモーダル内に配置し、`Sticky` は最も近いスクロールビュー内の親要素の内側に固定します（`Top` で上端から、または `Top` が auto の場合は `Bottom` で下端から固定し、水平方向は `Left` と `Right` を使います）。`Position: core.Sticky, Bottom: core.Pt(0)` を持つフッターは、コンテンツが長いときはスクロール中も下端に留まり、コンテンツが短いときはその直後に流れに沿って配置されます。`ZIndex` は兄弟要素間でのボックスの重なり順を決めます。

```go
core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Right: core.Pt(8)}}
```

`Static` はボックスをフロー内に保ち、インセット指定を無視するため、絶対配置（`Absolute`）された子要素はさらに上位の先祖を基準に探します。`Overflow` は子要素をボックスの範囲でクリップし、`DisplayNone` はボックスをレイアウトから除外します。

ボックス自身の外側のシャドウとアウトラインは、CSS と同様にクリップの外側に描画されます。これは Android、Windows、Linux、Web、Apple の各プラットフォームで、角ごとに異なる半径を持つ場合やズームされたスクロールコンテンツを含むあらゆる種類のビューに対応しています。iPhone 実機での確認が完了するまでは、`ui.Dialog` の `Bare` は引き続き同じ角丸を持ちボーダー分だけ内側に寄せた内側ビューにクリップを適用しており、これによりすべてのターゲットでシャドウとアウトラインが維持されます：

```go
core.View{Style: core.Style{CornerRadius: 16, Shadows: []core.BoxShadow{ui.Shadow(0, 8, 24, -6, ui.Black(0.18))}},
	Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, CornerRadius: 16, Overflow: core.Hidden}, Children: content}}}
```

## 方向と環境

`LayoutDirection` はアプリの方向を上書きしてサブツリーを左から右または右から左にレイアウトします。`DirectionRTL` と `DirectionLTR` で指定でき、デフォルトは継承です。マージン、パディング、ボーダー、インセットの左右はレイアウト方向に応じて入れ替わるため、React Native と同様に start と end として扱われます。`PhysicalEdges` と `PhysicalBorders` は指定した辺の位置をそのまま維持します。`WritingDirection` は異なる方向が混在するテキストの並び順を制御し、`WritingAuto` は最初の強い方向性を持つ文字（strong letter）から方向を判定します。

`PaddingEnv` と `MarginEnv` は各辺に環境値を加算します。`core.EnvSafeArea` はその辺のセーフエリアのインセット、`core.EnvKeyboard` はソフトウェアキーボードの高さを表し、キーボードの移動アニメーションに合わせてコンテンツをキーボードの上側に保ちます。

```go
core.View{Style: core.Style{
	Padding: core.All(16),
	PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvSafeArea},
}}
```

`core.EnvAll(core.EnvSafeArea)` ですべての辺をまとめて指定でき、コード内からは `cx.Env().SafeArea()` と `cx.Env().KeyboardHeight()` で値を読み取れます。

折りたたみ式またはデュアルスクリーンのスマートフォンでは、`cx.Env().Geometry()` が折り目（fold）やヒンジによってウィンドウが分割された領域（それぞれに含まれるセーフエリア付き）とその間の折り目の一覧を返します。また、カバーディスプレイや平らに開かれた画面のようにウィンドウを横切る折り目がない場合でも、その `State` と `Angle` によってデバイスがどの程度開いているか（`core.HingeClosed`、`HingeHalfOpen`、`HingeOpen`）がわかります。その他のデバイスでは 1 つの領域と `HingeUnknown` が報告されます。iOS 27.1 では、`cx.Env().VerticalBarEdge()` がシステムによって選ばれた縦型バーの辺（`core.VerticalBarLeading`、`core.VerticalBarTrailing`、または `core.VerticalBarUnspecified`）を報告し、`Edge` が `mobile.TabBarAutomatic` のときに `mobile.TabBar` と `ui.TabShell` がそれに従います。ジオメトリによって暗黙的にレイアウトが行われることはなく、ペインの配置はアプリ側で選択します。

## レスポンシブクラス

`Class` には `flex`、`flex-col`、`flex-wrap`、`items-center`、`justify-between`、`gap-3`、`p-4`、`w-full`、`grid`、`grid-cols-3`、`col-span-2` などの Tailwind CSS クラスを指定できます。ブレークポイントはウィンドウに追従し、ディスプレイの外観、レイアウト方向、視差効果を減らす設定などは環境に追従します：

```go
core.View{Class: "flex flex-col gap-3 p-4 md:flex-row md:gap-6 dark:bg-zinc-900 rtl:flex-row-reverse"}
```

`sm:`、`md:`、`lg:`、`xl:`、`2xl:` はそれぞれ幅 40、48、64、80、96 rem 以上で適用され、`max-md:` はその幅未満で適用されます。`portrait:`、`dark:`、`rtl:`、`motion-reduce:`、`contrast-more:`、`pointer-fine:`、`pointer-coarse:`、`ios:`、`android:`、`web:` は環境の報告に従います。クラスは `Style` の下位としてマージされ、状態のクラス（`hover:`）は状態スタイルになるため、両者を組み合わせて使えます。

## 次のステップ

- [スタイルとテーマ](styles.md): 色、角丸、シャドウ、サブツリー全体の見た目。
- [テキスト](text.md): テキストのサイズ決定と折り返しの仕組み。
- [リストとスクロール](lists.md): スクロール可能なコンテンツと行のリサイクル。
