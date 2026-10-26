// Package window is the rows a virtualized view of base makes: a table's,
// an editor's.
package window

// Rows is the rows a virtualized view makes: those in view and a
// reach around them, moved only when the viewport nears the window's edge,
// so scrolling a little changes no rows and a scroll finds the rows of the
// next frames made already.
type Rows struct {
	// margin is rows kept made past the viewport's edges, at least; ahead
	// rows made ahead of it when the window moves.
	margin, ahead     int
	start, end, first int
}

// New is a window keeping margin rows made past the viewport's edges and
// making ahead rows ahead of it when it moves, at least.
func New(margin, ahead int) *Rows { return &Rows{margin: margin, ahead: ahead} }

// Span is the rows to make with visible rows in view of count: the last
// window while it covers them with half a viewport to spare, else the
// visible rows, a viewport and a half more ahead the way they scrolled and
// half a viewport behind.
func (w *Rows) Span(vStart, vEnd, count int) (int, int) {
	page := vEnd - vStart
	margin := max(page/2, w.margin)
	needStart, needEnd := max(vStart-margin, 0), min(vEnd+margin, count)
	if w.start <= needStart && needEnd <= w.end && w.end <= count && w.end-w.start <= page*4+w.ahead*3 {
		return w.start, w.end
	}
	ahead, behind := max(page*3/2, w.ahead), max(page/2, w.ahead/2)
	before, after := behind, ahead
	if vStart < w.first {
		before, after = ahead, behind
	}
	w.start, w.end, w.first = min(max(vStart-before, 0), count), min(vEnd+after, count), vStart
	return w.start, w.end
}
