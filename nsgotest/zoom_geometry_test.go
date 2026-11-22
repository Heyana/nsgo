package nsgotest

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestZoomedScrollPressAtReachesVisibleContent(t *testing.T) {
	var scroll core.Ref
	pressed := -1
	app := Start(t, func(*core.Scope) core.Node {
		var children []core.Node
		for index := range 3 {
			children = append(children, core.View{TestID: []string{"first", "middle", "last"}[index], Style: core.Style{Width: core.Pt(120), Height: core.Pt(80), FlexShrink: 0}, OnPress: func(core.PressEvent) { pressed = index }})
		}
		return core.Scroll{TestID: "scroll", Ref: &scroll, MaxZoomScale: 2, Style: core.Style{Width: core.Pt(120), Height: core.Pt(80)}, Children: children}
	}, Size(300, 260))
	app.Post(func() { scroll.ZoomTo(2, nil, false); scroll.ScrollTo(core.Point{Y: 320}, false) })
	app.Settle()
	if got := app.Find("id:scroll").Offset.Y; got != 320 {
		t.Errorf("zoomed offset = %g, want 320", got)
	}
	app.PressAt("id:scroll", core.Point{X: 60, Y: 40})
	if pressed != 2 {
		t.Errorf("press in the zoomed viewport reached band%d, want last band", pressed)
	}
}

func TestZoomedScrollGeometryMatchesItsNativeViewport(t *testing.T) {
	for _, c := range []struct {
		name        string
		scale       float32
		horizontal  bool
		offset      driver.Point
		content     driver.Size
		child       driver.Bounds
		local, page driver.Point
		end         driver.Points
	}{
		{"vertical doubled", 2, false, driver.Point{Y: 320}, driver.Size{Width: 120, Height: 240}, driver.Bounds{Origin: driver.Point{Y: 160}, Size: driver.Size{Width: 120, Height: 80}}, driver.Point{X: 30, Y: 20}, driver.Point{X: 60, Y: 40}, 400},
		{"vertical halved", 0.5, false, driver.Point{Y: 60}, driver.Size{Width: 240, Height: 480}, driver.Bounds{Origin: driver.Point{Y: 160}, Size: driver.Size{Width: 240, Height: 160}}, driver.Point{X: 60, Y: 40}, driver.Point{X: 30, Y: 40}, 160},
		{"horizontal doubled", 2, true, driver.Point{X: 480}, driver.Size{Width: 360, Height: 80}, driver.Bounds{Origin: driver.Point{X: 240}, Size: driver.Size{Width: 120, Height: 80}}, driver.Point{X: 30, Y: 20}, driver.Point{X: 60, Y: 40}, 600},
	} {
		t.Run(c.name, func(t *testing.T) {
			scroll := &View{ID: 1, Kind: KindScroll, Style: viewStyle(driver.DefaultViewStyle), Frame: driver.Bounds{Size: driver.Size{Width: 120, Height: 80}}, Content: c.content, ZoomScale: c.scale, Offset: c.offset, Scroll: driver.ScrollConfig{Horizontal: c.horizontal}}
			child := &View{ID: 2, Parent: scroll, Kind: KindView, Style: viewStyle(driver.DefaultViewStyle), Frame: c.child}
			scroll.Children = []*View{child}
			app := &App{views: map[driver.NodeID]*View{1: scroll, 2: child}, rootID: 1}
			if got := child.pagePoint(c.local); got != c.page {
				t.Errorf("content point in viewport: %v, want %v", got, c.page)
			}
			if got := child.localPoint(c.page); got != c.local {
				t.Errorf("viewport point in content: %v, want %v", got, c.local)
			}
			if got := app.viewAt(c.page, func(v *View) bool { return v == child }); got != child {
				t.Error("visible child was not picked")
			}
			if got := scroll.scrollEnd(); got != c.end {
				t.Errorf("zoomed scroll end: %g, want %g", got, c.end)
			}
		})
	}
}

