//go:build linux && !android

package linux

import (
	"context"
	"math"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

func (w *window) shownDark() bool {
	if w == nil {
		return false
	}
	switch w.appearance {
	case driver.AppearanceDark:
		return true
	case driver.AppearanceLight:
		return false
	}
	return w.env.Dark
}

func (w *window) menuIconSize() int {
	scale := float64(w.displayScale())
	return max(16, min(64, int(math.Round(16*scale))))
}

// attachMenuIcon attaches icon to a GMenuItem via a GBytesIcon (which
// implements g_icon_serialize so GMenuModel stores it for GtkPopoverMenu).
func (w *window) attachMenuIcon(item uintptr, icon driver.MenuIcon) {
	if item == 0 || icon.Empty() {
		return
	}
	pngData := menuIconPNG(icon, w.menuIconSize(), w.shownDark())
	if len(pngData) == 0 {
		return
	}
	bytes := gBytesNew(unsafe.Pointer(&pngData[0]), uintptr(len(pngData)))
	if bytes == 0 {
		return
	}
	gicon := gBytesIconNew(bytes)
	gBytesUnref(bytes)
	if gicon == 0 {
		return
	}
	gMenuItemSetIcon(item, gicon)
	gObjectUnref(gicon)
}

// showMenuIcons keeps GTK's native menu rows, including their labels, actions,
// check/radio indicators and accessibility. GtkModelButton normally hides its
// image alongside text; show the GBytesIcon images NSGo supplied, using public
// widget/image APIs without depending on the private row type or child order.
// Menus and their submenus are constructed before this walk and rebuilt together.
func showMenuIcons(widget uintptr) {
	if widget == 0 {
		return
	}
	if gTypeIsA(widget, gtkImageGetType()) != 0 {
		icon := gtkImageGetGicon(widget)
		if icon != 0 && gTypeIsA(icon, gBytesIconGetType()) != 0 {
			if gObjectGetData(widget, cstr("nsgo-menu-icon")) == 0 {
				gObjectSetData(widget, cstr("nsgo-menu-icon"), 1)
				gSignalConnect(widget, "notify::visible", cbMenuIconVisible, 0)
			}
			gtkWidgetSetVisible(widget, 1)
		}
	}
	for child := gtkWidgetGetFirstChild(widget); child != 0; child = gtkWidgetGetNextSibling(child) {
		showMenuIcons(child)
	}
}

func menuIconVisibleCallback(image, spec, data uintptr) {
	// GTK can reapply its visibility policy when updating the label or shortcut.
	// Setting an already visible widget does not emit another notification.
	if gtkWidgetGetVisible(image) == 0 {
		gtkWidgetSetVisible(image, 1)
	}
}

// menuIconPNG decodes, fits, optionally tints and encodes icon as a size×size
// PNG suitable for GBytesIcon and com.canonical.dbusmenu icon-data.
func menuIconPNG(icon driver.MenuIcon, size int, dark bool) []byte {
	if icon.Empty() || size <= 0 {
		return nil
	}
	ink := driver.RGB(0x000000)
	if dark {
		ink = driver.RGB(0xffffff)
	}
	var pix []byte
	if icon.SVG != nil {
		img, err := imagedecode.RasterizeSVG(icon.SVG, size, size, ink, imagedecode.RGBA)
		if err != nil || img == nil || len(img.Frames) == 0 {
			return nil
		}
		pix = img.Frames[0].Pix
		if icon.Template && !icon.SVG.Monochrome() {
			imagedecode.TintPremultiplied(pix, imagedecode.RGBA, ink)
		}
	} else {
		img := decodeLinuxMenuIcon(icon, size)
		if img == nil || len(img.Frames) == 0 || img.Width <= 0 || img.Height <= 0 {
			return nil
		}
		pix = imagedecode.FitPremultiplied(img.Frames[0].Pix, img.Width, img.Height, size, size)
		if pix == nil {
			return nil
		}
		if icon.Template {
			imagedecode.TintPremultiplied(pix, imagedecode.RGBA, ink)
		}
	}
	encoded, err := imagedecode.EncodePNG(pix, size, size, imagedecode.RGBA)
	if err != nil {
		return nil
	}
	return encoded
}

func decodeLinuxMenuIcon(icon driver.MenuIcon, size int) *imagedecode.Image {
	opts := imagedecode.Options{Layout: imagedecode.RGBA, Cover: size}
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
		return imagedecode.FromPixels(src.Pixels, imagedecode.RGBA)
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
