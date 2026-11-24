---
title: TSX アプリ
description: Node 不要で事前コンパイルされ、Tailwind クラスを使って TSX でアプリを書く。
group: コアコンセプト
order: 63
---

## TSX アプリ

TSX アプリは、自身のソースを埋め込んでエントリーを実行する Go パッケージです：

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx` で作成できるほか、`examples/tsx` もサンプルとして用意されています。`nsgo dev` と `nsgo build` は TSX を事前にコンパイルするため、パッケージにコンパイラは含まれません。一方、`go run` と `go test` はアプリの起動時にコンパイルを行います。いずれの場合も Node は不要です。

## コンポーネントとモジュール

TSX では、プリミティブを `nsgo` から、`base` のコンポーネントを `nsgo/base` から、ゲームコントローラーを `nsgo/gamepad`（署名付きパッケージでは `gamepad` ケーパビリティ配下）から、サウンドを `nsgo/audio`（`audio` 配下）から、そしてアプリが `tsx/ui` をインポートしている場合は `ui` のコンポーネントを `nsgo/ui` からインポートします。これらの一覧は `examples/gallery-tsx` で確認できます。

Mermaid 図を使用するには、Go ホスト側で `tsx/textview` または `tsx/ui` とともに `github.com/nsgo-dev/nsgo/tsx/diagram` を副作用目的でインポートします。これにより、図パーサーを使用しないアプリにリンクすることなく、`nsgo/base` に `Diagram` が追加されます。`source`、`fontSize`、部分的な `theme`、`accessibilityLabel`、および通常のスタイリング用 props を受け取ります。直接使うか、TextView レンダラーとして指定してください：

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

同じ `renderDiagram` prop は `nsgo/ui` の TextView でも動作します。Mermaid 以外のコードブロックでは通常のレンダラーが使われ、無効な図の場合はそのソースが表示されます。

組版された数式を使用するには、Go ホスト側で `tsx/textview` または `tsx/ui` とともに `github.com/nsgo-dev/nsgo/tsx/formula` を副作用目的でインポートします。これにより `nsgo/base` に `Formula` が追加されます。このパッケージを省いたアプリのバイナリには数式エンジンとフォントは含まれません。`source`、`dialect`（デフォルトは `tex`、または `typst`）、`display`、`accessibilityLabel`、およびスタイリング用 props を受け取ります。フォントサイズと色は周囲のテキストに従います。直接使うか、TextView レンダラーとして指定してください：

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

同じ `renderMath` は `nsgo/ui` の TextView でも動作します。コピー時には数式の TeX が保持されます。Go の `Renderer` は引き続きホスト側の設定として扱われます。

`UrlScheme.url()` は追跡中のアプリケーションアドレスを読み取り、`UrlScheme.setUrl(url, { history: "push" })` はアドレスエントリを追加します（オプションを省略した場合は現在のエントリを置き換えます）。`UrlScheme.on("change", fn)` はブラウザーの戻る・進む・ハッシュ変更を監視し、購読解除用の関数を返します。ルーティング、同一オリジン検証、ディープリンクのホスティングについては[アプリケーションアドレス](addresses.md)を参照してください。

## TSX から Go を利用する

TSX からインポートできるよう Go 側を登録します：

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

すると `import { Badge, greet } from "app/native"` によって `Badge` のフィールドを props として渡し、`greet` を呼び出せるようになります。`context.Context` を受け取る関数は Promise になり、`tsx.RegisterClass` を使うと Go の型を TSX から `new` で生成できるクラスにできます。

省略可能な prop に `undefined` が渡された場合はそのデフォルト値が使われます。デフォルト値が型のゼロ値と異なるゲッターを持つ Go コンポーネントを登録する場合は、`tsx.Default("Max", float64(100))` のようなオプションを追加してください。コンポーネントを置き換えることなく、リアクティブな prop が undefined を返している間も同じデフォルト値が適用されます。

ドキュメント化された Go のデフォルト値を持つデータフィールドも省略できます。`Flow` ノードの `width` と `height` がない場合は JSX コンテンツから計測され、エッジの `width` を省略した場合はデフォルトの線が使用され、クォータ行の `pace` を省略した場合はペースマークが非表示のままになります。

Go のコンポーネントやクラスを登録すると、アプリの実行時にリフレクションによってエクスポートされたフィールドやメソッドが検索されます。この検索があるために、TSX が登録するものだけでなくアプリ全体において、どこからも呼ばれないエクスポート済みメソッドを Go リンカーが削除できなくなります。そのため、`tsx` をリンクするとストリップ済みのデスクトップバイナリでも約 20 MB 増加し、アプリがリンクするパッケージが多いほどさらに大きくなります。

## 署名付きパッケージ

アプリはダウンロードした TSX（新しく配信された自身の画面やプラグインなど）を、パッケージの検証後に実行できます。パッケージのコードがアクセスできる対象を `nsgo.yaml` で宣言してください。それ以外のアクセスはすべて拒否されます：

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

ルートキーはオフラインに保管し、それを使ってリリースキーを認証したうえで、そのディレクトリ内のアプリに署名します。`go tool nsgo keygen` は鍵ペアを出力するだけでファイルには書き込みません。ルートキー用と各リリースキー用でそれぞれ 1 回ずつ実行してください。公開鍵はフラグやアプリのコードに記述し、秘密鍵はリポジトリに含めず、`NSGO_ROOT_KEY` と `NSGO_RELEASE_KEY` を通じてのみツールへ渡します：

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

アプリはコード内に書かれたルートの公開鍵を信頼し、エンジンのゴルーチン外でパッケージを検証したうえで、自身のビューの隣にサンドボックス化された状態で `tsx.View` に表示します。パッケージはビューから与えられたホストチャネルを通じてのみアプリへアクセスでき、アプリのストレージやシークレットの独自領域を保持します：

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // refuses tampered, revoked and older builds
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // once it ran, older builds are refused
```

