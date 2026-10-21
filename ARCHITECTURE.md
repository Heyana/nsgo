# NSGo architecture

NSGo is a Go port of the design of [Native Shell (NSH)](https://github.com/longbridge/nsh). A Go engine owns the reactive graph, the shadow tree, styles and layout. Each commit produces one batch of mount items, and a thin platform backend applies it to native views. This document describes the target design; nothing here is implemented yet.

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

Planned packages, mirroring NSH's crates:

| Package | NSH counterpart | Responsibility |
| --- | --- | --- |
| `core` | `nsh-core` | Geometry, styles, shadow tree, layout, text merging, reactive graph, mount items |
| `base` | `nsh-base` | Unstyled base components: navigation, dialogs, controls, safe areas |
| `engine` | `nsh-engine` | Engine goroutine: runs the app, commits, hands one batch per frame to the platform |
| `platform/<os>` | `nsh-platform-*` | Native view mounting, text engine, input and the app view for one platform |
| `cmd/nsgo` | `nsh-cli` | Build, run and package apps |

## Boundaries

- `core` never imports `engine`, `platform` or any cgo package. It stays pure Go and testable headless.
- `base` depends on `core` only. Styled component libraries are never part of NSGo; apps build their own on `base`.
- The platform main thread never waits for the engine. The engine never touches main-thread-only UI objects.
- Text is prepared and laid out on the engine side through a platform `TextEngine`; the main thread only draws the result.
- One engine is one app instance with no global state, so a host app can embed several.
- Keep NSH's public names and semantics unless Go requires otherwise, so NSH's documentation stays a usable reference.

## Core flows

**Render.** App components run once and bind signals to element props. A change marks the shadow tree, `commit` recomputes layout and diffs frames, and the result is one `MountBatch`.

**Mount.** The platform receives the batch with a single hop to its main thread and applies the items in order to native views.

**Input.** Native gestures become engine events, run the handlers on the engine side and lead to the next commit.

## Open questions

- How Go reaches each platform: cgo with a C ABI, gomobile for iOS and Android, or a per-platform mix.
- Which layout engine to use: a Go port of Taffy or an existing Go flexbox implementation.
- Whether NSGo keeps NSH's TSX and dynamically delivered modules, or supports Go apps only.
- The web target: Go compiled to WebAssembly, and its binary size.

## Verification

There are no tests yet. `core` must be covered by headless Go tests; platform backends need manual review on each OS.

## Subjects

Subjects go in `docs/architecture/` once a subsystem has a lasting design.

| Document | When to read |
| --- | --- |
