package watchos

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type decodedStickyOp struct {
	scrollID driver.NodeID
	cells    []watchStickyCell
}

// decodeStickyOps decodes opStickyCells records using the exact byte layout
// Host.swift's Reader consumes: op (u8), scroll ID (u32), count (u32), then
// count tuples of (cell u32, start f64, end f64).
func decodeStickyOps(t *testing.T, buf []byte) []decodedStickyOp {
	t.Helper()
	var ops []decodedStickyOp
	pos := 0
	readU32 := func() uint32 {
		if pos+4 > len(buf) {
			t.Fatalf("truncated u32 at %d of %d", pos, len(buf))
		}
		v := binary.LittleEndian.Uint32(buf[pos : pos+4])
		pos += 4
		return v
	}
	readF64 := func() float64 {
		if pos+8 > len(buf) {
			t.Fatalf("truncated f64 at %d of %d", pos, len(buf))
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(buf[pos : pos+8]))
		pos += 8
		return v
	}
	for pos < len(buf) {
		op := buf[pos]
		pos++
		if op != opStickyCells {
			t.Fatalf("unexpected op %d at %d, want opStickyCells (%d)", op, pos-1, opStickyCells)
		}
		scrollID := driver.NodeID(readU32())
		n := int(readU32())
		cells := make([]watchStickyCell, n)
		for i := range n {
			cells[i] = watchStickyCell{
				cell:  driver.NodeID(readU32()),
				start: readF64(),
				end:   readF64(),
			}
		}
		ops = append(ops, decodedStickyOp{scrollID: scrollID, cells: cells})
	}
	return ops
}

func TestWatchStickyCellsWireFormatAndClear(t *testing.T) {
	var tracker stickyTracker
	var w writer

	inf := driver.Points(math.Inf(1))
	cells := []driver.StickyCell{
		{Cell: 10, Start: 40, End: 240},
		{Cell: 11, Start: 240, End: inf},
		{Cell: 12, Start: 840, End: -160},
	}
	if !tracker.set(5, cells, driver.Point{Y: 100}, false, nil, &w) {
		t.Fatal("initial set reported no change")
	}
	ops := decodeStickyOps(t, w.buf)
	if len(ops) != 1 || ops[0].scrollID != 5 || len(ops[0].cells) != 3 {
		t.Fatalf("decoded = %+v, want 1 op for scroll 5 with 3 cells", ops)
	}
	got := ops[0].cells
	if got[0] != (watchStickyCell{cell: 10, start: 40, end: 240}) {
		t.Fatalf("cell 0 = %+v", got[0])
	}
	if got[1].cell != 11 || got[1].start != 240 || !math.IsInf(got[1].end, 1) {
		t.Fatalf("cell 1 = %+v, want infinite end", got[1])
	}
	if got[2] != (watchStickyCell{cell: 12, start: 840, end: -160}) {
		t.Fatalf("cell 2 = %+v", got[2])
	}

	// Re-sending identical ranges emits no redundant wire op.
	w.buf = w.buf[:0]
	tracker.set(5, cells, driver.Point{Y: 100}, false, nil, &w)
	if len(w.buf) != 0 {
		t.Fatalf("identical set wrote %d bytes, want 0", len(w.buf))
	}

	// Clearing emits an empty opStickyCells and unpins all cells.
	if !tracker.set(5, nil, driver.Point{Y: 100}, false, nil, &w) {
		t.Fatal("clear reported no change")
	}
	ops = decodeStickyOps(t, w.buf)
	if len(ops) != 1 || ops[0].scrollID != 5 || len(ops[0].cells) != 0 {
		t.Fatalf("clear decoded = %+v, want 1 empty op for scroll 5", ops)
	}
	for _, id := range []driver.NodeID{10, 11, 12} {
		if tracker.isSticky(id) || tracker.shift(id) != (driver.Point{}) {
			t.Fatalf("cell %d stayed sticky after clear: sticky=%v shift=%v", id, tracker.isSticky(id), tracker.shift(id))
		}
	}

	// Clearing again when already empty is a no-op.
	w.buf = w.buf[:0]
	if tracker.set(5, nil, driver.Point{Y: 100}, false, nil, &w) {
		t.Fatal("second clear reported change")
	}
	if len(w.buf) != 0 {
		t.Fatalf("second clear wrote %d bytes, want 0", len(w.buf))
	}
}

