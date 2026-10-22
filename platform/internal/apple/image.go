//go:build darwin

package apple

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/imageload"
)

var (
	cfDataCreate                  func(alloc uintptr, bytes *byte, n int) uintptr
	cfDictionaryGetValue          func(dict, key uintptr) uintptr
	cfNumberGetValue              func(num uintptr, kind int, value unsafe.Pointer) bool
	cgImageGetWidth               func(uintptr) int
	cgImageGetHeight              func(uintptr) int
	cgImageSourceCreateWithData   func(data, options uintptr) uintptr
	cgImageSourceCopyProperties   func(src uintptr, i int, options uintptr) uintptr
	cgImageSourceCreateThumbnail  func(src uintptr, i int, options uintptr) uintptr
	cfTrue                        uintptr
	keyWidth, keyHeight, keyTurn  uintptr
	keyAlways, keyUpright, keyMax uintptr
	keyDecodeNow                  uintptr

	ciOnce    sync.Once
	ciContext objc.ID

	selNew, selRelease    = sel("new"), sel("release")
	selSetFrame           = sel("setFrame:")
	selSetContents        = sel("setContents:")
	selSetContentsGravity = sel("setContentsGravity:")
	selSetContentsScale   = sel("setContentsScale:")
	selSetMask            = sel("setMask:")
	selAddAnimation       = sel("addAnimation:forKey:")
	selNumberWithDouble   = sel("numberWithDouble:")
)

func loadImage(cf, cg uintptr) {
	io := load("/System/Library/Frameworks/ImageIO.framework/ImageIO")
	reg := purego.RegisterLibFunc
	reg(&cfDataCreate, cf, "CFDataCreate")
	reg(&cfDictionaryGetValue, cf, "CFDictionaryGetValue")
	reg(&cfNumberGetValue, cf, "CFNumberGetValue")
	reg(&cgImageGetWidth, cg, "CGImageGetWidth")
	reg(&cgImageGetHeight, cg, "CGImageGetHeight")
	reg(&cgImageSourceCreateWithData, io, "CGImageSourceCreateWithData")
	reg(&cgImageSourceCopyProperties, io, "CGImageSourceCopyPropertiesAtIndex")
	reg(&cgImageSourceCreateThumbnail, io, "CGImageSourceCreateThumbnailAtIndex")
	cfTrue = constant(cf, "kCFBooleanTrue")
	keyWidth = constant(io, "kCGImagePropertyPixelWidth")
	keyHeight = constant(io, "kCGImagePropertyPixelHeight")
	keyTurn = constant(io, "kCGImagePropertyOrientation")
	keyAlways = constant(io, "kCGImageSourceCreateThumbnailFromImageAlways")
	keyUpright = constant(io, "kCGImageSourceCreateThumbnailWithTransform")
	keyMax = constant(io, "kCGImageSourceThumbnailMaxPixelSize")
	keyDecodeNow = constant(io, "kCGImageSourceShouldCacheImmediately")
}

// request is how an image view shows what it loads.
type request struct {
	mode  core.ResizeMode
	view  CGSize  // the view's size in points; zero before layout
	ratio float64 // the screen's pixels per point
	scale float64 // the image's pixels per point at its natural size
	blur  float64 // points
}

// pointsPerPixel returns how many points one pixel of a w×h image covers
// when r's view shows it; 0 when the view has no size yet.
func (r request) pointsPerPixel(w, h float64) float64 {
	if r.view.Width <= 0 || r.view.Height <= 0 || w <= 0 || h <= 0 {
		return 0
	}
	sx, sy := r.view.Width/w, r.view.Height/h
	switch r.mode {
	case core.Contain:
		return min(sx, sy)
	case core.Center:
		if w/r.scale > r.view.Width || h/r.scale > r.view.Height {
			return min(sx, sy)
		}
		return 1 / r.scale
	}
	return max(sx, sy)
}

// maxPixels returns the longer side to decode a w×h image at so it stays
// sharp in the view, or 0 to decode all of it.
func (r request) maxPixels(w, h float64) float64 {
	ppp := r.pointsPerPixel(w, h)
	if ppp == 0 {
		return 0
	}
	if m := math.Ceil(max(w, h) * ppp * r.ratio); m < max(w, h) {
		return m
	}
	return 0
}

// decoded is a CGImage ImageIO decoded, released by its holder, and the
// source's upright size in pixels.
type decoded struct {
	image         uintptr
	width, height float64
	scale         float64
}

