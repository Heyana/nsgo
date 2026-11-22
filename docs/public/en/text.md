---
title: Text
description: Show text with spans, fonts, decorations, selection and documents.
group: Core concepts
order: 53
---

## A text

`core.Text` is one paragraph, typeset by the platform's own text engine on the engine side and drawn by the main thread. Its size comes from its font and its width, and it wraps inside its box.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` is fixed and `ContentFunc` follows a signal, so a text that changes reads it inside the function. `MaxLines` limits the lines shown, `EllipsizeMode` cuts the rest at its head, middle or tail, and `WhiteSpaceNoWrap` keeps a paragraph on one line. `AdjustsFontSizeToFit` shrinks the text until it fits its box and its line limit.

Text fields inherit as CSS does: a text inside a view with `FontSize`, `Color` or `LineHeight` takes them unless it sets its own. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor` and `TextShadowColor`, `TextShadowOffset` and `TextShadowRadius` style the lines and the glyphs.

## Runs and links

`Spans` makes a text of runs, each with a style of its own. A span with `OnPress` is a link, and a press on another span is the text's own `OnPress`.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` gives spans again when what it reads changes, and only the parts that differ are prepared again, so a code editor's rows recolor as highlighting arrives. A span's `Background` and `CornerRadius` draw a box behind its characters on every line, and its `Cursor` names the pointer's look over them ahead of the text's. `Visibility: core.VisibilityHidden` hides a span in place: its words keep their room, so the lines stay put as it shows, and copies and screen readers leave it out, for words that fade in one by one.

Set `BackgroundInsetTop: core.Pct(62)` on a span with a `Background` to highlight the lower 38% of its box on each line. `BackgroundInsetBottom` trims the lower edge. Both insets take points or percentages of the box's height, from the font's ascent to its descent. They leave text layout unchanged. In TSX, set `backgroundInsetTop: "62%"` and `backgroundInsetBottom` in a nested `<Text>`'s style. Rounded backgrounds and vertical insets also render on HarmonyOS and the four mini-program targets.

`Attachment` puts a view inside a text, in place of the span's content. It is laid out at its own size, wraps with the words around it and takes its own presses, so an icon, an image or a small button can sit in a line. A selection copies the span's `Content` in its place, leaving it out when empty. By default its first text line aligns to the surrounding baseline, using the metrics of its actual font. An `Em` canvas provides its own baseline, and a view without a baseline sits on its bottom edge. Set `Style.VerticalAlign` to choose another alignment.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## Fonts

`FontFamily` names a family or a CSS list (`Inter, "Noto Sans SC", sans-serif`), each used for the characters those before it lack. Each family in the list takes the weight and style. On macOS and iOS a line takes its height and baseline from the faces its characters are set in, so a line set wholly in a later family takes that family's, where a browser keeps the first family's as a floor. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing` and `LetterSpacingEm` set the face and the spacing, and `core.FontMonospace` is the platform's fixed-pitch family.

`Language` is the text's BCP 47 tag, inherited like the fields above on texts, spans and inputs. Han characters are drawn in its forms, so `直` set in `ja` takes its Japanese form even where the system prefers Chinese, and Korean (`ko`) breaks lines between words as CSS's `word-break: keep-all` does. Text and inputs that set none are in the language the app shows and follow it when it changes. Set it on a view for a subtree, or on a `Span` for one run:

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

Package the app's fonts under `resources/` and register them before the first view, or register a font's bytes while the app runs:

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // a TrueType or OpenType face
```

A text laid out before a font arrives is prepared again, and canvases run `Draw` again so their glyphs and text measurements use the loaded font. `cx.Env().FontScale()` is the user's text size, which texts follow unless `NoFontScaling` is set; `MaxFontSizeMultiplier` caps it.

## Selection and measurement

