//go:build windows

package windows

// Images decode with imagedecode: PNG, JPEG, GIF, WebP and SVG, an SVG
// drawn oversampled (4 pixels a point, at most 2048 a side) as NSH's
// Windows backend draws it, so it stays sharp however its view scales it.
// An animated GIF or WebP plays its frames on the frame clock with their
// delays and loop count, each view on its own; tinted, it shows its first
// frame, as NSH's does.

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

// progressInterval spaces an image's progress reports.
const progressInterval = 100 * time.Millisecond

// picture is a decoded image: its frames of premultiplied BGRA rows, top
// to bottom, more than one for an animation.
type picture struct {
	width, height int
	frames        []imagedecode.Frame
	plays         int // how many times an animation plays; 0 for ever
	// size is its intrinsic size: an SVG's in points, a bitmap's pixels.
	size driver.Size
	svg  bool
}

// animated reports whether the picture has frames to play.
func (p *picture) animated() bool { return len(p.frames) > 1 }

// frameAt returns the frame an animation shows elapsed after it started,
// and whether it plays on after that.
func (p *picture) frameAt(elapsed time.Duration) (int, bool) {
	var total time.Duration
	for _, f := range p.frames {
		total += f.Delay
	}
	if total <= 0 {
		return 0, false
	}
	if p.plays > 0 && elapsed >= total*time.Duration(p.plays) {
		return len(p.frames) - 1, false
	}
	t := elapsed % total
	for i, f := range p.frames {
		if t < f.Delay {
			return i, true
		}
		t -= f.Delay
	}
	return len(p.frames) - 1, true
}

// imageSlot is an image view's source or placeholder and what the window
// keeps for it.
type imageSlot struct {
	used uint64 // the last canvas draw that used this slot
	src  driver.ImageSource
	load *imageLoad // the load under way, if any
	pic  *picture
	// frame is the frame an animation shows, playing since start while
	// playing is set; ended once it played its loops.
	frame          int
	start          time.Time
	playing, ended bool
	// bitmap is frame as a Direct2D bitmap, tinted with tint, made in
	// generation.
	bitmap                    *comObject
	bitmapFrame               int
	bitmapWidth, bitmapHeight int
	tint                      driver.Color
	generation                uint64
	// blur is the Gaussian blur effect a blurred picture draws through,
	// made in blurGeneration.
	blur           *comObject
	blurGeneration uint64
	// A source that loaded fades in over fade from fadeAt, over the
	// placeholder.
	fadeAt time.Time
	fade   time.Duration
}

type imageLoad struct{ cancel context.CancelFunc }

func (s *imageSlot) empty() bool { return s.src.Empty() }

// reset stops the slot's load and frees its picture, for src.
func (s *imageSlot) reset(src driver.ImageSource) {
	if s.load != nil {
		s.load.cancel()
		s.load = nil
	}
	releaseAll(&s.bitmap, &s.blur)
	s.pic = nil
	s.src = src
	s.frame, s.playing, s.ended = 0, false, false
	s.fade = 0
}

// showPixels makes src's pixels the slot's picture, in place of the one it shows: no load, no blank, no fade. The converted pixels stay in the slot so the Direct2D bitmap can be updated in place.
func (s *imageSlot) showPixels(src driver.ImageSource) {
	if s.load != nil {
		s.load.cancel()
		s.load = nil
	}
	p := src.Pixels
	w, h := p.Rect.Dx(), p.Rect.Dy()
	var pix []byte
	if s.pic != nil && s.pic.width == w && s.pic.height == h && len(s.pic.frames) == 1 && len(s.pic.frames[0].Pix) == w*h*4 {
		pix = s.pic.frames[0].Pix
	} else {
		pix = make([]byte, w*h*4)
	}
	for y := range h {
		from := p.PixOffset(p.Rect.Min.X, p.Rect.Min.Y+y)
		to := y * w * 4
		for x := range w {
			src := p.Pix[from+x*4:]
			dst := pix[to+x*4:]
			dst[0], dst[1], dst[2], dst[3] = src[2], src[1], src[0], src[3]
		}
	}
	s.pic = &picture{width: w, height: h, frames: []imagedecode.Frame{{Pix: pix}}, size: driver.Size{Width: driver.Points(w), Height: driver.Points(h)}}
	s.src = src
	s.frame, s.playing, s.ended = 0, false, false
	s.bitmapFrame = -1
	s.fade = 0
}

