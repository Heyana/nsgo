package canvasmesh

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func vertexAt(x, y, u, v driver.Points, tint driver.Color) driver.CanvasVertex {
	return driver.CanvasVertex{Position: driver.Point{X: x, Y: y}, TexCoord: driver.Point{X: u, Y: v}, Color: tint}
}

func TestRasterSharesTheDiagonal(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	halfRed := color.RGBA{R: 128, A: 128}
	pixels.SetRGBA(0, 0, halfRed)
	a, b, c, e := vertexAt(0, 0, 0, 0, 0), vertexAt(4, 0, 1, 0, 0), vertexAt(4, 4, 1, 1, 0), vertexAt(0, 4, 0, 1, 0)
	src := driver.ImageSource{Pixels: pixels}
	// Opposite winding exercises the coverage tie on the same diagonal.
	op := driver.NewDrawing(nil).DrawVertices(src, []driver.CanvasVertex{a, b, c, a, e, c}).Ops[0]
	out := Raster(&op, RGBA(pixels, driver.Size{}), 2)
	for y := range out.Rect.Dy() {
		for x := range out.Rect.Dx() {
			if c := out.RGBAAt(x, y); c != halfRed {
				t.Errorf("pixel %d,%d = %v, want %v", x, y, c, halfRed)
			}
		}
	}
}

func TestRasterMultipliesInterpolatedVertexColors(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixels.SetRGBA(0, 0, color.RGBA{R: 128, G: 192, B: 255, A: 255})
	vertices := []driver.CanvasVertex{
		vertexAt(0, 0, 0, 0, driver.RGB(0xff0000)),
		vertexAt(8, 0, 1, 0, driver.RGB(0x00ff00)),
		vertexAt(0, 8, 0, 1, driver.RGB(0x0000ff)),
	}
	op := driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Pixels: pixels}, vertices).Ops[0]
	out := Raster(&op, RGBA(pixels, driver.Size{}), 1)
	if c, want := out.RGBAAt(0, 0), (color.RGBA{R: 112, G: 12, B: 16, A: 255}); c != want {
		t.Errorf("interpolated tint = %v, want %v", c, want)
	}
	if c := out.RGBAAt(7, 7); c != (color.RGBA{}) {
		t.Errorf("pixel outside the triangle = %v", c)
	}
}

func BenchmarkDeformingMesh(b *testing.B) {
	pixels := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			pixels.SetRGBA(x, y, color.RGBA{R: uint8(32 * x), B: uint8(32 * y), A: 255})
		}
	}
	tex := RGBA(pixels, driver.Size{})
	src := driver.ImageSource{Pixels: pixels}
	const columns, rows = 25, 20
	vertices := make([]driver.CanvasVertex, 0, columns*rows*6)
	for _, ratio := range []float64{1, 2} {
		b.Run(fmt.Sprintf("%gx", ratio), func(b *testing.B) {
			var frame float64
			for b.Loop() {
				frame += .01
				vertices = vertices[:0]
				for y := range rows {
					base := driver.Points(y * 14)
					dx := driver.Points(6 * math.Sin(frame+float64(y)/5))
					dxNext := driver.Points(6 * math.Sin(frame+float64(y+1)/5))
					for x := range columns {
						left := driver.Points(x * 20)
						a := vertexAt(left+dx, base, 0, 0, 0)
						c := vertexAt(left+20+dxNext, base+14, 8, 8, driver.RGB(0xddeeff))
						vertices = append(vertices, a, vertexAt(left+20+dx, base, 8, 0, 0), c,
							a, c, vertexAt(left+dxNext, base+14, 0, 8, 0))
					}
				}
				op := driver.NewDrawing(nil).DrawVertices(src, vertices).Ops[0]
				if Raster(&op, tex, ratio) == nil {
					b.Fatal("no raster")
				}
			}
		})
	}
}