func TestWatchStickyShiftParityWithDriver(t *testing.T) {
	inf := driver.Points(math.Inf(1))
	cases := []struct {
		name    string
		cell    driver.StickyCell
		offsets []driver.Points
	}{
		{"start finite", driver.StickyCell{Cell: 1, Start: 100, End: 300}, []driver.Points{-40, 0, 100, 150, 300, 450}},
		{"start infinite", driver.StickyCell{Cell: 2, Start: 50, End: inf}, []driver.Points{-20, 0, 50, 125, 2500}},
		{"end pinned", driver.StickyCell{Cell: 3, Start: 840, End: -160}, []driver.Points{-500, -160, 0, 500, 840, 1000}},
		{"short end pinned", driver.StickyCell{Cell: 4, Start: -70, End: -170}, []driver.Points{-200, -170, -70, 0, 100}},
		{"no travel", driver.StickyCell{Cell: 5, Start: 100, End: 100}, []driver.Points{0, 100, 200}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved := resolveStickyCells([]driver.StickyCell{tc.cell}, nil)
			if len(resolved) != 1 {
				t.Fatalf("resolved len = %d", len(resolved))
			}
			for _, horizontal := range []bool{false, true} {
				var tracker stickyTracker
				tracker.set(99, []driver.StickyCell{tc.cell}, driver.Point{}, horizontal, nil, nil)
				for _, off := range tc.offsets {
					var pt driver.Point
					if horizontal {
						pt.X = off
					} else {
						pt.Y = off
					}
					tracker.pin(99, pt, horizontal, nil)
					want := tc.cell.StickyShift(off)
					hostShift := driver.Points(resolved[0].shift(float64(off)))
					if hostShift != want {
						t.Errorf("horizontal=%v offset=%v: host shift=%v, want %v", horizontal, off, hostShift, want)
					}
					got := tracker.shift(tc.cell.Cell)
					wantPt := driver.Point{Y: want}
					if horizontal {
						wantPt = driver.Point{X: want}
					}
					if got != wantPt {
						t.Errorf("horizontal=%v offset=%v: tracker shift=%v, want %v", horizontal, off, got, wantPt)
					}
				}
			}
		})
	}
}

func TestWatchStickyAnimatedInsetResolution(t *testing.T) {
	anim := driver.NewAnimator(func(driver.Event) {})
	anim.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 7, Value: 30}})

	cell := driver.StickyCell{Cell: 20, Start: 100, End: 300, Node: 7, Inset: 10}
	var tracker stickyTracker
	var w writer
	scrollOffset := driver.Point{Y: 120}

	tracker.set(1, []driver.StickyCell{cell}, scrollOffset, false, anim, &w)
	ops := decodeStickyOps(t, w.buf)
	if len(ops) != 1 || len(ops[0].cells) != 1 {
		t.Fatalf("initial ops = %+v", ops)
	}
	// Delta is 30 - 10 = 20, so effective start/end shift by -20 to [80, 280].
	if ops[0].cells[0] != (watchStickyCell{cell: 20, start: 80, end: 280}) {
		t.Fatalf("resolved = %+v, want [80, 280]", ops[0].cells[0])
	}
	wantShift := anim.StickyShift(cell, scrollOffset.Y)
	if got := driver.Points(ops[0].cells[0].shift(float64(scrollOffset.Y))); got != wantShift {
		t.Fatalf("host shift = %v, want %v", got, wantShift)
	}
	if got := tracker.shift(20).Y; got != wantShift {
		t.Fatalf("tracker shift = %v, want %v", got, wantShift)
	}

	// Repin with unchanged animated value emits no wire op.
	w.buf = w.buf[:0]
	lookup := func(id driver.NodeID) (driver.Point, bool, bool) {
		if id == 1 {
			return scrollOffset, false, true
		}
		return driver.Point{}, false, false
	}
	if tracker.repinBound(lookup, anim, &w) {
		t.Fatal("repinBound reported change when animated value was unchanged")
	}
	if len(w.buf) != 0 {
		t.Fatalf("unchanged repinBound wrote %d bytes, want 0", len(w.buf))
	}

	// Updating animated value re-resolves ranges, shifts Go state and sends opStickyCells.
	anim.Apply([]driver.AnimationOp{{Kind: driver.AnimSet, Node: 7, Value: 50}})
	if !tracker.repinBound(lookup, anim, &w) {
		t.Fatal("repinBound reported no change after animated value update")
	}
	ops = decodeStickyOps(t, w.buf)
	if len(ops) != 1 || ops[0].cells[0] != (watchStickyCell{cell: 20, start: 60, end: 260}) {
		t.Fatalf("updated ops = %+v, want [60, 260]", ops)
	}
	wantShift = anim.StickyShift(cell, scrollOffset.Y)
	if got := tracker.shift(20).Y; got != wantShift {
		t.Fatalf("updated tracker shift = %v, want %v", got, wantShift)
	}
	if got := driver.Points(ops[0].cells[0].shift(float64(scrollOffset.Y))); got != wantShift {
		t.Fatalf("updated host shift = %v, want %v", got, wantShift)
	}
}

