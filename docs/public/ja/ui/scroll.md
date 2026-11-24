---
title: スクロール
description: ページング、スナップ、ズーム、更新機能を備えた、プラットフォームのスクロールビュー。
group: コンポーネント
order: 39
---

`core.Scroll` はプラットフォーム自身のスクロールビューです。その子要素は縦一列に、あるいは `Horizontal` を指定した場合は横一行にレイアウトされ、コンテンツがスクロールする間、コンテナいっぱいに広がります。

```go preview=scroll/basic height=240
core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
	core.View{Style: core.Style{Padding: core.All(16), Gap: 12}, Children: rows},
}}
```

パディングはスクロールビューではなくコンテンツ側に設定するため、スクロールバーは端に沿って表示されます。スクロールビューは、スタイルで別段の指定がない限り親に収まるように縮小します。兄弟要素が縮小する間も要求されたサイズを維持する必要がある場合は、`NoShrink` を設定してください。

## 振る舞い

`Behavior` はスクロールの動作を指定します。`Paging`、`SnapToAlignment` を伴う `SnapToInterval` または `SnapToOffsets`、`DecelerationRate`、`NoBounces`、`ContentInset`、およびキーボードのルールを指定できます。

```go preview=scroll/paging height=240
core.Scroll{
	Behavior: core.ScrollBehavior{
		Paging:                    true,
		KeyboardDismissMode:       core.KeyboardDismissOnDrag,
		KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
	},
	Children: pages,
}
```

`HideIndicator` はプラットフォームのスクロールバーを非表示にし、`Bar` はアプリ独自のスクロールバーを描画します。`MinZoomScale` と `MaxZoomScale` はピンチ操作によるズームを有効にし、`NoPinchZoom` はピンチズームを無視します。`CenterContent` はビューポートより小さいコンテンツを中央に配置します。

## 更新とイベント

`OnRefresh` を設定するとビューが更新可能になり、処理の実行中は `Refreshing` を設定します。`PullValue` はアプリ独自のインジケーター用にコンテンツがどれだけ引っ張られたかに追従し、`Behavior.RefreshIndicator` はその高さを設定します。

```go
refreshing := core.NewSignal(cx, false)
core.Scroll{
	Refreshing: refreshing.Get,
	OnRefresh:  func() { refreshing.Set(true); reload(cx) },
	Children:   content,
}
```

`OnScroll` は 1 ターンにつき最大 1 回、または `ScrollEventThrottle` ごとに 1 回オフセットを報告します。`Stale` は、コードからのスクロールがプラットフォームに適用される前のイベントであることを示します。`OnScrollDragStart`、`OnScrollDragEnd`、`OnMomentumStart`、`OnMomentumEnd`、`OnScrollEnd` はユーザーのジェスチャーの開始と終了を挟んで呼ばれます。

`OffsetValues` は、フレームごとのコミットなしに折りたたまれるヘッダーなどのために、メインスレッド上でオフセットに追従します。`Ref` は `ScrollTo`、`ScrollToEnd`、`ZoomTo` を使ってコードからスクロールし、内部の任意のビューの ref は `Reveal` を使って周囲のすべてのスクロールビューをスクロールさせながら自身を表示領域に入れます。

```go
row.Reveal(core.RevealOptions{Vertical: core.ScrollAlignCenter, Animated: true})
```

一方、`RevealBox` はテキストの `TextRangeBoxes` から得られる単語の矩形など、ビュー内の特定の矩形領域を表示領域に入れ、`Margin` は上に重なるバーのために最も外側のスクロールビューの端に余白を確保します。

キーボードによってフォーカスが移されたビューは、自動的に表示領域内へスクロールします。

## レイアウト

`Horizontal` を指定すると横方向にスクロールします。`Behavior.ContentInset` はコンテンツと一緒にスクロールする余白を周囲に設けます。セーフエリアまで広がるページの場合は、スクロールビューではなくコンテンツに `PaddingEnv` を使用してください。

## アクセシビリティ

`Label` はスクロール領域に名前を付け、`Role` はリストボックスなどそれが何であるかを示します。プラットフォームのスクロールジェスチャー、キーボードスクロール、およびスクリーンリーダースクロールはそのまま維持され、`Busy` はビューがまだ読み込み中であることを報告します。

## プラットフォーム

すべてのバックエンドがプラットフォームのスクロールビューを使用しているため、フリング、バウンス、スクロールバー、タッチスクロールはネイティブで動作します。端のエフェクトはプラットフォームによって異なり、macOS と iOS はラバーバンド、Android はストレッチ、Windows と Linux は停止、Web はブラウザーの挙動に従います。`EdgeEffects` と `EdgeBar` は、アプリが望む箇所でコンテンツの上にバーを追加します。iOS および Android のスマートフォンとタブレットでは、あふれている垂直・水平スクロールビューのアクティブな軸に沿って、コンテンツ全範囲をスクラブできるタッチドラッグ可能なスクロールハンドルが表示されます。あふれずに収まる短いスクロールビュー、ズーム可能なビュー、`Behavior.NoScroll`、`HideIndicator`、およびカスタム `Bar` や `base.SetScrollbarTheme` を使用しているビューでは、このハンドルは非表示になります。テストでは、`nsgotest.App.ScrollHandle` がそのビューポート境界を報告し、`DragAt` または `Touch` で直接ドラッグできます。Windows では、オーバーレイインジケーターはホイール、タッチ、バードラッグおよびそれらの慣性スクロール時に表示され、アプリ側が呼び出す `ScrollTo`、`ScrollBy`、リビール、インセット、アニメーションでは表示されません。

## 関連項目

- [リストとスクロール](../lists.md): スクロールとリストの完全なガイド。
- [リスト](list.md): 行を再利用するリスト。
- [レイアウト](../layout.md): パディングとセーフエリア。
- ギャラリーの Scroll および Lists ページ。
