# Choosing components

Use `ui`'s component whenever one exists: it is `base`'s control with NSGo's look, following the theme. Reach for `base` only for a look of your own, and for `core` for primitives. Check the `ui` and `base` packages in the module directory (`go list -m -f '{{.Dir}}' github.com/nsgo-dev/nsgo`) before building a control, and `examples/gallery` for each one's demo.

| Need | Use |
| --- | --- |
| Box, text, image, icon | `core.View`, `core.Text` with `core.Span` runs, `core.Image`, `core.Icon` with icons from `icons/lucide`; `ui.Textf` for text from a signal in a size, weight and color |
| Buttons | `ui.Button` (`ButtonPrimary`, `ButtonSecondary`, `ButtonOutline`, `ButtonGhost`), `ui.IconButton` with a `Label` for screen readers that also shows as its tooltip (`Tooltip` overrides it), `ui.ToggleButton`, `ui.ToggleGroup`, `ui.HoldToConfirm`, `ui.Link` (`Style.Color` colors its text and hover underline) |
| Text fields | `ui.Input` (controlled with `Value` and `OnChange`, `Multiline`, `Masked`), `ui.NumberInput`, `ui.OtpInput`, `ui.TagInput`, `ui.FormField` for a label and message |
| Choices | `ui.Switch`, `ui.Checkbox`, `ui.Radio` in a `ui.RadioGroup`, `ui.Select[T]` with `ui.Option[T]`, `ui.Combobox`, `ui.Slider`, `ui.Rating` |
| Dates, times and colors | `ui.DatePicker`, `ui.Calendar`, `ui.TimeField`, `ui.ColorPicker` |
| Scrolling and long data | `ui.ScrollView` or `core.Scroll`; `ui.List` or `core.List` for rows built only in view; `core.NewCollection` behind them |
| Tables, trees and charts | `base/datatable` for large tables, including `MultiSelect`, controlled `SelectedRows` and `CellTooltip` (a `ui.Tooltip` inside `RenderCell` never shows), `base.Table` for small ones, `ui.Tree`, `base/plot` for charts |
| Navigation | `base.NewNav` with `ui.NavPage` and `ui.NavHeader` (the system's header on iOS and Android), `ui.Tabs[T]`, `ui.Sidebar`, `ui.SidebarShell`, `ui.TabShell` (a bottom bar at every width; the system's on iOS when every tab has a `SystemIcon`, on Android when every tab has a `SystemIcon` or an SVG `Icon`), `mobile.TabBar` for the system tab bar, `ui.Breadcrumbs`, `ui.Pagination`, `ui.Stepper` |
| Overlays | `ui.Dialog`, `ui.AlertDialog`, `cx.ShowAlert` for the system's alert, `ui.Sheet`, `ui.Popover`, `ui.HoverCard`, `ui.Tooltip`, `ui.Toaster` for toasts |
| Menus and commands | `ui.Menu` with `ui.MenuItem` and `ui.Submenu`, `ui.ContextMenu`, `ui.CommandDialog`, `ui.Toolbar` |
| Disclosure and layout helpers | `ui.Accordion`, `ui.Collapsible`, `ui.Carousel`, `ui.Spacer`, `ui.HStack`, `ui.Wrap`, `ui.Settings` and `ui.FormSection` for settings pages |
| Progress and status | `ui.Progress`, `ui.ProgressRing`, `ui.Spinner`, `ui.Skeleton`, `ui.Meter`, `ui.Badge`, `ui.Avatar`, `ui.AvatarGroup` |
| Documents and editors | `base/textview` for streaming Markdown with formulas (`base/formula`) and Mermaid diagrams (`base/diagram`), `ui/editor` for code, `ui/richeditor` for rich text, `ui/diff`, `base/terminal` |
| Complex views | `ui/chat` for conversations, `ui/flow` for flow diagrams, `ui/commitgraph`, `base/dock` for dockable panels |
| Web content and media | `core.WebView`, `core.Video`, `core.Canvas` for 2D drawing and shaders |
| Platform services | `cx` methods and `core.Env` (clipboard, storage, secrets, links, pickers, speaking, notifications, haptics, permissions, biometrics, Sign in with Apple, in-app purchases through `cx.Purchase`, OAuth sign-in through `cx.AuthSession` rather than a web view, `cx.SetWebCookies` to carry a sign-in into web views, `cx.WebCookies` to read one out and `cx.ClearWebsiteData` to sign out of every site), `desktop` for windows, trays and menus, `mobile` for phones, `widget` for widgets and Live Activities, `updater` for updates |

## Using them well

- Set `tabBarBackground`, `tabBarIdle`, `tabBarSelected` and `tabBarIndicator` as Android `mobile.TabBar` palette roles. A custom palette uses `Accent` for selected content by default.