`Sandbox.Grant` はパッケージが宣言しているよりも少ない権限を付与でき、`Sandbox.Limits` は別の時間バジェットやクォータを設定し、`Sandbox.Modules` は NSGo 固有のもの以外の Go 関数やコンポーネントを提供します。パッケージのキーボードショートカット、戻るハンドラ、ナビゲーションスタックは、キーボードフォーカスがそのビュー内にある間だけ有効になり、自身でフォーカスを移動する操作（`focus()`、`autoFocus`、`Keyboard.dismiss`、`setAccessibilityFocus`）も、すでにフォーカスがそこにある間、またはその中でのユーザーのプレスに応答する場合にのみ機能します。パッケージは親から与えられた領域いっぱいに（それ以上にはならず）専用のビューとして表示され、メニュー、ポップオーバー、トーストを含めてクリッピングされます。スクロールビュー内でコンテンツの高さに合わせたい場合は、別のビューの中に入れてください。ダイアログ、アラート、コンテキストメニュー、アナウンスメントも同様の方法でのみ表示され、戻る操作はそのダイアログを取り除かなくても閉じます。`View.Export` はパッケージのエントリーモジュールがエクスポートするコンポーネント名を指定し、エントリーがマウントするルートの代わりにそのコンポーネントを表示します（その場合、エントリーの `mount` はそのコンポーネントをレンダリングも実行もしません）。また、ビューごとにコピーされる JSON エンコード可能なマップである `View.Props` を渡せるため、1 つのパッケージから同じケーパビリティと制限のもとでそれぞれ独自のステートを持つ複数のビューを提供できます。指定したエクスポートが見つからない場合は `tsx.ErrNoViewExport` でパニックし、関数でない場合は `tsx.ErrViewExportNotComponent` でパニックします。エントリーが関数をエクスポートするパッケージは関数のプラグインとなります。`tsx.Load(cx, p, tsx.Sandbox{})` は、ビューツリーに属する `cx` のもとで（ビューツリーに属さない場合は `tsx.ErrNoTree` を返します）呼び出し元のゴルーチン上で表示なしに同期的に実行するか、読み込み時にエントリーモジュールがスローした値を返し、`Plugin.Call` でそれらの関数を呼び出します。`go tool nsgo dev -limits` はこのようなビューと同じ制限下でアプリを実行し、`go tool nsgo revoke` は `Trust.UpdateRevocations` 向けに失効したリリースキーとパッケージの一覧に署名します。`examples/plugins` はパッケージのケーパビリティの付与をユーザーに求め、インストールして実行する例です。デフォルトで 64 MiB の `Limits.Memory` は、配列、文字列、バッファー、型付き配列に対する計測対象のネイティブストレージのバジェットを設定します。これには `concat`、`flat`、`slice`、配列スプレッド、文字列連結、テンプレートリテラル、`replace`、`replaceAll`、`TextEncoder`、`Blob` のコピーが含まれます。`-limits` に `memory=<bytes>` を指定するとバジェットを変更でき、`memory=0` で無効化できます。チェックは割り当て前に実行されます。Go が回収可能なストレージを確認した後でもバジェットを超過したパッケージは、`ERR_NSH_QUOTA_EXCEEDED` とリソース `memory` を報告して停止します。通常のオブジェクト、クロージャー、スタック、小さな割り当て、計測対象外の組み込み機能やホスト API もメモリを割り当てることがあります。ブラウザーは独自のヒープを保持します。`pkg.Build` を使って自らパッケージに署名するアプリや、ファイルのバイト列を添えて `go tool nsgo publish -extra <file>` を実行する場合は、プラグインがメニューに追加する項目など独自のメタデータを `pkg.Contents.Extra` に含めることができます。これはパッケージとともに署名され、`Verify` は変更せずそのまま `p.Extra` として返すため、アプリはパッケージのコードを実行することなく解析して読み取れます。