`SelectableOn` lets the user select and copy a text, and a `View` with `Selectable` makes its texts select together, in document order, as a web page's do. `SelectionColor` colors the highlight. `OnTextSelection` receives a `core.TextSelection` as the user moves the selection: its plain `Text`, `Markdown` and `HTML` (preserving the parsed inline and block structure of any `TextView`s and styled texts in the area, including partial spans and links), its per-text `Ranges` and the `Bounds` of the texts it touches, in the view, for copying as Markdown or HTML, a word list or a translation of the words selected; `desktop.LookUp` shows them in the system's dictionary ([services](services.md#opening-links-and-files)). `cx.MeasureText` measures a string in a font, for a column fitted to its text, and `OnTextLayout` reports the lines after each layout.

## Documents

`base/textview` shows Markdown or HTML with the same primitives: headings, paragraphs, lists, tables, code, footnotes, alerts, images, formulas and links.

```go
textview.TextView{
	Markdown: "## NSGo\n\nNative views from **Go**.",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

Formulas (`$…$`, `$$…$$`) show their TeX until the app passes `RenderMath: formula.RenderMath` from `base/formula`, which typesets them in the size and color of the text around them, read aloud in the app's language and copied as TeX; it adds about 6.5 MB to a desktop app and 3 MB gzipped to a web one, its font included. `formula.View` shows a formula anywhere else, in TeX or Typst; its `OnHit` callback reports the Untie source part, nearest caret and overlapping diagnostics for formula editors. On watchOS, formulas use the same canvas path; linking them adds about 6.1 MB of executable, including the 1.4 MB font. A watch simulator run measured about 3.9 MB of retained Go heap for loading and parsing that font, including its owned byte copy. Simulator measurements establish neither a physical watch's memory limit nor its available headroom; measure those on the watch that runs the app.

A `mermaid` code block shows its source until the app passes `RenderDiagram: diagram.RenderDiagram` from `base/diagram`, which draws it as a native diagram, with limu: flowcharts, sequence, class, state and entity relationship diagrams, Gantt charts, pies, mind maps, timelines and more, in the platform's font, light or dark as the app shows. It scrolls sideways when wider than the text, a copy takes its source, and a block still streaming in shows as code until it is complete. `diagram.View` shows a diagram anywhere else.

To give diagrams your `ui` palette, return a `diagram.View` with `ThemeFunc` from your `RenderDiagram` callback. The function reads colors as the canvas draws, so a palette or subtree appearance change recolors the existing view. Colors left zero keep the diagram's defaults.

```go
func drawDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ThemeFunc: func() *diagram.Theme {
		return &diagram.Theme{
			Text: colors.Text(), Surface: colors.Surface(), Border: colors.Border(),
			Line: colors.Muted(), Accent: colors.Accent(), Critical: colors.Danger(),
		}
	}}
}
// ui.TextView{Markdown: source, RenderDiagram: drawDiagram}
```

`textview.Parse` reads Markdown and `textview.ParseHTML` reads HTML into blocks the app can also build. `Options.Streaming` repairs a document that arrives in pieces, closing what its end leaves open and holding back a last line whose meaning the next characters decide, such as a list marker alone, so a reply never flashes markers as text, and `StreamingCaret` draws a caret at its end, for a reply written as it streams.

A `textview.Search` finds text across the views that join it, through their `Search` field or `textview.ProvideSearch` above them. `SetQuery`, `Open` and `Close` drive it from a find bar, `Count` and `Selected` say where it is, and `Next` and `Previous` step through its matches in reading order. The views mark each match with `PartSearch` and the selected one with `PartSearchSelected`, for the app's styles to color, scroll it into view, and leave the user's selection alone.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## Next

- [Images and icons](images.md): pictures and SVGs, in a text too.
- [Styles and themes](styles.md): colors and text fields that inherit.
- [Layout](layout.md): how a text takes its width.

## Rich text editing

`ui/richeditor.View` shows a `base/richeditor.Session` with the theme’s look; `base/richeditor.View` provides the behavior with app-supplied styles. On iOS and Android, a long press selects a word and a double tap does the same; drag the selection handles to extend the range across blocks. The native edit menu offers Copy and Select All, and Cut and Paste while the session is editable, as applicable to the selection and clipboard. `Session.SetEditable` also refreshes an open menu. Style the knobs with `Styles.SelectionHandle` or `Classes.SelectionHandle` on the base view.

For a custom text control, focus a `core.Input` and call `Ref.OpenEditMenu(bounds, actions)` with bounds in surface points and a bitmask of `core.EditCut`, `EditCopy`, `EditPaste` and `EditSelectAll`. On iOS and Android, `Input.OnEditAction` receives Cut, Copy and Select All; Paste uses `Input.OnPaste`. Call `Ref.CloseEditMenu` to dismiss it.
