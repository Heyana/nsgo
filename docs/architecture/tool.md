# The nsgo tool

`cmd/nsgo` builds, runs, tests and packages apps from their `nsgo.yaml`, shelling out to `go` and, for Apple targets, Xcode's tools. It follows NSH's `nsh-cli` (`nsh/docs/dev/tooling.md` and `embedding.md`). Read this before changing `cmd/nsgo`.

## Dev loop

`nsgo dev` (`dev.go`) starts a dev server on `-port` (8080), or the next free port, listening on every interface. Dev builds take the `nsgodev` tag, which links `nsgo`'s dev client (`dev_client.go`): it follows the server's `/_nsgo/events` and shows a failed build's output in an overlay over the running app, as it shows the app's own render failures; Command or Control+R posts `/_nsgo/reload`.

The server finds the app through `NSGO_DEV_SERVER`, which the simulator gets as `SIMCTL_CHILD_`; Android and iPhones, which pass no environment, get it with `-ldflags -X`, Android at `127.0.0.1` through `adb reverse`, removed when the run ends, an iPhone at the Mac's LAN address.

The web target is served from the same server; its page shows the overlay itself and loads each build. `r` and Return in the terminal build again too. Without hot reload, a run ends with the app; a failed first build ends it. `-release` builds as `nsgo build` does, keeping `dev.fps`'s monitor, for measuring. `-device` picks a simulator by name or UDID, booting it, or an adb serial or an emulator's AVD name.

## Tests

`nsgo test` (`test.go`) runs `go test -json` and prints a line per test, a failure's output under it, a tally, and with `-junit` a JUnit report, a suite per package. `-run` filters; `-trace` sets `NSGO_TEST_TRACE`, under which `nsgotest` prints each batch under its step with its items and the commit's time.

`-target` runs the tests on a platform, a process per test ([testing](testing.md#on-a-platform)): macOS, Windows and Linux on this machine, the web in a headless Chrome, Chromium or Edge it finds (or `NSGO_BROWSER`) over the DevTools protocol (`cdp.go`, on `x/net/websocket`), the simulator through `simctl launch`, and Android through `am start` with the test in an intent extra and its output read back from the device's log.

## Packages

`nsgo build` strips symbols, paths and VCS stamps (`nsgo dev` keeps symbols unless `-release`) and writes a directory per target under `build.output`:

- macOS: `<name>.app`, signed ad hoc. iOS: `<name>.app` and `<name>.ipa`, signed with `sign.profile`, else unsigned. Android: `<name>.apk`, signed with `sign.keystore`, else the project's debug key.
- Windows and Linux: the executable with `resources/`.
- With `-embed`, for apps of their own to embed ([embedding](embedding.md)): iOS's `<module>.xcframework` with `<name>Resources`, Android's `<name>.aar`, and no package for other targets.
- Web: `index.html`, `manifest.webmanifest` and every other file named by its content (`app.<10 hex>.wasm`, `resources.<hash>/`, which the page tells the runtime as `nsgoResources`), so hosts cache them for good.

`resources/` ships in each package's resource root, which `imageload.ResourceRoot`, the web backend and Android's APK reader resolve `asset:` and font paths against. App icons are drawn in Go; `actool` compiles Apple's `.icon` files, and a Windows icon links from a `.syso` in the app's package directory, the only place Go links one from.

`build_number` sets `CFBundleVersion` and `versionCode`, else they follow `version`. `appearance` starts the app light or dark through `-X`, and sets `UIUserInterfaceStyle`, macOS's Aqua appearance, Android's activity theme and the page's `color-scheme`. `-run` starts the package of the first target this machine runs: the app, the iPhone's, the simulator's, the Android device's, or the site served with a CDN's cache headers. `-device` picks the device as `nsgo dev`'s does; Xcode signs the iPhone's package again for `-team`, as in the dev loop, since devices install only development builds.

## Store signing

`sign.profile` signs the iOS package: `security cms` opens the profile, whose `application-identifier` must cover the bundle ID; a wildcard profile signs the app's own ID, which devices require. The profile goes in as `embedded.mobileprovision`, and `codesign` signs with `sign.identity`, else the keychain's identity for one of the profile's certificates.

`sign.keystore` signs release APKs (`apk/keystore.go`): PKCS#12, with PBES2 (PBKDF2 and AES or 3DES) as keytool and OpenSSL 3 write it or PKCS#12's SHA-1 schemes with 3DES and 40-bit RC2 as older tools do, or JKS, whose key protector is a SHA-1 stream of the password. `NSGO_KEYSTORE_PASSWORD` and `NSGO_KEY_PASSWORD` give the passwords, never `nsgo.yaml`; `sign.key_alias` picks a key. APKs sign with RSA keys only.

## Verification

`go test ./cmd/nsgo/apk` checks the APK and, when `apksigner` is installed, its signature.
