---
title: プラットフォームサービス
description: アラート、クリップボード、ファイル、写真、共有、ストレージ、シークレット、音声認識、読み上げ、ネットワーク、サインイン、アプリ内課金、アプリのライフサイクル。
group: プラットフォーム機能
order: 44
---

## アラートとクリップボード

`cx.ShowAlert` はプラットフォームのアラートまたはアクションシートを表示します。`Actions` の結果はインデックスで返され、ロールを指定するとキャンセルボタンや破壊的操作のボタンがプラットフォーム流に描画されます。macOS では、ウィンドウが表示されている間、アラートとファイルピッカーはシートを使います。ウィンドウが非表示、最小化、画面外にある場合や、ウィンドウのないトレイアプリの場合は、代わりにダイアログを独立して開きます。要求元のウィンドウを閉じると、そのダイアログはキャンセルされます。

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` はテキストを、`SetClipboardImage` は画像を書き込みます。`ReadClipboard`、`ReadClipboardImage`、`ReadClipboardFiles` は非同期に応答し、`HasClipboardText` と `HasClipboardImage` はペースト通知を出さずにクリップボードの中身を調べます。`core.FileFromURL` は、`ReadClipboardFiles` と `View.OnFileDrop` が渡す URL（`file:` URL、Web では `blob:` URL）を `core.File` として開きます。

tvOS には公開ペーストボードがありません。クリップボードへの書き込みは無視され、読み取りは内容を返さず、有無の確認は false を返します。

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

macOS と Windows では、`ReadClipboardItems` が利用可能なすべてのバイト表現のスナップショットを最大 48 MiB まで返します。各アイテムは、ネイティブの `Type` と生の `Data` を持つ順序付きの `ClipboardRepresentation` 値を保持し、データが空で存在だけを示すフォーマットも含みます。`ClipboardFormats` はペイロードをコピーせずにネイティブのフォーマット名を一覧します。スナップショットを復元するには、書き込みエラーを受け取る任意のコールバックとともに `WriteClipboardItems` に渡します。空のスライスを渡すとクリップボードがクリアされます。`ClipboardChangeCount` の値を等価比較すると、コピーされたテキストが同一の場合も含めて、新しいコピーを検出できます。

macOS はアイテムの境界とペーストボードの型名を保持します。Windows は 1 つのアイテムを使い、`CF_HDROP` のようなバイトで表される Win32 名や `Rich Text Format` のような登録済みフォーマットを保持します。書き込み時にはフォーマットが異なるアイテムを結合し、空のペイロードには 0 バイトを 1 つ使います。GDI ハンドル、オーナー描画フォーマット、プライベートな数値フォーマット ID は省かれます。その他のプラットフォームでは、読み取りと書き込みのコールバックは `CodeUnavailable` を受け取り、カウントのコールバックは `-1` を受け取ります。

## リンクとファイルを開く

`cx.OpenURL` は URL をシステムのブラウザやアプリで開きます。`core.CheckURL` は拒否する URL を示します。スキームがないもの、16 KiB を超えるもの、空白を含むもの、ホストのない Web アドレス、そして `javascript:`、`data:`、`blob:`、`file:`、`about:`、`asset:` です。

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` はアプリが持つファイルをその種類に関連付けられたアプリで開き、`cx.Share` はファイル、テキスト、またはその両方に対してシステムの共有シートを表示します。対象は macOS、iOS、Android、Windows、Linux（デスクトップポータルまたはファイルマネージャー経由）、Web（`navigator.share`）です。`core.SharedFile` はディスク上のパス（Web での `Share` や、ファイルをメモリに保持している場合はその `Data` バイト）と、相手のアプリから見える名前と種類です。デスクトップでは、`desktop.RevealFile` はファイルやフォルダーを Finder、エクスプローラー、ファイルマネージャーで選択した状態で表示し、`desktop.TrashFile` はそれをゴミ箱に移動します。完全に削除することはなく、ボリュームにゴミ箱がない場合や、ごみ箱より大きいファイルのようにゴミ箱が保持できない場合は、`core.ErrNoTrash` を返してアイテムをその場に残します。

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` は、[`android.share_types`](building.md#incoming-shares-on-android) または [`apple.share_types`](building.md#incoming-shares-on-ios) に対するテキストとファイルを受け取ります。Android の共有はアプリを起動できます。iOS はアプリがアクティブになるか、ユーザーが次にアプリを開くまで、共有を App Group にキューイングします。保留中の共有は最初のハンドラーを待ちます。iOS はコールバックが戻るまで受信箱のバッチを保持し、プロセス終了をまたいでも少なくとも 1 回は配信されます。テキストは `Text` に届き、ファイルは `Path`、`Name`、`MIME` を持つ読み取り可能なキャッシュのコピーとして届きます。キャッシュより長く残す必要があるファイルは、永続ストレージに移動またはコピーしてください。

`desktop.LookUp` は、macOS で 3 本指タップをしたときと同じように、テキストに対するシステムの辞書パネルをビュー内のボックスの上に表示します。渡すのは選択範囲のテキストとボックス、そしてその `Selectable` ビューの `Ref` です。`desktop.CanLookUp` は表示できるかどうかを示し、表示できるのは macOS のウィンドウ内です。Windows、Linux、Web にはそのようなパネルはありません。テストでは、`nsgotest.Environment` が `CanLookUp` を設定している場合、表示されたパネルを `nsgotest.App.LookUps` が一覧します。

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` はインデックスを起点にファイルのプレビューを要求し、パネルが開いている間に再度呼び出すとファイルを更新します。`desktop.CanQuickLook` は、macOS と Windows ではアプリ自身のウィンドウ内で、Linux ではセッションが GNOME Sushi の実行中またはアクティベート可能な D-Bus サービスを公開している場合に、利用可能であることを報告します。Windows はファイルの種類に登録されたプレビューハンドラーを使うため、対応フォーマットはインストールされているハンドラーやプレビューアーに依存します。true の結果はプラットフォームに要求したことを意味し、ファイルが描画されたことを意味するわけではありません。

Space または Escape でパネルが閉じます。矢印キーは、Windows と、X11 上で選択と所有者を報告する Sushi のバージョンで、要求内のファイルを選択します。Wayland では、Linux は現在親を持たないプレビューを開くため、所有者による自動クローズや矢印キーでの選択は利用できません。KDE 固有のプレビューアーと Web には対応していません。

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## ネイティブメニューのアイコン

`desktop.MenuItem.Icon` は、macOS、Windows、Linux のアプリケーションメニュー、Dock メニュー、トレイメニュー、ポップアップメニューで、項目の横にアイコンを表示します。`desktop.MenuIcon` には、パース済みのベクターアイコン（`SVG`、例えば `lucide.Folder()`）、エンコード済みの画像バイト（`Data`、PNG、JPEG、GIF、WebP、SVG）、またはローカル／アセット／ピクセルの `core.ImageSource`（`Source`）を渡します。`Template` を設定すると、モノクロ画像がメニューのテキスト色で着色され、ライト／ダークの外観（macOS ではハイライト状態も）に追従します。モノクロの `SVG` アイコンはデフォルトでテンプレートとして扱われます。

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

アイコンはローカルのソースから同期的に読み込まれます。リモート URL や不正な画像の場合、項目はアイコンなしになります。メニューのラベル、ショートカット、チェック／ラジオのインジケーターはネイティブの動作を保ちます。

テストでは、`nsgotest.MenuItem.Icon` が `app.Menu`、`app.DockMenu`、`app.PopupMenu` の各項目のアイコンを記録します。

## 写真とファイル

`cx.PickImages` はシステムの写真ピッカーを表示し、`core.PickedFile` で応答します。アプリはそのバイト、名前、種類、ピクセルサイズを読み取れます。`cx.TakePhoto` はプラットフォームにカメラがある場合にカメラを表示し、`cx.Env().CanTakePhoto` はカメラがあるかどうかを示します。macOS、Windows、Linux では、「写真を撮る」ボタン付きのスキャナーのカメラプレビューを表示します。`cx.ScanCode` はカメラプレビューを開き、最初に読み取ったコードで応答します。`ScanOptions.Kinds` で他の種類を指定しない限り QR コードを読み取り、ユーザーが閉じた場合は nil を返します。`cx.Env().CanScanCode` はプラットフォームにスキャナーがあるかどうかを示します。対象は、カメラ付きの macOS、iOS、Android デバイス、カメラ付きの Windows マシン、Camera ポータルと GStreamer の `pipewiresrc` がある Linux デスクトップ、`getUserMedia` と `BarcodeDetector` を持つブラウザーです。要求元のウィンドウが表示されている場合、macOS はシートを、Windows と Linux はウィンドウ上のモーダルを、Web はページ上のオーバーレイを表示します。macOS と iOS では `apple.info` に `NSCameraUsageDescription` を、Android では `android.permissions` に `CAMERA` を記述してください。Android、Windows、Linux は PDF417 以外のすべての種類を読み取ります。コードのテキストは信頼できない入力として扱ってください。

tvOS では、`PickImages` は画像なしで応答し、`TakePhoto` は `core.ErrNoCamera` を返します。NSGo は連係カメラのピッカーを提供しません。

