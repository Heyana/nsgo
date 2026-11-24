---
title: コンポーネントと状態
description: 要素、コンポーネント、シグナル、スタイル。
group: コアコンセプト
order: 50
---

## 要素

画面は Go の構造体値である要素（element）のツリーです。`core` には `View`、`Text`、`Image`、`Scroll`、`Input` などのプリミティブがあります。`ui` には `Button`、`Select`、`DatePicker`、`EmojiPicker` など、アプリが使うコントロールがテーマに沿った見た目で用意されています。（`base` には見た目のない同じコントロールがあり、[コンポーネント作者](authoring/index.md)向けです。）

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` は Web コンテンツを表示します。空の `WebSource` を渡すと `about:blank` を読み込み、通常の読み込みイベントやナビゲーションイベントを報告します。前のページをクリアするには、`SourceFunc` に空のソースを設定するか `Ref.Load(core.WebSource{})` を呼び出します。ページ自身のスクリプトより前に実行するスクリプトがシグナルに追従する場合は、`InjectedJavaScriptBeforeContentFunc` を設定してください。値が変化すると、ユーザーや `Ref.Load` で遷移したページであっても、現在表示中のページの代わりに新しいスクリプトで元のソースを再読み込みします。プラットフォーム自身のユーザーエージェント（または `UserAgent` が設定されている場合はその後ろ）に `Version/26.0 Safari/605.1.15` のようなトークンを追加するには、`UserAgentSuffix`（TSX では `userAgentSuffix`）を設定します。macOS と iOS では、プラットフォーム標準へのサフィックス追加はビューの初回作成時のみ有効で、HarmonyOS と Web ではそれぞれのユーザーエージェントが使われます。`Reload`、`GoBack`、`GoForward` を呼び出すには `core.Ref` を取り付けます。ネイティブターゲットでは、これらのコマンドはアプリ自身の HTML やデータソースに対する `core.AccessWebSchemes` の制限を免除したままにしますが、新しいナビゲーションを発生させるページは引き続きアクセスポリシーに従います。

tvOS では `cx.Env().HasWebView()` が false になります。`WebView` をマウントすると、WebKit を読み込まずに `OnError` 経由で `*core.WebError` が報告されるため、そのターゲット向けのアプリコンテンツを用意してください。

macOS、iOS、Windows、Linux では、`OnNavigationStateChange` は同一ドキュメント内での URL、履歴、タイトルの変更も報告します。

ページの `getUserMedia` リクエストを許可するには、`WebView.Media` に `core.WebMicrophone`、`core.WebCamera`、またはその両方を設定します。`apple.info` に使用目的の説明を、また `android.permissions` に `RECORD_AUDIO` または `CAMERA` を宣言してください。実行時には引き続きシステムがユーザーに許可を求めます。Android では、システムプロンプトが開いている間にアプリが要求デバイスの一部を削除したり、ページを離れたり、ビューを閉じたりすると、保留中のリクエストは拒否されます。そのような変更の後は、ページが再度アクセスを要求する必要があります。

`WebView.Muted` と `MutedFunc`（TSX では `muted`）は、Windows と Linux で Web Audio や埋め込みフレームを含むページ内のすべての音声を消音します。macOS 12 および iOS 15 以降では、消音中はメディア再生が一時停止されます。その他のホストでは通常通り再生されます。この更新は現在のページに適用され、ナビゲーションをまたいでも引き継がれます。

TSX では、`injectedJavaScriptBeforeContentLoaded` を変更すると、`InjectedJavaScriptBeforeContentFunc` と同様に同じ WebView 内で新しいスクリプトを使ってソースを再読み込みします。一方、`injectedJavaScript` を変更すると新しい WebView が作成されてソースを最初から読み込みます（`userAgent`、`userAgentSuffix`、`mobile` の変更も同様です）。HarmonyOS では、どちらの注入スクリプトを変更しても新しい WebView が作成され、新しいネイティブ履歴が始まります。Windows と HarmonyOS では、各注入スクリプトがメインフレーム内のブロックで実行されるため、トップレベルの `let`、`const`、`class` 宣言はそのスクリプト内にローカルに留まります。ページから参照できるグローバル変数を定義したい場合は `window` に代入してください。

Windows、macOS、Linux で、YouTube Shorts のようにスマートフォンでのみ縦スワイプで動画を切り替えるようなサイトをスマートフォン向け表示にするには、`Mobile`（TSX では `mobile`）を設定します。ページには（`UserAgent` が設定されていなければ）スマートフォンのユーザーエージェントとホバーなしのタッチスクリーン情報が渡され、マウスでのドラッグやスクロールしない領域上でのホイール・トラックパッドのスクロールが指のスワイプとして届きます。Windows は Chrome のデバイスモードと同様にスマートフォンをエミュレートします。macOS と Linux はスクリプトでこれを実現しているため、ページのスタイルシートからは依然としてマウスが見え、タッチイベントは非信頼（untrusted）となるためページ側で無視される場合があります。iOS、Android、HarmonyOS、Web、ミニプログラムではページがそのまま表示されます。

macOS、iOS、Windows、Linux では、Web ページからの新規ウィンドウ要求は `OnOpenWindow` が受け取ります。ビューはポップアップを作成しません。リクエストの `URL` と `UserInitiated` を使って、`Ref.Load` で同じビューを遷移させるか、別の場所で URL を開くか、無視するかを判断してください。制限付きビューでは、拒否されたリクエストは `OnError` に送られます。

macOS、iOS、Linux、Windows、Android で WebView のページがダウンロードするファイルを保存するには `OnDownload` を設定します。ダウンロードはアプリのデータディレクトリ内の `downloads/<id>/<file name>` に保存され、開始、バイト数付きの完了、または失敗が報告されます。`OnDownload` が設定されていないビューや制限付きビューではダウンロードがキャンセルされます。

`core.Video` はプラットフォームのプレーヤーでファイルまたは URL を再生します。macOS、iOS、tvOS では、有限の長さの動画クリップに対して音声を含む別のファイルや URL を `Video.AudioSource` に設定できます。プレーヤーの音声はそのソースから再生されます。映像と音声は同時に開始し、`Ref.Seek` は両方を同じタイムライン上でシークします。再生時間は動画側で決まり、音声が短い場合は終了後に無音になります。

透過クリップは、macOS ではアルファ付き HEVC の `.mov`（iOS は未確認）、Linux では GStreamer の codecalpha プラグイン（`gstreamer1.0-plugins-bad`、Linux パッケージの `nsgo.yaml` で推奨依存に指定可能）を使ったアルファ付き WebM として背景を透かして表示します。macOS、iOS、Windows、Linux で、アルファを保持できないローカルクリップ（Apple プラットフォームでの WebM、Linux での HEVC、および Windows 機で Media Foundation のアルファ出力が確認されるまでの Windows 上のあらゆるファイル）は、不透明な箱として表示する代わりに `OnError` で通知します。このときのエラーは `Reason` が `core.MediaErrorAlphaUnsupported` の `*core.MediaError` であり、アプリ側でそのクリップを自前デコードできるようにしています。Android、Web、および全プラットフォームのリモート URL は、デコーダーが出力する通りに再生するため、不透明になる場合があります。

`OnPause` と `OnPlay` は、Apple、Android、Windows、Linux、Web の組み込みコントロールに追従します。`OnPause` はコントロールによる一時停止に対して `VideoPauseControls` を渡します。`Paused` はソースが開かれたとき、またはその値が変化したときに適用されます。音量、再生速度、レイアウトの変更ではネイティブ側の一時停止や再生状態が維持され、`Paused` が true のままコントロールから再生した場合も含まれます。アプリ独自の再生ボタンを表示する場合は、`PausedFunc` を状態にバインドし、それらのコールバック内で状態を更新してください。

`Rate` は再生速度を設定し、`RateFunc` は再生中に速度を変更できます。速度コントロールを表示するには `OnLoad` から得られる `VideoInfo.RateChangeable` を使ってください。Linux では、GstPlay と GStreamer の `scaletempo` フィルターがインストールされていればシーク可能な動画の速度を変更できます。フォールバックのプレーヤーは 1× を使用します。

Android や、Linux、macOS、iOS、tvOS での MP4 / MOV といった直接 HTTP ファイルで `Referer`、`User-Agent`、`Cookie` などの HTTP リクエストヘッダーを送信するには、`Video.Headers` を設定します。

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

ビデオはマウント時にこのマップをコピーし、`SourceFunc` が新しい URL を返した場合も同じヘッダーを使用します。Apple プラットフォームでシークを可能にするには、バイト長が既知で Range リクエストに対応したソースを使ってください。プレーヤーは同一オリジンのリダイレクト先にも設定されたヘッダーを送信します。Apple プラットフォームでは、同じヘッダーが直接 HTTP の `AudioSource` ファイルにも適用されます。`.m3u` または `.m3u8` で終わるプレイリスト URL はネイティブローダーを使用します。

Linux では、GstPlay と GStreamer の `scaletempo` および `souphttpsrc` プラグインをインストールしてください。`User-Agent` を設定する場合、またはお使いの `souphttpsrc` に `location-trusted` プロパティがない場合は、最終的なメディア URL を使用してください。そのプロパティがあり、かつデフォルトのユーザーエージェントを使う場合、GStreamer は同一オリジンリダイレクトでもアプリ側のヘッダーを保持します。アプリのヘッダーはソースファイルに適用され、アダプティブメディアのセグメントは独自のリクエストを使用します。

## コンポーネント

コンポーネントは、スコープを受け取り要素を返す関数です。一度だけ実行され、その後の変化はシグナルによって駆動されます。

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

props を持つコンポーネントは、`Render` メソッドを持つ型です：

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` は `NoClose` で閉じるボタンを省略したり、`Trailing` ノードを追加したり、`OnPress` でトーストを閉じたりできます。`Toasts.NoClose` はスタック全体の閉じるボタンを非表示にします。

