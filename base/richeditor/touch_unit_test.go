package richeditor

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestTouchHelpers(t *testing.T) {
	// sweepSelection extends by whole words forward, backward, and stays inside pressed word.
	if a, h := sweepSelection(6, 11, 12, 15); a != 6 || h != 15 {
		t.Fatalf("forward sweep = (%d, %d), want (6, 15)", a, h)
	}
	if a, h := sweepSelection(6, 11, 0, 5); a != 11 || h != 0 {
		t.Fatalf("backward sweep = (%d, %d), want (11, 0)", a, h)
	}
	if a, h := sweepSelection(6, 11, 6, 11); a != 6 || h != 11 {
		t.Fatalf("same-word sweep = (%d, %d), want (6, 11)", a, h)
	}

	// handleSelection refuses collapsing onto fixed end and crosses cleanly.
	if _, _, ok := handleSelection(5, 5); ok {
		t.Fatal("handleSelection(5, 5) should refuse empty collapse")
	}
	if a, h, ok := handleSelection(5, 9); !ok || a != 5 || h != 9 {
		t.Fatalf("handleSelection(5, 9) = (%d, %d, %v)", a, h, ok)
	}
	if a, h, ok := handleSelection(5, 2); !ok || a != 5 || h != 2 {
		t.Fatalf("handleSelection(5, 2) = (%d, %d, %v)", a, h, ok)
	}

	// handleBox puts knob room above start and below end, centered horizontally.
	caret := Rect2{X: 100, Y: 40, Width: 0, Height: 20}
	start := handleBox(edgeStart, caret)
	if start != (Rect2{X: 100 - handleWidth/2, Y: 40 - knobRoom, Width: handleWidth, Height: 20 + knobRoom}) {
		t.Fatalf("start handleBox = %+v", start)
	}
	end := handleBox(edgeEnd, caret)
	if end != (Rect2{X: 100 - handleWidth/2, Y: 40, Width: handleWidth, Height: 20 + knobRoom}) {
		t.Fatalf("end handleBox = %+v", end)
	}

	// menuBoundsAroundHandles leaves knobRoom clear above and below.
	sel := Rect2{X: 10, Y: 40, Width: 80, Height: 20}
	grown := menuBoundsAroundHandles(sel)
	if grown != (Rect2{X: 10, Y: 40 - knobRoom, Width: 80, Height: 20 + knobRoom*2}) {
		t.Fatalf("menuBoundsAroundHandles = %+v", grown)
	}

	// menuActions matches native input rules.
	if got := menuActions(true, true, false); got != core.EditCut|core.EditCopy|core.EditPaste|core.EditSelectAll {
		t.Fatalf("editable selection = %d", got)
	}
	if got := menuActions(false, true, false); got != core.EditPaste|core.EditSelectAll {
		t.Fatalf("editable caret = %d", got)
	}
	if got := menuActions(true, false, false); got != core.EditCopy|core.EditSelectAll {
		t.Fatalf("readonly selection = %d", got)
	}
	if got := menuActions(false, false, true); got != 0 {
		t.Fatalf("readonly empty = %d", got)
	}
	if got := menuActions(true, true, true); got != core.EditCut|core.EditCopy|core.EditPaste {
		t.Fatalf("all selected = %d", got)
	}
}
