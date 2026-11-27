//go:build linux && !android

package linux

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"math"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var captureNeedsFrame = errors.New("linux: the surface has no rendered frame")

func (w *window) capture(id driver.NodeID, count uint32) {
	dispatch := w.engine.Dispatch
	var take func(bool)
	take = func(retry bool) {
		img, err := w.capturePixels(id)
		if err == captureNeedsFrame && retry {
			// A batch that changes an image can invalidate the widget's
			// render node until GTK has painted its next frame.
			time.AfterFunc(50*time.Millisecond, func() { w.post(func() { take(false) }) })
			return
		}
		if err != nil {
			dispatch(driver.CaptureResult(count, nil, err))
			return
		}
		// Swizzle BGRA to RGBA and encode PNG off the GTK loop so capture
		// never blocks frames.
		go func() {
			// GDK's download uses Cairo's native-endian premultiplied ARGB.
			for i := 0; i < len(img.Pix); i += 4 {
				img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i]
			}
			var data bytes.Buffer
			err := png.Encode(&data, img)
			dispatch(driver.CaptureResult(count, data.Bytes(), err))
		}()
	}
	w.post(func() { take(true) })
}

// capturePixels snapshots the GTK surface, including its native children,
// shader widgets and GtkGLArea GPU views, crops to the requested widget's
// presentation bounds, and downloads the GdkTexture on the GTK main loop
// (required for GL/Vulkan-backed textures) before returning the raw BGRA image.
func (w *window) capturePixels(id driver.NodeID) (*image.RGBA, error) {
	tex, err := w.captureTexture(id)
	if err != nil {
		return nil, err
	}
	defer gObjectUnref(tex)
	width, height := int(gdkTextureGetWidth(tex)), int(gdkTextureGetHeight(tex))
	if width <= 0 || height <= 0 {
		return nil, errors.New("linux: capture has no pixels")
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	gdkTextureDownload(tex, unsafe.Pointer(&img.Pix[0]), uintptr(img.Stride))
	return img, nil
}

// captureTexture snapshots the GTK surface, including its native children
// and shader widgets, and crops to the requested widget's presentation bounds.
func (w *window) captureTexture(id driver.NodeID) (uintptr, error) {
	failure := errors.New("linux: cannot capture the surface")
	if w.surface == 0 {
		return 0, failure
	}
	bindEffects()
	width, height := float64(gtkWidgetGetWidth(w.surface)), float64(gtkWidgetGetHeight(w.surface))
	region := grapheneRect{0, 0, float32(width), float32(height)}
	if id != 0 {
		v := w.view(id)
		if v == nil || v.widget == 0 || gtkWidgetComputeBounds(v.widget, w.surface, &region) == 0 {
			return 0, errors.New("linux: capture view is outside the surface")
		}
	}
	r := &region
	scale := float64(max(gtkWidgetGetScaleFactorG(w.surface), 1))
	left := math.Floor(max(float64(r.x), 0) * scale)
	top := math.Floor(max(float64(r.y), 0) * scale)
	right := math.Ceil(min(float64(r.x+r.w), width) * scale)
	bottom := math.Ceil(min(float64(r.y+r.h), height) * scale)
	if right <= left || bottom <= top {
		return 0, errors.New("linux: capture has no visible pixels")
	}
	native := gtkWidgetGetNative(w.surface)
	if native == 0 {
		return 0, failure
	}
	renderer := gtkNativeGetRenderer(native)
	if renderer == 0 {
		return 0, failure
	}
	paintable := gtkWidgetPaintableNew(w.surface)
	defer gObjectUnref(paintable)
	snap := gtkSnapshotNew()
	gtkSnapshotScale(snap, float32(scale), float32(scale))
	gdkPaintableSnapshot(paintable, snap, width, height)
	node := gtkSnapshotFreeToNode(snap)
	if node == 0 {
		return 0, captureNeedsFrame
	}
	defer gskRenderNodeUnref(node)
	tex := gskRendererRenderTexture(renderer, node, &grapheneRect{float32(left), float32(top), float32(right - left), float32(bottom - top)})
	if tex == 0 {
		return 0, failure
	}
	return tex, nil
}