アプリは `nsgo keygen` が出力する鍵を使って、プラグインのカタログなど独自の JSON ドキュメントに `pkg.SignDocument` で署名し、`Signed.Open` で検証できます。`app:catalog-v1` のように、ドキュメントの種類ごとにバージョン付きの独自の `app:` コンテキストを与えてください。ある種類のドキュメントが別の種類として、あるいはパッケージ、証明書、失効リストとして検証を通ることはありません。`Open` は渡された公開鍵のみを信頼し（証明書や失効リストは使用しません）、デコード先の型に存在しないボディフィールドを拒否します。エンベロープには時刻やシーケンス番号が含まれないため、古いドキュメントも新しいドキュメントと同様に検証を通ります。ボディ内にバージョンや有効期限を含めて確認してください。
ネイティブのサンドボックスでは、`JSON.stringify`、reviver 付きの `JSON.parse`、`Array.prototype.join`、`toString`、`toLocaleString`、`flat`（したがって配列に対する `String()` も）は、入れ子になった呼び出しを含めて（配列の `toString` とそれが呼び出す `join` はそれぞれ 1 段として数えられます）深さが 1,000 階層を超えると、キャッチ可能な `RangeError` をスローします。それより深いデータは呼び出しをハングさせたりアプリを終了させたりするためです。Web ではブラウザーの組み込み実装が使われます。

ネイティブターゲットでは、`DefaultLimits.Stack` は 1,000 フレームです。`Limits.Stack` または `-limits stack=<frames>` を設定することで、最大 10,000 フレームまで別の深さを選択できます。0 以下の Go の値および上限を超える入力には 10,000 が使われます。Web ではブラウザーのスタック制限が使われます。

互いに呼び出し合うネイティブ関数も JS の呼び出しと同様に `Limits.Stack` に対してカウントされます。組み込みのゲッター、`toString`、`valueOf`、自身の別メソッドを `exec` とする正規表現、入れ子になった `Reflect.apply` や `Reflect.construct`、バインドされた関数やプロキシの連鎖は、上限を超えるとアプリを終了させるのではなくキャッチ可能な `RangeError` をスローし、それより深くネストされたプラグインの値は Go へ変換する呼び出しで `RangeError` により失敗します。これは NSGo が必要とする goja のフォーク（`github.com/nsgo-dev/goja`）によるもので、`go.mod` を変更しなくても NSGo とともにアプリへ組み込まれます。

ネイティブターゲットでは、`Limits.Call` はタイマーコールバックや `await` 後の Promise 継続を含む各 JavaScript スライスを制限します。スライスが打ち切られると、保留中の `Plugin.Call` は `tsx.ErrBudget` を 1 度報告し、残りのタイマーを停止し、遅れて届いたホスト応答を無視します。中断されたスライス内でキューに積まれた Promise リアクションを含め、独立した呼び出しは実行を継続します。別の呼び出しの Promise を解決・拒否する呼び出しであっても、自身の結果またはエラーを報告します。バックトラックを行う正規表現は、ロードバジェットとコールバジェットのうち短い方をマッチタイムアウトとして使用し、処理されなかったタイムアウトも `tsx.ErrBudget` になります。

