//go:build darwin && !ios

package macos

import (
	"container/list"
	"context"
	"strings"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

const maxCustomCursors = 64

type customCursorEntry struct {
	key    string
	cursor objc.ID // retained NSCursor, or 0 on decode failure
}

// customCursorCache keeps decoded NSCursors on the main thread, bounded by
// maxCustomCursors, releasing evicted cursors so swaps never leak.
var customCursorCache = struct {
	order   *list.List
	entries map[string]*list.Element
	loading map[string]bool
}{
	order:   list.New(),
	entries: map[string]*list.Element{},
	loading: map[string]bool{},
}

func customCursorGet(key string) (objc.ID, bool) {
	if el, ok := customCursorCache.entries[key]; ok {
		customCursorCache.order.MoveToFront(el)
		return el.Value.(*customCursorEntry).cursor, true
	}
	return 0, false
}

func customCursorPut(key string, cur objc.ID) {
	if el, ok := customCursorCache.entries[key]; ok {
		ent := el.Value.(*customCursorEntry)
		if ent.cursor != 0 && ent.cursor != cur {
			ent.cursor.Send(selRelease)
		}
		ent.cursor = cur
		customCursorCache.order.MoveToFront(el)
		return
	}
	el := customCursorCache.order.PushFront(&customCursorEntry{key: key, cursor: cur})
	customCursorCache.entries[key] = el
	for customCursorCache.order.Len() > maxCustomCursors {
		back := customCursorCache.order.Back()
		ent := back.Value.(*customCursorEntry)
		customCursorCache.order.Remove(back)
		delete(customCursorCache.entries, ent.key)
		if ent.cursor != 0 {
			ent.cursor.Send(selRelease)
		}
	}
}

func clearCustomCursorCache() {
	for el := customCursorCache.order.Front(); el != nil; el = el.Next() {
		if cur := el.Value.(*customCursorEntry).cursor; cur != 0 {
			cur.Send(selRelease)
		}
	}
	customCursorCache.order.Init()
	clear(customCursorCache.entries)
	clear(customCursorCache.loading)
}

func isRemoteURI(uri string) bool {
	return strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://")
}

// customNSCursor returns a cached NSCursor for cc, creating it synchronously
// for local/embedded/pixel sources or loading asynchronously for remote URLs.
// Returns 0 while loading or if cc is empty/invalid.
func (s *surface) customNSCursor(cc *driver.CustomCursor) objc.ID {
	if cc.Empty() {
		return 0
	}
	key := cc.Key()
	if cur, ok := customCursorGet(key); ok {
		return cur
	}
	if isRemoteURI(cc.Image.URI) || isRemoteURI(cc.Image2x.URI) {
		if !customCursorCache.loading[key] {
			customCursorCache.loading[key] = true
			spec := *cc
			go func() {
				img1x := decodeCursorSource(spec.Image, 1)
				img2x := decodeCursorSource(spec.Image2x, 2)
				apple.OnMain(func() {
					delete(customCursorCache.loading, key)
					cur := buildNSCursor(&spec, img1x, img2x)
					customCursorPut(key, cur)
					if s != nil && s.window != 0 {
						s.pickHere()
					}
				})
			}()
		}
		return 0
	}
	img1x := decodeCursorSource(cc.Image, 1)
	img2x := decodeCursorSource(cc.Image2x, 2)
	cur := buildNSCursor(cc, img1x, img2x)
	customCursorPut(key, cur)
	return cur
}

func decodeCursorSource(src driver.ImageSource, scale float32) *imagedecode.Image {
	if src.Empty() {
		return nil
	}
	if src.Pixels != nil && !src.Pixels.Rect.Empty() {
		return imagedecode.FromPixels(src.Pixels, imagedecode.RGBA)
	}
	data, err := imageload.Load(context.Background(), src, nil)
	if err != nil || len(data) == 0 {
		return nil
	}
	img, err := imagedecode.Decode(data, imagedecode.Options{Layout: imagedecode.RGBA, Scale: scale})
	if err != nil || img == nil || len(img.Frames) == 0 || img.Width <= 0 || img.Height <= 0 {
		return nil
	}
	return img
}

// buildNSCursor constructs a retained NSCursor with both 1x and 2x
// NSBitmapImageRep representations on one NSImage when both are provided so
// AppKit automatically chooses 2x on Retina displays.
func buildNSCursor(cc *driver.CustomCursor, img1x, img2x *imagedecode.Image) objc.ID {
	if img1x == nil && img2x == nil {
		return 0
	}
	var logical driver.Size
	switch {
	case img1x != nil:
		switch {
		case cc.Image.Size.Width > 0 && cc.Image.Size.Height > 0:
			logical = cc.Image.Size
		case img1x.Format == imagedecode.SVG && img1x.Size.Width > 0 && img1x.Size.Height > 0:
			logical = img1x.Size
		default:
			d := cc.Image.Density()
			if d <= 0 {
				d = 1
			}
			logical = driver.Size{
				Width:  max(driver.Points(1), driver.Points(img1x.Width)/driver.Points(d)),
				Height: max(driver.Points(1), driver.Points(img1x.Height)/driver.Points(d)),
			}
		}
	default:
		switch {
		case cc.Image2x.Size.Width > 0 && cc.Image2x.Size.Height > 0:
			logical = cc.Image2x.Size
		case img2x.Format == imagedecode.SVG && img2x.Size.Width > 0 && img2x.Size.Height > 0:
			logical = img2x.Size
		default:
			logical = driver.Size{
				Width:  max(driver.Points(1), driver.Points(img2x.Width)/2),
				Height: max(driver.Points(1), driver.Points(img2x.Height)/2),
			}
		}
	}
	if logical.Width <= 0 || logical.Height <= 0 {
		return 0
	}
	nsSize := apple.CGSize{Width: float64(logical.Width), Height: float64(logical.Height)}
	nsImg := objc.ID(apple.Class("NSImage")).Send(selAlloc).Send(sel("initWithSize:"), nsSize)
	if nsImg == 0 {
		return 0
	}
	defer nsImg.Send(selRelease)

	added := false
	addRep := func(img *imagedecode.Image) {
		if img == nil || len(img.Frames) == 0 || img.Width <= 0 || img.Height <= 0 {
			return
		}
		cg := apple.RGBAImage(img.Frames[0].Pix, img.Width, img.Height)
		if cg == 0 {
			return
		}
		defer apple.Release(cg)
		rep := objc.ID(apple.Class("NSBitmapImageRep")).Send(selAlloc).Send(sel("initWithCGImage:"), objc.ID(cg))
		if rep == 0 {
			return
		}
		defer rep.Send(selRelease)
		rep.Send(sel("setSize:"), nsSize)
		nsImg.Send(sel("addRepresentation:"), rep)
		added = true
	}
	addRep(img1x)
	addRep(img2x)
	if !added {
		return 0
	}

	hot := driver.ClampHotspot(cc.Hotspot, logical)
	return objc.ID(apple.Class("NSCursor")).Send(selAlloc).Send(
		sel("initWithImage:hotSpot:"),
		nsImg,
		apple.CGPoint{X: float64(hot.X), Y: float64(hot.Y)},
	)
}
