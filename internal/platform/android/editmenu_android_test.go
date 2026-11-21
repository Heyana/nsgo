//go:build android

package android

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestEditMenuEncodesBoundsAndActions checks that ItemEditMenu encodes ID,
// Frame and EditAction bits in the order NsgoView.java reads them.
func TestEditMenuEncodesBoundsAndActions(t *testing.T) {
	bounds := driver.Bounds{
		Origin: driver.Point{X: 12.5, Y: 34},
		Size:   driver.Size{Width: 120, Height: 48.5},
	}
	actions := uint32(driver.EditCut | driver.EditCopy | driver.EditPaste | driver.EditSelectAll)
	batch := &driver.MountBatch{
		Root: 1,
		Items: []driver.MountItem{
			{Kind: driver.ItemEditMenu, ID: 7, Frame: bounds, Count: actions},
			{Kind: driver.ItemEditMenu, ID: 7, Count: 0},
		},
	}
	var w writer
	encode(&w, batch, false, nil, nil, nil, nil)
	r := reader{w.buf}
	if root := r.u32(); root != 1 {
		t.Fatalf("root = %d, want 1", root)
	}
	if anim := r.byte(); anim != 0 {
		t.Fatalf("layout animation kind = %d, want 0", anim)
	}
	if first := r.byte(); first != 0 {
		t.Fatalf("first flag = %d, want 0", first)
	}
	if count := r.u32(); count != 2 {
		t.Fatalf("item count = %d, want 2", count)
	}
	if kind, id := driver.ItemKind(r.byte()), driver.NodeID(r.u32()); kind != driver.ItemEditMenu || id != 7 {
		t.Fatalf("item 1 head = (%v, %d), want (ItemEditMenu, 7)", kind, id)
	}
	gotBounds := driver.Bounds{
		Origin: driver.Point{X: r.points(), Y: r.points()},
		Size:   driver.Size{Width: r.points(), Height: r.points()},
	}
	gotActions := r.u32()
	if gotBounds != bounds || gotActions != actions {
		t.Fatalf("item 1 payload = (%+v, %d), want (%+v, %d)", gotBounds, gotActions, bounds, actions)
	}
	if kind, id := driver.ItemKind(r.byte()), driver.NodeID(r.u32()); kind != driver.ItemEditMenu || id != 7 {
		t.Fatalf("item 2 head = (%v, %d), want (ItemEditMenu, 7)", kind, id)
	}
	_ = r.points()
	_ = r.points()
	_ = r.points()
	_ = r.points()
	if gotClose := r.u32(); gotClose != 0 {
		t.Fatalf("item 2 actions = %d, want 0", gotClose)
	}
	if len(r.buf) != 0 {
		t.Fatalf("%d trailing bytes in encoded batch", len(r.buf))
	}
}