`Limits.CallTotal` は、各プラグイン呼び出しとサンドボックス化されたビューイベントに対して実時間（ウォールクロック）の期限を追加します。これには待機時間も含まれ、呼び出しが解決・拒否された後でもそのタイマー、インターバル、フレーム、Promise 継続、ホスト応答を保持します。期限に達すると保留中の処理は破棄され、まだ待機中の呼び出しは `tsx.ErrBudget` で失敗します。エントリーモジュールの処理は `Limits.Load`（`Load` がゼロの場合は `CallTotal`）に従います。独立したホストイベントは配信時点から `CallTotal` を使用します。待機中呼び出しの期限へのハンドルを取得するには `Plugin.CallWithBudget` を使用してください。ホストの `Scope.Post` 内でその期限前に `budget.Extend(duration)` を呼び出すと、この呼び出しにのみ追加の時間を与えることができます。

`Plugin.Interrupt` は任意のゴルーチンからランタイムを停止し、保留中のタイマーを破棄して、現在および以降の呼び出しを失敗させます。`Plugin.Stopped()` と `Plugin.Err()` も任意のゴルーチンから読み取ることができます。`Err()` はランタイムの動作中は nil であり、停止すると最初の停止理由を返します。通常どおりプラグインを閉じるかその所有者を破棄すると、クリーンアップを実行して `tsx.ErrPluginClosed` で停止します。待機中の呼び出しが失敗した後にエンジンのゴルーチン上でその理由を 1 度受け取るには、`tsx.Load` の呼び出し時に `Sandbox.OnStop` を設定してください。

関数プラグインの読み込み時に `Sandbox.OnError` を設定すると、待機中の `Plugin.Call` の外で発生したキャッチされないタイマー、アニメーションフレーム、`queueMicrotask`、`reportError` の失敗を受け取れます。各プラグインのハンドラはエンジンゴルーチン上で実行されるため、ホストは失敗の原因を特定し、サーキットブレーカーが開いたときに `Plugin.Interrupt` を呼び出せます。ネイティブのスライスバジェット打ち切りも、呼び出しがすでに完了している Promise ジョブを含めて `tsx.ErrBudget` を報告します。通常のコールバックエラーではプラグインは実行を続けます。ロードエラーや呼び出し結果のエラーは引き続き呼び出し元に返され、ビューでは引き続き `core.Catch` が使われます。ハンドラがない場合は、既存の境界およびアプリレポートの動作が維持されます。

`Sandbox.OnViewRuntime` を設定すると、エントリーモジュールの実行前にエンジンのゴルーチン上で各ビューの `*tsx.ViewRuntime` を受け取れます。任意のゴルーチンから `Interrupt` を呼び出すと、処理を停止して最後にレンダリングされたビューをそのまま残します。`Stopped` と `Err` は終了状態を報告し、通常の破棄時には `tsx.ErrViewClosed` となります。`Sandbox.OnStop` もビューの終了理由を 1 度報告します。

`Plugin.JSTime` と `ViewRuntime.JSTime` は、現在のエントリーを含むネイティブランタイムの JavaScript エントリーで費やされた経過時間を報告します。エントリー間の待機時間や、登録された同期 Go 関数内の時間は除外されます。`ViewRuntime.EngineTime` には、同期 Go 呼び出しおよび所有するリアクティブ、マウント、コミット、レイアウト処理（現在実行中の処理を含む）も含まれます。エンジンエントリー間の待機時間、ホストや他のビューに属する処理、共有コミット管理処理、ネイティブ描画は除外されます。どちらのカウンターも任意のゴルーチンから安全に読み取れます。NSGo 外部のブラウザーの JavaScript ターンは計測もプリエンプトもできないため、そこでは両方のカウンターが利用不可を報告し、`Interrupt` はホストが制御を取り戻した際に以降の NSGo コールバックを停止します。

Web ではまだパッケージをページから分離することができません。そのコードはケーパビリティを超えて、ページ自身のネットワーク、クリップボード、ストレージ（トラストストアのものを含む）にアクセスできてしまいます。そのため、Web では `tsx.View` と `tsx.Load` は `tsx.ErrUnisolated` を返してパッケージを拒否します。ただし、`Sandbox.WebUnisolated` を設定してページの権限で実行する場合を除きます。これはアプリ自身の権限を任せられるほど信頼できるパッケージにのみ適しています。

## エディターとテスト

`.nsgo/tsconfig.json` を継承する `tsconfig.json` により、`go tool nsgo dev` や `go tool nsgo check` が出力する TSX の型情報をエディターで利用できます。アプリの横に配置し、`nsgo/test` で記述した `*.test.tsx` ファイルは、Go のテストとともに実行されます（[テスト](testing.md)）。
