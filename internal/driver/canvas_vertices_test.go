package driver

import (
	"image"
	"math"
	"testing"
)

func TestDrawVerticesSnapshotsItsTriangles(t *testing.T) {
	src := ImageSource{Pixels: image.NewRGBA(image.Rect(0, 0, 2, 2))}
	a := CanvasVertex{Position: Point{2, 4}, TexCoord: Point{0, 0}}
	b := CanvasVertex{Position: Point{8, 4}, TexCoord: Point{2, 0}}
	c := CanvasVertex{Position: Point{8, 12}, TexCoord: Point{2, 2}}
	e := CanvasVertex{Position: Point{2, 12}, TexCoord: Point{0, 2}}
	invalid := CanvasVertex{Position: Point{Points(math.NaN()), 0}}
	list := []CanvasVertex{a, b, c, a, c, e, a, a, a, a, b, invalid, e}
	d := NewDrawing(nil).DrawVertices(src, list)
	if len(d.Ops) != 1 {
		t.Fatalf("recorded %d ops, want one", len(d.Ops))
	}
	op := d.Ops[0]
	list[0] = CanvasVertex{}
	if op.Kind != DrawVertices || len(op.Mesh.Vertices) != 6 || op.Mesh.Vertices[0] != a || !op.Kind.Paints() {
		t.Fatalf("invalid mesh recording: %+v", op)
	}
	crop, ok := op.ImageRect()
	if !ok || crop.X != 2 || crop.Y != 4 || crop.W != 6 || crop.H != 8 || crop.X2 != 2 || crop.Y2 != 2 {
		t.Fatalf("rectangular mesh became %+v, ok=%v", crop, ok)
	}

	before := NewDrawing(nil).Translate(0, 0).DrawVertices(src, []CanvasVertex{a, b, c})
	after := append([]DrawOp(nil), before.Ops...)
	after[0].X = 10
	area, ok := CanvasDamage(before.Ops, after)
	if !ok || area != (Bounds{Origin: Point{2, 4}, Size: Size{16, 8}}) {
		t.Fatalf("mesh damage = %+v, ok=%v", area, ok)
	}
}
