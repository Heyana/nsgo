//go:build windows

package windows

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func testMenuIconPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMenuIconPixels(t *testing.T) {
	pngData := testMenuIconPNG(t, 32, 32, color.NRGBA{R: 0x10, G: 0x80, B: 0xe0, A: 0xff})
	monoSVG, err := driver.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect x="2" y="2" width="20" height="20" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}

	pix := menuIconPixels(driver.MenuIcon{Data: pngData}, 16, false)
	if len(pix) != 16*16*4 {
		t.Fatalf("PNG pix len = %d, want %d", len(pix), 16*16*4)
	}
	c := (8*16 + 8) * 4
	if pix[c] != 0xe0 || pix[c+1] != 0x80 || pix[c+2] != 0x10 || pix[c+3] != 0xff {
		t.Fatalf("BGRA = %x %x %x %x, want e0 80 10 ff", pix[c], pix[c+1], pix[c+2], pix[c+3])
	}

	// Template PNG tints black in light mode and white in dark mode.
	lightTpl := menuIconPixels(driver.MenuIcon{Data: pngData, Template: true}, 16, false)
	darkTpl := menuIconPixels(driver.MenuIcon{Data: pngData, Template: true}, 16, true)
	if lightTpl[c] != 0 || lightTpl[c+1] != 0 || lightTpl[c+2] != 0 || lightTpl[c+3] != 0xff {
		t.Fatalf("light template BGRA = %x %x %x %x, want 00 00 00 ff", lightTpl[c], lightTpl[c+1], lightTpl[c+2], lightTpl[c+3])
	}
	if darkTpl[c] != 0xff || darkTpl[c+1] != 0xff || darkTpl[c+2] != 0xff || darkTpl[c+3] != 0xff {
		t.Fatalf("dark template BGRA = %x %x %x %x, want ff ff ff ff", darkTpl[c], darkTpl[c+1], darkTpl[c+2], darkTpl[c+3])
	}

	// Monochrome SVG paints white in dark mode and black in light mode.
	svgDark := menuIconPixels(driver.MenuIcon{SVG: monoSVG}, 16, true)
	if svgDark[c] != 0xff || svgDark[c+1] != 0xff || svgDark[c+2] != 0xff || svgDark[c+3] != 0xff {
		t.Fatalf("dark SVG BGRA = %x %x %x %x, want ff ff ff ff", svgDark[c], svgDark[c+1], svgDark[c+2], svgDark[c+3])
	}

	if got := menuIconPixels(driver.MenuIcon{Data: []byte("bad")}, 16, false); got != nil {
		t.Fatal("invalid bytes produced pixels")
	}
	if got := menuIconPixels(driver.MenuIcon{Source: driver.ImageSource{URI: "https://example.invalid/a.png"}}, 16, false); got != nil {
		t.Fatal("remote URI produced pixels synchronously")
	}
	if !menuHasTemplateIcon([]driver.MenuEntry{{Label: "File", Items: []driver.MenuEntry{{Label: "Folder", Icon: driver.MenuIcon{SVG: monoSVG}}}}}) {
		t.Error("menuHasTemplateIcon missed nested monochrome SVG")
	}
}

func TestMenuAttachesAndFreesHBITMAPs(t *testing.T) {
	getItem := user32.NewProc("GetMenuItemInfoW")
	pngData := testMenuIconPNG(t, 16, 16, color.NRGBA{R: 0x30, G: 0x90, B: 0xd0, A: 0xff})
	monoSVG, err := driver.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}

	menu, _, _ := createPopupMenu.Call()
	if menu == 0 {
		t.Fatal("CreatePopupMenu returned 0")
	}
	var refs []menuRef
	var bitmaps []uintptr
	entries := []driver.MenuEntry{
		{Label: "PNG", ID: 1, Icon: driver.MenuIcon{Data: pngData}},
		{Separator: true},
		{Label: "Plain", ID: 2},
		{Label: "Sub", Icon: driver.MenuIcon{SVG: monoSVG}, Items: []driver.MenuEntry{
			{Label: "SVG", ID: 3, Icon: driver.MenuIcon{SVG: monoSVG}},
		}},
	}
	(&window{scale: 1}).appendMenuTo(menu, entries, &refs, &bitmaps)
	if len(bitmaps) != 3 {
		t.Fatalf("created %d bitmaps, want 3", len(bitmaps))
	}

	hbmpAt := func(m uintptr, pos uintptr) uintptr {
		mi := menuItemInfo{fMask: miimBitmap}
		mi.cbSize = uint32(unsafe.Sizeof(mi))
		getItem.Call(m, pos, 1, uintptr(unsafe.Pointer(&mi)))
		return mi.hbmpItem
	}
	if h := hbmpAt(menu, 0); h == 0 {
		t.Error("PNG item has no hbmpItem")
	}
	if h := hbmpAt(menu, 2); h != 0 {
		t.Errorf("Plain item has hbmpItem %v, want 0", h)
	}
	if h := hbmpAt(menu, 3); h == 0 {
		t.Error("Submenu holder item has no hbmpItem")
	}

	destroyMenu.Call(menu)
	freeBitmaps(bitmaps)
}
