package hover

import (
	"slices"
	"testing"

	"github.com/oneo-me/nsgo/core"
)

func TestMove(t *testing.T) {
	var tr Tracker
	ev := func(k core.EventKind, id core.NodeID) core.Event { return core.Event{Kind: k, ID: id} }
	steps := []struct {
		under []core.NodeID
		want  []core.Event
	}{
		{[]core.NodeID{1, 2}, []core.Event{ev(core.EventHoverIn, 1), ev(core.EventHoverIn, 2)}},
		{[]core.NodeID{1, 2}, nil},
		{[]core.NodeID{1, 3, 4}, []core.Event{ev(core.EventHoverOut, 2), ev(core.EventHoverIn, 3), ev(core.EventHoverIn, 4)}},
		{nil, []core.Event{ev(core.EventHoverOut, 4), ev(core.EventHoverOut, 3), ev(core.EventHoverOut, 1)}},
	}
	for i, s := range steps {
		if got := tr.Move(s.under); !same(got, s.want) {
			t.Fatalf("step %d: got %v, want %v", i, got, s.want)
		}
	}
	tr.Move([]core.NodeID{1, 2})
	tr.Forget(2)
	if got := tr.Move(nil); !same(got, []core.Event{ev(core.EventHoverOut, 1)}) {
		t.Fatalf("after Forget: got %v", got)
	}
}

func same(a, b []core.Event) bool {
	return slices.EqualFunc(a, b, func(x, y core.Event) bool { return x.Kind == y.Kind && x.ID == y.ID })
}
