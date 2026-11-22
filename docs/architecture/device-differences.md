# Device differences

Every place a component in `base` or `ui` changes on its own by device, without the app asking. Read this before branching a component on `Env.Desktop`, `Env.Idiom`, `Env.OS` or a width, and add the branch here.

## Rules

- Keep a control's look the same on every device, and its size but for the touch targets listed under [touch input](#touch-input): `ui`'s buttons, code cells and form rows, and a resize handle's 3-point band, which a finger cannot take. `core` and `base` never change a size by device.
- Let the app ask for a different size or form through a field or style.
- Branch only for what a device cannot do, such as hover without a pointer, for the touch sizes above, or for a form listed under [presentation](#presentation).
- Never branch by device in `core` or `base` ([principles](../../AGENTS.md#principles)); the branches of `base` below follow what an input can do, such as hover or a held finger.
- Use `ui.Touch` for a branch on input. Use `Env.Idiom` or `Env.OS` only when touch is not the reason. A TV takes no touches: its remote moves a focus, so `ui.Touch` is false there and `ui` presents as on a desktop.

Other sizes are constants: `ui.ButtonHeight` is a desktop button's 28 points, [`Button` and `Tabs`](themes.md#variants-and-sizes) taking another size when an app asks for one, and `ui.RowHeight` 36 on every device. `ui.Input`, `ui.Select` and the other fields stay 28 points tall on phones, so a field beside a button is shorter than the button there unless the app sizes one of them; `Button.Style` and `OtpInput`’s `Size`, `CellWidth` and `CellHeight` take the app’s dimensions on any device.

## Presentation

These swap one form for another on their own:

| Component | Desktop | Touch screen | Decided by |
| --- | --- | --- | --- |
| `ui.Dialog` | Centered dialog, footer in a row | Bottom sheet, footer stacked | `ui.Touch` |
| `ui.NumberInput` | Stacked chevrons, 22 points wide | A minus and a plus, each 28 points wide | `ui.Touch` |
| `ui/commitgraph.View` | Refs, subject, author, hash and date | Refs, subject and author's avatar | Width under 560 points |

Menus, context menus and dropdowns (`base` and `ui`'s `Menu`, `ContextMenu`, `Select`, `Combobox` and `DatePicker`) are popups unless the app sets `Presentation` to `PresentSheet`, and `dock.Area` keeps its panels docked unless the app makes it `Compact`: neither `base` nor `ui` chooses those by device (decided by the user, 2026-11-12). `ui.MenuItem` rows follow the presentation: 26 points in a popup and 44 across a sheet, which also hides `Shortcut`.

## Hover and pointer

Touch screens have no pointer, so these exist on desktops alone.

| Component | Desktop only |
| --- | --- |
| `base.Tooltip`, `View.Tooltip` | Opening on hover; the app can still open a controlled tooltip on touch screens |
| `ui.Rating` | The hovered star preview |
| `ui.AvatarGroup` | The fan on hover and each member's tooltip |
| `ui.MenuItem` | Row hover in a popup |
| `base/diff` | A drag over the line numbers selects lines |
| `base/datatable` | Its own scroll bars; touch screens show the system's indicator |

`ui.HandleThumb` covers the same gap the other way. Phones have no resize cursor, so a handle shows a pill while pressed.

## Touch input

| Component | Desktop | Touch screen |
| --- | --- | --- |
| `base.ResizeHandle`, `base.ResizablePanelGroup` | 3-point band | 10 more points either side, since 3 points cannot take a finger |
| `ui.Button`, and `ui.Menu` and `ui.Popover` triggers drawn by one | 24, 28 and 32 points tall for `SizeSmall`, `SizeDefault` and `SizeLarge` | 40, 44 and 48 points, the platforms' touch targets |
| `ui.OtpInput` | 36-point cells | 44-point cells |
| `ui.FormRow`, `ui.FormField` and `ui.Settings` rows | At least 40 points tall | At least 48 points tall |
| `base.Popover` | A press outside closes and goes on | A transparent backdrop takes the press |
| `base/dock` tab drag | Starts after moving | Starts after a long press |
| `base/editor` | Press places the caret, drag selects | Tap, long press and selection handles |
| `base/terminal` | No key bar | A bar of keys over the software keyboard, unless `NoKeyBar` is set |

## Keyboard and focus

| Component | Desktop | Touch screen |
| --- | --- | --- |
| `base.List` | One Tab stop with a focus ring; a double press activates | No focus stop; a tap activates, or toggles in `Multiple` |
| `ui.Slider`, `ui.Rating` | A press focuses and rings the control | No focus |
| `ui/chat` effort picker | Takes focus as it opens | Leaves the focus alone |
| `ui/chat` composer | Return submits | Return adds a line |

## System look

| Component | Differs on | What differs |
| --- | --- | --- |
| `ui.TabShell` | iOS | Floats over the content when it uses the system's tab bar |
| `ui.TabShell` | iOS, Android | The system's tab bar: on iOS when every tab has a `SystemIcon`, on Android when every tab has a `SystemIcon` or an `Icon`, whose SVG shows where `SystemIcon` names no drawable. watchOS, which reports iOS, gets the drawn bar |
| `ui.NavPage` | iOS, Android | iOS: the system's transparent header over the page, with its own glass, title and back button; content scrolls under it with a progressive blur. Android: Material 3's transparent 64 dp toolbar over the page, with a 22 sp title, back arrow and the header's views below the status bar, and a 152 dp large title that collapses as the scroll view at the top of the page scrolls under it with a progressive blur (API 31+ `RenderEffect`, translucent scrim before API 31); explicit custom header backgrounds override transparency. A page's own `Header` becomes the title view on both, filling the room between its ends when its views grow or take a percentage width and taking its content's size otherwise, keeping the page's left and right views; elsewhere, including watchOS, it is drawn above the page at the page's width |
| `ui/chat` glass surfaces | macOS, iOS, Windows | No drawn rim, as the system's glass draws its own |
| `ui/filepicker` | macOS | Shortcuts use Command instead of Control |
| `core.Input`'s and `base.Command`'s `InputSource` | macOS, Windows, Linux | Selects the keyboard input source; nothing elsewhere |
