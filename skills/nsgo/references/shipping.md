# Project layout and shipping

## Layout

`go tool nsgo init` writes the layout every app keeps; `examples/hello` in the module directory is the smallest complete one.

| Path | What |
| --- | --- |
| `app.go` (or `main.go`) | The main package: `Root`, or `App` for a desktop app opening its own windows, and `main` calling `nsgo.Run` |
| `app_test.go` | `nsgotest` tests, with `TestMain` calling `nsgotest.Main` |
| `nsgo.yaml` | The app's name, id, version, icons, targets and packaging |
| `locales/` | A YAML catalog per language (`en.yaml`, `zh-Hans.yaml`) and the package the tool generates its functions into (`catalog_gen.go`); never edit generated files |
| `assets/` | Icons (`AppIcon.icon`, `Icon.png`) and what the app embeds; `assets/*-dev.*` icons mark dev builds |
| `.nsgo/` | The tool's own files, such as TSX types and test screenshots; ignore it in Git |
| `dist/` | Packages `build` writes; ignore it in Git |

Keep packages the app imports in the same module; `go tool nsgo check` and `test` take a directory without `nsgo.yaml`, or `./...`, as every package under it.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello          # without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
icon:
  apple: assets/AppIcon.icon   # macOS, iOS and watchOS
  png: assets/Icon.png         # every other target
dev:
  target: native               # what `nsgo dev` runs without -target
build:
  targets: [macos_arm64, windows_x64, linux_x64, ios, android_arm64, web]
  # tags: [timetzdata]         # Go build tags every build, test and check of the app takes
  # ldflags: [-X main.releaseDate=2026-11-17]  # the app's own linker flags; -ldflags on build and test adds more
# i18n: { dir: locales, source: en }
# apple: { info: { NSCameraUsageDescription: Scans receipts. } }
# release: { github: me/hello }
# updates: { key: <public key from nsgo keygen> }
# store: { dir: store }       # YAML and Markdown store listings per language
# steam: { app_id: 480 }      # use your Steamworks App ID for native dev
```

A flag replaces the value it names (`-target`, `-version`, `-output`, or the signing profiles `-profile`, `-widgets-profile` and `-clip-profile`); `go tool nsgo build -h` lists them. Read the version-matched [Building and shipping](https://nsgo.dev/docs/building.md) page for the rest: installers, store packages, signing, notarizing, GitHub releases, updates, file types, App Clips, widgets and mini programs.

## Build

```sh
go tool nsgo build                       # every target nsgo.yaml lists
go tool nsgo build -target web           # or all, macos, windows, macos_arm64, ios, android_arm64, miniprogram_wechat, ...
go tool nsgo build -run -device "iPhone" # build and start or install it
go tool nsgo build -channel store        # store packages instead of installers and self-updates
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
go tool nsgo build -release-notes notes.md  # the release's notes, which the update prompts show
```

Packages land in `dist/<name>-<version>-<target>`, and everything to hand out in `dist/releases`, each file named `<name>-<version>-<system>-<arch>` with its `.sha256`. Builds are release builds; `go tool nsgo dev -release` runs one with hot reload for measuring.

On the store channel, `-target macos` and `-target macos_universal` write one universal Mac App Store PKG. `-target android_arm64,android_x64` writes one AAB for both ABIs and needs an NDK for x86_64 ([Building](https://nsgo.dev/docs/building.md#store-packages)).

Android store builds require API 36 or higher. Record outstanding store questionnaires in `compliance` and declare Apple's export compliance in `apple.info` ([Store compliance](https://nsgo.dev/docs/building.md#store-compliance)).
For Steam, upload the desktop target directories directly. Native dev with `steam.app_id` writes `steam_appid.txt` into its build and starts there. The depot output excludes that file, installers and self-updaters.


## Store listings

Keep App Store and Google Play metadata as plain-text YAML and Markdown under `store/` (or `store.dir`): top-level `listing.yaml` (`version: 1`) and per-locale `<locale>/listing.yaml` (`version: 1`, unknown keys refused) plus optional `description.md`, `release_notes.md`, `promotional_text.md` and `screenshots/`.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, required fields, locales and media
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
```

Validation runs before reading credentials or sending HTTP requests. Canonical locale directories like `zh-Hans` map to `zh-Hans` on App Store Connect and `zh-CN` on Google Play unless overridden.

