package core

import (
	"math"
	"testing"
)

// These follow NSH's ListState tests (crates/nsh-core/src/core/list.rs).

func testList(count int, estimated, viewport Points) *listState {
	s := newListState()
	s.estimated = estimated
	s.setCount(count)
	s.viewport = viewport
	return &s
}

func TestListStateWindowAndAnchoring(t *testing.T) {
	l := testList(1000, 50, 400)
	if first, last, ok, _ := l.window(); !ok || first != 0 || last != 16 {
		t.Errorf("window %d..%d, want 0..16", first, last)
	}
	l.offset = 1000
	// Item 0 lies above the viewport: growing it scrolls by as much.
	if d := l.measured(0, 80); d != 30 || l.itemAt(1) != 80 {
		t.Errorf("measured above: %v, item 1 at %v", d, l.itemAt(1))
	}
	if d := l.measured(30, 10); d != 0 {
		t.Errorf("measured in view: %v", d)
	}
}

func TestListStateColumns(t *testing.T) {
	l := testList(10, 50, 1000)
	l.setColumns(3)
	if l.blocks().total != 200 || l.itemAt(4) != 50 || l.columnOf(4) != 1 {
		t.Fatalf("total %v, item 4 at %v", l.blocks().total, l.itemAt(4))
	}
	l.measured(4, 80)
	if l.itemAt(6) != 130 {
		t.Errorf("item 6 at %v, want 130", l.itemAt(6))
	}
	if x, w := l.column(320, 2); x != 320*2/3. || w != 320/3. {
		t.Errorf("column 2 at %v, %v wide", x, w)
	}
}

func TestListStateFollowsTheEnd(t *testing.T) {
	l := testList(100, 50, 400)
	l.followEnd = 10
	if d := l.anchorEnd(); d != 0 {
		t.Fatalf("starts where it is, moved %v", d)
	}
	l.offset = 5000 - 400
	l.measured(99, 80)
	if d := l.anchorEnd(); d != 30 || l.offset != 5030-400 {
		t.Errorf("at the end: %v, offset %v", d, l.offset)
	}
	l.offset = 1000
	l.measured(99, 120)
	if d := l.anchorEnd(); d != 0 || l.offset != 1000 {
		t.Errorf("scrolled up: %v, offset %v", d, l.offset)
	}
}

func TestListStateInverted(t *testing.T) {
	l := testList(100, 50, 400)
	l.inverted = true
	if d := l.anchorEnd(); d != 4600 || l.toContent(l.itemAt(0), 50) != 4950 {
		t.Fatalf("starts by %v, item 0 at %v", d, l.toContent(l.itemAt(0), 50))
	}
	l.setCount(120)
	if d := l.anchorEnd(); d != 1000 || l.offset != 5600 {
		t.Errorf("older rows: %v, offset %v", d, l.offset)
	}
	if got := l.offsetForIndex(0, 0, 0); got != 5600 {
		t.Errorf("item 0 at the bottom of the viewport: %v", got)
	}
}

func TestListStateStickyRanges(t *testing.T) {
	l := testList(100, 50, 400)
	l.setSticky([]int{30, 0})
	l.stickyOffset = 20
	if r := l.stickyRange(0, 1, 50); r.Start != -20 || r.End != 1430 {
		t.Errorf("row 0 travels %v..%v", r.Start, r.End)
	}
	if r := l.stickyRange(30, 2, 50); r.Start != 1480 || !math.IsInf(float64(r.End), 1) {
		t.Errorf("row 30 travels %v..%v", r.Start, r.End)
	}
	l.offset = 1470
	if a, _ := l.activeSticky(); a != 0 {
		t.Errorf("active %d at 1470", a)
	}
	l.offset = 1480
	if a, _ := l.activeSticky(); a != 30 {
		t.Errorf("active %d at 1480", a)
	}
	// The pinned one and the one before stay in the window.
	l.setSticky([]int{0, 30, 60})
	l.stickyOffset = 0
	l.offset = 3200
	if _, _, _, sticky := l.window(); len(sticky) != 2 || sticky[0] != 60 || sticky[1] != 30 {
		t.Errorf("sticky rows in the window %v", sticky)
	}
}

func TestListStateViewability(t *testing.T) {
	l := testList(100, 100, 400)
	l.offset = 50
	check := func(want0, want1 int) {
		t.Helper()
		if f, la, ok := l.viewableItems(); !ok || f != want0 || la != want1 {
			t.Errorf("viewable %d..%d, want %d..%d", f, la, want0, want1)
		}
	}
	check(0, 4)
	l.viewability.ItemVisiblePercentThreshold = 60
	check(1, 3)
	l.viewability.ItemVisiblePercentThreshold = 50
	check(0, 4)
}

