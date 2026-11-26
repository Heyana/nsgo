---
title: Running and debugging
description: Run an app on any target with hot reload, and see its errors, updates, layout, speed and other languages.
group: Toolchain
order: 20
---

## Run

```sh
nsgo dev
```

`dev` builds and opens the app for this machine, or for the target `nsgo.yaml`'s `dev.target` names. With hot reload enabled (the default), saving a Go file or an embedded file in a Go-only app rebuilds and restarts the app. When an import needs a module missing from `go.mod` or `go.sum`, `dev` runs `go mod tidy` first. Changes only to a TSX bundle's sources update the running app in place, keeping its state. The run ends with the app.

On macOS and Windows, a native dev rebuild reopens the main view’s window at its last regular position and size within the current session. Windows also restores full screen. On Linux, GTK restores its regular size and requests its last full-screen mode. The window manager chooses the position.

`-target` runs it elsewhere, and `-device` picks the iPhone, Apple TV, Apple Watch, simulator or Android device by name, UDID or serial:

```sh
nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
nsgo dev -target ios_simulator      # in the iOS simulator
nsgo dev -target ios                # on the connected iPhone, signed by Xcode
nsgo dev -target tvos_simulator     # in the Apple TV simulator
nsgo dev -target watchos_simulator  # in the Apple Watch simulator
nsgo dev -target android            # on a connected device or emulator, over adb
nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

The dev server listens on all interfaces by default. Use `-host 127.0.0.1` to keep it on loopback, including when Android connects through `adb reverse`, or name a local interface's IP address to listen only there. Use an address reachable over the LAN for dev features on physical Apple devices.

`-h` lists every flag.

## Go debugger

Put a [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) version compatible with your Go version on `PATH` as `dlv`. Start a native desktop app with its debugging information and with Go optimization disabled.

```sh
nsgo dev -target native -debug
```

Copy the port from Delve's `API server listening at` line. In VS Code with the Go extension, add an attach configuration and replace `40000` with that port.

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

In GoLand, add a **Go Remote** run configuration with host `127.0.0.1` and the same port. Set a breakpoint in your app's Go code, attach, and run the action that reaches it. After a successful Go rebuild, reconnect the debugger at the same address so it sends the breakpoints to the new process. TSX changes still use hot reload.

`-debug` runs on macOS, Windows and Linux with `-target native`. Use a dev build for debugging and reserve `-release` for measuring a release build.

## Component previews

Put one or more `PreviewXxx` functions beside the components in an importable Go package. Save them in a Go file guarded by the preview build tag.

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
nsgo dev -preview ./components
nsgo dev -target web -preview ./components
```

Check the fixtures you want to display together and choose a locale, theme, light or dark appearance, viewport size and layout direction. The preview takes the closest app's `nsgo.yaml`, its translation catalogs, configured `theme` and resources. Pass an app directory after the flags to select a different configuration in the same module. In that case, the preview path is relative to the app directory.

Saving a file rebuilds the preview and keeps your selection and display settings. The `nsgopreview` tag excludes the fixtures from ordinary release builds. Use the same fixture in a tagged `_test.go` file with `nsgotest.Start(t, PreviewEmpty)` and run it with `GOFLAGS=-tags=nsgopreview nsgo test -target macos ./components` to take a native screenshot.

After the main app's first install with `-target ios_simulator`, a Go-only rebuild replaces the installed executable and relaunches the app. Editing `nsgo.yaml`, packaged resources or the app icon triggers a reinstall.

Use `nsgo dev -target android -android-host` for Android UI work with a shorter Go rebuild loop. The tool installs the native runtime once and runs Go on the development machine. Each Go rebuild reconnects to the same Activity over adb. Changes to packaged resources, precompiled classes, launcher icons or `nsgo.yaml` install the runtime APK again. The Go process uses the host's build constraints, files and networking. `Scope.DiskSpace` reports the volume containing the host's `DataDir`. Use the regular Android dev command to exercise `_android.go` files and on-device Go behavior.

## Errors

