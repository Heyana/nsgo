//go:build windows

package windows

import (
	"context"
	"math"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

var setMenuItemInfoW = user32.NewProc("SetMenuItemInfoW")

const (
	miimBitmap = 0x00000080
	smCxSmIcon = 49
)

type menuItemInfo struct {
	cbSize, fMask, fType, fState, wID    uint32
	hSubMenu, hbmpChecked, hbmpUnchecked uintptr
	dwItemData, dwTypeData               uintptr
	cch                                  uint32
	hbmpItem                             uintptr
}

// menuIconSize returns the pixel size for menu item bitmaps at w's DPI.
func (w *window) menuIconSize() int {
	if w != nil && w.hwnd != 0 && getSystemMetricsForDpi.Find() == nil {
		dpi, _, _ := getDpiForWindow.Call(w.hwnd)
		if dpi == 0 && w.scale > 0 {
			dpi = uintptr(w.scale * 96)
		}
		if dpi > 0 {
			if s, _, _ := getSystemMetricsForDpi.Call(smCxSmIcon, dpi); s >= 16 && s <= 64 {
				return int(s)
			}
		}
	}
	scale := 1.0
	if w != nil && w.scale > 0 {
		scale = w.scale
	}
	return max(16, min(64, int(math.Round(16*scale))))
}

// menuIconPixels decodes, fits and optionally tints icon into a size×size premultiplied BGRA pixel buffer, or returns nil when empty or invalid.
func menuIconPixels(icon driver.MenuIcon, size int, dark bool) []byte {
	if icon.Empty() || size <= 0 {
		return nil
	}
	ink := driver.RGB(0x000000)
	if dark {
		ink = driver.RGB(0xffffff)
	}
	if icon.SVG != nil {
		img, err := imagedecode.RasterizeSVG(icon.SVG, size, size, ink, imagedecode.BGRA)
		if err != nil || img == nil || len(img.Frames) == 0 {
			return nil
		}
		pix := img.Frames[0].Pix
		if icon.Template && !icon.SVG.Monochrome() {
			imagedecode.TintPremultiplied(pix, imagedecode.BGRA, ink)
		}
		return pix
	}
	img := decodeWindowsMenuIcon(icon, size)
	if img == nil || len(img.Frames) == 0 || img.Width <= 0 || img.Height <= 0 {
		return nil
	}
	pix := imagedecode.FitPremultiplied(img.Frames[0].Pix, img.Width, img.Height, size, size)
	if pix == nil {
		return nil
	}
	if icon.Template {
		imagedecode.TintPremultiplied(pix, imagedecode.BGRA, ink)
	}
	return pix
}

func decodeWindowsMenuIcon(icon driver.MenuIcon, size int) *imagedecode.Image {
	opts := imagedecode.Options{Layout: imagedecode.BGRA, Cover: size}
	if len(icon.Data) > 0 {
		img, err := imagedecode.Decode(icon.Data, opts)
		if err != nil {
			return nil
		}
		return img
	}
	src := icon.Source
	if src.Empty() {
		return nil
	}
	if src.Pixels != nil && !src.Pixels.Rect.Empty() {
		return imagedecode.FromPixels(src.Pixels, imagedecode.BGRA)
	}
	if len(src.Data) > 0 {
		img, err := imagedecode.Decode(src.Data, opts)
		if err != nil {
			return nil
		}
		return img
	}
	if isRemoteCursorURI(src.URI) {
		return nil
	}
	data, err := imageload.Load(context.Background(), src, nil)
	if err != nil || len(data) == 0 {
		return nil
	}
	img, err := imagedecode.Decode(data, opts)
	if err != nil {
		return nil
	}
	return img
}

// menuIconBitmap creates a 32-bit premultiplied BGRA HBITMAP for icon at w's DPI. The caller owns the returned HBITMAP and must free it with DeleteObject after the menu that references it is destroyed.
func (w *window) menuIconBitmap(icon driver.MenuIcon) uintptr {
	size := w.menuIconSize()
	dark := w != nil && w.shownDark()
	pix := menuIconPixels(icon, size, dark)
	if len(pix) < size*size*4 {
		return 0
	}
	return createMenuBitmap(pix, size)
}

func createMenuBitmap(pix []byte, size int) uintptr {
	bi := bitmapInfo{
		Width:    int32(size),
		Height:   -int32(size), // top-down DIB
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
	copy(unsafe.Slice((*byte)(bits), size*size*4), pix[:size*size*4])
	return dib
}

func attachMenuIcon(menu uintptr, pos uint32, hbmp uintptr) {
	mi := menuItemInfo{fMask: miimBitmap, hbmpItem: hbmp}
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	setMenuItemInfoW.Call(menu, uintptr(pos), 1, uintptr(unsafe.Pointer(&mi)))
}

func freeBitmaps(handles []uintptr) {
	for _, h := range handles {
		if h != 0 {
			deleteObject.Call(h)
		}
	}
}

// freeMenuBitmaps frees the HBITMAPs of the menu bar's items, which Windows does not free with the menu.
func (w *window) freeMenuBitmaps() {
	releaseWinMenuTree(win32MenuOps{w}, w.chrome.tree)
	w.chrome.tree = nil
}

func menuHasTemplateIcon(entries []driver.MenuEntry) bool {
	for i := range entries {
		e := &entries[i]
		if !e.Icon.Empty() && e.Icon.IsTemplate() {
			return true
		}
		if len(e.Items) > 0 && menuHasTemplateIcon(e.Items) {
			return true
		}
	}
	return false
}
