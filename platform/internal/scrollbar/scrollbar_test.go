package scrollbar

import (
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
)

func TestInsetsScrollWithTheContent(t *testing.T) {
	c := &core.ScrollConfig{Behavior: core.ScrollBehavior{ContentInset: core.Edges{Top: 20, Bottom: 30}}}
	frame, content := core.Size{Width: 100, Height: 200}, core.Size{Width: 100, Height: 800}
	if got := Clamp(c, frame, content, core.Point{Y: -50}); got.Y != -20 {
		t.Fatalf("clamped the start to %v, want -20", got.Y)
	}
	if got := Clamp(c, frame, content, core.Point{Y: 900}); got.Y != 630 {
		t.Fatalf("clamped the end to %v, want 630", got.Y)
	}
	// The thumb spans the content and its insets: 850 points.
	b := Bar{Frame: frame, Content: content, Offset: core.Point{Y: -20}, Inset: c.Behavior.ContentInset}
	if thumb, _ := b.Thumb(); thumb.Origin.Y != 0 {
		t.Fatalf("thumb at the inset's start at %v", thumb.Origin.Y)
	}
	if got := b.Press(core.Point{X: 95, Y: 1}).Offset(core.Point{X: 95, Y: 1000}); got.Y != 630 {
		t.Fatalf("dragging to the end scrolls to %v, want 630", got.Y)
	}
}

func TestWheelsSettleOnSnapPoints(t *testing.T) {
	c := &core.ScrollConfig{Behavior: core.ScrollBehavior{Paging: true}}
	frame, content := core.Size{Width: 100, Height: 200}, core.Size{Width: 100, Height: 1000}
	var w Wheel
	now := time.Now()
	w.Turn(now, 48)
	w.Turn(now.Add(50*time.Millisecond), 48)
	if _, ok := w.Settled(now.Add(100 * time.Millisecond)); ok {
		t.Fatal("settled while turning")
	}
	v, ok := w.Settled(now.Add(250 * time.Millisecond))
	if !ok || v <= 0 || w.Turning() {
		t.Fatalf("settled %v %v", v, ok)
	}
	// A wheel turned down moves on to the next page.
	if to, ok := Settle(c, frame, content, core.Point{Y: 96}, v); !ok || to.Y != 200 {
		t.Fatalf("settled at %v %v, want 200", to, ok)
	}
	// A released thumb settles at the nearest.
	if to, _ := Settle(c, frame, content, core.Point{Y: 260}, 0); to.Y != 200 {
		t.Fatalf("released at %v, want 200", to)
	}
	if _, ok := Settle(&core.ScrollConfig{}, frame, content, core.Point{Y: 96}, v); ok {
		t.Fatal("a free view snapped")
	}
}

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
