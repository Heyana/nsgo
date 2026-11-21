//go:build linux && !android

package linux

import (
	"container/list"
	"context"
	"fmt"
	"strings"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

const maxCustomCursors = 64

type customCursorEntry struct {
	key    string
	cursor uintptr // owned GdkCursor*, or 0 on failure
}

type customCursorCache struct {
	order   *list.List
	entries map[string]*list.Element
	loading map[string]bool
}

func (w *window) cursorCache() *customCursorCache {
	if w.customCursors == nil {
		w.customCursors = &customCursorCache{
			order:   list.New(),
			entries: map[string]*list.Element{},
			loading: map[string]bool{},
		}
	}
	return w.customCursors
}

func (c *customCursorCache) get(key string) (uintptr, bool) {
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*customCursorEntry).cursor, true
	}
	return 0, false
}

func (c *customCursorCache) put(key string, cur uintptr) {
	if el, ok := c.entries[key]; ok {
		ent := el.Value.(*customCursorEntry)
		if ent.cursor != 0 && ent.cursor != cur {
			gObjectUnref(ent.cursor)
		}
		ent.cursor = cur
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(&customCursorEntry{key: key, cursor: cur})
	c.entries[key] = el
	for c.order.Len() > maxCustomCursors {
		back := c.order.Back()
		ent := back.Value.(*customCursorEntry)
		c.order.Remove(back)
		delete(c.entries, ent.key)
		if ent.cursor != 0 {
			gObjectUnref(ent.cursor)
		}
	}
}

func (w *window) closeCustomCursors() {
	c := w.customCursors
	if c == nil {
		return
	}
	for el := c.order.Front(); el != nil; el = el.Next() {
		if cur := el.Value.(*customCursorEntry).cursor; cur != 0 {
			gObjectUnref(cur)
		}
	}
	c.order.Init()
	clear(c.entries)
	clear(c.loading)
}

func isRemoteCursorURI(uri string) bool {
	return strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://")
}

func (w *window) displayScale() float32 {
	if w.env.PixelRatio > 0 {
		return w.env.PixelRatio
	}
	return 1
}

// customGdkCursor returns a cached GdkCursor for cc at the window's HiDPI
// scale (selecting Image2x when scale >= 1.5), together with its cache key.
// Returns 0 while loading or if cc is empty/invalid.
func (w *window) customGdkCursor(cc *driver.CustomCursor, named driver.Cursor) (uintptr, string) {
	if cc.Empty() {
		return 0, ""
	}
	scale := w.displayScale()
	src, is2x := cc.SelectSource(scale)
	if src.Empty() {
		return 0, ""
	}
	density := float32(1)
	if is2x {
		density = 2
	} else if d := src.Density(); d > 0 {
		density = d
	}
	fb := cc.EffectiveFallback(named)
	key := fmt.Sprintf("%s|2x:%v|d:%g|hot:%g,%g|fb:%d", src.Key(), is2x, density, cc.Hotspot.X, cc.Hotspot.Y, fb)
	cache := w.cursorCache()
	if cur, ok := cache.get(key); ok {
		return cur, key
	}
	if isRemoteCursorURI(src.URI) {
		if !cache.loading[key] {
			cache.loading[key] = true
			specHotspot := cc.Hotspot
			go func() {
				img := decodeLinuxCursorSource(src, scale)
				w.post(func() {
					if w.surface == 0 {
						return
					}
					delete(w.cursorCache().loading, key)
					cur := createGdkCursor(img, specHotspot, density, fb)
					w.cursorCache().put(key, cur)
					// Invalidate surface cursor tracking so the newly loaded cursor applies.
					w.surfaceCursorSet = false
					w.updatePointerCursor()
				})
			}()
		}
		return 0, ""
	}
	img := decodeLinuxCursorSource(src, scale)
	cur := createGdkCursor(img, cc.Hotspot, density, fb)
	cache.put(key, cur)
	return cur, key
}

func decodeLinuxCursorSource(src driver.ImageSource, scale float32) *imagedecode.Image {
	if src.Empty() {
		return nil
	}
	if src.Pixels != nil && !src.Pixels.Rect.Empty() {
		return imagedecode.FromPixels(src.Pixels, imagedecode.BGRA)
	}
	data, err := imageload.Load(context.Background(), src, nil)
	if err != nil || len(data) == 0 {
		return nil
	}
	img, err := imagedecode.Decode(data, imagedecode.Options{Layout: imagedecode.BGRA, Scale: scale})
	if err != nil || img == nil || len(img.Frames) == 0 || img.Width <= 0 || img.Height <= 0 {
		return nil
	}
	return img
}

// createGdkCursor builds a GdkCursor from premultiplied BGRA pixels via
// gdk_memory_texture_new + gdk_cursor_new_from_texture with clamped hotspot.
func createGdkCursor(img *imagedecode.Image, hotspot driver.Point, density float32, fallback driver.Cursor) uintptr {
	if img == nil || len(img.Frames) == 0 || img.Width <= 0 || img.Height <= 0 {
		return 0
	}
	pix := img.Frames[0].Pix
	if len(pix) < img.Width*img.Height*4 {
		return 0
	}
	bytes := gBytesNew(unsafe.Pointer(&pix[0]), uintptr(img.Width*img.Height*4))
	if bytes == 0 {
		return 0
	}
	const bgraPremultiplied = 0
	tex := gdkMemoryTextureNew(int32(img.Width), int32(img.Height), bgraPremultiplied, bytes, uintptr(img.Width*4))
	gBytesUnref(bytes)
	if tex == 0 {
		return 0
	}
	defer gObjectUnref(tex)

	var fb uintptr
	if fallback != driver.CursorAuto {
		fb = gdkCursorNewFromName(cstr(fallback.Name()), 0)
		if fb != 0 {
			defer gObjectUnref(fb)
		}
	}
	hx, hy := driver.ClampHotspotPixels(hotspot, img.Width, img.Height, density)
	return gdkCursorNewFromTexture(tex, int32(hx), int32(hy), fb)
}
