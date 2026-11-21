//go:build darwin

package apple

import (
	"slices"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type imageAlpha struct {
	pixels        []byte // top row first; nil for an opaque image
	width, height int
	row, step     int
}

func (d *decoded) prepareAlpha() {
	if d.image == 0 || d.alpha != nil {
		return
	}
	d.alpha = make(map[uintptr]imageAlpha, max(1, len(d.frames)))
	for _, frame := range d.frames {
		d.alpha[frame] = readImageAlpha(frame)
	}
	if len(d.frames) == 0 {
		d.alpha[d.image] = readImageAlpha(d.image)
	}
}

func readImageAlpha(image uintptr) imageAlpha {
	a := imageAlpha{width: cgImageGetWidth(image), height: cgImageGetHeight(image)}
	a.row, a.step = a.width, 1
	const alphaNone, alphaNoneSkipLast, alphaNoneSkipFirst, alphaOnly = 0, 5, 6, 7
	switch cgImageGetAlphaInfo(image) {
	case alphaNone, alphaNoneSkipLast, alphaNoneSkipFirst:
		return a
	}
	bitmapsLoaded.Do(loadBitmaps)
	ctx := cgBitmapContextCreate(0, uintptr(a.width), uintptr(a.height), 8, uintptr(a.row), 0, alphaOnly)
	if ctx == 0 {
		return a
	}
	defer Release(ctx)
	cgContextDrawImage(ctx, Rect(0, 0, float64(a.width), float64(a.height)), image)
	data := cgBitmapContextGetData(ctx)
	if data != 0 {
		ptr := *(*unsafe.Pointer)(unsafe.Pointer(&data))
		a.pixels = slices.Clone(unsafe.Slice((*byte)(ptr), a.row*a.height))
	}
	return a
}

func (iv *ImageView) testsAlpha() bool {
	return iv.AlphaChanged != nil && iv.config.HitTestAlpha > 0
}

var selImagePresentation = sel("presentationLayer")
var selImageContents = sel("contents")

// HitTest reports whether the picture at p takes the pointer and whether
// the sampled image is animated and may change at the next hit test.
func (iv *ImageView) HitTest(p driver.Point) (hit, animated bool) {
	if !iv.testsAlpha() {
		return true, false
	}
	box := driver.Bounds{Size: driver.Size{Width: driver.Points(iv.bounds.Width), Height: driver.Points(iv.bounds.Height)}}
	if !box.Contains(p) {
		return false, false
	}
	frame := iv.image
	animated = len(iv.shown.frames) > 1 && iv.tint == 0
	if animated {
		if layer := iv.picture.Send(selImagePresentation); layer != 0 {
			f := uintptr(objc.Send[objc.ID](layer, selImageContents))
			if _, ok := iv.shown.alpha[f]; ok {
				frame = f
			}
		}
	}
	a := iv.shown.alpha[frame]
	fit := iv.fitted()
	rect := driver.Bounds{
		Origin: driver.Point{X: driver.Points(fit.Origin.X), Y: driver.Points(fit.Origin.Y)},
		Size:   driver.Size{Width: driver.Points(fit.Size.Width), Height: driver.Points(fit.Size.Height)},
	}
	x, y, ok := driver.ImagePixelAt(rect, p, a.width, a.height)
	if !ok {
		return false, animated
	}
	return a.pixels == nil || float32(a.pixels[y*a.row+x*a.step])/255 >= iv.config.HitTestAlpha, animated
}
