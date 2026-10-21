# Core

`core` turns Go components into one `MountBatch` per commit. Components run once; signals drive bindings, regions rebuild only what changed, and the commit lays the tree out and sends the differences. Read this before changing `core`, `engine` or the mount protocol a backend implements.

## Reactive graph

The graph follows NSH's (Solid 1.9's scheduling): signals, memos, render effects for element bindings and user effects for the app, run in that order after each batch.

- Every computation and scope is an owner. Disposing an owner disposes what it created and runs its cleanups, last first, as `testing.T.Cleanup` does.
- Go has no goroutine-local state, so code finds its graph through `cx *Scope`. While the graph runs code, what that code creates belongs to the innermost running owner, whichever `cx` it captured. Handlers run under the owner their element was built in.
- The graph belongs to the engine goroutine and never locks. Other goroutines reach it with `Scope.Post`. `Scope.Context` is cancelled when the owner goes, so work started for a screen stops with it.
- Timers go through the `Clock` interface. Tests inject a `ManualClock`.
- A panic in app code is recovered and sent to the nearest `Catch` above the owner it happened under. With no `Catch`, the host reports it and the app keeps running.

## Elements and components

Host elements (`View`, `Text`, `Input`, `Scroll`) are structs with exported fields: zero values are defaults, as in `http.Server`. A field that can change takes a function (`StyleFunc`, `ContentFunc`, `Value`, `Disabled`), bound by one render effect per part. Styles compose with `Style.Merge`, and text fields inherit down the tree as in CSS. State styles (`Hover`, `Focus`, `Active`, `Disabled`) merge over the rest; the engine tracks those states from the press, hover and focus events it asks the platform to report.

A component is any type whose `Render(cx) Node` returns its nodes, like `http.Handler`. Its fields are its props. `Render` runs once under a scope of its own, so `Key.Provide` and cleanups stay inside it. `Func` turns a function into a component.

## Regions

`Show`, `Each`, `Dynamic` and `Catch` mount as regions. A region is a node with no native view, laid out as part of its parent's children, and owned by one render effect that rebuilds its span.

- Branches and rows belong to the region's owner, not to its effect, so they outlive the effect's runs and go only when replaced.
- `Each` matches rows by key. A kept row gets its item and index through signals, and only the rows that moved are reinserted.
- Go methods cannot take type parameters, so `Each` is a generic function rather than a method or struct.

## Commit

`Tree.Commit` runs up to eight rounds. `OnLayout` handlers' changes go into the same batch.

1. **Resolve.** Inherit text styles and prepare text whose content or style changed.
2. **Layout.** CSS flexbox with React Native's defaults, in `layout.go`. Nodes are measured at any number of constraints, cached until they or a descendant change, and placed once at their final size.
3. **Emit.** Compare each view's parts with what was sent: view style, props, events, text, input, scroll. Then sync children against the sibling order last sent, reinserting only the views outside the longest run already in order.

## Text

A platform's `TextEngine` runs on the engine goroutine in three phases. `Prepare` shapes the text once, `PreparedText.Measure` answers layout at any width, and `PreparedText.Layout` lays it out at its final size. The `TextLayout` travels in `ItemText`, and the main thread only draws it and releases the one it replaces. The core releases prepared text when it changes or its node goes.

## Mount protocol

The protocol is defined in `mount.go` and `events.go`. Backends apply items in order on the main thread, in one hop per batch.

- IDs are dense and reused after `ItemDelete`, possibly later in the same batch.
- `ItemInsert` places a view before a sibling, not at an index, so views a platform adds itself cannot shift positions.
- A text field counts the user's edits. `ItemInputValue` carries the edit count the engine had seen, and the field ignores a value older than its last edit.
- Platforms report only the events in a view's mask, and never wait for the engine. The engine queue is unbounded, merges scroll events per view and keeps only the last resize.

## Where Go departs from NSH

| NSH (Rust) | NSGo | Why |
| --- | --- | --- |
| Builders, `#[component]`, derive macros | Struct literals; components are types with `Render` | Go's idiom for many optional fields; no macros |
| `Reactive<T>` and `IntoReactive` | A `func() T` field beside the static one | A method value such as `sig.Get` already is the accessor |
| Copy handles in a generational arena | Pointers and the garbage collector | Go frees what nothing references |
| `TypeId` context keys | `Key[T]` values declared as package variables | Typed, comparable and cheap |
| A future executor on the engine thread | Goroutines, `Scope.Post` and `Scope.Context` | Go has them built in |
| `catch_unwind` | `recover`, reported as `PanicError` | The same, in Go terms |
| `MountItem` enum | One struct with a `Kind` and pointers to large parts | No allocation per item kind; easy to encode across a pipe |

## TSX entry

Go is the only app language for now. The tree's internal operations are the entry a TSX runtime would drive, as NSH's JS renderer drives its ops: create a node, set its parts, place it among siblings, discard it. No runtime calls them yet.