// alpha returns the opacity s's fade-in shows it at by now: 1 once over.
func (s *imageSlot) alpha(now time.Time) float32 {
	if s.fade <= 0 {
		return 1
	}
	return float32(min(float64(now.Sub(s.fadeAt))/float64(s.fade), 1))
}

func sameSource(a, b driver.ImageSource) bool {
	return a.URI == b.URI && a.Size == b.Size && len(a.Data) == len(b.Data) &&
		(len(a.Data) == 0 || &a.Data[0] == &b.Data[0]) && a.Pixels == b.Pixels && a.Offscreen.Same(b.Offscreen) && a.Version == b.Version
}

// setImage applies an image view's configuration and reports whether a
// source changed, to load once the batch is applied and the view's
// events are known.
func (w *window) setImage(v *view, c *driver.ImageConfig) bool {
	v.image = *c
	changed := false
	switch {
	case sameSource(v.source.src, c.Source):
	case c.Source.Pixels != nil:
		v.source.showPixels(c.Source)
		w.redrawView(v)
	case c.Source.Offscreen != nil:
		v.source.reset(c.Source)
		w.redrawView(v)
	default:
		v.source.reset(c.Source)
		changed = true
	}
	if !sameSource(v.placeholder.src, c.Placeholder) {
		v.placeholder.reset(c.Placeholder)
		changed = true
	}
	return changed
}

// load fetches and decodes s's source on a goroutine of its own and hands
// the picture to the window thread. Only the source, not the placeholder,
// reports its loading.
func (w *window) load(v *view, s *imageSlot, report bool) {
	if s.load != nil || s.pic != nil || s.empty() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ld := &imageLoad{cancel: cancel}
	s.load = ld
	if report {
		w.report(v, driver.Event{Kind: driver.EventLoadStart, ID: v.id})
	}
	var progress imageload.Progress
	if report && v.events.Has(driver.EventProgress) {
		var last time.Time // the fetching goroutine's
		progress = func(loaded, total int64) {
			if now := time.Now(); now.Sub(last) >= progressInterval || loaded == total {
				last = now
				w.post(func() {
					if s.load == ld {
						w.report(v, driver.Event{Kind: driver.EventProgress, ID: v.id, Loaded: loaded, Total: total})
					}
				})
			}
		}
	}
	src := s.src
	go func() {
		data, err := imageload.Load(ctx, src, progress)
		var pic *picture
		if err == nil {
			pic, err = decode(ctx, data)
		}
		if ctx.Err() != nil {
			return // replaced or deleted
		}
		w.post(func() {
			if s.load != ld {
				return
			}
			s.load = nil
			cancel()
			if err != nil {
				if report {
					w.report(v, driver.Event{Kind: driver.EventError, ID: v.id, Text: err.Error()})
					w.report(v, driver.Event{Kind: driver.EventLoadEnd, ID: v.id})
				}
				return
			}
			s.pic = pic
			w.play(v, s)
			if report {
				// An SVG's size is its intrinsic one in whole points.
				size := driver.Size{Width: driver.Points(math.Round(float64(pic.size.Width))),
					Height: driver.Points(math.Round(float64(pic.size.Height)))}
				w.report(v, driver.Event{Kind: driver.EventLoad, ID: v.id, Size: size})
				w.report(v, driver.Event{Kind: driver.EventLoadEnd, ID: v.id})
				w.fadeIn(v, s)
			}
			w.redrawView(v)
		})
	}()
}

// report dispatches ev when v still exists and its events hold ev's kind.
func (w *window) report(v *view, ev driver.Event) {
	if !v.deleted && v.events.Has(ev.Kind) {
		w.engine.Dispatch(ev)
	}
}