func TestWatchStickyHitTestingAndZOrder(t *testing.T) {
	type node struct {
		id       driver.NodeID
		parent   *node
		children []*node
		frame    driver.Bounds
		offset   driver.Point
		scroll   bool
	}
	var tracker stickyTracker

	scroll := &node{
		id:     1,
		frame:  driver.Bounds{Size: driver.Size{Width: 180, Height: 200}},
		scroll: true,
	}
	header := &node{
		id:     2,
		parent: scroll,
		frame:  driver.Bounds{Origin: driver.Point{Y: 40}, Size: driver.Size{Width: 180, Height: 30}},
	}
	headerBtn := &node{
		id:     3,
		parent: header,
		frame:  driver.Bounds{Origin: driver.Point{X: 10, Y: 5}, Size: driver.Size{Width: 60, Height: 20}},
	}
	header.children = []*node{headerBtn}

	// Later non-sticky sibling that scrolls underneath the pinned header at offset Y=120.
	row := &node{
		id:     4,
		parent: scroll,
		frame:  driver.Bounds{Origin: driver.Point{Y: 120}, Size: driver.Size{Width: 180, Height: 40}},
	}
	// End-pinned sticky footer laid out at Y=500, pinned to bottom of 200pt viewport (Y=170..200).
	footer := &node{
		id:     5,
		parent: scroll,
		frame:  driver.Bounds{Origin: driver.Point{Y: 500}, Size: driver.Size{Width: 180, Height: 30}},
	}
	scroll.children = []*node{header, row, footer}

	var hitTree func(n *node, p driver.Point) (driver.NodeID, driver.Point)
	hitTree = func(n *node, p driver.Point) (driver.NodeID, driver.Point) {
		f := placedBounds(n.frame, tracker.shift(n.id))
		local, ok := driver.IntoView(f, driver.Identity3D, p)
		if !ok {
			return 0, driver.Point{}
		}
		if n.scroll && (local.X < 0 || local.Y < 0 || local.X >= f.Size.Width || local.Y >= f.Size.Height) {
			return 0, driver.Point{}
		}
		childPt := driver.Point{X: local.X + n.offset.X, Y: local.Y + n.offset.Y}
		var hitID driver.NodeID
		var hitLocal driver.Point
		hitChildOrder(n.children, func(c *node) bool { return tracker.isSticky(c.id) }, func(c *node) bool {
			if id, pt := hitTree(c, childPt); id != 0 {
				hitID, hitLocal = id, pt
				return true
			}
			return false
		})
		if hitID != 0 {
			return hitID, hitLocal
		}
		if local.X >= 0 && local.Y >= 0 && local.X < f.Size.Width && local.Y < f.Size.Height {
			return n.id, local
		}
		return 0, driver.Point{}
	}

	// Header pins from Y=40 to Y=300; footer pins to viewport bottom (500+30-200 = 330 down to 330-500 = -170).
	tracker.set(scroll.id, []driver.StickyCell{
		{Cell: header.id, Start: 40, End: 300},
		{Cell: footer.id, Start: 330, End: -170},
	}, scroll.offset, false, nil, nil)

	// At offset 0: header is at Y=40..70, footer is pinned at Y=170..200 (shift -330).
	if id, pt := hitTree(scroll, driver.Point{X: 25, Y: 55}); id != headerBtn.id || !nearPoint(pt, driver.Point{X: 15, Y: 10}) {
		t.Fatalf("offset 0 header hit = (%d, %v), want (%d, (15, 10))", id, pt, headerBtn.id)
	}
	if id, pt := hitTree(scroll, driver.Point{X: 40, Y: 185}); id != footer.id || !nearPoint(pt, driver.Point{X: 40, Y: 15}) {
		t.Fatalf("offset 0 footer hit = (%d, %v), want (%d, (40, 15))", id, pt, footer.id)
	}

	// Scroll to Y=120: row sits at viewport Y=0..40, but pinned header (shift +80 -> Y=120..150, viewport Y=0..30)
	// sits ON TOP of row and must receive the tap at viewport Y=15.
	scroll.offset = driver.Point{Y: 120}
	tracker.pin(scroll.id, scroll.offset, false, nil)
	if id, pt := hitTree(scroll, driver.Point{X: 25, Y: 15}); id != headerBtn.id || !nearPoint(pt, driver.Point{X: 15, Y: 10}) {
		t.Fatalf("offset 120 pinned header hit = (%d, %v), want (%d, (15, 10))", id, pt, headerBtn.id)
	}
	// Just below the pinned header (viewport Y=35), the underlying row receives the tap.
	if id, pt := hitTree(scroll, driver.Point{X: 25, Y: 35}); id != row.id || !nearPoint(pt, driver.Point{X: 25, Y: 35}) {
		t.Fatalf("offset 120 row below header hit = (%d, %v), want (%d, (25, 35))", id, pt, row.id)
	}

	// Scroll past header's End (Y=350): header clamps at shift 260 (content Y=300..330, scrolled above viewport).
	scroll.offset = driver.Point{Y: 350}
	tracker.pin(scroll.id, scroll.offset, false, nil)
	if id, _ := hitTree(scroll, driver.Point{X: 25, Y: 15}); id != scroll.id {
		t.Fatalf("offset 350 past travel hit = %d, want scroll (%d)", id, scroll.id)
	}
	// Footer at offset 350 has passed Start=330 so shift is 0 (content Y=500..530 -> viewport Y=150..180).
	if id, pt := hitTree(scroll, driver.Point{X: 40, Y: 165}); id != footer.id || !nearPoint(pt, driver.Point{X: 40, Y: 15}) {
		t.Fatalf("offset 350 unpinned footer hit = (%d, %v), want (%d, (40, 15))", id, pt, footer.id)
	}

	// Clearing sticky cells restores layout coordinates and non-sticky z-order.
	scroll.offset = driver.Point{Y: 120}
	tracker.set(scroll.id, nil, scroll.offset, false, nil, nil)
	if id, _ := hitTree(scroll, driver.Point{X: 25, Y: 15}); id != row.id {
		t.Fatalf("after clear hit at Y=15 = %d, want row (%d)", id, row.id)
	}
}