A failed build shows over the running app, with buttons to copy the error and to build again; fix the file and the next save replaces it. A component that panics while it renders shows the same way. Command+R in the app, Control+R off Apple platforms, builds and starts it again at any time, over the error too, unless the app binds the key itself, with `cx.OnKey` or as an enabled item of its application menu; in the browser the key reloads the page as usual. The app's own output, such as `log.Print`, appears in the terminal under the tool's lines.

Before running, `nsgo check` vets the app and type-checks its TSX; `-target all` vets it for every target, so code behind another platform's build tags is checked from a Mac too.

### Packaged Apple apps’ logs

On macOS, iOS, tvOS and watchOS, the default Go logger writes to Apple’s unified log, including `log.Print`, NSGo diagnostics and TSX console output. In Console, filter by the running app’s bundle identifier as the subsystem and `nsgo` as the category. On macOS you can also query the store:

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

Records use the default log type and retain their warning or error prefixes. Their text is public, so keep credentials and other secrets out of logs. Long messages become several entries. Terminal output remains available during development; a custom log writer or `slog` handler controls its own output.

### Linux web views

Ubuntu 24.04 and later can restrict unprivileged user namespaces through AppArmor. When bubblewrap cannot start WebKitGTK's sandbox, NSGo logs the error and shows a placeholder. Some WebKitGTK versions otherwise exit with `SIGTRAP` while opening the first web view. [Ubuntu AppArmor documentation](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

Use your distribution's bubblewrap AppArmor profile if it provides one, such as `bwrap-userns-restrict`. Otherwise, an administrator can load a profile granting `userns` to `/usr/bin/bwrap` using [the profile from NSGo's Linux CI](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml) as a starting point. The administrator manages the profile because it applies to every app using the system's bubblewrap.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` turns off WebKit's subprocess sandbox. Use it only for local development with trusted content.

## DevTools

```sh
nsgo dev -devtools
```

`-devtools` opens DevTools in the browser, served by `nsgo dev` for every target, a phone's app too. It shows the running app as it runs:

