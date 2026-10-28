# Roadmap

This plan lists what NSGo still lacks compared with NSH's 0.3.0 release (tag `v0.3.0`, `76fa8a7f`), in the order to build it. Each item names its NSH reference and the result that finishes it. What exists today is in [ARCHITECTURE.md](../ARCHITECTURE.md); design questions that block an item are in its [Open questions](../ARCHITECTURE.md#open-questions).

References are files in `nsh/docs/dev/` unless they give a path from `nsh/`, with the bullet or section to read in brackets. To refresh the plan for NSH's next release, check out its tag in `nsh/`, then read `git log v0.3.0..HEAD` there and the documents it changed, and compare the declarations any `nsh` command run on its gallery writes to `nsh/examples/gallery/.nsh/types/` with `go doc -all` of the public packages.

## Text, scrolling and navigation

| Item | NSH reference | Done when |
| --- | --- | --- |
| Font variants | `styles.md` (`fontVariant`), `canvas.md` (`fontVariantNumeric`) | Text, inputs and a drawing's text take small caps and tabular, proportional, lining or oldstyle figures on all six backends; the gallery's Styles page has NSH's Numerals card, and its Stocks and Charts pages set their figures tabular |
| Decoration lines | `styles.md` (text) | An underline or strike takes a thickness and an underline an offset from the text |
| Scroll event throttle | `scrolling-and-lists.md` (`scrollEventThrottle`) | A scroll view or list reports scrolls at most once per interval the app sets |
| iOS header buttons | `website/docs/navigation.md` (`headerRightSystemItem`) | `Screen.IOS` puts a UIKit system bar button (add, done, close, edit, compose, search, share, trash, save, cancel) at the header's end, pressing `HeaderRight`'s first pressable view |
| Leaving the app | `js-modules.md` (`BackHandler`) | `mobile` can end the app on Android, as `BackHandler.exitApp` does |

## Components

| Item | NSH reference | Done when |
| --- | --- | --- |
| TextView presses | `text-view.md`, `website/elements/text-view.md` (`onImagePress`, `links`) | `TextView` reports a press on an image outside a link, and can open links that leave the document itself; the gallery's TextView page opens NSH's image viewer, which zooms, pans and swipes closed |
| Editor's ref | `editor.md`, `website/elements/editor.md` (ref, `onTextPress`) | The ref applies several edits as one, folds by level and recursively, converts offsets and positions, and reports a range's boxes, the scroll offset and the visible range; `OnTextPress` reports a pressed character; a diagnostic links its code's description and related places |
| DiffView's ref | `diff.md`, `website/elements/diff-view.md` | The app draws file headers and separators; the ref collapses and expands one file, shows some or all of the lines a separator hides, and reports the current change |
| DockArea's ref | `dock.md`, `website/elements/dock.md` (ref) | The ref removes, moves and selects panels, splits a group, selects a tab, sizes a dock, and reports the zoomed group and a panel's group |
| Resize handles | `website/elements/resizable.md` (`renderHandle`) | `ResizablePanelGroup` shows the app's content in each handle, told its index, axis and state, drawn again only when the state changes |
| Editor demo | `examples/gallery/demos/EditorDemo.tsx`, `examples/gallery/demos/lsp/` | The gallery's Editor page is NSH's: a `DockArea` with a problems dock around the editor, and a simulated language server whose hovers show in a `TextView` card, whose code actions open a menu at the caret and whose document colors are decorations |

## Embedding and the tool

| Item | NSH reference | Done when |
| --- | --- | --- |
| Web host channel | `web.md` (the CLI's web target), `website/docs/web.md` (the host's functions) | A page answers an app's host calls and posts and sends it events, through the loader `nsgo.json` feeds and a host module `nsgo.yaml` names, which the web package copies |
| Commit traces | `website/docs/dev-server-and-debugging.md` (Logs), `performance.md` | `nsgo dev -trace` prints each commit's time and mount items from the running app on every target |

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
