package scrollbar

import (
	"testing"
	"time"

	"github.com/oneo-me/nsgo/internal/driver"
)

func TestTouchDragClaimsAlongTheAxisAndGlides(t *testing.T) {
	c := &driver.ScrollConfig{Behavior: driver.ScrollBehavior{}}
	frame, content := driver.Size{Width: 100, Height: 100}, driver.Size{Width: 100, Height: 1000}
	now := time.Unix(0, 0)
	d := NewTouchDrag(driver.Point{X: 50, Y: 80}, 0, now)
	if d.Claims(c, driver.Point{X: 70, Y: 75}) {
		t.Error("a sideways move claims a vertical view")
	}
	if !d.Claims(c, driver.Point{X: 50, Y: 60}) {
		t.Fatal("a move past the slop does not claim")
	}
	for i := range 6 {
		now = now.Add(10 * time.Millisecond)
		raw := d.Move(c, driver.Point{X: 50, Y: 60 - driver.Points(i+1)*10}, now)
		if want := driver.Points(i+1) * 10; raw != want {
			t.Fatalf("move %d: offset %v, want %v", i, raw, want)
		}
	}
	v := d.Velocity(c)
	if v < 0.9 || v > 1.1 {
		t.Fatalf("velocity %v, want 1 point/ms", v)
	}
	lo, hi := Limits(c, frame, content, false)
	m := Release(c, frame, content, driver.Point{Y: 60}, v, lo, hi, now)
	if !m.glide || m.to <= 60 {
		t.Fatalf("release = %+v, want a glide down the content", m)
	}
	at, done := m.At(now.Add(m.duration))
	if !done || at != min(m.to, hi) {
		t.Errorf("glide ends at %v (%v), want %v", at, done, m.to)
	}
}

func TestReleasePastTheTopSpringsBackOrRefreshes(t *testing.T) {
	c := &driver.ScrollConfig{Refreshable: true}
	frame, content := driver.Size{Width: 100, Height: 100}, driver.Size{Width: 100, Height: 1000}
	at := driver.Point{Y: -80}
	if !Refreshes(c, at, false) || Refreshes(c, driver.Point{Y: -20}, false) || Refreshes(c, at, true) {
		t.Error("Refreshes decides wrongly")
	}
	if p := Pulled(c, at); p != 80 {
		t.Errorf("pulled %v, want 80", p)
	}
	lo, hi := Limits(c, frame, content, true)
	if lo != -c.Behavior.RefreshInset() || hi != 900 {
		t.Errorf("refreshing limits %v..%v", lo, hi)
	}
	m := Release(c, frame, content, at, 0, lo, hi, time.Unix(0, 0))
	if m.to != lo || m.duration != Animation {
		t.Errorf("release past the top = %+v, want to the refresh room", m)
	}
	if got := Dragged(-80, 0, 900, 100, true); got >= 0 || got <= -80 {
		t.Errorf("rubber band %v", got)
	}
}
