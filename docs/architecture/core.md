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

Host elements (`View`, `Text`, `Input`, `Scroll`, `List`, `Image`, `Icon`, `Canvas`, `WebView`, `Video`, `Overlay`, `Modal`, `Stack`, `Screen`) are structs with exported fields: zero values are defaults, as in `http.Server`. [Web views and videos](native-views.md) covers the hosted platform controls.

- A field that can change takes a function (`StyleFunc`, `ContentFunc`, `Value`, `Disabled`), bound by one render effect per part.
- Styles compose with `Style.Merge`, and text fields inherit down the tree as in CSS.
- State styles (`Hover`, `Focus`, `Active`, `Selected`, `Checked`, `Indeterminate`, `Disabled`) merge over the rest. The engine tracks interaction states from the press, hover and focus events it asks the platform to report.
- A `Ref`, usable at its zero value, names a mounted element for anchoring, focus, scrolling and its bounds.

A component is any type whose `Render(cx) Node` returns its nodes, like `http.Handler`. Its fields are its props. `Render` runs once under a scope of its own, so `Key.Provide` and cleanups stay inside it. `Func` turns a function into a component.

## Regions

`Show`, `Each`, `Dynamic` and `Catch` mount as regions. A region is a node with no native view, laid out as part of its parent's children, and owned by one render effect that rebuilds its span.

- Branches and rows belong to the region's owner, not to its effect, so they outlive the effect's runs and go only when replaced.
- `Each` matches rows by key. A kept row gets its item and index through signals, and only the rows that moved are reinserted.
- Go methods cannot take type parameters, so `Each` is a generic function rather than a method or struct.
- Region nodes are never laid out. Layout marks pass through them, so a change inside a branch or row still reaches the root.

## Layers and navigation

`Overlay` and `Modal` stay in the shadow tree where they are built, for their owner, context, inherited text and lifetime. Their native views are hoisted into their host: the nearest modal around them, or the root view. Each is a layout root of its own at the host's size, placed after the main layout.

- An overlay with an anchor goes beside it as NSH's `place` does: the side asked unless only the opposite side fits or has more room, aligned along it, then clamped into the safe area less its margin.
- A modal provides the insets its content keeps clear of; a screen does too, without the top when it has a header. `Insets` reads them.
- `EventBack` (Escape, the back button) goes to the topmost overlay that listens for cancel, else the topmost modal's `OnRequestClose`, else the innermost stack that can pop. A popped screen reports `OnDismiss` and stays hidden until the app removes it; the last screen never pops.
- `EventPressOutside` reports presses outside an overlay, ignoring those on its anchor or on overlays above it.

## Lists

`List` mounts only the rows within a viewport of its scroll offset, in cells it reuses: a cell's content is built once, and its `ListRow` signals move it to another row. Row sizes start as an estimate and are corrected as rows are laid out, in a Fenwick tree, so offsets and the row at an offset cost O(log n). The window is refilled after each layout, within the same commit.

## Commit

`Tree.Commit` runs up to eight rounds. `OnLayout` handlers' changes go into the same batch.

1. **Resolve.** Inherit text styles and prepare text whose content or style changed.
2. **Layout.** CSS block, flexbox and grid from `internal/layout` over each view's native children; overlays and modals are layout roots of their own. Results stay cached until a node changes, and frames are rounded to whole points as Taffy does.
3. **Emit.** Compare each view's parts with what was sent: view style, props, events, text, input, scroll. Then sync children against the sibling order last sent, reinserting only the views outside the longest run already in order.

`layout.go` maps styles with React Native's defaults, as NSH does:

- Column direction, no shrinking and no automatic minimum sizes.
- `flex-start` content alignment outside grids.
- Left and right swapped when right to left.

## Text and drawing

A platform's `TextEngine` runs on the engine goroutine in three phases. `Prepare` shapes the text once, `PreparedText.Measure` answers layout at any width, and `PreparedText.Layout` lays it out at its final size. The `TextLayout` travels in `ItemText`, and the main thread only draws it and releases the one it replaces. The core releases prepared text when it changes or its node goes.

The family `FontMonospace` is each platform's fixed-pitch font: CoreText's user fixed-pitch font, CSS's `ui-monospace` stack, Consolas on Windows, Pango's `monospace` on Linux, Android's `monospace`.

`Text.Spans` style byte ranges over the text's style; offsets are UTF-8 bytes, Go's own, and platforms convert them. A press on a span with a handler comes back with the byte offset under the point.

- A span's `Background` and `CornerRadius` become its `TextBackground`: drawing only, a box per line from the font's ascent to its descent, rounded on every line, behind the selection and the glyphs. Hanging spaces at a line's end are not painted.
- A span's `Attachment` is a view inside the text, as NSH's inline attachments are. Its view is a layout root at its content size, laid out before the text is prepared, so a change inside it prepares the text again.
- The text holds an `AttachmentRune` in an attachment's place, and `AttributedText.Attachments` gives the engine its size and `VerticalAlign`.
- Every engine places an attachment by NSH's rule, `TextAttachment.Ascent` and `Extent` against the placeholder font's `FontExtent`, so a line grows where an attachment reaches past its font.
- After layout the core asks the `AttachmentLayout` where each attachment went and sends its view as a native child of the text view at that frame. Those cut off by the line limit are hidden.