// decode decodes data upright and at once, at the size r shows it.
func decode(data []byte, r request) (decoded, error) {
	if len(data) == 0 {
		return decoded{}, errors.New("apple: empty image data")
	}
	cfData := cfDataCreate(0, &data[0], len(data))
	defer Release(cfData)
	src := cgImageSourceCreateWithData(cfData, 0)
	if src == 0 {
		return decoded{}, errors.New("apple: unsupported image data")
	}
	defer Release(src)
	props := cgImageSourceCopyProperties(src, 0, 0)
	if props == 0 {
		return decoded{}, errors.New("apple: unsupported image data")
	}
	w, h := number(props, keyWidth), number(props, keyHeight)
	// EXIF orientations 5 to 8 turn the image a quarter.
	if number(props, keyTurn) >= 5 {
		w, h = h, w
	}
	Release(props)
	keys := []uintptr{keyAlways, keyUpright, keyDecodeNow}
	values := []uintptr{cfTrue, cfTrue, cfTrue}
	if m := r.maxPixels(w, h); m > 0 {
		n := float64Number(m)
		defer Release(n)
		keys, values = append(keys, keyMax), append(values, n)
	}
	opts := dictionary(keys, values)
	defer Release(opts)
	img := cgImageSourceCreateThumbnail(src, 0, opts)
	if img == 0 {
		return decoded{}, errors.New("apple: cannot decode the image")
	}
	if r.blur > 0 {
		dw, dh := float64(cgImageGetWidth(img)), float64(cgImageGetHeight(img))
		radius := r.blur * r.ratio
		if ppp := (request{mode: r.mode, view: r.view, scale: r.scale * dw / w}).pointsPerPixel(dw, dh); ppp > 0 {
			radius = r.blur / ppp
		}
		if out := blur(img, radius); out != 0 {
			Release(img)
			img = out
		}
	}
	return decoded{image: img, width: w, height: h, scale: r.scale}, nil
}

func number(dict, key uintptr) float64 {
	var v float64
	if n := cfDictionaryGetValue(dict, key); n != 0 {
		const float64Type = 6
		cfNumberGetValue(n, float64Type, unsafe.Pointer(&v))
	}
	return v
}

// blur returns a new CGImage of img blurred by radius pixels with
// CoreImage, or 0.
func blur(img uintptr, radius float64) (out uintptr) {
	ciOnce.Do(func() {
		load("/System/Library/Frameworks/CoreImage.framework/CoreImage")
		withPool(func() {
			ciContext = objc.ID(objc.GetClass("CIContext")).Send(sel("contextWithOptions:"), objc.ID(0)).Send(sel("retain"))
		})
	})
	withPool(func() {
		in := objc.ID(objc.GetClass("CIImage")).Send(sel("imageWithCGImage:"), img)
		extent := objc.Send[CGRect](in, sel("extent"))
		f := objc.ID(objc.GetClass("CIFilter")).Send(sel("filterWithName:"), NSString("CIGaussianBlur"))
		f.Send(sel("setValue:forKey:"), in.Send(sel("imageByClampingToExtent")), NSString("inputImage"))
		f.Send(sel("setValue:forKey:"), objc.ID(objc.GetClass("NSNumber")).Send(selNumberWithDouble, radius), NSString("inputRadius"))
		out = objc.Send[uintptr](ciContext, sel("createCGImage:fromRect:"), f.Send(sel("outputImage")), extent)
	})
	return out
}

// uriScale returns the pixels per point an "@2x"-style name declares.
func uriScale(uri string) float64 {
	base := uri
	if dot := strings.LastIndexByte(uri, '.'); dot > strings.LastIndexByte(uri, '/') {
		base = uri[:dot]
	}
	at := strings.LastIndexByte(base, '@')
	if at < 0 || !strings.HasSuffix(base, "x") {
		return 1
	}
	if f, err := strconv.ParseFloat(base[at+1:len(base)-1], 64); err == nil && f > 0 {
		return f
	}
	return 1
}

func hasSource(s core.ImageSource) bool { return s.URI != "" || len(s.Data) > 0 }

func sameSource(a, b core.ImageSource) bool {
	return a.URI == b.URI && len(a.Data) == len(b.Data) && (len(a.Data) == 0 || &a.Data[0] == &b.Data[0])
}

// ImageView shows an image view's source, and its placeholder until the
// source arrives, in a layer it adds to the view's own. It loads them
// with imageload and decodes them with ImageIO on goroutines, at the size
// the view shows them. Its methods run on the main thread.
type ImageView struct {
	// Dispatch reports an event to the engine if the view listens for it.
	Dispatch func(core.Event)

	layer, mask objc.ID
	config      core.ImageConfig
	tint        core.Color
	bounds      CGSize
	ratio       float64

	image   uintptr // the CGImage shown, retained
	shown   decoded // what image was decoded from
	source  bool    // whether image is the source rather than the placeholder
	reduced bool    // whether image is smaller than the source

	load    int // numbers loads; an older load's results are dropped
	cancel  context.CancelFunc
	pending bool // a load waits for Start
	quiet   bool // the pending load decodes the source shown again, larger
	closed  bool
}

