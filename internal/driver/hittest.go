package driver

import (
	"slices"
	"sync"
)

// A final text layout answers where its carets are, as NSH's TextEngine
// hit testing does: the caret nearest a point, the caret's box at a
// position, and the boxes a range covers. Points are in the layout's
// content box, y down; offsets are UTF-16 units, as platforms and
// selections count them.

// Affinity says which line a caret offset is on where one offset is two
// places on screen: the end of a wrapped line and the start of the next.
type Affinity uint8

const (
	// Downstream is with the character after the offset: at the start of
	// the later line.
	Downstream Affinity = iota
	// Upstream is with the character before the offset: at the end of the
	// earlier line.
	Upstream
)

// Caret is a caret position in a text: a UTF-16 offset and the side it
// is on.
type Caret struct {
	Offset   int
	Affinity Affinity
}

// TextHitTester is a TextLayout that knows where its characters are. Its
// methods are safe on the engine goroutine and the main thread, also
// after the layout was released: what they read before stays, and a
// layout released unread answers as an empty text.
type TextHitTester interface {
	// HitTest returns the caret nearest p: on the line at p's height (the
	// first line above the text, the last below it), at the character
	// edge nearest p's x. A point past the end of a wrapped line gives the
	// line's end, Upstream, so the caret stays on that line.
	HitTest(p Point) Caret
	// CaretRect returns the caret at c: a box of no width as high as its
	// line, at the edge of the characters c stands between; false when
	// the layout has no lines.
	CaretRect(c Caret) (Bounds, bool)
	// RangeRects appends to out the line-high boxes covering the
	// characters from UTF-16 offset start to end: per line, one box per
	// run of adjacent characters (several where bidirectional text splits
	// the range), and through the line's end where the range goes on past
	// it. out is the caller's, so a selection drawn often reuses it.
	RangeRects(start, end int, out []Bounds) []Bounds
}

// HitTest returns the caret nearest p in l, the start of the text when l
// does not hit-test.
func HitTest(l TextLayout, p Point) Caret {
	if h, ok := l.(TextHitTester); ok {
		return h.HitTest(p)
	}
	return Caret{}
}

// CaretRect returns the caret at c in l; false when l has no lines or
// does not hit-test.
func CaretRect(l TextLayout, c Caret) (Bounds, bool) {
	if h, ok := l.(TextHitTester); ok {
		return h.CaretRect(c)
	}
	return Bounds{}, false
}

// RangeRects appends to out the boxes of the UTF-16 range from start to
// end in l.
func RangeRects(l TextLayout, start, end int, out []Bounds) []Bounds {
	if h, ok := l.(TextHitTester); ok {
		return h.RangeRects(start, end, out)
	}
	return out
}

// CaretLine is a laid-out line as CaretLayout hit-tests it.
type CaretLine struct {
	// Start and End are the UTF-16 offsets of its first character and
	// past its last, its line break included.
	Start, End int
	// Box is the line box: as high as the line, from its left edge to its
	// right, in the content box.
	Box Bounds
	// RTL says the line runs right to left: it ends at its left.
	RTL bool
	// Clusters are its characters' places in logical order, without its
	// line break.
	Clusters []CaretCluster
}

// CaretCluster is where characters that take one caret stop at each end
// (a grapheme cluster, or a ligature) sit on their line: UTF-16 offsets
// and their left and right edges.
type CaretCluster struct {
	Start, End  int
	Left, Right Points
	// RTL says they run right to left: they start at Right.
	RTL bool
}

func (c *CaretCluster) leading() Points {
	if c.RTL {
		return c.Right
	}
	return c.Left
}

func (c *CaretCluster) trailing() Points {
	if c.RTL {
		return c.Left
	}
	return c.Right
}

// CaretLayout hit-tests the lines an engine read from its layout, so
// every engine answers as the others do.
type CaretLayout []CaretLine

// line returns the index of the line the caret at c is on: the line
// holding the offset, the earlier of two for Upstream at a line's end;
// past every line, the last starting at or before it; -1 without lines.
func (cl CaretLayout) line(c Caret) int {
	o := c.Offset
	for i := range cl {
		l := &cl[i]
		if c.Affinity == Upstream && l.Start < o && o <= l.End || c.Affinity == Downstream && l.Start <= o && o < l.End {
			return i
		}
	}
	at := -1
	for i := range cl {
		if cl[i].Start <= o {
			at = i
		}
	}
	if at < 0 && len(cl) > 0 {
		at = 0
	}
	return at
}

// HitTest implements TextHitTester.
func (cl CaretLayout) HitTest(p Point) Caret {
	if len(cl) == 0 {
		return Caret{}
	}
	l := &cl[len(cl)-1]
	for i := range cl {
		if p.Y < cl[i].Box.Origin.Y+cl[i].Box.Size.Height {
			l = &cl[i]
			break
		}
	}
	if len(l.Clusters) == 0 {
		return Caret{Offset: l.Start}
	}
	best, offset := Points(-1), l.Start
	for i := range l.Clusters {
		c := &l.Clusters[i]
		for _, e := range [2]struct {
			x Points
			o int
		}{{c.leading(), c.Start}, {c.trailing(), c.End}} {
			if d := abs(p.X - e.x); best < 0 || d < best {
				best, offset = d, e.o
			}
		}
	}
	// The end of a line that wraps is the end of that line.
	if last := l.Clusters[len(l.Clusters)-1].End; offset == last && last == l.End {
		return Caret{Offset: offset, Affinity: Upstream}
	}
	return Caret{Offset: offset}
}

