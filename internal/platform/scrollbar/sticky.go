package scrollbar

import "github.com/nsgo-dev/nsgo/internal/driver"

// StickyCells holds the cells pinned by one scroll view.
type StickyCells []driver.StickyCell

func (cells StickyCells) unpin(place func(driver.NodeID, driver.Point)) {
	for _, cell := range cells {
		place(cell.Cell, driver.Point{})
	}
}

// Pin places the cells along the scroll view's main axis at offset.
func (cells StickyCells) Pin(offset driver.Point, horizontal bool, animator *driver.Animator, place func(driver.NodeID, driver.Point)) {
	for _, cell := range cells {
		if horizontal {
			place(cell.Cell, driver.Point{X: animator.StickyShift(cell, offset.X)})
		} else {
			place(cell.Cell, driver.Point{Y: animator.StickyShift(cell, offset.Y)})
		}
	}
}

// StickyBindings tracks scroll views with sticky cells whose insets are
// animated. Tracking the view as well as its ID detects replacement views.
type StickyBindings[V comparable] map[V]driver.NodeID

// Replace restores the previous cells, tracks the new bindings and pins the new cells.
func (bound *StickyBindings[V]) Replace(v V, id driver.NodeID, current *StickyCells, cells StickyCells, place func(driver.NodeID, driver.Point), pin func(V)) {
	current.unpin(place)
	*current = cells
	bound.update(v, id, cells)
	pin(v)
}

func (bound *StickyBindings[V]) update(v V, id driver.NodeID, cells StickyCells) {
	for _, cell := range cells {
		if cell.Node != 0 {
			if *bound == nil {
				*bound = make(StickyBindings[V])
			}
			(*bound)[v] = id
			return
		}
	}
	delete(*bound, v)
}

// Repin replaces each tracked view's current shifts and drops views that no
// longer occupy their IDs.
func (bound StickyBindings[V]) Repin(lookup func(driver.NodeID) V, pin func(V)) {
	for view, id := range bound {
		if lookup(id) == view {
			pin(view)
		} else {
			delete(bound, view)
		}
	}
}
