---
title: ライブアクティビティ
description: ロック画面や Dynamic Island、また Android では Live Updates としてリアルタイムの状況を追従します。
group: プラットフォーム機能
order: 48
---

## 有効にする

ライブアクティビティ（Live Activity）は、配達状況やタイマーなど、リアルタイムに進行する出来事を追従します。iOS 16.1 以降の iPhone ではロック画面と Dynamic Island に表示され、Android 16 では Live Update として通知シェードの上部とステータスバーのチップに表示されます（それ以前の Android では継続中の通知として表示されます）。`nsgo.yaml` で有効にしてください。

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: ユーザーが許可した範囲で、システムの更新予算より多くの更新を行う
```

## 開始、更新、終了

`widget.StartActivity` は指定した種類（kind）のアクティビティを開始します。種類は追従対象に名前を付けるもので、ウィジェットの種類と同様に記述し、`widget.ActivityContent` を表示します。`Activity.Update` は表示内容を置き換え、`Activity.Alert` は表示内容を置き換えたうえでユーザーに通知（アラート）を出し、`Activity.End` はアクティビティを終了します（nil でない場合は最終コンテンツを表示し、`Dismissal` の指定に従ってロック画面から消去します）。`widget.Activities` はアプリで実行中のアクティビティを以前のアプリ起動時のものも含めて一覧表示し、`widget.ActivitiesEnabled` はユーザーとプラットフォームがライブアクティビティを許可しているかどうかを返します。

```go
a, err := widget.StartActivity("delivery", widget.ActivityContent{
	Title: "Your order", Body: "On its way", Symbol: "box.truck.fill",
	Timer: arrival, ShowProgress: true, Progress: 0.4,
	Actions: []widget.ActivityAction{{ID: "pause", Title: "Pause", Symbol: "pause.fill"}},
})
if err != nil {
	return
}
a.Update(widget.ActivityContent{Title: "Your order", Body: "Arrived", Status: "Now"})
a.End(nil, widget.DismissAt(time.Now().Add(2*time.Hour)))
```

`StartActivity` は、ライブアクティビティを含めずにビルドされたアプリでは `widget.ErrNoActivities` を、iOS 16.1 以降および Android 以外では `widget.ErrActivitiesUnsupported` を、ユーザーがオフにしている場合は `widget.ErrActivitiesDisabled` を返します。また、終了後は `Update` と `End` が `widget.ErrActivityEnded` を返します。アクティビティのコンテンツは、ActivityKit および APNs の仕様どおり最大 4 KB です。

## 表示内容

両プラットフォームとも、タイトルと本文、ステータスまたはシステムが進めるタイマー、プログレスバー、最大 3 つのアクション、アクセントカラーを表示します。iOS ではウィジェットエクステンションでこれらを描画し、ロック画面および Dynamic Island の各領域に対して、それらの代わりに `widget` のビューを受け取ります。展開表示の `LockScreen`、`Leading`、`Trailing`、`Center`、`Bottom`、コンパクト表示の `CompactLeading` と `CompactTrailing`、複数のアクティビティが共有する際の `Minimal` があります。Android では Live Updates がアプリのビューを受け取らないため、これらのフィールドのみをアプリアイコンとともに表示します。

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` はアクティビティがタップされたときにアプリで開かれ、`Stale` は iOS がコンテンツを古いものとして表示する時刻を指定し、`Background` はロック画面上のアクティビティの背景を塗りつぶします。

## 押下イベント

`widget.OnAction` は、アクションまたは `widget.Button` が押されたときのハンドラーを、対象のアクティビティとアクションの ID とともに実行します（[ウィジェットボタン](widgets.md#buttons)の場合、`Activity` は nil になります）。これは `main` 内の `nsgo.Run` より前に設定してください。iOS では、ボタンが押されるとシステムがビューなしでアプリをバックグラウンド起動し、ハンドラーが戻るまでアクティビティを保持するため、ハンドラー内でアクティビティを更新できます。Android では、Activity を開くことなくランタイムレシーバー経由で実行中の Go プロセスに押下イベントが届きます。アクションが動作するには、それを投稿したエンジンが実行され続けている必要があります。そのエンジンが終了した後の古いアクションは何も行いません。通知自体をタップした場合は引き続きアプリが開きます。ハンドラーはアプリのビューのゴルーチンではなくシステム側のゴルーチン上で実行されるため、ビューへアクセスするには `core.Scope` の `Post` を使用します。

```go
func main() {
	widget.OnAction(func(p widget.ActivityPress) {
		if p.Action == "pause" {
			p.Activity.Update(paused)
		}
	})
	nsgo.Run(Root)
}
```

iOS 17 以降では、アクティビティのビューでも同様に処理できます。`widget.Button{Action: "pause"}` はアプリを開かずに同じハンドラーを実行し、押下するとアプリを開くのみの場合には子要素だけを表示します。

## プッシュ更新

バックグラウンドにあるアプリは実行中しかアクティビティを更新できないため、外部で起きている出来事を追従するアプリでは `widget.ActivityOptions{Push: true}` を指定してアクティビティを開始し、`widget.OnPushToken` で受け取ったトークンをサーバーへ送信し、サーバーから APNs の `liveactivity` 更新をプッシュします。このときの `content-state` には、コンテンツに対する `widget.ActivityState` の戻り値を指定します（プッシュに画像は含められません）。iOS 17.2 以降では、属性タイプ `widget.ActivityAttributesType` と種類・ID の属性を使ってプッシュからアクティビティを開始するためのトークンも取得できます。プッシュを使用するには、アプリのプロファイルにプッシュ通知のエンタイトルメントが必要です。

```go
state, err := widget.ActivityState(content)  // プッシュの content-state
```

## 制限事項

システムは 8 時間後にアクティビティを終了します。`widget.DismissDefault` は、終了したアクティビティを iOS では最大 4 時間ロック画面に残し、Android では即座に消去します。`widget.DismissNow` は即座に消去し、`widget.DismissAt` は 4 時間以内の指定時刻に消去します。Android は `POST_NOTIFICATIONS` と `POST_PROMOTED_NOTIFICATIONS` 自体を要求するため、最初に `cx.RequestNotificationPermission` でユーザーに許可を求めてください（[通知](notifications.md)）。ギャラリーの Widgets デモおよび `examples/widgets` で動作例を確認できます。

## 次のステップ

- [ウィジェット](widgets.md): 同じエクステンションによって描画される、ホーム画面とロック画面のタイムライン。
- [通知](notifications.md): Android が最初に求めるパーミッション。
- [ビルドと配信](building.md): エクステンションと、それに必要なエンタイトルメント。
