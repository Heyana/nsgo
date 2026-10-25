# Components

`base` holds NSGo's built-in unstyled components, after NSH's `nsh-base` (`nsh/docs/dev/components.md`, `dock.md`): they own behavior, state and accessibility and take every style from the app. Read this before changing a component in `base`; [text view](textview.md) covers `base/textview`.

## Controls

`Button`, `Checkbox`, `Radio` in a `RadioGroup`, `Switch`, `Tab` in `Tabs`, `Collapsible` and `Progress` hold no value: the app passes it and sets it from `OnChange` or `OnPress`, as NSH's controlled props do.

- A control's parts (a checkbox's indicator, a switch's track and thumb) follow its states through `View.PartStates`, which styles a view by states it does not report, so assistive technology hears them from the control alone. Content the app puts inside follows them with a `GroupStyle` under the control's `Style.Group`.
- A radio reports checked and selected both, as `gpui-base`'s does, since screen readers read one or the other.
- `Collapsible` makes its content the first time it opens and keeps it, hidden, while closed.
- `Progress` is one accessible element with a range from 0 to 100, and none while indeterminate.

## Popups

Popovers, tooltips, menus, context menus, dialogs and sheets share NSH's contract (`popup.go`): `Open` makes them controlled, `OnOpenChange` reports each request with its `Reason`, `OnCancel` every attempt to close, and `OnOpen` and `OnClose` follow what is shown, after a sheet's motion.

They present themselves by `Env.Desktop`, as NSH's do:

| | Desktops | Touch screens |
| --- | --- | --- |
| `Popover` | A press outside closes it and goes on to what is under it | A transparent backdrop overlay takes the press outside |
| `Menu` | A popup at its trigger; submenus cascade beside their item, on hover or press | A sheet at the bottom over `BackdropStyle`; a submenu's items replace its menu's, below its `Back` row, and Escape or back return a level |
| `ContextMenu` | Opens at the pointer (`AnchorBounds` at the click) | Opens as the menu's sheet after a finger rests 400 ms, watched with `OnTouch` so the trigger's own presses and scroll views go on; 10 points of movement cancel |
| `Tooltip` | After the pointer rests half a second | Only when the app opens it |

Items reach their menu through a context key (`menuContext`), so the app arranges them freely. A sheet's hidden levels are out of layout and their content out of the tree, so screen readers and tests see one level.

- `Sheet` and a phone's menus are overlays, not modals: `sheetMotion` (`sheet.go`) ports NSH's `SheetMotion`. The panel's translation and the backdrop's opacity are animated nodes of one `fade` value and the panel's measured size, so they run on the platform's frame clock; the parts leave the tree when the motion out ends, or after its time where no end is reported.
- With `CloseOnSwipe` a bottom sheet's `PanValues` move a drag value; a release past 30% of its height or 800 points a second asks to close (`ReasonSwipe`), else it comes back. iOS's handoff from a scroll view inside the panel is not ported.
- `View.Tooltip` is the short form: the core shows the node above the view, 4 points away, after half a second of hover on desktops. Tooltips never take the pointer (`Overlay.PointerEvents`).
- `Dialog` stays a `core.Modal` the platform presents; `AlertDialog` is the system's alert, or with `AlertPresentActionSheet` its action sheet: `UIAlertController`'s (a popover anchored at the bottom on iPad, with an unshown cancel action for a tap outside), a list dialog on Android, stacked buttons with the cancel one last on macOS, the web and Windows (command links).

## Resizable panels

`ResizablePanelGroup` ports `gpui-base`'s sizing (`resizeState`, tested as NSH tests it): a drag grows the panel before its handle by taking from the panels after it, nearest first, as far as their minimums allow, and shrinking below a minimum takes from the panels before.

- Panels are a slice, not children registering through a context as NSH's do: Go lists them statically, and `Visible` hides one. `Axis` is NSH's: `Horizontal` side by side, `Vertical` stacked.
- Each panel's flex basis, grow, shrink and limits come from the size it wants: what a drag gave it, else `Size`. Unsized panels share the rest with a basis of 0; when every panel has a size they grow in proportion. Minimums shrink by one ratio when the group is too short for them.
- A handle is a 9-point band between two panels (`handleBandNode`) whose negative margins give its thickness back, so it takes no room yet lies over the boundary inside the group, above its neighbours: macOS gives a view its cursor and tracks the pointer only within what its parents show. Touch screens add a 10-point slop each side. `ResizeHandle` is the same band on a container's edge, its line on the edge.
- Drags resize on a state made from the panels' laid out sizes as they start, and only their end, or a screen reader's 10-point step, reports `OnResize`.

## Dock

`DockArea` ports NSH's dock in layers: the layout data (`dock_layout.go`: `DockNode` trees, normalization to a fixpoint, edits, the app's layouts installed with their IDs repaired), `gpui-base`'s persisted format (`dock_state.go`, `DockLayout`'s JSON) and the component (`dock.go`).

- Container IDs come from a counter per area above the largest ID its layout holds, not one per process.
- Each container renders from a memo of its shape (kind and children's IDs), so selecting a tab or resizing rebuilds nothing; a split whose children change is rendered again.
- A tab drag claims the touches with the responder (`OnMove`, after 4 points of a pointer or a long press of a finger), measures the groups' tab bars and content frames with `Ref.Bounds`, and resolves the target as NSH's: a slot between tabs, a content frame's middle to merge, its outer 35% to split. The indicator and the drag preview are overlays that never take the pointer.
- Zoom and the compact presentation narrow the tree to one group by hiding the containers off its path, so the panels keep their content.
- Below 600 points the docks are `sheetMotion` drawers and a bottom sheet, presented one at a time, without changing the layout's `Closed`.

| NSH | NSGo | Why |
| --- | --- | --- |
| Render keys move a panel's content to its new placeholder | A group renders its own panels' content | NSGo's core cannot move a mounted subtree; a panel moved to another group, or a dock's panel as the presentation switches, is rendered anew |
| `renderHandle` and handle thumbs by class | `HandleStyle` and `HandleThumbStyle`, with `GroupStyle` for hover and press | Styles are Go values |
| `DockAreaApi` through a ref | `DockAreaRef`, bound as the area mounts | Go has no refs to components |
