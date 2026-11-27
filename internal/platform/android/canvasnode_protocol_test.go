package android

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestCanvasNodeWireAndActionDecode(t *testing.T) {
	d := driver.NewDrawing(nil).Node(driver.CanvasNode{
		ID:        "vol",
		Bounds:    driver.Bounds{Origin: driver.Point{X: 10, Y: 20}, Size: driver.Size{Width: 120, Height: 30}},
		Role:      driver.RoleAdjustable,
		Label:     "Volume",
		Hint:      "Adjust volume",
		TestID:    "vol-test",
		Value:     45,
		Min:       0,
		Max:       100,
		Step:      5,
		ValueText: "45%",
		Selected:  true,
		Disabled:  false,
		Busy:      true,
		Expanded:  driver.ExpandedTrue,
		Checked:   driver.CheckedMixed,
		Live:      driver.LiveRegionAssertive,
		Actions:   driver.CanvasActionIncrement | driver.CanvasActionDecrement | driver.CanvasActionScroll,
	})
	var w writer
	writeDrawing(&w, d)
	r := reader{buf: w.buf}
	opsLen := int(r.u32())
	r.buf = r.buf[opsLen:]
	if slots := r.u32(); slots != 0 {
		t.Fatalf("slots = %d, want 0", slots)
	}
	if count := r.u32(); count != 1 {
		t.Fatalf("nodes = %d, want 1", count)
	}
	x, y, bw, bh := r.points(), r.points(), r.points(), r.points()
	role := driver.Role(r.byte())
	id, label, hint, testID := r.str(), r.str(), r.str(), r.str()
	val, lo, hi, step := r.float(), r.float(), r.float(), r.float()
	valText := r.str()
	flags := r.byte()
	expanded := driver.ExpandedState(r.byte())
	checked := driver.CheckedState(r.byte())
	live := driver.LiveRegion(r.byte())
	actions := driver.CanvasAction(r.byte())
	res, nearest := r.float(), r.byte()
	if len(r.buf) != 0 {
		t.Fatalf("%d trailing bytes", len(r.buf))
	}
	if x != 10 || y != 20 || bw != 120 || bh != 30 || role != driver.RoleAdjustable ||
		id != "vol" || label != "Volume" || hint != "Adjust volume" || testID != "vol-test" ||
		val != 45 || lo != 0 || hi != 100 || step != 5 || valText != "45%" ||
		flags != 1|4 || expanded != driver.ExpandedTrue || checked != driver.CheckedMixed ||
		live != driver.LiveRegionAssertive || actions != driver.CanvasActionIncrement|driver.CanvasActionDecrement|driver.CanvasActionScroll ||
		res != 1 || nearest != 0 {
		t.Fatalf("decoded node mismatch: role=%v id=%q label=%q hint=%q testID=%q val=%v[%v..%v/%v] text=%q flags=%b exp=%v chk=%v live=%v act=%b",
			role, id, label, hint, testID, val, lo, hi, step, valText, flags, expanded, checked, live, actions)
	}

	// Verify readEvent decodes CanvasNode, Value and HasValue on EventAccessibilityAction.
	var ew writer
	ew.u32(0) // surface
	ew.byte(byte(driver.EventAccessibilityAction))
	ew.u32(9) // canvas view ID
	ew.u32(0) // count
	ew.points(0)
	ew.points(0)
	ew.str(driver.ActionIncrement) // text
	ew.points(0)
	ew.points(0)
	ew.u32(0) // index
	ew.byte(0)
	ew.points(0)
	ew.points(0)
	ew.float(60) // value
	ew.byte(0)   // no navigation
	ew.u64(0)
	ew.u64(0)
	ew.byte(0)    // buffering
	ew.str("vol") // data -> CanvasNode
	ew.byte(1)    // finished -> HasValue
	ew.points(0)
	ew.points(0)
	ew.points(0)
	ew.points(0)
	ew.byte(0) // mods
	ew.byte(0) // button
	er := reader{buf: ew.buf}
	_, ev, trailing := readEvent(&er)
	if ev.Kind != driver.EventAccessibilityAction || ev.ID != 9 || ev.Text != driver.ActionIncrement ||
		ev.CanvasNode != "vol" || ev.Value != 60 || !ev.HasValue || ev.Finished || trailing != "" {
		t.Fatalf("decoded event = %+v trailing=%q", ev, trailing)
	}
}