// NewImageView adds an image layer to host, a view's layer.
func NewImageView(host objc.ID) *ImageView {
	iv := &ImageView{layer: objc.ID(objc.GetClass("CALayer")).Send(selNew)}
	iv.layer.Send(selSetMasksToBounds, true)
	host.Send(sel("addSublayer:"), iv.layer)
	iv.layer.Send(selRelease)
	return iv
}

// Configure sets the view's source and look. Start loads what changed.
func (iv *ImageView) Configure(c *core.ImageConfig) {
	old := iv.config
	iv.config = *c
	if iv.load == 0 || !sameSource(old.Source, c.Source) || !sameSource(old.Placeholder, c.Placeholder) ||
		old.BlurRadius != c.BlurRadius {
		iv.pending, iv.quiet = true, false
	}
	iv.render(0)
}

// SetStyle applies what of a view's style the image layer draws: the
// tint and the corners.
func (iv *ImageView) SetStyle(vs *core.ViewStyle) {
	WithoutAnimation(func() { iv.layer.Send(selSetCornerRadius, float64(vs.CornerRadius)) })
	if vs.TintColor != iv.tint {
		iv.tint = vs.TintColor
		iv.render(0)
	}
}

// Layout fits the image to the view's new size, decoding the source
// again when the view outgrows the size it was decoded at.
func (iv *ImageView) Layout(size CGSize) {
	iv.bounds = size
	iv.render(0)
	if !iv.source || !iv.reduced || iv.pending {
		return
	}
	r, w, h := iv.request(iv.ratio), iv.shown.width, iv.shown.height
	if r.pointsPerPixel(w, h) == 0 {
		return
	}
	// maxPixels is 0 when the view needs all of the source.
	have := max(float64(cgImageGetWidth(iv.image)), float64(cgImageGetHeight(iv.image)))
	if m := r.maxPixels(w, h); m == 0 || m > have*1.25 {
		iv.pending, iv.quiet = true, true
	}
}

func (iv *ImageView) request(ratio float64) request {
	return request{mode: iv.config.ResizeMode, view: iv.bounds, ratio: ratio,
		scale: uriScale(iv.config.Source.URI), blur: float64(iv.config.BlurRadius)}
}

// Start loads what Configure or Layout changed, for a screen of ratio
// pixels per point. Backends call it once a batch has sized the view.
func (iv *ImageView) Start(ratio float64) {
	if !iv.pending || iv.closed {
		return
	}
	if ratio <= 0 {
		ratio = 1
	}
	iv.ratio = ratio
	iv.pending = false
	if iv.cancel != nil {
		iv.cancel()
	}
	iv.load++
	n := iv.load
	ctx, cancel := context.WithCancel(context.Background())
	iv.cancel = cancel
	c := iv.config
	r := iv.request(ratio)
	quiet := iv.quiet
	iv.quiet = false
	if !quiet {
		iv.show(decoded{}, false, 0)
		if hasSource(c.Placeholder) {
			pr := r
			pr.scale = uriScale(c.Placeholder.URI)
			go iv.fetch(ctx, n, c.Placeholder, pr, false, true)
		}
	}
	if !hasSource(c.Source) {
		return
	}
	if !quiet {
		iv.Dispatch(core.Event{Kind: core.EventLoadStart})
	}
	go iv.fetch(ctx, n, c.Source, r, true, quiet)
}

// fetch loads and decodes src on a goroutine of its own, then shows it on
// the main thread unless a later load replaced this one, numbered n.
func (iv *ImageView) fetch(ctx context.Context, n int, src core.ImageSource, r request, source, quiet bool) {
	var progress imageload.Progress
	if !quiet {
		var last time.Time
		progress = func(loaded, total int64) {
			if now := time.Now(); now.Sub(last) >= 100*time.Millisecond || loaded == total {
				last = now
				OnMain(func() {
					if iv.load == n {
						iv.Dispatch(core.Event{Kind: core.EventProgress, Loaded: loaded, Total: total})
					}
				})
			}
		}
	}
	data, err := imageload.Load(ctx, src, progress)
	if ctx.Err() != nil {
		return
	}
	var img decoded
	if err == nil {
		img, err = decode(data, r)
	}
	OnMain(func() {
		if iv.load != n || (!source && iv.source) {
			Release(img.image)
			return
		}
		switch {
		case !source:
			if err == nil {
				iv.show(img, false, 0)
			}
		case err != nil:
			iv.Dispatch(core.Event{Kind: core.EventError, Text: err.Error()})
			iv.Dispatch(core.Event{Kind: core.EventLoadEnd})
		case quiet:
			iv.show(img, true, 0)
		default:
			iv.show(img, true, iv.config.FadeDuration)
			iv.Dispatch(core.Event{Kind: core.EventLoad, Size: core.Size{Width: core.Points(img.width), Height: core.Points(img.height)}})
			iv.Dispatch(core.Event{Kind: core.EventLoadEnd})
		}
	})
}

