---
title: セットアップ
description: NSGo に必要なものをインストールし、アプリを作成してすべてのターゲットで実行します。
group: はじめに
order: 10
---

## 必要要件

- [Go](https://go.dev/dl/) 1.27.3 以降。
- iOS、tvOS、watchOS 向けにビルドする場合は、Xcode をインストールした Mac が必要です。それ以外のターゲットはすべて macOS 上で Go のみを使ってビルドできます。x86_64 Android エミュレータ向けビルドには Android NDK も必要です。
- iPhone 実機で実行する場合は、Xcode の Accounts に Apple ID を登録し、iPhone を信頼済みにして、デベロッパモードを有効にする必要があります。
- Android で実行する場合は、Android SDK の platform-tools に含まれる `adb` が必要です。
- Windows で実行する場合は Go のみで動作し、Windows および Web パッケージのビルドも行えます。
- Linux では GTK 4.14 以降、Pango、Cairo が必要です。ネットワーク画像には libsoup 3、Web ビューには WebKitGTK 6.0、オーディオとビデオには GStreamer プラグインを使用します。AppArmor によるユーザー名前空間の制限があるシステムでは、[WebKitGTK に bubblewrap プロファイルが必要です](debugging.md#linux-の-webview)。

## アプリを作成する

モジュールを作成して NSGo とそのツールを追加し、ツールにアプリを生成させます：

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` はモジュールに `nsgo` を一度だけ追加するため、何かを別途インストールしなくても `go tool nsgo` を実行できます。`init` は次のファイルを生成します：

| ファイル | 内容 |
| --- | --- |
| `app.go` | アプリ本体（カウンター）：`Root` コンポーネントと、`nsgo.Run(Root)` を呼び出す `main` |
| `app_test.go` | ボタンを押すテスト |
| `nsgo.yaml` | アプリの名前、ID、バージョン、ターゲット |

代わりに `go tool nsgo init -template tsx` とすると TSX アプリが生成され、`-template mixed` とすると、TSX エントリから独自の Go コンポーネントを利用する Go アプリが生成されます。

### Go モジュールからのテンプレート

テンプレートのモジュールパスと、必要に応じてバージョンを渡します。バージョンを省略した場合は、Go のモジュールプロキシで見つかる最新バージョンに解決されます。

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

スターターテンプレートはモジュールの `template/` ディレクトリ内に公開します。`nsgo.yaml` と `main.go` を含めてください。ツールはこの 2 つと、`.tmpl` で終わるすべてのファイルを Go の `text/template` でレンダリングし、出力ファイル名から `.tmpl` を取り除きます。バイナリアセットを含むその他のファイルは、変更せずにそのままコピーします。

テンプレートには `.Name`（アプリの表示名）、`.ID`（生成されたアプリ ID）、`.Slug`（小文字でハイフン区切りの名前）、`.Module`（`-module` の値、またはスラッグ）が渡されます。Go はモジュールのダウンロード時にネストされた `go.mod` ファイルを除外するため、生成されるアプリのモジュールと依存関係を宣言するには `go.mod.tmpl` を同梱してください。`go.mod.tmpl` がない場合、`-module` は組み込みテンプレートと同じ最小限の `go.mod` を生成します。

## 実行する

```sh
go tool nsgo dev
```

`dev` はこのマシン向けにアプリをビルドして開きます。ファイルを保存するたびにリビルドしてアプリを再起動し、TSX のみの変更では実行中のアプリをその場で更新します。ビルドに失敗するとアプリの上にエラーが表示され、エラーをコピーするボタンと再ビルドするボタンが用意されます。

`-target` を使って別の環境で実行することもできます：

```sh
go tool nsgo dev -target web                # ブラウザで実行
go tool nsgo dev -target ios_simulator      # iOS シミュレータで実行
go tool nsgo dev -target tvos_simulator     # Apple TV シミュレータで実行
go tool nsgo dev -target watchos_simulator  # Apple Watch シミュレータで実行
go tool nsgo dev -target android            # 接続したデバイスまたはエミュレータで実行
```

`go tool nsgo doctor` はすべてのターゲットについて、このマシンでビルドおよび実行できるかどうかと、実行できない場合に何が不足しているか（Xcode、`adb`、接続済みデバイス、GTK など）を一覧表示します。

[実行とデバッグ](debugging.md) では、デバイス指定、更新のトレース、パフォーマンスモニター、他言語表示について解説しています。

## コーディングエージェントと一緒に使う

NSGo のスキルは、Claude Code、Codex、Cursor などのコーディングエージェントに、NSGo アプリの書き方、テスト方法、出荷方法を教えます。アプリのディレクトリに追加するか、`-g` を付けてすべてのプロジェクトに追加してください：

```sh
bunx skills add nsgo-dev/nsgo
```

ドキュメントを直接読むエージェントは <https://nsgo.dev/llms.txt> から開始でき、そこにはすべてのページが `https://nsgo.dev/docs/<page>.md` の Markdown として一覧表示されています。また、<https://nsgo.dev/llms-full.txt> には全ページがまとめられています。エージェントは実行中のアプリを操作することもできます（[実行とデバッグ](debugging.md#エージェント)）。

## 次のステップ

- [例：Todo アプリ](tutorials/todo.md): 状態、リスト、テストを約 10 分で学びます。
- [例：メモアプリ](tutorials/notes.md): ナビゲーションスタックとデバイス上に保存されるメモ。
- [例：人気の GitHub リポジトリ](tutorials/repos.md): エラー処理と再試行を含むネットワークからの読み込み。