// CaretRect implements TextHitTester.
func (cl CaretLayout) CaretRect(c Caret) (Bounds, bool) {
	i := cl.line(c)
	if i < 0 {
		return Bounds{}, false
	}
	l := &cl[i]
	x := l.Box.Origin.X
	if l.RTL {
		x += l.Box.Size.Width
	}
	found := false
	// The cluster after the offset, or the one before for Upstream: at a
	// change of direction they are in two places.
	for k := range l.Clusters {
		k := &l.Clusters[k]
		if k.Start == c.Offset && (c.Affinity == Downstream || !found) {
			x, found = k.leading(), true
		} else if k.End == c.Offset && (c.Affinity == Upstream || !found) {
			x, found = k.trailing(), true
		} else if !found && k.Start < c.Offset && c.Offset < k.End {
			x = k.leading()
		}
	}
	if len(l.Clusters) > 0 && !found && c.Offset >= l.Clusters[len(l.Clusters)-1].End {
		x = l.Clusters[len(l.Clusters)-1].trailing()
	}
	return Bounds{Origin: Point{X: x, Y: l.Box.Origin.Y}, Size: Size{Height: l.Box.Size.Height}}, true
}

// RangeRects implements TextHitTester.
func (cl CaretLayout) RangeRects(start, end int, out []Bounds) []Bounds {
	if start >= end {
		return out
	}
	for i := range cl {
		l := &cl[i]
		if l.Start >= end || start >= l.End {
			continue
		}
		from := len(out)
		for k := range l.Clusters {
			c := &l.Clusters[k]
			if c.Start < end && start < c.End {
				out = append(out, Bounds{Origin: Point{X: c.Left, Y: l.Box.Origin.Y},
					Size: Size{Width: c.Right - c.Left, Height: l.Box.Size.Height}})
			}
		}
		boxes := out[from:]
		slices.SortFunc(boxes, func(a, b Bounds) int {
			switch {
			case a.Origin.X < b.Origin.X:
				return -1
			case a.Origin.X > b.Origin.X:
				return 1
			}
			return 0
		})
		// Adjacent characters make one box.
		merged := boxes[:0]
		for _, b := range boxes {
			if n := len(merged); n > 0 && b.Origin.X <= merged[n-1].Origin.X+merged[n-1].Size.Width+0.5 {
				last := &merged[n-1]
				last.Size.Width = max(last.Size.Width, b.Origin.X+b.Size.Width-last.Origin.X)
				continue
			}
			merged = append(merged, b)
		}
		out = out[:from+len(merged)]
		// A range that goes on past the line covers the rest of it.
		if end > l.End || end == l.End && i+1 < len(cl) && cl[i+1].Start == l.End && start < l.End {
			out = cl.throughEnd(l, out, from)
		}
	}
	return out
}

// throughEnd stretches the boxes from index from of line l to the line's
// end edge, or adds a box of no width there.
func (cl CaretLayout) throughEnd(l *CaretLine, out []Bounds, from int) []Bounds {
	left, right := l.Box.Origin.X, l.Box.Origin.X+l.Box.Size.Width
	for _, c := range l.Clusters {
		left, right = min(left, c.Left), max(right, c.Right)
	}
	if len(out) == from {
		x := right
		if l.RTL {
			x = left
		}
		return append(out, Bounds{Origin: Point{X: x, Y: l.Box.Origin.Y}, Size: Size{Height: l.Box.Size.Height}})
	}
	if l.RTL {
		first := &out[from]
		first.Size.Width += first.Origin.X - left
		first.Origin.X = left
	} else {
		last := &out[len(out)-1]
		last.Size.Width = right - last.Origin.X
	}
	return out
}

// LazyCarets builds a CaretLayout once, when hit testing first asks, and
// guards the native layout it reads from against its release: an engine
// embeds it, calls Set with the function that reads the lines while the
// layout is alive, and Drop as it releases it.
type LazyCarets struct {
	mu    sync.Mutex
	read  func() CaretLayout
	lines CaretLayout
	done  bool
}

// Set has l read its lines with read, the first time they are asked for.
func (l *LazyCarets) Set(read func() CaretLayout) {
	l.mu.Lock()
	l.read, l.lines, l.done = read, nil, false
	l.mu.Unlock()
}

// Drop forgets the native layout: lines not read yet stay empty.
func (l *LazyCarets) Drop() {
	l.mu.Lock()
	l.read = nil
	l.mu.Unlock()
}

// Lines returns the lines, read on first use.
func (l *LazyCarets) Lines() CaretLayout {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.done && l.read != nil {
		l.lines, l.done = l.read(), true
		l.read = nil
	}
	return l.lines
}

// HitTest implements TextHitTester.
func (l *LazyCarets) HitTest(p Point) Caret { return l.Lines().HitTest(p) }

// CaretRect implements TextHitTester.
func (l *LazyCarets) CaretRect(c Caret) (Bounds, bool) { return l.Lines().CaretRect(c) }

// RangeRects implements TextHitTester.
func (l *LazyCarets) RangeRects(start, end int, out []Bounds) []Bounds {
	return l.Lines().RangeRects(start, end, out)
}
