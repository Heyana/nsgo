//go:build ios

package watchos

import (
	"sync"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// CoreGraphics' interpolation qualities.
const (
	interpolationNone    = 1
	interpolationDefault = 0
)

var (
	stretchOnce sync.Once
	// The CoreGraphics functions a canvas drawn below the pixel ratio is
	// stretched with, which the apple package loads.
	cgContextSaveGState, cgContextRestoreGState, cgContextSetInterpolationQuality uintptr
)

func loadStretch() {
	sym := func(name string) uintptr {
		addr, _ := purego.Dlsym(purego.RTLD_DEFAULT, name)
		return addr
	}
	cgContextSaveGState = sym("CGContextSaveGState")
	cgContextRestoreGState = sym("CGContextRestoreGState")
	cgContextSetInterpolationQuality = sym("CGContextSetInterpolationQuality")
}

// playCanvas plays v's drawing, size points, into ctx at host's pixel
// ratio, or, at a ResolutionScale below 1, into a layer of that share of
// its pixels, which it stretches over the canvas smoothly or to the
// nearest pixel (Drawing.Upscale). A drawing whose images are loading
// plays at the pixel ratio until they load.
func (s *surface) playCanvas(ctx uintptr, v *view, size driver.Size, host apple.CanvasHost) {
	ops := s.animator.CanvasOps(v.id, v.drawing)
	scale := v.drawing.ResolutionScale()
	w, h := float64(size.Width), float64(size.Height)
	if scale < 1 && len(ops) > 0 && w > 0 && h > 0 {
		host.Ratio *= scale
		if l := apple.NewCanvasLayer(ctx, v.drawing, ops, driver.CanvasSplit{At: len(ops)}, w, h, host.Ratio, host); l != nil {
			defer l.Release()
			stretchOnce.Do(loadStretch)
			if cgContextSaveGState == 0 || cgContextRestoreGState == 0 || cgContextSetInterpolationQuality == 0 {
				l.Draw(ctx, w, h)
				return
			}
			purego.SyscallN(cgContextSaveGState, ctx)
			quality := interpolationDefault
			if !v.drawing.Smooth() {
				quality = interpolationNone
			}
			purego.SyscallN(cgContextSetInterpolationQuality, ctx, uintptr(quality))
			l.Draw(ctx, w, h)
			purego.SyscallN(cgContextRestoreGState, ctx)
			return
		}
		host.Ratio /= scale
	}
	apple.Play(ctx, ops, host)
}
