---
title: 実行とデバッグ
description: ホットリロードを使って任意のターゲットでアプリを実行し、エラー、更新、レイアウト、速度、他言語表示を確認する。
group: ツールチェーン
order: 20
---

## 実行

```sh
go tool nsgo dev
```

`dev` は現在のマシン向け、または `nsgo.yaml` の `dev.target` が指定するターゲット向けにアプリをビルドして起動します。Go ファイルを保存するたびにアプリをリビルドして再起動し、`go.mod` や `go.sum` にないモジュールのパッケージをインポートした場合は先に `go mod tidy` を実行します。TSX のみの変更では、状態を保ったまま実行中のアプリをその場で更新します。アプリを終了すると実行も終了します。

macOS と Windows では、ネイティブの dev リビルド時に、現在のセッションで最後に表示されていた通常の位置とサイズでメインビューのウィンドウが再び開きます。Windows ではフルスクリーン状態も復元されます。Linux では GTK が通常のサイズを復元し、直前のフルスクリーンモードを要求しますが、配置位置はウィンドウマネージャが決めます。

別の環境で実行するには `-target` を使い、iPhone、Apple TV、Apple Watch、シミュレータ、Android デバイスを名前、UDID、シリアルで指定するには `-device` を使います：

```sh
go tool nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
go tool nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target ios                # on the connected iPhone, signed by Xcode
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator, over adb
go tool nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

開発サーバーはデフォルトですべてのインターフェースで待ち受けます。Android が `adb reverse` 経由で接続する場合などループバックのみに留めたい場合は `-host 127.0.0.1` を使うか、ローカルインターフェースの IP アドレスを指定してそこだけで待ち受けてください。Apple 実機で開発機能を使う場合は、LAN 経由で到達可能なアドレスを指定します。

`-h` で全フラグを確認できます。

## Go デバッガ

お使いの Go バージョンと互換性のある [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) を `PATH` 上に `dlv` として配置してください。デバッグ情報を含め、Go の最適化を無効にしてネイティブデスクトップアプリを起動します。

```sh
go tool nsgo dev -target native -debug
```

Delve の `API server listening at` 行からポート番号をコピーします。Go 拡張機能を入れた VS Code では、attach 構成を追加して `40000` をそのポートに置き換えます。

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

GoLand では、ホスト `127.0.0.1` と同じポートで **Go Remote** 実行構成を追加します。アプリの Go コードにブレークポイントを設定してアタッチし、そこに到達する操作を実行してください。Go のリビルドが成功した後は、新しいプロセスにブレークポイントを送るため同じアドレスへデバッガを再接続してください。TSX の変更では引き続きホットリロードが使われます。

`-debug` は macOS、Windows、Linux で `-target native` とともに動作します。デバッグには開発ビルドを使い、リリースビルドの計測には `-release` を使ってください。

## コンポーネントプレビュー

インポート可能な Go パッケージ内のコンポーネントの隣に 1 つ以上の `PreviewXxx` 関数を置きます。それらは preview ビルドタグでガードされた Go ファイルに保存してください。

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

並べて表示したいフィクスチャにチェックを入れ、ロケール、テーマ、ライト/ダークの外観、ビューポートサイズ、レイアウト方向を選択できます。プレビューは最も近いアプリの `nsgo.yaml`、翻訳カタログ、設定された `theme`、リソースを使用します。同じモジュール内の別の設定を選びたい場合は、フラグの後ろにアプリディレクトリを渡してください。その場合、プレビューパスはそのアプリディレクトリからの相対パスになります。

ファイルを保存するとプレビューがリビルドされ、選択状態と表示設定は維持されます。`nsgopreview` タグにより、通常のリリースビルドからはこれらのフィクスチャが除外されます。同じフィクスチャをタグ付きの `_test.go` ファイル内で `nsgotest.Start(t, PreviewEmpty)` とともに使い、`GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` を実行するとネイティブのスクリーンショットを撮ることができます。

`-target ios_simulator` での初回インストール後、Go のみの変更によるリビルドではインストール済みの実行ファイルだけが差し替えられてアプリが再起動されます。`nsgo.yaml`、パッケージ化されたリソース、アプリアイコンを編集すると再インストールが走ります。

Android UI の開発で Go のリビルドサイクルを短縮するには `go tool nsgo dev -target android -android-host` を使います。このコマンドはネイティブランタイムを1度だけインストールし、Go コードは開発マシン上で実行します。Go をリビルドするたびに adb 経由で同じ Activity に再接続します。パッケージ化されたリソース、プリコンパイル済みクラス、ランチャーアイコン、または `nsgo.yaml` を変更するとランタイム APK が再インストールされます。このとき Go プロセスはホスト側のビルド制約、ファイル、ネットワークを使用するため、`Scope.DiskSpace` はホストの `DataDir` があるボリュームを報告します。`_android.go` ファイルや実機上での Go の挙動を確認するには、通常の Android dev コマンドを使用してください。

## エラー

ビルドに失敗すると、エラーをコピーするボタンと再ビルドするボタン付きのオーバーレイが実行中のアプリ上に表示されます。ファイルを修正して保存すれば次のビルドで置き換わります。描画中にコンポーネントがパニックした場合も同じように表示されます。アプリ上で Command+R（Apple プラットフォーム以外では Control+R）を押すと、エラー表示中であってもいつでもアプリをリビルドして再起動できます（`cx.OnKey` や有効なアプリケーションメニュー項目としてアプリ自身がそのキーをバインドしている場合を除く。ブラウザでは通常通りページがリロードされます）。`log.Print` などアプリ自身の出力は、ツールの出力行の下のターミナルに表示されます。

実行前に `go tool nsgo check` を実行するとアプリを vet し、TSX を型チェックします。`-target all` を付けるとすべてのターゲット向けに vet するため、他プラットフォームのビルドタグ配下のコードも Mac からチェックできます。

### パッケージ化された Apple アプリのログ

macOS、iOS、tvOS、watchOS では、デフォルトの Go ロガーは `log.Print`、NSGo の診断メッセージ、TSX のコンソール出力を含めて Apple の統合ログ（unified log）に書き込みます。コンソールアプリで実行中アプリのバンドル識別子をサブシステム、カテゴリを `nsgo` としてフィルタしてください。macOS ではコマンドラインからログストアを検索することもできます：

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

レコードはデフォルトのログ種別を使用し、warning や error のプレフィックスを保持します。ログ内容は公開情報扱いとなるため、認証情報などの機密情報は出力しないでください。長いメッセージは複数のエントリに分割されます。開発中はターミナル出力も引き続き利用できます。カスタムのログライターや `slog` ハンドラを使う場合は各自の出力先を制御します。

### Linux の WebView

Ubuntu 24.04 以降では、AppArmor によって非特権ユーザー名前空間が制限されることがあります。bubblewrap が WebKitGTK のサンドボックスを起動できない場合、NSGo はエラーをログに記録してプレースホルダーを表示します。WebKitGTK のバージョンによっては、最初の WebView を開く際に `SIGTRAP` で終了することがあります。[Ubuntu AppArmor ドキュメント](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

ディストリビューションが `bwrap-userns-restrict` などの bubblewrap 用 AppArmor プロファイルを提供している場合はそれを使ってください。提供されていない場合は、管理者が [NSGo の Linux CI のプロファイル](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml) をたたき台にして `/usr/bin/bwrap` に `userns` を許可するプロファイルを読み込むことができます。システムの bubblewrap を使うすべてのアプリに影響するため、このプロファイルは管理者が管理します。

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` を設定すると WebKit のサブプロセスサンドボックスが無効になります。信頼できるコンテンツを扱うローカル開発専用として使用してください。

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` はブラウザで DevTools を開きます。これは `nsgo dev` によって配信され、スマートフォンのアプリを含む全ターゲットで利用できます。実行中のアプリの状態をリアルタイムで表示します：

- **Elements**：ビューのツリーです。各ノードにはそれを構築したコンポーネント、エディタの該当行へのリンク、テストが要素を見つけるためのセレクタ、クラス、境界、レイアウトされたマージン・ボーダー・パディング、設定されたスタイルが表示されます。行にホバーするとブラウザの DevTools と同様にアプリ上にマージン・パディング・コンテンツが描画され、**Pick** を使うと任意のウィンドウ、パネル、シート、埋め込みビュー内のビューをタップしてツリー上から探し出せます。
- **State**：コードが作成したシグナルが、それを所有するコンポーネントおよびコンピュテーション配下に、値、メモの値、それを読み取っているコンピュテーション数とともに表示されます。どのコンピュテーションも読んでいない値は画面上の何も変更しません。文字列・真偽値・数値はその場で編集できるため、コードを変えずに状態を試せます。NSGo 自身が作成した値は **NSGo's own** の中に格納されています。
- **Profiler**：各コミットの所要時間を、コミット前のあなたのコード、レイアウトを含むコミット処理、プラットフォームのメインスレッドへの適用に分けて表示します。またコミットを引き起こした書き込み行も表示し、実行されたコンピュテーションを所要時間の長い順に開きます。2つ目のリストでは直近1秒間に最も多く実行されたコンピュテーションが作成元の行ごとに表示されます。Android ランタイムは Java コード内でバッチを適用するため、Android のコミットにはプラットフォーム時間が表示されません。
- **Recorder**：アプリ内で行った押下、入力、クリア、スクロールの操作を、`_test.go` ファイルにコピーできる `nsgotest` のテストコードとして書き出します。Elements で **Assert shown** と **Assert text** を押すと選択中のビューの検証ステップを追加できます。テキストの検証はアプリが表示していた言語の文字列を保持するため、テストを実行する言語設定で記録してください。
- **Highlight updates**：`-highlight` と同様に、コミットで変更された各ビューを枠線で強調表示します。
- **Layout**：`-layout` で起動したときと同様に、デバイス上の[レイアウト監査](#layout)を表示・非表示にします。

TSX アプリの状態、コンピュテーション、書き込みは、バンドルのソースマップを通じて TSX の行番号で名前付けされ、アプリ内のファイルへリンクされます。

DevTools は `nsgo dev` のサーバー経由でアプリを読み取ります。このサーバーはスマートフォンからも到達できるようネットワーク上で待ち受けます。実行ごとにトークンが生成され、`nsgo dev` が表示したりブラウザで開いたりする DevTools と Web ターゲットの URL の `#` 以降に付与されます。サーバーはこのトークンを持つリクエストにのみ応答するため、DevTools や Web ターゲットは出力されたアドレスから開いてください。このアドレスを教えた相手は誰でも実行中のアプリの状態を変更したり Recorder のテストに手順を追加したりできるため、記録されたテストは実行前に必ず内容を確認してください。ネットワーク上の他のユーザーや、ブラウザで開いている他サイトの Web ページからこれらにアクセスすることはできません。