Publish built web and desktop packages with `go tool nsgo publish itch -target web,macos_arm64`. Set `itch.project` in `nsgo.yaml` and pass `BUTLER_API_KEY` as a CI secret ([itch.io publishing](https://nsgo.dev/docs/building.md#publish-on-itchio)).
## Documents

On Linux, audio and video rely on the system's GStreamer plug-ins. Use `linux.depends` and `linux.recommends` to declare the codecs your app needs. [Building](https://nsgo.dev/docs/building.md).

Declare `file_types` in `nsgo.yaml` and handle paths with `desktop.OnOpenFiles`. macOS, Windows (NSIS/MSIX) and Linux packages register them; Windows retains the user's default handler. Linux infers common extensions from a fixed table and defines unknown app-owned types as `application/x-<type id>`; set `mime: [video/mp4]` to reference existing types, especially for unknown imported extensions. AppImage needs desktop integration to install the bundled entry and any custom MIME definitions.

Declare `url_schemes: [myapp]` for the app's own links and handle them with `desktop.OnOpenLinks`, or `mobile.OnOpenLinks` on iOS and Android. The macOS, iOS and Android apps, the NSIS installer and MSIX package, and the Linux desktop entry register them; on Windows and Linux a second launch hands its link to the running app. On Windows and Linux the handler hears any URL the app is started with, so check the scheme, and confirm with the user before a link changes anything.

Set `android.share_types: [text/plain, 'image/*']` to receive those MIME types from the Android share sheet. Register `mobile.OnShared` for the text and cache-backed files, including what launched the app. [Building](https://nsgo.dev/docs/building.md#incoming-shares-on-android)

## Secrets

Never commit secret keys or passwords. The tool reads them from the environment: `NSGO_UPDATES_KEY` for update feeds, `NSGO_KEYSTORE_PASSWORD` and `NSGO_KEY_PASSWORD` for Android keystores, `NSGO_CERTIFICATE_PASSWORD` for a Windows code signing certificate, `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` and `NSGO_NOTARY_ISSUER` for notarizing in CI, `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` and `NSGO_APP_STORE_ISSUER` (falling back to the notary key) for App Store Connect listings, `NSGO_PLAY_SERVICE_ACCOUNT` for Google Play listings, and `NSGO_ROOT_KEY` and `NSGO_RELEASE_KEY` for signed TSX packages. Public keys belong in `nsgo.yaml` and the app's code.

For an adaptive Android launcher icon, set `icon.android.foreground` and `background` (a PNG or a `"#RRGGBB"` color), and `monochrome` for Android 13's themed icons: square 432 px layers with the logo inside the center 264 px, which every launcher mask keeps ([Building](https://nsgo.dev/docs/building.md)).

## Permissions and platform entries

Set `android.loopback_cleartext: true` to load plain HTTP from `localhost`, `127.0.0.1` or `::1` in a standalone APK or App Bundle. Set the host app’s network security policy for an embedded AAR.

Ask for a permission where the app uses it, through `cx`'s permission calls, and explain it in `apple.info`'s usage descriptions; on Android, `android.permissions` adds manifest permissions beyond the runtime's. An app calling `cx.SignInWithApple` sets `apple.sign_in_with_apple: true`, which signs it with the entitlement the sheet needs, one used beside other apps on an iPad sets `apple.requires_full_screen: false`, for Split View, Slide Over and Stage Manager, and lays out at whatever size `Env` reports; and one selling in-app purchases sets `apple.purchases: true`, which bundles StoreKit's library; it shows the store's `DisplayPrice`, never a price of its own, has its server verify a transaction's `Signed` JWS before delivering it, finishes it with `cx.FinishTransaction`, and registers `cx.OnPurchaseUpdate` as it starts. Check `cx.Env()` (`CanRecognizeSpeech`, `CanSpeak`, `CanPurchase`, `CanRequestReview`, `HasWebView` and the rest) before using a service a target may lack, rather than testing the target's name. For `cx.RequestReview()` on Android, supply the [Play Review SDK in the embedding app](https://nsgo.dev/docs/services.md#store-reviews).

Apple bundles carry a `PrivacyInfo.xcprivacy` declaring the required-reason APIs NSGo uses; declare the app's own under `apple.privacy` (`accessed_apis`, `tracking`, `tracking_domains`, `collected_data`) or in `resources/PrivacyInfo.xcprivacy`, which is merged. A host app linking the static `-embed` library declares NSGo's in its own manifest ([Building](https://nsgo.dev/docs/building.md#store-packages)).

For standalone Android apps that host a custom Android View with `core.Native`, list its precompiled `.dex` or DEX JAR under `android.classes`. NSGo includes those files when packaging an APK or App Bundle. [Building](https://nsgo.dev/docs/building.md#android-native-classes)

## Embedding

`go tool nsgo build -embed -target ios|android_arm64` packages an app as an XCFramework or an AAR for a host app of your own, `nsgo.NewView` shows one in a Go desktop program, and the web build's loader puts apps in a page. `examples/embed-*` show each host ([Embedding](https://nsgo.dev/docs/embedding.md)).

The web loader shows download progress. Style `progress.nsgo-load-progress` or replace it with `onProgress` in the mount options or the `web.host` module. A release includes smaller Brotli and gzip copies of its wasm for servers configured to negotiate precompressed files.
