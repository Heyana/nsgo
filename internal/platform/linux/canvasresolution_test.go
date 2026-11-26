//go:build linux && !android

package linux

import (
	"image"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// TestCanvasResolutionScale rasterizes canvastest.Resolution into half the
// pixels a side at canvastest.Scale and draws its textures, as a frame
// appends them, stretched over the canvas with each sampling.
func TestCanvasResolutionScale(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	for _, sampling := range []driver.ImageSampling{driver.ImageSamplingSmooth, driver.ImageSamplingNearest} {
		t.Run(map[driver.ImageSampling]string{driver.ImageSamplingSmooth: "smooth", driver.ImageSamplingNearest: "nearest"}[sampling], func(t *testing.T) {
			w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: canvastest.Scale}}
			v := &view{drawing: canvastest.Resolution(w, sampling)}
			v.canvas.whole = true
			const side = canvastest.ResolutionSize
			pw, ph, scale := w.canvasPixels(v, side, side)
			if pw != canvastest.ResolutionPixels || ph != canvastest.ResolutionPixels || scale != canvastest.Scale/2 {
				t.Fatalf("backing store %d × %d at %v, want %d a side at %v", pw, ph, scale, canvastest.ResolutionPixels, canvastest.Scale/2)
			}
			w.rasterize(v, pw, ph, scale)
			defer v.canvas.raster.release()

			// The node is laid out in display pixels: GSK's Cairo fallback
			// stretches a texture into an image in user units and paints
			// that blended, so a scaled context would blur nearest too.
			snap := gtkSnapshotNew()
			appendTiles(snap, v.canvas.raster, scale/canvastest.Scale, true, v.drawing.Smooth())
			node := gtkSnapshotFreeToNode(snap)
			defer gskRenderNodeUnref(node)
			const px = side * canvastest.Scale
			surface := cairoImageSurfaceCreate(0, px, px)
			defer cairoSurfaceDestroy(surface)
			cr := cairoCreate(surface)
			gskRenderNodeDraw(node, cr)
			cairoDestroy(cr)
			cairoSurfaceFlush(surface)
			img := image.NewRGBA(image.Rect(0, 0, px, px))
			data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
			rows := unsafe.Slice((*byte)(cPointer(data)), stride*px)
			for y := range px {
				for x := range px {
					b := rows[y*stride+x*4:]
					img.Pix[y*img.Stride+x*4+0], img.Pix[y*img.Stride+x*4+1], img.Pix[y*img.Stride+x*4+2], img.Pix[y*img.Stride+x*4+3] = b[2], b[1], b[0], b[3]
				}
			}
			for _, f := range canvastest.CheckResolution(img, image.Point{}, sampling) {
				t.Error(f)
			}
		})
	}
}

// TestCanvasResolutionScaleOne keeps a canvas without a resolution scale
// at the window's pixels.
func TestCanvasResolutionScaleOne(t *testing.T) {
	w := &window{env: driver.Environment{PixelRatio: 3}}
	for _, r := range []float32{0, 1, 2, -1} {
		d := driver.NewDrawing(nil)
		d.Resolution = r
		pw, ph, scale := w.canvasPixels(&view{drawing: d}, 10.5, 4)
		if pw != 32 || ph != 12 || scale != 3 {
			t.Errorf("Resolution %v: %d × %d at %v, want 32 × 12 at 3", r, pw, ph, scale)
		}
	}
	d := driver.NewDrawing(nil)
	d.Resolution = 0.01
	if pw, ph, _ := w.canvasPixels(&view{drawing: d}, 10, 4); pw != 1 || ph != 1 {
		t.Errorf("a hundredth: %d × %d, want a pixel a side", pw, ph)
	}
}

// TestCanvasResolutionScaleNoSeam stretches a raster wider than a tile
// smoothly with no seam: columns one backing pixel wide alternate black
// and white, so every pixel inside blends them, where a texture's edge
// would clamp to one.
func TestCanvasResolutionScaleNoSeam(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	const width, height = 2*tile + 88, 8 // points, and pixels at a ratio of 1
	d := driver.NewDrawing(nil)
	d.Resolution = 0.5
	d.SetFillColor(driver.RGB(0xffffff)).FillRect(0, 0, width, height)
	for x := 0; x < width; x += 4 {
		d.SetFillColor(driver.RGB(0)).FillRect(float32(x), 0, 2, height)
	}
	w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: 1}}
	v := &view{drawing: d}
	v.canvas.whole = true
	pw, ph, scale := w.canvasPixels(v, width, height)
	if pw <= tile {
		t.Fatalf("backing store %d pixels wide, want more than a tile's %d", pw, tile)
	}
	w.rasterize(v, pw, ph, scale)
	defer v.canvas.raster.release()
	if n := len(v.canvas.raster.tiles); n != 1 {
		t.Fatalf("a stretched raster has %d textures, want 1", n)
	}

	snap := gtkSnapshotNew()
	appendTiles(snap, v.canvas.raster, scale, true, true)
	node := gtkSnapshotFreeToNode(snap)
	defer gskRenderNodeUnref(node)
	surface := cairoImageSurfaceCreate(0, width, height)
	defer cairoSurfaceDestroy(surface)
	cr := cairoCreate(surface)
	gskRenderNodeDraw(node, cr)
	cairoDestroy(cr)
	cairoSurfaceFlush(surface)
	data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
	rows := unsafe.Slice((*byte)(cPointer(data)), stride*height)
	for x := 1; x < width-1; x++ {
		if g := rows[height/2*stride+x*4+1]; g <= 16 || g >= 239 {
			t.Fatalf("pixel column %d = %d, want a blend: a seam", x, g)
		}
	}
}
