---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---

## A test

`nsgotest` starts the app without a window. A test finds controls by their text or role and acts on them as a user would:

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestPress(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("text:Press me")
	app.Find("text:Pressed 1 times")
}
```

`TestMain` gives the platform the main thread when the tests run on a target.

When a package needs process-wide setup and teardown, put them around `m.Run` in `MainFunc`; deferred cleanup runs before the test process exits:

```go
func TestMain(m *testing.M) {
	nsgotest.MainFunc(func() int {
		setup()
		defer teardown()
		return m.Run()
	})
}
```

`Main(m)` is the one-line form of `MainFunc(m.Run)`. A target runs each test in its own process, so this setup and teardown run once per test process.


An app run with its function, `nsgo.Run(func(app *core.Scope) {...})`, is tested with `nsgotest.StartApp(t, fn)`, which runs the function once in the app's own scope, as `Run` does, so its application menu and quit handlers stay. With a desktop `nsgotest.Environment` the app's main view shows in its main window where `nsgo.Run` opens one: on macOS, and on Windows and Linux for an app's function. There `desktop.CurrentWindow` and `OnClosing` work; `app.CloseWindow()` sends the window's close button's request and reports whether it closed, and a handler alone is tested by calling it and reading `WindowClosing.Canceled()`, or `QuitRequest.Canceled()` for `OnQuitRequest`. `app.RequestQuit(false)` sends a user quit and `app.RequestQuit(true)` sends a forced quit, such as logout. Both work headless and on macOS, Windows and Linux, counting an approved request in `app.Quits`. Steps on `app` act on the main window. `app.Opened` lists the windows, panels and tray views the app opened, as it asked for them, `Transparent` and `SkipTaskbar` among them, and `app.Window` the main window's requests as `nsgotest.WindowRequest` values. Use `nsgotest.WindowSetConstraints` and the request's `Constraints` to check a size limit. Headless, `app.Shares` records share requests as `nsgotest.Share` values with the files, text and anchor. `app.DropFiles` may name anything inside a drop zone: the nearest view around it with `OnFileDrop` takes the files. `app.OpenLinks("myapp://settings")` delivers the app's own links to `desktop.OnOpenLinks` or `mobile.OnOpenLinks` in headless tests and on iOS and Android targets. `app.ReceiveShare(mobile.SharedItem{Text: "https://example.com", MIME: "text/plain"})` delivers a share to `mobile.OnShared` in headless tests and on Android targets. A view's `Bounds()` is its frame in the window's points, as `Ref.Bounds` reports it. An image view's `Image.Source` is the source the view shows, so `app.Find("id:video").Image.Source.Version` and its `Pixels.Rect` tell which frame of pixels reached it and at what size. Headless, `app.Press` flips the system switch (`ui.SwitchSystem`) as on a target, and `app.Emit("id:switch", "change", true)` reports a value directly. `app.Navigate` and `app.PostMessage` reach a web view that is mounted but hidden, as it goes on loading. `View.WebCommands` records commands the app sent to the web view. Match its `Command` against values such as `nsgotest.WebLoad` or `nsgotest.WebSendInput`.

Use `app.Surfaces()` to list the open windows, panels and tray views in opening order with the main window first. `app.Surface("title:Settings")`, `app.Surface("id:settings-root")` and `app.Surface("index:1")` pick one by its current title, its root view's test ID or its position in that list. A selector must match exactly one open surface.

A surface's `Press`, `Type`, `Key`, `Find` and `Query` act in its own window while steps settle the whole app. `Config` returns its opening options as a `nsgotest.SurfaceConfig`, also recorded in `app.Opened`. `Screenshot` captures that surface on a native target, and `Close` asks its window to close through `OnClosing` and reports whether it closed. A closed handle fails a step and drops out of the list. The same steps work headless and on the macOS, Windows and Linux test hosts. Single-surface targets list their main surface alone.

`app.Menu`, `app.DockMenu` and `app.PopupMenu` expose `nsgotest.MenuItem` snapshots with labels, roles, check state and nested items. Use the corresponding choose step with a path of labels to invoke an item.

`Drag` cancels the press under the pointer once it moves more than 8 points from where it went down. The same rule applies to the web, so a drag that returns to its starting view does not fire `OnPress`.

`app.KeyCommands()` lists the titled keyboard shortcuts that can fire with the current focus and visible views. It reports the key, title and group headless and on a target.

`app.PressWith("id:row", core.Modifiers{Shift: true})` holds modifiers for one press; `PressAtWith(selector, core.Point{X: 20, Y: 10}, mods)` presses a point in the view. `Shift`, `Control`, `Alt` and `Platform` (Command on Apple, the Windows/Super key elsewhere) reach `OnPressDown`, `OnPressUp` and `OnPress`, headless and through native target input. Native platform conventions still apply: Chrome on macOS turns Control-click into a context menu. A later `Press` or `PressAt` has no modifiers. watchOS rejects modified presses because it has no keyboard modifier input.

`PressAt` uses points in the named view's viewport. Pointer targeting, visibility and scroll limits account for scroll content zoom, including nested or fractional scales, headless and in the target model. A view's `Bounds()` keeps its layout frame before transforms, as `Ref.Bounds` does.

After the app locks the pointer, `app.PointerDelta(dx, dy)` sends relative motion to the locked view. Headless tests enable it with `nsgotest.Environment(core.Environment{CanLockPointer: true})`. Escape releases the lock in both headless and target tests.

A press can name a text field directly. Headless and on macOS, Windows and Linux, read `app.Find("id:field").Focused` immediately after the press, including when the pressed view's handler focuses the field.

`app.SelectTab("id:tabs", 1)` selects the second item of a mounted `mobile.TabBar`. It works headless with an iOS or Android `Environment` and uses the native tab bar on iOS and Android targets. The index starts at zero and must match an existing item.

`app.Focus(selector)` reaches a field as a tap does and, in a desktop or web `Environment` or on those targets, a pressable or `Focusable` view through Tab, so a following `app.Key("Return")` presses it. A `Ref.Focus()` the app calls follows the same stops: fields everywhere, other views only on desktops, the web and Apple TV, so a headless test of a button's focus needs one of those environments. A pressable view has any of `OnPress`, `OnPressDown`, `OnPressUp` or `OnLongPress`, as on the platforms; Return or Space on it sends the down, up and press events it handles, so a view with only `OnLongPress` takes the focus but no key presses it.

Headless runs use the phone idiom by default, whatever the project targets, so `cx.Env().Desktop()` is false and hover tooltips never show. Pass `nsgotest.Idiom(core.IdiomDesktop)` to test a desktop presentation, keeping the rest of the environment, an `Environment` option's included. Keyboard focus follows the environment's OS, not its idiom.

On macOS target tests, the test window ignores AppKit's screen-size constraint, so `nsgotest.Size` and `Resize` keep the requested content size even when it is taller than the visible screen.

On a real target, `nsgotest.Environment` pins the appearance, locale, text size, reduced motion, increased contrast, thermal state, low-power mode, the login launch reason, the application in front and whether the platform follows it, and the device's name and model. The rest stays the target's own: its OS, idiom, safe area and system header heights. An `Environment` with `core.OSWindows` therefore does not make `go tool nsgo test -target macos` lay the app out as on Windows; run that test headless, where the whole `Environment` applies, or with `-target windows`. `nsgotest.Idiom` likewise applies only headless. `nsgotest.Size` sizes the test's window on macOS, Windows, Linux and the web; Android, iOS, tvOS and watchOS keep the device's screen. On `-target macos` the app opens its windows as `nsgo.Run` does, without an `Environment`: a root view or the view `SetMainView` sets shows in the main window, where `desktop.CurrentWindow` and `OnClosing` work, which opens at 480 by 640 points unless `nsgotest.Size` sizes it, and steps act on it, as headless. Closing it, from its close button or a Close item's shortcut, asks its `OnClosing` handlers; once it closed no view is left to act on, and a quit the app asks for, as its last window closes, counts in `app.Quits` instead of ending the test.

`app.Key("mod+a")` selects the focused field's text, and `mod+c`, `mod+x` and `mod+v` copy, cut and paste with `app.Clipboard`, when no binding takes the key and no application-menu item has it as its shortcut. In a macOS, Windows or Linux `Environment`, a key that no binding, focused view or field takes chooses the enabled application-menu item it is the shortcut of, so `app.Key("mod+r")` tests that a menu's shortcut reaches its item: a disabled item takes nothing, and while a field has the focus, a `NotInText` item takes nothing and keeps its key from the items after it, and on Linux no item takes a key pressed without Control, Alt or the logo key. Headless, an item with a standard role and no `OnSelect`, chosen by `app.ChooseMenu` or its shortcut, does what the desktops do with it: Close closes the main window through its `OnClosing` handlers, Quit asks the `OnQuitRequest` handlers and an approved request counts in `app.Quits`, Minimize, Zoom and Toggle Full Screen change the main window's state its environment reports, the app's other windows staying as they are (`app.SetWindowEnvironment` changes any one window's), and the editing roles act on the focused field. Any other role, and Undo or Redo in a field, fails the step rather than swallow its key; under `-target` such an item skips the test. Headless, on desktop targets and on the web, a `KeyAlways` binding takes Return, Backspace and select-all before the field acts; `KeyAuto` and `KeyNotEditing` leave them to the focused field. The Android and iOS test hosts give `KeyAlways` bindings first for Backspace and select-all; Android Control shortcuts then reach native editing, and iOS routes unbound keys through its software keyboard. Software Return on iOS does not give the keymap the key first yet ([#1351](https://github.com/nsgo-dev/nsgo/issues/1351)). A focused view's `OnKeyDown` hears `app.Key` first, a modifier alone (`app.Key("shift")`) included, and a key it consumes reaches no binding, headless and on desktop and web targets. `app.Key` is a full press, so the view's `OnKeyUp` hears the key come up after it. `app.KeyDown` and `app.KeyUp` press one key and release it separately, never a chord: a modifier `KeyDown` holds joins every key after it until its `KeyUp`, so `app.KeyDown("shift")` then `app.Key("a")` presses Shift+A, and `KeyUp` of a key not held fails the step. Keys still held as the test ends are released then, under `-target` through the platform, so the view that heard them go down hears them come up and none stays held into the next test. `core.KeysDown` and `KeyHeld` read what they hold. Under `-target`, Android, HarmonyOS and watchOS skip a test with them. `app.MouseDown`, `app.MouseDownAt`, `app.MouseDownWith`, `app.MouseDownAtWith` and their `MouseUp` counterparts press and release any of the five mouse buttons (`core.MouseButtonLeft` through `core.MouseButtonForward`), updating `core.MouseButtonsDown` and `core.MouseButtonHeld`; `app.BlurWindow()` simulates window focus loss and clears all held keys and mouse buttons.

In a headless test, `app.KeyCode("KeyW")` presses a physical position and `KeyCodeDown` and `KeyCodeUp` hold and release it separately. Set `core.Environment.KeyCodeNames`, for example `map[string]string{"KeyW": "z"}`, to exercise another layout. Unspecified positions use the US layout.

Under `-target` on macOS, Windows, Linux and the web, `app.Select(selector, start, end)` moves the native field's selection with its editing keys, select-all and the arrows, so the next typing, copying or pasting acts on the range the test named; a binding or application-menu item that takes those keys takes them from it. Phone targets do not move the native selection yet: there it moves the test's model alone, and skips the test when the field has `OnSelectionChange`.

Headless Linux and Android tests can check the package paths passed to the installer in `app.Installs`. Set `app.InstallPackage` to return an error from the simulated installer.

`app.Orientation` holds the active `Scope.RequestOrientation` mask. It is zero when the app uses its usual orientation policy.

`app.Find(selector).Kind` is a `nsgotest.ViewKind`; compare it with constants such as `nsgotest.KindText`, `KindInput`, `KindCanvas` or `KindGPU` when a test needs to inspect the mounted view form.

Headless GPU tests can use `app.Surface("title:Inspector").GPUReadPixels("id:viewport")` for a window's software GPU pixels. `Surface.TriggerGPUDeviceLost` and `Surface.RestoreGPUDevice` simulate loss and recovery in that window, using its pixel ratio. Devices and readback callbacks stay separate even when view IDs repeat across windows; `app.GPUFrames()` records submissions across the whole app. These helpers operate on the headless software GPU model.

## Widgets and Live Activities

`nsgotest.Widgets(true)` records what the app publishes with `widget.Update`. Read the latest timeline for each kind with `app.Widgets()`. An entry with no date gets the app's wall time. Use `nsgotest.Widgets(false)` to make `widget.Available()` false and have updates return `widget.ErrNoWidgets`.

`nsgotest.LiveActivities(true)` records starts, updates, alerts and ends. `app.LiveActivities()` returns the activities in start order, with their content changes and any final content. Use `nsgotest.LiveActivities(false)` to make starts return `widget.ErrActivitiesDisabled`. These options take over the corresponding process-wide service for the test, including on a native target. Run tests that call these services sequentially.

Headless tests simulate game controllers with `app.ConnectGamepad(id, name)`, `app.SetGamepadInfo(id, family, vendorID, productID, canRumble)`, `app.DisconnectGamepad(id)`, `app.SetGamepadButton(id, btn, value)` and `app.SetGamepadAxis(id, axis, value)`: connect, info updates and disconnect settle and rerender scopes reading `gamepad.Pads`, while button and axis updates take effect on the next frame for `gamepad.State`. `app.GamepadRumbles()` and `app.LastGamepadRumble(id)` inspect recorded `gamepad.RumbleEvent` start and stop commands. Under `-target` they skip the test.

## Run the tests

```sh
go tool nsgo test
```

`test` prints a line per test. `-run` picks tests as `go test`'s does, including subtests such as `-run 'TestGallery/dark-home'`. On a target, `-timeout` sets the per-test deadline (three minutes by default; `0` disables it) and the result reports when that deadline is reached. On a simulator or a phone, a test whose `TestMain` calls `nsgotest.Main` has its deadline start when the test host starts, so a slow app launch does not count against it; the launch has a limit of its own, five minutes, and reaching it fails the test as a launch timeout. A test whose subtests all skipped counts as skipped. `-junit report.xml` writes a JUnit report that keeps each skip's reason, `-trace` prints each step's mount items and commit times and `-bench` runs benchmarks instead, a line per result. Headless runs forward flags after `--` to `go test`; target runs accept the runtime flags below. Compiler errors appear in the console and JUnit report even when a headless run only compiles its tests with `-- -c`.

Given a directory without `nsgo.yaml`, such as a library or the module's root, `test` takes every package under it; a directory may also end in `/...`, as Go's patterns do. It takes one directory, and stops with an error at a second: to test several, give it a directory above them, or run it once for each. `go tool nsgo check` takes packages the same way and vets them.

Tests check reactive graph ownership: access signals and scopes on the app's goroutine, and use `cx.Post` from workers. A call from another goroutine that lands while the app is idle panics with a `Scope.Post` diagnostic; `-race` catches the rest. Internal engine panics still fail tests; in a shipped app they stop the affected engine and report its stack to the host. Development builds also check ownership; release builds omit the check; WebAssembly development and test builds omit the goroutine-ID assertion because browser callbacks may resume on a browser-managed goroutine; the other test checks still run there.

`app.Post(func() { state.Set(value) })` queues work on the app's graph goroutine as the next step or wait settles: the test goroutine headless, the engine goroutine on a target. Call it from a test or a stubbed worker, then use `app.Settle()` or a wait to read the resulting views. The function may post more, which the same step runs; it takes no steps and does not call `t.Fatal`, as on a target it runs off the test goroutine.

Headless, each step then moves the clock a frame at a time, for at most 2 seconds, until the animations that end have ended and each style transition has run its `OnTransitionEnd`: a dialog or popover closing through its `EndingStyle` is gone when `app.Press` returns. To look at them on the way, start with `nsgotest.ManualFrames()`; they then move only as `app.Advance` moves the clock.

## Time

Headless and on a target, the app runs on `app.Clock`, a `core.ManualClock` that starts at 2026-01-01 00:00 UTC and moves only as the test moves it. `app.Advance(d)` moves it by `d`, running each timer when it comes due and, while the app waits for one, a display frame every frame interval, as a display does. `app.FastForward(d)` jumps ahead, as a laptop closed for a while and opened: each timer that came due on the way runs once, at the end, and so does one display frame.

The app reads this clock with `cx.Now()`, and the date it shows with `cx.WallTime()`, so time what it measures itself, such as a gesture's speed or a double tap's window, with them. `time.Now()` and `time.Sleep` follow the system's clock, which `Advance` does not move: an app timing with `time.Now()` misses the time a test moves, and a `time.Sleep` in a test only makes it slower. Wait for work that arrives from another goroutine, such as a `cx.Post` after a reply, with `app.WaitFor`, `app.WaitForGone` or `app.WaitUntil`, whose timeouts count real time.

Headless, these waits advance the clock a frame at a time between checks. Start with `nsgotest.ManualFrames()` to keep the clock still during a wait. On a target the waits settle the app while the platform runs its own animations.

A screen that depends on the date, such as what is due today, starts with `nsgotest.WallTime(t0)`: the first render's `cx.WallTime()` is `t0`, in its location, and timers keep running on the clock's own time. `nsgotest.Clock(c)` runs the app on a clock of the test's own, so code outside the app shares it; the app's timers fire as `c.Advance` moves it. A measurement on a target, started with `nsgotest.Measure()`, runs the app on the system's clock instead, as described below.

## Run on a platform

Tests run in a real window with the platform's own input. Discovery follows the target's build tags, including target-only test files in otherwise portable packages:

```sh
go tool nsgo test -target macos
```

On macOS, test windows ignore mouse, keyboard and trackpad input from outside the test host, so using the machine while a suite runs does not change its scenes. The test's own steps still work.

Test windows take the test's keys without activating the app, so the app you work in stays in front. AppKit may refuse a field's click while another app is active. `Focus` and `Type` diagnose that refusal. `go tool nsgo test -target macos -activate` lets the test app activate and take the keyboard before it clicks a field or presses keys. It needs an unlocked desktop session, and tests that import `github.com/nsgo-dev/nsgo` (a blank import is enough) and start `nsgotest.Main` or `nsgotest.MainFunc` from `TestMain`.

Cleanup closes native context menus left open, so they cannot take the next subtest's first click or key. On macOS a WindowServer capture includes the test window's open native popup, clipped to the window's bounds; other windows are excluded.

The targets are `macos`, `windows`, `linux`, `web` (in Chrome, Chromium or Edge), `ios` (on a connected iPhone, `-device` naming it when there are several, signed with the team Xcode last used or `-team`'s), `ios_simulator`, `tvos_simulator` (the remote's arrows, Return as its select button and Escape as Menu), `watchos_simulator` and `android`, or `all`. Each test runs in a process of its own. `app.Capture()` returns the current pixels as an `image.Image` and `app.CapturePNG()` the PNG bytes, both skipping the test headless. On macOS they are the window as WindowServer composites it, Core Animation's shadows, masks, filters, animations and caret included; when it cannot capture the window, one that is minimized or on another Space among them, the view draws itself through AppKit instead, without those, and the first time logs so. NSGo converts both macOS capture paths to sRGB before encoding. iOS and tvOS also capture in sRGB, so pixel values on these targets do not depend on whether the display uses Display P3. `app.Screenshot("en/library")` saves a screenshot as `.nsgo/test/screenshots/<target>/en/library.png`, `.nsgo/test/screenshots/macos/en/library.png` on `macos`; names may contain spaces but cannot be absolute or contain `..`. `-screenshots dir` saves them under `dir` instead, a path relative to the working directory. A screenshot that a device's log dropped or cut part of fails the test before a file is written, and a screenshot whose path an earlier one took fails its test. `-fps` logs the performance monitor's readings each second on `macos`, `windows`, `linux` and `ios_simulator`, and the engine's alone on `web` and `android`. `-env KEY=VALUE` sets a variable of the tests' environment, which a phone's tests see no other way. On a simulator or a phone, the tests of a directory without `nsgo.yaml` install as an app named after the directory, with the ID `dev.nsgo.` followed by the name's letters and digits: `dev.nsgo.formula` for `base/formula`. On `android` and the simulators the tests install under the app's ID followed by `.nsgotest`, an App Clip's under that ID too, beside an installed build of the app rather than over it, and are uninstalled when the run ends. On `ios` they install under the app's own ID, as the provisioning profile names it, replacing the build installed on the iPhone and its data.

On `android` and `ios` the tests run only in NSGo's test host: Android's runtime reads its protocol from the test process's standard output, and the tool learns a test's output and exit code there from the marks the host writes. A package whose tests run on them must link the host, by importing `github.com/nsgo-dev/nsgo` in a test file (a blank import is enough), and start it from `TestMain` with `nsgotest.Main` or `nsgotest.MainFunc`. Before building or installing, `nsgo test` checks that the host is linked and `TestMain` is declared. If that function bypasses the host, Android reports the invalid stdout protocol with a `nsgotest.Main` hint; keep logs and setup output on stderr until the host starts. A library package such as `ui/chat` runs on a phone once a test file adds both, and headless or on the other targets as it is.

Packages that do not link the platform backend run headless in the target's runtime. With `-target web`, library controls can be tested inside the browser this way. Storage stays in memory, and `DataDir`, `CacheDir` and `LogsDir` are empty because the browser has no filesystem. The file-backed `nsgotest.MicrophoneInput` fake skips there because it needs real WAV files.

An Android test that panics or exits before reporting its result fails with the process's exit status and its captured output, including the panic stack. A `TestMain` that exits without starting the host fails this way too, even with status 0. When an Android target test reaches its timeout, the result also says whether the host printed its start mark and includes the tail of that test's `logcat` output before the runner clears it.

On `web` the tests run in a headless Chrome, Chromium or Edge the tool finds, or in the browser executable `NSGO_BROWSER` names. On macOS the tool prefers Chrome for Testing when installed. The tool waits up to 30 seconds for the browser to start, stops waiting for startup when the command is cancelled, and reports the browser's error output if it exits or never answers.

Each step, and `app.Settle()`, waits for the engine's pending commit before counting 100 ms without batches or finite animations. The whole wait stops after five seconds even if the app keeps changing; a slow first dialog needs no extra post or sleep in the test.

On Android, the tool wakes a sleeping screen and dismisses a lock screen without a credential before building and before each test. A device locked with a PIN, password or pattern must be unlocked before the run can continue; the error names the device. A transient `adb` state-read failure is retried, and a device that remains unreachable stops the target run once instead of failing every remaining test.

To measure rather than check, start with `nsgotest.Measure()`: the app runs on the system's clock, `Advance` waits as long as it moves, and steps wait only for what animates to rest. `app.Mark("name")` names what the readings from there on measure, and `app.Mark("-")` ends it; the marks print with or without `-fps`.

Put NSGo flags before the directory: `go tool nsgo test -target web ./app`, not after it. For headless runs, flags after `--` go to `go test`. Target runs support `-short`, `-count`, `-cpu`, `-parallel`, `-failfast` and `-fullpath` there, for example `go tool nsgo test -target web ./app -- -short -count=3`; they reject other Go test flags explicitly, `-run` and `-timeout` included, which go before the directory there. `-count=0` runs no tests. Tests that link NSGo's native host repeat in fresh processes for each count and CPU setting, with `[run N]` in their results, screenshots in `run-N` subdirectories and a timeout per invocation; libraries keep Go's in-process repetitions.

## TSX tests

A TSX app's `*.test.tsx` files, written with `nsgo/test`, run with its Go tests.

`open("App.tsx", { clock: {} })` runs the app on the manual clock, and `app.clock` moves it: `app.clock.runFor(ms)`, like `app.wait(ms)`, moves it a frame at a time, as `Advance` does, and `app.clock.fastForward(ms)` jumps ahead, as `FastForward` does. `clock: { time }` starts what `Date.now()` and `new Date()` read at `time`, a `Date` or milliseconds since 1970, and `app.clock.setSystemTime(time)` sets it later; neither moves a timer. Without `clock`, a headless app's time also passes while `waitFor` and `waitUntil` wait, a frame at a time, as it would on a device.

`open("App.tsx", { camera: true })` gives the test environment a camera so `Picker.canTakePhoto()` resolves `true` and `Picker.takePhoto` is recorded in `app.picks()` (with `camera: true`) and answered by `app.onPick`: return a `PickFile` or `PickFile[]` for the taken photo, `null` or `[]` to cancel, or throw an error to reject `Picker.takePhoto`. Without `camera: true`, `Picker.canTakePhoto()` is `false` and `Picker.takePhoto()` rejects.

To script a TSX app without a test file, pass its entry and the steps on standard input, written as a `*.test.tsx` test's body with `app` the app:

```sh
go tool nsgo test App.tsx < steps.ts
```
