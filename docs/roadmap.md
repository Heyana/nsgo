# Roadmap

This plan lists what NSGo still lacks compared with NSH's 0.3.0 release (tag `v0.3.0`, `76fa8a7f`), in the order to build it. Each item names its NSH reference and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

References are files in `nsh/docs/dev/` unless they give a path from `nsh/`, with the bullet or section to read in brackets. To refresh the plan for NSH's next release, check out its tag in `nsh/`, then read `git log v0.3.0..HEAD` there and the documents it changed.

## Finish the existing platforms

This item waits for community contributions.

| Item | NSH reference | Done when |
| --- | --- | --- |
| Real file drags | `file-drop.md` | A real drag from Finder and from a Linux file manager is checked to reach a view, as one from Explorer is |

## Not ported

NSGo leaves out these NSH parts:

- The JS runtime, bundler, TSX compiler, `nsh check`, compatibility baselines and hot module replacement, since apps are Go and `go vet` checks them.
- Tailwind classes, since styles are Go values.
- `fetch`, WebSockets, `resource` and the standard modules, since apps use Go's standard library or other Go libraries with `Scope.Post`.
- The machinery that carries a Rust component's refs, slots, render props and typed rows across JS, since a Go component takes them as fields: content as `core.Node` (`Button.Children`, `Popover.Trigger`, `Checkbox.Indicator`), rows and callbacks as functions.
- `EngineContext`, since a host gives each engine's app function its values directly.
- The GPUI backend and the LayerFS trial.
- 32-bit Android (`armeabi-v7a`), since Go links android/arm only with cgo and an NDK.
