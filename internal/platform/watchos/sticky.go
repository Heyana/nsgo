package watchos

import (
	"math"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

// watchStickyCell is one sticky range resolved for Host.swift's opStickyCells:
// the cell's NodeID and its effective main-axis travel [start, end], already
// shifted for any bound animated StickyInset node so SwiftUI pins live during
// native scrolling without waiting for Go commits.
type watchStickyCell struct {
	cell       driver.NodeID
	start, end float64
}

// resolveStickyCells resolves a scroll view's sticky cells against anim so
// Host.swift receives each cell's current effective [start, end] travel.
func resolveStickyCells(cells []driver.StickyCell, anim *driver.Animator) []watchStickyCell {
	if len(cells) == 0 {
		return nil
	}
	out := make([]watchStickyCell, len(cells))
	for i, c := range cells {
		start, end := float64(c.Start), float64(c.End)
		if c.Node != 0 && anim != nil {
			if v, _, ok := anim.Value(c.Node); ok {
				delta := float64(driver.Points(v) - c.Inset)
				start -= delta
				end -= delta
			}
		}
		out[i] = watchStickyCell{cell: c.Cell, start: start, end: end}
	}
	return out
}

// shift computes the cell's main-axis sticky shift at offset, matching
// driver.StickyCell.StickyShift and Host.swift's StickyCell.shift(at:).
func (c watchStickyCell) shift(offset float64) float64 {
	travel := c.end - c.start
	if math.IsNaN(travel) {
		return 0
	}
	return min(max(offset-c.start, min(travel, 0)), max(travel, 0))
}

// stickyCells encodes opStickyCells for scroll view id: id (u32), count (u32),
// then count tuples of (cell u32, start f64, end f64).
func (w *writer) stickyCells(id driver.NodeID, cells []watchStickyCell) {
	w.op(opStickyCells, id)
	w.u32(uint32(len(cells)))
	for _, c := range cells {
		w.u32(uint32(c.cell))
		w.f64(c.start, c.end)
	}
}

// placedBounds shifts frame by a view's sticky offset so hit-testing, culling
// and DevTools geometry observe the cell's pinned visual position.
func placedBounds(frame driver.Bounds, shift driver.Point) driver.Bounds {
	frame.Origin.X += shift.X
	frame.Origin.Y += shift.Y
	return frame
}

// hitChildOrder visits children in front-to-back order matching Host.swift's
// z-ordering: sticky children (.zIndex(1)) in reverse order before non-sticky
// siblings (.zIndex(0)) in reverse order.
func hitChildOrder[T any](children []T, isSticky func(T) bool, visit func(T) bool) {
	for i := len(children) - 1; i >= 0; i-- {
		if isSticky(children[i]) && visit(children[i]) {
			return
		}
	}
	for i := len(children) - 1; i >= 0; i-- {
		if !isSticky(children[i]) && visit(children[i]) {
			return
		}
	}
}

type watchStickyScroll struct {
	cells scrollbar.StickyCells
	sent  []watchStickyCell
}

type watchStickyPin struct {
	owner driver.NodeID
	shift driver.Point
}

// stickyTracker keeps Go's view of sticky cells in sync with Host.swift for
// hit-testing, culling, DevTools geometry and animated StickyInset updates.
type stickyTracker struct {
	scrolls map[driver.NodeID]*watchStickyScroll
	cells   map[driver.NodeID]watchStickyPin
	bound   scrollbar.StickyBindings[driver.NodeID]
}

// set replaces scrollID's sticky cells, updates Go shifts and sticky flags,
// tracks animated StickyInset bindings and writes opStickyCells when the
// resolved ranges changed. It reports whether any cell's sticky state or shift
// changed.
func (t *stickyTracker) set(scrollID driver.NodeID, cells []driver.StickyCell, offset driver.Point, horizontal bool, anim *driver.Animator, w *writer) bool {
	st := t.scrolls[scrollID]
	if st == nil {
		if len(cells) == 0 {
			return false
		}
		if t.scrolls == nil {
			t.scrolls = map[driver.NodeID]*watchStickyScroll{}
		}
		st = &watchStickyScroll{}
		t.scrolls[scrollID] = st
	}
	changed := false
	next := slices.Clone(cells)
	t.bound.Replace(scrollID, scrollID, &st.cells, next, func(id driver.NodeID, shift driver.Point) {
		if shift == (driver.Point{}) {
			if cur, ok := t.cells[id]; ok && cur.owner == scrollID {
				delete(t.cells, id)
				changed = true
			}
			return
		}
		if t.setPin(scrollID, id, shift) {
			changed = true
		}
	}, func(id driver.NodeID) {
		if t.pinEntry(id, st, offset, horizontal, anim) {
			changed = true
		}
	})
	resolved := resolveStickyCells(st.cells, anim)
	if !slices.Equal(resolved, st.sent) {
		st.sent = resolved
		if w != nil {
			w.stickyCells(scrollID, resolved)
		}
	}
	if len(st.cells) == 0 {
		delete(t.scrolls, scrollID)
	}
	return changed
}

// pin recomputes Go's shifts for scrollID at offset along its scroll axis.
func (t *stickyTracker) pin(scrollID driver.NodeID, offset driver.Point, horizontal bool, anim *driver.Animator) bool {
	st := t.scrolls[scrollID]
	if st == nil || len(st.cells) == 0 {
		return false
	}
	return t.pinEntry(scrollID, st, offset, horizontal, anim)
}

func (t *stickyTracker) pinEntry(scrollID driver.NodeID, st *watchStickyScroll, offset driver.Point, horizontal bool, anim *driver.Animator) bool {
	changed := false
	st.cells.Pin(offset, horizontal, anim, func(id driver.NodeID, shift driver.Point) {
		if t.setPin(scrollID, id, shift) {
			changed = true
		}
	})
	return changed
}

func (t *stickyTracker) setPin(owner, cell driver.NodeID, shift driver.Point) bool {
	if t.cells == nil {
		t.cells = map[driver.NodeID]watchStickyPin{}
	}
	next := watchStickyPin{owner: owner, shift: shift}
	if cur, ok := t.cells[cell]; ok && cur == next {
		return false
	}
	t.cells[cell] = next
	return true
}

// repinBound re-evaluates scroll views whose sticky cells follow animated
// StickyInset nodes, updating Go shifts and sending opStickyCells when resolved
// ranges changed.
func (t *stickyTracker) repinBound(lookup func(driver.NodeID) (offset driver.Point, horizontal bool, ok bool), anim *driver.Animator, w *writer) bool {
	if len(t.bound) == 0 {
		return false
	}
	changed := false
	t.bound.Repin(func(id driver.NodeID) driver.NodeID {
		if _, _, ok := lookup(id); ok && t.scrolls[id] != nil {
			return id
		}
		return 0
	}, func(id driver.NodeID) {
		st := t.scrolls[id]
		if st == nil {
			return
		}
		offset, horizontal, ok := lookup(id)
		if !ok {
			return
		}
		if t.pinEntry(id, st, offset, horizontal, anim) {
			changed = true
		}
		resolved := resolveStickyCells(st.cells, anim)
		if !slices.Equal(resolved, st.sent) {
			st.sent = resolved
			if w != nil {
				w.stickyCells(id, resolved)
			}
		}
	})
	return changed
}

// delete forgets sticky state for a deleted scroll view or sticky cell.
func (t *stickyTracker) delete(id driver.NodeID) bool {
	changed := false
	if st := t.scrolls[id]; st != nil {
		for _, cell := range st.cells {
			if cur, ok := t.cells[cell.Cell]; ok && cur.owner == id {
				delete(t.cells, cell.Cell)
				changed = true
			}
		}
		delete(t.scrolls, id)
		delete(t.bound, id)
	}
	if _, ok := t.cells[id]; ok {
		delete(t.cells, id)
		changed = true
	}
	return changed
}

func (t *stickyTracker) shift(id driver.NodeID) driver.Point {
	if t.cells == nil {
		return driver.Point{}
	}
	return t.cells[id].shift
}

func (t *stickyTracker) isSticky(id driver.NodeID) bool {
	if t.cells == nil {
		return false
	}
	_, ok := t.cells[id]
	return ok
}
