---
name: nsgo
description: Build, run, test and ship cross-platform native apps with NSGo, the Go UI engine whose apps draw with each platform's own views on iOS, Android, macOS, Windows, Linux, the web, tvOS, watchOS and mini programs. Use when writing or reviewing code that imports github.com/nsgo-dev/nsgo, a TSX app on nsgo, nsgo.yaml or locales/ catalogs, nsgotest tests, `go tool nsgo` commands, or components built on NSGo's base and ui. Excludes changes to NSGo itself.
---

# NSGo

NSGo (`github.com/nsgo-dev/nsgo`) runs an app's logic, reactive graph and layout in Go, and each platform draws it with its own native views. An app is a Go main package whose components return elements; `go tool nsgo` creates, runs, tests and packages it.

NSGo is early and changes quickly, with no stable API yet. Use the latest version, and read the version-matched source before using an API rather than guessing:

```sh
go get github.com/nsgo-dev/nsgo@latest
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo@latest
go list -m -f '{{.Dir}}' github.com/nsgo-dev/nsgo   # the source, docs/public/en and examples/
```

The docs are at <https://nsgo.dev/docs/>; <https://nsgo.dev/llms.txt> lists every page as Markdown, and <https://nsgo.dev/llms-full.txt> holds them all. In the module directory, `examples/gallery` demonstrates every component, `examples/hello` is the smallest app and `examples/tsx` a TSX one; copy their structure rather than inventing one. What is missing is in `docs/roadmap.md`: check it before assuming a feature exists on a target.

## Create and run

```sh
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo@latest
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo@latest
go tool nsgo init          # app.go, app_test.go, nsgo.yaml; -template tsx or -template mixed
go tool nsgo dev           # hot reload; -target web, ios_simulator, android, tvos_simulator, watchos_simulator
go tool nsgo doctor        # which targets this machine builds and runs, and what it lacks
```

Use `-template <module>[@version]` to scaffold from a module's published [template directory](https://nsgo.dev/docs/getting-started.md#templates-from-go-modules).

Go 1.27.3 or later builds every target from a Mac, with Xcode for Apple's; nothing else (no NDK, Android SDK, JDK or GTK) is needed to build. Never hand-write C or add cgo to reach a platform: use NSGo's services first, then purego or `syscall`.

