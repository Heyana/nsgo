# Text

Text in NSGo: how the core prepares, lays out and reports text, how fields edit it, and how selections run across texts. Read this before changing text styles, a `TextEngine`, `Input` or selection.

## Text and drawing

A platform's `TextEngine` runs on the engine goroutine in three phases. `Prepare` shapes the text once, `PreparedText.Measure` answers layout at any width, and `PreparedText.Layout` lays it out at its final size. The `TextLayout` travels in `ItemText`, and the main thread only draws it and releases the one it replaces. The core releases prepared text when it changes or its node goes. Apple's engine caches fonts and style attributes across prepares, and a prepared text keeps its measured size per width; the process-wide caches are bounded and internal.

The core does what needs no engine, as NSH's does: `TextTransform` changes the case of each span's characters as they are joined, a text's own multiplier caps the user's text size (`NoFontScaling`, `MaxFontSizeMultiplier`), and `TextAlignVertical` moves the content box down to the text's measured height. `AdjustsFontSizeToFit` bisects a scale, down to `MinimumFontScale`, at which the untruncated text fits the box and its line limit, as NSH's `Paragraph::fit`, and prepares that text for the final layout only; layout measures the text as written. `Text.OnTextLayout` runs after the commit with the lines a `LineLayout` reports: UTF-8 ranges, boxes in the content box, and the metrics of the font each line starts in.

The engines draw the rest:

- `EllipsizeMode`. CoreText truncates the last line's rest at its head, middle or tail; Pango ellipsizes the same way; Android passes `TruncateAt`, which ellipsizes a head or middle on one line only. DirectWrite and the web trim only tails, so on one line without attachments they search with `AttributedText.CutToFit` for the most characters around an ellipsis. Clipping keeps the lines whole and cuts them at the box.
- `TextAlignJustify` is each engine's justification; Android's needs no newer API than NSGo's minimum.
- `TextDecorationStyle` and `TextDecorationColor`. CoreText underlines solid, double, dotted and dashed in a color, and the backend draws wavy underlines and every strikethrough itself; Windows and Android draw the lines a font or paint cannot; Pango has no dotted, dashed or styled strikethrough lines, so those are solid there.
- `AttributedText.Shadow`, the block's shadow: a Core Graphics shadow, a CSS `text-shadow`, Direct2D's shadow effect over the recorded glyphs, a GSK shadow node, Android's shadow layer.

The family `FontMonospace` is each platform's fixed-pitch font: CoreText's user fixed-pitch font, CSS's `ui-monospace` stack, Consolas on Windows, Pango's `monospace` on Linux, Android's `monospace`.

`Text.Spans` style byte ranges over the text's style; offsets are UTF-8 bytes, Go's own, and platforms convert them. A press on a span with a handler comes back with the byte offset under the point.

- A span's `Background` and `CornerRadius` become its `TextBackground`: drawing only, a box per line from the font's ascent to its descent, rounded on every line, behind the selection and the glyphs. Hanging spaces at a line's end are not painted.
- A span's `Attachment` is a view inside the text, as NSH's inline attachments are. Its view is a layout root at its content size, laid out before the text is prepared, so a change inside it prepares the text again.
- The text holds an `AttachmentRune` in an attachment's place, and `AttributedText.Attachments` gives the engine its size and `VerticalAlign`.
- Every engine places an attachment by NSH's rule, `TextAttachment.Ascent` and `Extent` against the placeholder font's `FontExtent`, so a line grows where an attachment reaches past its font.
- After layout the core asks the `AttachmentLayout` where each attachment went and sends its view as a native child of the text view at that frame. Those cut off by the line limit are hidden.

A `Canvas` records its drawing on the engine into an immutable `Drawing`, again when a signal it read or its size changes, and the platform plays the operations on the main thread.

## Text inputs

`Input` follows NSH's `<Input>`: one line, or several with `Multiline` in each platform's multi-line control (NSTextView, UITextView, `<textarea>`, GtkTextView, a multi-line EditText, and Windows' drawn editor wrapping its lines).

- A multiline input is measured with its value, or its placeholder while empty, wrapped at its width, so it grows unless its style sizes it. `OnContentSizeChange` is the core's: after a layout that changed the value's size, one empty line's for an empty input.
- `InputConfig.Submit` resolves `SubmitBehavior` with `Multiline` as NSH's `InputConfig::submit()`, and backends read only it: single lines blur and submit by default, multiline inputs break the line. Shift, Option or Alt with Return always break a multiline input's line; a Return that commits a composition never submits.
- The platform answers `MaxLength` and Return natively, without the engine. `platform/internal/field` keeps what every Go backend reports the same way: edit counts, selections, keys, compositions and a `Value` held while an input method composes until the composition ends. Android's `InputField` does the same in Java.
- Selections are UTF-16 offsets. `SelectedRange` and `Ref.SetSelection` send `ItemSelection` after the batch's value.
- `PlaceholderColor` and `SelectionColor` are inherited text styles; the caret takes the selection color, and `HideCaret` hides it.
- What a platform lacks is a no-op there: return keys, capitalization and the keyboard's appearance belong to software keyboards (iOS, Android, the web's `enterkeyhint` and `autocapitalize`; Linux passes GTK input hints); `AutoComplete` maps to iOS's `textContentType`, Android's autofill hints, the web's `autocomplete`, GTK's input purpose and macOS's content types for accounts, passwords and codes; Windows has no autofill or autocorrection. `KeyboardAppearance` is iOS's only.

## Text selection

Selection follows NSH's: one selection runs across the texts of a selection area (a `View` with `SelectionArea`, which `base.SelectionArea` sets), in document order, as a web page's does. A `Text` with `SelectableOn` outside any area is an area of its own; one with `SelectableOff` stays out. The highlight is the inherited `SelectionColor`, else the platform's.

- `ViewProps` carries `SelectionArea`, `Selectable` and `SelectionColor`, so a selection never waits for the engine.
- `SelectionModel` in `selection.go` is the platform-independent model macOS and iOS drive on their main thread. Windows does not select text yet.
- A backend lists an area's texts as `SelectionTexts`, with content, frames, hit test and NodeIDs, then feeds it presses, drags and long presses and draws its `Ranges`.
- Offsets are UTF-16. `Text` joins texts on one row with a tab and others with a line break, leaving attachments out.
- macOS takes mouse presses in its event monitor before views see them; pressables, fields and gestures inside an area keep theirs. The surface becomes first responder for Copy, Select All and Escape.
- iOS selects with a long press, then by words as the finger drags, and shows the edit menu; the view controller answers Copy and Select All. Touch handles are not drawn yet.
- The web uses the browser's own selection through `user-select`, and a `copy` handler joins texts as the model does.