- **Elements**: the views in a tree. Each shows the component that built it, a link to its line in your editor, the selector a test finds it by, its classes, its bounds, its margin, border and padding as laid out, and the style it sets. Hovering a row draws its margin, padding and content in the app, as a browser's DevTools do, and **Pick** lets you press a view in any window, panel, sheet or embedded view to find it in the tree.
- **State**: the signals your code made, under the components and computations that own them, with their values, memos' values, and how many computations read each. A value no computation reads changes nothing on screen. Strings, booleans and numbers can be edited, so you can try a state without a code change. Values NSGo made for itself are behind **NSGo's own**.
- **Profiler**: each commit's time, split into your code before it, the commit with its layout, and applying it on the platform's main thread. It also shows the lines whose writes caused the commit, and opens on the computations it ran, the longest first. A second list names the computations that ran most in the last second, by the line that made them. Android's runtime applies batches in its own Java code, so Android commits have no platform time.
- **Recorder**: what you press, type, clear and scroll in the app, written as an `nsgotest` test file to copy into a `_test.go` file. In Elements, **Assert shown** and **Assert text** add a check of the selected view. A text check holds the text in the language the app showed it in, so record with the language your tests run in.
- **Highlight updates** outlines each view a commit changes, as `-highlight` does.
- **Layout** shows and hides the [layout audit](#layout) on the device, as `-layout` starts it.

A TSX app's state, computations and writes are named by their TSX lines, through the bundles' source maps, and link to the files in your app.

DevTools read the app through `nsgo dev`'s server, which listens on your network so phones reach it. Each run makes a token, which the addresses `nsgo dev` prints and opens for DevTools and the web target carry after `#`, and the server answers only requests that hold it, so open DevTools and the web target from those addresses. Anyone you give one to can change the app's state while it runs and add steps to the test the Recorder writes, so read a recorded test before you run it. Others on your network and web pages of other sites you visit reach none of it.

## Updates

```sh
nsgo dev -highlight
```

`-highlight` outlines for half a second each view a commit changed, so an update that redraws more than it should shows on screen as you use the app. The outlines draw over every window, panel, modal, iOS sheet and embedded view (`nsgo.NewView`). DevTools turn them on and off while the app runs.

```sh
nsgo dev -trace
```

`-trace` prints each commit of the running app: its number, its time, and the mount items it sent, so you see which views an update created, changed or removed. Above the items it names what the commit followed from: the lines of your code that wrote signals since the last commit, the most writing first, and how many bindings, effects and memos ran.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

A line that writes many times, or many bindings run for one write, is where to batch writes (`cx.Batch`) or narrow what a binding reads. `nsgo test -trace` prints the same under each step of a test.

## Runtime profiles

Capture a running app's Go CPU activity, heap, or execution trace with `nsgo dev`. The app sends the capture to the dev server, which saves it on the machine running the command.

```sh
nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

The CPU example also works on a desktop, an iPhone, or an iOS simulator. On an iPhone, allow Local Network access if the system prompts for it. The heap is a snapshot and works on the web too. For scheduler and goroutine events, run `nsgo dev -profile trace` and open `.nsgo/profiles/execution.trace` with `go tool trace`. CPU and execution traces default to 10 seconds. `-profile-output` sets the local file's path. For component previews, the default and relative paths use the app directory. A dev run saves its first complete capture, then continues running the app.

```sh
nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Go's browser runtime supports heap profiles and execution traces. For browser CPU samples, use the browser's Performance tools. Choose either `-profile` or `-release`. Shipping with `nsgo build` leaves the dev capture client out. [Go runtime diagnostics](https://go.dev/doc/diagnostics)

## Layout

```sh
nsgo dev -layout
nsgo dev -layout=bounds,grid=4
```

`-layout` starts the app with DevTools' layout audit over every window, panel, iOS sheet and embedded view, which outlines every view: its border box in blue, the inner edge of its padding in green and the outer edge of its margin in orange, each cut where a scroll view or a view that clips hides it. `grid` draws a grid of 8 points, and `grid=N` one of N points, at least 4. Give the value after `=`: the flag alone means `bounds`.

A readout in the bottom corner shows the window's size in points, with buttons that turn the boxes, the grid and the ruler on and off. On Apple TV and other remote-only devices, focus reaches the readout's buttons so you can toggle them with the remote without leaving the app. While the ruler is on, presses pick two points instead of reaching the app, and the readout shows how far apart they are across and down; a third press starts again. On Apple TV or with a keyboard, directional arrows (or swiping the Siri Remote touchpad) move a crosshair across the screen and Select, Return or Space picks points, updating the distance live until the second point locks it.

| Key | Effect |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | Shows or hides the audit, in a build with `-layout` |
| Arrows, remote D-pad / swipe | Moves the ruler's crosshair by the grid step (8 pt by default) |
| `Shift` + Arrows | Moves the ruler's crosshair by 1 pt |
| Select, `Return`, `Space` | Picks the crosshair's point while measuring |
| `Esc`, back, Menu | Stops measuring |

If your app binds the same key, the app's binding wins. Without the key, as on a phone, run with `-devtools` too and show and hide the audit with DevTools' **Layout**; hide it to reach a control under the readout. Native backends and the web read presentation matrices so bounds, Pick and the audit follow displayed transforms, animation and scrolling; watchOS follows its affine drawing. HarmonyOS and mini programs retain the Go-side tracker until their render layers report geometry. Redrawing them costs every update time, so measure speed without them.

## Speed

```sh
nsgo dev -fps
nsgo dev -release -fps
```

`-fps` shows the performance monitor over the app on macOS, iOS, Windows, Linux, Android and the web. It measures frames and commits, and Android also shows native text round-trip time. Dev builds carry the dev client; measure with `-release`, which builds as `nsgo build` does.

## Languages and appearance

```sh
nsgo dev -lang zh-Hans
nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
nsgo dev -lang ar-XB   # right to left
```

`-lang` starts the app in a locale, a pseudo locale too ([Localization](localization.md)).

## Agents

```sh
nsgo dev -automation
nsgo dev -target android -automation
nsgo dev -target web -automation
```

`-automation` lets an AI agent or an end-to-end runner use the running app as a person does, on macOS, Windows, Linux (`-target native`), Android (`-target android`), the web (`-target web`), mini programs in their developer tools' simulators (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` or `miniprogram_douyin`) and Apple devices and simulators (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator` or `watchos_simulator`) so far: list its windows, outline what one shows, press, type, send keys, scroll, drag, hover, choose menu items, answer alerts, wait for what appears and take screenshots. Agents reach it over MCP through `nsgo mcp`, which uses the `dev -automation` running for the directory, or starts one with the same flags (such as `-target android`) and stops it as the client leaves:

```sh
claude mcp add nsgo -- nsgo mcp
```

Other clients take the same command, `go` with the arguments `tool nsgo mcp`, run in the app's directory. Views are named with the selectors of [tests](testing.md#a-test), or with the `view:` references a snapshot prints, which a save makes stale as it restarts the app.

The agent acts on the app's real data, as `dev` runs it. Only `dev -automation` builds carry it, and only tools on this machine holding the token in `.nsgo/automation.yaml`, which only you can read, reach it.

### App-defined MCP tools

Call `automation.Register` from a Go app or library on the engine goroutine. For example, this function gives agents the ID of the scene currently shown.

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

The name must be unique on the app's graph and distinct from the built-in names. `nsgo mcp` lists the tool until the scope is cleaned up. Set exactly one of `Handler` and `Start`. Both run on the engine goroutine, so they can read or set signals. Both receive a JSON object and check the arguments they need. `Handler` returns a JSON-encodable value or an error directly; it must not block waiting for `Scope.Post`, `audio.LoadSound` or other work that calls back on the engine goroutine.

For a deferred result, use `Start` instead. Start the work, return without waiting, and call `reply` once when the result is ready. For example, replace the `Handler` above with:

```go
Start: func(ctx context.Context, _ json.RawMessage, reply func(any, error)) {
    cx.Post(func() {
        if ctx.Err() == nil {
            reply(map[string]string{"id": sceneID.Get()}, nil)
        }
    })
},
```

You can reply from any goroutine. MCP waits up to one minute, including time queued on the engine. Scope cleanup ends a pending call with an error. Duplicate replies and replies after cancellation or cleanup are ignored.

In headless or target tests, capture the scope when the app starts. Call `automation.Call(cx, ctx, name, args)` for a `Handler` inside `app.Post`, or `automation.CallAsync(cx, ctx, name, args, done)` for a `Start`. The `done` callback may run on any goroutine; send the result to a buffered channel without blocking. Use `app.Settle()` for work queued on the graph, or `app.WaitUntil` to drive the app while waiting for a result from another goroutine. Registration is active in `dev -automation` and tests and is a no-op in release builds.

## Flags of every build

Every build of an app, packages included, takes NSGo's own flags as it starts, which override `nsgo.yaml` and the app's code and never reach the app's own flags:

| Flag | Effect |
| --- | --- |
| `--nsgo-fps` | Shows the performance monitor on macOS, iOS, Windows, Linux, Android and the web |
| `--nsgo-lang=zh-Hans` | Starts the app in a locale, a pseudo locale too |
| `--nsgo-appearance=dark` | Starts the app `light`, `dark` or following the `system` |
| `--nsgo-os=ios` | Runs a web build as `macos`, `windows`, `linux`, `ios` or `android`, `web` being itself |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

On the web they go in the page's query: `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os` previews another platform's layout in a browser: the app reads that OS and its idiom, a phone for iOS and Android and a desktop otherwise, and `Env.Simulated` reports true. The browser has none of that platform's native parts: `NavHeader` and the drawn tab bar take the place of the system's, there is no application menu, no native popup menu (`desktop.PopupMenu`) and no window functions, and `mobile.IsAvailable` and `desktop.IsAvailable` are false. Drawn menus such as `ui.ContextMenu` and `ui.Menu` still work. Shortcuts follow the visitor's keyboard. Other builds ignore the flag and log a warning.
