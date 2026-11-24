---
title: 組み込み
description: 独自の iOS、Android、デスクトップ、または Web アプリの中に NSGo アプリを組み込む。
group: ツールチェーン
order: 23
---

## 組み込み可能なアプリのビルド

組み込み可能なアプリとは、`Root` と `main` を持つ通常のアプリパッケージを、ホストのターゲット向けに `-embed` を付けてビルドしたものです。各ビューがアプリを1つずつ実行し、それぞれが独立したエンジンとなるため、ホストはそれぞれ独立した状態を持つビューを好きなだけ表示できます。

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

iOS ビルドは `<Name>.xcframework` と `<Name>Resources` を生成し、Android ビルドは `<Name>.aar` を生成し、Web ビルドは `nsgo.json` とローダーを含むディレクトリを生成します。

## iOS

アプリが `base/terminal` をインポートしている場合、ビルドはアプリフレームワークの隣に `GhosttyVT.xcframework` も出力します。静的・`-dynamic` のどちらのアプリフレームワークを使う場合も、このコンパニオンを **Embed & Sign** でホストターゲットに追加してください。これには実機用、Intel / Apple Silicon シミュレータ用、Mac Catalyst 用のライブラリが含まれており、`GhosttyVT.framework` という名前はそのまま維持してください。

アプリフレームワークは静的ライブラリとしてアプリターゲットに追加し（埋め込み＝Embed はしない）、リソースフォルダはフォルダ参照（folder reference）として追加した上で `import <Name>` します。最初のビューを作る前、アプリの起動時に一度だけ `NSGoStart()` を呼び出してください。静的ライブラリのコードはホストアプリの一部になるため、ホスト側のプライバシーマニフェストに NSGo が使用する Required Reason API（user defaults `CA92.1`、file timestamps `C617.1`、system boot time `35F9.1`）を宣言する必要があります。`-dynamic` フレームワークの場合は自身の `PrivacyInfo.xcprivacy` に含まれています。

```swift
import Embedded
import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
	func application(_ app: UIApplication, didFinishLaunchingWithOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
		NSGoStart()
		return true
	}
}

let view = NSGoView(frame: .zero)
view.params = ["user": "Ada"]              // cx.HostParams()
view.dataDirectory = documents.path        // where storage lives
view.localeIdentifier = "de-CH"            // in place of the system's
view.setFontScale(1.3)
view.overrideUserInterfaceStyle = .dark
view.tokens = ["accent": "#cf222e"]        // over the app's theme
view.handle("time") { _, reply in reply(Date().description, nil) }
view.send("message", ["text": "Hello"])    // cx.OnHostEvent("message", …)
view.onOpenURL = { url in router.open(url) }
view.onError = { message in log(message) }
view.register(component: "host.badge") { context in Badge(context) }
```

`NSGoView` はウィンドウに追加された時点でアプリを起動し、解放されたときまたは `stop()` が呼ばれたときに停止します。起動時パラメータはウィンドウに追加する前に設定し、それ以外の設定はいつでも変更できます。ハンドラ、コンポーネント、コールバックはメインスレッド上で実行され、応答（reply）は後から任意のスレッドで返すことができます。システムバーの設定に追従させるには、ビューコントローラからそのビューの `preferredStatusBarStyle`、`prefersStatusBarHidden`、`prefersHomeIndicatorAutoHidden` を転送してください。
また、`mobile.DeferSystemGestures` に追従させるには、コントローラからビューの `preferredScreenEdgesDeferringSystemGestures` を返すこともできます。

## Android

