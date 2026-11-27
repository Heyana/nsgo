# Project layout and shipping

## Layout

`nsgo init` writes the layout every app keeps; `examples/hello` in the module directory is the smallest complete one.

| Path | What |
| --- | --- |
| `app.go` (or `main.go`) | The main package: `Root`, or `App` for a desktop app opening its own windows, and `main` calling `nsgo.Run` |
| `app_test.go` | `nsgotest` tests, with `TestMain` calling `nsgotest.Main` |
| `nsgo.yaml` | The app's name, id, version, icons, targets and packaging |
| `locales/` | A YAML catalog per language (`en.yaml`, `zh-Hans.yaml`) and the package the tool generates its functions into (`catalog_gen.go`); never edit generated files |
| `assets/` | Icons (`AppIcon.icon`, `Icon.png`) and what the app embeds; `assets/*-dev.*` icons mark dev builds |
| `.nsgo/` | The tool's own files, such as TSX types and test screenshots; ignore it in Git |
| `dist/` | Packages `build` writes; ignore it in Git |

Keep packages the app imports in the same module; `nsgo check` and `test` take a directory without `nsgo.yaml`, or `./...`, as every package under it.

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
# linux: { share: packaging/share }  # files the Linux packages install under /usr/share, such as metainfo/<id>.metainfo.xml
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app; windows: { path: true } puts them on Path
# release: { github: me/hello, tag_prefix: v }
# updates: { key: <public key from nsgo keygen> }
# store: { dir: store }       # YAML and Markdown store listings per language
# steam: { app_id: 480 }      # use your Steamworks App ID for native dev
```

A flag replaces the value it names (`-target`, `-version`, `-output`, or the signing profiles `-profile`, `-widgets-profile`, `-share-profile` and `-clip-profile`); `nsgo build -h` lists them. Go tools that read `nsgo.yaml` share the tool's schema, defaults and validation through `github.com/nsgo-dev/nsgo/project` (`project.Load`, `project.Config.Channel`). Read the version-matched [Building and shipping](https://nsgo.dev/docs/building.md) page for the rest: installers, store packages, signing, notarizing, GitHub releases, updates, file types, App Clips, widgets and mini programs.

Set `game: true` for game classification and Game Mode metadata, and `gamepad: true` when the app supports a hardware gamepad. See [Games](https://nsgo.dev/docs/building.md#games).

An iOS app with `app_clip` keeps its clip when Xcode automatically signs `dev` or `build -run`; it needs a paid-team account and provisioning for both targets. `dev -clip <url>` runs the clip alone; use `-release` if development symbols exceed the QR size limit. [App Clips](https://nsgo.dev/docs/building.md#app-clips).

Set `launch_screen.background` to a quoted `"#RRGGBB"` color and/or `launch_screen.image` to a centered PNG path on iOS and Android 12 or later. Use `launch_screen.dark` to override either field for dark appearance ([Launch screen](https://nsgo.dev/docs/building.md#launch-screen)).

## Build

```sh
nsgo build                       # every target nsgo.yaml lists
nsgo build -target web           # or all, macos, windows, macos_arm64, ios, android_arm64, miniprogram_wechat, ...
nsgo build -run -device "iPhone" # build and start or install it
nsgo build -channel store        # store packages instead of installers and self-updates
nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
nsgo build -release-notes notes.md  # the release's notes, which the update prompts show
```

Packages land in `dist/<name>-<version>-<target>`, and everything to hand out in `dist/releases`, each file named `<name>-<version>-<system>-<arch>` with its `.sha256`. Direct releases with macOS or Windows installers also write a Homebrew cask (`dist/releases/homebrew/<slug>.rb`, with `auto_updates true`) and a winget multi-file manifest (`dist/releases/winget/`). Builds are release builds; `nsgo dev -release` runs one with hot reload for measuring.

On a headless Mac, set `build.skip_installers: [macos]` or pass `-skip-installers=macos` to keep the signed `.app` and build the other desktop installers without mounting a DMG. [Installers](https://nsgo.dev/docs/building.md#installers)

Use the raw Git tag prefix in `release.tag_prefix`; generated install scripts encode it for GitHub download URLs. [GitHub releases](https://nsgo.dev/docs/building.md#publish-on-github)

`sign.certificate` also signs the Store channel's MSIX in Go. For installation outside the Store, match the certificate subject to `windows.store.publisher` and use a certificate the receiving Windows machine trusts; without signing settings, the Store signs the uploaded package. See [Windows signing](https://nsgo.dev/docs/building.md#sign-windows-packages).

On the store channel, `-target macos` and `-target macos_universal` write one universal Mac App Store PKG. `-target android_arm64,android_x64` writes one AAB for both ABIs and needs an NDK for x86_64 ([Building](https://nsgo.dev/docs/building.md#store-packages)).

Linux store builds write an installable `.flatpak` in Go alongside the archive and `flatpak/<id>.yml` for flatpak-builder/Flathub. Install with `flatpak install --user ./<bundle>.flatpak`; use a configured remote for GNOME runtime dependencies.

Android store builds require API 36 or higher. Record outstanding store questionnaires in `compliance` and declare Apple's export compliance in `apple.info` ([Store compliance](https://nsgo.dev/docs/building.md#store-compliance)).

For Google Play, build with `-target android -channel store`, then use `nsgo publish -target android -track internal`. Set `NSGO_PLAY_SERVICE_ACCOUNT` to the service account JSON key or its file path. [Google Play publishing](https://nsgo.dev/docs/building.md#publish-to-google-play)

For Steam, upload the desktop target directories directly. Use `steam-launch.sh` for Linux x64 and upload `nsgo-gtk/` with it. Native dev with `steam.app_id` writes `steam_appid.txt` into its build and starts there. The depot output excludes that file, installers and self-updaters.

Set `web.offline: true` in `nsgo.yaml` to cache a published web build and its packaged resources for offline launch. Serve the site over HTTPS or preview it with `nsgo build -target web -run`. See [Offline web packages](https://nsgo.dev/docs/building.md#offline-web-packages).

Mac App Store builds use Apple's lookup and native store page. A Google Play install's manual `updater.Check` enters Play's immediate update flow when allowed; other store installs stay managed, and no store install reads a direct appcast. Embedded Android hosts supply `com.google.android.play:app-update:2.1.0` and its [shrink rules](https://nsgo.dev/docs/embedding.md#android).

Use `updater.CapabilityOf(cx)` to explain whether a copy updates itself, opens a page, is a development build, follows its channel or store, or has no updater; it makes no network request. Direct Linux copies owned by dpkg or pacman, or running from an AppImage, report `ManagedSelf`; other unpackaged copies report `ManagedPage`. Read `updater.Status(cx)` inside a binding for preferences, the available version, notes and last-check time. Persist runtime choices with `updater.SetAutomaticChecks`; `updater.SetAutomaticDownloads` and `updates.automatic_downloads` are macOS-only. TSX apps import the same API from `nsgo/updater` (`checkForUpdate`, `checkInBackground`, `checkDaily`, `capability`, `status`, `setAutomaticChecks`, `setAutomaticDownloads`), withheld from sandboxes unless listed in `Sandbox.Modules`. See [Updates](https://nsgo.dev/docs/building.md#updates).

Publish each Linux `.AppImage` with its `.AppImage.zsync` sidecar. Embedded update discovery defaults to `release.github` or the GitHub remote `origin`; `linux.appimage_update` overrides it with a stable `zsync|https://…` URL or a `gh-releases-zsync` locator, and an explicit empty string disables discovery. Keep the URL stable and serve HTTP ranges. [AppImage updates](https://nsgo.dev/docs/building.md#installers)

