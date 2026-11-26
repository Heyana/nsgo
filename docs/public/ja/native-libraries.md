---
title: ネイティブライブラリ
description: NSGo の通常のパッケージ化、ライセンス、署名手順を通じて、ビルド済みの共有ライブラリと Apple のフレームワークを Go パッケージに同梱して配布し、それらが必要とする Linux パッケージを指定します。
group: ツールチェーン
order: 23
---

Go パッケージは、パッケージディレクトリに `nsgo-natives.yaml` を配置することで、purego 経由で読み込む共有ライブラリを同梱して配布できます。バージョン 2 からは iOS と tvOS のフレームワークも配布でき、必要な Linux パッケージも指定できます。アプリがそのパッケージをインポートすると、NSGo はターゲットに応じた宣言を選択し、ファイルを検証して、署名やインストーラー作成の前にそれらを組み込みます。アプリ側に追加の設定やパッケージ化コマンドは不要です。

デスクトップアプリのビルドとターゲットでのテストでは、アプリのコードに対して cgo が無効になります。ネイティブコードには purego と、`nsgo-natives.yaml` で宣言したライブラリを使用してください。NSGo は cgo ファイルを含むアプリのパッケージを、ビルドの開始前に拒否します。

## ライブラリの宣言

```yaml
version: 2
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

バージョン 2 でサポートされているキーは `macos_arm64`、`macos_x64`、`windows_arm64`、`windows_x64`、`linux_arm64`、`linux_x64`、`ios`、`ios_simulator`、`tvos`、`tvos_simulator` です。バージョン 1 は 6 つのデスクトップキーのみをサポートします。ファイルはそのバージョンのキーについてのみ読み込まれるため、iOS と tvOS のビルドはバージョン 1 のファイルから何もパッケージ化しません。Go パッケージが選択されるすべてのターゲットを宣言してください。`[]` はそのターゲットにバンドルされたライブラリが不要であることを明示します。代わりに Go のビルドタグを使って、サポートしていないターゲットからパッケージを除外することもできます。`macos_universal` には 2 つの macOS エントリが使用され、ターミナルターゲットにはそのデスクトップのエントリが使用されます。たとえば `tui_macos_arm64` には `macos_arm64` が使用されます。

各ライブラリには以下の項目があります。

- `name`: インストールされるファイル名。macOS では `.dylib`、Windows では `.dll`、Linux では `.so` またはバージョン付きの `.so.*` で終わる必要があります。iOS と tvOS ではフレームワーク名 `<Name>.framework` で、`Name` は ASCII 英字、数字、`.`、`-` からなり、英字または数字で始まり英字または数字で終わる必要があります。
- このパッケージディレクトリからの相対パスである `path`、またはライブラリ自体を指す HTTP(S) URL である `url` のどちらか一方のみを指定します。バージョン 2 では https の URL のみを受け付け、リダイレクトが https を離れるとダウンロードは失敗します。アーカイブの URL ではなく、展開済みのライブラリを公開してください。フレームワークの場合はそのバイナリのみを公開します。URL にはユーザー名やパスワードを埋め込まないでください。
- `sha256`: 元のファイルの SHA-256 ハッシュ値。どちらのソースを指定する場合も必須です。たとえば macOS では `shasum -a 256 lib/macos-arm64/libexample.dylib` で出力できます。
- `licenses`: このパッケージディレクトリからの相対パスで指定する、元のライセンスおよび通知ファイルの空でないパス一覧。これらのテキストは `THIRD_PARTY_NOTICES.txt` に同梱され、リリースビルドの `core.ThirdPartyNotices()` で取得できるようになります。アプリの既存の[ライセンスポリシー](building.md#ライセンス表記)（完全一致によるモジュール例外を含む）は、ネイティブライセンスにも独立して適用されます。

ローカルパスおよびシンボリックリンクは宣言元パッケージの内部に留まる必要があります。NSGo は未知のフィールド、チェックサムの不一致、別の OS やアーキテクチャ向けのファイル、およびインストール名の競合を拒否します。2 つのパッケージが同じチェックサムを持つ同名ファイルを共有することは可能です。1 ファイルあたりのサイズ上限は 1 GiB です。

## パッケージ化と読み込み

| ターゲット | ライブラリディレクトリ |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows、Linux、ターミナルターゲット | 実行ファイルの隣 |
| iOS および tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

パッケージはそのディレクトリからライブラリを読み込みます。その際、作業ディレクトリではなく実行ファイルの場所を基準にします。依存関係は、同一ディレクトリ内の関連ライブラリが配布先のディレクトリから読み込まれるようにあらかじめビルドしておいてください。必要な関連ファイルと、ローダーが必要とする各ファイル名をすべて宣言してください。

NSGo はキャッシュされたダウンロードを再利用する前に再度チェックし、ファイルロックによって並行ビルドを調整します。コンテンツキャッシュは OS のユーザーキャッシュディレクトリ配下の `nsgo/natives` に保存されます。ローカルファイルはコピー時に検証されます。

ユニバーサル macOS パッケージでは、両アーキテクチャで同じライブラリ名を指定する必要があります。NSGo は `lipo` でスライスを結合します。配布元がすでにユニバーサル化したライブラリをどちらのエントリに指定することもできます。ライブラリはアプリより先に署名され、その後アプリの通常のディスクイメージ作成、署名、公証手順を通過します。Windows の署名では、`build.installers: false` の場合でも宣言された DLL が署名対象に含まれます。Linux インストーラーには、実行ファイルディレクトリと同じファイルおよび通知が含まれます。Mac では、ターミナルターゲットは macOS のライブラリにアドホック署名を行います。それ以外のホストでは公開されたまま（ユニバーサルなものはそのまま）同梱し、署名されていない arm64 のライブラリは拒否します。

開発時、`-run`、およびターゲットでのテストでも、デスクトップでも iOS と tvOS でも同じパッケージ選択およびコピー手順が使用されます。宣言はローカルの replace や vendoring されたパッケージを含め、ビルドのターゲット、タグ、および生成された Go オーバーレイに基づいて選択されます。Android、HarmonyOS、watchOS、Web、ミニプログラムでは、このファイルから何もパッケージ化されません。

## iOS と tvOS のフレームワーク

NSGo は各フレームワークのバイナリを検査し、以下の条件を満たさない場合は理由を示してビルドを停止します。

- `LC_BUILD_VERSION` が示すとおり、ターゲットのプラットフォーム（実機またはシミュレーター）向けにビルドされた arm64 の Mach-O 動的ライブラリであること。ユニバーサルファイルの場合は arm64 スライスが使われます。arm64e は拒否されます。
- 最小 OS がアプリのもの、つまり `apple.info.MinimumOSVersion`、未設定なら iOS 15.0 および tvOS 17.0 より新しくないこと（[ビルドと配信](building.md#nsgoyaml)）。
- インストール名が `@rpath/<Name>.framework/<Name>` であること。
- `@rpath` 経由で読み込むフレームワークが、アプリのパッケージが同じターゲットに宣言したものだけであること。

NSGo はフレームワークの `Info.plist` を書き込みます。その `CFBundleIdentifier` はアプリの ID、ドット、小文字の `Name` を連ねたもので、プラットフォームとアプリの最小 OS も記載されます。シミュレーター向けのビルドはフレームワークにアドホック署名を行います。実機向けのビルドは署名をアプリの署名手順に委ねます。`sign.profile` はアプリより先に、アプリと同じ ID でフレームワークに署名します。実機での `nsgo build -run` と `nsgo dev` では Xcode が同様に署名します。アプリの実行ファイルには実行パスがないため、関連フレームワークを読み込むフレームワークが `Frameworks` 内でそれらを見つけるには `@loader_path/..` または `@executable_path/Frameworks` の実行パスが必要です。また、`/usr/lib/swift` のようなシステムの実行パスがない限り、`@rpath` 経由でそれ以外のものを読み込むことはできません。バンドル ID が（大文字小文字を区別せず）アプリ内の別のバンドルと重なるフレームワーク、たとえば NSGo の `NSGoStoreKit.framework` と並ぶ `StoreKit.framework` は、ビルドを停止させます。

アプリが iOS のフレームワークを宣言するパッケージをリンクしている場合、`nsgo build -embed` はそのパッケージ名を示して停止します。ホストアプリ向けの XCFramework はまだそれらを同梱できません。

## Linux パッケージ

バージョン 2 からは、`linux` で Linux のアーキテクチャごとに、その Go パッケージをリンクするアプリの Debian パッケージと Arch パッケージが依存し推奨するシステムパッケージを指定します。

```yaml
version: 2
libraries:
  # ...