A `Canvas` records its drawing on the engine into an immutable `Drawing`, again when a signal it read or its size changes, and the platform plays the operations on the main thread.

## Text selection

Selection follows NSH's: one selection runs across the texts of a selection area (a `View` with `SelectionArea`, which `base.SelectionArea` sets), in document order, as a web page's does. A `Text` with `SelectableOn` outside any area is an area of its own; one with `SelectableOff` stays out. The highlight is the inherited `SelectionColor`, else the platform's.

- `ViewProps` carries `SelectionArea`, `Selectable` and `SelectionColor`, so a selection never waits for the engine.
- `SelectionModel` in `selection.go` is the platform-independent model macOS and iOS drive on their main thread. Windows does not select text yet.
- A backend lists an area's texts as `SelectionTexts`, with content, frames, hit test and NodeIDs, then feeds it presses, drags and long presses and draws its `Ranges`.
- Offsets are UTF-16. `Text` joins texts on one row with a tab and others with a line break, leaving attachments out.
- macOS takes mouse presses in its event monitor before views see them; pressables, fields and gestures inside an area keep theirs. The surface becomes first responder for Copy, Select All and Escape.
- iOS selects with a long press, then by words as the finger drags, and shows the edit menu; the view controller answers Copy and Select All. Touch handles are not drawn yet.
- The web uses the browser's own selection through `user-select`, and a `copy` handler joins texts as the model does.

## Gestures

`View.OnPan`, `OnPinch` and `OnRotation` follow gestures the platform recognizes; they report to the engine, which merges each gesture's moves per turn and never its first or last report.

- The platform decides on the main thread when a drag becomes a pan, from the view's `PanActivation`. Until it does, a scroll view around may still take the touch.
- An active pan takes the touch from presses and scroll views.
- A drag or wheel turn scrolls the innermost scroll view under it that can move its content that way, so an inner one passes on only at its edge. A touch or trackpad gesture keeps the scroll view it began with. Android still uses its scroll views' own handling.
- Desktop scroll bars show while the pointer is over their scroll view and can be dragged: AppKit's overlay scrollers on macOS, the browser's on the web, and `platform/internal/scrollbar`'s on Windows and Linux.
- NSGo has no native animated values yet, so a drag moves a view through a signal and a commit per turn, not on the main thread as NSH's bound values do.

## Desktop windows

Window chrome follows NSH's `nsh/desktop`: native controls, an app-drawn title bar.

- `SetTitleBar` hides the system title bar. `Env.TitleBar` then reports the area the window controls keep, and views with `WindowDrag` drag the window.
- The platform decides a window drag on the main thread, from the mounted props: the nearest view setting `WindowDrag` decides, and a nearer pressable, input or gesture view keeps the press.
- `SetApplicationMenu` sends the menu as data, with handlers replaced by IDs that come back in `EventMenu`. Role items are the platform's own and are never reported. The default menu returns when the owner that set the menu goes.

## Animation

Style `Transition`s animate on the platform, with no engine work per frame and nothing for Go's garbage collector to pause. `Scope.Animate` steps app values once a frame on the engine for what a platform cannot animate itself; every step commits.

## Services

Native alerts, the clipboard, announcements, keyboard shortcuts, the status bar, the window title, window commands and the application menu are command items answered by events. Shortcuts are scoped to their owner: the platform matches key presses against the table synchronously and reports only matches.

## Storage and directories

`Scope.Storage` follows NSH's `nsh/storage`: strings by key with a 5 MB quota, held in memory by a `Store` and loaded once from the platform's `StorageBackend`.

- Engines given the same `Store` share its entries and quota. Each graph keeps a signal per key it read and one for the key set; its own changes bump them at once, and other graphs' changes arrive through `Post`.
- The store hands each change to the backend under its lock, so every graph and the backend see one order.
- Native targets keep `storage.json` in the data directory: `engine.OpenFileStorage` returns one store per file in the process and rewrites the file on a goroutine of its own.
- A file that does not parse is set aside as `storage.json.corrupt`, and one that cannot be read leaves changes in memory. Platforms call `engine.SyncStorage` as the process ends.
- The web keeps entries in `localStorage` under `nsgo:<app id>:`.
- `Scope.DataDir` and `Scope.CacheDir` are named by the app's ID, which the tool passes from `nsgo.yaml`: `engine.AppDirs` on Apple platforms, Windows and Linux, the files and cache directories the runtime passes on Android, and none on the web. The engine makes them as it starts.

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
| Options with defaults of `true` (`closeOnEscape`) | Negative names (`NoCloseOnEscape`) | The zero value must be the default |
| Per-platform image loading and caching | One Go loader (`platform/internal/imageload`): `net/http`, shared fetches, an LRU, `context` cancellation | Written once; platforms only decode and draw |

## TSX entry

Go is the only app language for now. The tree's internal operations are the entry a TSX runtime would drive, as NSH's JS renderer drives its ops: create a node, set its parts, place it among siblings, discard it. No runtime calls them yet.
