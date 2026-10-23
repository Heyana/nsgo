package core

// Cursor is the pointer's look over a view (cursor). Auto leaves the
// look to the view under it or its nearest ancestor that names one; a
// pressable view's Auto is a pointer on the web.
type Cursor uint8

const (
	CursorAuto Cursor = iota
	CursorDefault
	CursorPointer
	CursorText
	CursorWait
	CursorProgress
	CursorHelp
	CursorCrosshair
	CursorMove
	CursorGrab
	CursorGrabbing
	CursorNotAllowed
	CursorZoomIn
	CursorZoomOut
	CursorColResize
	CursorRowResize
	CursorNone
)

var cursorNames = [...]string{
	"auto", "default", "pointer", "text", "wait", "progress", "help", "crosshair",
	"move", "grab", "grabbing", "not-allowed", "zoom-in", "zoom-out",
	"col-resize", "row-resize", "none",
}

// Name returns the cursor's CSS name.
func (c Cursor) Name() string {
	if int(c) < len(cursorNames) {
		return cursorNames[c]
	}
	return cursorNames[CursorAuto]
}
