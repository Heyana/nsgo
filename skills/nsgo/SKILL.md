---
name: nsgo
description: Build, run, test and ship cross-platform native apps with NSGo, the Go UI engine whose apps draw with each platform's own views on iOS, Android, macOS, Windows, Linux, the web, tvOS, watchOS and mini programs. Use when writing or reviewing code that imports github.com/nsgo-dev/nsgo, a TSX app on nsgo, nsgo.yaml or locales/ catalogs, nsgotest tests, `nsgo` or `go tool nsgo` commands, or components built on NSGo's base and ui. Excludes changes to NSGo itself.
---

# NSGo

NSGo (`github.com/nsgo-dev/nsgo`) runs an app's logic, reactive graph and layout in Go, and each platform draws it with its own native views. An app is a Go main package whose components return elements; `nsgo` creates, runs, tests and packages it.

NSGo is early and changes quickly, with no stable API yet. Use the latest version, and read the version-matched source before using an API rather than guessing:

```sh
go get github.com/nsgo-dev/nsgo@latest
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo@latest
go list -m -f '{{.Dir}}' github.com/nsgo-dev/nsgo   # the source, docs/public/en and examples/
```

The docs are at <https://nsgo.dev/docs/>; <https://nsgo.dev/llms.txt> lists every page as Markdown, and <https://nsgo.dev/llms-full.txt> holds them all. In the module directory, `examples/gallery` demonstrates every component, `examples/hello` is the smallest app and `examples/tsx` a TSX one; copy their structure rather than inventing one. What is missing is in `docs/roadmap.md`: check it before assuming a feature exists on a target.