## 更新の可視化

```sh
go tool nsgo dev -highlight
```

`-highlight` はコミットによって変更された各ビューを 0.5 秒間枠線で囲むため、アプリを操作しながら意図以上に再描画している更新を画面上で確認できます。枠線はすべてのウィンドウ、パネル、モーダル、iOS シート、埋め込みビュー（`nsgo.NewView`）の上に描画されます。DevTools から実行中にオン・オフを切り替えることもできます。

```sh
go tool nsgo dev -trace
```

`-trace` は実行中アプリの各コミット（番号、時間、送信されたマウント項目）を出力し、更新によってどのビューが作成・変更・削除されたかを確認できます。各項目の上にはそのコミットの原因（前回のコミット以降にシグナルへ書き込んだコード行を書き込み回数の多い順、および実行されたバインディング・エフェクト・メモの数）が表示されます。

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

同じ行が何度も書き込まれている箇所や、1回の書き込みで大量のバインディングが走っている箇所は、書き込みのバッチ化（`cx.Batch`）やバインディングが読み取る範囲の絞り込みを検討するポイントです。`go tool nsgo test -trace` はテストの各ステップの下に同じ情報を出力します。

## ランタイムプロファイル

`nsgo dev` を使って実行中アプリの Go CPU アクティビティ、ヒープ、または実行トレースを取得できます。アプリはキャプチャ結果を開発サーバーへ送信し、サーバーがコマンドを実行しているマシン上に保存します。

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