- For a marker band behind wrapped text, give a `core.Span` a `Background` and `BackgroundInsetTop`, such as `core.Pct(62)` ([text](https://nsgo.dev/docs/text.md#runs-and-links)).
- Set `core.SaveOptions.Start` to an existing absolute folder to start the system save dialog there on macOS, Windows, Linux and iOS. [Files](https://nsgo.dev/docs/services.md#files).
- Set `core.SaveOptions.Accept` to check the destination before a desktop or Android save writes it. A refusal reports the error and destination to the completion callback. [Files](https://nsgo.dev/docs/services.md#files).
- `core.Canvas.Draw` runs again when a signal it reads, its size or the available fonts change; `cx.RegisterFont` updates its glyphs and `Drawing.TextMetrics` once the font loads, including asynchronously on the web ([canvas](https://nsgo.dev/docs/canvas.md)).
- `cx.NewOffscreenImage` creates a retained `core.OffscreenImage` cached across frames until `Invalidate` or `Resize` is called, whose `Source()` draws via `Drawing.DrawImage`, `Drawing.DrawImageIn`, `Drawing.DrawImageFrom` or `core.Image`, and releases automatically on scope disposal.
- Show frames the app decodes or renders (video, emulators, previews) in a `core.Image` with `ImageSource{Pixels: frame, Version: n}`, bumping `Version` after writing each frame into the same `*image.RGBA`, and filter such a frame on the GPU by passing the same source in a `core.Shader`'s `Images`; never encode a PNG per frame ([images](https://nsgo.dev/docs/images.md)).
- Drive a native `core.WebView` through its `core.Ref` (`Reload`, `GoBack`, `GoForward`); app-owned history entries keep their `core.AccessWebSchemes` exemption, while new page-created navigations remain restricted.
- Call `cx.RequestOrientation(core.OrientationLandscape)` in the full-screen view's scope on iOS or Android. Disposal restores the previous policy, and `app.Orientation` records the request in tests. [Interface orientation](https://nsgo.dev/docs/services.md#interface-orientation)
- Set `core.WebView.Mobile` (TSX `mobile`) to show a site's phone layout on Windows, macOS and Linux, with mouse drags and wheel or trackpad swipes reaching the page as touches; phones and the web ignore it ([Components](https://nsgo.dev/docs/components.md)).
- Linux WebView needs WebKitGTK and a working bubblewrap sandbox. If NSGo logs AppArmor's user namespace restriction and shows a placeholder, follow the [Linux troubleshooting steps](https://nsgo.dev/docs/debugging.md#linux-web-views).
- On Linux, `WebView.OnNavigationStateChange` also reports same-document URL, history and title changes.
- A transparent `core.Video` needs HEVC with alpha (`.mov`) on macOS (iOS unchecked) and VP9 with alpha (WebM) on Linux. On macOS, iOS, Windows and Linux a local clip whose alpha cannot be kept reports `OnError` with a `*core.MediaError` whose `Reason` is `core.MediaErrorAlphaUnsupported` (Windows refuses every clip with alpha until Media Foundation's alpha output is checked), so check `Reason`, not the message, to fall back to the app's own decoder; Android, the web and remote URLs play it as the decoder outputs it, possibly opaque ([Components](https://nsgo.dev/docs/components.md)).
- A component's look comes from its `Variant`, a type of its own whose zero value is the default (`ui.ButtonVariant`), and a few take `Size` (`ui.SizeDefault`, small or large). Do not fake a variant with styles over the default.
- For validation in `base` controls, pass `Invalid: rejected.Get`; style it with `aria-invalid:` classes or `Style.AriaInvalid`. Custom controls pass it to their actual input or root and use `core.PartStates.Invalid` on decorative parts to avoid duplicate accessibility state.
- `ui.Button` uses larger touch targets on phones and tablets; use `Style: core.Style{Width: core.Pct(100)}` for a full-width action or set `Height` for another height. `ui.OtpInput.Size` sets both cell dimensions in points, and `CellWidth` and `CellHeight` override either one. Use `TextStyle`, `CornerRadius` and `CaretHeight` for the cell and caret. `ui.FormRow` and `ui.FormField` have a 48-point touch minimum.
- Controlled controls take a getter and a setter: `Value: name.Get, OnChange: name.Set`, `Checked: on.Get`, `Open: dialog.Get, OnOpenChange: ...`.
- `ui.Button` offers `ButtonTinted` and `ButtonDanger`. Set `TextStyle` for label typography and `BusyLabel` or `BusyLabelFunc` for a message beside the busy spinner.
- For Shift-press range selection, `base.Checkbox` and `ui.Checkbox` take `OnChangeEvent func(checked bool, press core.PressEvent)` in place of `OnChange`; TSX receives `{ checked, press }`. The app still owns the value ([Checkbox](https://nsgo.dev/docs/ui/checkbox.md)).
- Use `ui.Slider.MinFunc`, `MaxFunc` and `StepFunc` for changing ranges and steps; `base.Slider.Min`, `Max` and `Step` already take getters. This keeps focus and an active drag mounted.
- `ui.Dialog` and `ui.AlertDialog` take `Style` and `StyleFunc` for their surface; use `StyleFunc` for colors that follow the theme or other signals, merged after `Style` on both centered dialogs and sheets.
- Style the `ui.FormSection` surface with `CardStyle` and `CardStyleFunc`. Set `FormRow.LeadingWidth` for a custom `Leading` node so its divider aligns with the title, or change the icon slot with `TileSize`. Style text with `TitleStyle` and `DetailStyle`.
- Style every toast in a stack with `ui.Toasts.Style` and `StyleFunc`, or one with `ui.ToastContent.Style` and `StyleFunc`. Per-toast styles override the stack, and each `StyleFunc` follows its signals.
- Give each control a label screen readers hear (`Label`, or the button's text); icon-only buttons need one. An input's `Placeholder` is a separate hint, exposed on macOS for single-line, masked and multiline fields; keep its `Label` meaningful when that hint changes or clears.
- `ui.NavPage` and `ui.NavHeader` draw the system's own header and back gesture on phones, and `NavHeader` with its back button elsewhere, an Apple Watch included; do not build a header of views.
- `ui.Menu` and `ui.Select` open beside their trigger on any device, or as a sheet when their `Presentation` asks; choose it explicitly instead of switching by device.
- `core.Ref` focus, blur, scroll and zoom requests are dropped while the element is in unplaced `core.Holder` content, or when it leaves the tree before the commit; showing the content again does not replay them, so ask again once it is placed.
- Web canvases in a zoomed `core.Scroll` replay at the cumulative scroll zoom once it rests, up to four times the screen's pixel ratio, without rerunning `Draw` ([canvas](https://nsgo.dev/docs/canvas.md)).