テストでは、`nsgotest.App.PickImages`、`nsgotest.App.TakePhoto`、`nsgotest.App.ScanCode` が、選択と撮影については `nsgotest.ImagePick` レコードを、スキャンについては `nsgotest.CodeScan` レコードを受け取り、システム UI の代わりに応答します。これらのフックはヘッドレスでも、サポートされている `nsgo test -target` のホストでも動作します。`nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` を使うと、カメラのないターゲットにもカメラを持たせられます。

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` は任意の種類のファイルを選び、`cx.PickDirectories` はフォルダーを `fs.FS` として選び、`cx.SaveFile` はシステムの保存ダイアログを通じてファイルやバイトを保存します。どちらのピッカーも `Title`、`Button` のラベル、`Start` フォルダーを受け取り、`PickFilesOptions.TypesName` は種類メニューの名前を指定するので、アプリは自分の言語でピッカーを表示できます。これらはヒントであり、macOS、Windows、Linux は従いますが、iOS は `Start` のみに従い、Android と Web はまったく従いません。`cx.PickScreenColor` は、`cx.Env().CanPickScreenColor()` が true の環境で画面上の任意の位置の色を取得します。対象は macOS、Windows、デスクトップの Screenshot ポータルがある Linux、`EyeDropper` を持つブラウザーです。

`core.SaveOptions.Start` に既存の絶対パスのフォルダーを設定すると、macOS、Windows、Linux、iOS ではそこで保存ダイアログが開きます。ヒントが空または使用できない場合や、その他のターゲットでは、システムがフォルダーを選びます。

`core.SaveOptions.Accept` を設定すると、書き込みの前に選ばれた保存先を検証します。これはエンジンのゴルーチンで実行されます。エラーを返すと保存先には手を付けません。macOS、Windows、Linux のプロセス内 GTK チューザーでは、ダイアログが理由を表示し、ユーザーは別のパスを選べます。拒否の後にキャンセルすると、`saved == false`、拒否されたパス、エラーが報告されます。Android と Linux のデスクトップポータルでは、拒否するとその結果で保存が完了します。Android ではユーザーがフォルダーを選び、要求したファイル名を `name` クエリパラメーターに持つツリー URI が渡されます。承認後にドキュメントを作成し、成功時には新しいドキュメントの URI を報告します。チェックが 5 秒以内に応答しない場合、プラットフォームは書き込みを中止します。iOS と HarmonyOS は、チェックが必要な保存に対してエラーを報告します。Web には保存先のパスがなく、通常の保存フローを使います。

ヘッドレステストでは、`App.PickFiles`、`App.PickDirectories`、`App.SaveFile` が `nsgotest.FilePick`、`nsgotest.DirectoryPick`、`nsgotest.FileSave` レコードを受け取ります。同じレコードが `app.FilePicks`、`app.DirectoryPicks`、`app.Saves` にも入ります。

### 同梱アセット

ファイルをアプリの `resources/` ディレクトリに置き、Go から `cx.ReadAsset("asset:maps/level.bin", done)` で読み取ります。コールバックは読み取り後にエンジンのゴルーチンで `([]byte, error)` を受け取り、所有者がなくなるか再実行された場合は破棄されます。ファイルが見つからないことは `errors.Is(err, core.ErrAssetNotFound)` で判別します。読み取りはファイル全体をメモリに読み込みます。

## PCM オーディオのプル再生

`audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` を使うと、アプリがデコードまたは合成したインターリーブ形式の Float32 PCM を再生できます。`Start` でサンプルのプルを開始し、`Pause` でプルを停止します。所有するスコープが終了すると出力も閉じられます。macOS、iOS、Android、Windows、Linux、Web がこのストリームに対応しています。その他のターゲットは `audio.ErrUnsupported` を返します。

`Render` は埋めたフレーム数を返します。オーディオスレッドで実行されるため、ブロッキング、メモリ割り当て、グラフへのアクセスを避ける必要があります。埋めた量が足りない場合、残りは無音で再生され、`Underruns` が増えます。`Position` は単調増加するフレーム数と、そのフレームでのホスト時刻を返します。保留中のフレームを破棄するには `Flush` を、バッファーのレイテンシーを読み取るには `Latency` を使います。経路の変更や割り込みが起きると出力が一時停止し、エンジン上で `OnInterrupt` が呼ばれます。アプリは `Start` で再開できます。

## ストレージ

`cx.Storage()` はアプリのキーバリューストレージで、デバイス上に保持され、アプリのすべてのグラフで共有されます。読み取りは同期的で、計算の内側で読み取るとその計算が変更を購読します。`SetItem` はクォータ（`core.StorageQuota`、デフォルトは 5 MiB）を超えると `core.StorageFullError` を返します。これはユーザーのどのプログラムからでも読めるただのファイルなので、トークンやパスワードは代わりに [シークレット](#secrets) に保存してください。

`cx.Store()` は基盤となるストアを返します。これは、プラグインを独自のゴルーチンで実行するホストのように、`core.RootOptions.Storage` で別のルートを作成するホスト向けです。そうすると両方のルートがアプリのエントリとデバイス上に保持される内容を共有します。これはストア全体であり、アプリが書いていないコードに `cx.ProvideStorage` が与えるパーティションではありません。

独自の永続化を提供するホストは、`core.StorageBackend` を渡して `core.NewStore(backend)` を呼び出します。`Load() (map[string]string, error)` は一度だけ読み込み、`Write(core.StorageChange)` は変更を順番に受け取ります。`change.Op`（`core.StorageSet`、`core.StorageRemove`、`core.StorageClear`）で分岐し、`change.Key` と `change.Value` を使います。メモリ内だけに保持するストレージには `nil` を渡します。

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` はキーを一覧し、`RemoveItem` は 1 つを削除し、`Clear` はストレージを空にします。`cx.DataDir()` と `cx.CacheDir()` は、キーバリューストレージでは小さすぎるファイル用のアプリのフォルダーを返します。`cx.LogsDir()` はログファイル用のプラットフォームのディレクトリを返し、`Scope.ProvideStorage` はサブツリーに独自のパーティションを与えます。