Bind translated control words with their `…Func` fields to keep focus and input state. Number/code inputs and spinners accept `LabelFunc`; searchable `ui.Select` has `SearchPlaceholderFunc` and `EmptyFunc`, and form rows/sections have title, detail, label and footer functions. A number input’s `Format` follows signals outside editing. [Forms](https://nsgo.dev/docs/forms.md)

## Create and run

```sh
nsgo init -module example.com/hello hello   # app.go, app_test.go, nsgo.yaml, go.mod pinning this nsgo; -template tsx or -template mixed
cd hello
nsgo dev           # hot reload; -target web, ios_simulator, android, tvos_simulator, watchos_simulator
nsgo doctor        # which targets this machine builds and runs, and what it lacks
```

Commands are written `nsgo X`, the installed command (`brew install nsgo-dev/tap/nsgo`, `curl -fsSL https://nsgo.dev/install.sh | sh` or `go install github.com/nsgo-dev/nsgo/cmd/nsgo@latest`). Where `nsgo` is not on `PATH`, as in CI, run `go tool nsgo X` in the module instead, at the version its `tool` line, which `nsgo init` writes, pins. In an existing module, `nsgo init` adds only the `require` and `tool` lines `go.mod` lacks.

Any command `X` that `nsgo` does not build in runs an extension: the module's `go.mod` tool named `X` with `go tool`, else `nsgo-X` on `PATH`; built-ins win. An `nsgo` on `PATH` runs the module's pinned `go tool nsgo` first when the versions differ (`NSGO_NO_DELEGATE=1` turns it off); `nsgo version` says which ran. [Extensions](https://nsgo.dev/docs/building.md#extensions)

Add libraries with `nsgo add <name|@registry/name|module>[@version]` instead of `go get`, or search NSGo's built-in index and configured registries (inside or outside a module) with `nsgo add -list [@registry] [words...]`: bare names resolve only in the embedded index (`cmd/nsgo/registry.yaml`), while team registries configured under `registries:` in `nsgo.yaml` (`@name` to an `https` URL without userinfo, query or fragment, or a relative YAML path; `@nsgo` is reserved and `nsgo` never auto-adds unconfigured registries) resolve `@<registry>/<name>` and show `registry @<name> (<source>)` on the plan's first line. Both refuse untagged commits and component entries (until component copying lands) and plan the same way as module paths. `nsgo add` prints the plan (any custom registry first, versions, owners, notices, classified module and native-library licenses, each target's native libraries, download hosts and unbuildable packages, the `go.mod` lines and tools that move, plus `nsgo-module.yaml` import packages, unsupported `build.targets`, offered components and author notes printed last) and writes `go.mod` and `go.sum` only once a terminal confirms it or with `-y`; without either it writes nothing and exits 1, and `-dry-run` prints the plan alone. It refuses moving NSGo to a pseudo-version or pre-release, a module needing a newer Go, and any module or native library that fails `nsgo.yaml`'s `licenses` policy, naming the `go get` or `licenses.allow`/`warn` change that admits it on purpose: those are the app author's decisions, so ask before making them. A module that pins NSGo 0.3.4 or earlier needs a newer NSGo first, which the message names. `nsgo check` also warns for each target it checks when a linked module's `nsgo-module.yaml` lacks that target. [Adding libraries](https://nsgo.dev/docs/building.md#adding-libraries), [Module manifest](https://nsgo.dev/docs/authoring/shipping.md#module-manifest)

Use `-template <module>[@version][#<name>]` to scaffold from a module's published [template directory](https://nsgo.dev/docs/getting-started.md#templates-from-go-modules), or `-list` to list a module's templates.

With hot reload, saving a file embedded in a Go-only app rebuilds and restarts it; saving only TSX bundle inputs updates the running app in place. [Running and debugging](https://nsgo.dev/docs/debugging.md#run)

Go 1.27.3 or later builds every target from a Mac, with Xcode for Apple's; nothing else (no NDK, Android SDK, JDK or GTK) is needed to build. Never hand-write C or add cgo to reach a platform: use NSGo's services first, then purego or `syscall`.

For a signed HarmonyOS HAP, configure `harmonyos.signing` and use the SDK, Java and environment passwords described in [Building](https://nsgo.dev/docs/building/#harmonyos).
On `ios_simulator`, Go-only dev rebuilds of the main app reuse the staged bundle, replace the installed executable and relaunch. Changes to the app's resources, icon, launch screen, catalogs, frameworks or `nsgo.yaml` rebuild and reinstall it. [Running and debugging](https://nsgo.dev/docs/debugging.md)
For Android UI work, `nsgo dev -target android -android-host` rebuilds Go on the development machine and reconnects to the installed Activity. Changes to packaged resources, precompiled classes, icons, launch screens, catalogs or `nsgo.yaml` reinstall the APK. Use ordinary Android dev for Go code that needs the device OS. [Debugging](https://nsgo.dev/docs/debugging.md#run)


Use `-host 127.0.0.1` to bind the dev server to loopback only, including with Android's `adb reverse`.

Apps importing `base/terminal` ship libghostty-vt beside Windows/Linux executables, in Apple frameworks and in Android arm64 packages (`arm64-v8a/libghostty-vt.so`, Android 10 / API 29+). An iOS or tvOS `-embed` build also writes `GhosttyVT.xcframework`; hosts add this companion with Embed & Sign for static and dynamic app frameworks alike. An arm64 AAR carries `jni/arm64-v8a/libghostty-vt.so` beside `libnsgo.so`; Android x86_64 terminal libraries are unavailable. [Embedding](https://nsgo.dev/docs/embedding.md) covers the host setup.

Go dependencies can ship prebuilt desktop shared libraries, and with `version: 2` iOS and tvOS frameworks, through their package's `nsgo-natives.yaml`: NSGo verifies checksums, bundles licenses, signs and includes them in installers, `nsgo dev` and `-run` without app configuration. Package authors declare target names, URL (https in version 2) or local files, SHA-256 and license paths, and preserve these files with `go:embed` for vendoring; a framework is an arm64 dylib built for its platform, at most the app's minimum OS, with the install name `@rpath/<Name>.framework/<Name>`. `-embed` stops on a package's iOS frameworks. Version 2's `linux` names, under `linux_x64` and `linux_arm64`, the `depends` and `recommends` (`deb` and `arch` lists, as in nsgo.yaml's `linux`) that the app's `.deb` and Arch package list after the app's own. [Native libraries](https://nsgo.dev/docs/native-libraries.md) gives the contract and loading directories.

For terminal apps, use `nsgo dev -target tui` and `nsgo test -target tui` in an interactive terminal with `nsgotest.Main`. Choose cell-sized styles explicitly (8×16 points per cell); Control is `mod` on every host. Diagnostics go to `.nsgo/tui.log`; media/canvas/native hosted views are not implemented. [Terminal targets](https://nsgo.dev/docs/building.md#terminal)

## Writing an app

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}

func main() { nsgo.Run(Root) }
```

- For touch controls at a phone's screen edge, [hold system gesture deferrals](https://nsgo.dev/docs/services/#system-edge-gestures) for the controls' lifetime.
- `Overflow: core.Hidden` clips children inside the box's border, as CSS clips to the padding box, and preserves the box's own outer shadows and outline on Android, Windows, Linux, the web and Apple ([layout](https://nsgo.dev/docs/layout.md#position)).
- A component runs once. What changes later is driven by signals read inside the functions an element calls (`ContentFunc`, `StyleFunc`, `ClassFunc`, `Disabled`, `When`, `Count`): only those parts update. Reading `count.Get()` while building, outside such a function, takes the value once and never updates.
- A component with props is a type with `Render(*core.Scope) core.Node`. Give it fields whose zero value is the usual case.
- Keep state in signals: `core.NewSignal`, `core.NewMemo`, `core.NewCollection` for keyed items (one row runs when one item changes), `core.NewSelector` for selection, `core.NewHistory` for undo, `core.NewPersisted` for typed values in storage. `cx.Batch` groups writes into one commit.
- `cx.Storage()` reads and writes the app's persistent key-value store. A host creating a secondary root can pass `cx.Store()` to `core.RootOptions.Storage` so both roots share persistence. Custom host persistence implements `core.StorageBackend` for `core.NewStore`, with ordered `Write(core.StorageChange)` calls; see [services](https://nsgo.dev/docs/services.md#storage).
- For scheduled-notification acceptance on Android, wait for `ShowNotification` to answer true, then kill the app’s processes without force-stopping its package; force-stop suppresses alarms until another launch. [Notifications](https://nsgo.dev/docs/notifications.md#scheduling-one)
- A native release keeps crash reports locally. On a goroutine, use `cx.CrashReports()` to list them, `Open()` to read a trace, and `Acknowledge()` to remove a report after processing it ([services](https://nsgo.dev/docs/services.md#crash-reports)).
- Branch with `core.Show`, repeat with `core.Each` or, for long data, `core.List`, which builds only the rows near the screen and recycles cells (`Row` runs once per cell, so read `r.Index()` and item signals inside reactive functions such as `ContentFunc` or `StyleFunc`, not once while building). Catch a panicking subtree with `core.Catch`.
- `desktop.OpenWindow`, `desktop.OpenPanel`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` and `core.NewHolder` belong directly to the `*core.Scope` passed to them rather than the running handler's owner, so passing `app` from a button's `OnPress` keeps them alive after the button's scope goes; reactive primitives (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) belong to the running owner, which `Scope.Run` switches.
- Signals belong to the engine's goroutine. Do I/O on a goroutine and set signals through `cx.Post`, or use `core.NewResource`, whose fetcher runs off the engine with a `context` cancelled when its source changes, under `core.Suspense` for the loading state. Never block a component or a handler on the network or disk; desktop window resizes wait at most 16 ms for the engine's layout ([desktop services](https://nsgo.dev/docs/services.md#speech-and-the-lifecycle)).
- Use scoped `cx.WatchSteps`, `WatchActivity`, `WatchPressure`, `WatchGeofences`, `RequestPreciseLocation` and `WatchBackgroundMotion` for sensor services. Ask for `PermissionActivityRecognition` for fitness (also Apple pressure); keep background motion explicit with a positive duration, separate capability and Android `background_motion` opt-in. Handle unavailable providers and OS expiry ([sensors](https://nsgo.dev/docs/services.md#fitness-regions-and-bounded-motion)).
- Find the user's Documents, Downloads, Desktop, Pictures, Music or Videos folder with `desktop.UserDir`, which macOS, Windows and Linux answer and other targets report `core.ErrUserDirUnavailable`; tests get temporary folders, pinned with `nsgotest.UserDirs`. [The user's folders](https://nsgo.dev/docs/services.md#the-users-folders)
- Use `cx.DiskSpace` for an asynchronous reading of the app’s available storage. On Apple platforms, declare the reason in `apple.privacy.accessed_apis`; tests set the answer with `nsgotest.DiskSpace`. [Disk space](https://nsgo.dev/docs/services.md#disk-space)
- Use `cx.AdvertiseService` and `cx.BrowseServices` for local service discovery on macOS, iOS, Android, Windows and Linux. Set `NSBonjourServices` and `NSLocalNetworkUsageDescription` in `apple.info` for Apple builds. Linux needs the Avahi daemon. Android hosts may need to declare `CHANGE_WIFI_MULTICAST_STATE`. [Services](https://nsgo.dev/docs/services.md#discovering-local-services)
- Use `cx.Env().DeviceModel()` for a hardware model and `DeviceName()` for a user-assigned name; handle empty readings. watchOS, HarmonyOS and mini programs provide a model but keep the name empty ([device identity](https://nsgo.dev/docs/services.md)).
- In mini programs use `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)` with `core.AccessMiniProgramSignIn`, and exchange the returned host/code on your server. It is separate from OAuth `AuthSession`; keep app secrets on the server. Douyin never treats anonymous device codes as signed-in users and prompts for host login only with explicit `Prompt: true`. [Signing in](https://nsgo.dev/docs/services.md#signing-in)
- Read `cx.Env().Network()` for the system's online and metered state. `cx.OnNetworkChange` follows changes for the life of the scope. Handle request errors separately ([Networking](https://nsgo.dev/docs/services.md#networking)).
- Keep tokens, API keys and passwords in `cx.Secrets()`, the platform's keychain or keystore, never in `cx.Storage()`, a plain file; its calls may wait for the system, so make them on a goroutine too.
- On watchOS, `AuthSession` uses the SwiftUI host's system session: supply a callback `Scheme`, or `https` with an associated `Host`; an empty scheme is unavailable. PKCE and state validation stay in the app ([signing in](https://nsgo.dev/docs/services.md#signing-in)).
- `cx.KeepAwake(reason, core.KeepAwakeOptions{Display: true})` keeps a visible Android app or supported web page's display on until release or scope disposal. The web reacquires the lock when the page returns ([platform services](https://nsgo.dev/docs/services.md#keeping-the-display-awake)).
- Group settings with `ui.FormSection` and `ui.FormRow`: `Below`, `TitleMaxLines` and `MinHeight` customize the text column and height; `Role: core.RoleRadio` with `Checked` and `OnPress` uses `base.Radio` inside a `base.RadioGroup`. A section’s `DividerInset` returns each row’s inset and an override flag, with zero allowing full width. [Forms](https://nsgo.dev/docs/forms/#grouped-rows)
- Read `cx.Env().Display()` to follow the current display and its refresh rate. The web rate is an estimate ([platform services](https://nsgo.dev/docs/services.md#displays)).
- Apps use `ui`'s components, which have a look and follow the theme; `base` holds the same controls without a look, for component authors; `mobile` and `desktop` hold what only phones or only desktops have. [references/components.md](references/components.md) maps needs to components.
- On iOS and Android phones, overflowing `core.Scroll` and `ui` scroll views expose touch-draggable handles on either axis; use `nsgotest.App.ScrollHandle` to inspect and scrub them in tests.
- `cx.MeasureText` follows available-font changes in reactive functions. `base/formula.View` and `formula.RenderMath` typeset TeX and Typst; text runs missing from the configured fonts use system fonts automatically, after explicit `formula.Fonts.Fallbacks`. [Text](https://nsgo.dev/docs/text.md)
- A selectable `base/textview.TextView` reports `core.TextSelection.Text` as plain text and its `Markdown` and `HTML` fields with the parsed block and inline formatting preserved; attach `OnTextSelection` to copy or transform the selected content.
- `formula.RenderMath` wraps display math to each `TextView` block's padded viewport at the inherited font size; unbreakable parts scroll. For a width-dependent em canvas, use `Canvas.MeasureEm` without signal writes; `Ref.ContentBox()` tracks a mounted viewport inside padding and borders. [Text](https://nsgo.dev/docs/text.md), [Canvas](https://nsgo.dev/docs/canvas.md)
- `terminal.View` reports requested mouse input and alternate-screen wheel keys. `MouseReporting` is a reactive getter, nil for true; Shift keeps selection available. `MouseScrollMultiplier` controls precision/discrete reports while ordinary scrollback stays native. [Terminal](https://nsgo.dev/docs/terminal.md)
- `base/richeditor` and `ui/richeditor` support touch selection on iOS and Android: a long press selects a word, handles drag the range, and the native edit menu provides copy, cut, paste and select-all actions. Custom input controls use `Ref.OpenEditMenu`, `Input.OnEditAction` and `Input.OnPaste`. Inspect edit menus with the headless `nsgotest` menu helpers, then verify the native toolbar separately. [Text](https://nsgo.dev/docs/text/) and [testing](https://nsgo.dev/docs/testing/)
- Load cues with `audio.LoadSound` and stream longer audio with `audio.NewPlayer` where `Env.CanPlayAudio` reports support. On the web, play after a user gesture ([audio playback](https://nsgo.dev/docs/services.md#audio-playback)).
- Style native navigation titles with `HeaderTitleStyle` and `HeaderLargeTitleStyle` on `base.NavPage`, or their `Func` forms; `HeaderStyle` controls colors. Android keeps the expanded title with a custom compact title and can move trailing controls beside it with `Android.HeaderRightWithLargeTitle`. An embedded Android host enables `android:supportsRtl="true"` for native header direction. [Navigation](https://nsgo.dev/docs/navigation.md)
- Use `Video.OnPause` and `OnPlay` to update the state driving `PausedFunc` when showing your own play button. `Paused` applies when its value changes or a source opens; native playback survives other prop changes. [Videos](https://nsgo.dev/docs/components/)
- Enable `core.Video.PictureInPicture` for iOS or Android, follow `OnPictureInPicture`, and start from a user action through `Ref.StartPictureInPicture`; handle failures even after `Possible`. `AutoPictureInPicture` is explicit and requires a playing video on Android. Stop expands the Android activity; custom `mobile.PictureInPicture` stays iOS-only. Embedded Android hosts forward their Activity’s PiP callbacks. [Picture in Picture](https://nsgo.dev/docs/picture-in-picture.md)
- Use [`core.Video.Headers`](https://nsgo.dev/docs/components/) for authenticated video URLs on Android and direct HTTP files on Linux, macOS, iOS and tvOS.
- An Android embedding host with a `mediaPlayback` foreground service can retain its view and engine and call `NsgoView.setBackgroundVideoPlaybackEnabled(true)` for playback and callbacks when the view is hidden or removed from its window. [Embedding](https://nsgo.dev/docs/embedding.md#android)
- Use `cx.ManageSubscriptions` (TSX `Purchases.manageSubscriptions`) to open Apple's subscription management UI and `cx.RequestRefund` (TSX `Purchases.requestRefund`) for a transaction refund request; both require the purchases capability, and `nsgotest.App.ManageSubscriptionsError`, `nsgotest.App.RefundStatus` and `nsgotest.App.RefundError` drive their outcomes in tests.
- Bind changing names and hints with `LabelFunc` and `HintFunc` so controls keep their focus and state. Every core element with `Label` takes its twin, and `core.View` and `core.Text` take `HintFunc`.
- Tooltips provide their controls' accessibility descriptions before focus, omitting text that repeats the label and preserving an explicit description; their bubbles are hidden from screen readers ([accessibility](https://nsgo.dev/docs/accessibility.md)).
- Raise an engine's sound count or duration before loading with `audio.SetSoundLimits`; a signed TSX package's `nsgo/audio` sounds keep the defaults ([sound limits](https://nsgo.dev/docs/services.md#sound-limits)).
- A desktop app that opens its own windows, panels, tray icons and shortcuts runs `nsgo.Run(App)` with `func App(app *core.Scope)` and opens them with `desktop`. A popup card is a panel with `PanelOptions` `Transparent` and `FitContent` whose content draws its own corners and shadow, and a background where `desktop.Transparent` is false. `PanelOptions.Dock` with `desktop.PanelDock{Edge: core.PlaceBottom}` keeps a panel at a display edge; check `desktop.CanDockPanels` and handle `Window.OnPlacementError` (Wayland needs gtk4-layer-shell and layer-shell). A docked panel keeps its edge when resized and cannot use `ShowAt` or `MoveTo`. `nsgo.StartHidden()` opens the main window hidden, for a tray app or a launch at login, until `cx.MainWindow().Show()`.
- Read `desktop.Tray.ClickBounds()` in `TrayOptions.OnClick` or `Menu` to anchor a custom panel with `Window.ShowAt`. Check its availability: macOS reports the status item's rectangle, Windows the shell's when available, and Linux supplies none. `TrayOptions.Title` appears beside an image or drawn view on macOS.
- Set `desktop.MenuItem.Icon` to `desktop.MenuIcon{SVG: icon}`, encoded `Data` or a local/asset/pixel `Source` for native desktop menu icons. `Template` and monochrome SVGs follow light/dark appearance. [Native menu icons](https://nsgo.dev/docs/services.md#native-menu-icons)

- Use `desktop.SetLaunchAtLogin` for native login registration. Current MSIX builds declare a disabled StartupTask; an external Snap needs app `autostart` metadata. Flatpak status reads are limited to this run's last Background portal reply ([launch at login](https://nsgo.dev/docs/building.md#launch-at-login)).
- Gate `desktop.FrontmostApp(cx)` with `desktop.CanReadFrontmostApp(cx)`. Wayland needs the compositor's optional wlr foreign-toplevel protocol and reports PID 0; packaged Windows IDs are AUMIDs. [Services](https://nsgo.dev/docs/services.md)
- macOS alerts and file pickers show independently when their requesting window is hidden, minimized or offscreen, so tray apps can call the normal `cx.ShowAlert`, `cx.PickFiles` and `cx.SaveFile` APIs. Closing that surface cancels its dialogs.
- Set `core.PickDirectoriesOptions.Persist` to retain read access across restarts via `core.Directory.Token`, `cx.OpenDirectory` and `cx.ReleaseDirectory` (SAF persistable permissions on Android, security-scoped bookmarks on iOS/macOS, paths on Windows/Linux). Use `core.Directory.MediaURI` to stream files inside a picked directory directly in `core.Video` (`content://` on Android, `file://` on desktop/iOS; `core.ErrNoMediaURI` elsewhere). [Photos and files](https://nsgo.dev/docs/services.md#photos-and-files)
- Check `desktop.CanPlaceWindows` before using `desktop.SetWindowFrame` to position a window; it works on macOS, Windows and Linux on X11, while an ordinary Wayland toplevel stays where the compositor places it. `desktop.Frame` follows native moves. Use `WindowFrame{Animated: true}` to ease a visible panel on macOS or Windows; reduced motion applies its frame immediately.
- Set app icon actions with `desktop.SetDockMenu`: Linux needs an installed desktop entry matching `id` and a launcher supporting Unity quicklists. Record documents with `desktop.AddRecentFile` and clear the app's history with `desktop.ClearRecentFiles` ([file types](https://nsgo.dev/docs/building.md#file-types)).
- Use `desktop.WindowOptions.Parent` when a window should close with another window of the same app. On desktops it stays above its parent and follows its visibility; an explicitly hidden child stays hidden on parent restore. On macOS it also starts centered over its parent when `Frame` is unset and follows its full screen.
- Build an `AuthSession` authorization URL with the exact redirect passed to its function; web and desktop loopback redirects normalize their paths. Keep PKCE and state validation in the app ([signing in](https://nsgo.dev/docs/services.md#signing-in)).
- Use `cx.SetImageCacheLimit(bytes)` to bound retained image bytes (32 MiB by default); `cx.KeepImage` pins an atlas until release or scope disposal. This does not cap decoded pixels or live draws ([images](https://nsgo.dev/docs/images.md)).
- Set `HitTestAlpha` (TSX `hitTestAlpha`) above 0 for transparent pixels to pass the pointer through painted windows. Images support it on macOS and Windows. Untransformed videos without controls support it on macOS. [Services](https://nsgo.dev/docs/services.md)
- For an iOS App Clip handoff, configure `app_clip` and explicit `apple.app_group`: clip and full app share `cx.DataDir()` and `cx.Storage()`, with separate caches. Both signing profiles must grant the group. Enabling it does not migrate old sandbox data; an explicit host data directory still wins. [App Clips](https://nsgo.dev/docs/building.md#app-clips)
- Declare `file_types` for documents opened on macOS, iOS, Windows and Linux and handle them with `cx.OnOpenFiles`. Windows and Linux scan existing regular files and local file URLs only before `--`, including on a second launch. Use `nsgo.LaunchFiles(parse)` to apply your CLI's rules and return paths, or `nsgo.LaunchFiles(nil)` to opt out. iOS delivers temporary imported copies; move them to `cx.DataDir()` to retain them. App Clips and tvOS register none. [File types](https://nsgo.dev/docs/building.md#file-types)
- Gate `desktop.QuickLook` on `desktop.CanQuickLook`: macOS uses its panel, Windows registered file preview handlers, and Linux an available GNOME Sushi service. A true return means the request was sent; formats depend on the previewer. Linux arrow selection and owner-close require Sushi ownership signals and X11. [Desktop services](https://nsgo.dev/docs/services.md)
- Use `cx.OpenPermissionSettings` for sensor permissions: macOS opens Location Services or Motion & Fitness; iOS and Android open the app's settings. Opening settings does not grant access ([permissions](https://nsgo.dev/docs/permissions.md)).
- Before an app's first connection to a device on the local network, such as pairing with a desktop, ask for `core.PermissionLocalNetwork` with `cx.RequestPermission`: iOS 14+ and macOS 15+ fail LAN connections until the user allows them and never tell the status, so asking probes with Bonjour and waits for the user, while a check answers unknown until a request has. Give Apple builds `NSLocalNetworkUsageDescription` in `apple.info`, with which `nsgo build` adds NSGo's probe type to `NSBonjourServices`. Other platforms answer not gated ([permissions](https://nsgo.dev/docs/permissions.md#the-local-network)).
- Use `cx.ScanCode` on macOS, iOS and Android. Gate the action on `cx.Env().CanScanCode()`, and give Apple builds an `NSCameraUsageDescription` in `apple.info` or Android builds `CAMERA` in `android.permissions`. [Photos and files](https://nsgo.dev/docs/services.md#photos-and-files)
- `desktop.SetFullScreen` takes a display ID from `cx.Env().Displays()` on desktop. On the web enter from a press, and read `cx.Env().FullScreen()` to follow exits ([platform services](https://nsgo.dev/docs/services.md#full-screen)).
- Use `desktop.SetWindowConstraints` or `WindowOptions.Constraints` for size and window-button limits. Linux supports nonfixed maximums on X11 and native button restrictions on both X11 and Wayland; Wayland upper bounds still require fixed sizes. [Desktop services](https://nsgo.dev/docs/services.md)
- `desktop.TrayOptions.View` draws a live tray icon on macOS and Windows. Windows rasterizes the whole view; use `OnClick`, `Panel` and `Menu` on the tray options for its actions. Linux drawn tray views remain a goal.
- A window that lets the pointer through to the windows below sets `PointerEvents` in `desktop.WindowOptions` or `desktop.PanelOptions`, or calls `desktop.SetWindowPointerEvents`: `desktop.WindowPointerPainted` takes it only over views that draw or hear it, as around a desktop pet; `desktop.WindowPointerNone` takes none, so the app gives it back from a shortcut or a tray icon. `Desktop.WindowDrag` works inside a pressable ancestor; a nearer pressable or editing control keeps its input.
- Carry sign-in cookies into web views with `cx.SetWebCookies`, and wait for its callback before loading. To remove one, write its same name, domain and path with a past `Expires`; zero means a session cookie. Linux rejects public-suffix domains before writing any cookie, and Android shares cookies with Incognito views and lists only URL-filtered names and values. [Cookie store](https://nsgo.dev/docs/services/)
- Set `core.WebView.Media` to `core.WebMicrophone`, `core.WebCamera` or both for page capture, and declare the devices in `apple.info` or `android.permissions`. Android refuses pending requests when their device is removed, their page leaves or the view closes; pages must request again. [Web views](https://nsgo.dev/docs/components/)
- Android `cx.AuthSession` uses a Custom Tab or browser with an app-owned `AuthSessionOptions.Scheme` declared in `nsgo.yaml` `url_schemes`; the app still validates OAuth state and exchanges its PKCE code. Empty/HTTPS/undeclared callbacks are unavailable. Android `Ephemeral` is a browser-dependent request, not a cookie-isolation guarantee. [Signing in](https://nsgo.dev/docs/services.md#signing-in)
- Use `core.WebView.MutedFunc` (TSX `muted`) to change sound in a mounted web view. Windows and Linux mute the page, and macOS 12 and iOS 15 or later suspend its media. The setting follows navigations.
- For a panel triggered by a screen edge, sample the global pointer with `desktop.CursorPoint` and compare it with `desktop.Monitors`. Check the availability result, which is false on Wayland. [Desktop services](https://nsgo.dev/docs/services.md)
- Check `Env.CanLockPointer` and call `Ref.LockPointer` from a press for a camera or drag that needs relative motion. Handle `View.OnPointerDelta` and keep the returned unlock function. Escape and loss of window focus also release it.
- Loop a sound with `audio.Voice{Loop: true}`. For a music intro plus repeats, set `audio.PlayerOptions.LoopStart` and call `SetLoop(true)` ([looping audio](https://nsgo.dev/docs/services.md#looping-audio)).
- A desktop pet, widget or HUD opens its window with `desktop.WindowOptions{Transparent: true}` (the main window with `nsgo.WindowTransparent()`), leaves its root view without a background, and moves it by a view's `Desktop.WindowDrag`; `NoShadow` and `SkipTaskbar` drop the shadow and the taskbar button, and `desktop.CanMakeWindowsTransparent` and `desktop.CanSkipTaskbar` say whether they take effect, as on X11 without a compositor or on Wayland they do not. A window with the system's title bar keeps its frame and shadow on Windows, as one the window manager decorates does on Linux.
- A library's own audio engine on iOS, such as an `AudioQueue`, holds the shared audio session with `audio.HoldSession(core.AudioSessionPlayback)` while it plays and calls the release when it stops, never setting the `AVAudioSession` category itself ([audio](https://nsgo.dev/docs/services.md#audio)).
- Schedule sounds with `audio.Now(cx)` and `audio.Voice.StartAt` where `audio.CanSchedule(cx)` is true ([audio timing](https://nsgo.dev/docs/services.md#audio-timing)).
- Route from tracked `cx.AppURL()`; `cx.SetAppURL(url, core.AppURLOptions{History: core.HistoryPush})` requests a browser entry and `cx.OnAppURLChange` observes browser traversals. TSX uses `UrlScheme.url/setUrl/on`. For deep paths, set YAML `web.base` to the deployed directory and configure the server's app-entry fallback. [Application addresses](https://nsgo.dev/docs/addresses.md)
- TSX apps embed their sources and run `compiler.App(src, "App.tsx")`, importing `nsgo`, `nsgo/base` and `nsgo/ui`; Go components and functions reach TSX through `tsx.Register` and `tsx.RegisterFunctions` ([TSX apps](https://nsgo.dev/docs/tsx.md)).
- For short effects, use `audio.LoadSound` and `Sound.Play`. macOS, iOS, Windows, Linux, Android and the web return a playback handle with independent volume, rate, stop and reactive end state. Use `Voice.Muted` or `Player.SetMuted` to start silently without pausing. [Audio](https://nsgo.dev/docs/services.md#audio)
- `core.PanEvent` supplies `Position` in the view and `PagePosition` in its page on every phase.
- For function plugins, set `Sandbox.OnError` to attribute uncaught callbacks and native slice-budget cuts outside a waiting call. The handler runs on the engine goroutine and can open a breaker with `Plugin.Interrupt`; entry and call-result failures still return to their callers, and views use `core.Catch`. [TSX](https://nsgo.dev/docs/tsx/)
- A native `Plugin.Call` reports `tsx.ErrBudget` when `Limits.Call` cuts its JS, even after `await`. `Limits.CallTotal` also counts time spent waiting. Hosts can stop the plugin with `Plugin.Interrupt`, read `Plugin.Stopped()` and `Plugin.Err()` from any goroutine, or set `Sandbox.OnStop` for a notification on the engine's goroutine. Sandboxed backtracking regexps use the shorter load or call budget as their match timeout. For host deadline extensions, view interruption, native `JSTime` and owned engine work through `ViewRuntime.EngineTime`, see [TSX](https://nsgo.dev/docs/tsx/).
- Set `Sandbox.DisableStorage` to deny a plugin NSGo `Storage` access without deleting its data. On the web, sandboxed packages isolate in an opaque-origin iframe over a `MessagePort` bridge by default; do not use `WebUnisolated` for untrusted code. [Signed packages](https://nsgo.dev/docs/tsx.md#signed-packages)
- In a native sandbox `JSON.stringify`, `JSON.parse` with a reviver, `Array.prototype.join`, `toString`, `toLocaleString` and `flat` throw a catchable `RangeError` past 1,000 levels, counted across nested calls, instead of hanging or overflowing the host's Go stack.
- A native sandbox uses 1,000 frames with `DefaultLimits` and caps `Limits.Stack` at 10,000. Nonpositive Go values use the cap.
- Native functions calling each other (built-in getters, `toString`/`valueOf`, `RegExp` `exec` dispatch, `Reflect.apply`/`construct`, bound-function and proxy chains) count against `Limits.Stack` and throw a catchable `RangeError`, and exporting a plugin value nested deeper fails with a `RangeError`, through NSGo's goja fork `github.com/nsgo-dev/goja`, which apps get with NSGo without a `replace` of their own.
- `Limits.Memory` budgets metered native storage, including array copies and spread, string concatenation and replacement, and typed-array buffers. The browser uses its own heap. [TSX apps](https://nsgo.dev/docs/tsx.md)
- Register a getter's nonzero default with `tsx.Default("Max", float64(100))` so forwarded undefined props keep that default as they change.
- Publish system media controls with `Scope.SetNowPlaying` and handle them with `Scope.OnMediaCommand` where `Env.CanShowNowPlaying` reports support. For playback state and test steps, see [Now Playing](https://nsgo.dev/docs/services.md#now-playing).
- Gate `gameservices` (`Authenticate`, `ReportAchievement`, `SubmitScore`, `LoadScores`, `ShowAchievements`, `ShowLeaderboard`) on `cx.Env().CanUseGameServices()`. On Android, package `com.google.android.gms:play-services-games-v2` and declare `com.google.android.gms.games.APP_ID` in `AndroidManifest.xml`; configure partial-progress achievements as 100-step incremental achievements ([game services](https://nsgo.dev/docs/services.md#game-services)).

## Layout and styles

- Layout is flexbox with React Native's defaults (a column, children keep their size) plus CSS grid. Sizes take `core.Pt` or `core.Pct`; text shrinks and wraps in a row only with `FlexShrink: 1`, an unset minimum being 0 already (`core.AutoMinimum` is CSS's automatic minimum).
- A screen that fills the window takes `FlexGrow: 1, FlexShrink: 1`, as does every box between it and a list or scroll view that should scroll; with `FlexGrow` alone a long list stretches past the window instead of scrolling.
- Let a scroll view fill its container and put the padding on its content, so the scroll bar stays against the edge. Scroll only through `core.Scroll`, `core.List` or `ui`'s scroll views, never by stepping offsets from pointer or wheel events; they keep native inertia and edges.
- Set `core.Scroll.NoShrink` when a scroll view must keep its requested size while a sibling shrinks.
- Highlight text with a span’s `Background`, `CornerRadius`, `BackgroundInsetTop` and `BackgroundInsetBottom`, including HarmonyOS and mini programs; insets change only paint ([text](https://nsgo.dev/docs/text.md)).
- Use `core.Span.Attachment` to place a view in a line of text. Its first text line supplies the default baseline.
- Keep content clear of the notch and the keyboard with `PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvKeyboard}` rather than fixed insets.
- `core.Style` is typed CSS: zero fields are unset and `Merge` composes styles; `core.Zero` sets a length to none over another style's (`Padding: core.All(core.Zero)`), as `core.Transparent` does a color. `Class` takes Tailwind CSS 4 classes, responsive (`md:`), appearance (`dark:`), pointer (`pointer-fine:`, `pointer-coarse:`) and state (`hover:`) variants included; a `Style` wins over a class.
- Set `core.Style.GlassEffect` (`core.GlassRegular` or `core.GlassClear`) for Liquid Glass (with a fallback blur in `core.Style.BackdropFilters`), and `core.Style.GlassInteractive` (`glassInteractive` in TSX) to turn on the system's press response on macOS 27 and iOS 26.
- Take colors from the theme, never fixed hex values in a component: `ui.ColorsOf(cx)` inside a `StyleFunc` follows theme and appearance changes in place. Unset text and `currentColor` icons follow the subtree's `Palette.Text`; explicit colors keep inheriting. `cx.SetTheme` themes a subtree; `core.ParseTheme` reads a YAML or shadcn/ui theme.
- Do nothing implicitly by device. Choose sizes and forms for a phone or a desktop explicitly, from the window's width, `pointer-fine:` / `pointer-coarse:` classes or the target, not by a component changing on its own.
- Leave the pointer's arrow alone and give lists, tables and sidebars a selected look without hover or press: NSGo apps look native, not like web pages, on the web too.

For sandboxed Mac App Store widgets, provide `sign.macos_profile` and a macOS `sign.widgets_profile` for the app and `<id>.widgets`, from the same team granting `apple.app_group` (default `group.<id>`). Both timelines and images use the shared container; `-installers=false` still signs the bundles. [Widgets](https://nsgo.dev/docs/widgets.md)

## App Clips

An App Clip can request its full app's StoreKit recommendation from an explicit user action with `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` (or `AppClipOverlayBottomRaised`). Keep the returned dismissal function if needed; scope cleanup releases the request. It does nothing outside a native iOS App Clip and reports no offer/installation outcome. [App Clips](https://nsgo.dev/docs/building.md#app-clips)

## Words and configuration

- Put every user-visible word in `locales/`, a YAML catalog per language in ICU MessageFormat beside `nsgo.yaml`, and call the generated functions (`locales.Greeting(cx, "Ada")`), never string literals. `dev`, `build` and `test` check catalogs and regenerate the functions; `nsgo dev -lang en-XA` finds untranslated and cut text, `-lang ar-XB` right-to-left layout.
- Missing dependency translations warn by key for partial coverage. Use `i18n: { coverage: off }` in `nsgo.yaml` only when fallback to the dependency's source language is intentional. [Localization](https://nsgo.dev/docs/localization.md)
- Write configuration people edit by hand in YAML, not JSON: `nsgo.yaml`, catalogs and themes.

Declare `url_schemes` for private app URLs and `app_links` for explicit HTTPS DNS hosts (no scheme, path, port, IP or wildcard). On iOS, `mobile.OpenedURL(cx)` tracks cold and warm scheme/universal invocations; schemes still reach `mobile.OnOpenLinks`. Full-app signing claims `applinks:` domains, while App Clips keep their own `appclips:` list. The server must publish AASA and the profile must authorize Associated Domains; simulator injection does not prove association. [Links](https://nsgo.dev/docs/building.md#links)

## Terminal

`base/terminal.View` runs a session the app owns and closes, and `ui/terminal` (`terminal.View`) styles its default context menu and touch key bar in `ui`'s look. `Keybinds` overrides focused, noncomposing input defaults with clipboard, selection, clear and zoom actions; `Ref.FontSize` is tracked and `SetFontSize` keeps the session. `RightClickAction` defaults to the localized context menu; use `RightClickIgnore` with an app-owned menu. Import `tsx/terminal` for reactive `keybinds` and `rightClickAction` in `nsgo/base`. [Terminal](https://nsgo.dev/docs/terminal.md)

## Web documents

Use `web.Head` (`github.com/nsgo-dev/nsgo/web`) for a page's title and description, or `Head` from `nsgo/web` in TSX. It follows the visible owner's lifetime and renders no layout node. `RoleHeader` and `AccessibilityLevel` give views and texts semantic headings; `base.Link` and `ui.Link` publish browser addresses with native middle-click behavior. Custom links use `core.View.Web` with `core.WebViewProps.Href` or `HrefFunc`. Keep the app's styles and pointer behavior. [Publishing on the web](https://nsgo.dev/docs/web.md) covers metadata inheritance, confined content and CDN loading.

## Tests and checks

Run the tool, never `go vet`, `go test`, `go build` or `go run` directly: it type-checks TSX, generates catalogs and vets build tags for every target.

Use `app.Surface("title:Settings")` for the same view queries, input steps, resizing and waits in a secondary desktop window; its `Post` schedules through the app, and closed handles fail. Keep app-wide service fixtures on the original `App`. [Testing](https://nsgo.dev/docs/testing.md)

```sh
nsgo check                 # vet; -target web,android vets those targets instead, all every available one
nsgo test                  # headless; -run as go test's, -target macos|ios|ios_simulator|android|web|all on a platform
nsgo test . ./cmd/...      # the packages the patterns name, as go test's: a directory is its own package, /... its tree
nsgo test ./pkg -fuzz FuzzX -fuzztime 3s  # headless fuzzing of ./pkg only, not its subpackages
```

Vetting and tests use the same Go JSON policy as `build`. Headless `nsgo test -- -c` compiles tests without running them; compiler failures remain visible in the console and JUnit report.

Name every target a change touches in one `check -target`, with `-jobs n` to vet up to n at once, each `go vet` with n compile workers; never start several checks, as a check waits for the one already running in its module. Reports name expanded targets (`linux_arm64`, `linux_x64` for `linux`). A target with no matching Go packages is skipped with a reason; if every target is skipped, the check fails.

Test an app as a user uses it with `nsgotest`, finding views by text, role, label or test ID. Read [references/testing.md](references/testing.md) before writing tests.

## Debugging

On Apple targets, the default Go logger writes public unified-log entries under the running bundle ID and category `nsgo`, including packaged launches without a terminal. Keep secrets out of logs; custom writers and `slog` handlers own their output. [Packaged logs](https://nsgo.dev/docs/debugging.md#packaged-apple-apps-logs)

Run `nsgo dev -target native -debug` with Delve on `PATH` to attach a debugger to a desktop app. Reconnect to the printed loopback address after a Go rebuild. [Running and debugging](https://nsgo.dev/docs/debugging.md#go-debugger)

`nsgo dev -devtools` opens DevTools (elements, signals, a commit profiler, causal signal-trigger and scope-disposal diagnostics, and a test recorder); `-highlight` outlines what each commit changes, `-trace` prints each commit with the lines whose writes caused it, `-layout` outlines every box and `-release -fps` measures frame and commit cost, including native text round trips on Android. On macOS, Windows, Linux, Android (`-target android`), the web (`-target web`), mini programs (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay`, `miniprogram_douyin`) and Apple devices and simulators (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator`, `watchos_simulator`), `nsgo mcp` lets an agent drive the running app over MCP (`claude mcp add nsgo -- nsgo mcp`): list windows, outline views, press, type, scroll and take screenshots. Use it to check a change on screen instead of assuming it renders. Go apps can add tools with `automation.Register`; see [app-defined MCP tools](https://nsgo.dev/docs/debugging.md#app-defined-mcp-tools). On macOS and Windows a native Go rebuild restores the main window’s last regular frame during the same dev session. Windows also restores full screen, and Linux restores the window's size and full-screen mode. On macOS, Go-only edits update the `.app` executable in place and overlap the old and new windows until the new window draws its first frame. Within a dev session, `base.NewNav` restores JSON-serializable route stacks automatically, and `core.DevKeep` / `core.DevKeepFunc` preserve app signals or custom JSON-serializable state across restarts (compiled out of release builds). Command/Control+R reloads from every open window and remains available after an app function's main view closes.

For a Go automation tool, use `Handler` for an immediate result or `Start` to reply later from a `Scope.Post`, an audio callback or another goroutine. Never wait for graph work inside a handler. Test deferred tools with `automation.CallAsync` inside `app.Post`.

DevTools bounds and `-layout` boxes follow native presentation geometry where the backend reports it; HarmonyOS and mini programs retain Go-side tracking until native feedback is available ([limits](https://nsgo.dev/docs/debugging.md#layout)).

For Go component previews, put `PreviewXxx(*core.Scope) core.Node` functions in an importable package under `//go:build nsgopreview`. Run `nsgo dev -preview ./components` or add `-target web`. [Component previews](https://nsgo.dev/docs/debugging.md#component-previews)

Use `nsgo dev -profile cpu` or `-profile heap` for a Go profile, and `-profile trace` for a scheduler trace. The host writes them under `.nsgo/profiles` unless `-profile-output` sets a path. On the web, capture the heap or an execution trace. [Runtime profiles](https://nsgo.dev/docs/debugging.md#runtime-profiles)

## Shipping

Linux Arch packages include `.MTREE`; check installed files with `pacman -Qkk`. The [contribution guide](https://nsgo.dev/docs/contributing.md#check-and-test) describes optional package-manager acceptance in disposable containers.

`nsgo build` packages every target `nsgo.yaml` lists into `dist/`, installers and store packages into `dist/releases`. Read [references/shipping.md](references/shipping.md) before editing `nsgo.yaml`, icons, signing, updates or embedding. Use `updater.CapabilityOf` before offering update controls, and read `updater.Status` inside bindings for state and persisted preferences; TSX apps import `checkForUpdate`, `capability`, `status` and preference setters from `nsgo/updater`.

When using `sign.sign_command`, return success after writing each PE output's embedded Authenticode signature. [Windows signing](https://nsgo.dev/docs/building.md#sign-windows-packages)

For store ratings, call `cx.RequestReview()` where `cx.Env().CanRequestReview()` is true. Standalone Android packages include Play Review; embedded hosts supply it through Gradle. [Store reviews](https://nsgo.dev/docs/services.md#store-reviews)

Use `nsgo publish ios` with a signed IPA or `nsgo publish macos` with a signed PKG to upload to TestFlight. [App Store Connect setup](https://nsgo.dev/docs/building.md#app-store-connect)

Declare independently delivered files in `mobile.asset_packs`. Use `mobile.AssetPackReady` and `mobile.OpenAssetPackFile` for files in available packs. [Store asset packs](https://nsgo.dev/docs/building.md#store-asset-packs)

For Android embedding, an explicit `NsgoEngine` keeps the app alive across `NsgoView.detachFromEngine()` and same-view `attachToEngine()`: native widgets are recreated from the live graph. The engine still retains its first view and Activity and refuses replacements or additional views. Call `destroy()` for shutdown. [Embedding](https://nsgo.dev/docs/embedding.md#android)

## Reusable components

A library of components follows NSGo's own split between behavior in an unstyled form and a look over it. Read [references/authoring.md](references/authoring.md) before writing components for other apps.
