package driver

import (
	"math"
	"testing"
)

func TestHasTouchScrollHandle(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  Environment
		want bool
	}{
		{"ios phone", Environment{OS: OSIOS, Idiom: IdiomPhone}, true},
		{"android phone", Environment{OS: OSAndroid, Idiom: IdiomPhone}, true},
		{"ios tablet", Environment{OS: OSIOS, Idiom: IdiomTablet}, true},
		{"android tablet", Environment{OS: OSAndroid, Idiom: IdiomTablet}, true},
		{"tvos", Environment{OS: OSIOS, Idiom: IdiomTV}, false},
		{"watchos", Environment{OS: OSIOS, Idiom: IdiomWatch}, false},
		{"android tv", Environment{OS: OSAndroid, Idiom: IdiomTV}, false},
		{"simulated ios", Environment{OS: OSIOS, Idiom: IdiomPhone, Simulated: true}, false},
		{"macos", Environment{OS: OSMacOS, Idiom: IdiomDesktop}, false},
		{"windows", Environment{OS: OSWindows, Idiom: IdiomDesktop}, false},
		{"linux", Environment{OS: OSLinux, Idiom: IdiomDesktop}, false},
		{"web", Environment{OS: OSWeb, Idiom: IdiomDesktop}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasTouchScrollHandle(tc.env); got != tc.want {
				t.Fatalf("HasTouchScrollHandle(%+v) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestScrollHandleEnabled(t *testing.T) {
	if ScrollHandleEnabled(nil) {
		t.Fatal("nil config should disable handle")
	}
	if !ScrollHandleEnabled(&ScrollConfig{}) {
		t.Fatal("default scroll config should enable handle")
	}
	if !ScrollHandleEnabled(&ScrollConfig{Horizontal: true}) {
		t.Fatal("horizontal scroll config should enable handle")
	}
	if ScrollHandleEnabled(&ScrollConfig{HideIndicator: true}) {
		t.Fatal("HideIndicator should disable handle")
	}
	if ScrollHandleEnabled(&ScrollConfig{Behavior: ScrollBehavior{NoScroll: true}}) {
		t.Fatal("NoScroll should disable handle")
	}
	if ScrollHandleEnabled(&ScrollConfig{MinZoomScale: 1, MaxZoomScale: 3}) {
		t.Fatal("zoomable scroll config should disable single-axis handle")
	}
}

func TestScrollHandleVerticalAndHorizontal(t *testing.T) {
	shortV := ScrollHandle{
		Frame:   Size{Width: 390, Height: 600},
		Content: Size{Width: 390, Height: 600},
	}
	if _, _, ok := shortV.Rect(); ok {
		t.Fatal("short vertical scroll view should hide handle")
	}
	if shortV.Contains(Point{X: 385, Y: 50}) {
		t.Fatal("short vertical scroll view should not hit-test handle")
	}

	shortH := ScrollHandle{
		Horizontal: true,
		Frame:      Size{Width: 390, Height: 200},
		Content:    Size{Width: 390, Height: 200},
	}
	if _, _, ok := shortH.Rect(); ok {
		t.Fatal("short horizontal scroll view should hide handle")
	}

	longV := ScrollHandle{
		Frame:   Size{Width: 390, Height: 600},
		Content: Size{Width: 390, Height: 3000},
	}
	rect, travel, ok := longV.Rect()
	if !ok {
		t.Fatal("long vertical scroll view should show handle")
	}
	if rect.Origin.X != 390-ScrollHandleInset-ScrollHandleThickness || rect.Origin.Y != ScrollHandleMargin {
		t.Fatalf("vertical rect origin = %+v, want right edge at top margin", rect.Origin)
	}
	if rect.Size.Width != ScrollHandleThickness || rect.Size.Height < ScrollHandleMinLength {
		t.Fatalf("vertical rect size = %+v", rect.Size)
	}
	if travel <= 0 {
		t.Fatalf("vertical travel = %v, want > 0", travel)
	}
	// Hit-test inside generous grab strip and outside it.
	if !longV.Contains(Point{X: 390 - ScrollHandleGrabThickness + 1, Y: rect.Origin.Y + 10}) {
		t.Fatal("expected point inside vertical grab strip to hit handle")
	}
	if longV.Contains(Point{X: 100, Y: rect.Origin.Y + 10}) {
		t.Fatal("point far from right edge should not hit vertical handle")
	}

	// Drag across full vertical travel.
	startPt := Point{X: rect.Origin.X + rect.Size.Width/2, Y: rect.Origin.Y + rect.Size.Height/2}
	drag, ok := longV.BeginDrag(startPt)
	if !ok {
		t.Fatal("BeginDrag vertical failed")
	}
	mid := drag.Offset(Point{X: startPt.X, Y: startPt.Y + travel/2})
	if math.Abs(float64(mid.Y-1200)) > 1 {
		t.Fatalf("vertical midpoint offset = %v, want ~1200", mid.Y)
	}
	end := drag.Offset(Point{X: startPt.X, Y: startPt.Y + travel + 50})
	if end.Y != 2400 {
		t.Fatalf("vertical end offset = %v, want clamped 2400", end.Y)
	}
	top := drag.Offset(Point{X: startPt.X, Y: startPt.Y - 100})
	if top.Y != 0 {
		t.Fatalf("vertical top offset = %v, want clamped 0", top.Y)
	}

	// Horizontal handle along bottom edge.
	longH := ScrollHandle{
		Horizontal: true,
		Frame:      Size{Width: 390, Height: 200},
		Content:    Size{Width: 2390, Height: 200},
		Inset:      Edges{Left: 20, Right: 30},
		Offset:     Point{X: -20, Y: 0},
	}
	hRect, hTravel, ok := longH.Rect()
	if !ok {
		t.Fatal("long horizontal scroll view should show handle")
	}
	if hRect.Origin.X != ScrollHandleMargin || hRect.Origin.Y != 200-ScrollHandleInset-ScrollHandleThickness {
		t.Fatalf("horizontal rect origin = %+v", hRect.Origin)
	}
	hStart := Point{X: hRect.Origin.X + hRect.Size.Width/2, Y: hRect.Origin.Y + hRect.Size.Height/2}
	hDrag, ok := longH.BeginDrag(hStart)
	if !ok {
		t.Fatal("BeginDrag horizontal failed")
	}
	hEnd := hDrag.Offset(Point{X: hStart.X + hTravel, Y: hStart.Y})
	if math.Abs(float64(hEnd.X-2030)) > 0.5 {
		t.Fatalf("horizontal end offset = %v, want 2030 (2390+30-390)", hEnd.X)
	}
	hBack := hDrag.Offset(Point{X: hStart.X - 50, Y: hStart.Y})
	if hBack.X != -20 {
		t.Fatalf("horizontal start offset = %v, want -20 (-Inset.Left)", hBack.X)
	}

	// Very small viewport still keeps positive travel.
	tiny := ScrollHandle{
		Frame:   Size{Width: 200, Height: 48},
		Content: Size{Width: 200, Height: 500},
	}
	_, tinyTravel, ok := tiny.Rect()
	if !ok || tinyTravel <= 0 {
		t.Fatalf("tiny viewport: ok=%v travel=%v, want positive travel", ok, tinyTravel)
	}
}
