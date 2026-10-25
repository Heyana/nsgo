# Roadmap

This plan lists what NSGo still lacks compared with NSH's `main` at `2c2a1767`, in the order to build it. Each item names its NSH reference and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

References are files in `nsh/docs/dev/` unless they name a crate, with the bullet or section to read in brackets. To refresh the plan, read `git log 2c2a1767..` in `nsh/` and the documents it changed.

## Order

Performance comes next. The last section waits for community contributions, and nothing above it depends on it.

## 1. Performance

| Item | NSH reference | Done when |
| --- | --- | --- |
| Incremental layout | `pitfalls.md` (relayout boundaries), `performance.md` | Changes stop at fixed-size flex items, and a text that measures as before lays nothing out |
| Measure cache | `architecture.md` (data flow and threads) | Text measurements are cached by content and width per engine, so identical texts and list rows measure once |
| Paced list binds | `scrolling-and-lists.md` (paced binding) | A list binds its viewport first and its overscan over later frames |
| macOS frames | `macos.md` (architecture) | Scrolls move the container layer and commit at rest; batches take at most 4 ms a frame; resizes wait for layout; a timer covers a stalled display link |
| macOS canvas redraws | `canvas.md` (partial redraws) | A canvas redraws only the area that changed |
| Commits that follow changes | `performance.md` (Rust components run once) | A commit resolves styles and emits items only for the nodes that changed, not the whole tree |
| Windows frames | `windows.md` (frames) | The frame clock follows the compositor with `DwmFlush` |

## 2. Finish the existing platforms

These items wait for community contributions.

| Item | NSH reference | Done when |
| --- | --- | --- |
| iOS touch | `crates/nsh-platform-ios` | Gestures, text input and selection are checked in the simulator; selections have touch handles; hover, swipe back and touches outside the parent work |
| iOS system UI | `elements.md` (overlays), `pitfalls.md` | VoiceOver's escape gesture goes back and closes popups; inputs in scroll views stay clear of the keyboard |
| Text selection | `text.md` (selection areas) | Selectable text and selection areas work on Windows, Linux and Android |
| Desktop accessibility | `windows.md`, `linux.md` | Windows exposes UI Automation and Linux GTK's accessibility, canvas nodes included, and both announce |
| Desktop chrome | `windows.md` (alerts), `linux.md` | Windows draws alerts in the surface; Linux animates modals |
| Effects | `styles.md` (effects) | Windows draws backdrop filters and blend modes, Linux blend modes, and Android drop shadows from Android 12 |
| Android | `elements.md`, `styles.md` (Android differences) | Ripples, predictive back on screens, blurred images before Android 12, layout animations aligned with the IME's motion (`WindowInsetsAnimation`), and a forced appearance in the activity's theme and web views |
| Android frames | `elements.md` (interaction and the environment) | `RequestAnimationFrame` runs on `Choreographer` frames |
| Web | `web.md` | Browser history and back keys, a focus state for pressables, local time with daylight saving |
| Real file drags | `file-drop.md` | A real drag from Finder, Explorer and a Linux file manager is checked to reach a view |
| Embedding | `embedding.md` (the iOS SDK) | iOS frameworks add x86_64 simulator, Catalyst and dynamic slices; a Linux view forces its appearance on its hosted controls; a page shows several web apps |
| Tests on targets | `tooling.md` (on a real platform) | `nsgo test -target` chooses native menus' items, runs a test's several apps and its manual clock, and keeps the clipboard the test's |

## Not ported

Go apps do not need these NSH parts:

- The JS runtime, bundler, TSX compiler, `nsh check`, compatibility baselines and hot module replacement, since apps are Go and `go vet` checks them.
- Tailwind classes, since styles are Go values.
- `fetch`, WebSockets and `resource`, since apps use `net/http` or Go libraries with `Scope.Post`.
- Component refs, slots, render props and typed rows, since Go fields and functions carry them.
- `EngineContext`, since a host gives each engine's app function its values directly.
- The GPUI backend and the LayerFS trial.
- The theme and styled components, which apps or a component library build on `base`.
