# Roadmap

This plan lists what NSGo still lacks compared with NSH's 0.3.0 release (tag `v0.3.0`, `76fa8a7f`), in the order to build it. Each item names its NSH reference and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

References are files in `nsh/docs/dev/` unless they give a path from `nsh/`, with the bullet or section to read in brackets. To refresh the plan for NSH's next release, check out its tag in `nsh/`, then read `git log v0.3.0..HEAD` there and the documents it changed.

## Finish the existing platforms

These items wait for community contributions.

| Item | NSH reference | Done when |
| --- | --- | --- |
| iOS touch | `crates/nsh-platform-ios` | Gestures, text input and selection are checked in the simulator; selections have touch handles; hover, swipe back and touches outside the parent work |
| iOS system UI | `elements.md` (overlays), `pitfalls.md` | VoiceOver's escape gesture goes back and closes popups; inputs in scroll views stay clear of the keyboard |
| Text selection | `text.md` (selectable views) | Selectable text and selectable views work on Windows, Linux and Android |
| Desktop accessibility | `windows.md`, `linux.md` | Windows exposes UI Automation and Linux GTK's accessibility, canvas nodes included, and both announce |
| Desktop chrome | `windows.md` (alerts), `linux.md` | Windows draws alerts in the surface; Linux animates modals |
| Effects | `styles.md` (effects) | Windows draws backdrop filters and blend modes, Linux blend modes, and Android drop shadows from Android 12 |
| Android | `elements.md`, `styles.md` (Android differences) | Ripples, predictive back on screens, blurred images before Android 12, layout animations aligned with the IME's motion (`WindowInsetsAnimation`), and a forced appearance in the activity's theme and web views |
| Android frames | `elements.md` (interaction and the environment) | `RequestAnimationFrame` runs on `Choreographer` frames |
| Web | `web.md` | Browser history and back keys, local time with daylight saving |
| Desktop touch | `linux.md`, `windows.md` (scroll views) | Touch drags scroll Windows and Linux views with rubber banding and pull to refresh, the app's refresh indicators and pull values included |
| Real file drags | `file-drop.md` | A real drag from Finder, Explorer and a Linux file manager is checked to reach a view |
| Embedding | `embedding.md` (the iOS SDK) | iOS frameworks add x86_64 simulator, Catalyst and dynamic slices; a Linux view forces its appearance on its hosted controls; a page shows several web apps |
| Tests on targets | `tooling.md` (on a real platform) | `nsgo test -target` chooses native menus' items, runs a test's several apps and its manual clock, and keeps the clipboard the test's |

## Not ported

NSGo leaves out these NSH parts:

- The JS runtime, bundler, TSX compiler, `nsh check`, compatibility baselines and hot module replacement, since apps are Go and `go vet` checks them.
- Tailwind classes, since styles are Go values.
- `fetch`, WebSockets, `resource` and the standard modules, since apps use Go's standard library or other Go libraries with `Scope.Post`.
- The machinery that carries a Rust component's refs, slots, render props and typed rows across JS, since a Go component takes them as fields: content as `core.Node` (`Button.Children`, `Popover.Trigger`, `Checkbox.Indicator`), rows and callbacks as functions.
- `EngineContext`, since a host gives each engine's app function its values directly.
- The GPUI backend and the LayerFS trial.
- The theme and styled components, which apps or a component library build on `base`.
- 32-bit Android (`armeabi-v7a`), since Go links android/arm only with cgo and an NDK.
