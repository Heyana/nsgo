package scrollbar

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestStickyCellsFollowTheirAxisAndBoundInset(t *testing.T) {
	cells := StickyCells{
		{Cell: 1, Start: 10, End: 60, Node: 1, Inset: 5},
		{Cell: 2, Start: 30, End: 10},
	}
	animator := driver.NewAnimator(func(driver.Event) {})
	animator.Apply([]driver.AnimationOp{{Kind: driver.AnimCreateValue, Node: 1, Value: 25}})

	got := make(map[driver.NodeID]driver.Point)
	place := func(id driver.NodeID, shift driver.Point) { got[id] = shift }
	at := driver.Point{X: 48, Y: 3}
	cells.Pin(at, true, animator, place)
	if got[1] != (driver.Point{X: 50}) || got[2] != (driver.Point{}) {
		t.Fatalf("horizontal shifts = %v, want 50 and 0", got)
	}
	cells.Pin(at, false, animator, place)
	if got[1] != (driver.Point{Y: 13}) || got[2] != (driver.Point{Y: -20}) {
		t.Fatalf("vertical shifts = %v, want 13 and -20", got)
	}
	cells.unpin(place)
	if got[1] != (driver.Point{}) || got[2] != (driver.Point{}) {
		t.Fatalf("unpin shifts = %v, want both zero", got)
	}
}

func TestStickyBindingsDropRemovedAndReplacedViews(t *testing.T) {
	stale, current := new(int), new(int)
	const id driver.NodeID = 10
	cells := StickyCells{{Cell: 11, Node: 2}}

	var bound StickyBindings[*int]
	bound.update(stale, id, cells)
	bound.update(current, id, cells)
	lookup := func(driver.NodeID) *int { return current }
	var pinned []*int
	pin := func(v *int) { pinned = append(pinned, v) }
	bound.Repin(lookup, pin)
	if len(pinned) != 1 || pinned[0] != current || len(bound) != 1 {
		t.Fatalf("repin kept a replaced view: %d pins, %d tracked", len(pinned), len(bound))
	}
	bound.update(current, id, StickyCells{{Cell: 11}})
	bound.Repin(lookup, pin)
	if len(pinned) != 1 || len(bound) != 0 {
		t.Fatalf("repin kept a view whose inset is static: %d pins, %d tracked", len(pinned), len(bound))
	}
}

func TestReplacingStickyCellsRestoresAndPinsInOrder(t *testing.T) {
	view := new(int)
	var bound StickyBindings[*int]
	current := StickyCells{{Cell: 15}}
	next := StickyCells{{Cell: 16, Node: 3}}
	var restored, pinned bool
	bound.Replace(view, 14, &current, next, func(id driver.NodeID, shift driver.Point) {
		if id != 15 || shift != (driver.Point{}) {
			t.Errorf("restored cell %d at %v, want old cell 15 at its frame", id, shift)
		}
		restored = true
	}, func(v *int) {
		pinned = true
		if !restored || v != view || len(current) != 1 || current[0] != next[0] || bound[v] != 14 {
			t.Errorf("pin did not receive the replacement cells and binding after the old cell was restored")
		}
	})
	if !pinned {
		t.Fatal("replacement cells were not pinned")
	}
}
