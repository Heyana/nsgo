//go:build windows

package windows

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/draw"
	_ "image/gif" // registers the decoders image.Decode uses
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/imageload"
)

// progressInterval spaces an image's progress reports.
const progressInterval = 100 * time.Millisecond

// picture is a decoded image: premultiplied BGRA rows, top to bottom.
type picture struct {
	width, height int
	pix           []byte
}

// imageSlot is an image view's source or placeholder and what the window
// keeps for it.
type imageSlot struct {
	src        core.ImageSource
	load       *imageLoad // the load under way, if any
	pic        *picture
	bitmap     *comObject // pic as a Direct2D bitmap, tinted with tint, made in generation
	tint       core.Color
	generation uint64
}

type imageLoad struct{ cancel context.CancelFunc }

func (s *imageSlot) empty() bool { return s.src.URI == "" && len(s.src.Data) == 0 }

// reset stops the slot's load and frees its picture, for src.
func (s *imageSlot) reset(src core.ImageSource) {
	if s.load != nil {
		s.load.cancel()
		s.load = nil
	}
	releaseAll(&s.bitmap)
	s.pic = nil
	s.src = src
}

func sameSource(a, b core.ImageSource) bool {
	return a.URI == b.URI && a.Size == b.Size && len(a.Data) == len(b.Data) &&
		(len(a.Data) == 0 || &a.Data[0] == &b.Data[0])
}

// setImage applies an image view's configuration and reports whether a
// source changed, to load once the batch is applied and the view's
// events are known.
func (w *window) setImage(v *view, c *core.ImageConfig) bool {
	v.image = *c
	changed := false
	if !sameSource(v.source.src, c.Source) {
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
		w.report(v, core.Event{Kind: core.EventLoadStart, ID: v.id})
	}
	var progress imageload.Progress
	if report && v.events.Has(core.EventProgress) {
		var last time.Time // the fetching goroutine's
		progress = func(loaded, total int64) {
			if now := time.Now(); now.Sub(last) >= progressInterval || loaded == total {
				last = now
				w.post(func() {
					if s.load == ld {
						w.report(v, core.Event{Kind: core.EventProgress, ID: v.id, Loaded: loaded, Total: total})
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
			pic, err = decode(data)
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
					w.report(v, core.Event{Kind: core.EventError, ID: v.id, Text: err.Error()})
					w.report(v, core.Event{Kind: core.EventLoadEnd, ID: v.id})
				}
				return
			}
			s.pic = pic
			if report {
				size := core.Size{Width: core.Points(pic.width), Height: core.Points(pic.height)}
				w.report(v, core.Event{Kind: core.EventLoad, ID: v.id, Size: size})
				w.report(v, core.Event{Kind: core.EventLoadEnd, ID: v.id})
			}
			invalidateRect.Call(w.hwnd, 0, 0)
		})
	}()
}

// report dispatches ev when v still exists and its events hold ev's kind.
func (w *window) report(v *view, ev core.Event) {
	if !v.deleted && v.events.Has(ev.Kind) {
		w.engine.Dispatch(ev)
	}
}

// decode decodes a PNG, JPEG or GIF into premultiplied BGRA.
func decode(data []byte) (*picture, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	if b.Empty() {
		return nil, errors.New("windows: empty image")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Rect, img, b.Min, draw.Src)
	// image.RGBA is premultiplied already; Direct2D wants blue first.
	pix := rgba.Pix
	for i := 0; i+3 < len(pix); i += 4 {
		pix[i], pix[i+2] = pix[i+2], pix[i]
	}
	return &picture{width: b.Dx(), height: b.Dy(), pix: pix}, nil
}

// bitmapOf returns s's picture as a Direct2D bitmap with tint applied,
// made once per picture, tint and device; nil until the picture arrives.
func (s *imageSlot) bitmapOf(dc *comObject, tint core.Color) *comObject {
	if s.pic == nil {
		return nil
	}
	if s.bitmap != nil {
		if s.tint == tint && s.generation == gfxGeneration {
			return s.bitmap
		}
		releaseAll(&s.bitmap)
	}
	pix := s.pic.pix
	if tint&0xff != 0 {
		pix = make([]byte, len(s.pic.pix))
		tintInto(pix, s.pic.pix, tint)
	}
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96, dpiY: 96}
	if failed(dc.callSize(dcCreateBitmap, uint32(s.pic.width), uint32(s.pic.height), uintptr(unsafe.Pointer(&pix[0])),
		uintptr(s.pic.width*4), uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&s.bitmap)))) {
		s.bitmap = nil
		return nil
	}
	s.tint, s.generation = tint, gfxGeneration
	return s.bitmap
}

