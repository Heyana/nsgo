//go:build linux && !android

package linux

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
	src     core.ImageSource
	load    *imageLoad
	pic     *picture
	surface uintptr // pic as a Cairo surface, tinted with tint
	tint    core.Color
}

type imageLoad struct{ cancel context.CancelFunc }

func (s *imageSlot) empty() bool { return s.src.URI == "" && len(s.src.Data) == 0 }

// reset stops the slot's load and frees its picture, for src.
func (s *imageSlot) reset(src core.ImageSource) {
	if s.load != nil {
		s.load.cancel()
		s.load = nil
	}
	if s.surface != 0 {
		cairoSurfaceDestroy(s.surface)
		s.surface = 0
	}
	s.pic = nil
	s.src = src
}

func sameSource(a, b core.ImageSource) bool {
	return a.URI == b.URI && a.Size == b.Size && len(a.Data) == len(b.Data) &&
		(len(a.Data) == 0 || &a.Data[0] == &b.Data[0])
}

// setImage applies an image view's configuration and reports whether a
// source changed, to load once the batch is applied.
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
		var last time.Time
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
			return
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
			w.changed(v)
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
		return nil, errors.New("linux: empty image")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Rect, img, b.Min, draw.Src)
	// image.RGBA is premultiplied already; Cairo's ARGB32 wants blue
	// first on little-endian machines.
	pix := rgba.Pix
	for i := 0; i+3 < len(pix); i += 4 {
		pix[i], pix[i+2] = pix[i+2], pix[i]
	}
	return &picture{width: b.Dx(), height: b.Dy(), pix: pix}, nil
}

// surfaceOf returns s's picture as a Cairo image surface with tint
// applied, made once per picture and tint; 0 until the picture arrives.
func (s *imageSlot) surfaceOf(tint core.Color) uintptr {
	if s.pic == nil {
		return 0
	}
	if s.surface != 0 {
		if s.tint == tint {
			return s.surface
		}
		cairoSurfaceDestroy(s.surface)
		s.surface = 0
	}
	surf := cairoImageSurfaceCreate(0, int32(s.pic.width), int32(s.pic.height))
	if surf == 0 {
		return 0
	}
	data := cairoImageSurfaceGetData(surf)
	stride := int(cairoImageSurfaceGetStride(surf))
	rows := unsafe.Slice((*byte)(cPointer(data)), stride*s.pic.height)
	row := make([]byte, s.pic.width*4)
	for y := 0; y < s.pic.height; y++ {
		src := s.pic.pix[y*s.pic.width*4 : (y+1)*s.pic.width*4]
		if tint&0xff == 0 {
			copy(rows[y*stride:y*stride+len(src)], src)
			continue
		}
		tintInto(row, src, tint)
		copy(rows[y*stride:y*stride+len(row)], row)
	}
	cairoSurfaceMarkDirty(surf)
	s.surface, s.tint = surf, tint
	return surf
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
// arrives, into r as v's resize mode fits it.
func (p *painter) drawImage(v *view, tint core.Color, r rect) {
	s := &v.source
	if s.pic == nil {
		s = &v.placeholder
	}
	surf := s.surfaceOf(tint)
	if surf == 0 || r.w <= 0 || r.h <= 0 {
		return
	}
	src, dst := p.w.fit(s, v.image.ResizeMode, r)
	if dst.w <= 0 || dst.h <= 0 || src.w <= 0 || src.h <= 0 {
		return
	}
	cr := p.cr
	cairoSave(cr)
	cairoTranslate(cr, dst.x, dst.y)
	cairoScale(cr, dst.w/src.w, dst.h/src.h)
	cairoSetSourceSurface(cr, surf, -src.x, -src.y)
	cairoPaint(cr)
	cairoRestore(cr)
}

// fit returns the part of s's picture to draw and where, in r, for mode.
func (w *window) fit(s *imageSlot, mode core.ResizeMode, r rect) (src, dst rect) {
	pw, ph := float64(s.pic.width), float64(s.pic.height)
	src = rect{w: pw, h: ph}
	switch mode {
	case core.Stretch:
		return src, r
	case core.Cover:
		// Crop the middle of the picture to r's aspect.
		scale := math.Max(r.w/pw, r.h/ph)
		cw, ch := r.w/scale, r.h/scale
		src.x, src.y = (pw-cw)/2, (ph-ch)/2
		src.w, src.h = cw, ch
		return src, r
	case core.Center:
		// A picture's pixels are points, or its URI's "@2x" says how many
		// make one, unless the source gave its size.
		nw, nh := pw/density(s.src.URI), ph/density(s.src.URI)
		if size := s.src.Size; size.Width > 0 && size.Height > 0 {
			nw, nh = float64(size.Width), float64(size.Height)
		}
		if nw <= r.w && nh <= r.h {
			return src, centered(r, nw, nh)
		}
	}
	scale := math.Min(r.w/pw, r.h/ph) // Contain, and Center when larger than r
	return src, centered(r, pw*scale, ph*scale)
}

// centered returns a rectangle of width by height centered in r.
func centered(r rect, width, height float64) rect {
	return rect{
		x: r.x + (r.w-width)/2,
		y: r.y + (r.h-height)/2,
		w: width, h: height,
	}
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
