//go:build darwin

package apple

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// RenderDrawing plays a recording into owned RGBA pixels without a window.
// Its synchronous image resolver has no access to the view caches or main queue.
func RenderDrawing(d *driver.Drawing, size driver.Size, scale float64) (*image.RGBA, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r := &drawingRenderer{ctx: ctx, images: map[renderImageKey]*loadedImage{}, visiting: map[*driver.OffscreenCanvas]bool{}}
	defer func() {
		for _, img := range r.images {
			Release(img.image)
		}
	}()
	bitmap, width, height, err := r.bitmap(d, size, scale)
	if err != nil {
		return nil, err
	}
	defer Release(bitmap)
	pixels := cgOffscreenBitmapData(bitmap)
	if pixels == nil {
		return nil, errors.New("apple: bitmap has no pixels")
	}
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	copy(out.Pix, unsafe.Slice((*byte)(pixels), len(out.Pix)))
	return out, nil
}

type drawingRenderer struct {
	ctx      context.Context
	images   map[renderImageKey]*loadedImage
	visiting map[*driver.OffscreenCanvas]bool
	err      error
}

type renderImageKey struct {
	source string
	ratio  float64
}

func (r *drawingRenderer) bitmap(d *driver.Drawing, size driver.Size, scale float64) (uintptr, int, int, error) {
	if d == nil {
		return 0, 0, 0, errors.New("apple: no drawing")
	}
	if len(d.Slots) != 0 || len(d.ViewPaints) != 0 {
		return 0, 0, 0, errors.New("apple: drawing still depends on a view's animations or paints")
	}
	w, h := math.Ceil(float64(size.Width)*scale), math.Ceil(float64(size.Height)*scale)
	if !(scale > 0) || math.IsInf(scale, 0) || !(w > 0) || !(h > 0) || w*h > maxOffscreenPixels {
		return 0, 0, 0, errors.New("apple: render size and scale must be finite, positive and at most 16 Mi pixels")
	}
	width, height := int(w), int(h)
	const premultipliedLast = 1
	bitmap := cgOffscreenBitmapCreate(0, uintptr(width), uintptr(height), 8, uintptr(width*4), srgb, premultipliedLast)
	if bitmap == 0 {
		return 0, 0, 0, errors.New("apple: cannot create the render bitmap")
	}
	cgContextClearRect(bitmap, Rect(0, 0, w, h))
	cgContextTranslateCTM(bitmap, 0, h)
	cgContextScaleCTM(bitmap, scale, -scale)
	Play(bitmap, d, d.Ops, CanvasHost{Ratio: scale, BaseUp: true, image: r.image})
	if r.err != nil {
		Release(bitmap)
		return 0, 0, 0, r.err
	}
	return bitmap, width, height, nil
}

func (r *drawingRenderer) image(src driver.ImageSource, ratio float64) *loadedImage {
	if r.err != nil {
		return nil
	}
	key := renderImageKey{source: src.Key(), ratio: max(ratio, 1)}
	if img := r.images[key]; img != nil {
		// The same resource can be drawn at different logical sizes.
		copy := *img
		if src.Size.Width > 0 && src.Size.Height > 0 {
			copy.points = src.Size
		}
		return &copy
	}
	var d decoded
	if off := src.Offscreen; off != nil {
		if r.visiting[off] {
			r.err = errors.New("apple: offscreen drawing contains a cycle")
			return nil
		}
		r.visiting[off] = true
		bitmap, width, height, err := r.bitmap(off.Drawing, off.Size, float64(off.EffectiveScale()))
		delete(r.visiting, off)
		if err != nil {
			r.err = err
			return nil
		}
		d = decoded{image: cgBitmapContextCreateImage(bitmap), width: float64(width), height: float64(height), points: off.Size}
		Release(bitmap)
	} else if src.Pixels != nil {
		d = pixelsImage(src, request{})
	} else {
		data, err := imageload.Load(r.ctx, src, nil)
		if err == nil {
			d, err = decode(r.ctx, data, request{mode: driver.Stretch, ratio: key.ratio, scale: uriScale(src.URI)})
		}
		if err != nil {
			d.release()
			r.err = fmt.Errorf("apple: render image: %w", err)
			return nil
		}
	}
	if d.image == 0 {
		r.err = errors.New("apple: cannot render image")
		return nil
	}
	for _, frame := range d.frames[min(len(d.frames), 1):] {
		Release(frame)
	}
	img := &loadedImage{image: d.image, width: d.width, height: d.height, points: d.points}
	r.images[key] = img
	if src.Size.Width > 0 && src.Size.Height > 0 {
		copy := *img
		copy.points = src.Size
		return &copy
	}
	return img
}
