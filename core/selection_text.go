package core

// TextSelection is the selection of a Selectable view, as OnTextSelection receives it.
type TextSelection struct {
	// Text is the selected text as a copy gives it: texts on one row joined by a tab and the others by a line break.
	Text string
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
