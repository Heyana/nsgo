# Roadmap

This plan lists what NSGo still lacks compared with NSH's `main` at `0384424f`, in the order to build it. Each item names its NSH reference and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

References are files in `nsh/docs/dev/` unless they name a crate, with the bullet or section to read in brackets. To refresh the plan, read `git log 0384424f..` in `nsh/` and the documents it changed.

## Code and data components

| Item | NSH reference | Done when |
| --- | --- | --- |
| Editor | `editor.md` | `base` has a controlled code editor: display rows over a rope, input and keys, virtualized rows, incremental highlighting off the engine goroutine, search, diagnostics, folds, completion, definitions, touch handles and parts, fast at 200 000 lines |
| DiffView | `diff.md` | `base` shows the changes between two texts or a patch: pairing and inline changes, collapsed separators, highlighting and parts |
| DataTable | `data-table.md` | `base` has a table that scrolls both ways over any number of rows and columns, with selection, sorting, resizing, moving and fixed columns, loading more and keyboard |
| Table | `components.md` (`nsh/base`) | `Table` and its parts give the app's rows a table's accessibility roles |

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
| Text selection | `text.md` (selection areas) | Selectable text and selection areas work on Windows, Linux and Android |
| Desktop accessibility | `windows.md`, `linux.md` | Windows exposes UI Automation and Linux GTK's accessibility, canvas nodes included, and both announce |
| Desktop chrome | `windows.md` (alerts), `linux.md` | Windows draws alerts in the surface; Linux animates modals |
| Effects | `styles.md` (effects) | Windows draws backdrop filters and blend modes, Linux blend modes, and Android drop shadows from Android 12 |
| Android | `elements.md`, `styles.md` (Android differences) | Ripples, predictive back on screens, blurred images before Android 12, layout animations aligned with the IME's motion (`WindowInsetsAnimation`), and a forced appearance in the activity's theme and web views |
| Android frames | `elements.md` (interaction and the environment) | `RequestAnimationFrame` runs on `Choreographer` frames |
| Web | `web.md` | Browser history and back keys, a focus state for pressables, local time with daylight saving |
| Desktop touch | `linux.md`, `windows.md` (scroll views) | Touch drags scroll Windows and Linux views with rubber banding and pull to refresh, the app's refresh indicators and pull values included |
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