func TestListStateSplicesAndPins(t *testing.T) {
	l := testList(100, 50, 400)
	l.offset = 1000
	l.measured(25, 80)
	before := l.itemAt(20) - l.offset
	l.splice(0, 0, 3)
	if l.count != 103 || l.itemAt(23)-l.offset != before || l.takeSpliceAnchor() != 150 || l.rowExtent(28) != 80 {
		t.Errorf("after a splice above: count %d, anchor kept %v", l.count, l.itemAt(23)-l.offset == before)
	}
	off := l.offset
	l.splice(90, 2, 0)
	if l.offset != off {
		t.Errorf("a splice below moved the list")
	}

	l = testList(100, 50, 400)
	l.pin = &listPin{end: true, at: 4600, animating: true}
	l.measured(99, 90)
	if _, to, re := l.followPin(0); !re || to != 4640 {
		t.Errorf("retarget %v %v", re, to)
	}
	l.movedTo(3000, true)
	if l.pin == nil {
		t.Fatal("in flight, the pin ended")
	}
	l.movedTo(4640, true)
	l.measured(98, 60)
	if by, _, _ := l.followPin(0); by != 10 || l.offset != 4650 {
		t.Errorf("follows by %v to %v", by, l.offset)
	}
	l.movedTo(4000, true)
	if l.pin != nil {
		t.Error("the user scrolled away and the pin stayed")
	}
}

func TestListStateInItsParentsViewport(t *testing.T) {
	page := func(start, offset, max Points) *listState {
		l := testList(100, 50, 0)
		l.parent = true
		l.setOuter(start, offset, 400, max, true)
		return l
	}
	l := page(100, 0, 4700)
	if l.offset != -100 {
		t.Fatalf("offset %v", l.offset)
	}
	if f, la, ok, _ := l.window(); !ok || f != 0 || la != 14 {
		t.Errorf("window %d..%d", f, la)
	}
	if got := l.offsetForIndex(99, 0, 0); got != 4600 {
		t.Errorf("row 99 at %v", got)
	}
	if got := l.endOffset(); got != 4600 {
		t.Errorf("end at %v", got)
	}
	l.setSticky([]int{0, 30})
	l.stickyOffset = 20
	if r := l.stickyRange(30, 2, 50); r.Start != 100+1500-20 || r.End != 100+5000-50-20 {
		t.Errorf("last sticky row travels %v..%v", r.Start, r.End)
	}
	// Only changes above the viewport scroll the page.
	if d := l.measured(0, 80); d != 0 {
		t.Errorf("measured with the list's start in view: %v", d)
	}
	l = page(100, 1100, 4700)
	if d := l.measured(2, 80); d != 30 {
		t.Errorf("measured above the viewport: %v", d)
	}
}

func TestListSplice(t *testing.T) {
	for _, c := range []struct {
		old, new     []any
		at, rem, ins int
		changed      bool
	}{
		{[]any{1, 2, 3}, []any{1, 2, 3}, 3, 0, 0, false},
		{[]any{1, 2, 3}, []any{0, 1, 2, 3}, 0, 0, 1, true},
		{[]any{1, 2, 3}, []any{1, 3}, 1, 1, 0, true},
		{[]any{1, 2, 3}, []any{1, 2, 3, 4}, 3, 0, 1, true},
		{[]any{1, 1}, []any{1}, 1, 1, 0, true},
	} {
		at, rem, ins, ok := listSplice(c.old, c.new)
		if ok != c.changed || ok && (at != c.at || rem != c.rem || ins != c.ins) {
			t.Errorf("%v -> %v: %d %d %d %v", c.old, c.new, at, rem, ins, ok)
		}
	}
}

func TestListStateInvertedLoadingAtStartKeepsTheVisibleRows(t *testing.T) {
	l := testList(100, 50, 400)
	l.inverted, l.loadsAtStart = true, true
	l.anchorEnd()
	// 200 points from its start.
	l.offset = 5000 - 400 - 200
	onScreen := func(l *listState, i int) Points { return l.toContent(l.itemAt(i), 50) - l.offset }
	// Newer items arrive before item 0: the rows on screen stay.
	before := onScreen(l, 4)
	l.splice(0, 0, 4)
	if d := l.takeSpliceAnchor() + l.anchorEnd(); d != 0 || onScreen(l, 8) != before {
		t.Errorf("splice scrolled %v; item 8 at %v, want %v", d, onScreen(l, 8), before)
	}
	// So does their measuring.
	offset := l.offset
	anchor := l.measured(0, 80)
	l.offset += anchor
	if d := anchor + l.anchorEnd(); d != 0 || l.offset != offset {
		t.Errorf("measuring scrolled %v to %v, want %v", d, l.offset, offset)
	}

	// At the start, what arrives there shows (a chat's new message).
	c := testList(100, 50, 400)
	c.inverted, c.loadsAtStart = true, true
	c.anchorEnd()
	c.splice(0, 0, 1)
	c.takeSpliceAnchor()
	c.anchorEnd()
	if n := c.naturalOffset(); n != 0 {
		t.Errorf("at its start: natural offset %v, want 0", n)
	}

	// Without OnLoadMoreAtStart an inverted list keeps its distance from
	// the end wherever it is.
	d := testList(100, 50, 400)
	d.inverted = true
	d.anchorEnd()
	d.offset = 5000 - 400 - 200
	d.splice(0, 0, 4)
	if a := d.takeSpliceAnchor(); a != 0 {
		t.Errorf("splice anchor %v, want none", a)
	}
	d.anchorEnd()
	if n := d.naturalOffset(); n != 200 {
		t.Errorf("natural offset %v, want 200", n)
	}
}
