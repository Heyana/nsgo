# Scrolling and lists

How scroll views behave and report, and how `List` windows, measures, anchors and pins its rows. Read this before changing `Scroll`, `List` or a backend's scroll view.

## Scrolling

A scroll view follows NSH's `scrolling-and-lists.md`: `ScrollBehavior` holds paging, snapping, deceleration, bounces, the content inset and the keyboard props of `Scroll` and `List`, and `ScrollConfig` carries it with refreshing and zooming in `ItemScrollConfig`.

- Offsets are UIKit's `contentOffset`: a `ContentInset` makes them negative at the start. `ContentOffset` is an `ItemScrollTo` sent after the first commit.
- `ScrollBehavior.SnapTarget` decides where a release snaps, as NSH's `snap_target`; iOS and macOS call it, Windows and Linux through `internal/platform/scrollbar`'s `Settle`, Android's `Scrolling.java` ports it, and the web marks `SnapPoints` for CSS scroll snapping.
- Every scroll view reports its offset as it moves, so the core's hit tests and touches find what shows under a point; a scroll lays out again only while an overlay is placed beside an anchor. Platforms report drags, momentum and refreshes with the offset and the zoom scale (`Event.Value`); the core adds the content and viewport sizes for `ScrollEvent`. After `EventRefresh` the config goes out again, so a spinner the app did not keep stops.
- iOS uses UIScrollView's own props and UIRefreshControl, macOS NSScrollView's elasticity and insets with a spinner of its own, Android its scroll views with its own glides and pull, and the web CSS. Windows and Linux scroll with the wheel and the bar only, so they settle snapping views when the wheel rests and leave bounces, deceleration, refreshing and the keyboard props alone; macOS ignores deceleration (the system glides) and the keyboard props.
- Zooming (`MaxZoomScale` above `MinZoomScale`, `Ref.ZoomTo`) is iOS's zoom view, Android's `ZoomScroll` and a scaled content element on the web; the desktops ignore it, as NSH's do. Mouse drags never scroll content.

## Lists

`List` follows NSH's `<List>` and its `ListState` (`liststate.go`). It mounts only the rows within a viewport of the visible ones, in absolutely placed cells it reuses: a cell's content is built once, and its `ListRow` signals move it to another row. Item sizes start as an estimate and are corrected as cells are laid out; rows (`NumColumns` items, as long as the longest) sit in a Fenwick tree, so offsets and the row at an offset cost O(log n). The window is refilled after each layout, within the same commit.

- With `Key`, a change of the keys is a splice by their common start and end: kept rows keep their sizes and cells, and a splice or a measurement wholly above the viewport keeps the visible rows in place with an `ItemScrollBy`. Platforms move the offset by it unclamped and carry a drag, glide or animated scroll along.
- `Inverted` lays the content out from the end and keeps its distance from it; `MaintainVisibleContentPosition` follows the end of other lists within its threshold. The header, footer, empty view and separators (in the row's cell) are placed as rows are.
- Platforms count the `ItemScrollTo` and `ItemScrollBy` items each scroll view applied and report the count in `Event.Count`; the core replays the scrolls a report predates, as NSH's `ScrollState::issued` does.
- `Ref.ScrollToIndexAt`, `ScrollToEnd` and `InitialScrollIndex` resolve with the sizes known and pin their target: as the rows around it are measured the list follows it (an `ItemScrollBy`, or a new animated `ItemScrollTo`) until the rows in the viewport are measured or the user scrolls away.
- Sticky rows (one column, not inverted) are computed in the core and sent as `ItemStickyCells`: each cell's travel in offsets, which macOS, iOS, Windows, Linux and Android apply on every scroll, from its frame. The web puts each cell in a track spanning its travel and lets CSS `position: sticky` pin it, clipping views with `overflow: clip` where the browser has it. Sticky cells move to the end of their list's children, so they draw above the others.
- `OnViewableItemsChange` reports rows by `ViewabilityConfig`'s thresholds, after `MinimumViewTime` on the graph's clock.
- With `ListViewportParent` the list is a plain clipping view whose spacer gives it its full length in its parent's layout. It is virtualized against the nearest scroll view on its axis around it (else its root, modal, overlay or screen): its offset is that view's less where the list starts in its content. Its anchoring and scroll commands scroll that view, and its sticky rows go in that view's `ItemStickyCells`, the last stopping at the list's end. The scroll view props, `Inverted` and `MaintainVisibleContentPosition` are ignored there; NSH warns of them, NSGo does not yet. Cells bind synchronously, so NSH's paced binding has no counterpart.
