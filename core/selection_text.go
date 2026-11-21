package core

// TextSelection is the selection of a Selectable view, as OnTextSelection receives it.
type TextSelection struct {
	// Text is the selected text as a copy gives it: texts on one row joined by a tab and the others by a line break.
	Text string
	// Markdown is the selection as Markdown, preserving the block and inline structure of TextViews and styled texts in the area; empty without a selection.
	Markdown string
	// HTML is the selection as HTML, preserving the block and inline structure of TextViews and styled texts in the area; empty without a selection.
	HTML string
	// Bounds is the box of the texts the selection touches, in the view: no Size without a selection. Ref.Bounds of the view gives the surface's.
	Bounds Bounds
	// Ranges is what each text of the view has selected, in the view's document order: the UTF-16 offsets of each one's content it holds, with the text's Ref to tell texts of the same content apart. It is nil on a platform that does not report it; Text and Bounds alone then say only what a copy and a menu do.
	Ranges []SelectedText
}

// SelectedText is what one text of a selection has, in a
// TextSelection's Ranges: the Ref the Text carried, nil without one,
// and the UTF-16 offsets of its content the selection holds, from
// Start to End.
type SelectedText struct {
	Ref        *Ref
	Start, End int
}

// SelectionBlockKind identifies the semantic block role of a View or Text when serializing a TextSelection to Markdown and HTML; base/textview sets it on the blocks it renders.
type SelectionBlockKind uint8

const (
	// SelectionBlockAuto infers formatting from a Text's spans and styles.
	SelectionBlockAuto SelectionBlockKind = iota
	// SelectionBlockParagraph is a paragraph of inline content.
	SelectionBlockParagraph
	// SelectionBlockHeading is a heading of Level 1 to 6.
	SelectionBlockHeading
	// SelectionBlockQuote is a blockquote, or a GitHub alert when Alert is set.
	SelectionBlockQuote
	// SelectionBlockList is an ordered or unordered list container.
	SelectionBlockList
	// SelectionBlockListItem is one item of a list.
	SelectionBlockListItem
	// SelectionBlockCode is a fenced or preformatted code block.
	SelectionBlockCode
	// SelectionBlockTable is a table container.
	SelectionBlockTable
	// SelectionBlockTableCell is a header or body cell of a table.
	SelectionBlockTableCell
	// SelectionBlockDetails is a disclosure (<details>).
	SelectionBlockDetails
	// SelectionBlockSummary is a disclosure's summary (<summary>).
	SelectionBlockSummary
	// SelectionBlockMath is a display math formula block.
	SelectionBlockMath
	// SelectionBlockFootnote is a footnote definition block.
	SelectionBlockFootnote
	// SelectionBlockRule is a thematic break (--- / <hr>).
	SelectionBlockRule
	// SelectionBlockIgnore marks decorative views or texts (list markers, alert titles, disclosure arrows, footnote numbers, code actions) that belong to their container's markup and are omitted from Markdown and HTML output.
	SelectionBlockIgnore
)

// SelectionAlign is a table column's alignment for Markdown and HTML serialization; the core's part of base/textview's ColumnAlign.
type SelectionAlign uint8

const (
	SelectionAlignNone SelectionAlign = iota
	SelectionAlignLeft
	SelectionAlignCenter
	SelectionAlignRight
)

// SelectionBlock describes a View's or Text's semantic block structure for serializing a TextSelection to Markdown and HTML: the core's part of base/textview's document blocks, which base/textview sets on its views and texts.
type SelectionBlock struct {
	Kind SelectionBlockKind
	// Level is 1 to 6 for SelectionBlockHeading.
	Level int
	// Ordered, Start and Loose are for SelectionBlockList.
	Ordered bool
	Start   int
	Loose   bool
	// Index is the 0-based item index for SelectionBlockListItem, or the 1-based number for SelectionBlockFootnote.
	Index int
	// Task and Done are for task items in SelectionBlockListItem.
	Task bool
	Done bool
	// Alert is the GitHub alert kind ("NOTE", "TIP", "IMPORTANT", "WARNING", "CAUTION") for SelectionBlockQuote, and Title its display title.
	Alert string
	Title string
	// Language and Meta are for SelectionBlockCode.
	Language string
	Meta     string
	// Align is the column alignments for SelectionBlockTable.
	Align []SelectionAlign
	// Header, Row, Col and ColSpan are for SelectionBlockTableCell.
	Header  bool
	Row     int
	Col     int
	ColSpan int
	// Label is the footnote label for SelectionBlockFootnote.
	Label string
	// TeX is the raw formula for SelectionBlockMath.
	TeX string
	// Inlines describes the rich inline tree of a Text node's content, aligned with its rendered UTF-16 offsets.
	Inlines []SelectionInline
	// Children holds nested blocks for FormatSelection; mounted Views use their tree children instead.
	Children []SelectionBlock
}

// SelectionInlineKind identifies an inline element inside a Text node for serializing a TextSelection to Markdown and HTML; base/textview sets it.
type SelectionInlineKind uint8

const (
	SelectionInlineText SelectionInlineKind = iota
	SelectionInlineStrong
	SelectionInlineEmphasis
	SelectionInlineStrikethrough
	SelectionInlineUnderline
	SelectionInlineMark
	SelectionInlineSuperscript
	SelectionInlineSubscript
	SelectionInlineSmall
	SelectionInlineCode
	SelectionInlineKeyboard
	SelectionInlineMath
	SelectionInlineLink
	SelectionInlineImage
	SelectionInlineBreak
	SelectionInlineFootnoteRef
	// SelectionInlineSkip occupies UTF-16 code units in the rendered Text (such as a streaming caret or hidden span) and emits nothing.
	SelectionInlineSkip
)

// SelectionInline is one node in a Text's rich inline tree for selection serialization: the core's part of base/textview's Inline nodes. Leaf nodes occupy UTF16Len UTF-16 code units in the rendered Text (defaulting to the UTF-16 length of Text when UTF16Len is 0); formatting wrappers and links wrap Children.
type SelectionInline struct {
	Kind SelectionInlineKind
	// Text is the leaf text (plain text, code, keyboard, math TeX, or image alt).
	Text string
	// URL is the link target for SelectionInlineLink or image src for SelectionInlineImage.
	URL string
	// Title is the optional title for SelectionInlineLink or SelectionInlineImage.
	Title string
	// Label and Index identify a footnote for SelectionInlineFootnoteRef.
	Label string
	Index int
	// Color is the optional background color for SelectionInlineMark.
	Color Color
	// UTF16Len, when > 0, overrides the UTF-16 code units this leaf occupies in the rendered Text (e.g. 1 for an AttachmentRune).
	UTF16Len int
	// Children are nested inline nodes for formatting wrappers and links.
	Children []SelectionInline
}

// FormatSelection serializes selection blocks as Markdown and HTML: the core's part of base/textview's Document.Markdown and Document.HTML.
func FormatSelection(blocks []SelectionBlock) (markdown, html string) {
	nodes := make([]*selNode, 0, len(blocks))
	for _, b := range blocks {
		if n := selNodeFromBlock(b); n != nil {
			nodes = append(nodes, n)
		}
	}
	return serializeSelection(nodes)
}
