# Roadmap

This plan lists what NSGo still lacks compared with NSH's `main` at `2c2a1767`, in the order to build it. Each item names its NSH reference and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

References are files in `nsh/docs/dev/` unless they name a crate, with the bullet or section to read in brackets. To refresh the plan, read `git log 2c2a1767..` in `nsh/` and the documents it changed.

## Order

macOS scrolls come next. The last section waits for community contributions, and nothing above it depends on it.

## 1. Performance

| Item | NSH reference | Done when |
| --- | --- | --- |
| macOS scrolls | `macos.md` (architecture), `crates/nsh-platform-macos/src/scroll.rs` | Scroll views keep offsets of their own in place of `NSScrollView`'s clip view, shown by the container layer's `sublayerTransform` while they move and committed to its bounds origin at rest, with the system's trackpad phases and momentum, rubber banding and the indicator checked by hand on a trackpad |

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
