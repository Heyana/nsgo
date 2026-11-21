//go:build darwin

package apple

import (
	"math"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type pipVideo struct {
	owner         *PictureInPicture
	wake          func()
	output, layer objc.ID
	size          CGSize
	transform     affine
	until         time.Time
}

var (
	pipVideoOnce   sync.Once
	pixelFormatKey uintptr
	imageFromPixel func(uintptr, uintptr, *uintptr) int32
)

func loadPiPVideo() {
	cv := load("/System/Library/Frameworks/CoreVideo.framework/CoreVideo")
	vt := load("/System/Library/Frameworks/VideoToolbox.framework/VideoToolbox")
	pixelFormatKey = constant(cv, "kCVPixelBufferPixelFormatTypeKey")
	purego.RegisterLibFunc(&imageFromPixel, vt, "VTCreateCGImageFromCVPixelBuffer")
}

// SetPictureInPictureOwner selects the custom PiP tree that renders this
// video, or nil when the video returns to the ordinary view hierarchy.
func (v *Video) SetPictureInPictureOwner(owner *PictureInPicture, wake func()) {
	if v.frame.owner == owner {
		return
	}
	v.closePiPVideo()
	v.frame.owner, v.frame.wake = owner, wake
	if owner == nil {
		v.frame.wake = nil
	}
	v.show()
	v.openPiPVideo()
	v.applyGravity()
	v.configurePiP()
}

func (v *Video) openPiPVideo() {
	if v.frame.owner == nil || v.player == 0 {
		return
	}
	item := v.player.Send(sel("currentItem"))
	if item == 0 {
		return
	}
	pipVideoOnce.Do(loadPiPVideo)
	const bgra = 0x42475241
	format := int32Number(bgra)
	attrs := dictionary([]uintptr{pixelFormatKey}, []uintptr{format})
	output := objc.ID(Class("AVPlayerItemVideoOutput")).Send(sel("alloc")).Send(sel("initWithPixelBufferAttributes:"), attrs)
	Release(attrs)
	Release(format)
	if output == 0 {
		return
	}
	output.Send(sel("setSuppressesPlayerRendering:"), true)
	item.Send(sel("addOutput:"), output)
	v.frame.output = output
	v.setPiPBackgroundPolicy(true)
	v.requestPiPVideoFrame()
}

func (v *Video) setPiPBackgroundPolicy(continues bool) {
	policy := sel("setAudiovisualBackgroundPlaybackPolicy:")
	if v.player != 0 && objc.Send[bool](v.player, sel("respondsToSelector:"), policy) {
		const automatic, continuesIfPossible = 1, 3
		value := automatic
		if continues {
			value = continuesIfPossible
		}
		v.player.Send(policy, value)
	}
}

func (v *Video) closePiPVideo() {
	f := &v.frame
	if f.output != 0 {
		v.player.Send(sel("currentItem")).Send(sel("removeOutput:"), f.output)
		f.output.Send(sel("release"))
		v.setPiPBackgroundPolicy(false)
	}
	if f.layer != 0 {
		f.layer.Send(sel("removeFromSuperlayer"))
		f.layer.Send(sel("release"))
		f.owner.Dirty()
	}
	*f = pipVideo{owner: f.owner, wake: f.wake}
}

func (v *Video) requestPiPVideoFrame() {
	if v.frame.output != 0 {
		// A paused item can need another tick as it decodes its first frame
		// after loading or seeking. Resume and seek completion can wake it again.
		v.frame.until = time.Now().Add(2 * time.Second)
		if v.frame.wake != nil {
			v.frame.wake()
		}
	}
}

// UpdatePictureInPictureFrame brings the detached video layer up to the
// player's current time. It returns whether another video tick is needed.
func (v *Video) UpdatePictureInPictureFrame(background bool) bool {
	f := &v.frame
	if f.output == 0 || f.owner.closing || (background && !f.owner.ContinuesInBackground()) || v.status.failed {
		return false
	}
	at := objc.Send[cmTime](v.player, sel("currentTime"))
	if objc.Send[bool](f.output, sel("hasNewPixelBufferForItemTime:"), at) {
		if pixel := f.output.Send(sel("copyPixelBufferForItemTime:itemTimeForDisplay:"), at, uintptr(0)); pixel != 0 {
			v.showPiPVideoFrame(uintptr(pixel))
			Release(uintptr(pixel))
		}
	}
	return v.status.play.playing || time.Now().Before(f.until)
}

func (v *Video) showPiPVideoFrame(pixel uintptr) {
	var image uintptr
	if imageFromPixel(pixel, 0, &image) != 0 || image == 0 {
		return
	}
	defer Release(image)
	f := &v.frame
	if f.layer == 0 {
		f.layer = objc.ID(Class("CALayer")).Send(sel("new"))
		f.transform = pipVideoTransform(v.player.Send(sel("currentItem")))
		v.holder.Send(sel("layer")).Send(sel("addSublayer:"), f.layer)
	}
	size := CGSize{Width: float64(cgImageGetWidth(image)), Height: float64(cgImageGetHeight(image))}
	if f.size != size {
		f.size = size
		v.layoutPiPVideo()
	}
	f.layer.Send(sel("setContents:"), image)
	f.until = time.Time{}
	f.owner.Dirty()
}

func pipVideoTransform(item objc.ID) affine {
	tracks := item.Send(sel("tracks"))
	for i, count := uintptr(0), objc.Send[uintptr](tracks, sel("count")); i < count; i++ {
		track := tracks.Send(sel("objectAtIndex:"), i)
		if objc.Send[bool](track, sel("isEnabled")) {
			asset := track.Send(sel("assetTrack"))
			if GoString(asset.Send(sel("mediaType"))) == "vide" {
				return objc.Send[affine](asset, sel("preferredTransform"))
			}
		}
	}
	return affine{A: 1, D: 1}
}

func (v *Video) layoutPiPVideo() {
	f := &v.frame
	if f.layer == 0 {
		return
	}
	m := f.transform
	w, h := f.size.Width, f.size.Height
	rotatedW, rotatedH := math.Abs(m.A)*w+math.Abs(m.C)*h, math.Abs(m.B)*w+math.Abs(m.D)*h
	if rotatedW == 0 || rotatedH == 0 {
		return
	}
	bounds := objc.Send[CGRect](v.holder, sel("bounds"))
	sx, sy := bounds.Size.Width/rotatedW, bounds.Size.Height/rotatedH
	if v.config.ResizeMode == driver.Cover {
		sx, sy = max(sx, sy), max(sx, sy)
	} else if v.config.ResizeMode != driver.Stretch {
		sx, sy = min(sx, sy), min(sx, sy)
	}
	// The layer is anchored at its center, so only the linear part of
	// the track transform is needed to orient it in the holder.
	WithoutAnimation(func() {
		f.layer.Send(sel("setBounds:"), Rect(0, 0, w, h))
		f.layer.Send(sel("setPosition:"), CGPoint{X: bounds.Size.Width / 2, Y: bounds.Size.Height / 2})
		f.layer.Send(sel("setAffineTransform:"), affine{A: sx * m.A, B: sy * m.B, C: sx * m.C, D: sy * m.D})
	})
}
