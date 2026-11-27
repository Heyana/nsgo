# Testing NSGo apps

`nsgotest` starts an app without a window, on a manual clock, and acts on it as a user does. The same tests run in a real window with the platform's input through `nsgo test -target`.

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestAdd(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Type("id:draft", "Milk")
	app.Press("role:button Add")
	app.WaitFor("text:Milk", time.Second)
}
```

Target scroll reveal avoids native bars covering viewport edges, including nested scroll views. Physical iPhone test output retains stderr panic diagnostics after the host starts.

`TestMain` gives the platform the main thread when tests run on a target; keep it in every package with app tests. For setup and teardown around the tests, call `nsgotest.MainFunc(func() int { setup(); defer teardown(); return m.Run() })` instead, never code after `nsgotest.Main`, which does not return; on a target it runs once per test, each in a process of its own. On Android, write setup logs to stderr until the host starts: stdout carries its protocol, and a bypassed host or corrupt frame fails the run with a `nsgotest.Main` hint. If a target test times out, `nsgo test` reports whether the host started and includes the tail of that test's logcat. The target runner retries transient screen-state adb reads and stops once with a device-lost error if the device stays unreachable.

An app run with its function, `nsgo.Run(func(app *core.Scope) {...})`, starts with `nsgotest.StartApp(t, fn)`, which runs it once in the app's own scope as `Run` does; never pass `core.AppRoot(fn)` to `Start`, which shows a root view as the main view, so the function's scope would not last.

## Selectors

| Selector | Matches |
| --- | --- |
| `id:<TestID>` | The view whose `TestID` field is that |
| `label:<Label>` | The view with that accessibility label |
| `text:<text>` | A text containing it, or a field holding it (its placeholder while empty) |
| `role:<role>` | A view with that role: `button`, `checkbox`, `textbox`, ... |
| `role:<role> <name>` | One whose label is name, or whose texts contain it |
| anything else | A test ID, else a label, else a whole text |

`label:`, `text:` and a role's name take `/regexp/flags`. `a >> b` looks for `b` inside `a`, and `>> nth=<i>` keeps one match. A step fails the test when its selector matches no view, more than one, or a disabled one, and prints the view tree; that is the check, so a separate assertion is rarely needed.

Prefer what a user sees, text and roles, and set `TestID` on elements where text repeats or is translated. Tests read words in the language they run in.

Keep each graph on the goroutine that created it, cleanup included. Signal writes, new effects and memos, batches and commits from another goroutine panic in native tests and development builds when they land while the app is idle (`-race` catches the rest; WebAssembly builds skip this check); use `cx.Post` from workers and wait through the app's steps. An internal engine panic still fails the test, even though a shipped app isolates it to that engine.

## Steps

Use `app.Post(func() { signal.Set(value) })` from a test or stubbed worker to queue work on the app's graph goroutine, then `app.Settle()` or a wait before reading views. It runs on the test goroutine headless and the engine goroutine on a target, so the same test may set signals on both; the function takes no steps and does not call `t.Fatal`.

- Pointer and touch: `Press`, `PressWith`, `PressAt`, `PressAtWith`, `DoublePress`, `LongPress`, `RightClick`, `Hover`, `Drag`, `Pan`, `Scroll`, `ScrollTo`, `PullToRefresh`, `Pinch`, `Wheel`. Native `Pinch` runs on macOS, the iOS simulator, Android and the web. Physical iOS devices and other unsupported targets skip at that step.
- Keyboard and text: `Focus`, `Type`, `Clear`, `Submit`, `Key`, `KeyPress`, `Paste`, `Select`, `Compose`.
- Gamepads (headless only): `app.ConnectGamepad(id, name)`, `app.SetGamepadInfo(id, family, vendorID, productID, canRumble)`, `app.DisconnectGamepad(id)`, `app.SetGamepadButton(id, btn, value)`, `app.SetGamepadAxis(id, axis, value)` drive `gamepad.Pads` and `gamepad.State`, and `app.GamepadRumbles()` / `app.LastGamepadRumble(id)` inspect recorded `gamepad.Rumble` and `gamepad.StopRumble` events.
- Menus and system UI: `ChooseMenu`, `ChooseDockMenu`, `ChoosePopup`, `ClosePopup`, `ClickTray("index:0", core.MouseButtonLeft)` or `ClickTray("title:Tools", core.MouseButtonRight)`, `HeaderBack`, `Answer` for alerts (start with `nsgotest.PendingAlerts()`), `GrantPermission`, `DropFiles`, `OpenLinks` (delivers the app's own links headless and on iOS and Android targets), `ReceiveShare` (delivers `mobile.SharedItem` values headless and on iOS and Android targets), `CloseWindow` (with a desktop `Environment`, where the main view shows in the main window as `nsgo.Run` opens one: on macOS, and on Windows and Linux for an app's function). `ClickTrayIndex` and `ClickTrayTitle` are convenience forms. A tray menu becomes available to `ChoosePopup`; omit bounds to model unavailable tray geometry, or pass one `core.Bounds` after the button. Headless, a role item without `OnSelect`, chosen by `ChooseMenu` or its shortcut, does what the desktops do: Close runs the main window's `OnClosing`, Quit asks `OnQuitRequest` and counts each approved request in `app.Quits`, Minimize, Zoom and Toggle Full Screen change the main window's reported state alone (`app.SetWindowEnvironment(title, change)` changes any one window's environment, the main window's for `""`, so a test puts one window in the background while another shows), the editing roles act on the focused field; other roles, and Undo or Redo in a field, fail the step; under `-target` such an item skips the test. A focused field takes `mod+c`, `mod+x` and `mod+v` itself, with `app.Clipboard`, unless an application-menu item has the key as its shortcut.
- `app.Menu`, `app.DockMenu` and `app.PopupMenu` expose `nsgotest.MenuItem` snapshots to inspect before choosing an item by its labels.
- `app.KeyCommands()` lists the titled keyboard shortcuts that can fire with the current focus, visible views and modals ([services](https://nsgo.dev/docs/services.md)).
- Reading: `View.Props` is an owned `nsgotest.ViewProps` snapshot with `HelpText`, `HasAction`, `ActionList` and `FocusStop` policy helpers. `View.Kind` uses `nsgotest.ViewKind` and constants such as `nsgotest.KindText` and `KindCanvas`. `Find`, `Exists`, `Enabled`, `Query`, `Texts`, `Dump`, and `FindMounted` for views outside the window; a view's `Bounds()` is its frame in the window's points, and an image's `Image.Source.Version` the frame of pixels it shows.
- Timed haptics: set `HapticPatterns` through `nsgotest.Environment` and inspect `app.HapticPatterns`. A nil entry records a cancellation. [Haptics](https://nsgo.dev/docs/haptics.md)
- Waiting: every step settles until animations end and style transitions reach their `OnTransitionEnd` (start with `ManualFrames()` to inspect them midway with `Advance`); on a target it, and `Settle`, first waits for the engine's pending commit, then for 100 ms without batches or finite animations, five seconds at most in total, so never add an empty post to wait for a slow first commit. `WaitFor`, `WaitForGone` and `WaitUntil` wait for what arrives later, such as a `cx.Post` from a goroutine. Headless they advance the clock a frame at a time, unless `ManualFrames()` keeps the clock still; on a target only while a style transition waits for its `OnTransitionEnd`. `Advance` and `FastForward` move the clock, which `cx.Now()` reads, so time what the app measures itself, such as a gesture's speed, with `cx.Now()` rather than `time.Now()`; never `time.Sleep`.
- Options of `Start`: `nsgotest.Size(w, h)` (390 by 844 points by default), `Storage(map)` for saved state, `UpdaterState(core.UpdaterState{...})` for initial updater capability and preferences (`app.Update` supplies check results), `Secrets(map)` for what the secure store kept, which `app.Secrets` holds in memory, `Environment(core.Environment{...})` for an appearance, a locale or a safe area, and `app.SetEnvironment` to change it later, `Idiom(core.IdiomDesktop)` for what only desktops show, such as hover tooltips (headless apps start as phones), `WallTime(t0)` for the date the first render reads (`app.Clock` starts at 2026-01-01 UTC otherwise), `Clock(c)` to run the app on a `core.ManualClock` the test shares, `Host` for an embedding host, `HotKeyTaken(keys...)` and `HotKeyUnsupported(keys...)` for global shortcuts the headless platform refuses. On a real target, `Environment` pins only the appearance, locale, text size, reduced motion, contrast, thermal state, low-power mode, login launch reason, front application and device name and model; the target keeps its OS, idiom, safe area and header heights, so lay out another OS's scene headless or on that OS's target. `Size` sizes the window on desktop and web targets; Android, iOS, tvOS and watchOS keep the device's screen. On iOS and tvOS, `app.Resize(w, h)` changes the test host's native `UIWindow` for viewport checks; it does not arrange system multitasking, and live automation rejects it.

`app.RequestQuit(false)` sends a user quit and `app.RequestQuit(true)` sends a forced quit, headless or on macOS. The app’s `OnQuitRequest` handlers can check `q.Forced()`, and approved requests count in `app.Quits`.

`PressWith(selector, core.Modifiers{Shift: true})` and `PressAtWith(selector, at, mods)` hold `Shift`, `Control`, `Alt` or `Platform` for that press only, including its down and up events. Plain presses carry no modifiers. These steps use native input on a target; watchOS rejects modified presses.

`PressAt` takes viewport points. Pointer picks, visibility and scroll limits include nested and fractional scroll content zoom; `Bounds()` remains the layout frame before transforms.

For trusted events inside a native `WebView`, check `Env.CanSendInput` and call `app.Post(func() { page.SendInput(core.WebClick(12, 16)) })`. Wait for page readiness, and for acknowledged focus before `core.WebText`; Linux needs X11 and XTest. [Testing](https://nsgo.dev/docs/testing.md)

A press can name a text field directly. Read `app.Find("id:field").Focused` after the step headless or on any target, including when a press handler focuses the field.

`Select(selector, start, end)` uses UTF-16 offsets. On iOS, Android and HarmonyOS targets it moves the native field's range and reads it back; selection callbacks no longer skip those tests. HarmonyOS typing replaces the reported range, but its test host still lacks native clipboard keys. Desktop and web targets retain their select-all/arrow-key path.

Use `app.SelectTab("id:tabs", 1)` to choose the second item of a mounted `mobile.TabBar`. It works headless with an iOS or Android `Environment` and drives the native tab bar on iOS and Android targets; use it for phone navigation instead of model-only `Emit`.

Android native key steps wait for asynchronous consume continuations; `OnKeyDown` can consume before bindings and activation, and `OnKeyUp` returns to the original view. Explicitly focused iPhone/iPad views use the same callback contract. HarmonyOS supports held-key bridge steps and Go binding consumption; native plain-view focus and native-action consumption remain unavailable there. A native editor's keys do not enter `core.KeysDown`.

`Ref.Focus()` commands follow platform focus stops: text fields everywhere, plus enabled pressable and `Focusable` views on desktops, the web and Apple TV. Use a desktop or web `Environment` when a headless test exercises button focus. The `Focus` step reaches these desktop/web stops through Tab. Any of `OnPress`, `OnPressDown`, `OnPressUp` or `OnLongPress` makes a view a stop; Return or Space sends it the down, up and press events it handles.

`Key("mod+a")` selects a focused field unless a binding or an application-menu item with that shortcut takes it. Headless, on desktop targets and on the web, `KeyAlways` takes Return, Backspace and select-all before the field acts; `KeyAuto` and `KeyNotEditing` leave them to the field. On iOS, software Return also reaches `KeyAlways` before submitting, blurring or inserting a newline, and composition keeps its Return. The Android and iOS test hosts also give `KeyAlways` Backspace and select-all first. A focused view's `OnKeyDown` hears `Key` before any binding, a modifier alone (`Key("shift")`) included, and a key it consumes fires none; `Key` is a full press, so `OnKeyUp` hears its release. `KeyDown(key)` and `KeyUp(key)` hold and release one key, never a chord; a held modifier joins later keys (`KeyDown("shift")` then `Key("a")` is Shift+A), and `KeyUp` of a key not held fails. watchOS targets skip them. `MouseDown` / `MouseUp` (and `At` / `With` / `AtWith` variants) press and release all five `core.MouseButton` values; `BlurWindow()` simulates window focus loss, releasing held keys and mouse buttons.

`View.Style` and `View.Animated` use owned `nsgotest.ViewStyle` and `nsgotest.AnimatedValues` records. Read `app.Animated(selector)` for the style with bound values applied; geometry and animation policy helpers keep their existing behavior.

Use `app.Window` and `nsgotest.WindowAction` constants such as `nsgotest.WindowSetConstraints` to assert what the app asked of its main window. In headless tests, `app.Shares` records the files, text and anchor of each share request. Use `View.Web` to check a web view’s recorded settings, and `View.WebCommands` with `nsgotest.WebCommandKind` constants to check its requests.

For a headless platform report no step provides, `app.Report` takes an owned `nsgotest.Event`; optional `nsgotest.TouchGeometry` supplies native hit targets and local positions. Reports copy their collections and geometry while retaining `Data` payload identity.

`KeyCode("KeyW")` presses a physical position and `KeyCodeDown` / `KeyCodeUp` hold and release it. Headless, override `core.Environment.KeyCodeNames` to exercise a layout (unspecified positions use the US layout); under `-target` on macOS, Windows, Linux and the web the platform's current layout names the key, and iOS, Android and HarmonyOS report the US layout's key. watchOS and mini programs skip them.

For native headless canvas pixels on macOS, use `nsgotest.Render` on a recorded `Drawing`. Pass `nsgotest.TextEngine(nsgotest.PlatformTextEngine())` to match CoreText metrics, and call `nsgotest.RegisterFont` before starting the app. Rendering waits for images, returns errors and requires resolved view paints and animation slots; other targets return `nsgotest.ErrNoPlayer`. [Windowless drawings](https://nsgo.dev/docs/testing.md#drawings-without-a-window)

For headless GPU checks in another window, use `Surface.GPUReadPixels`, `Surface.TriggerGPUDeviceLost` and `Surface.RestoreGPUDevice`; devices, readbacks and recovery follow that window, including its pixel ratio. `App.GPUFrames` records every window's submissions. Use `nsgotest.TranslateShader` on `*core.ShaderProgram` for fragment shaders and `gpu.TranslateShaderModule` on `*gpu.ShaderModule` for 3D pipelines. They translate headlessly into the backend sources and report per-backend errors with panic recovery.

Read mounted image, input, modal, screen, scroll, video and Picture in Picture settings through `nsgotest` records. `View.Sticky` contains `nsgotest.StickyCell` values. Image variant maps and scroll lists are snapshots.

The test's private clipboard models `ReadClipboardItems`, `WriteClipboardItems` and `ClipboardChangeCount` alongside its text, image and file calls. Inspect `app.ClipboardItems` and `app.ClipboardChangeCount` after settling the app.

## Tests without the network

Use `app.AppleSignIn` or `app.PurchaseAnswer` when a test needs to answer Sign in with Apple or a purchase. The hooks receive `nsgotest.AppleSignInRequest` and `nsgotest.PurchaseRequest` records. [Services](https://nsgo.dev/docs/services.md)

Keep a test off the network and disk the user owns: inject the fetcher or the `http.Client` into the component, and serve its answers from `nsgotest.NewHTTPTransport`, whose routes answer in-process with no listener — `net/http/httptest` also serves one, but its listener keeps it off `-target web` — and give storage and secrets through `nsgotest.Storage` and `nsgotest.Secrets`, so no test reaches the machine's keychain. A resource's loading and error states are then tested by answering slowly or with an error.

Set the app's picker, camera or save hook to answer a request in an app test. The hooks receive the `nsgotest` records `ImagePick`, `CodeScan`, `FilePick`, `DirectoryPick` and `FileSave`. [Photos and files](https://nsgo.dev/docs/services.md#photos-and-files)

Use `nsgotest.Widgets(true)` and `nsgotest.LiveActivities(true)` to inspect publications with `App.Widgets` and `App.LiveActivities`. Run tests using these process-wide services sequentially. [Testing widgets and Live Activities](https://nsgo.dev/docs/testing.md#widgets-and-live-activities).

In headless Linux and Android tests, read `app.Installs` and set `app.InstallPackage` to simulate an installer error.

Use `desktop.RequestRestart` when an app restart must honor its `OnQuitRequest` cancellation or deferred save. Headless tests can inspect `app.Restarts` after answering the handler. [Lifecycle](https://nsgo.dev/docs/services.md#lifecycle)

Use `app.Play` with `core.VideoInfo.RateChangeable` set explicitly to test both a video's adjustable-speed controls and its fixed-speed fallback. [Media reports](https://nsgo.dev/docs/testing.md#media-reports)

## On a platform

Cleanup closes native context menus left open by an app, including between subtests in one process.

Use `app.Surface(selector)` for window-local `Exists`, `Texts`, `Enabled`, `Clear`, `Focus`, pointer and gesture steps, `DropFiles`, media reports, captures, `Resize`, `Settle` and waits. Its `Post` schedules through the original app and refuses a closed window; app-wide clocks and service fixtures stay on the parent `App`. Read `Surface.Config()` for opening options and `app.StatusBar` for the resolved status bar settings.

Test discovery follows the target's build tags, including target-only tests in packages that also have portable files.

```sh
nsgo test -target macos            # windows, linux, web, ios (a connected iPhone), ios_simulator, tvos_simulator, watchos_simulator, android; windows_x64 means windows on an x64 machine (only the architecture the tests run for)
nsgo test -target macos -run TestAdd
nsgo test -target macos -activate  # allow the test app to take the keyboard
```

On HarmonyOS, `nsgo test -target harmonyos` uses `build.harmonyos_bundles` and reports whether the HAP used the committed host or a host compiled from templates. See [Building](https://nsgo.dev/docs/building.md#harmonyos).

HarmonyOS target tests have fresh data, cache and log directories under `os.TempDir()` and can use Go file operations there.

On macOS, test windows accept only the test host's input, so the developer's mouse, keyboard and trackpad cannot change their scenes while the suite runs.

Test windows take keys without activating the app. AppKit may refuse a field's click while another app is active, which `Focus` and `Type` diagnose. `-activate` lets the test app take the keyboard on an unlocked desktop before a field click or key step. It needs tests that import `github.com/nsgo-dev/nsgo` (a blank import is enough) and call `nsgotest.Main` in `TestMain`.

`app.Screenshot("en/library")` saves `.nsgo/test/screenshots/<target>/en/library.png`, `app.Capture()` returns the pixels, and both skip headless. Captures on macOS, iOS and tvOS use sRGB, and the macOS WindowServer capture includes an open native popup clipped to the test window. Android captures the rendered window including shaders. `-trace` prints each step's commits, `-junit report.xml` writes a report, `-bench` runs benchmarks, `-env KEY=VALUE` reaches a phone's tests, and headless flags after `--` go to `go test`. Target runs accept `-short`, `-count`, `-cpu`, `-parallel`, `-failfast` and `-fullpath` there and reject unsupported arguments. Host-linked tests repeat in fresh processes for each count and CPU setting, a timeout per invocation, which on a simulator starts when the test host starts, the launch limited to five minutes of its own; libraries keep Go's in-process repetitions. Put NSGo flags before the packages; on a target that includes `-run` and `-timeout`, which headless runs hand to `go test` after them. Headless, `test` takes Go package patterns relative to the working directory (a directory is its own package, `/...` its tree, none tests `./...`); a single app directory (with `nsgo.yaml`), a directory in another Go module, a TSX entry, or a `-target` run takes one directory or entry.

DevTools' Recorder (`nsgo dev -devtools`) writes what you do in the running app as an `nsgotest` test to start from.

With `-target web`, libraries that do not link the backend run headless inside the browser. Storage stays in memory; `DataDir`, `CacheDir` and `LogsDir` are empty, and the file-backed `nsgotest.MicrophoneInput` fake skips.

`NSGO_BROWSER` names the browser `-target web` runs. macOS discovery prefers an installed Chrome for Testing. Startup waits up to 30 seconds and reports the browser's error output on failure.

Use `app.Surface("id:palette").WindowState()` on macOS, Windows or Linux to assert the real window’s `Frame`, `Visible` and `Focused`. Check `Frame.Move` before asserting position. This step skips headless tests and unsupported targets; compare both windows’ states to verify a `NoActivate` panel kept native focus in the original window.

## TSX

A TSX app's `*.test.tsx` files, written with `nsgo/test`, run with its Go tests under `nsgo test`.
