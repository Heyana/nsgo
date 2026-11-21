//go:build darwin

package apple

import (
	"slices"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type videoHitTest struct {
	output, item  objc.ID
	tried         bool
	pixels        []byte
	width, height int
}

// SampleHitTestAlpha reads the latest frame for macOS's painted-window timer.
// It returns true when a new mask may change the hit under a still pointer.
func (v *Video) SampleHitTestAlpha() bool {
	if v.layer == 0 || v.player == 0 || !v.status.loaded {
		return false
	}
	a := &v.hitTest
	if !a.tried {
		v.makeHitTestOutput()
	}
	if a.output == 0 {
		return false
	}
	at := objc.Send[cmTime](a.output, sel("itemTimeForHostTime:"), caCurrentMediaTime())
	if !objc.Send[bool](a.output, sel("hasNewPixelBufferForItemTime:"), at) {
		return false
	}
	buffer := objc.Send[uintptr](a.output, sel("copyPixelBufferForItemTime:itemTimeForDisplay:"), at, uintptr(0))
	if buffer == 0 {
		return false
	}
	defer Release(buffer)
	const readOnly = 1 // kCVPixelBufferLock_ReadOnly
	if cvPixelBufferFormat(buffer) != pixelBGRA || cvPixelBufferLock(buffer, readOnly) != 0 {
		return false
	}
	defer cvPixelBufferUnlock(buffer, readOnly)
	width, height := cvPixelBufferWidth(buffer), cvPixelBufferHeight(buffer)
	address, stride := cvPixelBufferBase(buffer), int(cvPixelBufferRow(buffer))
	if address == 0 || width <= 0 || height <= 0 || stride < width*4 {
		return false
	}
	data := *(*unsafe.Pointer)(unsafe.Pointer(&address))
	bgra := unsafe.Slice((*byte)(data), stride*height)
	a.width, a.height = width, height
	a.pixels = slices.Grow(a.pixels[:0], width*height)[:width*height]
	for y := range height {
		row := bgra[y*stride : y*stride+width*4]
		alpha := a.pixels[y*width : (y+1)*width]
		for x := range alpha {
			alpha[x] = row[x*4+3]
		}
	}
	return true
}

func (v *Video) makeHitTestOutput() {
	a := &v.hitTest
	a.tried = true
	item := v.player.Send(sel("currentItem"))
	tracks := item.Send(sel("asset")).Send(sel("tracksWithMediaType:"), NSString("vide"))
	if objc.Send[uint](tracks, sel("count")) == 0 {
		return
	}
	track := tracks.Send(sel("objectAtIndex:"), uint(0))
	if m := objc.Send[affine](track, sel("preferredTransform")); m != (affine{A: 1, D: 1}) {
		return
	}
	pixelBuffersLoaded.Do(loadPixelBuffers)
	format := objc.ID(Class("NSNumber")).Send(sel("numberWithUnsignedInt:"), pixelBGRA)
	attributes := objc.ID(Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), format, objc.ID(pixelFormatKey))
	output := objc.ID(Class("AVPlayerItemVideoOutput")).Send(sel("alloc")).Send(sel("initWithPixelBufferAttributes:"), attributes)
	if output != 0 {
		item.Send(sel("addOutput:"), output)
		a.item, a.output = item, output
	}
}

// ClearHitTestAlpha releases the frame output when macOS stops sampling.
func (v *Video) ClearHitTestAlpha() {
	a := &v.hitTest
	if a.output != 0 {
		a.item.Send(sel("removeOutput:"), a.output)
		a.output.Send(sel("release"))
	}
	*a = videoHitTest{}
}

// HitTestAlpha reports whether a cached video pixel takes the pointer in
// a painted macOS window. Before the first mask, the whole view counts.
func (v *Video) HitTestAlpha(p driver.Point) bool {
	a := &v.hitTest
	if len(a.pixels) == 0 || v.layer == 0 {
		return true
	}
	fit := objc.Send[CGRect](v.layer, sel("videoRect"))
	if fit.Size.Width <= 0 || fit.Size.Height <= 0 {
		return true
	}
	view := objc.Send[CGRect](v.holder, sel("bounds"))
	box := driver.Bounds{Size: driver.Size{Width: driver.Points(view.Size.Width), Height: driver.Points(view.Size.Height)}}
	if !box.Contains(p) {
		return false
	}
	bounds := driver.Bounds{
		Origin: driver.Point{X: driver.Points(fit.Origin.X), Y: driver.Points(fit.Origin.Y)},
		Size:   driver.Size{Width: driver.Points(fit.Size.Width), Height: driver.Points(fit.Size.Height)},
	}
	x, y, ok := driver.ImagePixelAt(bounds, p, a.width, a.height)
	return ok && float32(a.pixels[y*a.width+x])/255 >= v.config.HitTestAlpha
}