func TestRasterSamplesNearestWhenSmoothingIsOff(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 1))
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	pixels.SetRGBA(0, 0, red)
	pixels.SetRGBA(1, 0, blue)
	src := driver.ImageSource{Pixels: pixels}
	// A triangle stretches the two texels over 16 pixels; its top row crosses both.
	vertices := []driver.CanvasVertex{vertexAt(0, 0, 0, 0, 0), vertexAt(16, 0, 2, 0, 0), vertexAt(0, 16, 0, 1, 0)}
	draw := func(smooth bool) *image.RGBA {
		op := driver.NewDrawing(nil).SetImageSmoothing(smooth).DrawVertices(src, vertices).Ops[0]
		return Raster(&op, RGBA(pixels, driver.Size{}), 1)
	}
	nearest := draw(false)
	for x := range 15 {
		want := red
		if x >= 8 {
			want = blue
		}
		if c := nearest.RGBAAt(x, 0); c != want {
			t.Errorf("nearest pixel %d = %v, want %v", x, c, want)
		}
	}
	if c := draw(true).RGBAAt(7, 0); c == red || c == blue {
		t.Errorf("smooth pixel 7 = %v, want a blend", c)
	}
}

// quad is two triangles over x, y, w, h that show the whole of a texture of size points.
func quad(x, y, w, h, size driver.Points, tint driver.Color) []driver.CanvasVertex {
	a, b := vertexAt(x, y, 0, 0, tint), vertexAt(x+w, y, size, 0, tint)
	c, d := vertexAt(x+w, y+h, size, size, tint), vertexAt(x, y+h, 0, size, tint)
	return []driver.CanvasVertex{a, b, c, a, c, d}
}

func TestBoundsCapsPixels(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	tex := RGBA(pixels, driver.Size{})
	src := driver.ImageSource{Pixels: pixels}
	for _, tc := range []struct {
		name          string
		w, h          driver.Points
		scale         float64
		width, height int
	}{
		{"within the cap", 100, 50, 2, 200, 100},
		{"one side over 4096", 3000, 10, 2, 4096, 14},
		{"area over 4M", 2000, 2000, 2, 2048, 2048},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op := driver.NewDrawing(nil).DrawVertices(src, quad(10, 20, tc.w, tc.h, 1, 0)).Ops[0]
			got := Bounds(&op, tex, tc.scale)
			if got.Width != tc.width || got.Height != tc.height || got.X != 10 || got.Y != 20 {
				t.Fatalf("Bounds = %dx%d at %v,%v, want %dx%d at 10,20", got.Width, got.Height, got.X, got.Y, tc.width, tc.height)
			}
			if got.Width > maxSide || got.Height > maxSide || got.Width*got.Height > maxPixels {
				t.Fatalf("Bounds %dx%d exceeds the cap", got.Width, got.Height)
			}
			if sx, sy := float64(got.Width)/float64(tc.w), float64(got.Height)/float64(tc.h); got.ScaleX != sx || got.ScaleY != sy {
				t.Fatalf("scale = %v, %v, want %v, %v", got.ScaleX, got.ScaleY, sx, sy)
			}
		})
	}
	empty := driver.NewDrawing(nil).DrawVertices(src, quad(0, 0, 10, 10, 1, 0)).Ops[0]
	if got := Bounds(&empty, Texture{}, 2); got.Width != 0 {
		t.Fatalf("Bounds without a texture = %dx%d, want none", got.Width, got.Height)
	}
}

