package scrollbar

import (
	"testing"

	"github.com/oneo-me/nsgo/core"
)

func TestThumbFollowsTheOffset(t *testing.T) {
	b := Bar{Frame: core.Size{Width: 100, Height: 200}, Content: core.Size{Width: 100, Height: 800}}
	thumb, ok := b.Thumb()
	want := core.Bounds{Origin: core.Point{X: 92}, Size: core.Size{Width: Thickness, Height: 50}}
	if !ok || thumb != want {
		t.Fatalf("thumb = %v %v, want %v", thumb, ok, want)
	}
	b.Offset.Y = 600
	if thumb, _ := b.Thumb(); thumb.Origin.Y != 150 {
		t.Fatalf("thumb at the end starts at %v, want 150", thumb.Origin.Y)
	}
	if _, ok := (Bar{Frame: b.Frame, Content: b.Frame}).Thumb(); ok {
		t.Fatal("content that fits has a thumb")
	}
}

func TestContainsTheStripAlongTheEdge(t *testing.T) {
	b := Bar{Horizontal: true, Frame: core.Size{Width: 200, Height: 100}, Content: core.Size{Width: 400, Height: 100}}
	if !b.Contains(core.Point{X: 150, Y: 95}) || b.Contains(core.Point{X: 150, Y: 50}) || b.Contains(core.Point{X: 250, Y: 95}) {
		t.Fatal("the bar's strip is not its bottom edge")
	}
}

func TestDragMovesTheContentByTheThumb(t *testing.T) {
	b := Bar{Frame: core.Size{Width: 100, Height: 200}, Content: core.Size{Width: 100, Height: 800}}
	d := b.Press(core.Point{X: 95, Y: 10}) // on the thumb, 0 to 50
	// The thumb travels 150 for 600 of content.
	if got := d.Offset(core.Point{X: 95, Y: 40}); got.Y != 120 {
		t.Fatalf("dragging 30 scrolls to %v, want 120", got.Y)
	}
	if got := d.Offset(core.Point{X: 95, Y: 1000}); got.Y != 600 {
		t.Fatalf("dragging past the end scrolls to %v, want 600", got.Y)
	}
	// Off the thumb, it centers under the pointer.
	d = b.Press(core.Point{X: 95, Y: 100})
	if got := d.Offset(core.Point{X: 95, Y: 100}); got.Y != 300 {
		t.Fatalf("pressing the middle scrolls to %v, want 300", got.Y)
	}
}
