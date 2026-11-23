# Package layers

NSGo's packages form layers, and each imports only downward: apps sit on components, components on `core`, and `core` on the protocol in `internal/driver`. Backends speak that protocol too, so `core` imports no platform. Read this before adding a package, changing what one imports or deciding where new code belongs.

[Code map](../../ARCHITECTURE.md#code-map) says what each package does; this document says how they stack and why.

## Layers

Each layer imports only the layers below it, and packages within a layer may import each other. The tree groups packages by layer, not by folder:

```text
nsgo
├── Apps
│   ├── examples/gallery, tray, tsx      Demos of what apps are made of
│   ├── examples/embedded, embed-*       An embeddable app and its hosts
│   └── benchmark                        The same apps on other frameworks
│
├── Entry points
│   ├── nsgo (root)                      Run, NewView and their options
│   │   ├── internal/backend             Picks a backend and starts the app
│   │   ├── internal/dev                 The dev client of nsgo dev's builds
│   │   │   └── locales                  Its failure overlay's words
│   │   └── internal/restart             Starts a desktop app again once it exits
│   ├── tsx                              Runs TSX apps on goja, or the page's engine
│   │   ├── internal/js                  The engines' one API
│   │   ├── bundler, compiler            Compiles and loads their bundles
│   │   ├── textview, editor, diff, terminal  Register base's optional components
│   │   └── tsxtest                      Runs *.test.tsx as Go tests
│   └── nsgotest                         Runs apps in tests, headless or on a target, and drives nsgo dev -automation's
│
├── Components
│   ├── ui                               Styled components on base
│   │   ├── chat                         A conversation
│   │   └── filepicker, diff, commitgraph, flow  A file picker and styled forms
│   ├── base                             Unstyled controls, popups, navigation
│   │   ├── datatable, dock, plot, toast Complex components
│   │   ├── commitgraph                  A Git history's revision graph
│   │   ├── flow                         A directed diagram
│   │   ├── textview, editor, diff, formula  Complex components with heavier dependencies
│   │   ├── terminal                     A program in a pseudo-terminal
│   │   ├── locales                      Their catalogs
│   │   └── internal/                    What they share
│   ├── mobile                           What only phones have
│   ├── web                              Browser document metadata
│   ├── audio                            Sounds and players, on core's part
│   ├── gamepad                          Game controllers in the W3C Standard Gamepad layout
│   ├── desktop                          What only desktops have
│   │   └── locales                      Its menus' and permission guide's words, which backends read
│   ├── updater                          Updating the app the platform's way
│   │   └── locales                      Its alerts' words
│   ├── icons/lucide                     The bundled icons
│   └── i18n                             The language the app shows
│       └── internal/cldr                Dates and pseudo locales by CLDR's data
│
├── Backends
│   ├── internal/platform
│   │   ├── macos, ios                   AppKit and UIKit
│   │   ├── windows                      Win32 and Direct2D
│   │   ├── linux                        GTK 4
│   │   ├── android                      Java runtime over pipes
│   │   ├── web                          WebAssembly and the DOM
│   │   ├── miniprogram                  WebAssembly in a WeChat, QQ, Alipay or Douyin mini program
│   │   ├── apple                        Shared by macOS and iOS
│   │   ├── applelog                     Process diagnostics in Apple’s unified log
│   │   ├── privacy                      Shared permissions of macOS and iOS, which microphone shares too
│   │   ├── webstyle, canvas2d           Shared CSS and 2D canvas player of the web and mini programs
│   │   ├── field, hover, finger         Shared input
│   │   ├── scrollbar, textselect        Shared scrolling and selection
│   │   ├── speechd                      speech-dispatcher's protocol, Linux's speaking
│   │   ├── pcmoutput                    Shared pull-based PCM output loop and timing
│   │   ├── imagedecode, imageload       Shared images
│   │   ├── canvasmesh                   Shared triangle-mesh rasterizer of canvases without textured vertices
│   │   ├── menutext, menuspeech         Shared menu labels and the Speech menu's read of a focused page
│   │   ├── loopback                     Shared loopback redirects of desktops' sign-ins
│   │   ├── updates                      Shared update checks of Linux, Android and iOS
│   │   ├── appimage                     Identifies an executable's own AppImage
│   │   ├── picked, fontfile             Shared picked photos and font files
│   │   ├── codescan                     Codes read in Go from camera frames, where the system reads none (Android, Windows, Linux)
│   │   ├── videoplay                    App and platform ownership of video playback
│   │   ├── queue, perfhud               Shared batches and readings
│   │   └── canvastest, imageloadtest    Shared test drawings and HTTP client checks
│   └── internal/audiosession            iOS's one audio session, shared with microphone
│
├── Engine
│   ├── internal/engine                  One app on its goroutine, a batch per commit
│   └── internal/testhost                Where nsgotest finds a backend's test host
│
├── Core
│   ├── core                             Elements, styles, reactive graph, shadow tree
│   ├── internal/layout                  Block, flexbox and grid layout
│   └── internal/shader, shader/wgsl     The shader compilers
│
├── Protocol
│   ├── internal/automation              Requests the nsgo tool sends a running dev build's agent
│   ├── internal/crashreport             Crash output and pending report files
│   ├── internal/appstartup              Packaged startup identities shared by the tool and runtime
│   ├── internal/mimeinfo                Fixed extension/MIME table shared by the tool and runtime
│   ├── internal/woff                    WOFF and WOFF2 fonts unpacked, for the bundler
│   ├── internal/driver                  Mount items, events, TextEngine, shared values
│   ├── internal/devtools                What DevTools and the automation's agent read from core's trees
│   └── internal/shaderir                Shader IR, written as MSL, GLSL or HLSL
│
└── Build time, outside the layers
    ├── cmd/nsgo                         The nsgo tool
    ├── tools/changelog                  Folds changelog.d into CHANGELOG.md for a release
    ├── tools/lucidegen                  Generates icons/lucide
    └── tools/libghostty                 Builds libghostty-vt and its themes
```

What keeps each layer in its place:

| Layer | Never does |
| --- | --- |
| Apps | Import `internal/` |
| Entry points | Implement UI |
| Components | Style themselves or call the system |
| Backends | Decide what the UI contains |
| Engine | Touch main-thread-only UI objects |
| Core | Touch a native view or call the system |
| Protocol | Hold app logic |

The `nsgo` tool links no component or runtime, so changing them does not rebuild it. It shares packaged startup identities with backends through `internal/appstartup`, which has only constants and no dependencies. It is the only package that imports the MCP SDK (`github.com/modelcontextprotocol/go-sdk`), which every app's module graph gains through its `tool` line.

## Dependency graph

A line means "imports". `internal/moduletest/layering_test.go` checks the graph on every release and test build, including `nsgowatch`, `nsgoembed` and `nsgominiprogram`, and fails on an import no rule allows. A backend may import `core` only in its `nsgotest` or `nsgoautomation` test host; a release backend never does.

```text
                     apps (examples/*)
         ┌───────────────────────┼────────────────────┐
         │                       │                    │
    nsgo (root)         tsx, tsx/compiler         nsgotest
         │                       │                    │
         │       ui/*  base/*  mobile  audio  desktop  │
         │             icons/*  i18n                  │
         │                       │                    │
 internal/platform/<target>      │                    │
    │    │                       │                    │
    │  internal/engine           │                    │
    │    └───────────────────────┼────────────────────┘
    │                          core
    │                            │
    └───────────────────► internal/driver
```

- Apps may import any public package directly, `core` included. `core`, or the public package itself, aliases each internal type a public package's API reaches; `TestPublicAPINamesNoInternalType` lists the public packages with `go list`, every package outside `internal/`, `cmd/`, `tools/` and `examples/`, so a new one is checked without a list to edit. The test also forbids protocol types that have been removed from the public API, including the window and share messages.
- `internal/driver` imports `internal/shaderir` for shader programs and `internal/mimeinfo` for file types. The latter holds only fixed tables and standard-library helpers, so the build tool shares those names without linking the runtime.
- `core` imports `internal/driver`, `internal/devtools`, `internal/crashreport`, `internal/layout` and the shader compilers (`internal/shader`, `internal/shader/wgsl`, `internal/shaderir`). `internal/devtools` imports only `internal/driver`, for the batches and events it hands `nsgo dev -automation`'s agent, so DevTools' overlay, the layout audit's included, lives in `core` (`UpdateHighlights`).
- Backends import `internal/driver`, `internal/engine`, `desktop/locales`, their shared packages and the shader writers, never the compilers, which only `core` links.
- Only a backend's test host, built with the `nsgotest` tag, imports `core`; its surface (`testsurface.go`) also builds with `nsgoautomation`, for `nsgo dev -automation`'s live windows. The release, embed, watch and mini-program backend builds are checked separately so a test-only import cannot leak into an app.
- The root starts the dev client (`internal/dev`), which imports `internal/devtools` and `internal/driver` only in dev builds, to write `-trace`'s batches and DevTools' commits, its catalog `internal/dev/locales`, of `core` and `i18n` alone, for its failure overlay's words, and `nsgotest` only in `nsgo dev -automation`'s (`internal/dev/automation.go`, the `nsgodev` and `nsgoautomation` tags), whose agent `nsgotest` and the tool reach through `internal/automation`, which imports nothing of NSGo's; `TestLayering` lists that build too, and `TestMinimalAppLinks` fails a release app that links `testing`, `nsgotest` or the protocol.
- `audio` imports only `core`, as `mobile` does: `core` does the engine's part, so its sounds and players reach the platform through the batch.
- `nsgotest` imports `widget` for the timeline and activity values its test backend records. `internal/driver` holds the process-wide hook without importing `widget`.
- `gamepad` imports `core`, `internal/driver` and `internal/platform/objcblock`, reaching GameController on Apple, XInput on Windows, evdev on Linux, `navigator.getGamepads` on the web, and `internal/driver`'s bridge on Android and in `nsgotest`. `tsx` imports `gamepad` for `nsgo/gamepad`.
- `i18n` imports only `core` and `internal/cldr`; `base`'s packages import it for their catalogs, as `ui/chat/locales` does for its own.
- `updater` imports only `core` and its catalogs; `internal/backend` imports it, to check as the app starts where the platform's updater keeps no schedule.
- `ui` imports `base`, `core`, `mobile`, `desktop` and the icons; `base` never imports it.
- `tsx/tsxtest` is the one package that imports both `tsx` and `nsgotest`, and `tsx/ui` with the packages below it the TSX packages that import `ui`. `tsx/internal/js`, the engines' API, imports nothing of NSGo's, and on the web reaches the page through `syscall/js`.
- `base/terminal` is the one package of `base` that is not portable: it loads libghostty-vt through purego, runs programs on desktops and remote sessions there and on iOS, and elsewhere builds to report that it cannot (`unportable` in `layering_test.go`).

## Why core and backends do not meet

Backends exchange view and service messages with `core` through the engine, so `core` builds and tests without importing a backend. Three packages join them:

- `internal/driver` is what they exchange: `core` writes a `MountBatch`, and a backend reports `driver.Event`s and implements `TextEngine`.
- `internal/engine` runs `core` and defines `engine.Platform`, which each backend implements.
- `nsgo` gathers `Run`'s and `NewView`'s options and hands the app to `internal/backend`, which picks the backend by build tags (`run_windows.go`, `view_windows.go`).

`nsgotest` is a backend of its own: headless, without the engine, applying each batch to a model of the native views. On a platform, it drives a backend's test host through `internal/testhost`, whose `PostToTest` hook `tsx/tsxtest` uses to run its VM's work on the test goroutine.

`microphone` stands apart from the layers, as recording has nothing to do with views: it loads the system's audio libraries through purego, holds iOS's audio session while it records through `internal/audiosession`, which imports nothing of the module and which the backends' videos and speech hold too, reaches Android's runtime, whose foreground service records, through `internal/driver`'s recording channel, which the backend connects outside any surface, as a recording belongs to the process, and elsewhere builds to report that it cannot. It asks for the microphone through `internal/platform/privacy`, which the Apple backends answer `core`'s permission requests, speech and iOS's camera with, so the permission has one implementation; `privacy` imports `internal/driver` and the block bridge `internal/platform/objcblock`. `widget` stands apart too, as widgets are drawn by an extension the tool compiles rather than by the engine: it imports `core`, for its colors, edges and fonts, writes timelines for the extension and loads the tool's library of Swift through purego ([tool](tool.md#widgets)). On Android its Live Activities reach the runtime through `internal/driver`'s Live Updates, a channel outside any surface, which the backend connects, as an activity outlives the app's views. `gameservices` follows the same boundary ([game services](gameservices.md)): it imports `core`, `internal/driver` and `internal/platform/objcblock`, reaches Apple's GameKit through `purego` on macOS, iOS and tvOS, talks to Android's runtime through `internal/driver`'s game services channel, uses `core.GameServicesOf` for `nsgotest`'s per-app fake, and returns `ErrUnsupported` on other targets.

### Driver hooks

- `Applied` lets the dev trace time a backend applying a batch through `driver.Apply`.
- `BlobFile` lets the web backend bind a blob URL to a readable file with its metadata.
- `ResolvePageAddress` lets the web backend validate and canonicalize an app address with the browser's URL parser before the core updates its address or history.
- `UpdateDownload` lets `core.DownloadUpdate` use the platform HTTP client without a surface. It verifies and stages the package before installation is requested through the engine.

Portable packages build unchanged for every target, with no purego, `syscall` or `unsafe`: `core`, `base` but for `base/terminal`, `ui`, `i18n`, `mobile`, `web`, `audio`, `desktop`, `icons`, `tsx`, `nsgotest`, `internal/automation`, `internal/driver`, `internal/appstartup`, `internal/mimeinfo`, `internal/layout`, `internal/testhost`, the shader compilers and `internal/engine`. No package has cgo files or hand-written C.

## Backends

Each `internal/platform/<target>` maps the same protocol onto its system ([platforms](../../ARCHITECTURE.md#platforms)):

| Target | A view becomes |
| --- | --- |
| `macos`, `ios` | A layer-backed `NSView` or `UIView` per node |
| `windows` | An entry in a retained tree one window draws with Direct2D |
| `linux` | A GTK widget per view that draws itself with Cairo |
| `web` | An absolutely positioned element per view |
| `android` | An Android view per node, in the Java process the Go executable talks to over pipes |
| `watchos` | A SwiftUI view per node in the host linked into the executable, its look a Canvas Go draws with Core Graphics |

The other packages under `internal/platform` hold what two or more backends share, such as `apple` for macOS and iOS or `hover` and `textselect` for Windows and Linux. Move code there once a second backend needs it, and add it to `platformShared` in `layering_test.go`. The pull request check flags new copies of multi-statement functions across backends. It compares pairs of declarations with the base branch, so a refactor can edit or move a pair the base already had. When two copies must remain, record the reason in a `// nsgo:allow-duplicate` comment immediately above the function.

Apple backends, `privacy` and `microphone` create Objective-C blocks through `objcblock`. It pins each Go template while `_Block_copy` makes native storage, checks allocation, and keeps a callback until the block's last native release. Descriptors in native memory and one invoke trampoline per signature live for the process. This bridge avoids [purego v0.11.3's temporary stack address converted to `uintptr`](https://github.com/ebitengine/purego/blob/v0.11.3/objc/objc_block_darwin.go#L239-L246) before the copy, which Go cannot update when its stack moves; NSH's `RcBlock` has no Go stack to cross.

## What an app links

Go's linker keeps what a reachable function, method or package initializer reaches, so each package keeps what only some apps use out of reach of the rest:

- An element mounts through its own method (`mountIn`), not a case of the tree's type switch, so the elements an app never makes leave their code out: DevTools' overlay with the layout audit, which only the dev client mounts, links only into dev builds, the hooks that keep the audit's boxes up to date included, which the overlay sets as it mounts; a release build keeps a flag frames set and the hooks' nil checks (`TestReleaseBuildsLinkNoDevClient`).
- No package variable, `init` or package-level regular expression reaches a compiler, codec or table that only some apps need: build it on first use inside a function, as `core`'s default transition program and WGSL's built-ins are.
- A backend leaves to the system what it has: images download through its HTTP client (`imageload.HTTP`), as do Apple's videos with request headers (`internal/platform/apple/videoresource.go`, a URLSession), the web decodes and encodes images with the browser's codecs, and only Android imports `archive/zip`, for its APK. What it must serve itself takes the standard library's smallest part: a sign-in's loopback redirect reads its one request over `net` (`internal/platform/loopback`).
- What is large comes in by the app's own import: formulas, Untie and their font, only with `base/formula` (`RenderMath: formula.RenderMath`), which alone may import Untie, and diagrams and limu only with `base/diagram` (`RenderDiagram: diagram.RenderDiagram`), which alone may import limu (`owned` in `layering_test.go`), as code highlighting comes with `base/textview/highlight`.

`TestMinimalAppLinks` (`internal/moduletest/linked_test.go`) builds an app showing one text (`testdata/minimal`) for each target and fails when another package reaches one of `unlinked`'s optional packages, including Go's HTTP and TLS stack, the shader compilers and, on the web, Go's image codecs. `TestTextViewAppLinks` builds a `ui.TextView` without `RenderMath`, `RenderDiagram` or `EmojiPicker` (`testdata/textview`) and fails when it links Untie, limu or `base/internal/emoji`. `TestNetworkWatchersLinkOnUse` checks that native watcher methods and their bindings link only when the app reads network state.

The release build of an app showing one text went from 11.5 to 12.3 MB natively and 13.7 MB on the web to 7.5 to 8.1 MB and 10.3 MB with these rules and the tool's smaller encoding/json ([packages](tool.md#packages)) (2026-11-10). A backend's handling of every mount item still links into every app, its shaders' writers among them, since the linker cannot follow an item's kind, and Android keeps Go's DNS resolver for apps' own networking.

## A button, top to bottom

This follows a `base.Button` from an app to the Windows backend; another backend replaces only the Windows parts.

1. The app calls `nsgo.Run(Root)`, and `Root` returns a `base.Button`.
2. `internal/backend/run_windows.go` is the only `run` a Windows build has, so `nsgo.Run` reaches `windows.Run` through `backend.Run`.
3. `windows.Run` creates the window, which is an `engine.Platform`, and calls `engine.Start` with the app.
4. On the engine goroutine, `Button.Render` returns a `core.View` with the app's styles, its role and its handlers.
5. `core` resolves styles, prepares text through the window's `TextEngine` (DirectWrite), lays out and commits a `driver.MountBatch`.
6. `window.Mount` queues the batch (`internal/platform/queue`) and posts `wmApply` to wake the UI thread without waiting.
7. The UI thread applies the items to its retained views and draws them with Direct2D.
8. A press returns as a `driver.Event` through `Engine.Dispatch`; `OnPress` runs on the engine and leads to the next commit.

## Where a change goes

| Change | Layer |
| --- | --- |
| A look, a variant or a styled control | `ui`, or the app for what is its own |
| An unstyled component | A file of `base`, or a package under it if it is complex |
| What only phones or only desktops have | `mobile` or `desktop` |
| A new element or service | `core` and `internal/driver`, then every backend |
| A drawing or input bug on one target | `internal/platform/<target>`, then check the other backends |
| A new package | Its layer, with a rule in `layers` in `internal/moduletest/layering_test.go` |