func TestDeviceAlignsToDevicePixels(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	tex := RGBA(pixels, driver.Size{})
	op := driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Pixels: pixels}, quad(10, 5, 20, 10, 1, 0)).Ops[0]
	for _, tc := range []struct {
		name           string
		sx, sy, tx, ty float64
		x, y           float64
		width, height  int
	}{
		{"whole pixels", 2, 2, 0, 0, 10, 5, 40, 20},
		{"half-pixel offset", 2, 2, .5, .25, 9.75, 4.875, 41, 21},
		// A flipped context, as a canvas drawn upside down, maps the target's first column to the device's last.
		{"flipped x", -2, 2, 100.5, 0, 9.75, 5, 41, 20},
		{"flipped y", 2, -2, 0, 60.5, 10, 4.75, 40, 21},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, aligned := Device(&op, tex, tc.sx, tc.sy, tc.tx, tc.ty)
			if !aligned {
				t.Fatal("Device fell back to Bounds")
			}
			if got.X != tc.x || got.Y != tc.y || got.Width != tc.width || got.Height != tc.height {
				t.Fatalf("Device = %dx%d at %v,%v, want %dx%d at %v,%v", got.Width, got.Height, got.X, got.Y, tc.width, tc.height, tc.x, tc.y)
			}
			if got.ScaleX != math.Abs(tc.sx) || got.ScaleY != math.Abs(tc.sy) {
				t.Fatalf("scale = %v, %v, want %v, %v", got.ScaleX, got.ScaleY, math.Abs(tc.sx), math.Abs(tc.sy))
			}
			// Both of the target's edges land on whole device pixels, and it covers the op.
			for _, edge := range []struct{ at, s, t float64 }{
				{got.X, tc.sx, tc.tx}, {got.X + float64(got.Width)/got.ScaleX, tc.sx, tc.tx},
				{got.Y, tc.sy, tc.ty}, {got.Y + float64(got.Height)/got.ScaleY, tc.sy, tc.ty},
			} {
				if p := edge.at*edge.s + edge.t; p != math.Round(p) {
					t.Fatalf("edge at %v maps to device pixel %v", edge.at, p)
				}
			}
			if got.X > float64(op.X) || got.Y > float64(op.Y) ||
				got.X+float64(got.Width)/got.ScaleX < float64(op.X+op.W) || got.Y+float64(got.Height)/got.ScaleY < float64(op.Y+op.H) {
				t.Fatalf("Device %+v does not cover the op's bounds", got)
			}
		})
	}
}

func TestDeviceFallsBackToBoundsOverTheCap(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	tex := RGBA(pixels, driver.Size{})
	op := driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Pixels: pixels}, quad(0, 0, 2000, 1500, 1, 0)).Ops[0]
	for _, s := range []float64{4, -4} {
		got, aligned := Device(&op, tex, s, s, 0, 0)
		if aligned {
			t.Fatalf("scale %v: Device kept %dx%d device pixels past the cap", s, got.Width, got.Height)
		}
		if want := Bounds(&op, tex, 4); got.Width != want.Width || got.Height != want.Height ||
			got.X != want.X || got.Y != want.Y || got.ScaleX != want.ScaleX || got.ScaleY != want.ScaleY {
			t.Fatalf("scale %v: Device = %+v, want Bounds %+v", s, got, want)
		}
	}
}

