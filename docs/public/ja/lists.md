---
title: リストとスクロール
description: ネイティブジェスチャーを備えたスクロールビューと、任意の数のアイテムに対して行を再利用するリスト。
group: コアコンセプト
order: 55
---

## スクロールビュー

`core.Scroll` はプラットフォーム自身のスクロールビューであるため、フリング、バウンス、スクロールバー、スナップ、タッチスクロールはネイティブで動作します。その子要素は縦一列に、あるいは `Horizontal` を指定した場合は横一行に、必要な長さだけレイアウトされます。

```go
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

スクロールビューはコンテナいっぱいに広がり、パディングはコンテンツ側に設定されるため、スクロールバーは端に沿って表示されます。スタイルで別段の指定がない限り、親に収まるように縮小されます。兄弟要素が縮小する間も要求されたサイズを維持する必要がある場合は `NoShrink` を設定してください。`HideIndicator` はプラットフォームのインジケーターを非表示にし、`Bar` はアプリ独自のスクロールバーを描画します。

`Behavior` と `BehaviorFunc` はスクロール方法を選択します。`Paging`、`SnapToAlignment` を伴う `SnapToInterval` と `SnapToOffsets`、`DecelerationRate`、`NoBounces`、`ContentInset`、およびキーボードのルールを指定できます。`MinZoomScale` と `MaxZoomScale` はピンチによるズームを有効にし、`OnRefresh` と `Refreshing` は引っ張って更新（プル・トゥ・リフレッシュ）を実装し、`OffsetValues` はスクロールに合わせて動くエフェクトのためにメインスレッド上でオフセットに追従します。

```go
page := core.Scroll{
	OnScroll: func(e core.ScrollEvent) { y.Set(e.Offset.Y) },
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: []core.Node{...},
}
```

`OnScroll` は 1 ターンにつき最大 1 回、または `ScrollEventThrottle` ごとに 1 回オフセットを報告します。`OnScrollDragStart`、`OnScrollDragEnd`、`OnMomentumStart`、`OnMomentumEnd`、`OnScrollEnd` はユーザーのジェスチャーの開始と終了を挟んで呼ばれます。`Ref` を使うと、`ScrollTo`、`ScrollToEnd`、`ZoomTo`、およびリスト用の `ScrollToIndex` を使ってコードからスクロールできます。

## リスト

`core.List` は、画面付近の行だけを作成しながら、ネイティブのスクロールビュー上で長いデータを描画します。`Count` で行数を指定し、`Row` で 1 行を構築します。`Key` は各行に識別子を与えるため、上方の行が出入りしても、残る行はそのセルを保持します。

```go
core.List{
	Count:         func() int { return len(items.Get()) },
	Key:           func(i int) any { return items.Get()[i].ID },
	EstimatedSize: 56,
	Row: func(cx *core.Scope, r core.ListRow) core.Node {
		return core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return items.Get()[r.Index()].Title }},
		}}
	},
}
```

`Row` は再利用されるセルごとに 1 回実行され、そのセルが別の行に再利用されるときに `r.Index()` を更新します。そのため、行を構築する際に一度きりの値をキャプチャするのではなく、リアクティブ関数（`ContentFunc`、`StyleFunc`、`NewMemo`）の内部で `r.Index()` とアイテムのシグナルを読み取ってください。待機中のセルはパークされるため、その計算はセルが再びアクティブな行にバインドされるまで待機します。

`Header`、`Footer`、`Empty`、`Separator` は、行の前後および行間に配置するコンテンツを保持します。`NumColumns` は行を横に並べて配置し、`Gap` と `ColumnGap` はそれらの間隔を空け、`Horizontal` は横方向にスクロールします。`StickyHeaderIndices` は、次のスティッキーヘッダーに押し出されるまでそれらの行をビューポートの先頭に固定し、`StickyHeaderOffset` はその上にあるヘッダーと重ならないように余白を保ちます。

`OnViewableItemsChange` は `ViewabilityConfig` のしきい値に従って表示中の行を報告し、`OnLoadMore` はビューポートが末尾に近づくと、その行数に対して 1 回実行されます。`OnLoadMoreAtStart` は先頭側で同じ動作を行い、`InitialScrollIndex` は指定した行の位置からリストを開きます。`Alignment: core.ListAlignEnd` は、行が読み順になっているチャットのようにリストを末尾に配置します。最新の行で開き、ユーザーが末尾にいる間は新しい行に追従し、短い会話は下端に留めます。一方、`Inverted` は行 0 を末尾に配置し、`MaintainVisibleContentPosition` はユーザーが末尾付近にいる間に伸びていくリストの末尾に追従します。

```go
core.List{Ref: list, Count: count, EstimatedSize: 64, Row: row, OnLoadMore: loadMore,
	StickyHeaderIndices: headers(cx), Footer: core.View{Style: core.Style{Height: core.Pt(24)}}}
```

`list.ScrollToIndexAt(index, 0.5, 0, true)` は行をビューポートの中央へスクロールし、`list.ScrollToEnd(true)` は末尾へスクロールします。`list.ListPosition()` は読書位置を保持するために、ビューポートの先頭がどの行のどの位置にあるかを読み取ります。`list.ScrollToIndexAt(p.Index, 0, -p.Offset, false)` を使うと、新しく作り直されたリストを元の位置に戻せます。リストは他のコンテンツと同じページ内に配置することもできます。`Viewport: core.ListViewportParent` を設定すると、周囲のスクロールビューが他のコンテンツと一緒にリストをスクロールします。

## テーブル

`base/datatable` は、行と列の仮想化、スティッキーヘッダー、他の列が下をスクロールする間に左（`Fixed`）または右（`FixedRight`）に固定される列、ソート、単一行または複数行の選択、列のサイズ変更と移動、アプリ独自のセルを備えた 100 万行のテーブルを表示します。`MultiSelect` は Command または Control キーによるトグル、Shift キーによる範囲選択、Shift を使ったキーボードでの選択範囲拡張に対応し、`SelectedRows` と `OnSelectedRowsChange` によってアプリから行を制御できます。`CellTooltip` はセルにツールチップを付け、スクリーンリーダーに対してもそのセルを説明します（スタイルは `Styles.CellTooltip` で設定します）。`RenderCell` が描画する内容はポインターイベントを受け取らないため、セル内の `ui.Tooltip` は表示されません。`base.Table` は小規模なデータ向けのシンプルなテーブルで、`base/plot` はチャートを描画します。

ロケールやアプリケーションの状態に追従するグリッド名には `DataTable.LabelFunc` を使用します。`ColumnGroup.LabelFunc` は、グループが表示されている間にグループ化された列見出しを更新します。

## 次のステップ

- [アニメーション](animation.md): スクロールビューに追従するエフェクト。
- [テキスト](text.md): 折り返すテキストの行。
- [ナビゲーション](navigation.md): コンテンツがスクロールする画面。
