# Core

`core` turns Go components into one `MountBatch` per commit. Components run once; signals drive bindings, regions rebuild only what changed, and the commit lays the tree out and sends the differences. The mount protocol, the events and the values `core` shares with backends live in `internal/driver`; `core` aliases the values its app API reaches in `shared.go`, and `TestPublicAPINamesNoInternalType` fails when one is missing. Read this before changing `core`, `internal/engine` or the mount protocol a backend implements; [text](text.md), [scrolling and lists](scrolling.md) and [interaction](interaction.md) cover those parts of `core`.

## Reactive graph

The graph follows NSH's (Solid 1.9's scheduling): signals, memos, render effects for element bindings and user effects for the app, run in that order after each batch.

- Every computation and scope is an owner. Disposing an owner disposes what it created and runs its cleanups, last first, as `testing.T.Cleanup` does.
- Go has no goroutine-local state, so code finds its graph through `cx *Scope`. While the graph runs code, what that code creates belongs to the innermost running owner, whichever `cx` it captured. Handlers run under the owner their element was built in.
- The graph belongs to the engine goroutine and never locks. Other goroutines reach it with `Scope.Post`. `Scope.Context` is cancelled when the owner goes, so work started for a screen stops with it.
- Timers go through the `Clock` interface. Tests inject a `ManualClock`.
- A panic in app code is recovered and sent to the nearest `Catch` above the owner it happened under. With no `Catch`, the host reports it and the app keeps running.

## Elements and components

Host elements (`View`, `Text`, `Input`, `Scroll`, `List`, `Image`, `Icon`, `Canvas`, `Shader`, `WebView`, `Video`, `Native`, `Overlay`, `Modal`, `InputAccessoryView`, `Stack`, `Screen`) are structs with exported fields: zero values are defaults, as in `http.Server`. [Hosted views](native-views.md) covers the platform controls and the app's own views inside the tree; [images](images.md) the image pipeline; [shaders](shaders.md) the GPU's fills.

- A field that can change takes a function (`StyleFunc`, `ContentFunc`, `Value`, `Disabled`), bound by one render effect per part.
- Styles compose with `Style.Merge`, and text fields inherit down the tree as in CSS.
- State styles merge over the rest in NSH's order: `Checked`, `Indeterminate` and `Selected`, which the app sets, then `Hover`, `Focus` and `Active`, which the engine tracks from the press, hover and focus events it asks the platform to report, then `Disabled` over all; a `GroupStyle` follows its group's states first ([styles](styles.md)). Windows and Linux hit-test hovers themselves (`internal/platform/hover`).
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
- `EventBack` (Escape, the back button) goes to the topmost overlay that listens for cancel, else the topmost modal's `OnRequestClose`, else the `Scope.OnBackPress` handlers newest first until one takes it, else the innermost stack that can pop. A popped screen reports `OnDismiss` and stays hidden until the app removes it; the last screen never pops.
- Where the environment has `HeaderHeight` (iOS, Android), screens get NSH's system header: the core pads the content below the bar plus the status bar it reaches under, sends `ItemScreen`, and lays `HeaderLeft`, `HeaderRight` and `HeaderTitle` out as roots at their content size that the platform puts in the bar. `Header` is then not mounted; elsewhere it is the screen's first child. iOS gives each screen a `UINavigationBar` of its own with a back item, Android a `Toolbar`; their back buttons and iOS's leading-edge swipe send `EventBack` with the screen's ID. Large titles take the taller bar without collapsing on scroll.
- iOS presents a modal with `ModalPageSheet` or `ModalFormSheet` as a sheet view controller, which reports its size (`EventModalSize`) and its swipes (`EventRequestClose`, "swipe"); a swiped sheet stays hidden until the app removes it. `SupportedOrientations` goes to the controllers' `supportedInterfaceOrientations`. On Android a modal over the surface starts below the status bar unless `StatusBarTranslucent`. Other platforms ignore these.
- `EventPressOutside` reports presses outside an overlay, ignoring those on its anchor or on overlays above it.
- An `InputAccessoryView` is a layout root as wide as the surface and as tall as its content, which no view holds: its views are sent before the tree's, and an input's `InputConfig.Accessory` names it, resolved from `InputAccessoryViewID`. iOS makes it the field's `inputAccessoryView`; Android shows it above the keyboard while the field has focus; other platforms leave it unmounted.

## Commit

`Tree.Commit` runs up to eight rounds. `OnLayout` handlers' changes go into the same batch.

