---
title: 権限
description: システムの権限の確認とリクエスト、設定画面のオープン、および macOS の「システム設定」へのユーザー案内を行います。
group: プラットフォーム機能
order: 49
---

## 確認とリクエスト

アプリが実行できる操作の一部は、ユーザーが許可するまでシステムによって制限されています。スマートフォンと Mac ではマイク、カメラ、連絡先、カレンダー、リマインダー、写真、Bluetooth、メディアライブラリ、位置情報、通知が対象となり、スマートフォンではモーションとバックグラウンドでの位置情報も対象です。さらに macOS では、アクセシビリティ、画面収録、入力監視、フルディスクアクセス、アプリ管理、デベロッパツール、および他アプリのオートメーションも含まれます（[位置情報とモーション](services.md#位置情報とモーション)、[通知](notifications.md)）。`core.Permissions()` はこれらを `core.Permission` キーの一覧として返します。`cx.PermissionStatus` は尋ねることなくアプリが権限を持っているかを返し、`cx.RequestPermission` はシステムプロンプトが存在し、かつまだ尋ねていない場合にシステムのプロンプトでリクエストします。ユーザーがその権限を必要とする操作を行ったタイミングでリクエストしてください。Face ID と Touch ID はこれらに含まれません。`cx.BiometricAvailability` で利用可否を確認し、`cx.AuthenticateBiometric` で専用のプロンプトを使って認証を求めます（[生体認証](services.md#生体認証)）。

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

macOS では、オートメーションは対象アプリごとに許可されます。実行中の対象アプリをバンドル ID で確認またはリクエストするには `core.PermissionAutomationOf("com.apple.finder")` を渡し、「システム設定」のオートメーションペインを開くには `core.PermissionAutomation` を渡します。対象アプリが実行中でない場合は `PermissionUnknown` を返します。

| ステータス | `String()` | 意味 |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | 未リクエスト：リクエストするとシステムのプロンプトが表示されます |
| `PermissionGranted` | `granted` | アプリに権限が付与されています |
| `PermissionLimited` | `limited` | 選択された写真や連絡先など、一部へのアクセスのみ許可されています |
| `PermissionDenied` | `denied` | 拒否されています。システム設定からのみ変更できます |
| `PermissionUnknown` | `unknown` | 制限対象ですが、システムまたはプラットフォームが状態を通知しません |
| `PermissionNotGated` | `not-gated` | Linux のように、アプリに対して制限がかかっていません |
| `PermissionUnavailable` | `unavailable` | プラットフォームに該当する仕組みがありません |

`Allowed` は、granted、limited、および not gated の場合に true を返します。macOS では、アクセシビリティ、画面収録、入力監視はリクエストしても対象ペインにアプリが追加されるだけであり、ユーザーがオンにするまでは denied を返します。画面収録の許可は、アプリが再起動された時点で反映されます。

## 利用目的の説明

macOS と iOS では、権限をリクエストする際にアプリの Info.plist 内に理由（利用目的の説明）が必要です。これは `nsgo.yaml` の `apple.info` に記述します。これがない場合、システムによってアプリが終了されてしまうため、リクエストは unavailable を返し、そのキーをログに出力します。アプリの[カタログ](localization.md)に同じキーのメッセージ（たとえば `locales/en.yaml` 内の `NSCameraUsageDescription: Scans the codes you point it at.`）を用意すると利用目的が翻訳され、システムはユーザーの言語で尋ねます。翻訳がない言語ではソース言語の文言が使われます。

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # macOS Automation
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+; BLUETOOTH on Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+; READ_EXTERNAL_STORAGE on Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

Android では、各権限を `nsgo.yaml` の `android.permissions` に記載する必要があります。マニフェストから漏れている権限は `PermissionUnavailable` を返します。`nsgo dev` での実行時、macOS はアプリの動作を起動元のターミナルに紐づけるため、許可を求められ一覧に表示されるのもそのターミナルになります。アプリ自身の権限をテストするには、アプリをビルドしてください。

## 設定画面を開く

`cx.OpenPermissionSettings` は、権限を付与する設定ページを開き、開けたかどうかを返します。開く先は、macOS では「プライバシーとセキュリティ」内の該当ペイン（オートメーション、連絡先、写真、位置情報サービスを含む）または「通知」、iOS では「設定」内のアプリページ（iOS 16 以降では通知ページ）、Android では「設定」内のアプリページ、Windows ではマイク、カメラ、または通知のページです。

`PermissionLocation` と `PermissionBackgroundLocation` では、macOS は「位置情報サービス」を開きます。`PermissionMotion` は、macOS がモーションサンプルを提供していないにもかかわらず「モーションとフィットネス」を開きます。iOS と Android では、これら 3 つすべてでアプリのページが開きます。設定を開くこと自体は、権限のリクエストも付与も行いません。

## ユーザーの案内

`desktop.GuidePermission` はその設定ページを開き、ユーザーが権限を付与するまで待機します。macOS では、フォーカスを奪わずに「システム設定」のウィンドウに追従するパネルが表示され、一覧へアプリをドラッグして追加するタイプのペインでは、ユーザーがドラッグできるようにアプリを差し出します。アプリに権限が付与されるか、ユーザーがパネルまたは「システム設定」を閉じるか、`stop` が呼ばれるか、あるいはコンポーネントが破棄されると終了し、コールバック関数がステータスを受け取ります。新しいガイドを開始すると、前のガイドは終了します。

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

提示されるアプリはバンドル、存在しない場合は実行ファイルです。`GuideOptions.App` で別のアプリを指定でき、`Name` でパネル上の表示名を指定できます。Windows では設定ページを開き、「設定」が閉じられるまで待機します。その他の環境では、関数が即座にステータスを受け取ります。

## プラットフォーム別の対応

| プラットフォーム | 取得・リクエストできる権限 |
| --- | --- |
| macOS | すべての権限（オートメーションは `PermissionAutomationOf` 経由で実行中の対象アプリごと、通知はバンドルアプリのみ）。対象未指定のオートメーション、アプリ管理、デベロッパツール、メディアと Apple Music は unknown、モーションは unavailable |
| iOS | マイク、カメラ、連絡先、カレンダー、リマインダー、写真（`PermissionLimited` 対応）、Bluetooth、メディアライブラリ、位置情報、モーション、通知 |
| Windows | マイク、カメラ、および通知（プロンプトは一切表示されません）。その他は制限なし（not gated） |
| Linux | サンドボックス外では制限なし。通知は通知サーバーが応答するかどうかを反映します。Flatpak と Snap では、カメラはデスクトップポータル経由、マイクはポータルと PipeWire 経由 |
| Web | マイクとカメラ（Firefox では unknown）、通知、位置情報（バックグラウンド位置情報は対象外）、モバイルブラウザーでのモーション |
| Android | `android.permissions` に宣言されたマイク、カメラ、カレンダー、Bluetooth、メディアライブラリ |
| Apple Watch、ミニプログラム | それぞれのサービスを通じた通知、位置情報、モーション。システムによって制限されるその他の項目は現時点では unknown |

位置情報とモーションの権限は各[プラットフォームのセンサー](services.md#位置情報とモーション)に準拠し、`PermissionNotifications` は[通知](notifications.md)に準拠します。フィットネスでは `PermissionActivityRecognition`（Android では `ACTIVITY_RECOGNITION`、Apple では `NSMotionUsageDescription`）を使用し、Apple の気圧データも含まれます。Android の生モーションデータは引き続き別扱いになります。モバイル Safari では、押下ハンドラーの中から `PermissionMotion` をリクエストしてください。

## テスト

`nsgotest` は独自のモデルに基づいて応答します。`app.Permissions` が各ステータスを保持し（初期状態は未リクエスト）、未リクエストの権限に対してリクエストを行うと、`app.PermissionAnswers` に指定された値（未指定の場合は granted）になります。`PermissionNotifications` は `app.NotificationPermission` および `app.NotificationAnswer` と同期されます。`app.PermissionSettings` は開かれた設定画面の一覧を、`app.Guide` は表示中のガイドを保持し、ガイドは `app.GrantPermission` と `app.CloseGuide` で終了します。

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## 次のステップ

- [通知](notifications.md): 通知独自の許可、バッジ、押下の検知。
- [ビルドと配信](building.md): パッケージが宣言するエンタイトルメントと機能。