For a signed HarmonyOS HAP, configure `harmonyos.signing` and use the SDK, Java and environment passwords described in [Building](https://nsgo.dev/docs/building/#harmonyos).
On `ios_simulator`, Go-only dev rebuilds of the main app replace the installed executable and relaunch. Changes to the app's resources, icon or `nsgo.yaml` reinstall it. [Running and debugging](https://nsgo.dev/docs/debugging.md)

Use `-host 127.0.0.1` to bind the dev server to loopback only, including with Android's `adb reverse`.

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
- A component runs once. What changes later is driven by signals read inside the functions an element calls (`ContentFunc`, `StyleFunc`, `ClassFunc`, `Disabled`, `When`, `Count`): only those parts update. Reading `count.Get()` while building, outside such a function, takes the value once and never updates.
- A component with props is a type with `Render(*core.Scope) core.Node`. Give it fields whose zero value is the usual case.
- Keep state in signals: `core.NewSignal`, `core.NewMemo`, `core.NewCollection` for keyed items (one row runs when one item changes), `core.NewSelector` for selection, `core.NewHistory` for undo, `core.NewPersisted` for typed values in storage. `cx.Batch` groups writes into one commit.
- `cx.Storage()` reads and writes the app's persistent key-value store. A host creating a secondary root can pass `cx.Store()` to `core.RootOptions.Storage` so both roots share persistence.
- For scheduled-notification acceptance on Android, wait for `ShowNotification` to answer true, then kill the app’s processes without force-stopping its package; force-stop suppresses alarms until another launch. [Notifications](https://nsgo.dev/docs/notifications.md#scheduling-one)
- A native release keeps crash reports locally. On a goroutine, use `cx.CrashReports()` to list them, `Open()` to read a trace, and `Acknowledge()` to remove a report after processing it ([services](https://nsgo.dev/docs/services.md#crash-reports)).
- Branch with `core.Show`, repeat with `core.Each` or, for long data, `core.List`, which builds only the rows near the screen and recycles cells (`Row` runs once per cell, so read `r.Index()` and item signals inside reactive functions such as `ContentFunc` or `StyleFunc`, not once while building). Catch a panicking subtree with `core.Catch`.
- `desktop.OpenWindow`, `desktop.OpenPanel`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` and `core.NewHolder` belong directly to the `*core.Scope` passed to them rather than the running handler's owner, so passing `app` from a button's `OnPress` keeps them alive after the button's scope goes; reactive primitives (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) belong to the running owner, which `Scope.Run` switches.
- Signals belong to the engine's goroutine. Do I/O on a goroutine and set signals through `cx.Post`, or use `core.NewResource`, whose fetcher runs off the engine with a `context` cancelled when its source changes, under `core.Suspense` for the loading state. Never block a component or a handler on the network or disk.
- Use `cx.DiskSpace` for an asynchronous reading of the app’s available storage. On Apple platforms, declare the reason in `apple.privacy.accessed_apis`; tests set the answer with `nsgotest.DiskSpace`. [Disk space](https://nsgo.dev/docs/services.md#disk-space)
- Use `cx.AdvertiseService` and `cx.BrowseServices` for local service discovery on macOS, iOS and Linux. Set `NSBonjourServices` and `NSLocalNetworkUsageDescription` in `apple.info` for Apple builds. Linux needs the Avahi daemon. [Services](https://nsgo.dev/docs/services.md#discovering-local-services)
- Read `cx.Env().Network()` for the system's online and metered state. `cx.OnNetworkChange` follows changes for the life of the scope. Handle request errors separately ([Networking](https://nsgo.dev/docs/services.md#networking)).
- Keep tokens, API keys and passwords in `cx.Secrets()`, the platform's keychain or keystore, never in `cx.Storage()`, a plain file; its calls may wait for the system, so make them on a goroutine too.
- `cx.KeepAwake(reason, core.KeepAwakeOptions{Display: true})` keeps a visible Android app or supported web page's display on until release or scope disposal. The web reacquires the lock when the page returns ([platform services](https://nsgo.dev/docs/services.md#keeping-the-display-awake)).
- Group settings with `ui.FormSection` and `ui.FormRow`: `Below`, `TitleMaxLines` and `MinHeight` customize the text column and height; `Role: core.RoleRadio` with `Checked` and `OnPress` uses `base.Radio` inside a `base.RadioGroup`. A section’s `DividerInset` returns each row’s inset and an override flag, with zero allowing full width. [Forms](https://nsgo.dev/docs/forms/#grouped-rows)
- Apps use `ui`'s components, which have a look and follow the theme; `base` holds the same controls without a look, for component authors; `mobile` and `desktop` hold what only phones or only desktops have. [references/components.md](references/components.md) maps needs to components.
- Load cues with `audio.LoadSound` and stream longer audio with `audio.NewPlayer` where `Env.CanPlayAudio` reports support. On the web, play after a user gesture ([audio playback](https://nsgo.dev/docs/services.md#audio-playback)).
- Use [`core.Video.Headers`](https://nsgo.dev/docs/components/) for authenticated video URLs on Android and direct HTTP files on Linux, macOS, iOS and tvOS.
- Bind changing names and hints with `LabelFunc` and `HintFunc` so controls keep their focus and state. Every core element with `Label` takes its twin, and `core.View` and `core.Text` take `HintFunc`.
- Tooltips provide their controls' accessibility descriptions before focus, omitting text that repeats the label and preserving an explicit description; their bubbles are hidden from screen readers ([accessibility](https://nsgo.dev/docs/accessibility.md)).
- A desktop app that opens its own windows, panels, tray icons and shortcuts runs `nsgo.Run(App)` with `func App(app *core.Scope)` and opens them with `desktop`. A popup card is a panel with `PanelOptions` `Transparent` and `FitContent` whose content draws its own corners and shadow, and a background where `desktop.Transparent` is false. `nsgo.StartHidden()` opens the main window hidden, for a tray app or a launch at login, until `cx.MainWindow().Show()`.
- Read `desktop.Tray.ClickBounds()` in `TrayOptions.OnClick` or `Menu` to anchor a custom panel with `Window.ShowAt`. Check its availability: macOS reports the status item's rectangle, Windows the shell's when available, and Linux supplies none. `TrayOptions.Title` appears beside an image or drawn view on macOS.

- macOS alerts and file pickers show independently when their requesting window is hidden, minimized or offscreen, so tray apps can call the normal `cx.ShowAlert`, `cx.PickFiles` and `cx.SaveFile` APIs. Closing that surface cancels its dialogs.
- Use `desktop.WindowOptions.Parent` when a window should close with another window of the same app. On macOS it also starts centered over its parent when `Frame` is unset and follows the parent's visibility and full screen.
- In a painted macOS or Windows window, set `Image.HitTestAlpha` (TSX `hitTestAlpha`) above 0 to pass the pointer through transparent image pixels.
- Use `cx.ScanCode` on macOS, iOS and Android. Gate the action on `cx.Env().CanScanCode()`, and give Apple builds an `NSCameraUsageDescription` in `apple.info` or Android builds `CAMERA` in `android.permissions`. [Photos and files](https://nsgo.dev/docs/services.md#photos-and-files)
- A window that lets the pointer through to the windows below sets `PointerEvents` in `desktop.WindowOptions` or `desktop.PanelOptions`, or calls `desktop.SetWindowPointerEvents`: `desktop.WindowPointerPainted` takes it only over views that draw or hear it, as around a desktop pet; `desktop.WindowPointerNone` takes none, so the app gives it back from a shortcut or a tray icon.
- Carry sign-in cookies into web views with `cx.SetWebCookies`, and wait for its callback before loading. To remove one, write its same name, domain and path with a past `Expires`; zero means a session cookie. Android shares cookies with Incognito views and lists only URL-filtered names and values. [Cookie store](https://nsgo.dev/docs/services/)
- Use `core.WebView.MutedFunc` (TSX `muted`) to change sound in a mounted web view. Windows and Linux mute the page, and macOS 12 and iOS 15 or later suspend its media. The setting follows navigations.
- For a panel triggered by a screen edge, sample the global pointer with `desktop.CursorPoint` and compare it with `desktop.Monitors`. Check the availability result, which is false on Wayland. [Desktop services](https://nsgo.dev/docs/services.md)
- Check `Env.CanLockPointer` and call `Ref.LockPointer` from a press for a camera or drag that needs relative motion. Handle `View.OnPointerDelta` and keep the returned unlock function. Escape and loss of window focus also release it.
- Loop a sound with `audio.Voice{Loop: true}`. For a music intro plus repeats, set `audio.PlayerOptions.LoopStart` and call `SetLoop(true)` ([looping audio](https://nsgo.dev/docs/services.md#looping-audio)).
- A desktop pet, widget or HUD opens its window with `desktop.WindowOptions{Transparent: true}` (the main window with `nsgo.WindowTransparent()`), leaves its root view without a background, and moves it by a view's `Desktop.WindowDrag`; `NoShadow` and `SkipTaskbar` drop the shadow and the taskbar button, and `desktop.CanMakeWindowsTransparent` and `desktop.CanSkipTaskbar` say whether they take effect, as on X11 without a compositor or on Wayland they do not.
- Schedule sounds with `audio.Now(cx)` and `audio.Voice.StartAt` where `audio.CanSchedule(cx)` is true ([audio timing](https://nsgo.dev/docs/services.md#audio-timing)).
- TSX apps embed their sources and run `compiler.App(src, "App.tsx")`, importing `nsgo`, `nsgo/base` and `nsgo/ui`; Go components and functions reach TSX through `tsx.Register` and `tsx.RegisterFunctions` ([TSX apps](https://nsgo.dev/docs/tsx.md)).
- For short effects, use `audio.LoadSound` and `Sound.Play`. macOS and iOS return a playback handle with independent volume, rate, stop and reactive end state. Use `Voice.Muted` or `Player.SetMuted` to start silently without pausing. [Audio](https://nsgo.dev/docs/services.md#audio)
- `core.PanEvent` supplies `Position` in the view and `PagePosition` in its page on every phase.
- A native `Plugin.Call` reports `tsx.ErrBudget` when `Limits.Call` cuts its JS, even after `await`. `Limits.CallTotal` also counts time spent waiting. Hosts can stop the plugin with `Plugin.Interrupt`, read `Plugin.Stopped()` and `Plugin.Err()` from any goroutine, or set `Sandbox.OnStop` for a notification on the engine's goroutine. Sandboxed backtracking regexps use the shorter load or call budget as their match timeout. For host deadline extensions, view interruption and native `JSTime`, see [TSX](https://nsgo.dev/docs/tsx/).
- In a native sandbox `JSON.stringify`, `JSON.parse` with a reviver, `Array.prototype.join`, `toString`, `toLocaleString` and `flat` throw a catchable `RangeError` past 1,000 levels, counted across nested calls, instead of hanging or overflowing the host's Go stack.
- A native sandbox uses 1,000 frames with `DefaultLimits` and caps `Limits.Stack` at 10,000. Nonpositive Go values use the cap.
- Native functions calling each other (built-in getters, `toString`/`valueOf`, `RegExp` `exec` dispatch, `Reflect.apply`/`construct`, bound-function and proxy chains) count against `Limits.Stack` and throw a catchable `RangeError`, and exporting a plugin value nested deeper fails with a `RangeError`, through NSGo's goja fork `github.com/nsgo-dev/goja`, which apps get with NSGo without a `replace` of their own.
- `Limits.Memory` budgets metered native storage, including array copies and spread, string concatenation and replacement, and typed-array buffers. The browser uses its own heap. [TSX apps](https://nsgo.dev/docs/tsx.md)
- Register a getter's nonzero default with `tsx.Default("Max", float64(100))` so forwarded undefined props keep that default as they change.

## Layout and styles

- Layout is flexbox with React Native's defaults (a column, children keep their size) plus CSS grid. Sizes take `core.Pt` or `core.Pct`; text shrinks and wraps in a row only with `FlexShrink: 1`, an unset minimum being 0 already (`core.AutoMinimum` is CSS's automatic minimum).
- A screen that fills the window takes `FlexGrow: 1, FlexShrink: 1`, as does every box between it and a list or scroll view that should scroll; with `FlexGrow` alone a long list stretches past the window instead of scrolling.
- Let a scroll view fill its container and put the padding on its content, so the scroll bar stays against the edge. Scroll only through `core.Scroll`, `core.List` or `ui`'s scroll views, never by stepping offsets from pointer or wheel events; they keep native inertia and edges.
- Set `core.Scroll.NoShrink` when a scroll view must keep its requested size while a sibling shrinks.
- Use `core.Span.Attachment` to place a view in a line of text. Its first text line supplies the default baseline.
- Keep content clear of the notch and the keyboard with `PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Bottom: core.EnvKeyboard}` rather than fixed insets.
- `core.Style` is typed CSS: zero fields are unset and `Merge` composes styles. `Class` takes Tailwind CSS 4 classes, responsive (`md:`), appearance (`dark:`), pointer (`pointer-fine:`, `pointer-coarse:`) and state (`hover:`) variants included; a `Style` wins over a class.
- Take colors from the theme, never fixed hex values in a component: `ui.ColorsOf(cx)` inside a `StyleFunc` follows theme and appearance changes in place. Unset text and `currentColor` icons follow the subtree's `Palette.Text`; explicit colors keep inheriting. `cx.SetTheme` themes a subtree; `core.ParseTheme` reads a YAML or shadcn/ui theme.
- Do nothing implicitly by device. Choose sizes and forms for a phone or a desktop explicitly, from the window's width, `pointer-fine:` / `pointer-coarse:` classes or the target, not by a component changing on its own.
- Leave the pointer's arrow alone and give lists, tables and sidebars a selected look without hover or press: NSGo apps look native, not like web pages, on the web too.

## Words and configuration

- Put every user-visible word in `locales/`, a YAML catalog per language in ICU MessageFormat beside `nsgo.yaml`, and call the generated functions (`locales.Greeting(cx, "Ada")`), never string literals. `dev`, `build` and `test` check catalogs and regenerate the functions; `go tool nsgo dev -lang en-XA` finds untranslated and cut text, `-lang ar-XB` right-to-left layout.
- Write configuration people edit by hand in YAML, not JSON: `nsgo.yaml`, catalogs and themes.

## Tests and checks

Run the tool, never `go vet`, `go test`, `go build` or `go run` directly: it type-checks TSX, generates catalogs and vets build tags for every target.

Use `app.Surface("title:Settings")` to run test steps in a secondary desktop window. [Testing](https://nsgo.dev/docs/testing.md)

```sh
go tool nsgo check                 # vet; -target web,android vets those targets instead, all every available one
go tool nsgo test                  # headless; -run as go test's, -target macos|ios|ios_simulator|android|web|all on a platform
```

Headless `nsgo test -- -c` compiles tests without running them; compiler failures remain visible in the console and JUnit report.

Name every target a change touches in one `check -target`, with `-jobs n` to vet up to n at once, each `go vet` with n compile workers; never start several checks, as a check waits for the one already running in its module.

Test an app as a user uses it with `nsgotest`, finding views by text, role, label or test ID. Read [references/testing.md](references/testing.md) before writing tests.

## Debugging

Run `go tool nsgo dev -target native -debug` with Delve on `PATH` to attach a debugger to a desktop app. Reconnect to the printed loopback address after a Go rebuild. [Running and debugging](https://nsgo.dev/docs/debugging.md#go-debugger)

`go tool nsgo dev -devtools` opens DevTools (elements, signals, a commit profiler and a test recorder); `-highlight` outlines what each commit changes, `-trace` prints each commit with the lines whose writes caused it, `-layout` outlines every box and `-release -fps` measures frame and commit cost, including native text round trips on Android. On macOS and Apple devices and simulators (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator`, `watchos_simulator`), `go tool nsgo mcp` lets an agent drive the running app over MCP (`claude mcp add nsgo -- go tool nsgo mcp`): list windows, outline views, press, type, scroll and take screenshots. Use it to check a change on screen instead of assuming it renders. On macOS and Windows a native Go rebuild restores the main window’s last regular frame during the same dev session. Windows also restores full screen. Command/Control+R reloads from every open window and remains available after an app function's main view closes.

For Go component previews, put `PreviewXxx(*core.Scope) core.Node` functions in an importable package under `//go:build nsgopreview`. Run `go tool nsgo dev -preview ./components` or add `-target web`. [Component previews](https://nsgo.dev/docs/debugging.md#component-previews)

Use `go tool nsgo dev -profile cpu` or `-profile heap` for a Go profile, and `-profile trace` for a scheduler trace. The host writes them under `.nsgo/profiles` unless `-profile-output` sets a path. On the web, capture the heap or an execution trace. [Runtime profiles](https://nsgo.dev/docs/debugging.md#runtime-profiles)

## Shipping

`go tool nsgo build` packages every target `nsgo.yaml` lists into `dist/`, installers and store packages into `dist/releases`. Read [references/shipping.md](references/shipping.md) before editing `nsgo.yaml`, icons, signing, updates or embedding.

For store ratings, call `cx.RequestReview()` where `cx.Env().CanRequestReview()` is true. Standalone Android packages include Play Review; embedded hosts supply it through Gradle. [Store reviews](https://nsgo.dev/docs/services.md#store-reviews)

Use `go tool nsgo publish ios` with a signed IPA or `go tool nsgo publish macos` with a signed PKG to upload to TestFlight. [App Store Connect setup](https://nsgo.dev/docs/building.md#app-store-connect)

## Reusable components

A library of components follows NSGo's own split between behavior in an unstyled form and a look over it. Read [references/authoring.md](references/authoring.md) before writing components for other apps.