1. **Resolve.** Inherit text styles and prepare text whose content or style changed.
2. **Layout.** CSS block, flexbox and grid from `internal/layout` over each view's native children; overlays and modals are layout roots of their own. Results stay cached until a node changes, and frames are rounded to whole points as Taffy does.
3. **Emit.** Compare each view's parts with what was sent: view style, props, events, text, input, scroll. Then sync children against the sibling order last sent, reinserting only the views outside the longest run already in order.

`layout.go` maps styles with React Native's defaults, as NSH does:

- Column direction, no shrinking and no automatic minimum sizes.
- `flex-start` content alignment outside grids.
- Left and right swapped where the subtree lays out right to left.

## Desktop windows

Window chrome follows NSH's `nsh/desktop`: native controls, an app-drawn title bar.

- `SetTitleBar` hides the system title bar and gives the app's bar its height, in which the platform centers its controls; `Env.TitleBar` then reports the area they keep, and views with `WindowDrag` drag the window. macOS centers the traffic lights in the bar, 16 points in from the left edge, as Onevra does; Windows draws Onevra's rounded caption buttons; Linux reports the bar's height and the app draws its own buttons.
- `Env.Maximized` reports a maximized or zoomed window, so an app-drawn maximize button can show restore.
- The platform decides a window drag on the main thread, from the mounted props: the nearest view setting `WindowDrag` decides, and a nearer pressable, input or gesture view keeps the press.
- `SetApplicationMenu` sends the menu as data, with handlers replaced by IDs that come back in `EventMenu`. Role items are the platform's own and are never reported. The default menu returns when the owner that set the menu goes.
- `nsgo.WindowMaterial` (a `Run` option) puts a material behind the app's views: an `NSVisualEffectView` material on macOS, and Mica, Acrylic or Mica Alt on Windows 11 22H2 and later, with the closest backdrop for the other macOS names. It shows where the app draws nothing opaque and lasts for the window's life; `Env.WindowMaterial` reports the one the window has. Earlier Windows and the other platforms leave the window opaque.

## Animation

Style `Transition`s, animated values and layout animations run on the platform, with no engine work per frame and nothing for the engine's garbage collector to pause; [animation](animation.md) covers them. `Scope.RequestAnimationFrame` waits for the display's frames, and `Scope.Animate` steps app values on them for what a platform cannot animate itself; every step commits.

## Services

Native alerts, the clipboard, image sizes and prefetches, announcements, keyboard shortcuts, the status bar, the window title, window commands and the application menu are command items answered by events. Shortcuts are scoped to their owner: the platform matches key presses against the table synchronously and reports only matches.

- The status bar is layered as NSH's `status_bar.rs`: `SetStatusBar` patches the app's default, and each screen's or modal's `StatusBar` is over it, in mount order, while it is in the tree and not popped or swiped away. The latest wins field by field, and the commit sends one `ItemStatusBar` when the merged result changes. iOS and Android apply it; Android ignores `Animated`.
- An input method composing text keeps its keys on macOS and the web, so arrows still choose its candidates.
- Each `KeyBinding` says whether it fires while a text input or web view has the focus and on a held key's auto-repeats, and `driver.FindKey` decides for every Go backend, Android's Java the same way. By default (`KeyAuto`) a binding leaves an input the keys it uses (`EditsText`) and fires once while its key is held; `OnKeyWith` asks for `KeyAlways`, `KeyNotEditing` and `Repeat`. GTK flags no repeats, so Linux counts a key pressed again before its release; UIKit reports a held key once, so iOS repeats a binding on a timer of its own.
- iOS and Android match a hardware keyboard's presses too. `KeyLabel` and `Scope.KeyLabel` name a binding as the platform shows it (⇧⌘L, Ctrl+Shift+L), with `Environment.AppleKeyboard` telling the web on Apple platforms.

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
| `accessibilityActions` as a list in the props | `ViewProps.Actions` points at the element's list, never changed | `ViewProps` stays comparable, so the core diffs it with `!=` |
| Options with defaults of `true` (`closeOnEscape`) | Negative names (`NoCloseOnEscape`) | The zero value must be the default |
| Per-platform image loading and caching | One Go loader (`internal/platform/imageload`): `net/http`, shared fetches, an LRU, `context` cancellation | Written once; platforms only decode and draw |

## TSX entry

Go is the only app language for now. The tree's internal operations are the entry a TSX runtime would drive, as NSH's JS renderer drives its ops: create a node, set its parts, place it among siblings, discard it. No runtime calls them yet.
