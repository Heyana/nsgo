---
title: ピクチャ・イン・ピクチャ
description: iOS、tvOS、macOS、Android、およびブラウザのピクチャ・イン・ピクチャで動画を再生するか、iOS でカスタムツリーを表示します。
group: プラットフォーム機能
order: 45
---

iOS でピクチャ・イン・ピクチャを使用する前に、バックグラウンド再生を宣言してください。macOS ではバックグラウンドモードは不要です。

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

動画では、`PictureInPicture` を有効にし、`core.Ref` を保持して、アプリのコントロールから開始または停止します。状態コールバックを追跡して、`Possible` が true になったときにそのコントロールを有効にします。

```go
ref := &core.Ref{}
state := core.NewSignal(cx, core.PictureInPictureState{})
video := core.Video{
    Source: "asset:clip.mp4", Ref: ref,
    PictureInPicture: true, AutoPictureInPicture: true,
    OnPictureInPicture: state.Set,
}
start := ui.Button{
    Text: "Picture in Picture", OnPress: ref.StartPictureInPicture,
    Disabled: func() bool { return !state.Get().Possible },
}
```

`AutoPictureInPicture` は、iOS アプリがバックグラウンドに移行したときの自動開始を有効にします。tvOS と macOS ではこのオプションは無視されます。`Controls` は引き続きネイティブの再生コントロールを表示し、アプリの PiP ボタンはコントロール付き動画に対して同じ管理されたセッションを使用します。`OnPictureInPictureRestore` を使うと、ユーザーが戻ったときにアプリがプレーヤーページを表示できます。PiP が有効な間は動画をマウントしたままにしてください。そのスコープを削除すると再生と PiP が停止します。

Android では、パッケージ化されたアクティビティはすでに PiP サポートを宣言しています。動画はアプリの残りのインターフェースを非表示にしてアクティビティの PiP ウィンドウに入り、元に戻るときもプレーヤーと再生位置を維持します。`StopPictureInPicture` はその同じアクティビティを展開します。ウィンドウの再生/一時停止コントロールは `OnPlay` と `OnPause`（`VideoPauseControls` を伴う）を報告し、`Ref.Seek` は同じプレーヤーを使用します。自動開始には明示的な設定と、準備済みで再生中の動画が必要です。システムの現在の状態によっては開始に失敗することもあるため、`Possible` が true であった場合でも `Error` を処理してください。

埋め込みの Android ホストでは、`android:supportsPictureInPicture="true"` を宣言し、`orientation|screenSize|screenLayout|smallestScreenSize` の構成変更を処理する必要があります。アクティビティの `onPictureInPictureModeChanged` と `onUserLeaveHint` を `NsgoView` 上の同じメソッドに転送してください。Android 15 以降では、`PictureInPictureUiState.isTransitioningToPip()` も `NsgoView.onPictureInPictureTransition` に転送します。サポートしていないホストまたはデバイスは `Supported: false` を報告します。

Web では、標準の動画 PiP API を提供するブラウザにおいてユーザーの押下操作から開始します。`Supported` はページの Permissions Policy も反映し、`Possible` には動画メタデータの読み込みが必要です。ブラウザ自身の PiP コントロールも同じアクティブ状態を報告します。ソースの停止・置き換え・クリア、または動画の削除を行うとセッションが終了し、保留中の開始処理が以前のソースを再び開くことはできません。Web PiP は動画のみを表示し、カスタムの NSGo コンテンツ、バックグラウンドでの自動開始、および `OnPictureInPictureRestore` はサポートされません。非対応ブラウザは `Supported: false` を報告します。

iOS では、`mobile.PictureInPicture` は独自のレイアウトを持つ `Children` を受け取り、デフォルトサイズは 320 × 180 ポイントです。アプリ内ではスペースを占有しません。その `Ref` には同じ開始および停止メソッドがあり、`OnState`、`OnRestore`、`OnPlay`、`OnPause`、`OnSkip` がシステムアクションを配信します。`PausedFunc` と `PositionFunc` をそれらのコールバックが更新する再生状態にバインドしてください。`Duration: 0` はライブコンテンツを表します。サスペンドされたアプリは最後に描画されたフレームを保持します。

カスタムツリーに `core.Video` を配置すると、デコードされたフレームをアプリが描画した字幕やその他のオーバーレイと合成できます。外側の `Ref` を使ってセッションを開始してください。動画と一時停止シグナルを共有し、`Video.OnProgress` からカスタムセッションの `PositionFunc` へ値を渡し、`OnSkip` を動画の `Ref.Seek` に転送します。動画の音声は動画自身のプレーヤーを通じて再生されます。

カスタムツリーはテキスト、ビュー、画像、Canvas も描画します。WebView、Metal シェーダー、およびバックドロップエフェクトはキャプチャされたフレームでは利用できません。アニメーション画像とネイティブレイヤートランジションはモデル状態を表示します。動きには明示的なアニメーション値を使用してください。動画と無関係な用途は App Store の審査で却下される場合があります。

`Supported: false` は、利用できないプラットフォームと、準備ができていないサポート対象セッションや `Error` で失敗理由が示されるセッションとを区別します。動画は、デバイスが PiP をサポートしていると AVKit が報告する場合の iOS および tvOS、macOS、Android、ならびに標準 API を提供する Web ブラウザをサポートします。カスタムツリーは iOS のみをサポートします。iOS と tvOS では、PiP が有効な間（一時停止中を含む）、共有オーディオセッションは再生（playback）カテゴリに留まり、動画の `AudioSession` が `AudioSessionMixWithOthers` の場合は他のアプリの音声とミックスされます。

ヘッドレステストでは、`nsgotest.PictureInPictureSupport(supported, possible)` が iOS、macOS、または Android の `nsgotest.Environment` 下でシステムが提供する機能を設定し、ビューの `PiPState`、`PiPStarts`、`PiPStops` がセッションを記録し、`app.ReportPictureInPicture` がシステムが報告するはずの状態を報告し、`app.PictureInPictureControl` がウィンドウの `play`、`pause`、`skip`、`restore` を送信します。動画の PiP ウィンドウは動画自身のプレーヤーを再生および一時停止し、それを `Video.OnPlay` と `Video.OnPause`（一時停止時は `VideoPauseControls` を伴う）が報告します。テストでは `app.PlayVideo` と `app.PauseVideo` がそれらを報告します。

両方のパスを試すには `go tool nsgo dev -target ios examples/pip` を、動画 PiP を試すには `-target macos` または `-target android` を実行してください。ネイティブの Mac テストでは、ネイティブコントロールの有無にかかわらず開始、終了、およびコマンドの繰り返し、さらにアクティブなコントロール付きソースの置き換えが検証されています。コントロール付き動画のフルスクリーンや自動開始を含む iPhone での受け入れ確認は保留中であり、現在の実装にはネイティブのデリゲート/フレームテストとヘッドレスアプリのカバレッジがあります。

tvOS の動画では、`go tool nsgo dev -target tvos_simulator examples/pip` を実行してください。NSGo はプレーヤーをネイティブの Now Playing セッションに関連付けます。tvOS 27 シミュレータはネイティブコントロールの有無にかかわらず動画を読み込みますが、そこでは AVKit が PiP 非対応と報告します。実際の PiP の開始、停止、および復元については、Apple TV 実機での検証が引き続き必要です。

Web ターゲットのリグレッションテストでは、録画済みのローカルクリップと信頼されたアプリボタン押下を使用して、Chromium における開始、終了、ソースの置き換え、削除を通じたブラウザの実際の PiP セッションを検証しています。他のブラウザエンジンについてはまだ受け入れ確認が必要です。