// tintInto copies premultiplied pixels from src to dst with their colors
// replaced by tint's, keeping their alpha.
func tintInto(dst, src []byte, tint core.Color) {
	r, g, b := uint32(tint>>24&0xff), uint32(tint>>16&0xff), uint32(tint>>8&0xff)
	for i := 0; i+3 < len(src); i += 4 {
		a := uint32(src[i+3])
		dst[i], dst[i+1], dst[i+2], dst[i+3] = byte(b*a/255), byte(g*a/255), byte(r*a/255), byte(a)
	}
}

// drawImage paints v's source, or its placeholder until the source
// arrives, into r (points) as v's resize mode fits it.
func (p *painter) drawImage(v *view, tint core.Color, r rectF) {
	s := &v.source
	if s.pic == nil {
		s = &v.placeholder
	}
	if s.pic == nil || r.empty() {
		return
	}
	bmp := s.bitmapOf(p.dc, tint)
	if bmp == nil {
		return
	}
	// Fit in pixels, so the picture's edges fall on whole ones.
	scale := float64(p.scale)
	box := rect{int32(math.Round(float64(r.left) * scale)), int32(math.Round(float64(r.top) * scale)),
		int32(math.Round(float64(r.right) * scale)), int32(math.Round(float64(r.bottom) * scale))}
	src, dst := p.w.fit(s, v.image.ResizeMode, box)
	if src.empty() || dst.empty() {
		return
	}
	k := float32(1 / scale)
	d := rectF{float32(dst.Left) * k, float32(dst.Top) * k, float32(dst.Right) * k, float32(dst.Bottom) * k}
	sr := rectF{float32(src.Left), float32(src.Top), float32(src.Right), float32(src.Bottom)}
	p.dc.callF(rtDrawBitmap, 0b100, bmp.ptr(), uintptr(unsafe.Pointer(&d)), f32(1), d2dInterpolationLinear,
		uintptr(unsafe.Pointer(&sr)))
}

// fit returns the part of s's picture to draw and where, in r, for mode.
func (w *window) fit(s *imageSlot, mode core.ResizeMode, r rect) (src, dst rect) {
	pw, ph := float64(s.pic.width), float64(s.pic.height)
	rw, rh := float64(r.Right-r.Left), float64(r.Bottom-r.Top)
	src = rect{0, 0, int32(s.pic.width), int32(s.pic.height)}
	switch mode {
	case core.Stretch:
		return src, r
	case core.Cover:
		// Crop the middle of the picture to r's aspect.
		scale := max(rw/pw, rh/ph)
		cw, ch := rw/scale, rh/scale
		src.Left, src.Top = int32(math.Round((pw-cw)/2)), int32(math.Round((ph-ch)/2))
		src.Right, src.Bottom = int32(math.Round((pw+cw)/2)), int32(math.Round((ph+ch)/2))
		src.Left, src.Top = min(src.Left, src.Right-1), min(src.Top, src.Bottom-1)
		return src, r
	case core.Center:
		// A picture's pixels are points, or its URI's "@2x" says how many
		// make one, unless the source gave its size.
		nw, nh := pw/density(s.src.URI)*w.scale, ph/density(s.src.URI)*w.scale
		if size := s.src.Size; size.Width > 0 && size.Height > 0 {
			nw, nh = float64(size.Width)*w.scale, float64(size.Height)*w.scale
		}
		if nw <= rw && nh <= rh {
			return src, centered(r, nw, nh)
		}
	}
	scale := min(rw/pw, rh/ph) // Contain, and Center when larger than r
	return src, centered(r, pw*scale, ph*scale)
}

// centered returns a rectangle of width by height centered in r.
func centered(r rect, width, height float64) rect {
	x := float64(r.Left) + (float64(r.Right-r.Left)-width)/2
	y := float64(r.Top) + (float64(r.Bottom-r.Top)-height)/2
	return rect{int32(math.Round(x)), int32(math.Round(y)), int32(math.Round(x + width)), int32(math.Round(y + height))}
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