// decode decodes an image into premultiplied BGRA frames, an SVG drawn
// oversampled, in one of imagedecode's slots; it returns ctx's error if
// ctx ends first.
func decode(ctx context.Context, data []byte) (*picture, error) {
	done, err := imagedecode.Wait(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	img, err := imagedecode.Decode(data, imagedecode.Options{Layout: imagedecode.BGRA})
	if err != nil {
		return nil, err
	}
	return &picture{width: img.Width, height: img.Height, frames: img.Frames, plays: img.Plays, size: img.Size,
		svg: img.Format == imagedecode.SVG}, nil
}

// play plays s's picture in v from its first frame when it is animated
// and v does not tint it, until it ends, v goes or s changes.
func (w *window) play(v *view, s *imageSlot) {
	pic := s.pic
	if pic == nil || !pic.animated() || s.playing || s.ended || visible(v.style.TintColor) {
		return
	}
	s.frame, s.start, s.playing = 0, time.Now(), true
	w.onFrame(func(now time.Time) bool {
		if s.pic != pic || v.deleted || !s.playing {
			return false
		}
		if s == &v.placeholder && v.source.pic != nil {
			s.playing = false // the source covers it
			return false
		}
		if visible(v.style.TintColor) {
			// Tinted, it shows its first frame until the tint goes.
			s.frame, s.playing = 0, false
			w.redrawView(v)
			return false
		}
		frame, more := pic.frameAt(now.Sub(s.start))
		if frame != s.frame {
			s.frame = frame
			w.redrawView(v)
		}
		s.playing, s.ended = more, !more
		return more
	})
}

func (s *imageSlot) shownFrame(tint driver.Color) int {
	if tint&0xff != 0 || s.frame >= len(s.pic.frames) {
		return 0
	}
	return s.frame
}

// bitmapOf returns the frame s shows as a Direct2D bitmap with tint
// applied, made once per frame, tint and device; nil until the picture
// arrives. A tinted animation shows its first frame.
func (s *imageSlot) bitmapOf(dc *comObject, tint driver.Color) *comObject {
	if s.pic == nil {
		return nil
	}
	frame := s.shownFrame(tint)
	if s.bitmap != nil {
		if s.tint == tint && s.generation == gfxGeneration && s.bitmapFrame == frame {
			return s.bitmap
		}
		pix := s.pic.frames[frame].Pix
		if s.generation == gfxGeneration && s.bitmapWidth == s.pic.width && s.bitmapHeight == s.pic.height {
			if tint&0xff != 0 {
				pix = make([]byte, len(pix))
				tintInto(pix, s.pic.frames[frame].Pix, tint)
			}
			if !failed(s.bitmap.call(bitmapCopyFromMemory, 0, uintptr(unsafe.Pointer(&pix[0])), uintptr(s.pic.width*4))) {
				s.tint, s.bitmapFrame = tint, frame
				return s.bitmap
			}
		}
		releaseAll(&s.bitmap)
	}
	pix := s.pic.frames[frame].Pix
	if tint&0xff != 0 {
		pix = make([]byte, len(pix))
		tintInto(pix, s.pic.frames[frame].Pix, tint)
	}
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96, dpiY: 96}
	if failed(dc.callSize(dcCreateBitmap, uint32(s.pic.width), uint32(s.pic.height), uintptr(unsafe.Pointer(&pix[0])),
		uintptr(s.pic.width*4), uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&s.bitmap)))) {
		s.bitmap = nil
		return nil
	}
	s.tint, s.generation, s.bitmapFrame = tint, gfxGeneration, frame
	s.bitmapWidth, s.bitmapHeight = s.pic.width, s.pic.height
	return s.bitmap
}

// tintInto copies premultiplied pixels from src to dst with their colors
// replaced by tint's, keeping their alpha.
func tintInto(dst, src []byte, tint driver.Color) {
	r, g, b := uint32(tint>>24&0xff), uint32(tint>>16&0xff), uint32(tint>>8&0xff)
	for i := 0; i+3 < len(src); i += 4 {
		a := uint32(src[i+3])
		dst[i], dst[i+1], dst[i+2], dst[i+3] = byte(b*a/255), byte(g*a/255), byte(r*a/255), byte(a)
	}
}