func TestNestedScrollZoomKeepsVisibilityAndLayoutBounds(t *testing.T) {
	outer := &View{ID: 1, Kind: KindScroll, Style: viewStyle(driver.DefaultViewStyle), Frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 20}, Size: driver.Size{Width: 400, Height: 300}}, ZoomScale: 0.5, Offset: driver.Point{X: 5, Y: 7}, Scroll: driver.ScrollConfig{Behavior: driver.ScrollBehavior{NoScroll: true}}}
	inner := &View{ID: 2, Parent: outer, Kind: KindScroll, Style: viewStyle(driver.DefaultViewStyle), Frame: driver.Bounds{Origin: driver.Point{X: 100, Y: 80}, Size: driver.Size{Width: 120, Height: 80}}, ZoomScale: 2, Offset: driver.Point{Y: 320}, Scroll: driver.ScrollConfig{Behavior: driver.ScrollBehavior{NoScroll: true}}}
	child := &View{ID: 3, Parent: inner, Kind: KindView, Style: viewStyle(driver.DefaultViewStyle), Frame: driver.Bounds{Origin: driver.Point{Y: 160}, Size: driver.Size{Width: 120, Height: 80}}}
	outer.Children = []*View{inner}
	inner.Children = []*View{child}
	app := &App{views: map[driver.NodeID]*View{1: outer, 2: inner, 3: child}, rootID: 1}
	if got := child.pagePoint(driver.Point{X: 30, Y: 20}); got != (driver.Point{X: 85, Y: 73}) {
		t.Errorf("nested viewport point: %v, want {85 73}", got)
	}
	if got := child.localPoint(driver.Point{X: 85, Y: 73}); got != (driver.Point{X: 30, Y: 20}) {
		t.Errorf("nested content point: %v, want {30 20}", got)
	}
	if got := app.viewAt(driver.Point{X: 85, Y: 73}, func(v *View) bool { return v == child }); got != child {
		t.Error("nested visible child was not picked")
	}
	shown := false
	for _, v := range app.shown() {
		shown = shown || v == child
	}
	if !shown {
		t.Error("nested visible child was hidden by unscaled layout clipping")
	}
	if got := child.Bounds(); got != (core.Bounds{Origin: core.Point{X: 105, Y: -67}, Size: core.Size{Width: 120, Height: 80}}) {
		t.Errorf("layout Bounds changed: %v", got)
	}
	if got := inner.zoomedFrame(); got != (driver.Bounds{Origin: driver.Point{X: 55, Y: 53}, Size: driver.Size{Width: 60, Height: 40}}) {
		t.Errorf("nested viewport bounds: %v", got)
	}
}

func TestScrollZoomAppliesBeforeParentPerspective(t *testing.T) {
	parent := &View{Kind: KindScroll, Style: viewStyle(driver.DefaultViewStyle), Frame: driver.Bounds{Size: driver.Size{Width: 120, Height: 80}}, ZoomScale: 2, Offset: driver.Point{Y: 320}}
	parent.Style.ChildPerspective = 500
	child := &View{Parent: parent, Style: viewStyle(driver.DefaultViewStyle), Frame: driver.Bounds{Origin: driver.Point{Y: 160}, Size: driver.Size{Width: 120, Height: 80}}}
	child.Style.Transform = driver.Transform{RotateY: 0.3}
	point := driver.Point{X: 30, Y: 20}
	expected := driver.ChildPerspective(500, parent.Frame.Size, driver.Point{}).Mul(parent.contentMatrix()).Mul(driver.Translate3D(0, 160, 0)).Mul(child.Style.Matrix(child.Frame.Size))
	x, y := expected.Apply(float64(point.X), float64(point.Y))
	page := child.pagePoint(point)
	if math.Abs(float64(page.X)-x) > 0.0001 || math.Abs(float64(page.Y)-y) > 0.0001 {
		t.Errorf("perspective point: %v, want {%g %g}", page, x, y)
	}
	local := child.localPoint(page)
	if math.Abs(float64(local.X-point.X)) > 0.0001 || math.Abs(float64(local.Y-point.Y)) > 0.0001 {
		t.Errorf("perspective inverse: %v, want %v", local, point)
	}
}

func TestZoomedScrollRevealUsesBothAxes(t *testing.T) {
	var scroll core.Ref
	app := Start(t, func(*core.Scope) core.Node {
		return core.Scroll{TestID: "scroll", Ref: &scroll, MaxZoomScale: 2, Style: core.Style{Width: core.Pt(120), Height: core.Pt(80)}, Children: []core.Node{
			core.View{TestID: "last", Style: core.Style{Position: core.Absolute, Top: core.Pt(160), Width: core.Pt(120), Height: core.Pt(80)}, OnPress: func(core.PressEvent) {}},
		}}
	}, Size(300, 260))
	app.Post(func() { scroll.ZoomTo(2, nil, false); scroll.ScrollTo(core.Point{Y: 320}, false) })
	app.Settle()
	if got := app.reveal(app.Find("id:last")); got != (driver.Point{X: 60, Y: 40}) {
		t.Errorf("revealed center: %v, want {60 40}", got)
	}
	if got := app.Find("id:scroll").Offset; got != (driver.Point{X: 60, Y: 360}) {
		t.Errorf("reveal offset: %v, want {60 360}", got)
	}
}