linux:
  linux_x64:
    depends:
      deb: [libva2, libva-drm2, libpulse0]
      arch: [libva, libpulse]
    recommends:
      deb: [va-driver-all]
      arch: ["intel-media-driver: hardware decoding on Intel GPUs"]
  linux_arm64:
    depends:
      deb: [libavcodec61, libavformat61, libpulse0]
      arch: [ffmpeg, libpulse]
```

- `linux` は省略可能です。キーは `linux_x64` と `linux_arm64` なので、一方のアーキテクチャではライブラリを同梱し、もう一方ではシステムのものを読み込むパッケージも、それぞれに必要なものを指定できます。
- `depends` と `recommends` はそれぞれ `deb` と `arch` のリストを取り、[nsgo.yaml](building.md#nsgoyaml) にあるアプリの `linux.depends` と `linux.recommends` と同じ形です。`deb` の各エントリは `libsoup-3.0-0 | libsoup3` や `libavcodec61 (>= 7:7.1)` のように dpkg の書き方で、`arch` の各エントリは `ffmpeg>=7` や `libpulse: audio output` のように pacman の書き方で記述します。
- `.deb` の `Depends` と `Recommends`、Arch パッケージの `depend` 行と `optdepend` 行には、NSGo 自身のパッケージ、アプリのもの、ビルドがリンクする各 Go パッケージのものの順に並びます。Go パッケージ同士は Go が列挙する順序に従い、すでに含まれているエントリと、`depends` のエントリがすでに挙げている Go パッケージの推奨は省かれます。ターゲットやビルドタグによって除外された Go パッケージは何も追加しません。
- 空のエントリや改行を含むエントリがあると、その Go パッケージ名を示してビルドが停止します。バージョン 1 のファイルにある `linux` や、これら 2 つ以外のキーも同様です。
- AppImage とストアビルドの Flatpak は `linux` から何も取り込みません。

## vendoring 時にファイルを保持する

Go の vendor 操作では、パッケージディレクトリ配下のファイルを保持するために埋め込みデータが必要です。パッケージは実行時にそのバイト列を使用することなく、マニフェストとライセンスを同梱対象としてマークできます。

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

`path` を使用する場合は、その embed ディレクティブにローカルライブラリのパス（たとえば `lib/*`）も含めてください。URL ダウンロードの場合は、Go パッケージ内にマニフェストとライセンスファイルのみがあれば十分です。