`ui.Toasts.Style` と `StyleFunc` は、背景、ボーダー、角丸、パディングを含むすべてのトーストのサーフェスをカスタマイズします。スタックのスタイルがテーマの上にマージされ、続いて各 `ToastContent.Style` と `StyleFunc` が適用されます。各 `StyleFunc` は対応する `Style` の後に実行され、参照しているシグナルが変化すると更新されます。これらのスタイルで `Color` を指定するとタイトルにその色が使われ、説明文には不透明度を下げた色が使われるため、暗い背景や色のついたサーフェス上でも文字が読みやすくなります。登場、退場、スワイプの各フェーズには引き続きモーションスタイルが適用されます。TSX では、`Toasts` の `style` prop を設定し、1つのトーストに対しては `show` に `style` または `styleFunc` を渡します。

## シグナル

シグナルは値を保持します。`ContentFunc` のように要素が呼び出す関数の中でシグナルを読み取ると、画面のその部分だけが購読し、値が変わったときにその部分だけが更新されます。

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

スライス全体のシグナルは、それを読むすべての行を更新します。`core.NewCollection` は各要素をキーごとに個別のシグナルとして保持するため、各行が自身の要素に追従し、1つの要素を変更してもその1行だけが再実行されます：

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

その他の状態管理も同じようにシグナルの上に構築されています：