func (iv *ImageView) show(img decoded, source bool, fade time.Duration) {
	Release(iv.image)
	iv.image, iv.shown, iv.source = img.image, img, source && img.image != 0
	iv.reduced = img.image != 0 && float64(cgImageGetWidth(img.image)) < img.width
	iv.render(fade)
}

// gravity returns the CALayer contents gravity of the resize mode.
func (iv *ImageView) gravity() string {
	switch iv.config.ResizeMode {
	case core.Contain:
		return "resizeAspect"
	case core.Stretch:
		return "resize"
	case core.Center:
		s := iv.shown
		if s.scale > 0 && (s.width/s.scale > iv.bounds.Width || s.height/s.scale > iv.bounds.Height) {
			return "resizeAspect"
		}
		return "center"
	}
	return "resizeAspectFill"
}

// render shows the image: as it is, or as a template the tint fills
// through a mask of the image's alpha.
func (iv *ImageView) render(fade time.Duration) {
	WithoutAnimation(func() {
		frame := Rect(0, 0, iv.bounds.Width, iv.bounds.Height)
		iv.layer.Send(selSetFrame, frame)
		target := iv.layer
		if iv.tint == 0 {
			iv.layer.Send(selSetBackgroundColor, uintptr(0))
			iv.layer.Send(selSetMask, objc.ID(0))
		} else {
			if iv.mask == 0 {
				iv.mask = objc.ID(objc.GetClass("CALayer")).Send(selNew)
			}
			iv.mask.Send(selSetFrame, frame)
			color := CGColor(iv.tint)
			iv.layer.Send(selSetBackgroundColor, color)
			Release(color)
			iv.layer.Send(selSetContents, uintptr(0))
			iv.layer.Send(selSetMask, iv.mask)
			target = iv.mask
		}
		if fade > 0 {
			t := objc.ID(objc.GetClass("CATransition")).Send(sel("animation"))
			t.Send(sel("setType:"), NSString("fade"))
			t.Send(sel("setDuration:"), fade.Seconds())
			target.Send(selAddAnimation, t, NSString("contents"))
		}
		target.Send(selSetContents, iv.image)
		target.Send(selSetContentsGravity, NSString(iv.gravity()))
		scale := iv.shown.scale
		if scale <= 0 {
			scale = 1
		}
		target.Send(selSetContentsScale, scale)
	})
}

// Close stops loading and lets the image go; the view is being deleted.
func (iv *ImageView) Close() {
	if iv.cancel != nil {
		iv.cancel()
	}
	iv.load++
	iv.closed = true
	Release(iv.image)
	iv.image = 0
	if iv.mask != 0 {
		iv.mask.Send(selRelease)
		iv.mask = 0
	}
}

// AnimateIn presents a modal's layer as a asks: sliding up from distance
// points below, or fading in. Reduced motion fades instead of sliding.
func AnimateIn(layer objc.ID, a core.ModalAnimation, distance float64, reduceMotion bool) {
	if a == core.ModalSlide && reduceMotion {
		a = core.ModalFade
	}
	number := objc.ID(objc.GetClass("NSNumber"))
	basic := objc.ID(objc.GetClass("CABasicAnimation"))
	var anim objc.ID
	switch a {
	case core.ModalSlide:
		anim = basic.Send(sel("animationWithKeyPath:"), NSString("transform.translation.y"))
		anim.Send(sel("setFromValue:"), number.Send(selNumberWithDouble, distance))
		anim.Send(sel("setDuration:"), 0.3)
		anim.Send(sel("setTimingFunction:"), objc.ID(objc.GetClass("CAMediaTimingFunction")).Send(
			sel("functionWithName:"), NSString("easeOut")))
	case core.ModalFade:
		anim = basic.Send(sel("animationWithKeyPath:"), NSString("opacity"))
		anim.Send(sel("setFromValue:"), number.Send(selNumberWithDouble, 0.0))
		anim.Send(sel("setDuration:"), 0.2)
	default:
		return
	}
	layer.Send(selAddAnimation, anim, NSString("nsgoPresent"))
}
