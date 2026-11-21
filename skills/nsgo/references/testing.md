# Testing NSGo apps

`nsgotest` starts an app without a window, on a manual clock, and acts on it as a user does. The same tests run in a real window with the platform's input through `go tool nsgo test -target`.

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestAdd(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Type("id:draft", "Milk")
	app.Press("role:button Add")
	app.WaitFor("text:Milk", time.Second)
}
```

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

- Pointer and touch: `Press`, `PressWith`, `PressAt`, `PressAtWith`, `LongPress`, `RightClick`, `Hover`, `Drag`, `Scroll`, `ScrollTo`, `PullToRefresh`, `Pinch`, `Wheel`.
- Keyboard and text: `Focus`, `Type`, `Clear`, `Submit`, `Key`, `KeyPress`, `Paste`, `Select`, `Compose`.
- Menus and system UI: `ChooseMenu`, `ChooseDockMenu`, `ChoosePopup`, `ClosePopup`, `HeaderBack`, `Answer` for alerts (start with `nsgotest.PendingAlerts()`), `GrantPermission`, `DropFiles`, `OpenLinks` (delivers the app's own links headless and on iOS and Android targets), `CloseWindow` (with a desktop `Environment`, where the main view shows in the main window as `nsgo.Run` opens one: on macOS, and on Windows and Linux for an app's function). Headless, a role item without `OnSelect`, chosen by `ChooseMenu` or its shortcut, does what the desktops do: Close runs the main window's `OnClosing`, Quit asks `OnQuitRequest` on macOS (an approved request counts in `app.Quits`) and quits at once on Windows and Linux, Minimize, Zoom and Toggle Full Screen change the main window's reported state alone (`app.SetWindowEnvironment(title, change)` changes any one window's environment, the main window's for `""`, so a test puts one window in the background while another shows), the editing roles act on the focused field; other roles, and Undo or Redo in a field, fail the step; under `-target` such an item skips the test. A focused field takes `mod+c`, `mod+x` and `mod+v` itself, with `app.Clipboard`, unless an application-menu item has the key as its shortcut.
- `app.KeyCommands()` lists the titled keyboard shortcuts that can fire with the current focus, visible views and modals ([services](https://nsgo.dev/docs/services.md)).
- Reading: `Find`, `Exists`, `Enabled`, `Query`, `Texts`, `Dump`, and `FindMounted` for views outside the window; a view's `Bounds()` is its frame in the window's points, and an image's `Image.Source.Version` the frame of pixels it shows.
- Waiting: every step settles until animations end, headless also until style transitions reach their `OnTransitionEnd` (start with `ManualFrames()` to inspect them midway with `Advance`); on a target it, and `Settle`, first waits for the engine's pending commit, then for 100 ms without batches or finite animations, five seconds at most in total, so never add an empty post to wait for a slow first commit. `WaitFor`, `WaitForGone` and `WaitUntil` wait for what arrives later, such as a `cx.Post` from a goroutine. `Advance` and `FastForward` move the clock, which `cx.Now()` reads, so time what the app measures itself, such as a gesture's speed, with `cx.Now()` rather than `time.Now()`; never `time.Sleep`.
- Options of `Start`: `nsgotest.Size(w, h)` (390 by 844 points by default), `Storage(map)` for saved state, `Secrets(map)` for what the secure store kept, which `app.Secrets` holds in memory, `Environment(core.Environment{...})` for an appearance, a locale or a safe area, and `app.SetEnvironment` to change it later, `Idiom(core.IdiomDesktop)` for what only desktops show, such as hover tooltips (headless apps start as phones), `WallTime(t0)` for the date the first render reads (`app.Clock` starts at 2026-01-01 UTC otherwise), `Clock(c)` to run the app on a `core.ManualClock` the test shares, `Host` for an embedding host, `HotKeyTaken(keys...)` and `HotKeyUnsupported(keys...)` for global shortcuts the headless platform refuses. On a real target, `Environment` pins only the appearance, locale, text size, reduced motion, contrast, login launch reason, front application and device name and model; the target keeps its OS, idiom, safe area and header heights, so lay out another OS's scene headless or on that OS's target. `Size` sizes the window on desktop and web targets; Android, iOS, tvOS and watchOS keep the device's screen.

`app.RequestQuit(false)` sends a user quit and `app.RequestQuit(true)` sends a forced quit, headless or on macOS. The app’s `OnQuitRequest` handlers can check `q.Forced()`, and approved requests count in `app.Quits`.

`PressWith(selector, core.Modifiers{Shift: true})` and `PressAtWith(selector, at, mods)` hold `Shift`, `Control`, `Alt` or `Platform` for that press only, including its down and up events. Plain presses carry no modifiers. These steps use native input on a target; watchOS rejects modified presses.

A press can name a text field directly. Read `app.Find("id:field").Focused` after the step on macOS, Windows, Linux or headless, including when a press handler focuses the field.

`Ref.Focus()` commands follow platform focus stops: text fields everywhere, plus enabled pressable and `Focusable` views on desktops, the web and Apple TV. Use a desktop or web `Environment` when a headless test exercises button focus. The `Focus` step reaches these desktop/web stops through Tab. Any of `OnPress`, `OnPressDown`, `OnPressUp` or `OnLongPress` makes a view a stop; Return or Space sends it the down, up and press events it handles.

`Key("mod+a")` selects a focused field unless a binding or an application-menu item with that shortcut takes it. Headless, on desktop targets and on the web, `KeyAlways` takes Return, Backspace and select-all before the field acts; `KeyAuto` and `KeyNotEditing` leave them to the field. The Android and iOS test hosts do not give the keymap these keys first yet. A focused view's `OnKeyDown` hears `Key` before any binding, a modifier alone (`Key("shift")`) included, and a key it consumes fires none; `Key` is a full press, so `OnKeyUp` hears its release. `KeyDown(key)` and `KeyUp(key)` hold and release one key, never a chord; a held modifier joins later keys (`KeyDown("shift")` then `Key("a")` is Shift+A), and `KeyUp` of a key not held fails. Android, HarmonyOS and watchOS targets skip them.

## Tests without the network

Keep a test off the network and disk the user owns: inject the fetcher or the `http.Client` into the component, or serve answers from `net/http/httptest`, and give storage and secrets through `nsgotest.Storage` and `nsgotest.Secrets`, so no test reaches the machine's keychain. A resource's loading and error states are then tested by answering slowly or with an error.

Use `nsgotest.Widgets(true)` and `nsgotest.LiveActivities(true)` to inspect publications with `App.Widgets` and `App.LiveActivities`. Run tests using these process-wide services sequentially. [Testing widgets and Live Activities](https://nsgo.dev/docs/testing.md#widgets-and-live-activities).

## On a platform

Cleanup closes native context menus left open by an app, including between subtests in one process.

Test discovery follows the target's build tags, including target-only tests in packages that also have portable files.

```sh
go tool nsgo test -target macos            # windows, linux, web, ios (a connected iPhone), ios_simulator, tvos_simulator, watchos_simulator, android; windows_x64 means windows on an x64 machine (only the architecture the tests run for)
go tool nsgo test -target macos -run TestAdd
go tool nsgo test -target macos -activate  # allow the test app to take the keyboard
```

On macOS, test windows accept only the test host's input, so the developer's mouse, keyboard and trackpad cannot change their scenes while the suite runs.

Test windows take keys without activating the app. AppKit may refuse a field's click while another app is active, which `Focus` and `Type` diagnose. `-activate` lets the test app take the keyboard on an unlocked desktop before a field click or key step. It needs tests that import `github.com/nsgo-dev/nsgo` (a blank import is enough) and call `nsgotest.Main` in `TestMain`.

`app.Screenshot("en/library")` saves `.nsgo/test/screenshots/<target>/en/library.png`, `app.Capture()` returns the pixels, and both skip headless. Captures on macOS, iOS and tvOS use sRGB, and the macOS WindowServer capture includes an open native popup clipped to the test window. `-trace` prints each step's commits, `-junit report.xml` writes a report, `-bench` runs benchmarks, `-env KEY=VALUE` reaches a phone's tests, and headless flags after `--` go to `go test`. Target runs accept `-short`, `-count`, `-cpu`, `-parallel`, `-failfast` and `-fullpath` there and reject unsupported arguments. Host-linked tests repeat in fresh processes for each count and CPU setting, a timeout per invocation, which on a simulator starts when the test host starts, the launch limited to five minutes of its own; libraries keep Go's in-process repetitions. Put NSGo flags before the directory; on a target that includes `-run` and `-timeout`, which headless runs hand to `go test` after it. `test` takes one directory, every package under it, and errors on a second.

DevTools' Recorder (`go tool nsgo dev -devtools`) writes what you do in the running app as an `nsgotest` test to start from.

With `-target web`, libraries that do not link the backend run headless inside the browser. Storage stays in memory; `DataDir`, `CacheDir` and `LogsDir` are empty, and the file-backed `nsgotest.MicrophoneInput` fake skips.

`NSGO_BROWSER` names the browser `-target web` runs; startup waits up to 30 seconds and reports the browser's error output on failure.

## TSX

A TSX app's `*.test.tsx` files, written with `nsgo/test`, run with its Go tests under `go tool nsgo test`.
