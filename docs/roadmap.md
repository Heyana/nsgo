# Roadmap

This plan lists what NSGo still lacks compared with NSH's `main` at `2c2a1767`, in the order to build it. Each item names its NSH reference and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

References are files in `nsh/docs/dev/` unless they name a crate, with the bullet or section to read in brackets. To refresh the plan, read `git log 2c2a1767..` in `nsh/` and the documents it changed.

## Order

Sections run in build order: services, the tool, embedding and performance. The last section waits for community contributions, and nothing above it depends on it.

## 1. Services and windows

| Item | NSH reference | Done when |
| --- | --- | --- |
| Clipboard | `crates/nsh-core/src/runtime/clipboard.rs` | Images and files besides text |
| Open URLs | `js-modules.md` (`nsh/device`) | `OpenURL` opens a link outside the app, through the embedding host when there is one |
| Photo picker | `js-modules.md` (`nsh/picker`) | The system's picker returns the photos a user chose as files |
| Image disk cache | `images.md` (disk cache) | Downloaded images stay on disk across launches under HTTP's caching rules |
| Environment | `crates/nsh-core/src/platform.rs` | The OS version and the mac and TV idioms |
| System toast | `crates/nsh-core/src/runtime/android.rs` | Android shows the platform's toast |
| Window commands | `crates/nsh-core/src/runtime/desktop.rs` | Animated, top-anchored `SetFrame`, window levels, `OrderFront`, an auto-hiding menu bar, native popup menus |
| App lifecycle | `macos.md` (standalone shutdown) | `nsgo.Run` returns for cleanup; apps quit themselves or outlive their last window; a forced appearance reaches native chrome |

## 2. Testing and the tool

| Item | NSH reference | Done when |
| --- | --- | --- |
| Test steps | `tooling.md` (headless runs) | Press, type, clear, submit, scroll, drag, hover and long press send what real input sends, move the clock and fail on disabled views |
| Test queries | `crates/nsh-testing/src/selector.rs`, `tooling.md` | Bare and regex selectors, Playwright's visibility, more waits, `FastForward`, the environment from the first frame, default dialog and picker answers, the URLs opened, a span's resolved style |
| `nsgo test` | `tooling.md` (`nsh test`) | Runs an app's tests with a filter, JUnit output and per-commit traces |
| Tests on real targets | `tooling.md` (on a real platform) | `nsgo test -target` drives the steps on each backend with native input and takes screenshots |
| Dev loop | `tooling.md` (dev server), `macos.md` (measuring) | Release dev builds, a free port, LAN hosts, an error overlay, a reload key and simulators by name |
| Packages | `embedding.md`, `web.md` | Build numbers, the app's appearance, Linux and Windows archives, IPAs, a cacheable web site with a manifest, `-run` |
| Store signing | `embedding.md` | `nsgo build` signs an IPA with a provisioning profile, wildcard ones too, and an APK with a keystore |

## 3. Embedding

| Item | NSH reference | Done when |
| --- | --- | --- |
| Embedding SDKs | `embedding.md` | iOS, Android and desktop host apps show an NSGo view run by their own loop |
| Several engines | `architecture.md` | Backends keep no singletons, so one process runs several surfaces |
| Host settings | `embedding.md` | A host registers components per view, sets locale, text size, appearance and theme tokens, receives app errors and opened URLs and sets the data directory |
| Host channel | `embedding.md` (the host channel) | The host and the app call each other's functions and send events without a view between them |
| Component lifetime | `embedding.md` (native components) | Native components are destroyed when their app stops, on Android too |

## 4. Performance

| Item | NSH reference | Done when |
| --- | --- | --- |
| Incremental layout | `pitfalls.md` (relayout boundaries), `performance.md` | Changes stop at fixed-size flex items, and a text that measures as before lays nothing out |
| Measure cache | `architecture.md` (data flow and threads) | Text measurements are cached by content and width per engine, so identical texts and list rows measure once |
| Paced list binds | `scrolling-and-lists.md` (paced binding) | A list binds its viewport first and its overscan over later frames |
| macOS frames | `macos.md` (architecture) | Scrolls move the container layer and commit at rest; batches take at most 4 ms a frame; resizes wait for layout; a timer covers a stalled display link |
| macOS canvas redraws | `canvas.md` (partial redraws) | A canvas redraws only the area that changed |
| Commits that follow changes | `performance.md` (Rust components run once) | A commit resolves styles and emits items only for the nodes that changed, not the whole tree |
| Windows frames | `windows.md` (frames) | The frame clock follows the compositor with `DwmFlush` |

## 5. Finish the existing platforms

These items wait for community contributions.

| Item | NSH reference | Done when |
| --- | --- | --- |
| iOS touch | `crates/nsh-platform-ios` | Gestures, text input and selection are checked in the simulator; selections have touch handles; hover, swipe back and touches outside the parent work |
| iOS system UI | `elements.md` (overlays), `pitfalls.md` | VoiceOver's escape gesture goes back and closes popups; inputs in scroll views stay clear of the keyboard |
| Text selection | `text.md` (selection areas) | Selectable text and selection areas work on Windows, Linux and Android |
| Desktop accessibility | `windows.md`, `linux.md` | Windows exposes UI Automation and Linux GTK's accessibility, canvas nodes included, and both announce |
| Desktop chrome | `windows.md` (alerts), `linux.md` | Windows draws alerts in the surface; Linux animates modals |
| Effects | `styles.md` (effects) | Windows draws backdrop filters and blend modes, Linux blend modes, and Android drop shadows from Android 12 |
| Android | `elements.md`, `styles.md` (Android differences) | Ripples, predictive back on screens, blurred images before Android 12, and layout animations aligned with the IME's motion (`WindowInsetsAnimation`) |
| Android frames | `elements.md` (interaction and the environment) | `RequestAnimationFrame` runs on `Choreographer` frames |
| Web | `web.md` | Browser history and back keys, a focus state for pressables, local time with daylight saving |
| Real file drags | `file-drop.md` | A real drag from Finder, Explorer and a Linux file manager is checked to reach a view |

## Not ported

Go apps do not need these NSH parts:

- The JS runtime, bundler, TSX compiler, `nsh check`, compatibility baselines and hot module replacement, since apps are Go and `go vet` checks them.
- Tailwind classes, since styles are Go values.
- `fetch`, WebSockets and `resource`, since apps use `net/http` or Go libraries with `Scope.Post`.
- Component refs, slots, render props and typed rows, since Go fields and functions carry them.
- `EngineContext`, since a host gives each engine's app function its values directly.
- The GPUI backend and the LayerFS trial.
- The theme and styled components, which apps or a component library build on `base`.
