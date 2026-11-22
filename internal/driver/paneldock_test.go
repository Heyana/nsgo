package driver

import "testing"

func TestDockWindowUsesTheSelectedUsableEdge(t *testing.T) {
	primary := DisplayInfo{ID: "main", Main: true, Frame: Bounds{Size: Size{Width: 800, Height: 600}}, VisibleFrame: Bounds{Origin: Point{X: 10, Y: 20}, Size: Size{Width: 780, Height: 560}}, Scale: 1}
	secondary := DisplayInfo{ID: "left", Frame: Bounds{Origin: Point{X: -600}, Size: Size{Width: 600, Height: 400}}, VisibleFrame: Bounds{Origin: Point{X: -600}, Size: Size{Width: 600, Height: 360}}, Scale: 2}
	for _, c := range []struct {
		name    string
		edge    WindowPlacement
		display string
		x, y    Points
	}{
		{"primary top", WindowPlaceTop, "", 340, 30},
		{"primary bottom", WindowPlaceBottom, "", 340, 510},
		{"primary left", WindowPlaceLeft, "", 20, 270},
		{"primary right", WindowPlaceRight, "", 660, 270},
		{"other scale top", WindowPlaceTop, "left", -360, 10},
		{"other scale bottom", WindowPlaceBottom, "left", -360, 290},
	} {
		t.Run(c.name, func(t *testing.T) {
			frame, ok := DockWindow(Size{Width: 120, Height: 60}, []DisplayInfo{secondary, primary}, PanelDock{Edge: c.edge, Display: c.display, Margin: 10})
			want := WindowFrame{X: c.x, Y: c.y, Width: 120, Height: 60, Move: true}
			if !ok || frame != want {
				t.Fatalf("dock = %+v,%v; want %+v", frame, ok, want)
			}
		})
	}
	for _, c := range []struct {
		name string
		size Size
		dock PanelDock
	}{
		{"missing display", Size{Width: 120, Height: 60}, PanelDock{Edge: WindowPlaceTop, Display: "unplugged"}},
		{"no edge", Size{Width: 120, Height: 60}, PanelDock{}},
		{"invalid edge", Size{Width: 120, Height: 60}, PanelDock{Edge: 255}},
		{"empty size", Size{}, PanelDock{Edge: WindowPlaceTop}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if frame, ok := DockWindow(c.size, []DisplayInfo{primary}, c.dock); ok || frame != (WindowFrame{}) {
				t.Fatalf("unavailable dock = %+v,%v", frame, ok)
			}
		})
	}
}

func TestDockWindowClampsMarginsAndOversizedPanels(t *testing.T) {
	all := []DisplayInfo{{ID: "screen", Frame: Bounds{Origin: Point{X: 100, Y: -200}, Size: Size{Width: 300, Height: 200}}, Main: true}}
	for _, c := range []struct {
		margin Points
		size   Size
		want   Point
	}{
		{-10, Size{Width: 100, Height: 40}, Point{X: 200, Y: -200}},
		{1000, Size{Width: 100, Height: 40}, Point{X: 200, Y: -40}},
		{10, Size{Width: 500, Height: 400}, Point{X: 100, Y: -200}},
	} {
		frame, ok := DockWindow(c.size, all, PanelDock{Edge: WindowPlaceTop, Margin: c.margin})
		if !ok || frame.X != c.want.X || frame.Y != c.want.Y {
			t.Fatalf("margin%v size%+v: %+v,%v", c.margin, c.size, frame, ok)
		}
	}
}
