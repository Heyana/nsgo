package scrollbar

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestZoomAxisLimitsAndCentering(t *testing.T) {
	c := &driver.ScrollConfig{MinZoomScale: 0.5, MaxZoomScale: 4, CenterContent: true,
		Behavior: driver.ScrollBehavior{ContentInset: driver.Edges{Left: 10, Top: 20, Right: 10, Bottom: 20}}}
	frame := driver.Size{Width: 200, Height: 200}
	content := driver.Size{Width: 100, Height: 100}

	// Zoomed 1x: 10+100+10=120 < 200 -> centered at -(10 + 40) = -50
	xlo, xhi := ZoomAxisLimits(c, frame, content, 1, true)
	if xlo != -50 || xhi != -50 {
		t.Fatalf("centered X limits = (%v, %v), want (-50, -50)", xlo, xhi)
	}
	// Zoomed 3x: 300+10-200 = 110 -> [-10, 110]
	xlo, xhi = ZoomAxisLimits(c, frame, content, 3, true)
	if xlo != -10 || xhi != 110 {
		t.Fatalf("zoomed 3x X limits = (%v, %v), want (-10, 110)", xlo, xhi)
	}
	ylo, yhi := ZoomAxisLimits(c, frame, content, 3, false)
	if ylo != -20 || yhi != 120 {
		t.Fatalf("zoomed 3x Y limits = (%v, %v), want (-20, 120)", ylo, yhi)
	}
}

func TestLimitZoomAndRubberBand(t *testing.T) {
	c := &driver.ScrollConfig{MinZoomScale: 0.5, MaxZoomScale: 4}
	if got := LimitZoom(c, 2, false); got != 2 {
		t.Fatalf("in range = %v, want 2", got)
	}
	if got := LimitZoom(c, 8, false); got != 4 {
		t.Fatalf("clamped hi = %v, want 4", got)
	}
	if got := LimitZoom(c, 0.25, false); got != 0.5 {
		t.Fatalf("clamped lo = %v, want 0.5", got)
	}
	rub := LimitZoom(c, 8, true)
	if rub <= 4 || rub >= 8 {
		t.Fatalf("rubber hi = %v, want between 4 and 8", rub)
	}
	c.NoBouncesZoom = true
	if got := LimitZoom(c, 8, true); got != 4 {
		t.Fatalf("no bounce hi = %v, want 4", got)
	}
}

func TestZoomFocalRoundTripAndEase(t *testing.T) {
	c := &driver.ScrollConfig{MinZoomScale: 0.5, MaxZoomScale: 4}
	frame := driver.Size{Width: 300, Height: 300}
	content := driver.Size{Width: 300, Height: 300}
	focus := driver.Point{X: 150, Y: 150}
	pt := ContentAt(driver.Point{X: 30, Y: 60}, focus, 2)
	if pt != (driver.Point{X: 90, Y: 105}) {
		t.Fatalf("ContentAt = %+v, want (90, 105)", pt)
	}
	off := OffsetFor(pt, focus, 2)
	if off != (driver.Point{X: 30, Y: 60}) {
		t.Fatalf("OffsetFor = %+v, want (30, 60)", off)
	}
	ease, endAt, ok := NewZoomEase(c, frame, content, driver.Point{}, 1, driver.Point{X: 100, Y: 100}, focus, 3)
	if !ok {
		t.Fatal("expected animated ease")
	}
	if endAt != (driver.Point{X: 150, Y: 150}) {
		t.Fatalf("endAt = %+v, want (150, 150)", endAt)
	}
	midAt, midZ := ease.Step(c, frame, content, 0.5)
	if math.Abs(midZ-2) > 1e-6 {
		t.Fatalf("midZ = %v, want 2", midZ)
	}
	if midAt.X <= 0 || midAt.X >= endAt.X {
		t.Fatalf("midAt = %+v, want strictly between 0 and %+v", midAt, endAt)
	}
	finalAt, finalZ := ease.Step(c, frame, content, 1)
	if finalAt != endAt || math.Abs(finalZ-3) > 1e-6 {
		t.Fatalf("final = (%+v, %v), want (%+v, 3)", finalAt, finalZ, endAt)
	}
}

func TestWheelZoom(t *testing.T) {
	if got := WheelZoom(1, -100*math.Log(3)); math.Abs(got-3) > 1e-6 {
		t.Fatalf("WheelZoom = %v, want 3", got)
	}
	if got := WheelZoom(2, 0); got != 2 {
		t.Fatalf("WheelZoom(0) = %v, want 2", got)
	}
}