`app_clip` と明示的な `apple.app_group` が設定されている場合、iOS の App Clip とそのフルアプリはこれらのデータとストレージの場所を共有します。キャッシュは別々のままです。グループを有効にしても既存のサンドボックスデータは移行されません。設定と署名については [App Clip のストレージ](building.md#app-clips) を参照してください。

HarmonyOS では、`os.ReadFile`、`os.WriteFile`、`os.Rename` などの Go のファイル操作は、これらのディレクトリと `os.TempDir()` の下で動作します。相対パスは files ディレクトリを起点とします。アドバイザリファイルロックとシンボリックリンクは利用できません。

### クラッシュレポート

ネイティブのリリースビルドは、Go のクラッシュトレースを `cx.LogsDir()/crashes` の下に保存します。Apple はキャッチされなかった Objective-C 例外を、Android はキャッチされなかった Java 例外を、Windows はトップレベルフィルターに到達した SEH 例外を記録します。次回起動時に `cx.CrashReports()` で保留中のレポートを一覧します。各レポートは、それを生成したバージョン、ビルド、プラットフォームを示します。`Open()` を呼び出して診断テキストを読み、リーダーを閉じてから、アプリが処理を終えたら `Acknowledge()` を呼び出してレポートを削除します。これらのファイル操作はゴルーチン上で実行してください。

## ディスク容量

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` は `cx.DataDir()` を含むボリュームを非同期に問い合わせ、エンジンのゴルーチンでコールバックします。`Total` はシステムが報告する容量、`Available` はユーザーが要求した操作にアプリが使える容量、`Opportunistic` はキャッシュやプリフェッチに使える容量です。3 つともバイト単位です。システムがその値を報告する場合、`Available` には必要に応じてシステムが回収する領域も含まれます。空き容量の値が 1 つしかないプラットフォームでは、その値を両方のフィールドに返します。Web では、`Total` はブラウザーが推定したそのオリジンのストレージクォータで、`Available` はその未使用分です。サービスが利用できない場合は `core.ErrDiskSpaceUnavailable` を返します。

Apple のビルドでは、`nsgo.yaml` の `apple.privacy.accessed_apis` で `NSPrivacyAccessedAPICategoryDiskSpace` を宣言する必要があります。書き込み前に空き容量を確認したり、容量が少ないときにユーザーが観察できる変更としてファイルを解放したりするには `E174.1` を宣言します。ユーザーに空き容量を表示するには `85F4.1` を宣言します。アプリは、取得した値の使用と共有に関する Apple の制限に従う必要があります。このカテゴリーのないビルドは `core.ErrNotDeclared` を返します。[Apple が承認している理由](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

テストでの応答は `nsgotest.DiskSpace(space, err)` で設定します。

## クラウドストレージ

`cloud` パッケージは、小さなキーバリューのエントリ（`cloud.Get`、`cloud.Set`、`cloud.Delete`、`cloud.Keys`）と、より大きなファイルやセーブのスナップショット（`cloud.ReadFile`、`cloud.WriteFile`、`cloud.DeleteFile`、`cloud.ListFiles`）をデバイス間で同期します。macOS と iOS では iCloud Key-Value Store と ubiquity ドキュメントコンテナ、Android では Google Play Games の Saved Games を使います。

すべての操作はエンジンのゴルーチンでコールバックを呼び出し、`cx` が破棄されている場合はスキップします。`cx.Env().CanUseCloudStorage()` はクラウドストレージが利用可能かどうかを報告します。ユーザーがサインアウトしている場合、エンタイトルメントがない場合、またはプラットフォームにネイティブのクラウドバックエンドがない場合（Windows、Linux、Web）、すべての呼び出しは `cloud.ErrUnavailable` を返し、書き込みを黙って捨てることはありません。

```go
if cx.Env().CanUseCloudStorage() {
	cloud.WriteFile(cx, "saves/slot1.sav", saveBytes, func(err error) {
		if err != nil {
			status.Set(err.Error())
		}
	})
}

cloud.WatchChanges(cx, func(ev cloud.ChangeEvent) {
	reload(ev.Keys, ev.Files)
})

cloud.OnConflict(cx, func(c cloud.Conflict) {
	_ = c.Resolve(mergeSaves(c.Local, c.Remote))
})
```

`cloud.WatchChanges` と `cloud.OnConflict` は、他のデバイスからの外部変更とバージョンの競合をエンジンのゴルーチンで届け、`cx` が破棄されると自動的に後始末します。競合しているファイルを読み取ると `*cloud.ConflictError`（`errors.Is(err, cloud.ErrConflict)` に一致）が返ります。

`nsgo build` で Apple の iCloud エンタイトルメントを有効にするには、`nsgo.yaml` に `cloud:`（または `icloud:`）セクションを追加します。

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

署名プロファイルは、要求する iCloud の機能を許可している必要があります。macOS ではプロビジョニングプロファイルがある場合にのみ iCloud が許可されるため、iCloud を含むのは Mac App Store ビルド（`sign.macos_profile`）だけです。Developer ID ビルドでは警告とともに省かれます。Android では、クラウドストレージにはアプリ内の Play Games Services v2 SDK と、サインイン済みのプレイヤーが必要です。

Steam のようなデスクトップストアでは、Steam Auto-Cloud が追加のコードなしで `cx.DataDir()` を直接同期します。ローカルのセーブファイルはそこに書き続け、ディレクトリの同期は Steam に任せてください。

テストでは、`nsgotest` が各アプリに独立したメモリ内のクラウドストアを与えます。`nsgotest.CloudAvailable` または `app.SetCloudAvailable` で利用可否を切り替え、`app.SimulateCloudChange` で他のデバイスからの外部更新をシミュレートし、`app.SimulateCloudConflict` で競合の解決をテストします。

## シークレット

`cx.Secrets()` はアプリのセキュアストアで、リフレッシュトークン、API キー、パスワード用です。アカウントごとに文字列を `Get`、`Set`、`Delete` し、プラットフォームがそれを保護します。存在しないアカウントは `("", false, nil)` として読み取られます。Web のようにプラットフォームにセキュアストアがない場合、すべての呼び出しが `core.ErrSecretsUnavailable` を返します。NSGo が通常のストレージにフォールバックすることはありません。キーチェーンがユーザーに確認する場合のように、システムが呼び出しを待たせることがあるので、エンジンのゴルーチンの外で呼び出し、結果は `cx.Post` で戻してください。

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| プラットフォーム | シークレットの保存場所 |
| --- | --- |
| macOS | ログインキーチェーンの汎用パスワード。`kSecAttrService` がサービス、`kSecAttrAccount` がアカウント |
| iOS、tvOS、watchOS | キーチェーンの汎用パスワード。方式は同じ |
| Windows | 資格情報マネージャーの汎用資格情報で、ターゲットは `service/account`、最大 2560 バイト |
| Linux | `service` と `account` 属性を持つ Secret Service（GNOME Keyring、KWallet）のアイテム。セッションで何も動いていない場合は利用不可 |
| Android | Android Keystore の AES キーで暗号化し、アプリのプライベートファイル内、バックアップ対象外に保存。読み取りでは、`expo-secure-store` が `SecureStore` の `SharedPreferences` に暗号化して保存した値にもフォールバック |
| WeChat ミニプログラム | プラットフォームによる暗号化ストレージ（`encrypt: true` を付けた `wx.setStorage` / `wx.getStorage`、基礎ライブラリ 2.21.3 以上）。それより古い基礎ライブラリでは利用不可 |
| Web、その他のミニプログラム | 利用不可（`core.ErrSecretsUnavailable`）。ブラウザーには OS が保護するクライアント側のシークレットストアがなく（Web のセッションはサーバーが設定する `HttpOnly` Cookie に保持してください）、QQ、Alipay、Douyin のミニプログラムには暗号化ストレージがありません |

サービスはデフォルトでアプリの ID（`core.App().ID`）です。他のフレームワークから移行するアプリは、そのフレームワークが書いたアイテムを `Service`、Apple プラットフォームでは `AccessGroup` と `Accessible` で指定することで、ユーザーのサインイン状態を維持できます。Android では、`Service` に `expo-secure-store` の `keychainService`（デフォルトは `"key_v1"`）を設定すると、それが `SharedPreferences` に暗号化した値を読み取れます。書き込みまたは削除すると、移行元のエントリはクリアされます。iOS では、`core.SecretAfterFirstUnlock` によってバックグラウンド処理が端末のロック中でもアイテムを読み取れるようになり、`ThisDeviceOnly` のアクセシビリティを指定するとバックアップから除外されます。macOS では、`AccessGroup` を指定するとアイテムがデータ保護キーチェーンに移動します。これには、`keychain-access-groups` エンタイトルメントにそのグループを列挙したチームによって署名されたビルドが必要で、Mac App Store ビルドと Developer ID ビルドの両方について `nsgo.yaml` の `macos.entitlements` で設定します。

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` はアプリのシークレットをメモリ内の `App.Secrets` に保持し、`nsgotest.Secrets` でいくつかのシークレットを持った状態からテストを始められるので、テストがマシンのキーチェーンに触れることはありません。TSX パッケージは `secrets` ケイパビリティを通じて、独自のサービスの下でストアにアクセスします。

## ネットワーク

`cx.Env().Network()` は、システムに使用可能な経路があるか（`Online`）、そしてシステムがその経路を従量制または高コストとみなしているか（`Expensive`）を報告します。バインディングは他の環境値と同様にこれに追従します。`cx.OnNetworkChange(func(core.Network))` は、スコープが生きている間、変化のたびにエンジンのゴルーチンでハンドラーを呼び出します。システムの経路は、特定のサーバーが応答するかどうかについては何も示しません。接続状態を報告できないターゲットではデフォルトでオンライン、コストが不明な場合はデフォルトで false になります。

オフライン時の動作をテストするには、`nsgotest.Environment` または `App.SetEnvironment` で `core.Environment{Network: &core.Network{Online: false}}` を設定します。

Go 自身の `net/http` は、システムの TLS とプロキシ設定を使って、すべてのターゲットでリクエストを行います。シグナルはエンジンのゴルーチンに属するため、別のゴルーチンからのレスポンスは `cx.Post` を通じてシグナルに届けます。

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` は、シグナルを設定できるグラフのゴルーチンで関数を実行します。Android ではマニフェストがインターネット権限を要求し、画像のダウンロードは自動的にディスクにキャッシュされます。

`core.NewResource` は、ソースから読み込む値に対して同じことを行います。フェッチャーは独自のゴルーチンで実行され、ソースが変わると再度読み込みます。

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`、`items.Err()`、`items.Loading()` はバインディングが追従するシグナルです。新しいクエリは `ctx` を通じて実行中のリクエストをキャンセルするので、遅れて届いた応答が新しい応答を置き換えることはありません。`items.Refetch()` は再試行し、読み込み中は直前のリストを保持します。`items.Mutate` は楽観的更新のために値をすぐに設定します。

`core.Suspense` は、コンテンツが読み取るリソースが最初の値を読み込んでいる間フォールバックを表示するので、画面ごとに読み込み中フラグを持つ必要はありません。

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

待機中もコンテンツはマウントされたまま非表示になっているので、値が届くとすぐに表示されます。再フェッチの間は、直前の値で表示されたままです。

### ローカルサービスの検出

macOS、iOS、Android、Windows、Linux では、`cx.AdvertiseService` が待ち受けポートと TXT 属性を公開します。`cx.BrowseServices` は、解決されたサービスをホスト、ポート、TXT 属性とともに報告します。`Host` は DNS ホスト名または IP アドレスです。サービスがいなくなると、コールバックは `Removed` が設定された状態でそのサービスを再び受け取ります。プラットフォームの対応状況は `cx.Env().CanDiscoverServices()` で確認します。どちらの呼び出しも停止関数を返し、スコープが破棄されると終了します。プラットフォームのエラーはエンジンのゴルーチンで非同期に報告されます。Linux は Avahi を使い、デーモンが利用できない場合はエラーを報告します。

Apple プラットフォームの Bonjour では、`nsgo.yaml` でサービスの種類を宣言し、ローカルネットワークへのアクセスの理由を説明します。権限の確認ダイアログはシステムが管理し、ユーザーの選択を記憶します。

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android は `NsdManager` を使います。Android 12 以前と、T Extensions 7 より前の Android 13 では、Wi-Fi での検出に `CHANGE_WIFI_MULTICAST_STATE` が必要です。スタンドアロンビルドはこの権限を宣言します。NSGo を Android ホストに埋め込む場合は、それらのデバイス向けにホストのマニフェストに追加してください。Android の生の TXT API（API 37 または T Extensions 22）がないデバイスでは、公開する TXT の値は有効な UTF-8 でなければなりません。ブラウズでは生の TXT 値が保持されます。Android ホストが API 37 以上をターゲットにする場合、Android 17 以降では `ACCESS_LOCAL_NETWORK` を宣言して要求する必要があります。

ヘッドレステストでブラウズ結果を与えるには `nsgotest.LocalServices` を使います。

## 再生中の情報

`cx.SetNowPlaying(core.NowPlaying{...})` を使うと、macOS のコントロールセンターとメニューバーにアプリの現在のメディアを表示できます。`Title`、`Artist`、`Album`、そして任意で `*core.ImageSource` としての `Artwork` を指定します。`Duration` が 0 の場合はライブストリームを意味します。現在の `Position` と再生の `Rate`（一時停止中は 0、通常速度では 1）を渡します。トラック、位置、レートが変わったら再度公開します。呼び出しの間はシステムが位置を進めます。

対応するトランスポートコントロールを `Commands` に列挙します。例えば `core.MediaPlay`、`core.MediaPause`、`core.MediaTogglePlayPause`、`core.MediaSeek` です。要求は `cx.OnMediaCommand` で処理します。コールバックはコマンドの `Kind` を受け取り、シークの場合は要求された `Position` も受け取ります。コマンドを受け取るのは最も新しい有効なハンドラーです。アプリの再生が終わったら `cx.ClearNowPlaying()` を呼び出します。アプリが手動で公開している間、macOS の制御付きビデオは独自の公開を停止します。プラットフォームの対応状況は `cx.Env().CanShowNowPlaying()` で確認します。

## バックグラウンドダウンロード

`cx.Download` と `cx.OnDownloadUpdate` は、スタンドアロンの iOS アプリと macOS アプリ、および Android で使います。埋め込まれた Apple のビューには転送のホストがありません。Android アプリは `nsgo.yaml` で `android.background_downloads: true` を有効にします。ダウンロードは、アプリが表示されている間にユーザーの操作から開始してください。保存先には、アプリが書き込める既存の親ディレクトリを持つ絶対パスを使います。

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

転送は GET を使い、任意のリクエストヘッダーを受け付けます。`CertificatePEM` でサーバーのリーフ証明書をピン留めしない限り、通常の HTTPS 信頼ストアを使います。ピン留めした場合も、ホスト名の一致と有効な証明書が必要です。サービスはその URL からの成功レスポンスを必要とするため、最終的な URL を使ってください。

Apple では、公開ホスト名での自己署名証明書には、そのホストに対する App Transport Security の例外を `apple.info` に記述する必要があります。それでも転送では、ピン留めした証明書との完全一致、ホスト名、有効性が確認されます。ローカルの NAS への接続には、アクセスの理由を説明する `NSLocalNetworkUsageDescription` も必要で、修飾されていないホストや `.local` のホストには `NSAllowsLocalNetworking` を使います。[Apple の ATS 設定](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Android のフォアグラウンドサービスは、アクティビティが画面から離れた後も続きます。Android の通知権限に従って、進行状況の通知を表示します。進行状況はバイト数で表され、合計が分かるまで `Total` は -1 になります。成功すると保存先がアトミックに置き換えられます。`task.Pause()` は途中までのファイルを保持し、`task.Resume()` はサーバーがバリデーターを提供する場合に HTTP の範囲指定を使い、`task.Cancel()` は途中までのファイルを削除します。

iOS は、アプリがサスペンドされている間も転送を続けるシステムのバックグラウンド URLSession を使います。アプリは起動時にセッションに再接続し、バックグラウンドの完了を確認応答する前にファイルを公開します。macOS の転送はプロセス内で実行されます。Apple プラットフォームでは、一時停止したダウンロードにネイティブの再開データを使い、最終結果が保存されるまでリクエストのメタデータをアプリのプライベートストレージに保持します。再開できないサーバーの場合は、転送を最初からやり直します。

アクティブなタスクと保存された結果を確認するには、アプリの起動時に `OnDownloadUpdate` を再度登録します。最終結果を処理したら `p.Task.Forget()` を使います。Android は、例えばフォアグラウンドでの転送時間の上限に達したときなどに、サービスを停止することがあります。その場合アプリは `DownloadInterrupted` を受け取ります。Windows、Linux、tvOS、watchOS、Web では、`Download` は `ErrDownloadUnavailable` を返します。

## 音声認識とライフサイクル

`cx.StartSpeech` は、`cx.Env().CanRecognizeSpeech()` が認識器の存在を示す環境（macOS、iOS、Android、Windows、および `SpeechRecognition` または `webkitSpeechRecognition` を公開するブラウザー）でシステムの認識器を使って音声を認識し、変化に応じて書き起こしを返します。Apple プラットフォームでは `apple.info` に `NSMicrophoneUsageDescription` と `NSSpeechRecognitionUsageDescription` を宣言し、Android では `android.permissions` に `RECORD_AUDIO` を追加します。ヘッドレステストでは、認識がアクティブな間 `app.Speech` が `nsgotest.SpeechSession` を保持します。結果を届けるには `app.HearSpeech` と `app.EndSpeech` を使います。

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` は、アプリがアクティブ、非アクティブ、バックグラウンドのいずれであるかを示します。デスクトップでは `cx` が属するウィンドウについての状態なので、別のウィンドウが表示されている間に最小化されたウィンドウの内容を一時停止できます。アプリ自身のスコープでは、すべてのウィンドウをまとめた状態です。`desktop.Minimized(cx)` はそのウィンドウが最小化されているかどうかを示し、`desktop.FrontmostApp(cx)` は、デスクトップペットやタイムトラッカーが追従するように、デスクトップで最前面にあるアプリケーション（アプリ自身を含む）と、そのウィンドウがフルスクリーンかどうかを示します。`desktop.CanReadFrontmostApp(cx)` は追従できるかどうかを示し、対象は macOS、Windows、X11、対応する Wayland コンポジターです。`cx.OnQuit` は、どのような終了の仕方であっても、終了時に 200 ms の作業時間で後始末をします。`desktop.OnQuitRequest` は、デスクトップの「終了」メニュー項目や `desktop.RequestQuit(cx)` をキャンセルまたは延期できます。Windows と Linux では、ウィンドウを閉じるとアプリが終了する場合、`OnClosing` の後にも実行されます。`desktop.RequestRestart(cx)` は再起動の前に同じハンドラーに確認します。`desktop.Restart(cx)` は引き続き即座に再起動します。Linux と Windows でのアップデーターによる再起動も確認を行うので、保存処理でそれをキャンセルまたは延期できます。システムがログアウト、再起動、シャットダウンを報告した場合、`q.Forced()` は true になり、アプリは `OnQuit` を経て終了に進みます。ユーザーに判断を求める前に `Forced()` を確認してください。`cx.SetAppearance` はライトまたはダークを強制し、`cx.SetLocale` は言語を選び、`cx.OnKey` はキーボードショートカットをバインドします。このバインディングは、所有者が表示しているものが非表示になっているかダイアログの下にある間は休止します（`core.KeyOptions{OverModals: true}` で維持する場合を除く）。また、アプリケーションメニューより先にキーを受け取ります（`YieldsToMenu` を指定すると、フォーカスのある場所で同じショートカットを持つ有効なメニュー項目にキーを譲ります。テキストフィールドや Web ビューにフォーカスがある間は、`MenuNotEditing` の項目はバインディングにキーを渡し、同じショートカットを持つ後続の項目にはキーを渡しません。また Linux のメニューは Control、Alt、ロゴキーなしで押されたキーを受け取らないため、そこではバインディングがそのようなキーを保持します）。バインディングは `cx` が属するウィンドウで動作しますが、`AllSurfaces` を指定するとアプリ全体のショートカットとして、後から開かれるものも含めてアプリのすべてのウィンドウとパネルでバインドされます。その場合、バインドされたウィンドウで所有者が表示しているものが非表示の間は、すべてのウィンドウで休止します。さらに、`core.KeyAlways` または `core.KeyNotEditing` でバインドしない限り、フォーカスのあるコントロールが使うキー（ボタンの Return と Space、スライダー、タブバー、ラジオグループの矢印キー）はそのコントロールに残します。`cx.OnOpenFiles` は、macOS、Windows、Linux、iOS で開かれたドキュメントを受け取ります。iOS は通常のファイルをアプリが所有する一時ストレージにインポートします。保持するには `cx.DataDir()` に移動してください。macOS では、アプリケーションメニューを提供していたウィンドウを閉じると、残っているウィンドウのうち最も新しいもののメニューが復元され、残っていない場合はデフォルトのメニューになります。

Wayland では、最前面アプリの追跡にはコンポジターの wlr foreign-toplevel プロトコルのバージョン 2 以降が必要です。その他のコンポジターでは `CanReadFrontmostApp` が false になります。ID は Wayland のアプリケーション ID で、名前は可能であればデスクトップエントリから取得し、PID は 0 です。パッケージ化された Windows アプリは、フレームホストのパスの代わりに AUMID を使います。macOS は、ウィンドウサイズから推測するのではなく、AppKit のフルスクリーン表示状態に従います。

アプリケーションメニューの編集ロールは、`OnSelect` のフォールバックより先に、フォーカスのあるエディターを対象にします。macOS では、「取り消す」と「やり直す」はそのエディターの履歴に従い、「カット」はその選択範囲に従います。アプリが非アクティブな間も同様です。エディターがない場合、フォールバックは有効なままです。

iOS では、`cx.OnKeyWith` に `core.KeyOptions.Title` 関数を渡すと、バインディングが発火可能な間、システムのキーボードメニューにショートカットが表示されます。`Group` は iPadOS 26 以降でメニューの名前を指定します。これらの関数はアプリのカタログを読めるので、ショートカットのタイトルを言語に追従させられます。現在のリストを調べるには、テストで `app.KeyCommands()` を使います。

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` はユーザーがデバイスに付けた名前、`cx.Env().DeviceModel()` は `Mac15,7`、`iPhone15,2`、`Google Pixel 8` のようなハードウェアモデルで、アプリはペアリングされた 2 台のデバイスを区別できます。macOS、Android、Windows、Linux は両方を返します。iOS はモデルを返し、名前は Apple のユーザー割り当てデバイス名エンタイトルメントを持つアプリにのみ返します。watchOS はハードウェア識別子を、HarmonyOS は市場名または製品モデルを、ミニプログラムはプラットフォームのブランドとモデルを返します。これらのターゲットでは、ユーザーが付けた名前は空になります。Web はどちらも返しません。テストでは `nsgotest.Environment` で設定します。

`cx.Env().ThermalState()` は、Apple プラットフォームと Android API 29 以降で `core.ThermalNominal`、`ThermalFair`、`ThermalSerious`、`ThermalCritical` のいずれかを報告します。`cx.Env().LowPowerMode()` は、Apple プラットフォーム、Android、Windows、power-profiles-daemon のある Linux セッションで `core.LowPowerOn` または `LowPowerOff` を報告します。システムが値を返さない場合、読み取りは `ThermalUnknown` と `LowPowerUnknown` を返します。それぞれの読み取りは独立して変化に追従するので、アプリは実行中に処理量を調整できます。テストでは、`nsgotest.Environment` または `app.SetEnvironment` を通じて `core.Environment` の `ThermalState` と `LowPowerMode` を設定します。

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` は、macOS、Windows、X11 で、アプリのウィンドウの外にあってもポインターの位置を読み取ります。1 回の呼び出しで 1 回サンプリングします。座標は `desktop.Monitors(cx)` や `Window.ShowAt` と同じスクリーン座標です。Wayland やその他の非対応ターゲット、またはスコープが `core.AccessCursorPoint` を持たない場合、`available` は false になります。

`desktop.SetWindowConstraints` はコンテンツのサイズを制限し、リサイズ、最小化、最大化を制御します。ウィンドウを開くときに `WindowOptions.Constraints` を設定するか、メインウィンドウには `nsgo.WindowConstraints` を使います。Linux は X11 で固定でない最大サイズを適用し、X11 と Wayland で GTK のネイティブタイトルバーから禁止された最小化／最大化ボタンを取り除きます。Wayland は現在、上限については固定サイズにのみ対応しています。

`desktop.SetWindowLevel` はウィンドウを他のウィンドウより前面に浮かせ、`desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` または `WindowOptions.Workspaces` は、デスクトップペット、HUD、タイマーのように、ウィンドウをすべてのワークスペースに表示し続けます。macOS ではすべての Space と他のアプリのフルスクリーン Space の上に表示され、Cmd-` の対象から外れます。Linux では X11 ウィンドウマネージャーのすべての仮想デスクトップに表示されます。Windows と Wayland では開いたワークスペースに留まり、`desktop.CanSetWindowWorkspaces` が効果があるかどうかを示します。true になるのは macOS と、`_NET_WM_DESKTOP` に対応する X11 ウィンドウマネージャーの下です。メインウィンドウは、そのコンテンツから `desktop.SetWindowWorkspaces` を呼び出すとすべてのワークスペースに表示されます。テストでは、`nsgotest.App.Window` が要求を一覧し、`nsgotest.Environment` が `CanSetWindowWorkspaces` を設定します。

`desktop.RegisterHotKey` はグローバルショートカットをバインドします。これはどのアプリが最前面にあっても動作し、対象は macOS、Linux、`nsgo.Run(App)` で実行する Windows アプリです。ハンドルの `State` はシステムがそれを受け付けたかどうかを示します。応答があるまでは `HotKeyPending`、その後は `HotKeyRegistered`、別の登録がその組み合わせを保持している場合は `HotKeyTaken`、グローバルショートカットのないキーやプラットフォームでは `HotKeyUnsupported` になるので、ショートカットレコーダーは別の組み合わせを求めることができます。macOS では、アプリがすでに保持しているか、別のアプリが排他的に登録している場合にのみ、組み合わせが使用中とみなされます。Wayland ではコンポジターがユーザーにショートカットのバインドを求めます。ユーザーがバインドすると登録済みになり、ユーザーまたはコンポジターがそれを外すと使用中になります。テストでは、`nsgotest.HotKeyTaken` と `nsgotest.HotKeyUnsupported` で組み合わせにそのように応答させます。

`desktop.TitleBarHidden` を使うと、Windows と Linux では Alt を単独で押して離すか F10 を押したときに、アプリケーションメニューをポップアップとして表示します。Alt とトップレベルのラベルの最初の文字または数字（ラベルの文字体系で）を押すと、そのサブメニューを開くかそのコマンドを選択します。アプリのバインディングはキーを保持します。Shift+F10 は引き続きコンテキストメニューのキーで、その他の Alt の組み合わせはシステムに残ります。Escape でポップアップが閉じ、キーボードフォーカスが戻ります。

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

macOS、Windows、Linux のデスクトップでのリサイズは、描画の前に新しいレイアウトを最大 16 ms 待ちます。レイアウトがその期限に間に合うよう、リアクティブな計算とイベントハンドラーは素早く終わるようにしてください。遅い I/O はゴルーチンで行い、結果は `cx.Post` で戻します。処理中のアプリもリサイズは続け、追いついた時点でレイアウトを表示します。

`desktop.SetWindowFrame` は、`WindowFrame.Move` が true のとき、メインスクリーンの左上を原点とするポイント単位の `X` と `Y` を使ってウィンドウを移動します。設定されていない寸法は現在のサイズを保ちます。`desktop.CanPlaceWindows` を確認してください。macOS、Windows、X11 上の Linux は位置の指定に対応しています。通常の Wayland のトップレベルウィンドウはコンポジターが決めた位置を保ちます。これらのデスクトップでは、`desktop.Frame` がネイティブウィンドウの位置に追従します。

macOS と Windows では、`WindowFrame{Animated: true}` を指定すると、表示中のパネルが新しいフレームへ滑らかに移行します。ドッキングされたパネルのサイズも含みます。非表示のパネルや視差効果を減らす設定の場合は、新しいフレームを即座に適用します。通常の macOS ウィンドウは AppKit のアニメーションを使います。

`desktop.WindowOptions.Parent` にアプリの開いているウィンドウを設定すると、親が閉じたときに両方が閉じます。デスクトップでは、子は親の上に表示され続け、親が非表示または最小化されると追従します。明示的に非表示にした子は、親が戻っても非表示のままです。macOS では、`Frame` が設定されていない場合は親の中央に配置され、親がフルスクリーンになると追従します。

`WindowOptions.NoShadow` と `desktop.SetWindowShadow(cx, false)` は、タイトルバーを非表示にして独自のクロームを描画する Windows のウィンドウからも影を取り除きます。システムのタイトルバーを持つウィンドウは枠と影を保ちます。ウィンドウマネージャーが装飾する Linux のウィンドウも同様で、その場合 `NoShadow` は GTK 自身の装飾の影だけを消します。

`desktop.SetWindowPointerEvents` は、ポインターをウィンドウを通り抜けさせて、その下にある任意のアプリのウィンドウに届けます。`WindowPointerNone` はヘッドアップディスプレイのようにすべての場所で、`WindowPointerPainted` は、自身へのプレス、ドラッグ、右クリックは受け取るデスクトップペットの周囲のように、どのビューも何も描画せずポインターも受け取らない場所で通り抜けさせます。システムが描画するタイトルバーは何も描画していない扱いになります。`WindowOptions.PointerEvents` と `PanelOptions.PointerEvents` はその設定でウィンドウを開きます。`Desktop.WindowDrag` の領域は、押下可能な祖先の内側でも引き続きドラッグできます。より近い押下可能なコントロールや編集コントロールは、ウィンドウをドラッグする代わりに自身の入力を保ちます。ポインターを受け取らないウィンドウはプレスを受け取らないので、アプリはショートカット、トレイアイコン、別のウィンドウから操作を戻せるようにします。`Access` を持つコードには `core.AccessOverlay` が必要です。

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

macOS と Windows では、painted ウィンドウ内の画像で `Image.HitTestAlpha`（TSX では `hitTestAlpha`）を 0 より大きく 1 以下の値に設定できます。例えば 0.5 では、少なくとも半分不透明な画像のピクセル上でポインターを受け取ります。このしきい値は画像の `OnPress` にも適用されます。ビューの背景や枠線は引き続き描画済みとして扱われます。0 の場合は画像ビュー全体を使い、Linux の画像も同様です。

macOS の painted ウィンドウでは、`Video.HitTestAlpha`（TSX では `hitTestAlpha`）がデコードされたビデオフレームに同じしきい値を適用します。これは `Controls` がオフで、デスクトップペット用のアルファ付き HEVC のような変形されていないビデオトラックで動作します。0 の場合、フレームが利用できない場合、およびその他のデスクトップホストでは、ビデオビューの境界を使います。

`desktop.NewTray` は、macOS と Windows で `TrayOptions.View` を使ってライブアイコンを描画できます。Windows は、ビューの透過レンダリングを通知領域のアイコンサイズに合わせ、ビューが変わるか画像の読み込みが完了すると更新します。アクションには `TrayOptions.OnClick`、`Panel`、`Menu` を使います。Linux は現在 PNG アイコンを表示します。トレイのビューを描画することは今後の目標です。

`desktop.OpenPanel` は、そのようなショートカットやトレイアイコンが表示するパネル、つまり他のウィンドウより前面にある枠なしのウィンドウを開きます。`desktop.OpenWindow`、`desktop.NewTray`、`mobile.OpenOverlay`、`audio.NewPlayer`、`audio.LoadSound`、`core.NewHolder` と同様に、実行中のハンドラーの所有者ではなく、渡した `cx` に直接属します。そのため、ボタンの `OnPress` からアプリのスコープを渡すと、ボタンのスコープがなくなった後もウィンドウ、パネル、プレーヤーは生き続けます。一方、リアクティブなプリミティブ（`Effect`、`Cleanup`、`AfterFunc`、`OnKey`）は実行中の所有者に属し、それは `cx.Run` で切り替わります。ポップアップカードは `Transparent` を指定したパネル上に自身を描画します。このパネルには独自の背景、枠、影がないため、表示されるのはカードの角丸と `BoxShadow` だけで、パネル内には影のための余白を取ります。`FitContent` は、最初に表示する前とコンテンツが変化したときに、`Width` と `Height` を上限としてパネルをコンテンツに合わせたサイズにします。`desktop.Transparent` は、透明なウィンドウの場合のように、パネルが現在独自の描画を何もしていないかどうかを示します。一方 `desktop.CanMakeWindowsTransparent` はウィンドウを透明にできるかどうかだけを示します。これが false の場合（コンポジターのない Linux など）は、コンテンツに背景を付けてください。

macOS では、`Transparent` のないパネルは、どちらかの寸法が 24 ポイント未満のときは角を四角にし、通常のサイズでは角丸に戻します。

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` は、macOS で画像または描画されたビューの横に `TrayOptions.Title` を表示します。カスタムのポップアップには、`OnClick` または `Menu` の中で `tray.ClickBounds()` を読み、利用可能な矩形を `panel.ShowAt(bounds, core.PlaceBottom)` に渡します。これは `Env.Displays` と同じスクリーンのポイントを使い、タイトルを含みます。境界は、最初のクリックの前、削除の後、Linux、および Windows シェルが提供できない場合には利用できません。クリックのたびに以前の境界が置き換えられるので、利用できない結果で古い矩形が再利用されることはありません。代わりに `TrayOptions.Panel` を設定すると、組み込みのクリックによる表示切り替えの動作が保たれます。

`PanelOptions.Dock` は、`Show`、`Focus`、`Toggle`、コンテンツのリサイズを通じて、パネルをディスプレイの使用可能な端に沿って中央に保ちます。`Edge` に `core.PlaceTop`、`PlaceBottom`、`PlaceLeft`、`PlaceRight` のいずれかを、`Display` に `Env.Displays` の ID（プライマリディスプレイの場合は空）を、`Margin` にその端からのポイント単位の距離を設定します。ドッキングされたパネルは `ShowAt` や `MoveTo` を使えず、`SetWindowFrame` はサイズだけを変更します。トレイアイコンは、アイコンの下ではなくドックの位置にパネルを表示します。

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` は、任意のウィンドウ配置とは別に、ドッキングへの対応を報告します。macOS、Windows、X11 は対応しています。Wayland では、任意のランタイムライブラリ `libgtk4-layer-shell.so.0` と、`zwlr_layer_shell_v1` を実装したコンポジターが必要です。NSGo はそのライブラリを GTK より先に読み込みます。すでに Wayland を読み込んでいる埋め込みホストは、ホストを開始する前に gtk4-layer-shell をプリロードする必要があります。ライブラリ、プロトコル、選択したディスプレイのいずれかがない場合、パネルは非表示のままになり、`OnPlacementError` が呼ばれます。Wayland のドックはワークスペースを予約せずにトップレイヤーを使います。`NoActivate` を指定するとキーボードは現在のウィンドウに残り、指定しないパネルは非表示になるまでキーボードフォーカスを排他的に取得します。通常の Wayland のウィンドウは、コンポジターによる通常の配置を保ちます。

ショートカットレコーダーを使うと、ユーザーはそのようなキーを選べます。`Focusable` なビューの `OnKeyDown` は、フォーカスがある間に押されたキーを、アプリのバインディングより先に、押されている修飾キー（修飾キー単独の押下も含む）とともに受け取ります。`true` を返すとキーがバインディングに届かなくなるので、Command+S を記録しても保存は実行されません。`KeyEvent.Shortcut` は、`OnKey` や `RegisterHotKey` が受け取る形式で押下を書き出し、各修飾キーを名前で表します（"ctrl+shift+k"、"alt+space"、Command+Shift+K なら "shift+meta+k"）。修飾キー単独の場合は "" になります。`OnKeyUp` は、`OnKeyDown` がキーを消費した後やフォーカスが移った後でも、それらのキーが離されたことを、まだ押されている修飾キーとともに受け取ります。どちらも macOS、Windows、Linux、Web（ブラウザーはページに渡さないショートカットを保持します）、Apple TV、`nsgotest` で動作します。Android はハードウェアキーボードのイベントをフォーカスのあるビューに非同期に届けるため、`OnKeyDown` の戻り値でキーを消費することはできません。iPhone と iPad はハードウェアキーのポーリングに対応しています。アプリがグローバルに登録したショートカットはビューより先にキーを受け取るので、レコーダーは記録中にそれを登録解除します。

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

ゲームやキャンバスは、代わりにキーをポーリングします。`core.KeysDown(cx)` はウィンドウで押されているキーを一覧し、`core.KeyHeld(cx, "left")` はあるキーが押されているかどうかを示します。これらは追跡される読み取りで、`RequestAnimationFrame` のループからも読めます。最初の読み取りで、プラットフォームはそれ以降すべてのキーを報告するようになるので、すでに押されているキーは、もう一度押されるまで不明のままです。フィールドや Web ビューに入力されたキー、およびインプットメソッドの変換中に入力されたキーは押されている扱いにならず、ウィンドウがフォーカスを失ったときにまだ押されているキーは、その時点で離された扱いになります。これらは `OnKeyUp` が動作する環境と、ハードウェアキーボードを接続した iPhone と iPad で動作します。

マウスボタンについては、`View.OnMouseDown` と `View.OnMouseUp` が、macOS、Windows、Linux、Web、`nsgotest` で、すべてのボタン（`core.MouseButtonLeft`、`core.MouseButtonRight`、`core.MouseButtonMiddle`、`core.MouseButtonBack`、`core.MouseButtonForward`）について `core.MouseButtonEvent` を届けます。ビューをキーボードのフォーカス停止位置にすることはありません。`core.MouseButtonsDown(cx)` と `core.MouseButtonHeld(cx, core.MouseButtonRight)` は、ウィンドウで押されているボタンの追跡される読み取りで、ウィンドウがフォーカスを失うと自動的にクリアされます。

物理的な位置に結び付いた操作には、`KeyEvent.Code`、`core.KeyCodesDown(cx)`、`core.KeyCodeHeld(cx, "KeyW")` を読み取ります。コードは `KeyboardEvent.code` の名前を使います。AZERTY では、QWERTY の W の位置は `Code: "KeyW"` かつ `Key: "z"` になります。`core.KeyCodeLabel(cx, "KeyW")` は、操作説明に使うための、アクティブなレイアウトでの表示ラベルを返します。iOS、Android、HarmonyOS、レイアウトマップのないブラウザーでは、その位置で最後に報告されたキー名を使い、キーが観測されるまでは空文字列を返します。HarmonyOS では ArkUI がスキャンコードを報告しないため、位置とキーの両方を InputKit の `KeyCode` から名付けます。そのため、そこではキー名はまだ QWERTY 以外のレイアウトに追従しません。

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## ディスプレイのスリープ防止

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` は、返された解放関数が実行されるか、現在のスコープが破棄されるまで、ディスプレイをオンに保ちます。Android では、アプリが表示されている間だけ保持が有効です。Web では Screen Wake Lock に対応したセキュアコンテキストで動作し、ページが表示状態になると再度ロックを要求します。Safari では、最初の要求を許可する前にプレスやキー入力が必要な場合があります。コンピューターのみの保持は、これら 2 つのプラットフォームでは効果がありません。

## ポインターロック

ゲームのカメラや、画面の端でも続ける必要があるドラッグでは、ref を持つビューへのプレスから `ref.LockPointer()` を呼び出します。ポインターはその場に留まって非表示になり、ビューは `OnPointerDelta(dx, dy)` で相対的な移動量を受け取ります。正の値は右と下への移動です。

ロックを解除するには、返された関数を呼び出します。Escape、ウィンドウのフォーカス喪失、ビューの非表示や削除でも解除されます。サーフェスが対応しているかどうかは `cx.Env().CanLockPointer()` で確認します。Web ではプレスのようなユーザー操作が必要です。Wayland 上の Linux では、ポインター制約と相対移動に対応したコンポジターが必要です。

## サウンドの制限

エンジンの制限を変更するには、サウンドを読み込む前に `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` を使います。`MaxEncodedBytes`、`MaxDecodedBytes`、`MaxPlaybacks` も設定できます。0 のフィールドはデフォルト値を使います。デフォルトは、サウンドごとにエンコード済み 8 MiB と 5 秒、ソース 64 個、全サウンド合計でデコード済み 16 MiB、出力ごとに重なり合う再生 32 個です。既存のサウンドは読み込まれたまま残り、新しい制限に算入されます。制限付きのスコープは、エンジンの予算内でデフォルトの制限を保ちます。署名された TSX パッケージの `nsgo/audio` のサウンドは、Go アプリが何を設定していてもデフォルトの制限に達しますが、アプリ自身の TSX は Go アプリの制限を使います。Android の SoundPool は、制限にかかわらず、同時に最大 32 個のサウンドを再生し、各サウンドの最大 1 MiB までをデコードします。

## フルスクリーン

`desktop.SetFullScreen(cx, true, displayID)` は、macOS、Windows、Linux で、`cx.Env().Displays()` で指定したディスプレイ全体に表示します。ID を省略するとウィンドウの現在のディスプレイを使います。不明な ID の場合、ウィンドウはそのままです。同じ要求を繰り返しても効果はありません。`desktop.SetFullScreen(cx, false)` はフルスクリーンを解除し、ウィンドウの以前のフレームを復元します。Web では、プレスのハンドラーから開始してください。ディスプレイはブラウザーが選び、`cx.Env().FullScreen()` は Esc を含む開始と終了に追従します。

## ディスプレイ

`cx.Env().Display()` はウィンドウの現在の `core.DisplayInfo` を返し、ウィンドウが別のディスプレイに移動するとバインディングを更新します。`cx.Env().Displays()` はプラットフォームが報告するディスプレイを一覧します。`RefreshRate` と `MaxRefreshRate` は Hz 単位で、利用できないレートは 0 です。iOS はパネルの最大値を報告します。HarmonyOS はウィンドウのディスプレイ、その現在のレート、対応する最大のレートを報告します。Web は現在のスクリーンを報告し、フレームコールバックによる測定値には `RefreshRateEstimated` を付けます。テストでは `nsgotest.Environment` を通じて `Displays` と `DisplayID` を設定します。

## テキストの読み上げ

`cx.Speak` は、`cx.Env().CanSpeak()` がプラットフォームの読み上げ対応を示す環境（macOS、iOS、tvOS、Android、Windows、Web、speech-dispatcher のある Linux）で、言語に応じたシステムの音声でテキストを読み上げます。一度に読み上げる発話は 1 つです。別の `Speak` は読み上げ中の発話を中断し、`cx.StopSpeaking` や読み上げを開始したスコープの終了も同様です。`done` は終わり方を一度だけ受け取ります。完了、停止、または `core.SpeakError` による失敗です。例えば、その言語を話す音声がインストールされていない場合は `core.SpeakLanguageUnavailable` になります。

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` と `Pitch` は 0.5 から 2 の範囲で、1 が音声本来の値です。`cx.Voices("ja", ...)` はその言語用にインストールされている音声を一覧するので、アプリは足りない音声のインストールをユーザーに求めたり、ユーザーに音声を選ばせたりできます。選んだ音声の `ID` は `core.Voice` に入れます。ブラウザーは、ユーザーがページ上で何かを押した後でのみ読み上げ、それより前は `core.SpeakNotAllowed` で失敗します。TSX パッケージの Go コンポーネントは、`text_to_speech` ケイパビリティがある場合にのみ読み上げます。

## オーディオのループ再生

`audio.Voice.Loop` を設定すると、`Stop` または `Close` までサウンドの再生を繰り返します。ストリーミングされる音楽では、`audio.PlayerOptions.LoopStart` を設定して `SetLoop(true)` を呼び出します。1 周目にはイントロが含まれます。2 周目以降はそのオフセットから始まります。オフセットが 0 の場合はトラック全体を繰り返します。

macOS 14 と iOS 17 以降では、長さが有限のトラックはネイティブのキューを使い、現在の周が終わる前に次の繰り返しを準備します。それより前の Apple のバージョンでは、現在の周が終わったときにループの開始位置へシークするため、短い途切れが生じます。Linux と Web では、ループするプレーヤーは最大 32 MiB かつ 5 分までのトラックをメモリにデコードし（1 分あたり約 23 MB）、途切れなく繰り返します。Linux では、それより大きいか長いトラックは独自のデコーダーでストリーミングされ、やはり途切れなく繰り返します。Web では終わりで先頭側にシークするため、短い途切れが生じます。Windows と Android では、開始位置を持つループは終わりでそこへシークするため、短い途切れが生じます。Web では、ブラウザーの途切れのない再生はレートに応じてピッチを変えてしまうため、レートが 1 以外のループするプレーヤーも終わりでシークします。Web では、ループするプレーヤーはサウンドと同様に、ユーザーがページ上で何かを押した後でのみ再生され、それより前は `NotAllowed` で失敗します。

## 生体認証

`cx.BiometricAvailability` は、Face ID、Touch ID、デバイスの指紋のいずれかが登録されているかを報告します。Android はどの種類が登録されているかを示さないため、指紋が登録されていれば指紋を報告し、そうでなければデバイスに顔認証のハードウェアがある場合は顔を報告します。`cx.AuthenticateBiometric` は、空であってはならない理由を添えてシステムにユーザーの確認を求め、成功、失敗、キャンセル、利用不可を報告します。アプリのためにパスコードのフォールバックを選ぶことはありません。

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

Face ID を使うアプリは `apple.info` に `NSFaceIDUsageDescription` を宣言し、Android アプリは `android.permissions` に `USE_BIOMETRIC` を記述します。

## サインイン

`cx.AuthSession` は、Web ビューではなくシステムの認証セッションで、OAuth プロバイダーを使ってユーザーをサインインさせます。macOS、iOS、watchOS では `ASWebAuthenticationSession`、Android では Custom Tab またはブラウザー、Windows、Linux、スキームのない Mac ではデフォルトのブラウザー、Web ではポップアップを使います。Web では、リダイレクトページが同一オリジンの `BroadcastChannel` 経由でもコールバックを送るので、プロバイダーの `Cross-Origin-Opener-Policy` がポップアップをページから切り離した場合でもサインインが完了します。そのようなポップアップはページからは閉じているように見えるため、プロバイダーのページを表示した後にポップアップを閉じても、セッションがキャンセルとして終了するのは 5 分後になります。セッションにはキャンセルボタンを用意してください。プロトコルはアプリが担います。アプリは PKCE の verifier と state を作り、渡されたリダイレクト用の認可 URL を組み立て、コードを交換する前にコールバックを検証します。キャンセルは失敗ではありません。`errors.Is(err, core.AuthCancelled)` で、ユーザーがセッションを閉じたことや、返された `cancel` がセッションを終了させたことを判別できます。

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

リダイレクトは、スキームが設定されている場合は `com.example.app:/oauth2redirect`（Windows、Linux、Android では、OS がコールバックを実行中のアプリに渡せるよう `nsgo.yaml` の `url_schemes` に記述する必要があります。macOS、iOS、watchOS のシステムセッションはその宣言なしでも受け取ります）、デスクトップのループバックでは `http://127.0.0.1:<port>/oauth2redirect` になるので、両方をプロバイダーに登録します。多くの場合、それぞれ別のクライアントとして登録します。ループバックだけを使う場合は `Scheme` を空にし、アプリの関連ドメインを使う場合は `Host` とともに `https` を設定します。これは iOS 17.4、macOS 14.4、watchOS 10.4 が受け取ります。watchOS ではスキームまたは関連する HTTPS ドメインが必要で、ループバックのフォールバックはありません。また、NSGo の既存の watchOS 26 という最小要件も引き続き適用されます。ブラウザーのタブが閉じられてもループバックのセッションには何も伝わらないので、`Timeout` かキャンセルボタンを用意してください。`Ephemeral` は、macOS、iOS、watchOS ではブラウザーの Cookie を共有しないよう求め、Android ではブラウザーが対応している場合にプライベートな Custom Tab を要求します。Android で Cookie が分離されると想定しないでください。tvOS とミニプログラムは現在 `core.AuthUnavailable` を返します。TSX では、`AuthSession.start(url, options)` には `auth_session` ケイパビリティが必要で、返されるセッションの `result` はコールバックで解決されるか、キャンセルされると null で解決されます。

Android は、一致するコールバックを通常の `OnOpenLinks` より先に処理し、コールバックなしで戻るとセッションはキャンセルとして終了します。コールバックにはアプリが所有するプライベートスキームを使う必要があります。空のスキーム、HTTPS のコールバック、宣言されていないスキームの場合は `core.AuthUnavailable` が返り、サインインセッション導入前にビルドされた Android ランタイムでも同様です。

ブラウザーセッションは一致するコールバックのリンクだけを処理します。関係のないリンクや遅れて届いたリンクは引き続き `OnOpenLinks` に届きます。同じコールバック URI で別のブラウザーセッションを開始すると、前のセッションはキャンセルされます。異なる URI であれば、別々のウィンドウで待機できます。

認可 URL を組み立てる関数に渡されたリダイレクトをそのまま使ってください。Web とデスクトップのループバックのリダイレクトは、関数が受け取る前に Unicode のパスをエンコードし、ドットセグメントを解決します。ループバックの `Path` はデコード済みのテキストなので、リテラルの `%` もエスケープされます。

ミニプログラムでは、代わりに `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)` を使います。コールバックは、`Platform`（`MiniProgramWeChat`、`MiniProgramQQ`、`MiniProgramDouyin`、`MiniProgramAlipay`）と、空でない短命の `Code` を持つ `core.MiniProgramCredential` を受け取ります。これをサーバーに送り、ホストのコード交換を行います。アプリのシークレットはサーバー側に保持してください。これは `wx.login`／`qq.login`／`tt.login`、または Alipay の基本的な `auth_base` 認可で、プロフィール情報は要求しません。URL を開いたり OAuth の URL ビルダーを実行したりはしません。

`cx.Env().CanSignInMiniProgram()` はホスト API が利用可能かどうかを報告します。その他のターゲットは `core.MiniProgramSignInUnavailable` を返し、`core.AccessMiniProgramSignIn` を持たないコードは `AccessDenied` を受け取ります。Douyin はデフォルトでサイレントに要求します。ホストにサインインしていない場合は `MiniProgramSignInNotSignedIn` を返し、`MiniProgramSignInOptions{Prompt: true}` を指定するとホストアカウントのログインウィンドウを許可します。そのウィンドウを閉じると `MiniProgramSignInCancelled` が返ります。匿名のデバイスコードがサインイン済みユーザーとみなされることはありません。テストでは `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` と `app.MiniProgramSignIn` を設定し、`app.MiniProgramSignIns` が要求を記録します。TSX はミニプログラムでは実行されません。

ユーザーが自分のブラウザーなど別の場所ですでにサインインしているのと同じ状態で Web ビューをサインインさせたいアプリは、`cx.SetWebCookies` でそのサインインの Cookie を書き込んでから（ページのスクリプトでは設定できない HttpOnly のものも含む）、ページを読み込みます。done はストアに Cookie が保持されると一度だけ実行されます。このストアはアプリのすべての Web ビューが共有する永続的なストアなので、`Expires` を持つ Cookie はアプリより長く残ります。シークレットモードのビューからは見えませんが、アプリごとに 1 つのストアを持つ Android は例外です。HarmonyOS、Web、tvOS、watchOS は `Unavailable` を返します。

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

Cookie を 1 つ削除するには、同じ名前、ドメイン、パスで、`Expires` を過去にして書き込み直します。有効期限が 0 の場合はセッション Cookie になります。削除を反映すべきページを読み込む前に、コールバックを待ってください。

逆方向には、`cx.WebCookies` がストアの Cookie を一覧します。`document.cookie` では読めない HttpOnly の Cookie と、そのドメイン、パス、有効期限、フラグも含み、そのサイトのページが表示されているかどうかは問いません。`core.WebCookieFilter` は、ある `URL` へのリクエストが送る Cookie、またはある `Domain` とその下のすべてのホストの Cookie を選び、ゼロ値のフィルターはすべてを選びます。ユーザーのサインイン状態で Go からサイトを呼び出すアプリや、ユーザーがどこにサインインしているかを表示するアプリは、この方法で読み取ります。`cx.ClearWebsiteData` はユーザーをすべてのサイトからサインアウトさせます。ストアの Cookie、ローカルストレージとセッションストレージ、IndexedDB、Service Worker、キャッシュを削除し、それらがなくなると done が一度実行されます。Android の `CookieManager` は URL が送る名前と値しか返さないため、そこでは `URL` フィルターだけが一覧を返し、`Domain` にそのホストを入れて他のフィールドは空のままにします。`Domain` フィルターやゼロ値のフィルターは `Unavailable` を返します。そこでのクリアでは HTTP キャッシュは残ります。HarmonyOS、Web、tvOS、watchOS はどちらにも `Unavailable` を返します。

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Sign in with Apple

`cx.SignInWithApple` は、`cx.Env().CanSignInWithApple()` が true になる macOS、iOS、tvOS で、アプリ自身のボタンからシステムの Sign in with Apple シートを表示します。資格情報またはエラーで一度だけ応答します。ユーザーがシートを閉じた場合は `core.AppleSignInCancelled` で、これは失敗ではありません。それ以外は `AppleSignInFailed`、その他のプラットフォームでは `AppleSignInUnavailable` です。Apple がメールアドレスと名前を渡すのは、ユーザーがそのアプリに初めてサインインしたときだけなので、そのときに保存してください。`Nonce` は SHA-256 の小文字 16 進数としてリクエストに入り、ID トークンの `nonce` クレームがそれを保持します。サーバーはアプリに渡したノンスをハッシュして比較します。`cx.AppleCredentialState` は、ユーザーがその後に資格情報を取り消したかどうかを示します。ヘッドレステストでは、`app.AppleSignIn` が `nsgotest.AppleSignInRequest` を受け取り、資格情報またはエラーを返します。

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

アプリは Sign in with Apple のエンタイトルメントで署名されている必要があります。`nsgo.yaml` で要求すると `build` が追加します。これがないとシートは失敗します。

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

`nsgo dev` からのデバイス向けビルドでは Xcode がケイパビリティを登録します。`sign.profile` または `sign.macos_profile` のプロビジョニングプロファイルは、Sign in with Apple を有効にした App ID から作成されたものでなければなりません。macOS はプロファイルがある場合にのみこれを許可するため、ディスクイメージや Mac 上の `nsgo dev` の実行ではサインインできません。シミュレーターも同様で、そのビルドは開発者向けのエンタイトルメントを一切要求しません。

## アプリ内課金

`cx.Products`、`cx.Purchase` とその関連の呼び出しは、`nsgo.yaml` で StoreKit を要求したビルドにおいて、`cx.Env().CanPurchase()` が true になる macOS、iOS、tvOS で App Store を通じて販売します。

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

ストアが返す価格 `DisplayPrice` を表示し、アプリ独自の価格は決して表示しないでください。ストアは各ストアフロントの価格を自ら設定します。購入は、トランザクションまたはエラーで一度だけ応答します。ユーザーがシートを閉じた場合は `core.PurchaseCancelled`、「承認と購入のリクエスト」のように承認を待っている間は `core.PurchasePending`、それ以外はストアのメッセージ付きの `core.PurchaseFailed`、その他の環境では `PurchasesUnavailable` です。トランザクションの `Signed`（ストアの JWS）をアプリのサーバーに送って検証し、購入されたものを提供してから、トランザクションを完了させます。ストアは完了していないトランザクションを起動のたびに再びアプリに渡します。

`cx.ManageSubscriptions` は Apple のサブスクリプション管理 UI（iOS ではシステムのシート、macOS と tvOS では App Store のサブスクリプションページ）を開きます。`cx.RequestRefund` はトランザクションの返金リクエストを開始し、`core.RefundSuccess` または `core.RefundCancelled` で応答します。

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction` は、`CurrentEntitlements` が返したものを含め、すでに完了したトランザクションに対しては `PurchaseNotFound` を返します。完了した非消耗型アイテムは引き続き所有されたままです。

アプリの起動時に `OnPurchaseUpdate` を登録してください。ハンドラーより前に届いた更新は、最初のハンドラーを待ちます。`cx.CurrentEntitlements` はユーザーが現在所有しているものを返すので、起動時にそれをアンロックします。「購入を復元」ボタンは `cx.SyncPurchases` を呼び出し、これはユーザーにサインインを求めることがあります。`AccountToken` はアプリ独自の UUID で、トランザクションと App Store のサーバー通知に含まれます。macOS では Mac App Store ビルドでのみ販売でき、App Clip では何も販売できません。Android、watchOS、Windows、Linux、Web、ミニプログラムは現在 `PurchasesUnavailable` を返します。購入のテストには `nsgotest` を使い、その `Products`、`PurchaseAnswer`、`UpdatePurchase`、`UnfinishedTransactions` がストアの代わりになります。`app.PurchaseRequests` はストアの操作を `nsgotest.PurchaseRequest` の値として記録します。TSX では、`Purchases` モジュールには `purchases` ケイパビリティが必要です。`purchase` は `{ status: "purchased", transaction }`、`"pending"`、`"cancelled"` のいずれかで解決され、`on("update", listener)` は更新を受け取ります。

## 画面の向き

フルスクリーンプレーヤーのスコープで `cx.RequestOrientation(core.OrientationLandscape)` を呼び出すと、iOS、Android、HarmonyOS、ブラウザーに横向きを要求します。スコープが生きている最新の要求が有効になります。そのスコープが終了すると、以前の要求が再開されるか、アプリは通常の向きのポリシーに戻ります。

iOS 16 以降では、要求によってインターフェイスを即座に回転させられます。それより前の iOS では、次のシステムによる回転を制限します。表示中のモーダルの `IOS.SupportedOrientations` が優先されます。Android と HarmonyOS では、システムが表現できない縦向きと横向きの組み合わせは、すでに表示されている向きを優先して、許可される 1 つの向きに絞り込まれます。Android は、大画面モードやマルチウィンドウモードでは向きの要求を無視することがあります。

Web アプリでは、プレーヤーのスコープを開くプレスのハンドラーから `desktop.ToggleFullScreen(cx)` を呼び出します。ブラウザーはそのプレスでフルスクリーンに入り、その後 NSGo がプレーヤーの向きの要求を適用します。フルスクリーンを抜けるとロックが解除されます。未対応の操作や拒否はブラウザーのコンソールに報告されます。

`nsgotest.App.Orientation` は最新の要求されたマスクを記録し、通常の向きのポリシーでは 0 になります。

## ストアのレビュー

`cx.RequestReview()` は、macOS と iOS では App Store に、Android では Google Play に、アプリ内評価のプロンプトを要求します。`cx.Env().CanRequestReview()` はサービスが利用可能かどうかを報告します。プロンプトを表示するかどうかとそのタイミングはストアが決め、結果は報告されません。Apple のプロンプトをテストするには開発ビルドを使います。TestFlight のビルドでは表示されません。

スタンドアロンの Android パッケージには、Play Review SDK、そのダイアログのアクティビティ、Play ストアの可視性クエリが含まれます。ビルドに Android SDK や JDK は必要ありません。有効な Play ストアのないデバイスでは、サービスは利用不可と報告されます。リリース済みのパッケージは [内部アプリ共有](https://developer.android.com/guide/playcore/in-app-review/test) を使ってテストします。

NSGo を埋め込む場合は、Gradle のホストに `com.google.android.play:review:2.0.2` を追加します。アプリが Play ストアを参照できるよう、ホストのマニフェストの `<queries>` の下に `<package android:name="com.android.vending" />` を追加します。ホストがコードを圧縮する場合、NSGo は Play Review と Tasks の API を名前で読み込むため、以下のルールでそれらを保持してください。

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

テストでは、`nsgotest.Environment` で `CanRequestReview` を設定し、`nsgotest.App.ReviewRequests` を確認します。

## システムの端のジェスチャー

画面の端にタッチ操作を持つゲームは、その操作がアクティブな間 `mobile.DeferSystemGestures` を呼び出せます。要求を早めに解除するには、返された関数を使います。要求はスコープが後始末されたときにも終了します。同じウィンドウ内の複数のスコープからの要求は結合されます。

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS は物理的な画面の端を使います。Android 10 以降は、左上から測ったサーフェスのポイント単位の矩形を使います。矩形は、それを必要とする操作部分の周囲に限定してください。Android はホームジェスチャーを予約しており、アプリが「戻る」ジェスチャーから除外できる領域に上限を設けています。`nsgotest.App.SystemGestures` は、メインのサーフェスにおける現在の結合された要求をテストに渡します。別のウィンドウには `Surface.SystemGestures()` を使います。

## Android のオーバーレイ

`mobile.OpenOverlay` は、Android で他のアプリの上にアプリのビューを表示します。これはアプリのシグナルを共有する独自のツリーで、表示中はフォアグラウンドサービスがアプリを実行し続けます。アプリは `android.permissions` に `SYSTEM_ALERT_WINDOW` を記述し、ユーザーがシステムの設定でそれを許可します。設定は `mobile.RequestOverlayPermission` で開きます。許可がない場合、オーバーレイはすぐに終了し、`OnEnd` がその理由を受け取ります。オーバーレイはタッチを受け取りますが、`Focusable` または `Passive` でない限り、キーと周囲のタッチは前面のアプリに残します。また、コンテンツから `desktop.SetWindowFrame` で移動できます。その他のプラットフォームでは何も開きません。

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## 位置情報とモーション

`cx.Locate` は位置を 1 回取得し、`cx.WatchLocation` は位置を連続して配信し、`cx.WatchMotion` は加速度計、ジャイロスコープ、磁力計、融合されたデバイスモーションのサンプルを連続して配信します。それぞれ `*core.SensorStream` を返し、その `Stop` でキャンセルできます。開始したスコープが終了した場合も同様です。コールバックはエンジンのゴルーチンで実行されます。権限は `core.PermissionLocation`、`PermissionBackgroundLocation`、`PermissionMotion` です（[権限](permissions.md)）。ストリームの開始で権限を求めることはなく、権限のないストリームは `core.SensorNotAllowed` で失敗します。

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate` は新しい位置を待ちます。デフォルトは 30 秒（`Timeout`）です。キャッシュされた位置を使えるのは `MaxAge` を指定した場合だけで、その場合は `Cached` と `Age` でそれを示します。`Approximate` は、ユーザーがおおよその位置のみを許可した位置であることを示します。Android はそのような位置を最大でも 10 分ごとにしか更新しないので、そこでは `Locate` に `MaxAge` を指定してください。座標は WGS 84 です。プロバイダーが持たない測定値は `Valid` が false になり、`Altitude` は楕円体高、`MSLAltitude` は平均海面からの高さです。モーションはデバイス本来の軸を使い、加速度は m/s²（表を上にして置くと z が +9.81）、回転は rad/s、磁場は µT です。デフォルトでは 20 ms ごとで、10 ms から 1 秒の範囲に制限されます。負荷が高いときは、ストリームはセンサーごとに最新のサンプルを保持し、置き換えたサンプルの数を `Dropped` に数えます。

ストリームはアプリが非アクティブな間も続き、バックグラウンドでは一時停止し、その後の最初のサンプルには `Gap` が付きます。保留中の `Locate` はタイムアウトの残り時間を保持します。`LocationOptions.Background` は、`PermissionBackgroundLocation` の下で位置情報のストリームをバックグラウンドでも続けます。Apple のアプリは `location` バックグラウンドモードを追加し、Android ではフォアグラウンドサービスが `NotificationTitle` と `NotificationBody` を持つ通知を表示して、アクティビティが終了した後もアプリを実行し続けます。システムが終了させたプロセスを起動するものはありません。

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS と watchOS は位置情報とモーションを、macOS は位置情報を、tvOS は一度に 1 回の位置取得を提供し、Android はフレームワーク自身のプロバイダーを通じて両方を提供します。Linux は、システムバス上の GeoClue 2 から位置の取得とストリームを得ます。GeoClue のエージェントはアプリの最初の要求の開始時にユーザーに確認します。GeoClue には確認なしで問い合わせることができないため、権限はその実行中にアプリが要求するまでは未確認として読み取られ、バックグラウンドの位置情報は利用できません。Windows は WinRT の `Geolocator` から得ます。権限は、アプリに対して位置情報がオフの間は拒否、その実行中にアプリが要求して許可された後は許可、それ以外は未確認として読み取られ、こちらでもバックグラウンドの位置情報は利用できません。セキュアなブラウザーは、位置情報の取得と、モバイルでは加速度計、ジャイロスコープ、融合されたモーションの値を提供します。Safari では、プレスのハンドラーから `cx.RequestPermission(core.PermissionMotion, done)` を呼び出してください。ブラウザーは、`navigator.geolocation` がない場合は位置情報を、デスクトップおよび磁力計のストリームについてはモーションを利用不可と報告します。TSX では、`Sensors` が `permission`、`requestPermission`、`locate`、`watchLocation`、`watchMotion` を持ち、パッケージのケイパビリティ `location`、`backgroundLocation`、`motion` の下で、ミリ秒単位で動作します。

## オーディオ

`audio` パッケージは、`cx.Env().CanPlayAudio()` が true の環境で、一度メモリにデコードする短いサウンド（`audio.LoadSound`）と、ストリーミングする長いオーディオ（`audio.NewPlayer`）を再生します。それ以外の環境では、呼び出しは `audio.Unavailable` で失敗します。

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` は再生ごとの `Volume`（0 から 1、0 は 1 として扱う）、`Rate`（0.5 から 2、0 は 1 として扱う）、`Pan`（-1 が左いっぱい、1 が右いっぱい、0 が中央）を設定します。範囲外の値は範囲内に丸められ、NaN はデフォルト値になります。`Sound.Stop` はそのサウンドのすべての再生を停止し、`Close` はサウンドを解放します。所有するスコープが破棄されたときも同様に解放されます。`Player` は `Play`、`Preload`、`Pause`、`Seek`、`SetVolume`、`SetRate`、`SetLoop`、`Close` に対応しています。現在オーディオを再生できるのは macOS、iOS、Windows、Linux、Android、Web です。Apple、Windows、ブラウザーのバックエンドは WAV、AAC、MP3 に対応しています。Linux は GStreamer のコーデックを使い、ストリーミングのプレーヤーは 1× で再生します。Android は、ビューが画面から離れるとプレーヤーを一時停止し、デコードする各サウンドを 1 MiB に制限します。tvOS、watchOS、HarmonyOS、ミニプログラムは、現在も `CanPlayAudio` が false です。

ブラウザーでは、音の出る再生にユーザー操作が必要です。短いサウンドは事前に読み込んでおき、クリック、タップ、キー入力の後に `Sound.Play` を呼び出してください。操作の前に再生したサウンドは無音になります。ブラウザーが再生をブロックした場合、プレーヤーの `Play` のコールバックは `audio.NotAllowed` を受け取ります。ユーザー操作から再試行してください。

`Sound.Play` は `audio.Playback` を返します。macOS、iOS、Windows、Linux、Web では、これを使って個別に音量やレートを設定したり停止したりできます。`Ended()` はリアクティブで、その再生が終了した、停止した、ボイス数の上限で押し出された、または開始できなかった場合に true になります。個別の制御を持たないバックエンドは、終了済みのハンドルを返します。

iOS では、`AudioQueue` のように `audio` と `Video` の外で再生するライブラリ独自のオーディオエンジンは、再生中は `audio.HoldSession(core.AudioSessionPlayback)` でアプリの唯一のオーディオセッションを保持し、停止したら返された解放関数を呼び出します。これは、アプリのビデオ、プレーヤー、録音と並んで、プレーヤーの `AudioSession` と同じレベルを受け取るので、`AudioSessionPlayback` と `AudioSessionMixWithOthers` は着信／サイレントスイッチがサイレントでも再生を続けます。その他の環境では何も保持しません。

効果音を無音で開始するには `audio.Voice{Muted: true}` を、再生中の効果音を無音にするには `playback.SetVolume(0)` を使います。長いオーディオでは、`audio.NewPlayer` がソースをストリーミングし、`player.SetMuted(true)` は再生を続けたまま出力を無音にします。ミュートを解除すると、プレーヤーに設定された音量が戻ります。`Voice.Volume` と `Player.SetVolume` では、0 はデフォルトの最大音量を意味します。

TSX では、`nsgo/audio` が `audio` パッケージケイパビリティの下で `supported()` と `loadSound(uri)` を公開します。`loadSound` はサウンドで解決され、その `play({ volume, rate, pan, loop, muted })` は `setVolume`、`setRate`、`stop`、リアクティブな `ended` を持つ再生を返します。そこでは音量 0 は無音です。拒否された読み込みは `Error` で reject され、その `reason` は `quota-exceeded` のようなオーディオエラーの名前、`code` はケイパビリティがない場合は `ERR_ACCESS_DENIED`、制限を超えた場合は `ERR_NSH_QUOTA_EXCEEDED`、オーディオを再生できない環境では `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM` です。サウンドが解放されるのは `close()` を呼んだときか、そのビューがなくなったときだけです。サンドボックスでは、各読み込みは失敗するかサウンドが閉じられるまで `Limits.Sounds`（デフォルトは 256）に算入されます。

## ゲームパッド

`gamepad` パッケージは、接続されたゲームコントローラーを W3C Standard Gamepad のレイアウト（`ButtonSouth`／`ButtonA` から `ButtonHome` まで、`AxisLeftX`／`AxisLeftY`／`AxisRightX`／`AxisRightY`）で読み取ります。`cx.Env().SupportsGamepads()` は、ターゲットがコントローラーに対応しているかどうかを報告します（macOS、iOS、Windows、Linux、Android、Gamepad API を持つブラウザー）。

`gamepad.Pads(cx)` は追跡されるリアクティブな読み取りで、コントローラーの接続や切断でスコープを再レンダリングしますが、ボタンやスティックの変化では再レンダリングしません。各 `gamepad.Pad` は、`Family`（`FamilyXbox`、`FamilyPlayStation`、`FamilyNintendo`、`FamilyGeneric` のいずれかで、`gamepad.DetectFamily` でも分類できます）、プラットフォームが報告する場合は USB の `VendorID` と `ProductID`、振動モーターへの対応を示す `CanRumble` を公開します。`RequestAnimationFrame` のループ内では、`gamepad.State(cx, id)` が、表示フレームごとに 1 回スナップショットされたコントローラーのボタン（`[0, 1]`、`>= 0.5` で `Pad.Pressed` が true）と軸（`[-1, 1]`、負が左／上、正が右／下）を返します。

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` は、低周波（重い／左）と高周波（軽い／右）の振動モーターを `[0, 1]` に丸めた強度で駆動します。そのコントローラーで実行中の振動を置き換え、`duration` の経過後、アプリがバックグラウンドに移るかフォーカスを失ったとき（`cx.Env().Lifecycle() != core.LifecycleActive`）、`cx` が破棄されたとき、またはコントローラーが切断されたときに自動的に停止します。`gamepad.StopRumble(cx, id)` は振動を早めに停止します。`CanRumble == false` のコントローラーは振動の呼び出しを問題なく無視します。

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

`gamepad.EnableFocusNavigation(cx)` を呼び出すと、コントローラーでネイティブのフォーカス停止位置を移動できるようになります（十字キーと左スティックでフォーカスを移動し、`ButtonSouth` でフォーカスのあるコントロールを実行し、`ButtonEast` で戻ります）。ゲームプレイ中のビューは、マウントされている間 `gamepad.CaptureInput(cx)` を呼び出してフォーカスナビゲーションを一時停止します。TSX では、`nsgo/gamepad` が `gamepad` パッケージケイパビリティの下で `supported()`、`pads()`、`state(id)`、`Button`、`Axis` を公開します。

## オーディオのタイミング

時刻を指定した再生のクロックには `audio.Now(cx)` を使います。`audio.CanSchedule(cx)` が true の場合、`audio.Voice.StartAt` に未来の時刻を設定します。アクティブな出力経路について `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` を呼び出します。macOS、iOS、Linux、Web は時刻指定の開始に対応しています。Windows と Android では `CanSchedule` が false で、`StartAt` を指定してもすぐに再生されます。コールバックの `known` の値は、経路がレイテンシーを報告したかどうかを示します。Windows と Android は報告しません。

## ゲームサービス

`gameservices` パッケージは、`cx.Env().CanUseGameServices()` が true の環境で、プレイヤーのサインイン、実績、リーダーボード、ネイティブの Game Center／Play Games のオーバーレイのための、プロバイダーに依存しない API を提供します。すべてのコールバックはエンジンのゴルーチンで実行されるので、フレームをブロックせずにシグナルを直接設定できます。

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement` は進捗を `0..100` に丸めます（`100` で実績がアンロックされます）。`gameservices.LoadScores` は、`Global` または `Friends` について、`AllTime`、`Week`、`Today` の期間で、順位付けされた `gameservices.ScoreEntry` の行を読み取ります。`gameservices.ShowAchievements` と `gameservices.ShowLeaderboard` はプラットフォームのネイティブのオーバーレイを開きます。macOS、iOS、tvOS は purego 経由で GameKit を使います。Android はランタイムにまだ入っていない Java ランタイムフックを通じて Play Games Services を使う予定のため、現在は `CanUseGameServices() == false` を報告します。Windows、Linux、Web、watchOS、ミニプログラムは `CanUseGameServices() == false` を報告し、`gameservices.ErrUnsupported` を返します。

テストでは、`nsgotest.Environment(core.Environment{CanUseGameServices: true})` でゲームサービスを有効にし、`App.SetGameServicesPlayer`、`App.SetGameServicesFriends`、`App.SeedLeaderboardScores`、`App.SetGameServicesError`、`App.GameServicesAchievements`、`App.GameServicesScores`、`App.GameServicesShownAchievements`、`App.GameServicesShownLeaderboards` で各アプリの独立したフェイクを操作します。

## 次へ

- [通知](notifications.md)：権限、バッジ、プレス。
- [フォーム](forms.md)：テキストフィールドとキーボード。
- [埋め込み](embedding.md)：ホストアプリが埋め込みアプリに提供するサービス。
- [ビルドと配布](building.md)：パッケージに必要な権限とファイル。

### フィットネス、領域、期間限定のモーション

`cx.WatchSteps`、`cx.WatchActivity`、`cx.WatchPressure` はスコープ付きのストリームを返します。歩数／アクティビティの前、および Apple では気圧の前にも、`core.PermissionActivityRecognition` を要求してください。Apple は 3 つすべてを Core Motion で提供します。Android は歩数カウンターと気圧を提供し、アクティビティの分類は利用不可と報告します。カウントはサンプルの `Since` の期間に属し、再開した期間には `Gap` が付きます。気圧はパスカル単位で、任意でメートル単位の相対高度を伴います。欠けている距離、階数、高度は `Valid` が false になります。

`cx.WatchGeofences` は、1 から 20 個の互いに異なる `GeofenceRegion` の円（WGS 84 の座標とメートル単位の半径）を持つ `core.GeofenceOptions` を受け取ります。領域ごとに `GeofenceInside`、`GeofenceOutside`、`GeofenceUnknown` を報告します。`Background` には、バックグラウンドの位置情報へのアクセス／宣言と、Apple の「常に」の許可が必要です。Android では通知のタイトルも必要です。停止または破棄で登録が削除されます。プロセスの終了後も残るサブスクリプションはありません。

`cx.RequestPreciseLocation("route", done)` は正確な位置情報の許可を明示的に要求し、位置ではなくブール値で応答します。`apple.info` では `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}` を宣言します。Android では coarse と fine の両方の権限が必要です。false の応答でも、おおよその位置情報は許可されたままの場合があります。

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` は、期間が限られたライブストリームを要求します。`AccessMotion` と `AccessBackgroundMotion` が必要で、TSX パッケージは `motion` と `backgroundMotion` を宣言します。iOS は実行のリースをより早く失効させることがあります。Android アプリは `android.background_motion: true` を設定して `FOREGROUND_SERVICE` を宣言します。非 sticky のサービスは、要求された期間がより長くても 3 分以内に停止します。期間の終了や OS による失効は `SensorTimedOut` を報告します。停止／破棄でプロバイダーとリースが終了し、終了後は何も記録しません。その他のターゲットは利用不可と報告します。

TSX の `Sensors` はこれら 6 つのメソッドを反映しており、ミリ秒を使います。ヘッドレステストでは `FeedSteps`、`FeedActivity`、`FeedPressure`、`FeedGeofence` でデータを与え、精度の要求には `PreciseLocationAnswer` を設定します。物理的なフィットネス、領域による起動、サスペンド中のバックグラウンドの動作については、引き続き実機での受け入れテストが必要です。