`base/terminal` をインポートする arm64 アプリの場合、AAR には `libnsgo.so` の隣に `jni/arm64-v8a/libghostty-vt.so` が含まれます。ホストではネイティブライブラリの展開（extraction）を有効のままにしておいてください。Android 向け事前ビルドとターミナルローダーには [#1452](https://github.com/nsgo-dev/nsgo/issues/1452) が必要であり、x86_64 用のターミナルライブラリはまだ利用できません。

AAR をアプリモジュールに追加し、ランタイムがネイティブライブラリを見つけられるよう展開設定を有効にします：

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

コードまたはレイアウト上で `dev.oneo.nsgo.NsgoView` を作成します。各ビューは最初にサイズが決まったときに独自のプロセスでアプリを起動し、`stop()` で終了します。アクティビティの結果をすべてのビューに転送し、設定変更時にアプリが再起動しないようアクティビティ側で設定変更をハンドリングしてください。

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));
app.setStorageDirectory(dir);
app.setLocale(Locale.forLanguageTag("de-CH"));
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);
app.handle("time", (data, reply) -> reply.resolve(now()));
app.handle("tapped", (data, reply) -> status.setText("tapped " + ((Map<?, ?>) data).get("count")));
app.setOpenUrlHandler(url -> router.open(url));
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);
```

```java
@Override
protected void onActivityResult(int request, int result, Intent data) {
	super.onActivityResult(request, result, data);
	for (NsgoView app : apps) app.activityResult(request, result, data);
}
```

Go アプリを生存させたままネイティブビューツリーを切り離すには、ホスト側で `NsgoEngine` を保持し、ビューを明示的にアタッチします：

```java
NsgoEngine engine = new NsgoEngine(context.getApplicationContext());
app.attachToEngine(engine);
// Before taking the view off screen:
app.detachFromEngine();
// Reuse the same view later:
app.attachToEngine(engine);
// When the host ends the app:
engine.destroy();
```

デタッチするとネイティブの子ビューが解放され、再アタッチするとコンポーネントを再実行することなく現在のシグナル、入力、フォーカス、スクロール位置、描画内容が復元されます。Web ページとメディアプレーヤーはアプリ設定から再作成されます。明示的に渡されたエンジンに対しては `stop()` はデタッチのみを行います。各エンジンは最初のビューと Activity を保持し、別のビューの割り当ては拒否するため、その Activity 内で設定変更を処理してください。Activity の差し替えや1プロセス内で複数の独立したビューを表示することはまだサポートされていません。

Google Play のアップデートチェックを使うには、Android ホストの Gradle 依存関係に `com.google.android.play:app-update:2.1.0` を追加してください。NSGo が名前で読み込む AppUpdateManager、AppUpdateInfo、AppUpdateOptions、Tasks の各 API を、[ランタイムのシュリンクルール](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro)を使って保護してください。上記のアクティビティ結果の転送により、アップデートのキャンセルも通知されます。

組み込みホストで Android の認証セッションを使う場合は、ホスト側の Activity にプライベートコールバックスキームを宣言し、`onNewIntent`（および新規 `onCreate`）で受け取ったリンクを `view.openLink(uri.toString())` に転送してください。一致するアクティブなセッションがそのコールバックを消費し、それ以外のリンクは `OnOpenLinks` に届きます。スタンドアロンの `NsgoActivity` はこれらを自動で転送します。

## デスクトップ

Go のホストがプラットフォームのイベントループを回し、各ビューの `Handle` をウィンドウに配置します。`nsgo.NewView` はハンドルがウィンドウに入った時点でアプリを起動するため、それ以前に行った設定は最初のフレームから反映されます。

```go
channel := core.NewHost()
channel.SetParams(map[string]any{"user": "Ada"})
channel.Handle("time", func(c *core.HostCall) { c.Resolve(time.Now().Format("15:04:05")) })
channel.HandleOpenURL(func(url string, answer func(bool)) {
	answer(strings.HasPrefix(url, "https://example.com/orders/"))
})

view := nsgo.NewView(app.Root, nsgo.HostChannel(channel), nsgo.DataDir(dataDir))
defer view.Close()

view.SetTokens(core.Tokens{Values: map[string]string{"accent": "#cf222e"}})
view.SetLocale("de-CH")
view.SetFontScale(1.3)
view.SetAppearance(core.AppearanceDark)
handle := view.Handle() // an NSView, GtkWidget or child window
```

`view.Host()` はアプリへイベントを送り、アプリからの呼び出しを受け取ります。`nsgo.Component` はホストプラットフォーム向けのネイティブコンポーネントを登録します。

## Web

Web ビルドは単独のページにもなりますが、既存のページ内の要素にマウントしてホストになることもできます。`mount(element, url, options)` はプレーヤーを返し、`nsgo.json` は自身からの相対パスでローダー、wasm、リソースを指定します。

```html
<div id="app"></div>
<script type="module">
const url = new URL("./Embedded-0.1.0-web/nsgo.json", location.href);
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url));