// fadeIn fades s, v's source that just loaded, in over the placeholder
// for the view's fade duration, on the frame clock.
func (w *window) fadeIn(v *view, s *imageSlot) {
	fade := v.image.FadeDuration
	if fade <= 0 {
		return
	}
	pic := s.pic
	s.fadeAt, s.fade = time.Now(), fade
	w.onFrame(func(now time.Time) bool {
		if v.deleted || s.pic != pic || s.fade <= 0 {
			return false
		}
		w.redrawView(v)
		if s.alpha(now) >= 1 {
			s.fade = 0
			return false
		}
		return true
	})
}

func (w *window) imageBox(style *driver.ViewStyle, width, height float32) rectF {
	bw := w.borderEdges(style)
	return rectF{bw[3], bw[0], width - bw[1], height - bw[2]}
}

// imageHit uses the alpha of the source or placeholder at p, in the
// same fitted rectangle and frame that drawImage uses.
func (w *window) imageHit(v *view, size driver.Size, p driver.Point) bool {
	style := v.shownStyle()
	if driver.Painted(driver.KindView, style, 0, 0) {
		return true
	}
	box := w.imageBox(style, float32(size.Width), float32(size.Height))
	if box.empty() || float32(p.X) < box.left || float32(p.X) >= box.right ||
		float32(p.Y) < box.top || float32(p.Y) >= box.bottom {
		return false
	}
	s := &v.source
	if s.pic == nil {
		s = &v.placeholder
	}
	if s.pic == nil {
		return false
	}
	d := w.fit(v, s, box)
	fit := driver.Bounds{
		Origin: driver.Point{X: driver.Points(d.left), Y: driver.Points(d.top)},
		Size:   driver.Size{Width: driver.Points(d.right - d.left), Height: driver.Points(d.bottom - d.top)},
	}
	x, y, ok := driver.ImagePixelAt(fit, p, s.pic.width, s.pic.height)
	if !ok {
		return false
	}
	pix := s.pic.frames[s.shownFrame(style.TintColor)].Pix
	return float32(pix[(y*s.pic.width+x)*4+3])/255 >= v.image.HitTestAlpha
}

// drawImage paints v in r (points): its placeholder until the source
// arrives or while the source fades in over it, then the source, each as
// v's resize mode and position fit it, blurred by its blur radius.
func (p *painter) drawImage(v *view, tint driver.Color, r rectF) {
	if r.empty() {
		return
	}
	if v.source.src.Offscreen != nil {
		p.pushClipRect(r)
		defer p.pop()
		p.drawOffscreenImage(v, &v.source, r)
		return
	}
	alpha := float32(1)
	if v.source.pic != nil {
		alpha = v.source.alpha(time.Now())
	}
	p.pushClipRect(r)
	defer p.pop()
	if v.source.pic == nil || alpha < 1 {
		p.drawPicture(v, &v.placeholder, tint, r)
	}
	if v.source.pic == nil || alpha <= 0 {
		return
	}
	if alpha < 1 {
		p.pushLayer(alpha, nil)
	}
	p.drawPicture(v, &v.source, tint, r)
	if alpha < 1 {
		p.pop()
	}
}

func imageInterpolation(sampling driver.ImageSampling) uintptr {
	if sampling == driver.ImageSamplingNearest {
		return d2dInterpolationNearest
	}
	return d2dInterpolationLinear
}

