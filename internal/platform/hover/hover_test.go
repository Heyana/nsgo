package hover

import (
	"slices"
	"testing"

	"github.com/oneo-me/nsgo/internal/driver"
)

func TestMove(t *testing.T) {
	var tr Tracker
	ev := func(k driver.EventKind, id driver.NodeID) driver.Event { return driver.Event{Kind: k, ID: id} }
	steps := []struct {
		under []driver.NodeID
		want  []driver.Event
	}{
		{[]driver.NodeID{1, 2}, []driver.Event{ev(driver.EventHoverIn, 1), ev(driver.EventHoverIn, 2)}},
		{[]driver.NodeID{1, 2}, nil},
		{[]driver.NodeID{1, 3, 4}, []driver.Event{ev(driver.EventHoverOut, 2), ev(driver.EventHoverIn, 3), ev(driver.EventHoverIn, 4)}},
		{nil, []driver.Event{ev(driver.EventHoverOut, 4), ev(driver.EventHoverOut, 3), ev(driver.EventHoverOut, 1)}},
	}
	for i, s := range steps {
		if got := tr.Move(s.under); !same(got, s.want) {
			t.Fatalf("step %d: got %v, want %v", i, got, s.want)
		}
	}
	tr.Move([]driver.NodeID{1, 2})
	tr.Forget(2)
	if got := tr.Move(nil); !same(got, []driver.Event{ev(driver.EventHoverOut, 1)}) {
		t.Fatalf("after Forget: got %v", got)
	}
}

func same(a, b []driver.Event) bool {
	return slices.EqualFunc(a, b, func(x, y driver.Event) bool { return x.Kind == y.Kind && x.ID == y.ID })
}
