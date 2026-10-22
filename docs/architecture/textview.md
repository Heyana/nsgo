# Text view

`base/textview` is NSH's `TextView` (`nsh/docs/dev/text-view.md`): a document of Markdown, HTML or the app's own blocks, shown with `Text`, `View`, `Scroll` and `Image`. Read this before changing the document model, the parser, the parts or highlighting.

## Layers

| Package | What |
| --- | --- |
| `base/textview` | `Document` and its blocks and inlines, `Parse`, `ParseHTML`, `Extension`, and the unstyled `TextView`, styled by `Part` |
| `ui` | `ui.TextView`: NSH's reading typography in the theme's colors, and a copy button on code blocks |
| `ui/highlight` | A `Highlighter` from chroma's lexers in NSH's light and dark palette |

Highlighting is its own package because chroma's lexers add about 4.5 MB to a binary, against about 0.3 MB for goldmark. Apps opt in with `Highlighter: highlight.New(cx)`.

## Parsing

`Parse` reads Markdown with goldmark's CommonMark parser and its GFM and footnote extensions, then turns goldmark's tree into `Document` values. The model is plain Go values, so apps build documents themselves and compare them with `reflect.DeepEqual`.

- Alerts are blockquotes whose first line is `[!NOTE]`, `[!TIP]`, `[!IMPORTANT]`, `[!WARNING]` or `[!CAUTION]`.
- Heading anchors are GitHub's slugs, made unique per document.
- HTML inside reads as `ParseHTML` reads it.
- An HTML block that opens an element a later block closes (`<details>`, Markdown, `</details>`) holds the Markdown between them. So does a formatting tag in a paragraph (`<b>x *y*</b>`), as gpui-kit pairs them.
- Other paired tags read as one HTML fragment from the source between them.
- `RepairStreaming`, which `Streaming` applies before parsing, ports NSH's streaming repair.
- It closes what the last block leaves open (emphasis, strikethrough, inline code) and shows unfinished links as their text.
- It drops unfinished images, footnote references and HTML tags, and completes a table's delimiter row. It omits NSH's `$` and `\(` formula repair until formulas parse.
- An `Extension` makes `Custom` nodes from a regular expression in text (`Pattern`), a fenced code language (`Language`), a `:::name` container (`Container`) or an HTML element (`Tag`).
- Containers are a goldmark block parser, so their content parses as the document does. A container inside another closes with fewer colons.

## HTML

`ParseHTML` ports NSH's `html/`, with `golang.org/x/net/html` in place of html5ever: both build the tree a browser builds, so unclosed and misplaced tags read as a browser shows them. The reader walks the body in block or inline context; runs of inline content between blocks become paragraphs, and blocks met inside inline content are lines of their own. Whitespace collapses as a browser collapses it, except in `<pre>`.

- What a reader does not show is skipped: metadata, scripts, styles, form controls, embedded media, and elements `hidden`, `aria-hidden="true"` or styled `display: none`, such as KaTeX's HTML beside its MathML.
- GitHub's own markup reads back as what it writes: alert `<div>`s, task list boxes, `language-*` classes of code, and `<math>` with a TeX annotation as `Math`.
- `<details>` is `Details`; `<u>`, `<mark>`, `<sup>`, `<sub>`, `<small>` and `<kbd>` have inline nodes of their own. A `<mark>` keeps the color its `color` attribute or `background-color` asks for.
- Depth is bounded at 256 and `colspan` at 64, since the input is untrusted.

## Rendering

- `TextView` parses in one memo, then shows the blocks as rows of an `Each` keyed by position and kind. Each row keeps its block in a memo compared deeply, so a document that grows at its end rebuilds only its last block.
- A row is a `Dynamic`: the part styles and the highlighter it reads, such as the theme's appearance, rebuild it.
- Paragraphs, headings, cells and code are each one `Text` with spans; links are spans with a press handler that calls `OnLinkPress`.
- `Text` has no inline attachments yet, so images show as blocks when a paragraph holds only images, and as their description inside text.
- `Selectable` makes the root a selection area, so one selection runs across the blocks.
- A disclosure's summary toggles it on press; its blocks are built each time it opens.
- `Math` shows its TeX in `PartMath` or `PartMathBlock` until formulas render.
- Tables are one CSS grid in a horizontal `Scroll`. Each column is `minmax(floor, auto)`, the floor estimated from its text as NSH does. The grid is the scroll view's width and at least the floors' sum, so narrow views wrap cells, then scroll.

## Parts

A node with several parts merges `StyleFunc`'s styles for each, then the app's `Styles` for each, later parts winning. `ui.TextView` passes its typography as `StyleFunc`, so the app's `Styles` override it.

Where NSGo departs from NSH:

| NSH | NSGo | Why |
| --- | --- | --- |
| `classes` and `styles` props by part | `Styles` map and `StyleFunc` by `Part` | Styles are Go values |
| Typography in `nsh/base`'s `TextView` | Typography in `ui.TextView` | `base` has no look |
| Alert icon | Alert title text, and `PartAlertContent` around its blocks | The title takes the alert's color while the blocks keep the text's |
| `renderCustom` for inline nodes | `CustomSpan` returns a span | Text has no inline attachments |
| syntect and its feature flag | chroma in `ui/highlight` | Pure Go; linked only when used |

## Limits

These are goals in the [roadmap](../roadmap.md): formulas, inline attachments, span backgrounds and strikethrough on Apple platforms. Until spans draw backgrounds, marks and keys show no tint. Superscripts and subscripts are smaller but sit on the baseline, since spans have no baseline offset. A disclosure's summary reports no expanded state to assistive technology. Anchors are reported, not scrolled to. Highlighting runs on the engine goroutine, with the last 64 blocks' tokens cached for the process.
