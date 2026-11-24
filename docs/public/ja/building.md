---
title: ビルドと配信
description: あらゆるプラットフォーム向けにアプリをパッケージ化し、インストーラーの作成、署名と公証、GitHub での公開、他アプリへの組み込みを行う。
group: ツールチェーン
order: 22
---

## ビルド

```sh
go tool nsgo build
```

`build` は、`nsgo.yaml` に記載された各ターゲット向けにシンボルを strip したリリース用パッケージを `dist/<name>-<version>-<target>` に書き出します。出力物は `.app`、`.ipa`、`.apk`、Windows や Linux 向けのディレクトリ、または Web サイト（`import()` で指定された各 TSX モジュールをインポート実行時に読み込むページ）になります。`-target` フラグでターゲット一覧を上書きでき、`all` を指定すると Go と Xcode だけでビルドできるすべてのターゲットが対象になります。Android NDK がインストールされている場合は `android_x64` を、[ミニプログラム](#ミニプログラム)向けには `miniprogram` または `miniprogram_wechat`・`miniprogram_qq`・`miniprogram_alipay`・`miniprogram_douyin` を個別に、[HarmonyOS](#harmonyos) プレビュー向けには `harmonyos` を指定してください。`-run` を付けると、現在のマシン向けにビルドされたパッケージを起動するか、`-device` で選んだ iPhone、シミュレーター、または Android デバイスにインストールします。

配布用の成果物はすべて `dist/releases` にまとめられます。確認すべきディレクトリはここだけです。

| `dist/releases` 内のファイル | 内容 |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | macOS、Windows、Linux 用の[インストーラー](#インストーラー) |
| `*.AppImage.zsync` | AppImageUpdate および互換ランチャー向けの差分更新制御ファイル |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | ストアまたは実機配布向けの iOS、tvOS、Android パッケージ |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz`, `flatpak/` | `store` ビルドの[ストアパッケージ](#ストアパッケージ) |
| `install.sh`, `install.ps1` | [GitHub リリース](#github-での公開)からアプリをインストールするスクリプト |
| `appcast-<target>.xml` | アプリの[アップデート](#アップデート)が読み込むフィード |
| `homebrew/<name>.rb`, `winget/` | 直接配信用の [Homebrew cask および winget マニフェスト](#チャンネル) |
| `*-web.zip` | 展開してそのまま配信できる Web サイト |
| `miniprogram/<platform>/` | 各プラットフォームの開発者ツールからアップロードするミニプログラムプロジェクト |

各ファイルは `hello-1.2.0-windows-x64-setup.exe` のように `<name>-<version>-<system>-<arch>` の形式で命名されるため、ユーザーはリリースページで自分の環境に合ったファイルをすぐに見つけられ、隣には `.sha256` チェックサムも置かれます。別バージョンのビルドを実行するとこのディレクトリはいったん空になり、同じバージョンのビルドは別マシンで実行したものも含めて同じディレクトリに追加されます。シミュレーターや watchOS のパッケージはそれぞれの `dist/<name>-<version>-<target>` ディレクトリに留まります。

Web サイトには、元のモジュールより小さい場合に `.wasm.br` と `.wasm.gz` も含まれます。事前圧縮ファイルをサポートするホストでは有効化してください。元の `.wasm` URL に対して `Content-Type: application/wasm`、対応する `Content-Encoding`、および `Vary: Accept-Encoding` ヘッダー付きで配信します。元のモジュール自体は任意の静的ホストで動作します。セマンティック見出し、ブラウザーリンク、ページごとのメタデータ、CDN 読み込みについては[Web での公開](web.md)で解説しています。

### オフライン Web パッケージ

`nsgo.yaml` で `web.offline: true` を設定すると、初回のオンライン起動時に Web リリースをキャッシュします。

```yaml
web:
  offline: true
```

公開ディレクトリを HTTPS で配信するか、localhost で `go tool nsgo build -target web -run` を使用してください。Service Worker がページ、WebAssembly、スクリプト、およびパッケージ化されたリソースをキャッシュします。オンラインアクセス時は公開バージョンが読み込まれます。インストールが成功し、前のリリースのタブがすべて閉じられると、最新リリースをキャッシュから起動できるようになります。アプリが利用するオンラインサービスへのリクエストには引き続きネットワーク接続が必要です。

## ターミナル

`go tool nsgo dev -target tui` でインタラクティブなターミナル上でアプリを実行できます。`go tool nsgo test -target tui` は 1 テストプロセスにつき 1 アプリとして `nsgotest.Main` 経由でテストを実行します。サイズはターミナル側が決めるため、1 セルは 8 × 16 レイアウトポイントに相当します。アプリケーションのスタイルは明示的に指定してください。各コンポーネントは通常の形式とサイズを保ちます。`examples/tui` にフィールド、ボタン、ショートカット、リサイズの例があります。

コンソール実行ファイルをビルドするには、`go tool nsgo build -target tui_macos_arm64`、または対応する `tui_macos_x64`、`tui_windows_arm64`、`tui_windows_x64`、`tui_linux_arm64`、`tui_linux_x64` を指定します。`tui` ビルド／チェックグループにはこれら 6 つすべてが含まれ、`all` にも含まれます。各パッケージディレクトリには実行ファイルとリソースが収められ、GUI インストーラーは含まれません。6 つすべてが Go 単体でビルドできます。

このバックエンドは、セル単位のテキスト、背景色、実線ボーダー、クリッピング、スクロール、フォーカス、テキスト編集をサポートします。フォントと色はターミナルの設定を使用し、Mac を含め `mod` は Control になります。Ctrl+C はエディター内ではコピーとして動作し、キーバインドやエディターが処理しない場合は終了します。ブラケットペースト（bracketed paste）はショートカットを発火させずにテキストを挿入し、クリップボードはプロセス内に限定されます。キャンバス、画像、メディア、ネイティブホストコンポーネント、システムメニュー／アラート、アニメーション、ズーム、双方向シェイピング（bidirectional shaping）、先頭／中央の省略記号、システムクリップボードとの連携はまだ実装されていません。GUI アプリケーションに組み込むターミナルコントロールは引き続き `base/terminal` です。

`nsgo dev` がターミナルを占有している間、ツールおよびアプリケーションの診断ログは `.nsgo/tui.log` に出力されます。終了するとシェルに戻る前に画面、カーソル、ターミナルモードが復元されます。ターミナルのテストスクリーンショットはエミュレーターをキャプチャするのではなく固定のポータブルフォントでセルをラスタライズするため、その書体にない Unicode グリフについてはテキストアサーションを使用してください。

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

`targets` や `nsgo build -target`、`nsgo check -target` にアーキテクチャなしの OS 名を指定すると、その OS の各アーキテクチャを意味します（`windows` は `windows_arm64` と `windows_x64`、x86_64 ビルドに NDK が必要な `android` は `android_arm64` のみ）。`macos` と `all` には含まれない `macos_universal` は、両方の macOS アーキテクチャをビルドしてライブラリやウィジェットを含めた 1 つのアプリに結合します。Steam チャンネルはこのユニバーサルアプリバンドルを使用します。インストーラービルドはそれを 1 つのディスクイメージにまとめ、direct チャンネルでは 1 つの Sparkle フィードになります。`nsgo test -target` もテストを実行するアーキテクチャ名を受け取ります（デスクトップ OS は実行中マシンのアーキテクチャ、Android は `arm64`）。たとえば x64 Windows マシンで `windows_x64` を指定すると `windows` ターゲットが実行され、実行できないアーキテクチャを指定すると使用すべき名前を添えて拒否されます。

`-target`、`-version`、`-output` などのフラグは対応する設定値を上書きします。`-h` で全フラグを確認できます。

`build.ldflags` にはアプリ独自のリンカーフラグを `go build` の `-ldflags` と同じ形式で指定します（`-X main.releaseDate=2026-11-17` や `-X 'main.channel=beta builds'` など）。アプリをリンクするすべてのビルド（`dev` とそのリロード、`-embed` を含む `build`、`-target` を含む `test`）が全ターゲットでこれらを受け取ります。`build` や `test` に `-ldflags` を指定すると上書きではなく追加になり、複数回指定できます。これらはツール自身の `-X` フラグより前に渡され、リンカーは変数ごとに最後の指定を採用するため、`core.App` 内の名前・ID・バージョンは `nsgo.yaml` の値が維持されます。ツールは Go に独自の `-ldflags` を渡すため `GOFLAGS` 内の指定は上書きされるので、リンカーフラグはここへ記載してください。ディレクトリ指定の後ろに付けるヘッドレス `test` の `-ldflags` は `go test` 側のフラグであり、すべてを置き換えます。

`build.tags` は、ツールがアプリに対して実行するすべての `go` コマンド（`dev` とリロード、`-embed` を含む `build`、`-target` を含む `test`、`check`）で全ターゲットに適用されます。`dev` での `nsgodev` やターゲットテストでの `nsgotest` など NSGo 自身のタグも追加され、通常は上書きされてしまう `GOFLAGS` の `-tags` も引き継がれます。`GOFLAGS` のその他の指定はそのまま適用されます。

`apple.info.LSMinimumSystemVersion` は macOS パッケージの最小 OS バージョンを引き上げ、`apple.info.MinimumOSVersion` は選択した iOS、tvOS、または watchOS パッケージの最小 OS バージョンを引き上げます。`"14.2"` や `"18.0"` のように 1〜3 個の数値パートからなるクォートされたバージョン文字列で指定してください。ランタイムの下限は macOS 13.0（Go 1.27）、iOS 15.0、tvOS 17.0、watchOS 26.0 であり、これより低い値や不正な形式はエラーになります。モバイル用キーは選択したすべてのモバイルターゲットの下限を満たす必要があります。App Clips は独自の `app_clip.minimum` を持ちます。これらの値はパッケージメタデータ、アセットコンパイル、署名プロジェクト、アプリのリンカーに渡されます。組み込み iOS フレームワークでも使用され、Mac Catalyst では Go の macOS 13 ランタイムのために最低 16.0 となります。

`icon.windows` と `icon.linux` には、各アプリパッケージ、インストーラー用画像、ストアロゴに使用する正方形 PNG を指定します。デフォルトは `assets/IconWindows.png` と `assets/IconLinux.png` で、存在しない場合は `icon.png` にフォールバックします。`dev.icon` も同じ設定と `-dev` サフィックス付きのデフォルトファイルを受け取ります。`icon.android.png` はアダプティブレイヤーを設定していない場合も含め、Android 用ビットマップを独立して指定します。その他のターゲットでは共通の `icon.png` フォールバックが使われます。

Apple TV のアイコンは正方形ではなくレイヤー構造のカードです。`tvos` には Xcode の `.brandassets`（`App Icon` スタックとトップシェルフ画像を含む）または 1280×768 などの横長画像を指定でき、後者の場合はエッジ色で塗りつぶしたカードに合わせて描画し、トップシェルフ画像の中央に配置します。どちらも指定しない場合は、PNG がカードの高さの 4 分の 3 のサイズで配置されます。Apple Watch はアイコンを円形にマスクするため、`watchos` には watchOS の円向けに描かれた `.icon` または正方形 PNG を指定します。指定しない場合は `apple`、次に `png` が使われます。どちらも `apple` や `png` と同様に `assets/AppIconTV.*` と `assets/AppIconWatch.*` にフォールバックします。

Android では、`icon.android.foreground` と `background` によってランチャーアイコンがアダプティブになり、ランチャーが `png` を独自のプレート上に縮小配置するのではなく、任意の形状にマスクして動かせるようになります。各レイヤーは 108 dp 四方（432 px にスケーリングされた PNG）です。ランチャーは中央の 72 dp（288 px）を表示し、中央 66 dp（264 px）の外側を切り取る可能性のある形状でマスクするため、ロゴはその円内に収め、背景は正方形全体を満たすようにしてください。`background` には PNG または `"#1F1F1F"` のような色を指定します。`monochrome` はアルファ値のみが意味を持つ形状で、ユーザーがテーマアイコンを有効にした際に Android 13 以降が着色します。指定しない場合、アイコンはその場でも元の色を保ちます。パッケージには通常のビットマップアイコンとして `icon.android.png`（なければ `icon.png`）も保持されますが、NSGo が動作する最小バージョンの Android 9 以降のランチャーではアダプティブアイコンより優先されることはありません。`png` がない場合は、ツールが背景の上にフォアグラウンドを描画して生成します。アダプティブレイヤーがない場合、ランチャーは Android ビットマップまたは共有の `png` を表示します。

`android.uses_cleartext_traffic: true` を指定すると、APK と App Bundle で任意のホストへの平文 HTTP 通信を許可し、マニフェストフラグとネットワークセキュリティポリシーの両方を設定します。このオプトインがない場合でも、`android.loopback_cleartext: true` を設定すると、アプリ自身が配信する動画などの用途向けに `localhost`、`127.0.0.1`、`::1` からの平文 HTTP のみを許可します。ループバック限定ポリシーは他のホストへの平文通信を拒否し、両方が true の場合は全ホスト許可の設定が優先されます。組み込み AAR の場合は、ホストアプリの Android ネットワークセキュリティポリシーを設定してください。

すべてのビルドでは起動時に `--nsgo-fps` など NSGo 自身のフラグも受け取ります（[実行とデバッグ](debugging.md#すべてのビルドで使えるフラグ)）。`nsgo dev` では、開いているどのウィンドウからでも Command/Control+R でリロードでき、別のウィンドウでアプリ関数が動き続けている限りメインビューを閉じた後も利用できます。

### 起動画面

iOS および Android 12 以降で不透明な背景、中央配置の PNG、またはその両方を表示するには `launch_screen` を設定します。画像パスは `nsgo.yaml` からの相対パスです。画像は 128 × 128 ポイントまたは dp の範囲内に縦横比を保って収められます。長辺が 512 ピクセル以上の透過 PNG にすると高密度画面でも鮮明に表示されます。

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

`appearance` で固定していない限り、システムが外観を選択します。`dark` 側のいずれかのフィールドを省略すると通常側の値が再利用されます。背景や画像を省略するとプラットフォームのデフォルトが維持されます。組み込みアプリはホスト側から起動画面を取得します。

## Android ネイティブクラス

`core.Native` からカスタム View を使用するには、アプリの `nsgo.yaml` にコンパイル済み Android クラスを列挙します。

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

パスは絶対パスでない限り `nsgo.yaml` からの相対パスです。各パスには `.dex` ファイル、または D8 が出力したルートの `classes.dex`、`classes2.dex` 以降のシャードを含む JAR を指定します。これらのファイルは D8 で `--min-api 28` を指定して準備してください。arm64 向けには、NSGo がこれらのコンパイル済みファイルから Go 単体で APK または App Bundle をビルドします。Android の開発セッションでは、リストされたファイルが変更されると再ビルドが行われます。組み込み AAR の場合は、ホストの Android ビルド側で Java 依存関係を提供します。[D8 ドキュメント](https://developer.android.com/tools/d8)

[Android ネイティブサンプル](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native)では、パブリックな `Context` コンストラクターを持つクラスを登録し、Go の prop で設定する例を示しています。

## ゲーム

ゲームの場合は `nsgo.yaml` で `game: true` を設定してください。Android はアプリをゲームとして分類してシステムの Game Mode 最適化を使用し、macOS と iOS は Game Mode にオプトインします。また macOS はデフォルトで Games カテゴリーになり、Linux はデスクトップエントリに `Game` を追加します。明示的な `macos.category` や `apple.info` 内の Game Mode キーが指定されている場合はそちらが優先されます。

アプリがハードウェアゲームパッドに対応する場合は `gamepad: true` を設定してください。Android はゲームパッドとタッチスクリーンの両方を任意機能として宣言します。macOS、iOS、tvOS は Extended Gamepad プロファイルを宣言し、tvOS はさらに Siri Remote の Micro Gamepad も宣言します（`apple.info` 内のコントローラーキーが優先されます）。

## ライセンス表記

`nsgo build` はスタンドアロンパッケージのリソースに `THIRD_PARTY_NOTICES.txt` を出力します。これには、そのターゲットで使用されている Go モジュール、Go ランタイム、および NSGo にバンドルされた Java または JavaScript ランタイムのライセンステキストが含まれます。Web ビルドでは他のリソースと同様に内容ハッシュでファイル名が付けられます。アプリ内のクレジット画面に同じテキストを表示するには `core.ThirdPartyNotices()` を使用してください。この関数は開発時、テスト時、組み込みライブラリのビルド時には空文字列を返します。[ネイティブライブラリ宣言](native-libraries.md)を使うと、その元のライセンステキストも同じ notices に追加されます。

デフォルトでは、不明または欠落したサードパーティライセンスがあるとビルドが失敗し、コピーレフトライセンスが検出されると警告が出ます。アプリの Go ワークスペース（`go.work`）に記載されたモジュールはアプリ自身のモジュールと並ぶファーストパーティコードとして扱われ、含まれるライセンスファイルは収録されますがサードパーティチェックの対象にはなりません。いずれのルールも `error` または `warn` に設定できます。`allow` リストは完全一致するモジュールパスを両方のルールから除外し、ビルドが見つけたライセンステキストは保持します。

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Go パッケージ由来のライブラリ

`base/terminal` をインポートするアプリには libghostty-vt も同梱されます。Windows と Linux パッケージでは実行ファイルの隣に配置され、macOS と iOS バンドルではフレームワーク内に署名されます。iOS ホスト向けのコンパニオン XCFramework については[組み込み](embedding.md)で説明しています。

依存パッケージは `nsgo-natives.yaml` でビルド済みデスクトップ共有ライブラリを宣言できます。NSGo はその SHA-256 を検証し、署名前にアプリへコピーしてインストーラーにも含めるため、アプリ作者が追加の手順を踏む必要はありません。パッケージ作者向けのフォーマットと読み込みパスについては[ネイティブライブラリ](native-libraries.md)で説明しています。

## コマンド

アプリのデータに対して動作する CLI など、デスクトップ実行ファイルの隣にコマンドラインプログラムを同梱できます。それぞれ Go の main パッケージからビルドし、`commands` の下に列挙します。

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

コマンド名には英数字、`-`、`_` が使え、アプリの実行ファイル名、Linux コマンド名、`Uninstall`、`AppRun`、および他のコマンド名（大文字・小文字を問わず）と重複してはなりません。`macos_universal` はアプリと同様に各コマンドの 2 つの macOS ビルドも結合します。

パッケージは `build.ldflags` と `-ldflags`、次に独自の `ldflags`（その `-X` の値が優先）、最後にアプリのアイデンティティ情報でリンクされるため、`core.App()` を呼ぶとアプリと同様に `nsgo.yaml` の名前・ID・バージョン・ビルド番号を取得できます。`core` をインポートしないコマンドは `build` の `-ldflags "-X main.version=1.2.0"` のように `-X` でバージョンを受け取り、これはアプリとコマンドの双方に適用されます。

| パッケージ | コマンドの扱い |
| --- | --- |
| macOS | `Contents/MacOS` に配置され、アプリ本体より前に `<id>.<command>` としてそのまま署名されます（アドホック署名、または `sign.identity`・Hardened Runtime・タイムスタンプ付きで署名されるため[公証](#公証notarize)を通過できますが、アプリのエンタイトルメントは付きません）。ディスクイメージ自体は `PATH` に何も追加しません。[Homebrew cask](#チャンネル) が各コマンドをリンクし、ユーザー自身で `/Applications/<name>.app/Contents/MacOS/<command>` をリンクすることもできます |
| Windows | `<name>.exe` の隣に `<command>.exe` として配置され、本体と同様に[署名](#windows-パッケージの署名)され、インストーラーが導入しアンインストーラーが削除します。`windows.path` を設定すると、インストーラーは `windows.command_line` と同様にユーザーの `Path` にもフォルダーを追加するため、インストール後に開いたコンソールからコマンドを実行できます。アンインストーラーはそのバージョンがインストールしたものだけを削除するため、アップデートでリネームや削除されたコマンドは（アプリが配布しなくなった他ファイルと同様に）フォルダーに残ります |
| Linux | `/opt/<slug>` に配置され、`.deb` と Arch パッケージが所有する `/usr/bin/<command>` リンクが張られるため、パッケージ削除時に同時に削除され、dpkg と pacman は他パッケージのファイルへの上書きインストールを拒否します。AppImage はアプリ本体と同じ場所に保持し、`PATH` には追加しません |

インストーラー、チェックサム、および[アップデート](#アップデート)の appcast にはアプリの他のファイルと同様にコマンドも含まれるため、アップデート時にはアプリと一緒に置き換わります。[ストアパッケージ](#ストアパッケージ)では Mac App Store のサンドボックス、MSIX、Flatpak がアプリ自身の実行ファイルしか実行できないためコマンドは除外されます。

## インストーラー

デスクトップターゲット向けに、`build` はユーザーがアプリをインストールするためのファイルを `.sha256` チェックサム付きで `dist/releases` に出力します。

| ターゲット | インストーラー |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`。アプリ、矢印、ドラッグ先の Applications へのリンクを表示するディスクイメージで、`sign.identity` で署名され[公証](#公証notarize)されます |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`。ユーザー単位でインストールするネイティブ Windows インストーラーで、以前ユーザーが選択したフォルダーに再インストールし、起動中のアプリを終了し、スタートメニューショートカットと任意のデスクトップショートカットを作成し、自身がインストールしたものだけを削除するアンインストーラーを備えます（アプリで Windows パッケージに署名する場合は [`sign.certificate`](#windows-パッケージの署名) で署名）。`windows.command_line` を有効にすると `.com` コンパニオンもインストールしてアプリのフォルダーをユーザーの `Path` に追加するため、インストール後に開いたコンソールからアプリのコマンドを実行でき、コンパニオンを見つける Win+R でもアプリを起動して開いたコンソールを閉じます。アンインストーラーはインストーラー自身が追加したエントリのみ `Path` から削除し、ユーザーが元々持っていたエントリには触れません |
| Linux | Debian/Ubuntu 向けの `<name>-<version>-linux-<arch>.deb` と Arch Linux 向けの `<name>-<version>-linux-<arch>.pkg.tar.gz` は、アプリを `/opt/<slug>` にインストールし、コマンド本体、各[コマンド](#コマンド)へのリンク、デスクトップエントリ、アイコンを配置します（Arch パッケージには `pacman -Qkk` 用の `.MTREE` が含まれます）。また `<name>-<version>-linux-<arch>.AppImage` はインストール不要でどのディストリビューションでも実行できます |

Windows のセットアップとアンインストーラーはどのシステム上でも Go だけでビルドでき、Windows ネイティブのダイアログを使います。アプリのカタログにある言語のうち訳語を持つ言語（簡体字中国語・繁体字中国語を含む）で表示され、システムの UI 言語を選びます。`/LANG=<LCID>` を指定するとその実行に限って言語を選べます（簡体字中国語なら `/LANG=2052`）。`/S` はサイレントでインストールまたはアンインストールし、末尾の `/D=<folder>` はインストール先フォルダーを指定し（スペースを含んでも可）、`/RELAUNCH` はアップデート後にアプリを起動します。`/D` を指定しない場合、アップグレードは置き換える既存インストールのフォルダーにインストールします。`-installers=false` または `build.installers: false` を指定するとパッケージのみをビルドします。

インストーラーはアプリ独自の画像やアイコンも使用でき、指定しない場合はアプリのアイコンまたは組み込み画像が使われます。

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

背景 PNG に `DMGBackground@2x.png` のような 2 倍サイズのペアを用意すると Retina 画面でも鮮明に表示されます。アプリのアイコンは左から 4 分の 1、Applications は右から 4 分の 1 の位置に配置され、それぞれ名前は組み込み画像が矢印を描く高さの中央に揃えられるため、独自の背景画像を用意する際はその余白を空けてください。Linux パッケージには独自の画像はなく、アプリのアイコンをインストールします。

Linux パッケージはソフトウェアセンターがアプリを掲載するための AppStream メタデータ `/usr/share/metainfo/<id>.metainfo.xml` もインストールします。これは `linux.summary`、`linux.license`（設定時）、およびアプリ名から生成され、`linux.categories` に指定がない場合はカテゴリーが `Utility` になります。AppImage ではこれを AppDir の `usr/share` 内に保持し、メタデータが参照する `usr/share/applications` 内のデスクトップエントリへのリンクも並べます。より詳細なメタデータ、追加のデスクトップエントリ、シェル補完、マニュアルページなど、システムが `/usr/share` から読み込むファイルは `linux.share` で指定したディレクトリ（`nsgo.yaml` からの相対パス）に配置してください。`.deb` と Arch パッケージはそのツリーを `/usr/share` 配下に、AppImage は AppDir の `usr/share` 配下にインストールします。その中の `metainfo/<id>.metainfo.xml` は NSGo が生成するものを置き換え、アプリのデスクトップエントリやアイコンなどパッケージ自身が書き込むパスと衝突するファイルがあるとビルドは失敗します。`resources/` 内のファイルはどのような名前であっても `/opt/<slug>` 内の実行ファイルの隣に置かれ、ストアビルドの Flatpak では NSGo のメタデータのみが使われます。

AppImage はほとんどのディストリビューションに備わっている FUSE を必要とし、パッケージ版と同様にシステムの GTK 4 を利用します。ランタイムは初回時にユーザーキャッシュへダウンロードされます。

AppImage には、`release.github` または Git リモート `origin` を使って最新の安定リリースとこのアプリのアーキテクチャを指す `gh-releases-zsync` ロケーターが埋め込まれます。各 `.AppImage` は生成された `.AppImage.zsync` ファイルと一緒にアップロードしてください。別のホストを使う場合は、`linux.appimage_update` を HTTP Range リクエスト対応の固定 URL を使った `zsync|https://example.com/latest.AppImage.zsync` に設定します。プレリリースチャンネル向けに `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` も受け付けます。明示的に空文字列を設定すると埋め込みディスカバリーが無効になり、GitHub リポジトリやロケーターの設定がない場合も空になります。`.zsync` ファイルは署名済み appcast とは別に引き続き生成されます。

## ストアパッケージ

`-channel store` または `updates.channel: store` を指定すると、インストーラーやインストールスクリプトの代わりに、アップデートをストア側が担うデスクトップおよび Android 向けパッケージをビルドします。

| ターゲット | ストアパッケージ |
| --- | --- |
| macOS | Mac App Store 向けの `<name>-<version>-macos-<arch>.pkg`。App Sandbox 内で動作し、プロビジョニングプロファイルのアイデンティティで署名され、チームのインストーラーアイデンティティでパッケージ化されます |
| Windows | Microsoft Store 向けの `<name>-<version>-windows-<arch>.msix`。公開時にストア側で署名されます |
| Linux | インストール可能な `<name>-<version>-linux-<arch>.flatpak` に加え、GNOME ランタイム上で Flathub または flatpak-builder 向けに使う AppStream 同梱 `.tar.gz` と `flatpak/<id>.yml` |
| Android | Google Play 向けの App Bundle `<name>-<version>-android-<arch>.aab`。アップロードキーである `sign.keystore` で署名されます |

MSIX は各言語の表示名をカタログの `appName` から読み取り、ない場合はソース言語にフォールバックします。スタートメニュー、タスクバー、ストア用ロゴには 100%、125%、150%、200%、400% のバリアントが含まれます。ツールはリソースインデックスを macOS 上で書き出すため、Windows SDK は不要です。

ストアチャンネルで `-target macos` を指定すると両方の macOS アーキテクチャが選択され、`-target macos_universal` と同様に 1 つの `<name>-<version>-macos-universal.pkg` が出力されます。`arm64-v8a` と `x86_64` の両方を含む 1 つの `<name>-<version>-android-universal.aab` を作るには `-target android_arm64,android_x64` を選択してください。x86_64 スライスには Android NDK が必要です。

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

`android.target_sdk` が未設定の場合、ランタイムは API 36 をターゲットにします。Android App Bundle ビルドは、2026 年 8 月 31 日以降に新規モバイルアプリおよびアップデートで [Google Play が必須とする](https://support.google.com/googleplay/android-developer/answer/11926878) API 36 未満では失敗します。スタンドアロン APK ビルドは警告を出して続行します。

アップロード用に Mac パッケージへ署名するには `sign.macos_profile` を設定します。ウィジェットがない場合、未署名のパッケージでもサンドボックス内で実行できるため、アプリコンテナ・ネットワーク・ユーザー選択ファイルへのアクセスをテストできます。ウィジェット付きのストアビルドでは、同じチームかつ `apple.app_group`（デフォルトは `group.<id>`）を付与した、`<id>` 用の `sign.macos_profile` と `<id>.widgets` 用の macOS `sign.widgets_profile` の両方が必要です。両バンドルはその共有コンテナを使用し、ホームディレクトリの例外は追加されません。`-installers=false` を指定すると `.pkg` とインストーラーアイデンティティを省略しつつ、サンドボックスとプロファイル署名は保持します。`.msix` はパートナーセンターへ、`.aab` は Play Console へアップロードしてください。リリースに指定のアーカイブが揃ったら、`flatpak/<id>.yml` を Flathub に提出します。ローカルでは `flatpak-builder` でもビルドできます。バンドルのインストールは `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak` で行います（対応する GNOME ランタイムがインストール済みであるか、設定済みリモートから取得できる必要があります）。ビルドは Linux ツールチェーンなしで Go 上でバンドルを生成します。ストアパッケージは、アートワークがない場合は透過アイコンを、`linux.categories` が未設定の場合は Utility を使用します。ストア外でのインストール向けに MSIX へ署名するには [`sign.certificate`](#windows-パッケージの署名) を設定します。証明書のサブジェクトは `windows.store.publisher` と一致する必要があり、インストール先のマシンがその証明書を信頼している必要があります。署名設定がない場合、アップロードされたパッケージはストア側が署名します。

macOS と iOS では、スクロールビューのエッジエフェクト（`Scroll.EdgeEffects`）、`IOS.BackdropMaterial` を指定しない macOS の背景ブラー、および背景カラーフィルター（単独、または `IOS.BackdropMaterial` なしのブラーとの併用）は、半径指定のブラー、ビュー内でのブラー変化、背景カラーマトリクス適用を行う公開 API が存在しないため、AppKit や UIKit 自身のマテリアルやエッジエフェクトと同じく Core Animation のプライベート API である `CABackdropLayer` と `CAFilter` を通じて描画します。App Store 審査はプライベート API を使用するアプリをリジェクトする場合があります。NSGo がそれらを呼び出すのはエッジバー付きスクロールビューと前述の背景フィルターを使用するときだけですが、macOS および iOS のすべてのビルドにそのシンボル名は含まれます。

`build` が出力するすべての Apple バンドル（アプリ、ウィジェット拡張、App Clip、Apple Watch アプリ、および `-dynamic` `-embed` フレームワーク）にはプライバシーマニフェスト `PrivacyInfo.xcprivacy` が含まれ、NSGo と Go ランタイムが使用する必須理由 API（UserDefaults: `CA92.1`、ファイルタイムスタンプ: `C617.1`、システム起動時刻: `35F9.1`）が宣言されます。`apple.privacy` ではアプリ自身の申告を追加できます（`accessed_apis` と各カテゴリーの理由、`tracking`、`tracking_domains`、Apple の名称に基づく `NSPrivacyCollectedDataTypes` のエントリである `collected_data`）。`resources/` に置かれた `PrivacyInfo.xcprivacy` は置き換えられるのではなくこれらとマージされます。

実機向けに `build` が書き出す各 iOS・tvOS・watchOS バンドル（アプリ、ウィジェット拡張、App Clip）の `Info.plist` には、App Store Connect が確認する SDK および Xcode の `DT` キー（ビルドに使用した Xcode から取得）と `UIRequiredDeviceCapabilities: [arm64]` が含まれます。シミュレーターバンドルには含まれず、`apple.info` で設定したキーはそちらの値が維持されます。`apple.multiple_scenes: true` は iOS アプリのシーンマニフェストで `UIApplicationSupportsMultipleScenes`（デフォルトは false）を有効にします。これにより iPad でユーザーがアプリの複数ウィンドウを開けるようになり、各ウィンドウはユーザーが閉じるまでアプリ共有のシグナルとストレージを使いつつ独立した状態を持ってメインビューを新しく表示します。

### Google Play への公開

Google Play Developer API を有効にし、選択したトラックへアプリをリリースする権限をサービスアカウントに付与します。`NSGO_PLAY_SERVICE_ACCOUNT` にサービスアカウントの JSON キー本体またはキーファイルのパスを設定してください。[Google Play API の設定](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

このコマンドは `build.output/releases` からプロジェクトバージョンに一致する App Bundle を探します。複数ビルドがある場合はフラグの後に `.aab` パスを渡して選択できます。トラックには `internal`、`production`、またはクローズドテストのトラック ID を指定します。コマンドはバンドルからパッケージ名とバージョンコードを読み取り、そのトラックの既存リリースより大きいバージョンコードであることを検証した上で、バンドルをアップロードしてリリースを確定します。

## ストア掲載情報

ストアのメタデータ（タイトル、サブタイトル、説明、キーワード、リリースノート、URL、スクリーンショット、プレビュー）は、`store/`（または `nsgo.yaml` の `store.dir`）配下にプレーンテキストの YAML と Markdown としてリポジトリに保存します。

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

どちらの `listing.yaml` ファイルにも `version: 1` が必須であり、未知のキーは拒否されます。`zh-Hans` などの正規ロケールディレクトリは、`app_store.locale` または `google_play.locale` でストアごとに上書きしない限り、各ストアのコード（App Store Connect では `zh-Hans`、Google Play では `zh-CN`）へ自動的にマッピングされます。

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

検証は認証情報の読み取りや HTTP リクエストの前に実行され、必須フィールド、ロケールコード、URL スキーム、メディアファイルと枚数（App Store のディスプレイタイプごとにスクリーンショット最大 10 枚・プレビュー最大 3 本、Google Play の画像タイプごとにスクリーンショット最大 8 枚）、文字数制限（App Store: タイトル 30、サブタイトル 30、キーワード 100、プロモーションテキスト 170、説明 4000、リリースノート 4000。Google Play: タイトル 30、簡単な説明 80、詳しい説明 4000、リリースノート 500）をチェックします。

アップロード用認証情報は環境変数のみから取得します。App Store Connect には `NSGO_APP_STORE_KEY`、`NSGO_APP_STORE_KEY_ID`、`NSGO_APP_STORE_ISSUER`（未設定時は公証キーの `NSGO_NOTARY_KEY`、`NSGO_NOTARY_KEY_ID`、`NSGO_NOTARY_ISSUER` にフォールバック）を、Google Play には `NSGO_PLAY_SERVICE_ACCOUNT`（サービスアカウント JSON ファイルパスまたは JSON 文字列）を使用します。

### ストアコンプライアンス

各リリース向けアンケートの完了状況を `nsgo.yaml` で管理できます。

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

アンケートを完了したら対応するエントリを `true` に変更してください。該当する `-channel store` または `-channel steam` のビルド時に、`false` のままのエントリについてリマインダーが表示されます。YAML で管理しないエントリは省略できます。

- App Store Connect で[年齢制限アンケート](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating)と[アプリのプライバシー詳細](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy)を完了してください。
- Apple の[暗号化に関するアンケート](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation)については、`apple.info.ITSAppUsesNonExemptEncryption` にアプリの回答を YAML の真偽値として設定します。Apple から承認コードが発行された場合は、`apple.info.ITSEncryptionExportComplianceCode` に文字列として設定してください。これらのキーを省略してアップロード時に回答することもできます。Apple ストアビルドではこれらのキーの型とバンドルのプライバシーマニフェストの構造がチェックされます。
- Play Console で[データセーフティ](https://support.google.com/googleplay/android-developer/answer/10787469)を完了してください。内部テストトラックのみで配布されるアプリは免除されます。
- ゲームを審査に提出する前に Steamworks で[コンテンツアンケート](https://partner.steamgames.com/doc/gettingstarted/contentsurvey)を完了してください。

## App Store Connect

App Store Connect でアプリを作成し、iOS 実機用 IPA には App Store 配布プロファイルで、macOS ストア PKG にはチームのインストーラーアイデンティティで署名します。Xcode を備えた Mac 上で、`NSGO_APP_STORE_KEY` に App Store Connect チームキーの `.p8` ファイルまたはテキスト、`NSGO_APP_STORE_KEY_ID` にキー ID、`NSGO_APP_STORE_ISSUER` に Issuer ID を設定します。これらが未設定の場合は、ストア掲載情報と同様に公証キー（`NSGO_NOTARY_KEY`、`NSGO_NOTARY_KEY_ID`、`NSGO_NOTARY_ISSUER`）が使用されます。そのキーには対象アプリへのアクセス権とビルドアップロード権限を付与しておいてください。

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

このコマンドはアップロード前にバンドルのビルド番号を確認し、TestFlight に対する Apple の処理結果を報告します。次回のアップロード時には `build_number` を増やしてください。macOS のビルド番号はすべてのアプリバージョンを通じて単調増加する必要がありますが、iOS では新バージョンごとに番号をリセットできます。処理済みのビルドを App Review へ提出する操作は App Store Connect 上で別途行います。[Apple の配布ガイド](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## ストアアセットパック

大規模なアセットディレクトリを `mobile.asset_packs` で宣言します。それらのディレクトリ内のファイルは、ベースの Android App Bundle および iOS アプリとは別に配信されます。パック名は小文字英字で始まり、小文字英字・数字・アンダースコアを含められます。

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Android パックは `go tool nsgo build -channel store -target android_arm64` でビルドします。実機でバンドルをテストするには `bundletool build-apks --local-testing` と `bundletool install-apks` を使用してください。NSGo はインストール済みの `install-time` パックを開けます。`fast-follow` モジュールは Play 配信用に含められますが、Play Asset Delivery SDK からそれらのパックを読み取る機能はまだ対応待ちです。

iOS では、`go tool nsgo build -target ios` が `install-time` を Apple の essential ポリシーに、`fast-follow` を prefetch にマッピングします（初回インストール時およびアップデート時の両方）。Xcode 26 以降で Background Assets ダウンローダーがビルドされます。共通の `apple.app_group` またはデフォルトの `group.<id>` を使用してください。アプリの `sign.profile` と `<id>.assetdownloader` 拡張の `sign.asset_packs_profile` の両方にそのグループが必要です。App Store Connect のアセットパックアップロードワークフローを使って、`dist/releases` の `.aar` ファイルを IPA とともにアップロードしてください。[マネージドアセットパックの作成](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

ゴルーチンから `mobile.AssetPackReady(ctx, "levels")` を呼んで準備状況を確認し、`mobile.OpenAssetPackFile("levels", "intro.map")` で宣言ディレクトリからの相対パス指定でファイルを開きます。使用後は返されたリーダーを閉じてください。[Apple アセットパックのローカルテスト](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Steam デポ

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

デフォルトでこのチャンネルを使うには `updates.channel: steam` を設定します。macOS、Windows、Linux ターゲットを選択してください。各 `dist/<name>-<version>-<target>` ディレクトリを対応する Steam デポにアップロードし、起動オプションには macOS ではアプリバンドル、Windows と Linux arm64 では実行ファイル、Linux x64 では `steam-launch.sh` を指定します。アップデートは Steam が処理します。これらのディレクトリにはインストーラー、自己アップデーター、`steam_appid.txt` は含まれません。

Linux x64 デポには Steam Linux Runtime 3.0 および 4.0 向けの GTK 4 とその依存ライブラリが含まれます。ランチャーはシステムに GTK 4.14 以降がない場合にのみこれらを使用します。`nsgo-gtk/` はランチャーの隣に置いたままにしてください。その中の `licenses/` にはライブラリのライセンス、ビルド元ソース、ビルドスクリプトが収められており、GLib、Pango、GTK の LGPL 要件を満たすために必要です。Linux arm64 ではランタイム環境側に GTK 4 が利用可能である必要があります。

Steam クライアント外でローカルテストを行う場合は、`nsgo.yaml` に `steam: {app_id: 480}` を設定し、480 を自分の Steamworks App ID に置き換えてください。`go tool nsgo dev` はネイティブビルドディレクトリに `steam_appid.txt` を書き出し、そのディレクトリを作業ディレクトリとしてアプリを起動します。[Steamworks API](https://partner.steamgames.com/doc/api/steam_api)、[SteamPipe](https://partner.steamgames.com/doc/sdk/uploading)。

## GitHub での公開

ビルドに macOS、Windows、Linux ターゲットが含まれる場合、`dist/releases` には `install.sh` と `install.ps1` も出力されます。これらは `release.github` で指定された GitHub リポジトリ（未指定時はアプリの Git リモート `origin`）のリリースからアプリをインストールします。どちらも存在しない場合、`build` はこれらを出力せず、その旨を表示します。`miniprogram/` を除くトップレベルのファイルを `v<version>` タグのリリースにアップロードしてください（`release.tag_prefix` で `v` を変更でき、スクリプトはそのプレフィックス内の URL 予約文字をエンコードします）。

```sh
gh release create v1.2.0 dist/releases/*.*
```

ユーザーはビルド時に出力される次の 1 行のコマンドでアプリをインストールまたはアップデートできます。

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

macOS ではスクリプトがディスクイメージから `/Applications`（書き込めない場合は `~/Applications`）へアプリをコピーします。Linux では `sudo` 経由で `pacman` により Arch パッケージを、なければ `apt-get` または `dpkg` により Debian パッケージをインストールします。Windows ではインストーラーをサイレント実行し、既存のアプリがある場所へインストールします。いずれも事前にチェックサムでファイルを検証します。`HELLO_VERSION=1.1.0` のように大文字のアプリ名を使った `<NAME>_VERSION` で別バージョンをインストールでき、`<NAME>_DOWNLOAD_URL` でミラーからダウンロードできます。

## itch.io での公開

`nsgo.yaml` で `itch.project` に itch.io ゲームの `owner/game` スラッグを設定します。チャンネル名を上書きするには、`itch.channels` をビルドターゲットごとに指定します。デフォルトは Web が `html5`、macOS が `osx-<arch>`、Windows が `windows-<arch>`、Linux が `linux-<arch>` です。

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

CI のシークレットとして `BUTLER_API_KEY` を設定した状態で、ターゲットをビルドしてアップロードします。

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

`-output` や `-version` を上書きする場合は両方のコマンドで同じ値を指定してください。アップロードは各ターゲットのパッケージディレクトリを対象とし、itch.io のバージョンをビルドのバージョンに設定します。NSGo は `PATH` 上の `butler` を使用するか、バージョン固定のコピーを itch.io からダウンロードします。最初の Web アップロード後、itch.io のゲーム種別を HTML に設定し、アップロードをブラウザーでプレイ可能としてマークしてください。[Butler ドキュメント](https://itch.io/docs/butler/pushing.html)

## アップデート

`updates` はプラットフォームおよびユーザーがアプリを入手したチャンネルに応じた方法でアプリ自身を最新に保ちます。

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` はファイルを書き出さずに鍵ペアを標準出力へ出力します。公開鍵は `updates.key` に設定してコミットし、秘密鍵はリポジトリに含めずパスワードマネージャーとリリースビルド環境の `NSGO_UPDATES_KEY`（GitHub Actions シークレットなど）に保管してください。各 direct ビルドは秘密鍵でインストーラーに署名し、その隣に [Sparkle](https://sparkle-project.org) 形式のフィード `appcast-<target>.xml` を書き出します（アプリはこれを `feed` 配下から読み取ります）。appcast はインストーラーと一緒にアップロードしてください。秘密鍵がない場合、macOS と Windows は署名済みインストーラーしかインストールしないため `build` は macOS・Windows 用の appcast を出力せず、Linux と Android アプリは未署名の appcast を読むとパッケージをインストールする代わりにリリースページを開きます。

`-release-notes notes.md` を指定すると、チェンジログの該当セクションなどのリリースノート用 Markdown ファイルが各 appcast に含められます（ファイル内の生 HTML は除いた上で HTML としてレンダリングされます）。Sparkle と WinSparkle はアップデートダイアログ内にこれを表示し、Linux と Android のプロンプトではバージョン名の下に最大 1,500 文字のテキストとして表示します。アプリ側では `core.UpdateCheck.Notes` として読み取れます。

| ターゲット | direct ビルドのアップデート方法 |
| --- | --- |
| macOS | `build` がアプリに組み込む Sparkle が毎日確認し、新バージョンを提案し、ディスクイメージをダウンロードして署名を検証し、インストールしてアプリを再起動します。デフォルトのアプリメニューには「アップデートを確認…」が含まれ、独自メニューを設定するアプリは `updater.Check` で追加できます |
| Windows | 実行ファイルの隣にある WinSparkle が、起動時にアプリが表示している言語（WinSparkle が対応している場合）で同じ処理を行い、インストーラーをサイレント実行して新バージョンを起動します |
| Linux, Android | アプリが appcast を読み込み、ユーザーがアップデートを選択すると署名済みパッケージを直接ディスクへストリーミングダウンロード・検証し、進捗（`updater.Status` およびプラットフォームのタスク進捗インジケーター）を報告した上で、`pkexec dpkg`、`pkexec pacman`（`.pkg.tar.gz`/`.pkg.tar.zst`）、アトミックな AppImage 置換（Linux では再起動）、または Android のパッケージインストーラーで導入します。これら以外の形式や未署名 appcast の場合は代わりにリリースページを開きます |
| iOS | アプリが App Store に問い合わせてアップデートを確認し、App Store 上の該当ページを案内します |

公開されたスタンドアロン Web アプリでは、`updater.Check(cx)` が実行中のリリースと最新の `nsgo.json` を比較し、確認後にリロードを提案します。同じバージョンラベルのままビルドファイルが変化した場合も検出します。appcast 鍵は不要ですが、明示的な `none`/`steam` チャンネルでは引き続きチェックが無効になり、組み込みホストではホスト側がリロードを管理します。ミニプログラムはプラットフォームのアップデートマネージャーを使用し、ダウンロードされたアップデートの準備が整った後にのみ再起動します。これらのターゲットはアプリがアップデーターを呼んだときにチェックを行い、ミニプログラムのダウンロード／チェックタイミングはホスト側に属します。また自動チェック設定のセッターは公開しません。カスタムの `cx.CheckForUpdates` ハンドラーでは、バージョンラベルが欠落している可能性のある準備済みアップデートに対して `UpdateCheck.Restart` を使用します。

### チャンネル

チャンネルは誰がアプリをアップデートするかを決定します。`-channel` は 1 回のビルドに対して `updates.channel` を上書きするため、1 つのプロジェクトからそれぞれ向けにビルドできます。

| チャンネル | 対象 | パッケージの振る舞い |
| --- | --- | --- |
| `direct` | GitHub、Web サイト、Homebrew cask、winget（リリース自身のファイルをインストールする経路） | 上記の方法で自己アップデートします |
| `store` | App Store、Mac App Store、Microsoft Store、Google Play、Flathub などのストア | [ストアパッケージ](#ストアパッケージ)となり、自己アップデーターを持ちません。iOS と macOS は鍵なしで自身の App Store に問い合わせ、Google Play からのインストールでは Play のネイティブアップデートフローを使用できます |
| `steam` | Steam | Steam がアップデートする[デポディレクトリ](#steam-デポ)を生成します |
| `none` | Linux ディストリビューションやアプリを再パッケージ化する配布者 | 何もチェックしません |

ストアは自己アップデートするアプリをリジェクトするため、必ず direct ではなく `store` ビルドをアップロードしてください。macOS または Windows のインストーラーを含む各 direct リリースでは、Homebrew cask（`dist/releases/homebrew/<name>.rb`：ビルドされたディスクイメージの `auto_updates true`、チェックサム、ダウンロード URL、およびアプリの各[コマンド](#コマンド)への `binary` リンクを宣言）と winget マルチファイルマニフェスト（`dist/releases/winget/<id>.yaml`、`<id>.installer.yaml`、`<id>.locale.en-US.yaml`：ビルドされた NSIS インストーラー用）も書き出されます。システムがどこからインストールされたかを判別できる場合、アプリ側でもそれをチェックします。別ストアからの Android アプリ、Flatpak や Snap、TestFlight ビルドではアップデートをそのストアに委ねます。Google Play インストール版で手動の `updater.Check` を実行すると、許可されている場合に Play In-App Updates へ即時フローの提示を要求し、同意・ダウンロード・インストール・再起動は Play が処理します。開発ビルドではこのフローに入りません。スタンドアロン Android パッケージには SDK がバンドルされ、[組み込みホスト](embedding.md#android)では Gradle 経由で提供します。

### アプリからの確認

`updater.Check(cx)` は *アップデートを確認…* メニュー項目と同様に確認を行い、見つかったものを表示します。`updater.CheckInBackground(cx)` はより新しいバージョンが見つかった場合のみ表示し、`updater.CheckDaily(cx)` はそれを 1 日最大 1 回まで行います。`updater.Status(cx)` は Linux と Android でダウンロードとインストールのライブ進捗を読み取ります。`automatic` が有効な間、Linux、direct Android ビルド、Apple App Store ビルドでは起動時とユーザーがアプリに戻ってきたときにその方法でチェックを行い、オフラインなどで失敗した場合は 1 時間後に再試行します。開発ビルドはアップデートしません。Steam ビルドではこれら 3 つの関数は何もしません。

`updater.CapabilityOf(cx)` はネットワークリクエストなしでこのコピーがどのようにアップデートされるかを報告します（`ManagedSelf`、`ManagedPage`、`ManagedDevelopment`、`ManagedChannel`、`Store` 名付きの `ManagedStore`、または `ManagedUnavailable`）。開発ビルド、無効化されたチャンネル、またはストア経由のインストールでは、チェック時に理由固有の説明が生成されます。`core.UpdateCheck.Err` は不正なフィードや読み込めないバンドルアップデーターなどの失敗専用です。

バインディング内で `updater.Status(cx)` を読み取ると、`Capability`、`AutomaticChecks`、`AutomaticDownloads`、`Available`、`Notes`、`LastCheck`、さらに Linux と Android ではアップデートの `Phase`、`Loaded`、`Total`（プログレスバー用の `Fraction()`）に追従できます。`updater.SetAutomaticChecks(cx, on)` はユーザーの選択を永続化し、実行時に日次スケジューラーを変更します。Sparkle と WinSparkle は独自に設定を保持し、Linux、Android、iOS はプライベートストレージパーティションに選択を保存します。`updater.SetAutomaticDownloads(cx, on)` は macOS のみでサポートされ、デフォルトは `updates.automatic_downloads` です。他のプラットフォームでは false を返し、セッターは無視されます。非対応および外部管理のビルドでは両設定とも false を報告します。WinSparkle は更新検出コールバック後に appcast を読み取って検出したアップデートのメタデータを報告しますが、インストールの制御は引き続き自身で行います。Linux は起動時に Debian または Arch パッケージの所有権と AppImage の識別を行い、それらは `ManagedSelf` を報告し、その他の未パッケージ化コピーはチェック前に `ManagedPage` を報告します。

## ログイン時起動

`desktop.SetLaunchAtLogin(cx, on)` はプラットフォームのログイン起動機構にアプリを登録し、`desktop.LaunchAtLogin(cx, done)` はその状態を読み取ります。現行ツールがビルドした MSIX には無効状態のスタートアップタスクが自動的に含まれており、有効化するとユーザーの「スタートアップ アプリ」スイッチおよび管理者ポリシーが尊重されます。通常の Windows と Linux のインストールでは Run レジストリ値と XDG autostart エントリが使われます。macOS はバンドルされたアプリのネイティブログインサービスを使用します。

Flatpak は Background ポータルを要求し（ユーザーに同意を求める場合があります）、ホスト設定ファイルへのアクセス権は付与されません。後続の読み取りはネイティブの応答を待ちます。このポータルには読み取り専用のスタートアップ照会機能がないため、Flatpak での読み取りはこの実行中に確認された最後の応答（それ以前は false）のみを返し、アプリ外での変更を検知したり次回の実行時に設定を復元したりすることはできません。

外部でパッケージ化された Snap では、アプリ定義上で autostart ファイル名を宣言します。

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo はそのファイル名を `$SNAP_USER_DATA/.config/autostart` 配下に書き出し、snapd がアプリのラッパー経由でそれを起動します。`desktop.LaunchedAtLogin(cx)` はアプリが構築される前に、MSIX StartupTask アクティベーションを含めてログイン起動かどうかを判別します。

## ファイル形式

`file_types` はアプリが開くドキュメント形式を登録します。macOS では Finder、Dock、「このアプリケーションで開く」、Windows では Windows インストーラーまたは MSIX を通じてエクスプローラーの「プログラムから開く」、Linux では Debian・Arch・AppImage・Flatpak パッケージのデスクトップエントリ、iOS では「次で開く」に対応します。

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

Windows インストーラーはユーザーの既定アプリを上書きせずに「プログラムから開く」へアプリを追加し、アップグレード時とアンインストール時に登録を削除します。Linux は一般的な拡張子の MIME タイプを固定テーブルから割り当て、アプリ独自の未知の拡張子には shared-mime-info 定義とともに `application/x-<type id>` を与えます。代わりに既存の MIME タイプを指定するには `mime` を使います（テーブルにない拡張子を持つインポート済みタイプには指定が必要です）。AppImage を単体で実行しただけでは何も登録されません。デスクトップ統合ツールがエントリと定義をインストールします。

アプリは `desktop.OnOpenFiles` でファイルを受け取ります（Windows や Linux で起動中に再度別のファイルから起動された場合や、iOS シーンのドキュメントオープンも含みます）。アプリを起動させたファイルは最初のハンドラーが登録されるまで待機します。

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

iOS では、通常のドキュメントは元のファイル名のままアプリ所有の一時ファイルへインポートされます。ファイルプロバイダーの読み取りは UI スレッド外で調整され、ハンドラーが実行される前にセキュリティスコープが終了します。保持したい場合は `cx.DataDir()` へ移動してください。編集はコピー側に対して行われます。iOS はアプリのドキュメントアイコンを使用し、`icon` は macOS で適用されます。App Clips と tvOS パッケージではドキュメント形式は登録されません。ファイルオープンは `OnOpenLinks` とは分離されています。

`desktop.AddRecentFile(cx, path)` はユーザーが開いたドキュメントを記録します。macOS では Dock メニューと「ファイル › 最近使った項目」、Windows ではジャンプリストの「最近使ったもの」カテゴリー、Linux ではデスクトップの最近使ったファイル一覧に表示され、選択すると同じハンドラーに届きます。`desktop.ClearRecentFiles` はアプリの登録を削除し、Linux では共有ファイルの他アプリ側の履歴を保持しつつ、GTK の履歴設定も尊重します。

`desktop.SetDockMenu(cx, items)` はアプリアイコン上のアクションを設定します。macOS の Dock メニュー、Windows のジャンプリストタスク、Linux の Ubuntu Dock・Plasma・Dash to Dock（クイックリスト対応環境）のクイックリストに反映されます。Linux ではアプリの `id` に一致するインストール済みデスクトップエントリが必要で、サブメニュー、セパレーター、チェック済み項目、無効化項目を保持します。メニューはその設定を行ったスコープに属し、アプリの複数ウィンドウの中で最後に設定したものが上書きします。

## リンク

`url_schemes` は `myapp://settings` のようなアプリ独自のリンクを設定し、ブラウザー、他のアプリ、`open`、`start`、`xdg-open` からアプリを開けるようにします。macOS、iOS、Android アプリに加え、Windows インストーラーと MSIX パッケージ、および Debian・Arch・AppImage・Flatpak パッケージが持つ Linux デスクトップエントリがこれらを登録します。アンインストールすると削除されます。

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

アプリは起動中も含めて `desktop.OnOpenLinks` でこれらを受け取ります。Windows と Linux では 2 回目の起動時に実行中のアプリへリンクを渡して終了します。アプリを起動させたリンクは最初のハンドラーが登録されるまで待機します。誰でもこのようなリンクを作成でき、Windows と Linux ではハンドラーはアプリの起動時に渡されたあらゆる URL を受け取るため、リンクによって何かを変更する前にスキームを確認し、ユーザーに確認を求めてください。

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

iOS と Android では、`mobile.OnOpenLinks` が起動時および実行中のリンクを受け取ります。iOS シミュレーターでは `xcrun simctl openurl booted myapp://settings`、Android では `adb shell am start -a android.intent.action.VIEW -d myapp://settings` で開けます。テストでは `app.OpenLinks("myapp://settings")` でリンクを配信できます。

iOS のユニバーサルリンクには、`app_links: [links.example.com]` で明示的な HTTPS ホストを宣言してください。各ホストはスキーム、パス、ポート、IP アドレス、ワイルドカードを含まない DNS 名で指定し、小文字化され重複は拒否されます。ツールは Xcode 管理の署名を含め、フルアプリの Associated Domains エンタイトルメントに `applinks:<host>` を書き込みます。IPA 用プロファイルでもそれを許可する必要があります。App Clips は引き続き `app_clip.domains` の独立した `appclips:` 宣言を保持します。

各ホストは `https://<host>/.well-known/apple-app-site-association` でフルアプリを記載した `apple-app-site-association` ファイルを配信する必要があります。パッケージのメタデータだけではユニバーサルリンクは有効になりません。サーバー設定については [Apple の関連付け要件](https://developer.apple.com/documentation/xcode/supporting-associated-domains)を参照してください。シミュレーターのアドホック署名では制限された開発者エンタイトルメントが省略されるため、デリゲートテストだけでサーバー関連付けの検証が完結するわけではありません。

iOS では、`mobile.OpenedURL(cx)` が直近の App Clip 起動 URL、ユニバーサルリンク、またはカスタムスキーム URL を追跡し、ルートがマウントされる前の初期値も含めて取得できます。カスタムスキームの配信は引き続き `mobile.OnOpenLinks` にも届き、そのハンドラー実行時点で起動 URL の値は最新になっています。ファイルオープンは `OnOpenFiles` に留まり、起動 URL を上書きしません。Android と Web の起動 URL 対応は #1475 で別途追跡されています。

## Android での共有の受け取り

Android の共有シートからアプリが受け取る MIME タイプを選択します。

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

APK および App Bundle ビルドでは、単一アイテムと複数アイテムの両方に対してこれらのタイプを登録します。各共有を受け取るには [`mobile.OnShared`](services.md#リンクとファイルを開く) を登録してください。

パスベースのアドレスを使う Web アプリでは、`web.base` にデプロイ先ディレクトリ（`/` または `/my-app/`）を設定し、本番サーバーがアプリケーションルートに対して `index.html` を返すように構成してください。開発サーバーは HTML ナビゲーションリクエストに対してすでにこのフォールバックを提供しています。ディープリンクがリロードされた際も、アセットはその固定された HTML `<base>` からの相対パスのまま解決されます。[アプリケーションアドレス](addresses.md)を参照してください。

## iOS での共有の受け取り

Share 拡張機能が受け入れる Uniform Type Identifiers を、URL およびテキストタイプを広範なファイルタイプより前に並べて宣言します。

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` と `dev` は Go 製の Share 拡張 `<id>.share` をアプリに同梱します。Xcode は実機実行時にこれに署名し App Group を登録します。IPA の場合は、`<id>` と `<id>.share` の両方で同じ App Group を有効にする必要があり、`sign.profile` と `sign.share_profile` でそれが許可されている必要があります。App Clips、tvOS、watchOS には Share 拡張はありません。

拡張機能は最大 128 アイテムかつ合計 512 MiB までのファイルを App Group へコピーし、共有シートを完了します。`mobile.OnShared` はアプリがアクティブな間、またはユーザーが次にアプリを開いたときにこれらのバッチを受け取ります。データは配信前に拡張機能やアプリが終了しても失われません。iOS には Share 拡張から親アプリをフォアグラウンドにしたり起動したりするための公式 API がないため、この受け渡しによってアプリが自動的に開くことはありません。

## ミニプログラム

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

各ターゲットはプラットフォームの開発者ツール用プロジェクトを出力し（`dist/releases/miniprogram/<platform>` にも置かれます）、`-run` と `dev` がそれを開きます。`dev` は保存のたびに再ビルドし、ツールがリロードします。

| ターゲット | プラットフォーム | ツール |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html)（Settings > Security でサービスポートを有効化） |
| `miniprogram_qq` | QQ | [QQ 開発者ツール](https://q.qq.com/wiki/tools/devtool/)（macOS） |
| `miniprogram_alipay` | Alipay | [Alipay Mini Program Studio](https://opendocs.alipay.com/mini/ide/download)（macOS） |
| `miniprogram_douyin` | Douyin および ByteDance の他アプリ | [Douyin 開発者ツール](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download)（macOS） |

`miniprogram.appid` には各プラットフォーム（`wechat`、`qq`、`alipay`、`douyin`）のミニプログラム AppID を設定します。設定しない場合、WeChat DevTools ではゲスト AppID でプレビューされ、他のツールでは AppID の入力を求められます。WeChat では `-run` および `dev` の各再ビルド時に CLI 経由でプロジェクトを閉じて自動的に開き直すため、DevTools が新しいファイルをコンパイルします。この動作は DevTools 2.01.2510290 で確認されています。プラットフォームのナビゲーションバーにはアプリのタイトルが表示され、`custom_navigation` を有効にするとアプリがページ全体を満たし、セーフエリアによってメニューボタンとの重なりが回避されます。アプリは `Env.Simulated` が true の状態でスマートフォンの OS を読み取ります（`--nsgo-os` を指定した Web と同様）。システムヘッダーが置き換わらないため、スタックの各ページには独自の `Header` が表示されます。QQ、Alipay、Douyin はそれぞれのツール上での実機／ツール検証がまだ済んでおらず、Alipay はまだアプリ経由の戻る操作、キーフレーム、キーボードの高さに対応していません。

ミニプログラムは実験的機能です。ミニプログラムでは Go アプリのみが動作し TSX は動作しません。また各プラットフォームが圧縮済みモジュールサイズを 2 MB に制限しているため、`examples/gallery-miniprogram` のように `base` を使った数画面のアプリは収まりますが、`ui` のコントロールを使ったアプリはまだ収まらず、収まらない場合は `build` がその旨を通知します。シェーダーは描画されず、WebView、動画、ネイティブコンポーネント、アプリのフォントファイルもまだ表示されません。

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` は `dist/releases` に `<name>-<version>-harmonyos.hap` を書き出します。エンジンは ArkWeb 内の WebAssembly として動作し、ArkTS ホストがマウントされた内容を ArkUI ビューで描画するため、テキスト、フィールド、スクロール、ダイアログはシステムネイティブのものになります。`PATH` に `hdc` がある場合、`-run` は接続された実機またはエミュレーターに HAP をインストールして起動し、`nsgo test -target harmonyos` は別の `.nsgotest` バンドル ID を使ってエミュレーター上の未署名 HAP からアプリのテストを実行し、インストール済みのアプリを残したまま実行後にテスト用 HAP を削除します。

テンプレートから ArkTS ホストをコンパイルし、アプリの `id` をバンドル名として使用するには、`PATH` に `hvigorw` を通し、`OHOS_BASE_SDK_HOME` にテンプレートのコンパイルおよびターゲット SDK バージョンである API 26（`26.0.0`）の OpenHarmony SDK を設定してください。`build.harmonyos_bundles` にはデフォルトの `auto`、SDK が必須の `exact`、またはコミット済みホストの固定長に合わせて ID をパディングし SDK 不要でビルドできる `padded` を指定します。ターゲットテストも同じ設定を使用し、どのホストをパッケージ化したかを表示します。`harmonyos.permissions` はアプリが要求する `ohos.permission` 名（`MICROPHONE` や `CAMERA` など）を列挙し、`widget.title` はホーム画面カードに表示するタイトルを指定します。

HarmonyOS はプレビュー機能です。エミュレーターには未署名の HAP をインストールできます。プレビューが描画する ArkUI 機能の一覧は[アーキテクチャノート](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md)に記載されています。

### 署名済み HAP

AppGallery Connect の署名資材で HAP に署名するには `harmonyos.signing` を設定します。Huawei 製スマートフォンへ直接インストールするには、その実機用のデバッグ証明書とプロファイルが必要です。配布にはリリース用資材を使用してください。[Huawei 署名ガイド](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

パスは `nsgo.yaml` からの相対パスです。環境変数 `NSGO_HARMONY_KEYSTORE_PASSWORD` と `NSGO_HARMONY_KEY_PASSWORD` を設定してください。署名には SDK の `hap-sign-tool.jar` と `JAVA_HOME` または `PATH` 上の Java を使用します。プロファイルに一致させるためビルドはアプリの正確な `id` を維持する必要があり、`auto` モードであっても署名には SDK と `hvigorw` が必要です。アプリディレクトリで `go tool nsgo doctor` を実行すると、パスワードを使わずに署名設定とプロファイルのバンドル名を検証できます。`build -run` は署名済み HAP を `hdc` に渡します。

## 署名

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

キーストアのパスワードは `NSGO_KEYSTORE_PASSWORD` と `NSGO_KEY_PASSWORD` から取得します。`-team` は iPhone 用パッケージに署名します。`-profile`、`-widgets-profile`、`-share-profile`、`-clip-profile`、`-macos-profile` はコマンドラインからアプリ、ウィジェット拡張、Share 拡張、App Clip、Mac App Store 用のプロビジョニングプロファイルを上書きします。`-installer` は Mac Installer Distribution アイデンティティを上書きします。

### Windows パッケージの署名

Authenticode は Windows ダウンロードの発行元を証明します（[SmartScreen は評価を別途チェックします](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation)）。`.pfx` ファイルである `sign.certificate` は、実行ファイル、コマンドコンパニオン、アプリのリソースに含まれるプログラムやライブラリ、アンインストーラーを含むインストーラー、および Store チャンネル向けにビルドされた MSIX に署名します。パスワードは `NSGO_CERTIFICATE_PASSWORD` から取得します。署名は `signtool` と同等の形式で Go により生成されるため、Mac 上でも Windows SDK なしで署名できます。

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` のデフォルトは DigiCert のサーバーであり、`none` を指定するとタイムスタンプなしになります（その場合、証明書の有効期限切れとともに署名も失効します）。ハードウェアトークンやクラウドサービス上の証明書を使う場合は、代わりに独自のコマンドで署名します。`%1` が署名対象ファイルになり、ビルドは上記の各ファイルに対してこのコマンドを実行します。

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

コマンドが成功したにもかかわらず、アプリ実行ファイル、コマンドコンパニオン、PE リソース、またはセットアップ実行ファイルに埋め込み Authenticode 署名が付いていない場合、`build` は停止します。

インストーラー内部のアンインストーラーは、埋め込まれる前に、アプリやセットアップと同じ `sign.certificate` または `sign.sign_command` で署名されます。`-certificate`、`-sign-command`、`-timestamp-url` を使ってコマンドラインからこれらを指定することもできます。

### 公証（Notarize）

Mac 向けにダウンロードされるアプリは、Apple による公証を受けることで Gatekeeper の警告なしに開けるようになります。Developer ID で署名し、`notarytool` がキーチェーンに保存する認証情報名を指定してください。

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

これにより `build` は Hardened Runtime 付きで署名し、ディスクイメージを提出し、Apple の審査結果（数分程度）を待ってから、ディスクイメージとアプリにチケットをステープルします。却下された場合は Apple のログが出力されます。CI 環境では、プロファイルの代わりに App Store Connect API キーの `.p8` ファイル、キー ID、Issuer ID である `NSGO_NOTARY_KEY`、`NSGO_NOTARY_KEY_ID`、`NSGO_NOTARY_ISSUER` を使用します。Hardened Runtime はアプリが宣言しない限りマイク、カメラ、カレンダー、位置情報へのアクセスを拒否しますが、`apple.info` に `NSMicrophoneUsageDescription`、`NSCameraUsageDescription`、カレンダーまたはリマインダーの利用説明、あるいは位置情報の利用説明が含まれている場合、`build` がこれらを自動的に許可します。

## ウィジェットとライブアクティビティ

`build` と `dev` は、アプリのウィジェットとライブアクティビティを描画する拡張機能を Xcode の Swift コンパイラで自動的にコンパイルし、追加手順なしで macOS と iOS パッケージに組み込みます。iOS ではアプリとそのウィジェットで App Group を共有します。iPhone での `-run` では Xcode が両方に署名してグループを登録し、IPA の場合は `<id>` と `<id>.widgets` の両方でグループを有効にした上で、`sign.widgets_profile` で拡張機能に署名してください。Android のスタンドアロン APK と AAB には `widgets` から生成された App Widget プロバイダーとリソースが含まれ、ビルド済みの Java ランタイムがアプリビルド時の SDK や Java コンパイラなしでタイムラインを描画します。Android 16 ではライブアクティビティを Live Updates として表示し、拡張機能は不要です。設定と API については[ウィジェット](widgets.md)および[ライブアクティビティ](live-activities.md)を参照してください。

## Android App Links

`app_links` には、リンクを開いたときにフル Android アプリを起動する明示的な HTTPS ホストを指定します。

```yaml
app_links: [example.com, shop.example.com]
```

APK と App Bundle は各ホストに対して `autoVerify` 付きの browsable `ACTION_VIEW` フィルターを宣言します。スキーム、パス、ポート、ワイルドカード、IP アドレスを含まない DNS ホストを指定してください。`https://<host>/.well-known/assetlinks.json` でアプリのパッケージ ID と署名証明書のフィンガープリントを配信すると、Android はアプリへリンクをルーティングする前にその関連付けを検証します。ツール自体は関連付けファイルを公開しません。設定方法は [Android の検証ガイド](https://developer.android.com/training/app-links/verify-applinks)を参照してください。組み込み Android ホストでは独自のインテントフィルターを宣言し、受け取ったリンクを `NsgoView` へ転送します。

`mobile.OpenedURL(cx)` は初回のルート描画前から受け付けた起動 URL を読み取れ、実行中のアプリを別のリンクが開いたときにも更新されます。`mobile.OnOpenLinks` も引き続き受け付けた各リンクを受け取ります。ページ全体を使う Web アプリはブラウザー履歴やハッシュ変更を含めて現在のページアドレスを `mobile.OpenedURL` から読み取りますが、別ページ内にマウントされたアプリは親ページのアドレスを読み取りません。アプリ側からアドレスを書き換える場合は [`cx.AppURL`](addresses.md) を使用してください。

## App Clips

App Clip は、アプリをインストールしなくてもリンク、コード、または NFC タグから起動できる iOS アプリの軽量な一部です。`app_clip` を設定すると、表示するものだけをリンクできるように専用の main パッケージから iOS パッケージ内へ組み込みます。

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

クリップは起動に使われたリンクを `mobile.OpenedURL` で読み取り（別のリンクで再度開かれると更新されます）、コードが App Clip として動いているかどうかは `mobile.IsAppClip` で判別できます。システムがダウンロードできるクリップの上限は 15 MB（リンクからのみ開く設定で iOS 17 以降を対象にする場合は 100 MB）であり、これを超えると `build` がエラーにします。`ui` をリンクするクリップは約 11 MB になります。`dev -target ios_simulator -clip https://example.com/order?table=12` はそのリンクから開いた状態でシミュレーター上にクリップ単体を起動し、`-target ios` は iPhone 上で実行してリンクの QR コードを表示します（[設定] > [デベロッパ] > [Local Experiences] にリンクを追加し、カメラでスキャンしてください）。開発用シンボルによってクリップが QR サイズ制限を超える場合は `-release` を使用してください。Apple が App Clip への署名を許可しているのは個人チームではなく Apple Developer Program のチームのみです。

`dev -target ios` と `build -run -target ios` では、フルアプリ内にもクリップを含められます。Xcode はクリップを別ターゲットとして署名し、アプリを署名する前に埋め込みます。自動署名には、アプリとクリップ双方のプロビジョニングを含む認証済みの有料チームアカウントが Xcode に必要です。App Store 向けには、`<id>` で App Clips を有効にし、`sign.clip_profile` でクリップに署名し、各ドメインで `apple-app-site-association` ファイルを配信してください。リポジトリの `examples/appclip` は、クリップがテーブルで注文を行うカフェのサンプルです。

注文情報などのローカルデータをフルアプリへ引き渡すには、`app_clip` と並べて明示的なグループを設定します。

```yaml
apple:
  app_group: group.dev.example.hello
```

これにより、クリップとフルアプリはグループのコンテナ内で `cx.DataDir()` と `cx.Storage()` を共有します。クリップが書き込んだデータディレクトリ配下のファイルや Key-Value エントリは、フルアプリ起動時に利用できます。`cx.LogsDir()` もそのディレクトリに追従しますが、キャッシュは各バンドルが個別に保持します。Xcode 署名では両ターゲットにそのグループを要求し、プロファイルを使用する場合は両方の App ID で同じグループを有効にして `sign.profile` と `sign.clip_profile` で付与する必要があります。付与が欠けていると署名が拒否され、アクセスできないグループを指定すると起動が停止します。ホストが明示的にデータディレクトリを指定している場合はそちらが優先されます。

`apple.app_group` を指定しない場合、各バンドルは独自のサンドボックスストレージを保持します。この設定を有効にしても以前それらのサンドボックスに保存されていたデータは移行されず、ウィジェットのみのアプリは従来のストレージ場所を維持します。共有シークレットは平文ストレージではなくキーチェーンに保存してください。カフェのサンプルではクリップ側の注文を永続化し、フルアプリ側で表示しています。

クリップ内の明示的なユーザー操作から `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` を呼ぶと、StoreKit に対して対応するフルアプリを推奨表示するよう要求できます。`AppClipOverlayBottomRaised` は下部バー用の余白を空けます。戻り値の関数はその要求を閉じ、スコープのクリーンアップ時にも閉じられます。また古い要求から新しい要求を閉じることはできません。ネイティブ iOS App Clip 以外での呼び出しは何も行いません。推奨を読み込んで表示できるかどうかは App Store 側が判断するため、この呼び出しはオファーが表示されたかやインストールされたかは報告しません。フルアプリをインストールしなくてもクリップのタスクを完了できるように設計してください。[Apple のオーバーレイガイダンス](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## 他のアプリへの組み込み

`-embed` を指定すると、独自の iOS または Android アプリ向けにパッケージ化できます。出力は XCFramework（`-dynamic` 指定時は動的フレームワーク群）または AAR となり、それぞれの `NSGoView` または `NsgoView` で表示できます。Go デスクトッププログラムでは `nsgo.NewView` を、Web ページではサイトの `nsgo.js`（ホストはページ自身または `nsgo.yaml` の `web.host` で指定したもの）を使って表示します。リポジトリの `examples/embed-*` にそれぞれの例があり、ホスト側の詳細は[組み込み](embedding.md)で解説しています。
