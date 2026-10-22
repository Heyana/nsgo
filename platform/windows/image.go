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
	"syscall"
	"time"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/imageload"
)

var (
	msimg32 = syscall.NewLazyDLL("msimg32.dll")

	alphaBlend       = msimg32.NewProc("AlphaBlend")
	createDIBSection = gdi32.NewProc("CreateDIBSection")
)

const (
	dibRGBColors = 0
	// blendPremultiplied is BLENDFUNCTION{AC_SRC_OVER, 0, 255,
	// AC_SRC_ALPHA}, passed by value.
	blendPremultiplied = 0x01ff0000
)

// progressInterval spaces an image's progress reports.
const progressInterval = 100 * time.Millisecond

type bitmapInfo struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
	Colors                       [1]uint32
}

// picture is a decoded image: premultiplied BGRA rows, top to bottom.
type picture struct {
	width, height int
	pix           []byte
}

// imageSlot is an image view's source or placeholder and what the window
// keeps for it.
type imageSlot struct {
	src    core.ImageSource
	load   *imageLoad // the load under way, if any
	pic    *picture
	bitmap uintptr // pic as a DIB section, tinted with tint
	bits   unsafe.Pointer
	tint   core.Color
}

type imageLoad struct{ cancel context.CancelFunc }

func (s *imageSlot) empty() bool { return s.src.URI == "" && len(s.src.Data) == 0 }

// reset stops the slot's load and frees its picture, for src.
func (s *imageSlot) reset(src core.ImageSource) {
	if s.load != nil {
		s.load.cancel()
		s.load = nil
	}
	if s.bitmap != 0 {
		deleteObject.Call(s.bitmap)
		s.bitmap = 0
	}
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
	// image.RGBA is premultiplied already; GDI wants blue first.
	pix := rgba.Pix
	for i := 0; i+3 < len(pix); i += 4 {
		pix[i], pix[i+2] = pix[i+2], pix[i]
	}
	return &picture{width: b.Dx(), height: b.Dy(), pix: pix}, nil
}

// bitmapOf returns s's picture as a DIB section with tint applied, made
// once per picture and tint; 0 until the picture arrives.
func (s *imageSlot) bitmapOf(tint core.Color) uintptr {
	if s.pic == nil {
		return 0
	}
	if s.bitmap != 0 {
		if s.tint == tint {
			return s.bitmap
		}
		deleteObject.Call(s.bitmap)
		s.bitmap = 0
	}
	h, bits := newDIB(int32(s.pic.width), int32(s.pic.height))
	if h == 0 {
		return 0
	}
	dst := unsafe.Slice((*byte)(bits), len(s.pic.pix))
	if tint&0xff == 0 {
		copy(dst, s.pic.pix)
	} else {
		tintInto(dst, s.pic.pix, tint)
	}
	s.bitmap, s.bits, s.tint = h, bits, tint
	return h
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
// arrives, into r as v's resize mode fits it: blended by GDI where the
// transform keeps it a rectangle, else sampled here.
func (p *painter) drawImage(v *view, tint core.Color, r rect) {
	s := &v.source
	if s.pic == nil {
		s = &v.placeholder
	}
	bmp := s.bitmapOf(tint)
	if bmp == 0 || r.empty() {
		return
	}
	src, dst := p.w.fit(s, v.image.ResizeMode, r)
	if src.empty() || dst.empty() {
		return
	}
	switch {
	case p.m == identity:
		blendBitmap(p.dc, dst, bmp, src)
	case p.m.axisAligned():
		d := p.m.bound(dst).offset(-p.ox, -p.oy)
		p.device(func() { blendBitmap(p.dc, d, bmp, src) })
	default:
		p.sample(s, src, dst)
	}
}

// blendBitmap blends the premultiplied bitmap's src into dst.
func blendBitmap(dc uintptr, dst rect, bmp uintptr, src rect) {
	mem, _, _ := createCompatibleDC.Call(dc)
	old, _, _ := selectObject.Call(mem, bmp)
	alphaBlend.Call(dc, uintptr(dst.Left), uintptr(dst.Top), uintptr(dst.Right-dst.Left), uintptr(dst.Bottom-dst.Top),
		mem, uintptr(src.Left), uintptr(src.Top), uintptr(src.Right-src.Left), uintptr(src.Bottom-src.Top),
		blendPremultiplied)
	selectObject.Call(mem, old)
	deleteDC.Call(mem)
}

// sample draws src of s's bitmap into dst, in the window's pixels, under
// a rotating transform AlphaBlend cannot take: each target pixel inside
// the clipping maps back to its nearest source pixel.
func (p *painter) sample(s *imageSlot, src, dst rect) {
	gdiFlush.Call()
	inv := p.m.invert()
	box := p.m.bound(dst).offset(-p.ox, -p.oy)
	target := unsafe.Slice((*uint32)(p.target.bits), int(p.target.width)*int(p.target.height))
	pix := unsafe.Slice((*uint32)(s.bits), s.pic.width*s.pic.height)
	sx := float64(src.Right-src.Left) / float64(dst.Right-dst.Left)
	sy := float64(src.Bottom-src.Top) / float64(dst.Bottom-dst.Top)
	for _, c := range p.clipRects() {
		c = c.intersect(box)
		for y := c.Top; y < c.Bottom; y++ {
			for x := c.Left; x < c.Right; x++ {
				lx, ly := inv.apply(float64(x+p.ox)+0.5, float64(y+p.oy)+0.5)
				if lx < float64(dst.Left) || ly < float64(dst.Top) || lx >= float64(dst.Right) || ly >= float64(dst.Bottom) {
					continue
				}
				u := min(int(src.Left)+int((lx-float64(dst.Left))*sx), int(src.Right)-1)
				w := min(int(src.Top)+int((ly-float64(dst.Top))*sy), int(src.Bottom)-1)
				sp := pix[w*s.pic.width+u]
				i := int(y)*int(p.target.width) + int(x)
				switch a := sp >> 24; a {
				case 0:
				case 255:
					target[i] = sp
				default:
					target[i] = over(sp, target[i])
				}
			}
		}
	}
}

// over composites premultiplied pixel s over d.
func over(s, d uint32) uint32 {
	k := 255 - s>>24
	var out uint32
	for shift := 0; shift < 32; shift += 8 {
		c := s>>shift&0xff + (d>>shift&0xff)*k/255
		out |= min(c, 255) << shift
	}
	return out
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