CPU の例はデスクトップ、iPhone、iOS シミュレータでも動作します。iPhone でシステムからローカルネットワークへのアクセス許可を求められた場合は許可してください。ヒーププロファイルはスナップショットであり Web でも動作します。スケジューラとゴルーチンのイベントを確認するには `go tool nsgo dev -profile trace` を実行し、`go tool trace` で `.nsgo/profiles/execution.trace` を開きます。CPU トレースと実行トレースのデフォルト時間は 10 秒です。`-profile-output` でローカルの保存先パスを指定できます。コンポーネントプレビューでは、デフォルトパスと相対パスはアプリディレクトリを基準にします。dev 実行では最初の完全なキャプチャを保存した後もアプリの実行を継続します。

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Go のブラウザランタイムはヒーププロファイルと実行トレースに対応しています。ブラウザでの CPU サンプリングにはブラウザの Performance ツールを使用してください。`-profile` と `-release` はいずれか一方のみを指定します。`nsgo build` でリリースビルドする際は dev キャプチャクライアントは含まれません。[Go ランタイム診断](https://go.dev/doc/diagnostics)

## レイアウト

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` はすべてのウィンドウ、パネル、iOS シート、埋め込みビューの上に DevTools のレイアウト監査を表示した状態でアプリを起動します。各ビューのボーダーボックスを青、パディングの内側を緑、マージンの外側をオレンジの枠線で描き、スクロールビューやクリップするビューで隠れる部分は切り取られて表示されます。`grid` は 8 ポイント間隔のグリッドを描き、`grid=N` は N ポイント（最小 4）間隔のグリッドを描きます。値は `=` の後に指定し、フラグ単体では `bounds` になります。

隅に表示されるリードアウトにはウィンドウサイズがポイント単位で表示され、ボックス・グリッド・定規のオン/オフを切り替えるボタンがあります。Apple TV などのリモコン専用デバイスでは、アプリを離れずにリモコンで切り替えられるようリードアウトのボタンにもフォーカスが移動します。定規がオンのときは、タップ/クリックがアプリに届く代わりに2点を選択する操作になり、リードアウトに2点間の横方向・縦方向の距離が表示されます。3回目の押下で最初からやり直します。Apple TV やキーボードでは、方向キー（または Siri Remote タッチパッドのスワイプ）で画面上のクロスヘアを動かし、Select、Return、Space で点を選ぶと、2点目が確定するまで距離がリアルタイムに更新されます。

| キー | 効果 |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | `-layout` 付きビルドで監査を表示・非表示 |
| 矢印キー、リモコン十字キー / スワイプ | 定規のクロスヘアをグリッド間隔（デフォルト 8 pt）分移動 |
| `Shift` + 矢印キー | 定規のクロスヘアを 1 pt 移動 |
| Select, `Return`, `Space` | 計測中にクロスヘアの位置の点を選択 |
| `Esc`, 戻る, Menu | 計測を終了 |

アプリが同じキーをバインドしている場合はアプリのバインドが優先されます。スマートフォンなどキーがない環境では `-devtools` も併用し、DevTools の **Layout** から監査を表示・非表示にしてください。リードアウトの下にあるコントロールを操作したいときも非表示にします。ネイティブバックエンドと Web は表示用の変換行列を読み取るため、境界・Pick・監査は表示中のトランスフォーム、アニメーション、スクロールに追従します。watchOS は自身のアフィン描画に追従します。HarmonyOS とミニプログラムでは、描画レイヤーがジオメトリを報告するまで Go 側のトラッカーを使い続けます。これらを再描画すると更新のたびに時間がかかるため、速度計測時は無効にしてください。

## 速度

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` は macOS、iOS、Windows、Linux、Android でアプリ上にパフォーマンスモニタを表示します。フレームとコミットを計測し、Android ではネイティブテキストのラウンドトリップ時間も表示します。dev ビルドには dev クライアントが含まれるため、計測時は `nsgo build` と同様にビルドする `-release` を付けてください。

## 言語と外観

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` は指定したロケール（擬似ロケールも可）でアプリを起動します（[ローカライズ](localization.md)）。

## エージェント

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation` を使うと、AI エージェントや E2E ランナーが人間と同じように実行中のアプリを操作できます。これまでに macOS、Windows、Linux（`-target native`）、Android（`-target android`）、Web（`-target web`）、各開発者ツールのシミュレータ上のミニプログラム（`-target miniprogram_wechat`、`miniprogram_qq`、`miniprogram_alipay`、`miniprogram_douyin`）、および Apple デバイスとシミュレータ（`-target ios`、`tvos`、`watchos`、`ios_simulator`、`tvos_simulator`、`watchos_simulator`）に対応しており、ウィンドウの一覧表示、表示内容の取得、押下、入力、キー送信、スクロール、ドラッグ、ホバー、メニュー項目の選択、アラートへの応答、要素の表示待機、スクリーンショット撮影が可能です。エージェントは `go tool nsgo mcp` を通じて MCP 経由でアクセスします。これはそのディレクトリで実行中の `dev -automation` を利用するか、なければ同じフラグ（`-target android` など）で起動し、クライアント終了時に停止します：

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

他のクライアントでも同様に、アプリのディレクトリで引数 `tool nsgo mcp` を付けて `go` を実行するように設定します。ビューは[テスト](testing.md#a-test)のセレクタ、またはスナップショットが出力する `view:` 参照で指定します（ファイルを保存してアプリが再起動すると `view:` 参照は古くなります）。

エージェントは `dev` が実行している状態のアプリの実際のデータに対して操作を行います。この機能は `dev -automation` ビルドにのみ含まれ、あなただけが読める `.nsgo/automation.yaml` 内のトークンを持つこのマシン上のツールだけが接続できます。

### アプリ定義の MCP ツール

エンジンゴルーチン上で Go アプリまたはライブラリから `automation.Register` を呼び出します。たとえば次の関数は、現在表示中のシーンの ID をエージェントに返します。

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

名前はアプリのグラフ上で一意であり、かつ組み込みツール名と重複しないものでなければなりません。`nsgo mcp` はスコープがクリーンアップされるまでそのツールを一覧に表示します。ハンドラはエンジンゴルーチン上で実行されるため、シグナルの読み取りや設定が可能です。ハンドラは JSON オブジェクトを受け取り、必要な引数を検証して JSON エンコード可能な値またはエラーを返します。

テストでは、アプリ起動時にスコープをキャプチャし、`app.Post` 内で `automation.Call(cx, ctx, name, args)` を呼び出してから、結果やビューを確認する前に `app.Settle()` を呼び出してください。登録機能は `dev -automation` とテストで有効になり、リリースビルドでは何もしない（no-op）動作になります。

## すべてのビルドで使えるフラグ

パッケージ化されたものを含むすべてのアプリビルドは、起動時に NSGo 独自のフラグを受け付けます。これらは `nsgo.yaml` やアプリコードの設定を上書きし、アプリ自身のフラグには渡されません：

| フラグ | 効果 |
| --- | --- |
| `--nsgo-fps` | macOS、iOS、Windows、Linux、Android でパフォーマンスモニタを表示 |
| `--nsgo-lang=zh-Hans` | 指定ロケール（擬似ロケールも可）でアプリを起動 |
| `--nsgo-appearance=dark` | アプリを `light`、`dark`、または `system` 追従で起動 |
| `--nsgo-os=ios` | Web ビルドを `macos`、`windows`、`linux`、`ios`、`android` として実行（`web` はそのまま） |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

Web ではページのクエリ文字列で渡します：`?nsgo-fps&nsgo-lang=de`。

`--nsgo-os` はブラウザ上で他プラットフォームのレイアウトをプレビューします。アプリはその OS とイディオム（`ios` と `android` ならスマートフォン、それ以外はデスクトップ）を読み取り、`Env.Simulated` は true を返します。ブラウザにはそのプラットフォームのネイティブ部品はないため、システム標準の代わりに `NavHeader` と自前描画のタブバーが使われ、アプリケーションメニュー・ネイティブポップアップメニュー（`desktop.PopupMenu`）・ウィンドウ関数はなくなり、`mobile.IsAvailable` と `desktop.IsAvailable` は false になります。`ui.ContextMenu` や `ui.Menu` など自前描画のメニューは引き続き動作し、ショートカットは閲覧者のキーボードに従います。他のビルドではこのフラグは無視され、警告がログに出力されます。
