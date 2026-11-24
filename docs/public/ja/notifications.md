---
title: 通知
description: 通知の許可リクエスト、通知の表示・スケジュール・削除、件数のカウント、アプリアイコン上のタスク進捗表示、および押下の検知を行います。
group: プラットフォーム機能
order: 45
---

## 許可

`cx.RequestNotificationPermission` は、アプリが通知を表示してよいかをユーザーに尋ねます。初回はシステムのプロンプトが表示され、`core.NotificationPermission`（`NotificationsGranted`、`NotificationsDenied`、またはプラットフォームやビルドが通知表示に対応していない場合は `NotificationsUnavailable`）を返します。ユーザーが通知を必要とする操作を行ったタイミングでリクエストしてください。iOS と Android では一度しか尋ねられず、ブラウザーはユーザー操作に起因しないプロンプトを拒否します。`cx.NotificationPermission` は、尋ねることなく現在の回答を読み取ります。

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## 通知の表示

`cx.ShowNotification` はアプリが前面にある間も通知を表示し、同じ `ID` の通知があれば置き換えます。`Silent` を指定すると、システムのサウンドとバイブレーションを省略します。システムが通知を受け付けたかどうかが `done` に渡され、許可がない場合は false になります。ID またはタイトルがない通知に対しては、`done` を呼ばずに `core.ErrNotification` を返します。`cx.CloseNotification` は、指定した ID の通知を削除します。

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` は、アプリが表示したすべての通知を削除し、スケジュール済みの通知をキャンセルします。macOS、iOS、watchOS、Windows、Android では、過去の起動で通知センター、アクション センター、または通知シェードに残ったものも対象に含まれます。Linux と Web では、今回の実行で表示またはスケジュールしたもののみに作用します。通知が伝えていた内容をウィンドウに表示している状態でアプリが前面に来たときに呼び出してください：

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## 通知のスケジュール

`At` に現在より後の時刻が指定された通知はスケジュールされ、同じ `ID` で `cx.ShowNotification` を再度呼び出すと時刻が変更され、`cx.CloseNotification` を呼び出すと表示前にキャンセルされます。`Repeat` を指定すると、デバイスのタイムゾーンにおいて次に該当する時刻から、毎日 `At` の時刻に、または毎週同じ曜日の同じ時刻に表示されます。`cx.Env().CanScheduleNotifications()` はプラットフォームがスケジュールに対応しているかを返します。バンドルされた macOS、iOS、watchOS アプリ、Android、および Windows は、アプリが起動中かどうかにかかわらずネイティブにスケジュールを行います（Windows ではアプリ実行中に日次・週次の繰り返しを連鎖させ、起動時に再設定します）。一方、Linux と Web（`Notification` が利用可能な環境）では、アプリまたはページが開いている間だけ保持されます。それ以外の環境（通知センターにバッジのみを表示する tvOS やミニプログラム）では、`done` に false が渡されます。

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

Android では、端末が Doze 状態のとき、スケジュールされた通知が指定時刻の数分後に表示されることがあります。目覚まし時計のように分単位の正確さを必要とするアプリは、`android.permissions` に `android.permission.SCHEDULE_EXACT_ALARM`（Android 12 以降はユーザーがシステム設定で許可）または `USE_EXACT_ALARM`（Google Play ではアラームアプリとカレンダーアプリにのみ許可）を指定します。強制停止された Android アプリの通知は、次回起動時まで待機します。プロセス終了後の配信をテストするには、`ShowNotification` が true を返すのを待ってから、パッケージを強制停止せずにプロセスを終了してください。

## 通知の押下

`cx.OnNotificationPress` は、設定したコンポーネントが存在する間にユーザーが通知を押下した際、その通知の `ID` を引数に関数を実行し、アプリを前面に表示します。アプリを起動するきっかけとなった押下は、最初に登録されたハンドラーに届きます。押下によってアプリが新規起動されることがあるため、押下で開く対象を表す情報（`"chat:42"` など）を `ID` に含めておいてください。Windows では、インストーラーによってスタートメニューに登録されたアプリでこの動作が行われます。それ以外の方法で起動されたアプリは、実行中にのみ押下を受け取ります。

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Android 13 以降で通知をリクエストする Android アプリは、`nsgo.yaml` の `android.permissions` に `android.permission.POST_NOTIFICATIONS` を記載します。

## バッジ

`cx.SetBadge` はアプリアイコンに件数を表示します。対象は、macOS の Dock、iOS のホーム画面（通知の許可が必要）、Windows のタスクバーボタン、Unity 互換の Linux ランチャー、およびインストール済み Web アプリのアイコンです。0 以下の値を指定するとバッジが消去されます。macOS アプリでは、通知の許可を確認・リクエストするか通知を表示するまでは Dock バッジに許可は不要ですが、一度でもそれらを行うと、その実行中および以降の実行では、ユーザーが「システム設定」でそのアプリのバッジを許可している場合にのみ Dock にバッジが表示されます（`cx.RequestNotificationPermission` はアラートとともにこの許可も求めます）。Linux のバッジインターフェースを持たないランチャーや Android には件数を直接設定する仕組みがなく、Android のランチャーは通知が表示されているアプリにマークを付けます。

```go
cx.SetBadge(len(unread.Get()))
```

## タスクの進捗

`cx.SetTaskProgress` は、時間のかかるタスクの進捗状況をアプリアイコン上に表示します。macOS では Dock タイル上のバー、Windows ではタスクバーボタン、Linux では Unity 互換ランチャー上に表示されます。許可は不要で、バッジと並べて表示できます。`TaskProgressNormal` は 0 から 1 までの割合である `Value` を表示し、`TaskProgressPaused` と `TaskProgressError` は Windows と macOS でバーに色を付けます。`TaskProgressIndeterminate` は Windows ではアニメーションし、macOS と Linux では半分ほど満たされた静止バーになります。ゼロ値の `TaskProgress` を渡すと表示が消去されます。Windows ではスコープが属するウィンドウのボタンに表示され、アプリ自身のスコープやトレイパネルのように独自のボタンを持たない場所から設定された進捗は、個別に設定していないアプリのすべてのウィンドウに表示されます。iOS、Android、Web では何も表示されません。

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## 次のステップ

- [権限](permissions.md): マイクやアクセシビリティなど、システムのその他の権限。
- [触覚フィードバック](haptics.md): ユーザーの指先へデバイスが返すフィードバック。
- [プラットフォームサービス](services.md): アラート、クリップボード、ファイル、ストレージ、ネットワーキング。
- [ライブアクティビティ](live-activities.md): ロック画面や Dynamic Island でアプリが追跡する情報。
- [ビルドと配信](building.md): パッケージが必要とする権限。
