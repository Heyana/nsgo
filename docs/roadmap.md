# Roadmap

This plan lists what NSGo still lacks compared with NSH's 0.3.0 release (tag `v0.3.0`, `76fa8a7f`), in the order to build it. Each item names its NSH reference and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

References are files in `nsh/docs/dev/` unless they give a path from `nsh/`, with the bullet or section to read in brackets. To refresh the plan for NSH's next release, check out its tag in `nsh/`, then read `git log v0.3.0..HEAD` there and the documents it changed.

## NSH 0.3.0's API

NSH 0.3.0 replaced wrapper components with styles and props, and the system's toast with `nsh/base`'s.

| Item | NSH reference | Done when |
| --- | --- | --- |
| Platform props | `js-modules.md` (platform-only props) | Props only iOS, only Android or only the desktops have are grouped in a field of their own, as NSH's `ios`, `android` and `desktop` objects; what two of them have stays top-level |
| Environment insets | `styles.md` (environment variables), `website/elements/keyboard.md` | Padding and margin take the safe area's and the software keyboard's insets as terms that follow them, the keyboard's moving with it natively; `SafeAreaView` and `KeyboardAvoidingView` leave `base` |
| Selectable views | `text.md` (selectable views) | `View.Selectable` makes the texts inside select together, replacing `View.SelectionArea` and `base.SelectionArea` |
| Toasts | `components.md` (toasts), `website/elements/toast.md` | `base` has `Toast`, `ToastStack` and `ToastManager`, `gpui-base`'s toasts, and `Scope.ShowToast` and the platforms' toasts are gone |
| Animated native props | `embedding.md` (native components) | A `Native` component's props take animated values, which the platform's frame clock hands Go, Swift and Java components as numbers or colors on the frames they change |
| Video sound | `website/modules/nsh-video.md` (sound and other apps) | `Video` can play speech in iOS's silent mode, pausing other apps' audio or mixing with it, and hold Android's audio focus while it plays |
| Gallery | `examples/gallery` | The gallery has NSH's changes since `b2af2f21`, the commit it was translated from, that need none of the components below: the Collapsing header demo, toasts on the Overlays page and the other pages' changes |

## Code and data components

Each item includes the component's demo in the gallery, from NSH's gallery or, for `Table`, its website.

| Item | NSH reference | Done when |
| --- | --- | --- |
| Charts | `api-naming.md` (Plot), `website/elements/plot.md`, `canvas.md` (the view's background) | `Plot` with `Line`, `Area` and `Grid` marks draws the app's rows natively, a canvas per mark, with scales, hover parts moved on the frame clock, motion and one accessible image; a drawing fills a shape with its canvas view's background |
| Table | `components.md` (`nsh/base`) | `Table` and its parts give the app's rows a table's accessibility roles |
| DataTable | `data-table.md` | `base` has a table with no look of its own that scrolls both ways over any number of rows and columns, with selection, sorting, resizing, moving and fixed columns, loading more and keyboard |
| Editor | `editor.md` | `base` has a controlled code editor: display rows over a rope, input and keys, virtualized rows, incremental highlighting off the engine goroutine, search, diagnostics, folds, completion, definitions, touch handles and parts, fast at 200 000 lines |
| DiffView | `diff.md` | `base` shows the changes between two texts or a patch: pairing and inline changes, collapsed separators, the editor's highlighting and parts |

## Rendering costs

| Item | NSH reference | Done when |
| --- | --- | --- |
| Linux canvases | `canvas.md` (partial redraws), `linux.md` (performance) | Canvases rasterize into tiles, animations redraw their damage over the kept static part, canvases out of sight wait, and GSK uses its GL renderer under NVIDIA |
| macOS canvases | `macos.md` (canvases) | A canvas draws at once unless it paints ten times a second, and a long static part painted unchanged is kept as a `CGLayer` |
| Windows frames | `windows.md` (partial frames) | Frames draw and present only what changed, canvases record only their animated tail, and children outside a clip are not drawn |
| Performance monitors | `linux.md`, `windows.md` (performance) | Linux, Windows and iOS report the monitor's readings as macOS does |

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