Published standalone web apps use `updater.Check` to compare their current descriptor with `nsgo.json` and confirm a reload. Mini programs use the host update manager and apply only ready updates; custom check handlers must accept `core.UpdateCheck.Restart` without a version label. Embedded hosts manage reloads themselves. See [Updates](https://nsgo.dev/docs/building.md#updates).

## Store listings

Keep App Store and Google Play metadata as plain-text YAML and Markdown under `store/` (or `store.dir`): top-level `listing.yaml` (`version: 1`) and per-locale `<locale>/listing.yaml` (`version: 1`, unknown keys refused) plus optional `description.md`, `release_notes.md`, `promotional_text.md` and `screenshots/`.

```sh
nsgo publish -store all -dry-run                  # validate lengths, required fields, locales and media
nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
```

Validation runs before reading credentials or sending HTTP requests. Canonical locale directories like `zh-Hans` map to `zh-Hans` on App Store Connect and `zh-CN` on Google Play unless overridden.

Publish built web and desktop packages with `nsgo publish itch -target web,macos_arm64`. Set `itch.project` in `nsgo.yaml` and pass `BUTLER_API_KEY` as a CI secret ([itch.io publishing](https://nsgo.dev/docs/building.md#publish-on-itchio)).

Standalone builds bundle `THIRD_PARTY_NOTICES.txt`. Show the text with `core.ThirdPartyNotices()`. Configure missing or unrecognized licenses with `licenses.unknown`, copyleft handling with `licenses.copyleft`, and reviewed exceptions by exact module path in `licenses.allow`.

MSIX display names use each catalog's `appName`, with source-language fallback. The tool writes the language/scale resource index and 100–400% logo variants directly; building on macOS needs no Windows SDK.

## Documents

On Linux, audio and video rely on the system's GStreamer plug-ins. Use `linux.depends` and `linux.recommends` to declare the codecs your app needs. [Building](https://nsgo.dev/docs/building.md).

Declare `file_types` in `nsgo.yaml` and handle paths with `desktop.OnOpenFiles`. macOS, Windows (setup/MSIX) and Linux packages register them; Windows retains the user's default handler. Linux infers common extensions from a fixed table and defines unknown app-owned types as `application/x-<type id>`; set `mime: [video/mp4]` to reference existing types, especially for unknown imported extensions. AppImage needs desktop integration to install the bundled entry and any custom MIME definitions.

Declare `url_schemes: [myapp]` for the app's own links and handle them with `desktop.OnOpenLinks`, or `mobile.OnOpenLinks` on iOS and Android. The macOS, iOS and Android apps, the Windows installer and MSIX package, and the Linux desktop entry register them; on Windows and Linux a second launch hands its link to the running app. On Windows and Linux the handler hears any URL the app is started with, so check the scheme, and confirm with the user before a link changes anything.

Set `android.share_types: [text/plain, 'image/*']` to receive those MIME types from the Android share sheet. Register `mobile.OnShared` for the text and cache-backed files, including what launched the app. [Building](https://nsgo.dev/docs/building.md#incoming-shares-on-android)

Set `apple.share_types: [public.url, public.image]` for the UTIs an iOS share extension accepts. It keeps each share in an App Group inbox until the app is next active, and cannot launch the app. Device runs use Xcode signing; IPA builds need `sign.share_profile` for `<id>.share`, granting the same `apple.app_group` as the app. [Building](https://nsgo.dev/docs/building.md#incoming-shares-on-ios)

## Secrets

Never commit secret keys or passwords. The tool reads them from the environment: `NSGO_UPDATES_KEY` for update feeds, `NSGO_KEYSTORE_PASSWORD` and `NSGO_KEY_PASSWORD` for Android keystores, `NSGO_CERTIFICATE_PASSWORD` for a Windows code signing certificate, `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` and `NSGO_NOTARY_ISSUER` for notarizing in CI, `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` and `NSGO_APP_STORE_ISSUER` (falling back to the notary key) for App Store Connect listings, `NSGO_PLAY_SERVICE_ACCOUNT` for Google Play listings, and `NSGO_ROOT_KEY` and `NSGO_RELEASE_KEY` for signed TSX packages. Public keys belong in `nsgo.yaml` and the app's code.

Use `icon.windows` and `icon.linux` for their square PNGs, and `icon.android.png` for an independent Android bitmap; all fall back to `icon.png`. `dev.icon` has the same fields. Apple package minima use quoted native plist values in `apple.info`: `LSMinimumSystemVersion` for macOS, `MinimumOSVersion` for iOS/tvOS/watchOS. Never go below runtime floors: macOS 13, iOS 15, tvOS 17, watchOS 26; App Clips use `app_clip.minimum`. [Building](https://nsgo.dev/docs/building.md).

For an adaptive Android launcher icon, set `icon.android.foreground` and `background` (a PNG or a `"#RRGGBB"` color), and `monochrome` for Android 13's themed icons: square 432 px layers with the logo inside the center 264 px, which every launcher mask keeps ([Building](https://nsgo.dev/docs/building.md)).

## Permissions and platform entries

Use `cx.Download` in standalone iOS and macOS apps, and on Android, enabling `android.background_downloads: true` on Android. Embedded Apple views have no transfer host. Start transfers from visible app actions and listen with `cx.OnDownloadUpdate`. iOS uses a background URLSession. Re-register the listener at launch to receive saved results. For a self-signed public hostname on Apple, configure its per-domain ATS exception in `apple.info` and supply the exact `CertificatePEM` pin. [Services](https://nsgo.dev/docs/services.md#background-downloads)

Set `android.uses_cleartext_traffic: true` for plain HTTP to any host in an APK or App Bundle; it overrides loopback-only mode. Set `android.loopback_cleartext: true` to allow HTTP only from `localhost`, `127.0.0.1` or `::1` in a standalone APK or App Bundle. Set the host app’s network security policy for an embedded AAR.

Ask for a permission where the app uses it, through `cx`'s permission calls, and explain it in `apple.info`'s usage descriptions; on Android, `android.permissions` adds manifest permissions beyond the runtime's. An app calling `cx.SignInWithApple` sets `apple.sign_in_with_apple: true`, which signs it with the entitlement the sheet needs, one used beside other apps on an iPad sets `apple.requires_full_screen: false`, for Split View, Slide Over and Stage Manager, and lays out at whatever size `Env` reports; and one selling in-app purchases sets `apple.purchases: true`, which bundles StoreKit's library; it shows the store's `DisplayPrice`, never a price of its own, has its server verify a transaction's `Signed` JWS before delivering it, finishes it with `cx.FinishTransaction` (`core.PurchaseNotFound` when already finished, even if still entitled), and registers `cx.OnPurchaseUpdate` as it starts. Check `cx.Env()` (`CanRecognizeSpeech`, `CanSpeak`, `CanPurchase`, `CanRequestReview`, `HasWebView` and the rest) before using a service a target may lack, rather than testing the target's name. For `cx.RequestReview()`, standalone Android packages include Play Review; an embedding Gradle host supplies the [SDK and manifest entries](https://nsgo.dev/docs/services.md#store-reviews). Verify Apple prompts in development builds and Android prompts through Internal App Sharing.

Apple bundles carry a `PrivacyInfo.xcprivacy` declaring the required-reason APIs NSGo uses; declare the app's own under `apple.privacy` (`accessed_apis`, `tracking`, `tracking_domains`, `collected_data`) or in `resources/PrivacyInfo.xcprivacy`, which is merged. A host app linking the static `-embed` library declares NSGo's in its own manifest ([Building](https://nsgo.dev/docs/building.md#store-packages)).

For standalone Android apps that host a custom Android View with `core.Native`, list its precompiled `.dex` or DEX JAR under `android.classes`. NSGo includes those files when packaging an APK or App Bundle. [Building](https://nsgo.dev/docs/building.md#android-native-classes)

## Embedding

`nsgo build -embed -target ios|tvos|android_arm64` packages an app as an XCFramework or an AAR for a host app of your own, `nsgo.NewView` shows one in a Go desktop program, and the web build's loader puts apps in a page. `examples/embed-*` show each host ([Embedding](https://nsgo.dev/docs/embedding.md)).

The web loader shows download progress. Style `progress.nsgo-load-progress` or replace it with `onProgress` in the mount options or the `web.host` module. A release includes smaller Brotli and gzip copies of its wasm for servers configured to negotiate precompressed files.

Windows setup and uninstall programs build with Go alone. Use `/S` for silent installation or removal, a final `/D=<folder>` to choose an installation directory, `/RELAUNCH` to reopen after an update, and `/LANG=<LCID>` to select dialog language for that invocation. Omit `/D` when updating to keep the registered directory. Signing applies to setup and its embedded uninstaller.
