// Package scrollbar lays out the scroll bars of the backends that paint
// their own views: where a scroll view's thumb is, whether a point is on
// its bar, and where dragging the thumb scrolls to. The backends show a
// bar while the pointer is over its scroll view or drags its thumb.
package scrollbar

import "github.com/oneo-me/nsgo/core"

const (
	// Thickness is the thumb's, Inset its distance from the edge.
	Thickness core.Points = 6
	Inset     core.Points = 2
	// MinLength is the shortest a thumb gets on long content.
	MinLength core.Points = 24
	// Grab is the width of the strip along the edge where a press takes
	// the bar.
	Grab core.Points = 12
)

// Bar is a scroll view's scroll bar.
type Bar struct {
	Horizontal bool
	// Frame is the scroll view's size, Content its content's and Offset
	// how far the content is scrolled.
	Frame, Content core.Size
	Offset         core.Point
}

// along returns the bar's lengths along its axis: the view's, the
// content's, the offset and the thumb's.
func (b Bar) along() (frame, content, offset, thumb core.Points) {
	frame, content, offset = b.Frame.Height, b.Content.Height, b.Offset.Y
	if b.Horizontal {
		frame, content, offset = b.Frame.Width, b.Content.Width, b.Offset.X
	}
	if content <= frame || frame <= 0 {
		return frame, content, offset, 0
	}
	return frame, content, offset, min(max(frame*frame/content, MinLength), frame)
}

// Thumb returns the thumb's rectangle in the scroll view, or false when
// the content fits and there is nothing to scroll.
func (b Bar) Thumb() (core.Bounds, bool) {
	frame, content, offset, length := b.along()
	if length == 0 {
		return core.Bounds{}, false
	}
	at := min(max(offset/(content-frame), 0), 1) * (frame - length)
	if b.Horizontal {
		return core.Bounds{Origin: core.Point{X: at, Y: b.Frame.Height - Inset - Thickness},
			Size: core.Size{Width: length, Height: Thickness}}, true
	}
	return core.Bounds{Origin: core.Point{X: b.Frame.Width - Inset - Thickness, Y: at},
		Size: core.Size{Width: Thickness, Height: length}}, true
}

// Contains reports whether p, in the scroll view, is on the bar: in the
// strip along its edge, while there is something to scroll.
func (b Bar) Contains(p core.Point) bool {
	if _, ok := b.Thumb(); !ok || !(core.Bounds{Size: b.Frame}).Contains(p) {
		return false
	}
	if b.Horizontal {
		return p.Y >= b.Frame.Height-Grab
	}
	return p.X >= b.Frame.Width-Grab
}

// Drag follows the pointer pressing a bar.
type Drag struct {
	bar   Bar
	from  core.Points // where the pointer took the thumb, along the axis
	start core.Points // the offset then
}

// Press starts dragging the bar from p, in the scroll view. Off the
// thumb, the thumb first jumps to center under p.
func (b Bar) Press(p core.Point) Drag {
	frame, content, offset, length := b.along()
	at := p.Y
	if b.Horizontal {
		at = p.X
	}
	d := Drag{bar: b, from: at, start: offset}
	if thumb := min(max(offset/(content-frame), 0), 1) * (frame - length); length > 0 && frame > length &&
		(at < thumb || at >= thumb+length) {
		d.start = (at - length/2) / (frame - length) * (content - frame)
	}
	return d
}

// Offset returns where the content scrolls to with the pointer at p, in
// the scroll view, clamped to what it can show.
func (d Drag) Offset(p core.Point) core.Point {
	frame, content, _, length := d.bar.along()
	out := d.bar.Offset
	if length == 0 || frame <= length {
		return out
	}
	at := p.Y
	if d.bar.Horizontal {
		at = p.X
	}
	to := min(max(d.start+(at-d.from)*(content-frame)/(frame-length), 0), content-frame)
	if d.bar.Horizontal {
		out.X = to
	} else {
		out.Y = to
	}
	return out
}