func TestDrawSwapsRedAndBlueForBGRA(t *testing.T) {
	red := image.NewRGBA(image.Rect(0, 0, 1, 1))
	red.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	white := image.NewRGBA(image.Rect(0, 0, 1, 1))
	white.SetRGBA(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	for _, tc := range []struct {
		name        string
		texture     *image.RGBA
		textureBGRA bool
		tint        driver.Color
		targetBGRA  bool
		want        [4]byte
	}{
		{"RGBA texture into BGRA", red, false, 0, true, [4]byte{0, 0, 255, 255}},
		{"BGRA texture into RGBA", red, true, 0, false, [4]byte{0, 0, 255, 255}},
		{"BGRA texture into BGRA", red, true, 0, true, [4]byte{255, 0, 0, 255}},
		{"red tint into BGRA", white, false, driver.RGB(0xff0000), true, [4]byte{0, 0, 255, 255}},
		{"red tint into RGBA", white, false, driver.RGB(0xff0000), false, [4]byte{255, 0, 0, 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tex := RGBA(tc.texture, driver.Size{})
			tex.BGRA = tc.textureBGRA
			op := driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Pixels: tc.texture}, quad(0, 0, 4, 4, 1, tc.tint)).Ops[0]
			dst := Bounds(&op, tex, 1)
			dst.BGRA = tc.targetBGRA
			dst.Stride = dst.Width * 4
			dst.Pix = make([]byte, dst.Stride*dst.Height)
			Draw(dst, &op, tex)
			if got := [4]byte(dst.Pix[:4]); got != tc.want {
				t.Fatalf("first pixel's bytes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDrawBandsMatchOneWorker(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			pixels.SetRGBA(x, y, color.RGBA{R: uint8(32 * x), G: 80, B: uint8(32 * y), A: 255})
		}
	}
	tex := RGBA(pixels, driver.Size{})
	// Overlapping translucent quads, so a band that broke triangle order would blend differently.
	var vertices []driver.CanvasVertex
	for i := range 12 {
		offset := driver.Points(i * 23)
		vertices = append(vertices, quad(offset, offset/2, 300, 260, 8, driver.RGBA(0xffffff, .5))...)
		vertices = append(vertices, vertexAt(offset, 500, 0, 0, 0), vertexAt(500, offset, 8, 0, driver.RGB(0x88ccff)), vertexAt(500-offset, 500, 4, 8, 0))
	}
	op := driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Pixels: pixels}, vertices).Ops[0]
	draw := func(procs int) []byte {
		defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))
		dst := Bounds(&op, tex, 1)
		if procs > 1 && min(procs, (dst.Height+31)/32, dst.Width*dst.Height/(64<<10)) < 2 {
			t.Fatalf("a %dx%d target takes one worker", dst.Width, dst.Height)
		}
		dst.Stride = dst.Width * 4
		dst.Pix = make([]byte, dst.Stride*dst.Height)
		Draw(dst, &op, tex)
		return dst.Pix
	}
	one, many := draw(1), draw(8)
	if !bytes.Equal(one, many) {
		for i := range one {
			if one[i] != many[i] {
				t.Fatalf("byte %d = %d with 8 workers, %d with one", i, many[i], one[i])
			}
		}
	}
}

func TestDrawSkipsTrianglesBesideTheTarget(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixels.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	tex := RGBA(pixels, driver.Size{})
	op := driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Pixels: pixels}, quad(0, 0, 4, 4, 1, 0)).Ops[0]
	// The quad lies wholly right of a target at -100 and wholly left of one at 100, so its columns cross.
	for _, x := range []float64{-100, 100} {
		dst := Target{Width: 4, Height: 4, Stride: 16, Pix: make([]byte, 64), X: x, ScaleX: 2, ScaleY: 2}
		Draw(dst, &op, tex)
		if !bytes.Equal(dst.Pix, make([]byte, 64)) {
			t.Errorf("a target at x %v drew pixels of a quad beside it", x)
		}
	}
}

func TestDrawClampsHugeTexCoordsToTheEdgeTexel(t *testing.T) {
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 1))
	pixels.SetRGBA(0, 0, red)
	pixels.SetRGBA(1, 0, blue)
	tex := RGBA(pixels, driver.Size{})
	for _, tc := range []struct {
		name        string
		left, right driver.Points
		want        [4]color.RGBA
	}{
		{"far right", 1e12, 1e12, [4]color.RGBA{blue, blue, blue, blue}},
		{"far left", -1e12, -1e12, [4]color.RGBA{red, red, red, red}},
		{"across", -1e12, 1e12, [4]color.RGBA{red, red, blue, blue}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vertices := []driver.CanvasVertex{
				vertexAt(0, 0, tc.left, 0, 0), vertexAt(4, 0, tc.right, 0, 0), vertexAt(4, 1, tc.right, 1, 0),
				vertexAt(0, 0, tc.left, 0, 0), vertexAt(4, 1, tc.right, 1, 0), vertexAt(0, 1, tc.left, 1, 0),
			}
			op := driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Pixels: pixels}, vertices).Ops[0]
			out := Raster(&op, tex, 1)
			for x, want := range tc.want {
				if c := out.RGBAAt(x, 0); c != want {
					t.Errorf("pixel %d = %v, want %v", x, c, want)
				}
			}
		})
	}
}