- `core.NewSelector` はあるキーが選択されているかを通知し、選択が外れた行と新たに選択された行だけを実行します。
- `core.NewHistory` はアンドゥ・リドゥ付きの値を保持し、Edit メニュー用に `CanUndo` と `CanRedo` シグナルを提供します。
- `core.NewPersisted` は型付きの値をバージョン付き JSON としてアプリのストレージに保持し、古いバージョンが書き込んだデータをマイグレーションします。
- `core.From` は別のゴルーチンから配信されるサブスクリプションなど、グラフ外のソースに追従します。

## スタイル

`core.Style` は CSS に準拠しています。フレックスボックスとグリッド、パディングとマージン、角丸、ボーダー、シャドウ、グラデーション、トランスフォーム、そして `Active` などの状態を備えています。要素は `Class` を通じて Tailwind クラスも受け取れます：

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

テーマはライトモードとダークモードで `ui` コンポーネントの見た目を提供します。

## デスクトップアプリ

`nsgo.Run(Root)` は1つのルートビューを表示します。代わりにデスクトップアプリが自身で表示するものを開くこともできます。`nsgo.Run(App)` は `func App(app *core.Scope)` を1回実行し、そこから `desktop` を使ってシグナルを共有するウィンドウ、パネル、トレイアイコン、グローバルショートカットを開きます。これは macOS、Windows、Linux で動作します。macOS では `examples/tray` は Dock に出さずメニューバーに常駐します。`nsgo.Run(App, nsgo.StartHidden())` はメインウィンドウを非表示のまま開くため、トレイアプリやログイン時にシステムが起動したアプリ（`desktop.LaunchedAtLogin`）で一瞬ウィンドウがちらつくのを防ぎます。アプリは `app.MainWindow().Show()` で表示します。スマートフォンと Web は起動時の状態のままでアプリを表示します。

