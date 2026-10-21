# NSGo architecture

NSGo is a Go port of the design of [Native Shell (NSH)](https://github.com/longbridge/nsh). A Go engine owns the reactive graph, the shadow tree, styles and layout. Each commit produces one batch of mount items, and a thin platform backend applies it to native views. A minimal version renders text, inputs and buttons; sections below say what exists and what is planned.

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
| `core` | `nsh-core` | Geometry, styles, reactive graph, elements, shadow tree, flex layout, mount items |
| `base` | `nsh-base` | Unstyled base components; `Button` today |
| `engine` | `nsh-engine` | Engine goroutine: runs the app, commits, hands each batch to the platform |
| `platform/<target>` | `nsh-platform-*` | Native view mounting, text engine and input for one target |
| `platform/internal/apple` | Shared Apple code | CoreFoundation, CoreGraphics and CoreText through purego |
| `nsgo` (module root) | `platform::run_app` | `nsgo.Run`: picks the backend by build tags; the tool sets name and window size |
| `cmd/nsgo` | `nsh-cli` | `nsgo dev` and `nsgo build`, driven by an app's `nsgo.yaml` |
| `examples/hello` | `examples/counter` | Text, input and button in one main package |

## Platforms

Every target builds on macOS with Go, and iOS also with Xcode. No backend contains hand-written C.

| Target | Package | Reaches the system through | Status |
| --- | --- | --- | --- |
| macOS | `platform/macos` | purego and the Objective-C runtime; `CGO_ENABLED=0` | Runs |
| iOS | `platform/ios` | The same; cgo only because Go links iOS with Xcode's linker | Runs in the simulator |
| Web | `platform/web` | `syscall/js` in WebAssembly; DOM elements | Runs |
| Windows | `platform/windows` | `syscall`: Win32 window drawn with GDI, native EDIT inputs | Builds, never run |
| Android | `platform/android` | A Go executable the prebuilt Java runtime starts and talks to over pipes | Builds; runtime `classes.dex` and APK packaging pending |
| Linux | — | Planned: GTK 4 through purego | Not started |

Android runs the app as a second process because Go builds Android shared libraries only with cgo and the NDK. Without cgo, Go builds Android executables for arm64 only, so 32-bit devices and x86_64 emulators are out of reach. iOS forbids child processes, so it links in-process with Xcode instead.

## Boundaries

- `core` never imports `engine`, `platform` or any cgo package. It stays pure Go and testable headless.
- `base` depends on `core` only. Styled component libraries are never part of NSGo; apps build their own on `base`.
- Backends call system APIs through purego or `syscall`, never hand-written C.
- The platform main thread never waits for the engine. The engine never touches main-thread-only UI objects.
- Text is prepared and laid out on the engine side through a platform `TextEngine`; the main thread only draws the result.
- One engine is one app instance with no global state, so a host app can embed several.
- Keep NSH's public names and semantics unless Go requires otherwise, so NSH's documentation stays a usable reference.

## Tool

`nsgo` is the only build entry point; it shells out to `go build`, and to Xcode's `xcrun`, `codesign` and `simctl` for Apple targets.

- Targets name an OS and an architecture (`macos_arm64`, `windows_x64`), so each package holds one binary.
- `nsgo build` is a release build: `-s -w`, `-trimpath`, no VCS stamp. `nsgo dev` keeps symbols.
- `nsgo dev` watches the directories `go list -deps` reports, NSGo's own included, and keeps the last good build running when a build fails.
- The web dev server reloads open pages over server-sent events after each build.

## Core flows

**Render.** App components run once and bind signals to element props. A change marks the shadow tree, `commit` recomputes layout and diffs frames, and the result is one `MountBatch`.

**Mount.** The platform receives the batch with a single hop to its main thread and applies the items in order to native views.

**Input.** Native gestures become engine events, run the handlers on the engine side and lead to the next commit.

## Open questions

- Whether NSGo keeps NSH's TSX and dynamically delivered modules, or supports Go apps only.
- Whether `core`'s small flexbox grows into a full one or gives way to a Go port of Taffy.

## Verification

`core` has headless Go tests for signals, layout and commits. Backends need a run on their target: screenshots of the window, simulator or headless browser, and input where it can be simulated.

## Subjects

Subjects go in `docs/architecture/` once a subsystem has a lasting design.

| Document | When to read |
| --- | --- |