// drawPicture draws s's picture fitted in r. A blurred one draws through
// a Direct2D Gaussian blur with hard borders, so its edges stay opaque,
// under a clip to its own rectangle.
func (p *painter) drawPicture(v *view, s *imageSlot, tint driver.Color, r rectF) {
	if s.pic == nil {
		return
	}
	bmp := s.bitmapOf(p.dc, tint)
	if bmp == nil {
		return
	}
	d := p.w.fit(v, s, r)
	if d.empty() {
		return
	}
	interpolation := imageInterpolation(v.image.Sampling)
	blur := max(float32(v.image.BlurRadius), 0)
	if blur <= 0 {
		p.dc.callF(rtDrawBitmap, 0b100, bmp.ptr(), uintptr(unsafe.Pointer(&d)), f32(1), interpolation, 0)
		return
	}
	effect := s.blurOf(p.dc)
	if effect == nil {
		return
	}
	// The effect draws in the picture's own pixels, one DIP each, which
	// the world transform maps onto d; a blur radius is twice the
	// standard deviation, in points, as NSH reads it.
	sx := float64(d.right-d.left) / float64(s.pic.width)
	sy := float64(d.bottom-d.top) / float64(s.pic.height)
	sigma := blur / 2 / float32(max(min(sx, sy), 1e-6))
	const standardDeviation, borderMode, borderHard = 0, 2, 1
	const d2dPropertyEnum = 11
	effect.call(effectSetValue, standardDeviation, d2dPropertyFloat, uintptr(unsafe.Pointer(&sigma)), 4)
	hard := uint32(borderHard)
	effect.call(effectSetValue, borderMode, d2dPropertyEnum, uintptr(unsafe.Pointer(&hard)), 4)
	effect.call(effectSetInput, 0, bmp.ptr(), 1)
	var out *comObject
	effect.call(effectGetOutput, uintptr(unsafe.Pointer(&out)))
	if out == nil {
		return
	}
	defer out.release()
	p.pushClipRect(d)
	saved := p.m
	p.setTransform(scaling(sx, sy).then(translation(float64(d.left), float64(d.top))).then(saved))
	p.dc.call(dcDrawImage, out.ptr(), 0, 0, interpolation, 0)
	p.setTransform(saved)
	p.pop()
	// The effect keeps no bitmap alive between draws.
	effect.call(effectSetInput, 0, 0, 0)
}

// blurOf returns s's blur effect, made once per device.
func (s *imageSlot) blurOf(dc *comObject) *comObject {
	if s.blur != nil && s.blurGeneration == gfxGeneration {
		return s.blur
	}
	releaseAll(&s.blur)
	if failed(dc.call(dcCreateEffect, uintptr(unsafe.Pointer(&clsidGaussianBlur)), uintptr(unsafe.Pointer(&s.blur)))) {
		s.blur = nil
		return nil
	}
	s.blurGeneration = gfxGeneration
	return s.blur
}

// fit returns where s's picture goes in r, in points: core's fit of its
// natural size by v's resize mode and position, its edges moved to whole
// pixels.
func (w *window) fit(v *view, s *imageSlot, r rectF) rectF {
	box := driver.Size{Width: driver.Points(r.right - r.left), Height: driver.Points(r.bottom - r.top)}
	b := v.image.ResizeMode.Fit(natural(s), box, v.image.Position)
	scale := max(w.scale, 1e-6)
	snap := func(at float32, x driver.Points) float32 {
		return float32(math.Round((float64(at)+float64(x))*scale) / scale)
	}
	return rectF{snap(r.left, b.Origin.X), snap(r.top, b.Origin.Y),
		snap(r.left, b.Origin.X+b.Size.Width), snap(r.top, b.Origin.Y+b.Size.Height)}
}

// natural returns the size in points s's picture shows at: the size its
// source declares, an SVG's intrinsic size, or its pixels over the
// density its URI's "@2x" declares. Bitmaps are 96 DPI, so a pixel is a
// point.
func natural(s *imageSlot) driver.Size {
	if size := s.src.Size; size.Width > 0 && size.Height > 0 {
		return size
	}
	if s.pic.svg {
		return s.pic.size
	}
	d := density(s.src.URI)
	return driver.Size{Width: driver.Points(float64(s.pic.width) / d), Height: driver.Points(float64(s.pic.height) / d)}
}

// imageInfo answers an ItemImageInfo off the window thread.
func (w *window) imageInfo(it *driver.MountItem) {
	item := *it
	go func() { w.engine.Dispatch(imageload.Info(&item, imagedecode.Size)) }()
}

// density returns the N of a "name@Nx.png" URI, else 1.
func density(uri string) float64 {
	name := uri[strings.LastIndexAny(uri, `/\:`)+1:]
	at := strings.LastIndexByte(name, '@')
	if at < 0 {
		return 1
	}
	n, _, ok := strings.Cut(name[at+1:], "x")
	f, err := strconv.ParseFloat(n, 64)
	if !ok || err != nil || f <= 0 {
		return 1
	}
	return f
}
