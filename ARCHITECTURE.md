# NSGo architecture

NSGo is a Go port of the design of [Native Shell (NSH)](https://github.com/longbridge/nsh). A Go engine owns the reactive graph, the shadow tree, styles, layout and text. Each commit produces one batch of mount items, and a thin platform backend applies it to native views and runs what animates on its own frame clock. Sections below say what exists; the [roadmap](docs/roadmap.md) orders what is missing.

## Reference implementation

NSH is the reference for behavior and naming, not a constraint ([AGENTS.md](AGENTS.md#principles)). Its checkout lives in `nsh/`; port the design rather than the code, and record each deviation in the owning subject.

## Code map

Apps import the public packages; `internal/` holds the implementation, which only this module can import ([boundaries](#boundaries)).

| Package | NSH counterpart | Responsibility |
| --- | --- | --- |
| `nsgo` (module root) | `run_app`, `AppView` | `nsgo.Run`, and `nsgo.NewView` in a host's window ([embedding](docs/architecture/embedding.md)): picks the backend by build tags |
| `core` | `nsh-core` | The app's API: elements, styles, the reactive graph and regions, services; the shadow tree, layout and text preparation |
| `base` | `nsh-base` | Unstyled components: controls, `Input` among them, popups, dialogs, menus, safe area, keyboard, navigation pages, resizable panels and a dock ([components](docs/architecture/components.md)) |
| `mobile`, `desktop` | `nsh/mobile`, `nsh/desktop` | What only phones have (the tab bar, a bar above the keyboard) and only desktops (the window, its title bar, menus) |
| `base/textview` | `nsh-base`'s `text_view` | `TextView`: Markdown (goldmark), HTML (x/net/html) or the app's blocks shown with the primitives ([text view](docs/architecture/textview.md)) |
| `base/textview/highlight` | `nsh-base`'s `highlight` feature | Code highlighting for `TextView` with chroma |
| `icons/lucide` | — (NSH loads SVG as images) | The bundled Lucide pack: one generated accessor per icon, parsed on first use ([icons](docs/architecture/icons.md)) |
| `nsgotest` | `nsh-testing` | App tests, headless or on a platform: selectors over a model of the views, steps as a user's input ([testing](docs/architecture/testing.md)) |
| `internal/testhost` | `nsh-testing`'s agent | A backend's test host for `nsgotest`: the main thread, native input, screenshots |
| `internal/driver` | `nsh-core`'s mount protocol | Mount items, events, `TextEngine` and the values both sides share; the `Animator`, selection model and edit limits every backend runs |
| `internal/layout` | Taffy | Block, flexbox and grid layout, checked against Taffy's fixtures |
| `internal/shader`, `internal/shaderir` | — (MyGo's fork of Ebitengine's Kage) | The shader compiler: Go's syntax checked into an IR, written as MSL, GLSL or HLSL ([shaders](docs/architecture/shaders.md)) |
| `internal/engine` | `nsh-engine` | The engine goroutine: a mailbox, one batch per commit to the platform, storage files and the performance monitor |
| `internal/platform/<target>` | `nsh-platform-*` | Native view mounting, text engine and input for one target |
| `internal/platform/apple` | Shared Apple code | Layers, images, canvas, Metal shaders, web views, videos and CoreText for macOS and iOS |
| `internal/platform/*` | Platforms' shared code | Code backends share: image loading, caching and decoding ([images](docs/architecture/images.md)), picked photos |
| `examples/gallery` | `examples/gallery` | NSH's gallery in Go, a demo of each element, component and service, with headless tests; its web build is the website |
| `examples/embedded`, `examples/embed-*` | `examples/embed-*` | An app to embed, and desktop, iOS and Android apps embedding it |
| `cmd/nsgo` | `nsh-cli` | `nsgo init`, `dev`, `test` and `build` from an app's `nsgo.yaml` ([tool](docs/architecture/tool.md)); `apk` packages and signs Android apps in Go |
| `tools/lucidegen` | — | Regenerates `icons/lucide` from a pinned Lucide checkout |

## Platforms

Every target builds on macOS with Go; iOS also needs Xcode, and x86_64 Android needs an NDK's clang.

| Target | Package | Reaches the system through | Status |
| --- | --- | --- | --- |
| macOS | `internal/platform/macos` | purego and the Objective-C runtime; `CGO_ENABLED=0` | Runs |
| iOS | `internal/platform/ios` | The same; cgo only to link with Xcode's linker, and for an embedded framework's one export | Runs in the simulator and on devices |
| Web | `internal/platform/web` | `syscall/js` in WebAssembly; DOM elements | Runs |
| Windows | `internal/platform/windows` | `syscall` and COM: Direct2D, DirectWrite, WebView2, Media Foundation | Runs |
| Linux | `internal/platform/linux` | GTK 4, Pango and Cairo loaded with purego; WebKitGTK and GStreamer | Runs |
| Android | `internal/platform/android` | A Go executable the prebuilt Java runtime starts and talks to over pipes | Runs on arm64 devices and x86_64 emulators; `nsgo` builds, signs and installs the APK |

Android runs the app as a second process because Go builds Android shared libraries only with cgo and the NDK, and executables without cgo for arm64 only. iOS forbids child processes, so it links in-process with Xcode.


## Boundaries

Packages import only downward; `layering_test.go` checks it on every target:

```text
                    apps (examples/*, bench/*)
         ┌───────────────┬───────┴───────┬───────────────┐
    nsgo (root)       nsgotest base/* mobile desktop  icons/*
         │               │               │               │
 internal/platform/*     │               │               │
         │               │               │               │
  internal/engine        │               │               │
         └───────────────┴───────┬───────┴───────────────┘
                               core
                                 │
                          internal/driver
```

- Apps import `nsgo`, `core`, `base`, `mobile`, `desktop`, `icons` and `nsgotest` only, native components included; `core` and `nsgotest` alias the internal types their API reaches (`TestPublicAPINamesNoInternalType`).
- `internal/driver` is the protocol and imports nothing else from the module. Backends import it and `internal/engine`, never `core`.
- `core` imports only `internal/driver`, `internal/layout` and the shader compiler (`internal/shader`, `internal/shaderir`).
- `internal/engine` runs one app on `core` and defines `engine.Platform`, which backends implement. `nsgotest` is the headless backend, without the engine; on a platform it drives the test host a backend registers under the `nsgotest` tag.
- `core`, `base`, `mobile`, `desktop`, `icons`, `nsgotest`, `internal/driver`, `internal/layout`, `internal/testhost`, the shader compiler and `internal/engine` build unchanged for every target: no purego, `syscall` or `unsafe`. No package has cgo files or hand-written C.
- `base` holds NSGo's built-in unstyled components, each taking styles part by part (`StyleFunc` included); apps, or a component library, build styled ones on it. A component gets a package under `base` only for dependencies an app without it should not link, as `base/textview` links goldmark.
- Each API has one home, as in NSH: `core` holds elements and the environment, `base` controls and navigation, `mobile` and `desktop` what only phones or desktops have. A part `core` keeps for another package's API (`Input`, `Screen`, window commands) names that package in its comment.
- The platform main thread never waits for the engine, but for macOS's 16 ms at most for the layout at a new window size, and the engine never touches main-thread-only UI objects.
- What animates runs on the platform's frame clock, which stops when nothing animates or waits: transitions, animated values and layout animations are data the engine sends once, never a commit per frame. The app's frame callbacks wait for the same clock ([animation](docs/architecture/animation.md)).
- The reactive graph belongs to the engine goroutine and never locks; other goroutines reach it through `Scope.Post`.
- Text is prepared and laid out on the engine side through a platform `TextEngine`; the main thread only draws the result.
- One engine is one app with no global state, so a host app embeds several.
- Layout (`internal/layout`) ports Taffy 0.15's block, flexbox and grid algorithms step by step and must pass every Taffy fixture that applies to NSH's features.

## Tool

`nsgo` is the only build entry point; it shells out to `go`, and to Xcode's `xcrun`, `xcodebuild`, `codesign` and `simctl` for Apple targets ([tool](docs/architecture/tool.md)).

- Targets name an OS and an architecture (`macos_arm64`, `windows_x64`), so each package holds one binary.
- Hot reload watches the directories of `go list -deps`, NSGo's included, and keeps the last good build running on a failure, which its dev server shows over the app.

## Core flows

**Render.** Components run once and bind signals to element parts; regions (`Show`, `Each`, `Dynamic`, `Catch`) rebuild only their span. A commit resolves styles, prepares text, lays out and sends what changed as one `MountBatch` ([core](docs/architecture/core.md)).

**Mount.** The platform receives the batch with a single hop to its main thread and applies the items in order. Services answer with events ([services](docs/architecture/services.md)).

**Input.** Native gestures become engine events, queued without blocking, run the handlers on the engine side and lead to the next commit.

## Open questions

Record a design question here, and confirm the answer with the user before choosing it.

- **Signed packages and capabilities.** Port NSH's package signing and capability sandbox, or drop them because Go apps are native code?

## Verification

`core`, `internal/driver` and `internal/engine` have headless tests; apps test themselves with `nsgotest`, as `examples/gallery` does, and `nsgo test -target` runs them on each backend with its native input. The Windows and Linux backends' tests open a real window and check input, services and pixels.

## Subjects

| Document | When to read |
| --- | --- |
| [Core](docs/architecture/core.md) | Changing the graph, elements, regions, layers, commit, storage or the mount protocol |
| [Styles](docs/architecture/styles.md) | Changing `Style` or how a backend draws a view |
| [Text](docs/architecture/text.md) | Changing text, a `TextEngine`, `Input` or selection |
| [Scrolling](docs/architecture/scrolling.md) | Changing `Scroll`, `List` or a scroll view |
| [Interaction](docs/architecture/interaction.md) | Changing accessibility, pointers, focus, touches, gestures or drops |
| [Services](docs/architecture/services.md) | Changing a service, a menu, the desktop window or the app's lifecycle |
| [Hosted views](docs/architecture/native-views.md) | Changing `WebView`, `Video` or `Native` components |
| [Images](docs/architecture/images.md) | Changing how backends load, cache, decode or play images |
| [Canvas](docs/architecture/canvas.md) | Changing `Canvas`, `Drawing` or a backend's canvas player |
| [Animation](docs/architecture/animation.md) | Changing animated values, layout animations or a backend's frame clock |
| [Shaders](docs/architecture/shaders.md) | Changing `Shader`, the shader compiler or a backend's shader view |
| [Components](docs/architecture/components.md) | Changing a component in `base`: controls, popups, resizable panels or the dock |
| [Text view](docs/architecture/textview.md) | Changing `TextView`, its parsers, parts or highlighting |
| [Icons](docs/architecture/icons.md) | Changing SVG parsing, `core.Icon` or the Lucide pack |
| [Performance](docs/architecture/performance.md) | Changing the performance monitor, a frame clock or a per-frame path |
| [Embedding](docs/architecture/embedding.md) | Changing `nsgo.NewView`, the host channel or `-embed` packages |
| [Testing](docs/architecture/testing.md) | Changing `nsgotest`, its steps or a backend's test host |
| [Tool](docs/architecture/tool.md) | Changing `nsgo`: the dev server, tests on targets, packages or signing |
