---
title: 開発セットアップ
description: ソースから NSGo をビルドし、ギャラリーを実行して変更をテストする。
group: コントリビュート
order: 80
---

## 要件

Xcode と Go 1.27.3 以降を入れた macOS があれば、Android の APK を含め arm64 向けの全ターゲットをビルドできます。実機で動かすには[セットアップ](getting-started.md#必要要件)に記載されている環境が必要で、x86_64 Android エミュレータ向けのターゲット `android_x64` には Go が android/amd64 を cgo ありでしかリンクしないため Android NDK が必要です。Windows のセットアップとアンインストールのプログラムは Go だけでビルドできます。

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

コードを変更する前に [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) と [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) を読んでください。

## ギャラリー

ギャラリーには各要素、`base` および `ui` コンポーネント、各種サービスのデモが揃っています。広いウィンドウではサイドバーのタブにデモが表示され、狭いウィンドウでは下部バーにタブと設定が表示され、その上にデモが遷移表示されます。リポジトリのルートから実行します：

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## チェックとテスト

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` を指定すると、ホストの代わりに指定したターゲットを vet し、NSGo のリポジトリ内では通常の `check` が vet する開発クライアントもチェックします。変更が触れるプラットフォームをすべてチェックするには、`check -target web,android -jobs 2` のように1回の `check` にまとめて指定してください。複数回に分けて起動する必要はありません。`-jobs n` は最大 n 個のターゲットを同時に vet し、各 `go vet` で n 個のコンパイルワーカーを使います（デフォルトは Go のデフォルト設定で1ターゲットずつ実行）。同じモジュール内で別の check が実行中の場合は、終わるまで待機する旨を表示して待ちます。CI の `check.yml` はすべてのターゲットを vet します。

リポジトリルートまたは `internal/dev` で実行すると、ヘッドレスコマンドは `internal/dev` を開発クライアントのファイルに必要な `nsgodev` ビルドタグ付きでもう一度テストするため、そのテストがローカルでも CI でも実行されます。両方の実行は同じフィルタとフラグを受け取り、1つの集計と JUnit レポートを共有します。2回目の実行のパッケージ名は `internal/dev [nsgodev]` になります。

`check -target all` はどのホスト上でも macOS、Windows、Linux、Android arm64、Web のターゲットを vet します。Xcode SDK が利用可能な場合は iOS、tvOS、watchOS も vet し、スキップしたターゲットを一覧表示します。`check -target ios` のように明示的に指定したターゲットは、ツールが利用できなければ（`all` と組み合わせた場合でも）失敗します。あるターゲットが失敗しても残りのターゲットのチェックは継続され、必要なチェックのいずれかが失敗するとコマンド全体が失敗します。`build -target all` には引き続き Xcode を入れた macOS が必要です。NSGo のリポジトリ内では、指定された各ターゲットについて `nsgotest` のプラットフォーム用テストホストも vet されます。プレビュー版の `harmonyos` ターゲットは `all` に含まれないため、CI で明示的に指定されています。

Linux の CI ジョブでは、HarmonyOS Go バックエンドの `js/wasm` テストを Node.js と Go の Wasm ランチャーで実行します。ローカルでギャラリーテストを実行するには、`hdc` 経由で HarmonyOS エミュレータを接続してください。[HarmonyOS ホストテンプレート](building.md#harmonyos)のコンパイルには OpenHarmony SDK API 26 が必要です。

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

Linux デスクトップの Wayland セッションでは、`NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux` で実際のグローバルショートカットポータル受け入れテストを実行できます。ID には `.desktop` エントリが必要です。コンポジタのプロンプトで Ctrl+Alt+K を許可した後、別のアプリケーションにフォーカスを移して押してください。このテストは登録と有効化の両方を待機します。環境変数が設定されていない場合はスキップされます。

Linux ホストで Docker が利用できる場合、`go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` を実行すると、使い捨ての Debian、Ubuntu、Arch コンテナ内で実際のインストールを確認できます。公式イメージをダウンロードして内部に GTK 依存関係をインストールし、各パッケージデータベースとインストールされたファイルを検証した後、パッケージを削除します。ホスト側のパッケージデータベースは一切使用されません。

## リリース

NSGo は、下流プロジェクトがタグ付きバージョンを必要とするとき、またはメンテナが出荷準備のできた変更をまとめたときにオンデマンドでリリースされます。ライブラリ向けにリリースをリクエストする場合は、依存先の作業にリンクし、その変更に必要な最後の NSGo コミットを明記してください。そのコミットを含む最初のリリースが、そのライブラリが指定できるバージョンになります。

タグ付け前にマージ済みの変更をテストするには、依存元モジュールから `go get github.com/nsgo-dev/nsgo@main` を実行します。検証済みのリビジョンを指定したい場合は、`main` の代わりにコミット ID を使えます。Go はその結果の擬似バージョンを `go.mod` に記録します。

メンテナは `go run ./tools/changelog release X.Y.Z` を実行して `CHANGELOG.md` の変更にバージョンと日付を付与し、新しい Unreleased セクションを残します。それを `main` にマージしたら、そのバージョンで[リリースワークフロー](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml)を実行します：`gh workflow run release.yml -f version=X.Y.Z`。ワークフローは `main` の最新コミットですべてのチェックとテストを実行し、すべて通過したときにだけそのコミットに `vX.Y.Z` のタグを付けて公開するため、失敗してもバージョン番号は失われません。公開されたノートは `CHANGELOG.md`（未リリースの変更のみ保持）から [Releases](https://github.com/nsgo-dev/nsgo/releases) へと移されます。

## NSH

NSGo は設計の参照として [Native Shell](https://github.com/longbridge/nsh) を参照しています。テストには不要です。コードを読む場合は、リポジトリの隣にクローンし、`main` ではなく最新のリリースをチェックアウトしてください：

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## 次のステップ

- [コードベース](contributing/codebase.md)：NSGo の各パートがどこにあるか。
- [変更を加える](contributing/changes.md)：プルリクエストに必要なこと。
