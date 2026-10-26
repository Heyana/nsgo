// Package scrollbar is the scroll logic of the backends that keep scroll
// offsets of their own: where a scroll view's thumb is, whether a point is
// on its bar, and where dragging the thumb scrolls to; the offsets its
// content inset allows; and where a snapping view settles after a wheel
// or a thumb drag. The backends show a bar while the pointer is over its
// scroll view or drags its thumb. macOS takes the motion too (motion.go):
// the rubber band, eased snaps, the indicator's geometry and fade, and
// trackpad events shown in step with the display.
package scrollbar

import (
	"github.com/oneo-me/nsgo/internal/driver"
)

const (
	// Thickness is the thumb's, Inset its distance from the edge.
	Thickness driver.Points = 6
	Inset     driver.Points = 2
	// MinLength is the shortest a thumb gets on long content.
	MinLength driver.Points = 24
	// Grab is the width of the strip along the edge where a press takes
	// the bar.
	Grab driver.Points = 12
)

// Bar is a scroll view's scroll bar.
type Bar struct {
	Horizontal bool
	// Frame is the scroll view's size, Content its content's and Offset
	// how far the content is scrolled.
	Frame, Content driver.Size
	Offset         driver.Point
	// Inset is the content inset, which scrolls as part of the content.
	Inset driver.Edges
}

// along returns the bar's lengths along its axis: the view's, the
// content's, the offset and the thumb's.
func (b Bar) along() (frame, content, offset, thumb driver.Points) {
	frame, content, offset = b.Frame.Height, b.Content.Height+b.Inset.Top+b.Inset.Bottom, b.Offset.Y+b.Inset.Top
	if b.Horizontal {
		frame, content, offset = b.Frame.Width, b.Content.Width+b.Inset.Left+b.Inset.Right, b.Offset.X+b.Inset.Left
	}
	if content <= frame || frame <= 0 {
		return frame, content, offset, 0
	}
	return frame, content, offset, min(max(frame*frame/content, MinLength), frame)
}

// Thumb returns the thumb's rectangle in the scroll view, or false when
// the content fits and there is nothing to scroll.
func (b Bar) Thumb() (driver.Bounds, bool) {
	frame, content, offset, length := b.along()
	if length == 0 {
		return driver.Bounds{}, false
	}
	at := min(max(offset/(content-frame), 0), 1) * (frame - length)
	if b.Horizontal {
		return driver.Bounds{Origin: driver.Point{X: at, Y: b.Frame.Height - Inset - Thickness},
			Size: driver.Size{Width: length, Height: Thickness}}, true
	}
	return driver.Bounds{Origin: driver.Point{X: b.Frame.Width - Inset - Thickness, Y: at},
		Size: driver.Size{Width: Thickness, Height: length}}, true
}

// Contains reports whether p, in the scroll view, is on the bar: in the
// strip along its edge, while there is something to scroll.
func (b Bar) Contains(p driver.Point) bool {
	if _, ok := b.Thumb(); !ok || !(driver.Bounds{Size: b.Frame}).Contains(p) {
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
	from  driver.Points // where the pointer took the thumb, along the axis
	start driver.Points // the offset then
}

// Shift moves the drag along with the core moving the offset by d along
// the axis (ItemScrollBy), so it continues from the content it was on.
func (d *Drag) Shift(by driver.Points) { d.start += by }

// Press starts dragging the bar from p, in the scroll view. Off the
// thumb, the thumb first jumps to center under p.
func (b Bar) Press(p driver.Point) Drag {
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
func (d Drag) Offset(p driver.Point) driver.Point {
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
		out.X = to - d.bar.Inset.Left
	} else {
		out.Y = to - d.bar.Inset.Top
	}
	return out
}
