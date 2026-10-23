# Interaction

Accessibility, gestures and file drops: what the core sends so platforms decide on the main thread. Read this before changing accessibility props, gestures or drops.

## Accessibility

Elements carry NSH's accessibility props: `Role`, `Label` (`LabelFunc` when it changes), `Hint`, `AccessibilityValue` (a range when `Max` is above `Min`, and `Text`), `Expanded`, `AccessibilityActions` with `OnAccessibilityAction`, `Accessible`, `ImportantForAccessibility`, `AccessibilityElementsHidden` and `LiveRegion`. `Ref.SetAccessibilityFocus` moves the screen reader, as an `ItemAccessibilityFocus` command.

- The props go in `ViewProps`, except a label changing alone on mounted props, which goes as `ItemAccessibilityLabel`.
- An `Accessible` view is one element. Without a label of its own, the core labels it with the text inside it, so every platform reads the same words, and the label follows the text.
- Standard actions (`activate`, `increment`, `decrement`, `longpress`, `expand`, `collapse`, `escape`, `magicTap`) take the screen reader's own gestures; others are listed as custom actions. A disabled view's actions are dropped.
- macOS sets NSAccessibility attributes: AXIncrement, AXDecrement and AXCancel are the standard actions, the others `NSAccessibilityCustomAction`s. iOS sets UIAccessibility properties: the adjustable trait's swipes, `accessibilityPerformEscape` and `accessibilityPerformMagicTap`, and `UIAccessibilityCustomAction`s. Both read an unlabelled element's texts, and announce live regions themselves when a label or a text inside one changes.
- The web maps the props to ARIA. A group hides its children with `aria-hidden`, and a focused view's arrow keys, Escape, Enter and Space perform its standard actions; ARIA has no custom actions, so those are not offered.
- Android sets `importantForAccessibility`, live regions and screen-reader focus on the view, and range info, hint, state description and actions in its `AccessibilityNodeInfo`. It has no escape or magic tap.
- Windows and Linux keep the props but expose none of them, since neither backend has an accessibility provider yet.

## Cursors

`Style.Cursor` sets the pointer's look over a view. `Auto` leaves it to the nearest ancestor that names one, and the web shows a pointer over pressable views. macOS gives each view cursor rects, Windows answers `WM_SETCURSOR` with the view under the pointer, Linux sets GTK's named cursor, and the web sets CSS `cursor`. Touch platforms ignore it; `Split` asks for the horizontal or vertical resize cursor on its divider.

## Gestures

`View.OnPan`, `OnPinch` and `OnRotation` follow gestures the platform recognizes; they report to the engine, which merges each gesture's moves per turn and never its first or last report.

- The platform decides on the main thread when a drag becomes a pan, from the view's `PanActivation`. Until it does, a scroll view around may still take the touch.
- An active pan takes the touch from presses and scroll views.
- A drag or wheel turn scrolls the innermost scroll view under it that can move its content that way, so an inner one passes on only at its edge. A touch or trackpad gesture keeps the scroll view it began with. Android still uses its scroll views' own handling.
- Desktop scroll bars show while the pointer is over their scroll view and can be dragged: AppKit's overlay scrollers on macOS, the browser's on the web, and `platform/internal/scrollbar`'s on Windows and Linux.
- `View.PanValues` binds a drag to animated values: the platform moves them before it reports each move, so the view follows on the main thread, as NSH's bound values do. A drag through `OnPan` and a signal costs a commit per turn.

## File drops

Files dragged from other apps follow NSH's `file-drop.md`. `View.OnFileDrop` receives `file:` URLs; `OnFileDragEnter` and `OnFileDragLeave` follow the drag.

- The destination is the innermost view under the pointer whose mask has `EventFileDrop`, unless it is disabled; the topmost modal bounds the search. The platform decides on the main thread from the mounted views and answers Copy or nothing.
- Leaving one destination for another reports a leave, then an enter. A deleted destination hears nothing more, since its ID may be reused.
- macOS takes `NSPasteboardTypeFileURL` on the surface, Windows registers an OLE `IDropTarget` written in Go while a view listens, and Linux adds a `GtkDropTarget` for `GdkFileList`. Browsers hide paths, so the web delivers each file as a `blob:` URL that `fetch` and `net/http` read, its name as the fragment.
