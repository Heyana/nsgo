//go:build windows

package windows

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

var destroyCursor = user32.NewProc("DestroyCursor")

const maxCustomCursors = 64

type customCursorEntry struct {
	key    string
	handle uintptr // HCURSOR from CreateIconIndirect, or 0 on failure
}

type customCursorCache struct {
	order   *list.List
	entries map[string]*list.Element
	loading map[string]bool
	active  uintptr
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
		return el.Value.(*customCursorEntry).handle, true
	}
	return 0, false
}

func (c *customCursorCache) put(key string, h uintptr) {
	if el, ok := c.entries[key]; ok {
		ent := el.Value.(*customCursorEntry)
		if ent.handle != 0 && ent.handle != h {
			c.destroyHandle(ent.handle)
		}
		ent.handle = h
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(&customCursorEntry{key: key, handle: h})
	c.entries[key] = el
	for c.order.Len() > maxCustomCursors {
		back := c.order.Back()
		ent := back.Value.(*customCursorEntry)
		c.order.Remove(back)
		delete(c.entries, ent.key)
		if ent.handle != 0 {
			c.destroyHandle(ent.handle)
		}
	}
}

func (c *customCursorCache) destroyHandle(h uintptr) {
	if h == 0 {
		return
	}
	if c.active == h {
		arrow, _, _ := loadCursorW.Call(0, idcArrow)
		setCursor.Call(arrow)
		c.active = 0
	}
	destroyCursor.Call(h)
}

func (w *window) closeCustomCursors() {
	c := w.customCursors
	if c == nil {
		return
	}
	for el := c.order.Front(); el != nil; el = el.Next() {
		if h := el.Value.(*customCursorEntry).handle; h != 0 {
			c.destroyHandle(h)
		}
	}
	c.order.Init()
	clear(c.entries)
	clear(c.loading)
	c.active = 0
}

func isRemoteCursorURI(uri string) bool {
	return strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://")
}

// customHCURSOR returns a cached Windows HCURSOR for cc at the window's DPI
// scale (selecting Image2x when scale >= 1.5), or 0 while loading / on error.
func (w *window) customHCURSOR(cc *driver.CustomCursor) uintptr {
	if cc.Empty() {
		return 0
	}
	scale := float32(w.scale)
	if scale <= 0 {
		scale = 1
	}
	src, is2x := cc.SelectSource(scale)
	if src.Empty() {
		return 0
	}
	density := float32(1)
	if is2x {
		density = 2
	} else if d := src.Density(); d > 0 {
		density = d
	}
	key := fmt.Sprintf("%s|2x:%v|d:%g|hot:%g,%g", src.Key(), is2x, density, cc.Hotspot.X, cc.Hotspot.Y)
	cache := w.cursorCache()
	if h, ok := cache.get(key); ok {
		return h
	}
	if isRemoteCursorURI(src.URI) {
		if !cache.loading[key] {
			cache.loading[key] = true
			specHotspot := cc.Hotspot
			go func() {
				img := decodeWindowsCursorSource(src, scale)
				w.post(func() {
					if w.destroying {
						return
					}
					delete(w.cursorCache().loading, key)
					h := createWindowsCursor(img, specHotspot, density)
					w.cursorCache().put(key, h)
					w.refreshCursor()
				})
			}()
		}
		return 0
	}
	img := decodeWindowsCursorSource(src, scale)
	h := createWindowsCursor(img, cc.Hotspot, density)
	cache.put(key, h)
	return h
}

func decodeWindowsCursorSource(src driver.ImageSource, scale float32) *imagedecode.Image {
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

// createWindowsCursor builds a 32-bit BGRA DIB + 1bpp mask HCURSOR via
// CreateDIBSection and CreateIconIndirect with Icon=0 and clamped hotspot.
func createWindowsCursor(img *imagedecode.Image, hotspot driver.Point, density float32) uintptr {
	if img == nil || len(img.Frames) == 0 || img.Width <= 0 || img.Height <= 0 {
		return 0
	}
	w, h := img.Width, img.Height
	srcPix := img.Frames[0].Pix
	if len(srcPix) < w*h*4 {
		return 0
	}
	bi := bitmapInfo{
		Width:    int32(w),
		Height:   -int32(h), // top-down DIB
		Planes:   1,
		BitCount: 32,
	}
	bi.Size = uint32(unsafe.Sizeof(bi)) - 4
	dc, _, _ := createCompatibleDC.Call(0)
	if dc == 0 {
		return 0
	}
	defer deleteDC.Call(dc)

	var bits unsafe.Pointer
	dib, _, _ := createDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 || bits == nil {
		return 0
	}
	defer deleteObject.Call(dib)

	dstPix := unsafe.Slice((*byte)(bits), w*h*4)
	copy(dstPix, srcPix[:w*h*4])

	mask, _, _ := createBitmap.Call(uintptr(w), uintptr(h), 1, 1, 0)
	if mask == 0 {
		return 0
	}
	defer deleteObject.Call(mask)

	hx, hy := driver.ClampHotspotPixels(hotspot, w, h, density)
	info := iconInfo{
		Icon:  0, // cursor, not icon
		HotX:  uint32(hx),
		HotY:  uint32(hy),
		Mask:  mask,
		Color: dib,
	}
	hcur, _, _ := createIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	return hcur
}
