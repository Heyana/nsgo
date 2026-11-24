---
title: ドラッグ＆ドロップ
description: 1つのウィンドウ内のビュー間でアプリケーションの値を転送する。
group: コアコンセプト
order: 57
---

`base/dragdrop` は、1つのウィンドウ内の同じ `Area` にあるソースとターゲットを結び付けます。アプリ側は独自のビュー、スタイル、選択状態、そして値を処理するアクションをそのまま維持できます。ソースはデフォルトで 4 ポイント移動するとドラッグを開始するため、通常のタップ・クリック操作は損なわれません。

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` はドラッグ開始時に一度だけ実行されます。複数の選択項目をまとめて運ぶにはスライスを返し、スナップショットとして保持したい場合はその中でミュータブルなデータをコピーしてください。`Data` が nil の場合ドラッグは無効になります。`Threshold` は開始に必要な移動量を設定します。`Activation: dragdrop.AfterLongPress` を指定すると、まず `View.DelayLongPress`（デフォルト 500 ms）を待ってから移動を判定するため、スワイプで先にスクロールさせたい場面では明示的に選択してください。

`View` は押下ハンドラやアクセシビリティを含め通常の props をそのまま保ちます。ソースは `View.Responder` を占有するため、そこには何も設定しないでください。子要素に通常のアクションを持つボタンを含めることは可能です。`Preview` は入力を受け取ったりソースを再マウントしたりせずにポインタに追従し、指定しない場合はソースのラベルが表示されます。ソースにはラベルかプレビューのいずれかを必ず設定してください。ネストされたエリアはそれぞれ独立して動作します。

ターゲットが値を受け入れるには `OnDrop` が必要です。最も内側にあるターゲットが最初に判定を受け、`Accept` が拒否した場合は受け入れ可能な祖先のターゲットが引き取ることができます。`OnEnter`、`OnMove`、`OnLeave` でターゲットの外観を更新できます。各イベントは `Data`、ターゲット内の `Position`、サーフェス内の `PagePosition` を運びます。アクティブなターゲットを無効化または削除すると entered 状態がクリアされ、ドロップ時（離したとき）には再度受け入れ可否がチェックされます。別のビューの後ろにあるターゲット、クリップされているターゲット、無効化されたターゲット、またはモーダルの下にあるターゲットはドロップを受け取れません。

`OnStart` と `OnEnd` はソースのセッションを通知します。`OnEnd` は `Result{Data, Dropped, Target}` を受け取ります。Escape キー、ネイティブのタッチキャンセル、ソースの無効化または削除、あるいは受け入れ可能なターゲットの外で離した場合はドロップせずに終了します。ターゲットの配置には最後にコミットされたジオメトリが使われ、コミット間のネイティブアニメーションはサンプリングされません。これはウィンドウ内での値の転送用であり、システムファイルには引き続き `OnFileDrop` を使用します。

同じ操作を実行できるメニュー、キーボードコマンド、またはアクセシブルアクションも併せて用意してください。ドラッグはその操作へのショートカットとして扱います。テストでは `app.DragTo("id:poster", "id:playlist")` または選択したサーフェスの `DragTo` を使い、TSX テストでは `await app.dragTo("id:poster", "id:playlist")` を使います。どちらのビューもあらかじめ表示中のウィンドウ内に収まっている必要があり、このステップはドロップを合成したりドラッグ中にスクロールしたりせず、レスポンダーを通じてタッチシーケンスを送信します。[テスト](testing.md)も参照してください。