func TestWatchStickyLifecycleOwnershipAndDelete(t *testing.T) {
	var tracker stickyTracker
	var w writer

	// Cell 10 starts in scroll 1, then moves to scroll 2 before scroll 1 clears/deletes.
	tracker.set(1, []driver.StickyCell{{Cell: 10, Start: 0, End: 100}}, driver.Point{Y: 40}, false, nil, &w)
	tracker.set(2, []driver.StickyCell{{Cell: 10, Start: 0, End: 200}}, driver.Point{Y: 80}, false, nil, &w)
	if got := tracker.shift(10); got != (driver.Point{Y: 80}) {
		t.Fatalf("cell 10 after move to scroll 2: shift=%v, want (0, 80)", got)
	}

	// Deleting scroll 1 must not clobber cell 10 owned by scroll 2.
	tracker.delete(1)
	if !tracker.isSticky(10) || tracker.shift(10) != (driver.Point{Y: 80}) {
		t.Fatalf("deleting old scroll 1 clobbered cell 10: sticky=%v shift=%v", tracker.isSticky(10), tracker.shift(10))
	}

	// Switching scroll 2 to horizontal re-pins along X.
	tracker.pin(2, driver.Point{X: 60, Y: 80}, true, nil)
	if got := tracker.shift(10); got != (driver.Point{X: 60}) {
		t.Fatalf("horizontal shift = %v, want (60, 0)", got)
	}

	// Deleting scroll 2 unpins cell 10.
	if !tracker.delete(2) {
		t.Fatal("deleting scroll 2 reported no change")
	}
	if tracker.isSticky(10) || tracker.shift(10) != (driver.Point{}) {
		t.Fatalf("after deleting scroll 2: sticky=%v shift=%v", tracker.isSticky(10), tracker.shift(10))
	}
}
