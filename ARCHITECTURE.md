# NSGo architecture

NSGo is a Go port of the design of [Native Shell (NSH)](https://github.com/longbridge/nsh). A Go engine owns the reactive graph, the shadow tree, styles, layout and text. Each commit produces one batch of mount items, and a thin platform backend applies it to native views and runs what animates on its own frame clock. Sections below say what exists; the [roadmap](docs/roadmap.md) orders what is missing.

## Reference implementation

NSH is the reference for behavior and naming, not a constraint: NSGo keeps its names and semantics by default, so NSH's documentation stays usable, and departs where a design is clearer or easier to maintain. Its checkout lives in `nsh/` and is never committed.

| Need | Read in `nsh/` |
| --- | --- |
| Overview and crate layout | `AGENTS.md`, `README.md` |
| Threads, data flow and layers | `docs/dev/architecture.md` |
| One subsystem | The matching page in `docs/dev/` |
| How a module works inside | The module docs in `crates/*/src` |

Port the design, not the code line by line, and record each deviation in the owning subject.

## Code map

Apps import the public packages; `internal/` holds the implementation, which only this module can import ([boundaries](#boundaries)).

| Package | NSH counterpart | Responsibility |
| --- | --- | --- |
| `nsgo` (module root) | `platform::run_app` | `nsgo.Run`: picks the backend by build tags |
| `core` | `nsh-core` | The app's API: elements, styles, the reactive graph and regions, services; the shadow tree, layout and text preparation; aliases of the driver values apps use |
| `base` | `nsh-base` | Unstyled base components: a pressable view, popups, dialogs, sheets, menus, safe area, keyboard, navigation stack, split panes |
| `base/textview` | `nsh-base`'s `text_view` | `TextView`: Markdown (goldmark), HTML (x/net/html) or the app's blocks shown with the primitives ([text view](docs/architecture/textview.md)) |
| `base/textview/highlight` | `nsh-base`'s `highlight` feature | Code highlighting for `TextView` with chroma |
| `icons/lucide` | — (NSH loads SVG as images) | The bundled Lucide pack: one generated accessor per icon, parsed on first use ([icons](docs/architecture/icons.md)) |
| `nsgotest` | `nsh-testing` | Headless app tests: approximate text, a manual clock, selectors over a model of the views |
| `internal/driver` | `nsh-core`'s mount protocol | Mount items, events, `TextEngine` and the values both sides share; the `Animator`, selection model and edit limits every backend runs |
| `internal/layout` | Taffy | Block, flexbox and grid layout, checked against Taffy's fixtures |
| `internal/engine` | `nsh-engine` | The engine goroutine: a mailbox, one batch per commit to the platform, storage files and the performance monitor |
| `internal/platform/<target>` | `nsh-platform-*` | Native view mounting, text engine and input for one target |
| `internal/platform/apple` | Shared Apple code | Layers, images, canvas, web views, videos and CoreText for macOS and iOS |
| `internal/platform/*` | Platforms' shared code | Image loading and pure Go decoding ([images](docs/architecture/images.md)), text field edits, drawn scroll bars, hover and finger tracking |
| `examples/gallery` | `examples/gallery-rs` | Every element, component and service, a page each, with headless tests; its web build is the website |
| `cmd/nsgo` | `nsh-cli` | `nsgo init`, `dev` and `build`, driven by an app's `nsgo.yaml`; `apk` packages and signs Android apps in Go |
| `tools/lucidegen` | — | Regenerates `icons/lucide` from a pinned Lucide checkout |

## Platforms

Every target builds on macOS with Go; iOS also needs Xcode, and x86_64 Android needs an NDK's clang.

| Target | Package | Reaches the system through | Status |
| --- | --- | --- | --- |
| macOS | `internal/platform/macos` | purego and the Objective-C runtime; `CGO_ENABLED=0` | Runs |
| iOS | `internal/platform/ios` | The same; cgo only because Go links iOS with Xcode's linker | Runs in the simulator and on devices |
| Web | `internal/platform/web` | `syscall/js` in WebAssembly; DOM elements | Runs |
| Windows | `internal/platform/windows` | `syscall` and COM: Direct2D on a Direct3D 11 swap chain, DirectWrite text, drawn inputs, WebView2 in DirectComposition, Media Foundation videos | Runs; native tests open a real window |
| Linux | `internal/platform/linux` | GTK 4, Pango and Cairo loaded with purego: a widget per view drawn with GtkSnapshot, hosting GtkEntry inputs, WebKitGTK web views and GStreamer videos | Runs; native tests open a real window |
| Android | `internal/platform/android` | A Go executable the prebuilt Java runtime starts and talks to over pipes | Runs on arm64 devices and x86_64 emulators; `nsgo` builds, signs and installs the APK |

Android runs the app as a second process because Go builds Android shared libraries only with cgo and the NDK. Without cgo, Go builds Android executables for arm64 only, so 32-bit devices stay out of reach. iOS forbids child processes, so it links in-process with Xcode instead.

Neither an NDK nor the Android SDK is needed for arm64: `internal/platform/android/runtime` holds the Java runtime's prebuilt `classes.dex` (its `build.sh` rebuilds it), and `cmd/nsgo/apk` writes and signs the APK in Go.

The Linux backend loads GTK 4 at run time, so building it needs only Go; without WebKitGTK a web view shows a placeholder. Each Linux view is a widget its parent places at the core's frame, so hosted controls stack, clip and transform with the views around them. It draws no window controls, so a hidden title bar leaves them to the app.

## Boundaries

Packages import only downward; `layering_test.go` checks it on every target:

```text
                    apps (examples/*, bench/*)
         ┌───────────────┬───────┴───────┬───────────────┐
    nsgo (root)       nsgotest         base/*         icons/*
         │               │               │               │
 internal/platform/*     │               │               │
         │               │               │               │
  internal/engine        │               │               │
         └───────────────┴───────┬───────┴───────────────┘
                               core
                                 │
                          internal/driver
```

- Apps import `nsgo`, `core`, `base`, `icons` and `nsgotest` only, including for their native components. `core` and `nsgotest` alias the internal types their API reaches; `TestPublicAPINamesNoInternalType` checks it.
- `internal/driver` is the protocol and imports nothing else from the module. Backends import it and `internal/engine`, never `core`.
- `core` imports only `internal/driver` and `internal/layout`. `internal/engine` runs one app on `core` and defines `engine.Platform`, which backends implement. `nsgotest` is the headless backend, without the engine.
- `core`, `base`, `icons`, `nsgotest`, `internal/driver`, `internal/layout` and `internal/engine` build unchanged for every target: no purego, `syscall` or `unsafe`. No package has cgo files or hand-written C.
- `base` holds NSGo's built-in unstyled components. Each takes styles, including `StyleFunc` for each part, so apps can style it freely.
- A component gets its own package under `base` only when it links dependencies an app without it should not, as `base/textview` links goldmark and `base/textview/highlight` chroma.
- NSGo ships no theme or styled components. Apps, or a component library, build them on `base`.
- The platform main thread never waits for the engine, and the engine never touches main-thread-only UI objects.
- What animates runs on the platform's frame clock: transitions, animated values and layout animations are data the engine sends once, never a commit per frame. The app's frame callbacks wait for the same clock.
- The reactive graph belongs to the engine goroutine and never locks; other goroutines reach it through `Scope.Post`.
- Text is prepared and laid out on the engine side through a platform `TextEngine`; the main thread only draws the result.
- Icons draw through the canvas protocol: an SVG becomes one `Drawing`, so no backend learns about icons.
- One engine is one app with no global state, so a host app can embed several.
- Layout (`internal/layout`) ports Taffy 0.15's block, flexbox and grid algorithms step by step and must pass every Taffy fixture that applies to NSH's features.

## Tool

`nsgo` is the only build entry point; it shells out to `go build`, and to Xcode's `xcrun`, `xcodebuild`, `codesign` and `simctl` for Apple targets.

- Targets name an OS and an architecture (`macos_arm64`, `windows_x64`), so each package holds one binary. `nsgo build` strips symbols, paths and VCS stamps; `nsgo dev` keeps symbols.
- Hot reload watches the directories `go list -deps` reports, NSGo's own included, and keeps the last good build running when a build fails.
- `resources/` ships in each package's resource root, which `imageload.ResourceRoot`, the web backend and Android's APK reader resolve `asset:` paths against.
- App icons are written in Go, except that `actool` compiles Apple's `.icon` files. A Windows icon links in from a `.syso` written into the app's package directory, since Go links `.syso` files only from there.

## Core flows

**Render.** Components run once and bind signals to element parts; regions (`Show`, `Each`, `Dynamic`, `Catch`) rebuild only their span. A commit resolves styles, prepares text, lays out and sends what changed as one `MountBatch` ([core](docs/architecture/core.md)).

**Mount.** The platform receives the batch with a single hop to its main thread and applies the items in order to native views. iOS and Android draw screens' system headers natively.

**Input.** Native gestures become engine events, queued without blocking, run the handlers on the engine side and lead to the next commit.

**Frame.** `Scope.RequestAnimationFrame` callbacks wait for the next display frame, and the core asks the host for one when the first waits. The platform's frame clock calls `Engine.Frame`, the engine runs the callbacks with the graph clock's time and commits, and the platform stops its clock when nothing animates or waits ([animation](docs/architecture/animation.md)).

## Open questions

Record a design question here, and confirm the answer with the user before choosing it.

- **Signed packages and capabilities.** Port NSH's package signing and capability sandbox, or drop them because Go apps are native code?

## Verification

`core`, `internal/driver` and `internal/engine` have headless tests; apps test themselves with `nsgotest`, as `examples/gallery` does. The Windows and Linux backends' tests open a real window and check input and painted pixels. Other backends need a run on their target: screenshots of the window, simulator, device or headless browser. `go test ./cmd/nsgo/apk` checks the APK and, when `apksigner` is installed, its signature.

## Subjects

| Document | When to read |
| --- | --- |
| [Core](docs/architecture/core.md) | Changing the graph, elements, regions, layers, commit, storage or the mount protocol |
| [Text](docs/architecture/text.md) | Changing text, a `TextEngine`, `Input` or selection |
| [Scrolling](docs/architecture/scrolling.md) | Changing `Scroll`, `List` or a scroll view |
| [Interaction](docs/architecture/interaction.md) | Changing accessibility, pointers, focus, touches, gestures or drops |
| [Hosted views](docs/architecture/native-views.md) | Changing `WebView`, `Video` or `Native` components |
| [Images](docs/architecture/images.md) | Changing how backends load, decode or play images |
| [Animation](docs/architecture/animation.md) | Changing animated values, layout animations or a backend's frame clock |
| [Text view](docs/architecture/textview.md) | Changing `TextView`, its parsers, parts or highlighting |
| [Icons](docs/architecture/icons.md) | Changing SVG parsing, `core.Icon` or the Lucide pack |
| [Performance](docs/architecture/performance.md) | Changing the performance monitor, its panel or a backend's timings |
