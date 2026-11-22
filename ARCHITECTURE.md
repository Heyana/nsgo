# NSGo architecture

NSGo is a Go port of the design of [Native Shell (NSH)](https://github.com/longbridge/nsh). A Go engine owns the reactive graph, the shadow tree, styles, layout and text. Each commit produces one batch of mount items, and a thin platform backend applies it to native views and runs what animates on its own frame clock. Sections below say what exists; the [roadmap](docs/roadmap.md) orders what is missing.

## Reference implementation

NSH, checked out beside the repository at its latest release ([contributing](docs/public/en/contributing.md#nsh)), is a reference, not a constraint ([principles](AGENTS.md#principles)): port its design rather than its code. [NSGo and NSH](docs/architecture/nsh.md) says what NSGo keeps, changes, adds and leaves out.

## Code map

Apps import the public packages; `internal/` holds the implementation, which only this module can import ([boundaries](#boundaries)).

| Package | NSH counterpart | Responsibility |
| --- | --- | --- |
| `nsgo` (module root) | `run_app`, `AppView` | `nsgo.Run` of a root view or of an app that opens its windows ([application](docs/architecture/application.md)), and `nsgo.NewView` in a host's window ([embedding](docs/architecture/embedding.md)), on the backend build tags pick (`internal/backend`) |
| `core` | `nsh-core` | The app's API: elements, styles, Tailwind classes, themes, the reactive graph and regions, services; the shadow tree, layout and text preparation |
| `base` | `nsh-base` | Unstyled components, a file each: controls, popups, navigation, panels and tables ([components](docs/architecture/components.md)) |
| `base/datatable`, `base/dock`, `base/plot`, `base/toast` | `nsh-base` | Complex components, a package each: a virtualized table, a dock, charts and toasts |
| `base/dragdrop` | — (NSH's component-local drags) | Application values dragged between views in one window, through the shared responder ([interaction](docs/architecture/interaction.md#in-app-drags)) |
| `base/commitgraph` | — | A Git history's revision graph: its layout, collapsed merges and a row's drawing ([commit graph](docs/architecture/components.md#commit-graph)) |
| `base/camera` | — | A plane the user pans and zooms around the app's content ([camera](docs/architecture/components.md#camera)) |
| `base/flow` | — | A directed diagram: its layered layout and orthogonal routes, for a camera to show ([flow diagram](docs/architecture/components.md#flow-diagram)) |
| `ui`, `ui/chat`, `ui/filepicker`, `ui/editor`, `ui/diff`, `ui/commitgraph`, `ui/flow`, `ui/richeditor` | `examples/nsh-gallery-ui` | Styled components on `base`, a file each, a conversation, a file picker and the styled forms of `base/editor`, `base/diff`, `base/commitgraph`, `base/flow` and `base/richeditor` as packages ([themes](docs/architecture/themes.md#ui)) |
| `mobile`, `desktop` | `nsh/mobile`, `nsh/desktop` | What only phones have (the tab bar) and only desktops (the window, menus) |
| `web` | `nsh/web` | Scoped browser document metadata through `Head`, ignored by native and embedded hosts ([publishing](docs/public/en/web.md)) |
| `base/textview`, `base/textview/highlight` | `nsh-base`'s `text_view`, `highlight` | `TextView`: Markdown, HTML or the app's blocks, and its code highlighting ([text view](docs/architecture/textview.md)) |
| `base/formula` | — (MyGo's `contrib/mygo` adapter of Untie) | `View`: TeX and Typst math laid out by Untie and drawn as canvas shapes in New Computer Modern Math, and `RenderMath` for `TextView` ([formulas](docs/architecture/components.md#formulas)) |
| `base/diagram` | — (limu's `limumygo`, its MyGo adapter) | `View`: Mermaid diagrams read and laid out by limu and drawn on a canvas in the platform's font, and `RenderDiagram` for `TextView` ([diagrams](docs/architecture/components.md#diagrams)) |
| `base/terminal`, `tools/libghostty` | — (Ghostty's libghostty-vt) | `Terminal` and its `View`: a program in a pseudo-terminal with libghostty-vt loaded through purego, and the command building the library ([terminal](docs/architecture/terminal.md)) |
| `base/editor`, `base/diff` | `nsh-base`'s `editor`, `diff` | `Editor`, a code editor, and `DiffView`, the changes between texts or of a patch ([editor](docs/architecture/editor.md)) |
| `base/richeditor`, `ui/richeditor` | `nsh-base`'s `rich_editor` | `RichEditor`, a ProseMirror-style document with Tiptap's extensions, and its styled form ([rich editor](docs/architecture/richeditor.md)) |
| `base/internal/*` | `nsh-base`'s `syntax`, `window` | What `base`'s packages share: highlighting, the rows a virtualized view makes, part styles and sheets |
| `tsx`, `tsx/textview`, `tsx/diagram`, `tsx/formula`, `tsx/editor`, `tsx/diff`, `tsx/richeditor`, `tsx/terminal`, `tsx/ui`, `tsx/ui/chat`, `tsx/ui/richeditor` | `nsh-js` | TSX apps on goja, or the page's engine on the web, with NSH's modules, named `nsgo`, and Go components; the subpackages register `base`'s own, the rich editor's sessions and the terminal, and `ui`'s as `nsgo/ui` and `nsgo/ui/chat` ([TSX](docs/architecture/tsx.md)) |
| `tsx/internal/js` | `nsh-platform-web`'s `browser` | The shared JavaScript API with adapters for Goja and the page's engine over `syscall/js` ([engines](docs/architecture/tsx.md#the-engines)) |
| `tsx/websrc` | | The scripts a TSX app's realm runs on the web, which a release's page ships as files of its own ([engines](docs/architecture/tsx.md#the-engines)) |
| `tsx/bundler` | `nsh-bundler` | TSX to one script with esbuild and a port of NSH's JSX compiler; hot and split bundles |
| `tsx/compiler` | | `compiler.App`: runs an app's bundle compiled ahead, loading a split one's chunks on the web, or compiles it as it starts |
| `tsx/tsxtest` | `nsh-testing`'s `nsh/test` | Runs `*.test.tsx` files as Go tests on `nsgotest` |
| `tsx/pkg` | `nsh-package` | Signed packages of TSX apps: keys, certificates, revocation lists and the `Trust` that verifies them for `tsx.View` ([signed packages](docs/architecture/tsx.md#signed-packages)) |
| `i18n`, `*/locales`, `internal/cldr` | — | The language the app shows, and what the code `nsgo` writes from a package's catalogs runs on, formatting by CLDR's data; `base`'s own catalogs ([languages](docs/architecture/languages.md)) |
| `icons/lucide`, `tools/lucidegen` | — (NSH loads SVG as images) | The bundled Lucide pack, an accessor per icon, and its generator ([icons](docs/architecture/icons.md)) |
| `audio` | `nsh`'s `Sound` and `AudioPlayer` | Overlapping, panned short sounds and streaming players on macOS, iOS, Windows, Linux, Android and the web, with shared state in `core` ([audio playback](docs/architecture/audio.md)) |
| `gamepad` | — | Connected game controllers (`Pads`) and per-frame button and axis readings (`State`) in the W3C Standard Gamepad layout on macOS, iOS, Windows, Linux, Android and the web |
| `microphone` | — | Recording the default input (`Capture`) on macOS, iOS, Windows (WASAPI) and Linux (PulseAudio/ALSA), and WAV files (`RecordWAV`) on macOS, iOS, Windows, Linux and Android (in the runtime's foreground service, [Android recording](docs/architecture/android-recording.md)), reporting `ErrUnsupported` elsewhere |
| `internal/audiosession` | — | iOS's one `AVAudioSession`, set by the highest level its holders ask for: videos, speech, `microphone` and audio's players and sounds ([sessions](docs/architecture/audio.md#sessions)) |
| `updater`, `updater/locales` | `nsh/updater` | Updating the app the way its platform and channel do: Sparkle, WinSparkle, the appcast `nsgo build` signs, or the App Store ([updates](docs/architecture/updates.md)) |
| `widget` | — | Timelines of views that macOS and iOS widgets show, drawn by a SwiftUI extension the tool compiles, and Live Activities, which the extension draws on iOS and Android's runtime posts as Live Updates ([widgets](docs/architecture/tool.md#widgets)) |
| `gameservices` | — | Provider-neutral platform game services: player sign-in, achievements, leaderboards and native Game Center / Play Games UI overlays, via `purego` GameKit on macOS/iOS/tvOS and Android's runtime bridge, reporting `ErrUnsupported` elsewhere ([game services](docs/architecture/gameservices.md)) |
| `nsgotest` | `nsh-testing` | App tests, headless or on a platform, and the agent `nsgo dev -automation` drives a running app through ([testing](docs/architecture/testing.md)) |
| `internal/automation` | — (MyGo's `plugins/automation`) | The requests the nsgo tool sends a running dev build's agent ([automation](docs/architecture/tool.md#automation)) |
| `internal/testhost` | `nsh-testing`'s agent | A backend's test host for `nsgotest`: the main thread, native input, screenshots |
| `internal/appstartup` | — | Packaged startup identities shared by the tool and native backends without linking the runtime |
| `internal/mimeinfo` | — | Fixed extension/MIME tables shared by packaging and the driver, independent of the host's database |
| `internal/driver` | `nsh-core`'s mount protocol | The mount protocol, events and `TextEngine`; what every backend runs, such as the `Animator` |
| `internal/devtools` | — | What DevTools read from a running app's trees and send them; `core` draws their overlay, the layout audit's included ([tool](docs/architecture/tool.md#dev-loop)) |
| `internal/layout` | Taffy | Block, flexbox and grid layout, checked against Taffy's fixtures |
| `internal/shader`, `internal/shader/wgsl`, `internal/shaderir` | — (a fork of Ebitengine's Kage); `nsh-core`'s `shader` | The shader compilers: Go's syntax and WGSL checked into an IR, written as MSL, GLSL or HLSL ([shaders](docs/architecture/shaders.md)) |
| `internal/engine` | `nsh-engine` | The engine goroutine: a mailbox, a batch per commit, storage files, the performance monitor |
| `internal/platform/tui` | `nsh-platform-tui` | Terminal cells, fixed-grid text and terminal input under the `nsgotui` tag ([terminal backend](docs/architecture/terminal.md#backend)) |
| `internal/platform/<target>` | `nsh-platform-*` | Native view mounting, text engine and input for one target |
| `internal/platform/*` | Platforms' shared code | What backends share: `apple` for macOS and iOS, `privacy` for their permissions, which `microphone` shares too, `webstyle` and `canvas2d` for the web and mini programs, `loopback` for desktops' sign-in redirects, images ([images](docs/architecture/images.md)), picked photos, process readings, update checks |
| `examples/gallery` | `examples/gallery` | NSH's gallery in Go, with headless tests; the website runs its web build |
| `examples/previews` | | The docs' component examples, one demo per marked code block, in one web build the website mounts per block |
| `examples/embedded`, `examples/embed-*` | `examples/embed-*` | An app to embed, and desktop, iOS and Android apps and a web page embedding it |
| `examples/tsx` | — | An app written in TSX and styled with Tailwind classes, with tests |
| `examples/gallery-tsx` | — | The gallery's Components in TSX, its apps among them, on `nsgo/ui`, `nsgo/ui/chat` and `nsgo/base`, with tests |
| `examples/gallery-miniprogram` | — | The gallery for mini programs: the components they show, on `base` in `ui`'s look, small enough for their 2 MB package, with tests |
| `examples/plugins` | — | A Go app installing signed TSX packages once the user grants their capabilities, running one in `tsx.View` and calling another's functions, with tests refusing tampered, revoked and older ones |
| `examples/orbit` | — | Six live native views on a ring in perspective, one animated angle turning the cards, a floor dial shader and the header, with tests |
| `examples/widgets` | — | A counter whose macOS and iOS widgets show its count in each family, and whose Live Activity changes it, with tests |
| `examples/appclip` | — | A café whose iOS App Clip, a package of its own, orders at the table its link names, with tests |
| `examples/hello` | — | A greeting in English and Chinese, built for every target but mini programs, whose 2 MB it is past, and an App Clip greeting the name its link gives, to try the packages and installers, with tests |
| `cmd/nsgo` | `nsh-cli` | The `nsgo` tool, run on an app's `nsgo.yaml` ([tool](docs/architecture/tool.md)), with `nsgo mcp` serving agents MCP; `apk` signs Android packages in Go, `authenticode` signs Windows images in Go, and `install` holds the scripts installing a release from GitHub |

[nsgo-dev/game](https://github.com/nsgo-dev/game) is the companion repository for Go games built on NSGo's public API. The [games roadmap](docs/roadmap.md#games) tracks its 2D and 3D goals and the NSGo primitives they need.

## Platforms

Every target builds on macOS with Go; iOS, tvOS and watchOS also need Xcode, and x86_64 Android an NDK's clang.

| Target | Package | Reaches the system through | Status |
| --- | --- | --- | --- |
| Terminal | `internal/platform/tui` | tcell in pure Go on macOS, Windows and Linux; `nsgotui` | Cell text, views, editing and input; [limits](docs/architecture/terminal.md#backend) |
| macOS | `internal/platform/macos` | purego and the Objective-C runtime; `CGO_ENABLED=0` | Runs |
| iOS | `internal/platform/ios` | The same; cgo only to link with Xcode's linker, and for an embedded framework's one export | Runs in the simulator and on devices; App Clips in the simulator |
| tvOS | `internal/platform/ios` | iOS's backend linked with tvOS's SDK (Go's `GOOS=ios`), the remote moving a focus | Runs in the simulator |
| watchOS | `internal/platform/watchos` | A SwiftUI host the tool links into the Go executable (`GOOS=ios`, arm64 alone: Series 9, Ultra 2 and later), each view a node Go draws into with Core Graphics | Runs in the simulator |
| Web | `internal/platform/web` | `syscall/js` in WebAssembly; DOM elements | Runs |
| Windows | `internal/platform/windows` | `syscall` and COM: Direct2D, DirectWrite, WebView2, Media Foundation | Runs |
| Linux | `internal/platform/linux` | GTK 4, Pango and Cairo loaded with purego; WebKitGTK and GStreamer | Runs |
| Android | `internal/platform/android` | A Go executable the prebuilt Java runtime starts and talks to over pipes | Runs on arm64 devices and x86_64 emulators |
| Mini programs: WeChat, QQ, Alipay, Douyin | `internal/platform/miniprogram` | `syscall/js` in WebAssembly in the logic layer; a runtime that hands the render layer a tree of views with `setData`, through each platform's APIs and markup | Experimental: runs in a stand-in host, unchecked in the platforms' tools, and larger apps exceed the platforms' 2 MB package |
| HarmonyOS | `internal/platform/harmonyos` | `GOOS=js/wasm` in ArkWeb under an ArkTS host, which draws what it mounts with ArkUI `FrameNode`s | Preview with emulator support and optional HAP signing |

A mini program runs the engine in its logic layer, which has no DOM: text breaks into lines there on an offscreen canvas's widths, and the render layer draws views and lines where they were placed ([mini programs](docs/architecture/miniprogram.md)). Android runs the app as a second process because Go builds Android shared libraries only with cgo and the NDK, and executables without cgo for arm64 only. iOS forbids child processes, so it links in-process with Xcode. watchOS has no public UIKit or Core Animation, so its views are SwiftUI's in `cmd/nsgo/watch/Host.swift`, which places, clips and scrolls them; Go keeps their state, hit-tests touches, runs what animates on the host's frames, and builds only the views within a screen of what each scroll view shows, as a watch has the memory for a screen's views, not a page's.

## Boundaries

Packages import only downward, which `internal/moduletest/layering_test.go` checks on every target; [package layers](docs/architecture/layers.md) holds the graph and the rules for each layer.

- Each API has one home, `base`'s components have no look and `ui`'s are `base`'s in one ([principles](AGENTS.md#principles), [components](AGENTS.md#components)).
- The main thread waits for the engine in bounded platform paths. A resized macOS, Windows or Linux window waits up to 16 ms for layout; the backend applies its queued commit before returning to the system's drawing path. AppKit's synchronous save delegate waits up to five seconds for a preflight answer, which the backend delivers before queuing the mount batch. The engine never touches main-thread-only UI objects.
- What animates runs on the platform's frame clock, which stops when idle: the engine sends transitions and animated values once, never a commit per frame ([animation](docs/architecture/animation.md)).
- The reactive graph belongs to the engine goroutine and never locks; other goroutines reach it through `Scope.Post`.
- Text is prepared and laid out on the engine side through a platform `TextEngine`; the main thread only draws the result.
- One engine is one app with no global state, so a host app embeds several; its windows are surfaces of that engine, a tree each. `tsx.Register` configures the process before apps start.
- Go code, an app's and its components', is trusted native code; only TSX is signed, sandboxed and held to capabilities (decided by the user, 2026-11-08).
- Layout (`internal/layout`) ports Taffy 0.15's algorithms step by step and passes every Taffy fixture that applies to NSH's features.

## Tool

`nsgo` is the only build entry point; it shells out to `go` and Xcode's tools ([tool](docs/architecture/tool.md)).

- Targets name an OS and an architecture (`macos_arm64`, `windows_x64`). Universal macOS apps and dual-ABI Android App Bundles combine their architecture builds in one package.
- A TSX app's bundles are compiled ahead into its package, so packages ship no compiler.
- A package's catalogs compile to Go committed beside them, so apps parse no catalog and `go build` needs no tool.
- Desktop installers are written in Go where they can be: Linux packages and disk images' layouts, with `hdiutil` for the image; the Windows installer alone takes another tool, NSIS's `makensis`, and without it the package keeps the script ([installers](docs/architecture/tool.md#installers)).

## Core flows

**Render.** Components run once and bind signals to element parts; regions (`Show`, `Each`, `Dynamic`, `Catch`) rebuild only their span. A commit resolves styles, prepares text, lays out and sends what changed as one `MountBatch` ([core](docs/architecture/core.md)).

**Mount.** The platform receives the batch with a single hop to its main thread and applies the items in order. Services answer with events ([services](docs/architecture/services.md)).

**Input.** Native gestures become engine events, queued without blocking; their handlers run on the engine and lead to the next commit.

## Open questions

Record a design question here, and confirm the answer with the user before choosing it.

- **Implicit presentations in `ui`.** `ui.Dialog`, `ui.NumberInput` and `ui/commitgraph.View` still swap their form on touch screens or by width ([device differences](docs/architecture/device-differences.md#presentation)), while menus, dropdowns and `dock.Area` take the form the app sets, popups and docked panels by default, and `ui` adds no choice of its own (decided by the user, 2026-11-12). Make `ui`'s a field the app sets too?
- **Texts drawn into one layer on Apple platforms.** Core Animation spends about 10 µs a frame on each text layer that changes, so a table of 600 numbers changing every frame holds about 90 fps at 120 Hz on a Mac ([performance](docs/architecture/performance.md#keeping-frames-cheap), #51). Drawing a container's texts into one layer of its own reached 135–143 fps in #51's measurements, but its texts would no longer stack, clip, fade, move or transform as their views do, and with few texts it cost about 30 MB more. Draw texts so, and when?
- **Bluetooth's first contract.** The [Bluetooth Low Energy](docs/architecture/bluetooth.md) design ([#58](https://github.com/nsgo-dev/nsgo/issues/58)) leaves five choices to an answer: is the first API central-only, or do advertising and the peripheral role join it; does a second scan replace the first or fail; do operation IDs and the negotiated MTU and write limits reach the public API or stay backend details; is Bluetooth one `AccessBluetooth` capability or separate scan and connect grants; and which background guarantees follow [#12](https://github.com/nsgo-dev/nsgo/issues/12): restored connections, scans, notification delivery, or all three under separate policies?
- **Sizes on a TV.** `ui`'s sizes are constants on every device, but a TV is read from across a room. Since `core` scales nothing by device, does `ui` take larger sizes on a TV, or leave them to the app ([roadmap](docs/roadmap.md#apple-tv))?
- **Remote push.** [Remote push](docs/architecture/remote-push.md#open-decisions) is designed but not built, and five choices block it: the public API's shape, whether `nsgo build` packages an FCM adapter for standalone apps, whether a background push runs app code with no view, whether data delivery takes a capability of its own beside `AccessNotifications`, and which provider metadata the API exposes, a press's ID among it. Which answers?
- **Scenes an iOS app opens.** Each scene beyond an iPad app's first shows the main view in a tree of its own, opened as the user opens a window ([application](docs/architecture/application.md#ios)). An app opening a scene itself (`requestSceneSessionActivation`), showing a given view or document in it, and getting back what each scene showed through its session's user activity (`stateRestorationActivity`) need public API: does `desktop.OpenWindow` open a scene on an iPad, or does `mobile` take a scene API of its own, and what does a scene carry to say what it shows?
- **Contacts and calendar.** [The design](docs/architecture/contacts-calendar.md#open-decisions) for #57 waits on five answers: whether the first release writes contacts and events or only picks and reads, whether contacts and calendar are separate capabilities and how contacts' limited and calendars' write-only access read through `core.Permission`, which contact fields, recurrence forms and attendee data are stable across platforms, whether the calendar reports the provider's changes or apps query again, and whether provider IDs are strings or handles that expire with an engine. Which, before building it?

## Verification

`core`, `internal/driver` and `internal/engine` have headless tests; apps test themselves with `nsgotest`, and `nsgo test -target` runs them on each backend with its native input. The Windows and Linux backends' tests open a real window and check input, services and pixels.

## Subjects

| Document | Read before changing |
| --- | --- |
| [Package layers](docs/architecture/layers.md) | A package's imports, or where new code belongs |
| [Core](docs/architecture/core.md) | The graph, elements, regions, layers, commit, storage or the mount protocol |
| [Styles](docs/architecture/styles.md) | `Style` or how a backend draws a view |
| [Themes](docs/architecture/themes.md) | Themes, theme files or the gallery's look |
| [Text](docs/architecture/text.md) | Text, a `TextEngine`, `Input` or selection |
| [Scrolling](docs/architecture/scrolling.md) | `Scroll`, `List` or a scroll view |
| [Interaction](docs/architecture/interaction.md) | Accessibility, pointers, focus, touches, gestures or drops |
| [Services](docs/architecture/services.md) | A service, a menu, the desktop window or the app's lifecycle |
| [Remote push](docs/architecture/remote-push.md) | Remote notification tokens, payload delivery or provider adapters |
| [Contacts and calendar](docs/architecture/contacts-calendar.md) | Contacts, calendars, events or their permissions, which are designed and not built |
| [Location and motion](docs/architecture/location-motion.md) | Location fixes, motion samples or sensor permissions |
| [Picture in Picture](docs/architecture/picture-in-picture.md) | iOS video or custom-content Picture in Picture |
| [Android floating windows](docs/architecture/android-floating-windows.md) | Android system overlays, service ownership or overlay permissions |
| [Foldable displays](docs/architecture/foldable.md) | Hinge geometry, reserved regions or a hinge's state in the environment |
| [Mention completion](docs/architecture/mention-completion.md) | `core.Suggest`, `base.SuggestList` or Composer's mentions |
| [Android recording](docs/architecture/android-recording.md) | Long-running microphone capture, foreground services or level metering |
| [Bluetooth Low Energy](docs/architecture/bluetooth.md) | Bluetooth central sessions, GATT operations or platform permissions |
| [Updates](docs/architecture/updates.md) | `updater`, an update check, the updaters `nsgo build` bundles or a release channel |
| [Application and windows](docs/architecture/application.md) | `nsgo.Run`, the windows, panels and tray icons an app opens, global shortcuts or shutdown |
| [Hosted views](docs/architecture/native-views.md) | `WebView`, `Video` or `Native` components |
| [Audio playback](docs/architecture/audio.md) | The `audio` package, its native players and audio-session sharing |
| [Game services](docs/architecture/gameservices.md) | The `gameservices` package, GameKit / Play Games hooks and `nsgotest`'s fake |
| [Images](docs/architecture/images.md) | How backends load, cache, decode or play images |
| [Canvas](docs/architecture/canvas.md) | `Canvas`, `Drawing` or a backend's canvas player |
| [Animation](docs/architecture/animation.md) | Animated values, layout animations or a backend's frame clock |
| [Shaders](docs/architecture/shaders.md) | `Shader`, the shader compiler or a backend's shader view |
| [GPU view](docs/architecture/gpu.md) | The planned `gpu` package, vertex/fragment shader IR extensions or a backend's 3D GPU surface (designed, not yet built) |
| [Components](docs/architecture/components.md) | A component in `base`: controls, popups, resizable panels or the dock |
| [Device differences](docs/architecture/device-differences.md) | A component that changes by device, input or width |
| [Text view](docs/architecture/textview.md) | `TextView`, its parsers, parts or highlighting |
| [Text-level search](docs/architecture/text-search.md) | Document matches, search highlights or shared find state |
| [Editor](docs/architecture/editor.md) | `Editor`, `DiffView` or their highlighting |
| [Rich editor](docs/architecture/richeditor.md) | `RichEditor`, its kernel, extensions or formats |
| [Terminal](docs/architecture/terminal.md) | `base/terminal`, its emulator binding or pseudo-terminals |
| [Languages](docs/architecture/languages.md) | `i18n`, a package's catalogs or the code `nsgo` writes from them |
| [Icons](docs/architecture/icons.md) | SVG parsing, `core.Icon` or the Lucide pack |
| [Performance](docs/architecture/performance.md) | The performance monitor, a frame clock or a per-frame path |
| [Embedding](docs/architecture/embedding.md) | `nsgo.NewView`, the host channel or `-embed` packages |
| [Android engine and view lifetime](docs/architecture/android-lifecycle.md) | `NsgoEngine`, `NsgoView`'s attach and detach, or Android embedding in services and several Activities |
| [Mini programs](docs/architecture/miniprogram.md) | The mini program backend, its runtime or the `miniprogram_*` targets |
| [HarmonyOS](docs/architecture/harmonyos.md) | The ArkTS host, the render layer or the `harmonyos` target |
| [Testing](docs/architecture/testing.md) | `nsgotest`, its steps or a backend's test host |
| [Tool](docs/architecture/tool.md) | `nsgo`: the dev server, tests on targets, packages or signing |
| [TSX and dynamic delivery](docs/architecture/tsx.md) | TSX, signed packages or the sandbox |
| [NSGo and NSH](docs/architecture/nsh.md) | Porting NSH's work, departing from it or moving to its next release |
