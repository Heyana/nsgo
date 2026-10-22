# NSGo architecture

NSGo is a Go port of the design of [Native Shell (NSH)](https://github.com/longbridge/nsh). A Go engine owns the reactive graph, the shadow tree, styles, layout and text layout. Each commit produces one batch of mount items, and a thin platform backend applies it to native views. Most of NSH's elements, layers, navigation, base components and services exist today; sections below say what exists, and the [roadmap](docs/roadmap.md) orders what is missing.

## Reference implementation

NSH is the reference for behavior, naming and layering. Its checkout lives in `nsh/` and is never committed.

| Need | Read in `nsh/` |
| --- | --- |
| Overview and crate layout | `AGENTS.md`, `README.md` |
| Threads, data flow and layers | `docs/dev/architecture.md` |
| One subsystem (text, styles, lists, animation) | The matching page in `docs/dev/` |
| How a module works inside | The module docs in `crates/*/src` |

Port the design, not the code line by line. Where Go idioms differ from Rust, follow Go and record the deviation in the owning subject.

## Code map

Packages mirror NSH's crates:

| Package | NSH counterpart | Responsibility |
| --- | --- | --- |
| `core` | `nsh-core` | Geometry, styles, reactive graph, elements and regions, shadow tree, layout, text, mount items |
| `base` | `nsh-base` | Unstyled base components: controls, popups, dialogs, sheets, menus, safe area, keyboard, navigation stack |
| `base/textview` | `nsh-base`'s `text_view` | `TextView`: Markdown (goldmark) or the app's blocks shown with the primitives ([text view](docs/architecture/textview.md)) |
| `ui` | `nsh-base`'s `Theme` | The theme as Go values, and `base`'s controls, popups and text view styled with it |
| `ui/highlight` | `nsh-base`'s `highlight` feature | Code highlighting for `TextView` with chroma, in NSH's palette |
| `engine` | `nsh-engine` | Engine goroutine: an unbounded mailbox, commits, one batch per commit to the platform |
| `core/internal/layout` | Taffy | Block, flexbox and grid layout, checked against Taffy's fixtures |
| `nsgotest` | `nsh-testing` | Headless app tests: approximate text, a manual clock, selectors over a model of the views |
| `platform/<target>` | `nsh-platform-*` | Native view mounting, text engine and input for one target |
| `platform/internal/apple` | Shared Apple code | CoreFoundation, CALayer styling, images, canvas and the CoreText text engine through purego |
| `platform/internal/imageload` | Platforms' image loaders | Fetching and caching image bytes once, in Go, for the backends that decode images themselves |
| `nsgo` (module root) | `platform::run_app` | `nsgo.Run`: picks the backend by build tags; the tool sets name and window size |
| `cmd/nsgo` | `nsh-cli` | `nsgo init`, `nsgo dev` and `nsgo build`, driven by an app's `nsgo.yaml` |
| `examples/hello` | `examples/counter` | A todo list: text, input, button, scroll view and keyed list, with a headless test |
| `examples/gallery` | `examples/gallery-rs` | A sidebar of sections showing each element, base component, gesture and desktop service, with headless tests |

## Platforms

Every target builds on macOS with Go, and iOS also with Xcode. No backend contains hand-written C.

| Target | Package | Reaches the system through | Status |
| --- | --- | --- | --- |
| macOS | `platform/macos` | purego and the Objective-C runtime; `CGO_ENABLED=0` | Runs |
| iOS | `platform/ios` | The same; cgo only because Go links iOS with Xcode's linker | Runs in the simulator |
| Web | `platform/web` | `syscall/js` in WebAssembly; DOM elements | Runs |
| Windows | `platform/windows` | `syscall`: Win32 window drawn with GDI into a 32-bit buffer, native EDIT inputs | Runs; native tests check its window, input and pixels |
| Android | `platform/android` | A Go executable the prebuilt Java runtime starts and talks to over pipes | Go side builds; the Java runtime has never been compiled; `classes.dex` and APK packaging pending |
| Linux | — | Planned: GTK 4 through purego | Not started |

Android runs the app as a second process because Go builds Android shared libraries only with cgo and the NDK. Without cgo, Go builds Android executables for arm64 only, so 32-bit devices and x86_64 emulators are out of reach. iOS forbids child processes, so it links in-process with Xcode instead.

## Boundaries

- `core` never imports `engine`, `platform` or any cgo package. It stays pure Go and testable headless.
- `base` depends on `core` only and has no look: it takes styles, including `StyleFunc` for each part, so apps can style it freely.
- `ui` styles `base` with a theme. Apps change the theme with `ui.ThemeView`, adjust a component with its `Style`, or build their own components on `base`.
- Backends call system APIs through purego or `syscall`, never hand-written C.
- The platform main thread never waits for the engine. The engine never touches main-thread-only UI objects.
- The reactive graph belongs to the engine goroutine and never locks; other goroutines reach it through `Scope.Post`.
- Text is prepared and laid out on the engine side through a platform `TextEngine`; the main thread only draws the result.
- One engine is one app instance with no global state, so a host app can embed several.
- Keep NSH's public names and semantics unless Go requires otherwise, so NSH's documentation stays a usable reference.
- Apps are written in Go. The shadow tree's operations are the reserved entry for a TSX runtime, which does not exist yet.
- Layout (`core/internal/layout`) reimplements Taffy 0.15's block, flexbox and grid algorithms in Go, step by step, and must pass every Taffy fixture that applies to NSH's features.

## Tool

`nsgo` is the only build entry point; it shells out to `go build`, and to Xcode's `xcrun`, `codesign` and `simctl` for Apple targets.

- Targets name an OS and an architecture (`macos_arm64`, `windows_x64`), so each package holds one binary.
- `nsgo build` is a release build: `-s -w`, `-trimpath`, no VCS stamp. `nsgo dev` keeps symbols.
- `nsgo build` shows the output directory when it finishes, unless `nsgo.yaml` sets `build.open` to false.
- `nsgo dev` runs the app once and exits when the app does, unless `nsgo.yaml` sets `dev.hot_reload` to true; the web target serves until interrupted.
- With hot reload, `nsgo dev` watches the directories `go list -deps` reports, NSGo's own included, and keeps the last good build running when a build fails.
- With hot reload, the web dev server reloads open pages over server-sent events after each build.
- A command's flags replace the `nsgo.yaml` values they name: `dev.target` and `dev.hot_reload` for `nsgo dev`; `build.targets`, `build.output` and `build.open` for `nsgo build`; `name`, `id`, `version` and `window` for both.

## Core flows

**Render.** Components run once and bind signals to element parts; regions (`Show`, `Each`, `Dynamic`, `Catch`) rebuild only their span. A commit resolves styles, prepares text, lays out and sends what changed as one `MountBatch` ([core](docs/architecture/core.md)).

**Mount.** The platform receives the batch with a single hop to its main thread and applies the items in order to native views.

**Input.** Native gestures become engine events, queued without blocking, run the handlers on the engine side and lead to the next commit.

## Open questions

Record a design question here, and confirm the answer with the user before choosing it.

- **Windows web view and video.** Reach WebView2 and Media Foundation through COM over `syscall`, or ship WebView2's loader?
- **Signed packages and capabilities.** Port NSH's package signing and capability sandbox, or drop them because Go apps are native code?

## Verification

`core` and `engine` have headless Go tests for the graph, layout, regions, commits and the mailbox; apps test themselves with `nsgotest`, as `examples/hello` does. The Windows backend's tests open a real window and check clicks, typing, submit and painted pixels (`go test ./platform/windows`). Backends need a run on their target: screenshots of the window, simulator or headless browser, and input where it can be simulated.

## Subjects

Subjects go in `docs/architecture/` once a subsystem has a lasting design.

| Document | When to read |
| --- | --- |
| [Core](docs/architecture/core.md) | Changing the reactive graph, elements, regions, commit, text, selection, storage or the mount protocol |
| [Text view](docs/architecture/textview.md) | Changing `TextView`: the document model, Markdown parsing, its parts or code highlighting |
