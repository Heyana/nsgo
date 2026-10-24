package core

// The caret and pastes of text inputs, as NSH's onCaretMove and onPaste
// report them.

// CaretEvent is where an input's caret is: Bounds in the input's view,
// as wide as the platform draws the caret and as tall as its line, and
// PageBounds the same in the surface, which an Overlay's AnchorBounds
// takes. Empty Bounds mean the platform could not place it yet.
type CaretEvent struct {
	Bounds     Bounds
	PageBounds Bounds
}

// PasteEvent is what the user pasted into an input, read from the
// clipboard as they pasted: its plain text (empty without any), files
// with their bytes, such as an image copied in another app, and the
// file: URLs of local files copied in a file manager.
type PasteEvent struct {
	Text  string
	Files []PastedFile
	URIs  []string
}

// PastedFile is a file the clipboard held: its name when it has one, its
// MIME type and its bytes.
type PastedFile struct {
	Name string
	MIME string
	Data []byte
}

// PasteBehavior says whether an input inserts the text the user pastes.
type PasteBehavior uint8

const (
	// PasteInsert inserts it, as inputs do.
	PasteInsert PasteBehavior = iota
	// PasteReport only reports it with OnPaste, for the app to handle.
	PasteReport
)