const player = await mount(document.getElementById("app"), url.href, {
	config,
	params: { user: "Ada" },
	host: {
		call(name) {
			if (name === "time") return new Date().toLocaleTimeString();
			throw new Error(`no host function ${name}`);
		},
		post(name, data) { console.log(name, data); },
		openURL(url) { return url.startsWith("https://example.com/orders/"); },
	},
});
player.hostEvent("message", { text: "Hello from the page" });
</script>
```

`host` オプションを省略すると、ローダーは `nsgo.yaml` の `web.host` が指定するモジュールをインポートし、そのモジュールが `createHost(player)` をエクスポートしていればそれを await します。要素やフレーム内に組み込まれたアプリは、ブラウザの履歴とタイトルをホストページ側に委ねます。

`appearance`（`"light"`、`"dark"`、または `"system"`）、`locale`、`fontScale` はネイティブホストの設定と同様にブラウザ設定の代わりとして機能し、プレーヤーの `setAppearance`、`setLocale`、`setFontScale` で後から変更できます。`module` にコンパイル済みの `WebAssembly.Module` またはその Promise を渡すと、アプリを何度もマウントするページでダウンロードとコンパイルを1回だけに抑えられます。

ローダーはアプリ起動前に `progress.nsgo-load-progress` 要素を表示します。CSS でスタイルを付けるか、マウントオプションに `onProgress` を渡すか、アプリ単独ページ向けには `web.host` から `onProgress` をエクスポートしてください。このコールバックは標準のプログレスバーを置き換え、デコード済みバイト数としての `{ loaded, total }`（サイズ不明時は `total` が `null`）を受け取ります。起動完了時に UI を片付けられるよう、最後に `null` を渡して呼び出します。コンパイル済みの `module` を渡した場合はダウンロード進捗はスキップされます。

```yaml
web:
  host: host.js
```

## ホストチャネル

`core.Host` はすべてのホストが実装する単一のコントラクトです。起動パラメータ、ホストが送るイベント、アプリが呼び出すホスト関数を定義します。値は JSON と同様にやり取りされるため、マップは `map[string]any`、数値は `float64` となり、同じコードが Swift、Java、Go、JavaScript で動作します。

- `HostParams` は起動パラメータを起動時に一度だけ読み取ります。
- `cx.OnHostEvent` は `Host.Send` やページ側の `hostEvent` を受信します。
- `cx.CallHost` は名前付きハンドラを呼び出して応答を解決し、`cx.PostHost` は応答なしで呼び出します。
- `Host.HandleOpenURL` はプラットフォームより先に `cx.OpenURL` を受け取り、その返答によってプラットフォーム側でも開くかどうかを決めます。
- `Host.SetParams`、`Send`、`Handle`、`HandleDefault` はどのゴルーチンからでも安全に呼び出せます。

iOS ではホストが `params` を設定し、`handle` でハンドラを登録し、`send` でイベントを送り、`onOpenURL` で応答します。Android では `setParams`、`handle`、`send`、`setOpenUrlHandler` で同じことを行います。デスクトップでは `core.Host` のメソッドを使い、Web ではページの `host` オブジェクトが応答します。

## ネイティブコンポーネント

`core.Native` はホストが作成したビューを表示します。その `Props` はホストコンポーネントの `SetProp` に届き、コンポーネントが発火したイベントは `OnEvent` に届き、アニメーションノードを値に持つ prop はエンジンに負荷をかけずにフレームクロック上で滑らかに動きます。

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

ホスト側でコンポーネントを登録します。iOS では `app.register(component:)` と `NSGoComponent`、Android では `registerComponent` と `NsgoComponent`、デスクトップでは `nsgo.Component` を使います。Web にはネイティブコンポーネントがないため、未知の名前は空のボックスとして表示されます。

## 設定

`tokens` はアプリのテーマをロール名（`accent`、`inset`、`raised`）単位で、ライト・ダーク両方または片方について上書きし、`cx.Palette()` から参照されます。ホストはロケール、文字サイズ、外観、データディレクトリも設定でき、アプリの障害通知も受け取ります。`nsgo build -embed` はウィンドウ、メニュー、トレイアイコンを書き出さず、それらはホスト側の管轄になります。

ビューのエンジンゴルーチン上で内部パニックが発生すると、そのエンジンが停止し、アタッチされたサーフェスが破棄され、パニックとスタックトレースがホストのエラーリスナー（Go では `nsgo.OnError`）に報告されます。他のエンジンは動作し続けます。失敗したアプリを再起動するには新しいビューを作成してください。アプリコールバックでの通常のパニックは引き続き最も近い `core.Catch` に渡されます。

`examples/embed-ios`、`examples/embed-android`、`examples/embed-desktop`、`examples/embed-web` は、同じアプリを各種ホストに組み込んだサンプルです。

## 次のステップ

- [プラットフォームサービス](services.md)：アプリがプラットフォームに要求できる機能。
- [ビルドと配布](building.md)：`-embed` が作るパッケージとその署名。
- [例：Todo アプリ](tutorials/todo.md)：ホストが組み込む対象となるアプリ。
