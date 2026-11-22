package driver

import (
	"math"
	"testing"
)

func TestGeometryBoundsComposeViewportsAndScroll(t *testing.T) {
	views := map[NodeID]ViewGeometry{
		1: {Frame: Bounds{Origin: Point{20, 30}, Size: Size{200, 100}}, Offset: Point{5, 7},
			Matrix: Transform{ScaleX: 2}.Matrix(Point{100, 50})},
		2: {Parent: 1, Frame: Bounds{Origin: Point{10, 20}, Size: Size{40, 20}}, Matrix: Transform{Rotate: 90}.Matrix(Point{20, 10})},
	}
	want := Bounds{Origin: Point{-50, 33}, Size: Size{40, 40}}
	if got := GeometryBounds(views)[2]; got != want {
		t.Fatalf("rotated child in scaled, scrolled parent = %v, want %v", got, want)
	}
}

func TestGeometryBoundsLeaveOutUndisplayedPlanes(t *testing.T) {
	behind := Identity3D
	behind[15] = -1
	views := map[NodeID]ViewGeometry{
		1: {Frame: Bounds{Size: Size{40, 20}}, Matrix: behind},
		2: {Parent: 3, Frame: Bounds{Size: Size{40, 20}}},
	}
	if got := GeometryBounds(views); len(got) != 0 {
		t.Fatalf("undisplayed geometry = %v", got)
	}
}

func TestGeometryObserverPublishesOnlyCompleteChangedFrames(t *testing.T) {
	var got []GeometrySnapshot
	var observer GeometryObserver
	observer.Observe(func(snapshot GeometrySnapshot) { got = append(got, snapshot) })
	bounds := map[NodeID]Bounds{1: {Size: Size{40, 20}}}
	observer.Notify(GeometrySnapshot{Bounds: bounds})
	observer.Begin(2)
	observer.Notify(GeometrySnapshot{Bounds: map[NodeID]Bounds{1: {Size: Size{10, 10}}}})
	if len(got) != 1 {
		t.Fatal("published a partly applied batch")
	}
	observer.End()
	observer.Notify(GeometrySnapshot{Bounds: bounds})
	observer.Notify(GeometrySnapshot{Bounds: bounds})
	if len(got) != 2 || got[1].Version != 2 {
		t.Fatalf("completed frames = %+v", got)
	}
	observer.Observe(nil)
	observer.Notify(GeometrySnapshot{Bounds: bounds})
	if len(got) != 2 {
		t.Fatal("published after observation ended")
	}
}

func TestGeometryBoundsRejectFloat32Overflow(t *testing.T) {
	views := map[NodeID]ViewGeometry{1: {Frame: Bounds{Size: Size{40, 20}}, Matrix: Translate3D(1e100, 0, 0)}}
	if got := GeometryBounds(views); len(got) != 0 {
		t.Fatalf("overflowing projection has bounds: %v", got)
	}
	if box, ok := PointBounds([4]Point{{X: -math.MaxFloat32}, {X: math.MaxFloat32}, {}, {}}); ok {
		t.Fatalf("overflowing box width was accepted: %v", box)
	}
}

func TestMatrixFromQuadKeepsPerspectiveAndPicking(t *testing.T) {
	size := Size{Width: 80, Height: 40}
	original := Translate3D(35, 50, 0).Mul(Transform{RotateY: 40, Perspective: 300}.Matrix(Point{X: 40, Y: 20}))
	var quad [4]Point
	for i, p := range [4]Point{{}, {X: 80}, {X: 80, Y: 40}, {Y: 40}} {
		x, y := original.Apply(float64(p.X), float64(p.Y))
		quad[i] = Point{X: Points(x), Y: Points(y)}
	}
	matrix, ok := MatrixFromQuad(quad, size)
	if !ok {
		t.Fatal("finite native viewport could not be recovered")
	}
	for _, p := range []Point{{X: 15, Y: 10}, {X: 60, Y: 30}} {
		x, y := original.Apply(float64(p.X), float64(p.Y))
		mappedX, mappedY := matrix.Apply(float64(p.X), float64(p.Y))
		if math.Abs(mappedX-x) > 1e-4 || math.Abs(mappedY-y) > 1e-4 {
			t.Fatalf("perspective point = %v,%v, want %v,%v", mappedX, mappedY, x, y)
		}
		local, valid := matrix.Unproject(Point{X: Points(x), Y: Points(y)})
		if !valid || math.Abs(float64(local.X-p.X)) > 1e-4 || math.Abs(float64(local.Y-p.Y)) > 1e-4 {
			t.Fatalf("native pick = %v,%v, want %v", local, valid, p)
		}
	}
	if _, valid := MatrixFromQuad([4]Point{}, size); valid {
		t.Fatal("collapsed native quad was accepted")
	}
}