`desktop.SetWindowLevel` はタイマーやデスクトップペットのようにウィンドウを他より前面または背面に保ち、`desktop.SetWindowFrame` は移動とサイズ変更を行います。ウィンドウのスコープでこれらが有効かどうかは `desktop.CanSetWindowLevel` と `desktop.CanPlaceWindows` で確認できます。macOS、Windows、X11 上の Linux ではどちらも有効ですが、通常の Wayland トップレベルウィンドウではコンポジタが重ね順と配置を決めるためどちらも効きません。

`desktop.WindowOptions{Transparent: true}` で開いたウィンドウ、または `nsgo.WindowTransparent()` を指定したメインウィンドウには背景もフレームもないため、デスクトップペットやウィジェットのようにアプリが何も描画しない部分にはデスクトップが透けて見えます。`Desktop.WindowDrag` を持つビューでウィンドウを移動できます。`NoShadow` はシステムの影を消し、`SkipTaskbar` は Windows のタスクバーや Alt+Tab、X11 のタスクリストからウィンドウを隠します。後から変更するには `desktop.SetWindowTransparent`、`SetWindowShadow`、`SetSkipTaskbar` を使います。Windows では透過状態で開いたウィンドウでのみ透過度を変更でき（影はありません）、Wayland にはタスクリストから隠すヒントがありません。ウィンドウのスコープでこれらが有効かは `desktop.CanMakeWindowsTransparent` と `desktop.CanSkipTaskbar` で確認できます。透過には Windows では DirectComposition、Linux ではコンポジタが必要で（コンポジタのない X11 では機能しません）、macOS では `desktop.SetDockVisible` でアプリを Dock から非表示にできます。

## ターミナル

`base/terminal` をインポートし、リモートストリームには `terminal.New(terminal.Options{Conn: conn})` でセッションを作成するか、macOS、Linux、Windows 上のローカルプログラムには `Command` を設定します。`terminal.View{Terminal: session}` をレンダリングし、セッション終了時に `Close` を呼び出します。`Options.ImageLimit` は Kitty グラフィックスのストレージ上限をバイト単位で指定します。0 は 64 MiB を使い、負の値は画像を無効にします。プログラムは RGB、RGBA、または PNG 画像と Unicode プレースホルダー配置を送信できます。

`Search(text)` を呼び出すと画面とスクロールバック内の一致箇所がハイライトされ、`SearchNext` と `SearchPrevious` でそれらを選択し、`EndSearch` でクリアします。`Options.OnSearch` は `SearchStatus` を報告し、`Theme.Search`、`SearchText`、`SearchSelected`、`SearchSelectedText` は一致箇所の色を設定します。OSC 133 マーカーを出力するシェルでは、アクティブなコマンド入力内を Alt/Option+クリックしてカーソルを移動できます。NSGo はシェル統合スクリプトを自動ではインストールしません。
