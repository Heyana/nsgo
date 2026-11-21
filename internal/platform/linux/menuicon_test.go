//go:build linux && !android

package linux

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func testLinuxMenuPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
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

// A serialized GMenu icon is insufficient: GTK normally hides it beside text.
// Open real native menus and check the image and label are mapped and allocated,
// while native model buttons still provide focus, check/radio state and actions.
func TestMenuIconsAreVisibleInNativeRows(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	icon := core.MenuIcon{Data: testLinuxMenuPNG(t, 16, 16, color.NRGBA{R: 0xf0, G: 0x20, B: 0x60, A: 0xff})}
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		selected := core.NewSignal(cx, "ready")
		cx.Effect(func() { cx.SetTitle("menu-icons|" + selected.Get()) })
		cx.SetApplicationMenu([]core.MenuItem{{Label: "Actions", Items: []core.MenuItem{
			{Label: "Plain", Icon: icon, OnSelect: func() { selected.Set("plain") }},
			{Label: "Checked", Icon: icon, Checkable: true, Checked: true, OnSelect: func() {}},
			{Label: "Radio", Icon: icon, Radio: true, Checked: true, OnSelect: func() {}},
			{Label: "Disabled", Icon: icon, Disabled: true, OnSelect: func() {}},
			{Label: "More", Icon: icon, Items: []core.MenuItem{{Label: "Inside", Icon: icon}}},
		}}})
		return core.View{}
	}, Options{Title: "menu-icons", Width: 360, Height: 240})
	waitForTitle(t, w, "menu-icons|ready")
	var getCSSName func(uintptr) uintptr
	var popoverType func() uintptr
	var getSensitive func(uintptr) int32
	var labelType func() uintptr
	var labelText func(uintptr) uintptr
	w.onLoop(t, func() {
		purego.RegisterLibFunc(&getCSSName, gtkLib, "gtk_widget_get_css_name")
		purego.RegisterLibFunc(&popoverType, gtkLib, "gtk_popover_get_type")
		purego.RegisterLibFunc(&getSensitive, gtkLib, "gtk_widget_get_sensitive")
		purego.RegisterLibFunc(&labelType, gtkLib, "gtk_label_get_type")
		purego.RegisterLibFunc(&labelText, gtkLib, "gtk_label_get_text")
	})
	var mappedLabel func(uintptr, string) bool
	mappedLabel = func(widget uintptr, text string) bool {
		if gTypeIsA(widget, labelType()) != 0 && goString(labelText(widget)) == text {
			return gtkWidgetGetMapped(widget) != 0 && gtkWidgetGetWidth(widget) > 0
		}
		for child := gtkWidgetGetFirstChild(widget); child != 0; child = gtkWidgetGetNextSibling(child) {
			if mappedLabel(child, text) {
				return true
			}
		}
		return false
	}
	var findPopover func(uintptr) uintptr
	findPopover = func(widget uintptr) uintptr {
		if gTypeIsA(widget, popoverType()) != 0 {
			return widget
		}
		for child := gtkWidgetGetFirstChild(widget); child != 0; child = gtkWidgetGetNextSibling(child) {
			if popover := findPopover(child); popover != 0 {
				return popover
			}
		}
		return 0
	}
	type nativeRow struct{ row, image uintptr }
	rows := func(root uintptr) map[string]nativeRow {
		found := map[string]nativeRow{}
		var walk func(uintptr)
		walk = func(widget uintptr) {
			if gTypeIsA(widget, gtkImageGetType()) != 0 {
				if icon := gtkImageGetGicon(widget); icon != 0 && gTypeIsA(icon, gBytesIconGetType()) != 0 {
					row := gtkWidgetGetParent(widget)
					for row != 0 && goString(getCSSName(row)) != "modelbutton" {
						row = gtkWidgetGetParent(row)
					}
					if row != 0 {
						var text uintptr
						gObjectGet(row, cstr("text"), &text, uintptr(0))
						found[goString(text)] = nativeRow{row, widget}
						gFree(text)
					}
				}
			}
			for child := gtkWidgetGetFirstChild(widget); child != 0; child = gtkWidgetGetNextSibling(child) {
				walk(child)
			}
		}
		walk(root)
		return found
	}
	waitMapped := func(root uintptr, names ...string) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
			ready := true
			w.onLoop(t, func() {
				found := rows(root)
				for _, name := range names {
					r := found[name]
					if r.image == 0 || gtkWidgetGetMapped(r.image) == 0 || gtkWidgetGetWidth(r.image) <= 0 || gtkWidgetGetHeight(r.image) <= 0 || !mappedLabel(r.row, name) {
						ready = false
					}
				}
			})
			if ready {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("menu icons and labels %v did not map with a nonzero allocation", names)
			}
		}
	}
	var popover uintptr
	w.onLoop(t, func() {
		popover = findPopover(w.menuBar)
		if popover == 0 {
			t.Error("menu bar has no native popover")
			return
		}
		gtkPopoverPopup(popover)
	})
	if popover == 0 {
		return
	}
	waitMapped(popover, "Plain", "Checked", "Radio", "Disabled", "More")
	w.onLoop(t, func() {
		found := rows(popover)
		for _, tc := range []struct {
			name string
			role int32
		}{{"Plain", 0}, {"Checked", 1}, {"Radio", 2}} {
			r := found[tc.name]
			var role, active, iconic int32
			gObjectGet(r.row, cstr("role"), &role, cstr("active"), &active, cstr("iconic"), &iconic, uintptr(0))
			if role != tc.role || iconic != 0 || (tc.role != 0 && active != 1) {
				t.Errorf("%s: role=%d active=%d iconic=%d", tc.name, role, active, iconic)
			}
			if gtkWidgetGrabFocus(r.row) == 0 {
				t.Errorf("%s: native row did not take keyboard focus", tc.name)
			}
			// A GTK update must not hide the app's image again.
			gtkWidgetSetVisible(r.image, 0)
			if gtkWidgetGetVisible(r.image) == 0 {
				t.Errorf("%s: image stayed hidden after visibility update", tc.name)
			}
		}
		if getSensitive(found["Disabled"].row) != 0 {
			t.Error("disabled row became sensitive")
		}
		gSignalEmitByName(found["More"].row, cstr("clicked"))
	})
	waitMapped(popover, "Inside")
	w.onLoop(t, func() {
		gtkPopoverPopdown(popover)
		w.popupMenu([]driver.MenuEntry{{Label: "Popup", ID: 100, Icon: icon}}, driver.Point{X: 40, Y: 40}, 10)
		popover = w.popover
	})
	waitMapped(popover, "Popup")
	w.onLoop(t, func() { gtkPopoverPopdown(popover) })
	// Rebuild and activate an icon-bearing native row through GTK's own signal.
	w.onLoop(t, func() {
		w.setMenu(w.chrome.menu)
		popover = findPopover(w.menuBar)
		gtkPopoverPopup(popover)
	})
	waitMapped(popover, "Plain")
	w.onLoop(t, func() { gSignalEmitByName(rows(popover)["Plain"].row, cstr("clicked")) })
	waitForTitle(t, w, "menu-icons|plain")
}

func TestMenuIconPNG(t *testing.T) {
	pngData := testLinuxMenuPNG(t, 32, 32, color.NRGBA{R: 0x20, G: 0x70, B: 0xd0, A: 0xff})
	monoSVG, err := driver.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect x="2" y="2" width="20" height="20" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}

	out := menuIconPNG(driver.MenuIcon{Data: pngData}, 16, false)
	dec, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode menuIconPNG: %v", err)
	}
	if dec.Bounds().Dx() != 16 || dec.Bounds().Dy() != 16 {
		t.Fatalf("size = %v, want 16x16", dec.Bounds())
	}

	// Dark monochrome SVG renders white.
	darkOut := menuIconPNG(driver.MenuIcon{SVG: monoSVG}, 16, true)
	darkImg, err := png.Decode(bytes.NewReader(darkOut))
	if err != nil {
		t.Fatalf("decode dark SVG PNG: %v", err)
	}
	r, g, b, a := darkImg.At(8, 8).RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff || a != 0xffff {
		t.Fatalf("dark SVG center = %x %x %x %x, want opaque white", r, g, b, a)
	}

	if got := menuIconPNG(driver.MenuIcon{Data: []byte("bad")}, 16, false); got != nil {
		t.Fatal("invalid bytes produced PNG")
	}
	if got := menuIconPNG(driver.MenuIcon{Source: driver.ImageSource{URI: "https://example.invalid/a.png"}}, 16, false); got != nil {
		t.Fatal("remote URI produced PNG synchronously")
	}
	if !menuHasTemplateIcon([]driver.MenuEntry{{Label: "File", Items: []driver.MenuEntry{{Label: "New", Icon: driver.MenuIcon{SVG: monoSVG}}}}}) {
		t.Error("menuHasTemplateIcon missed nested monochrome SVG")
	}
}

func TestGMenuItemAttachesSerializedIcon(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	var (
		getAttr     func(model uintptr, itemIndex int32, attr *byte, expectedType uintptr) uintptr
		getItemLink func(model uintptr, itemIndex int32, link *byte) uintptr
	)
	purego.RegisterLibFunc(&getAttr, gioLib, "g_menu_model_get_item_attribute_value")
	purego.RegisterLibFunc(&getItemLink, gioLib, "g_menu_model_get_item_link")

	pngData := testLinuxMenuPNG(t, 16, 16, color.NRGBA{R: 0x30, G: 0xa0, B: 0x50, A: 0xff})
	monoSVG, err := driver.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}

	w := &window{actionRefs: map[uintptr]menuRef{}, actionGroup: gSimpleActionGroupNew()}
	defer gObjectUnref(w.actionGroup)

	sub := w.buildSubmenu([]driver.MenuEntry{
		{Label: "With PNG", ID: 1, Icon: driver.MenuIcon{Data: pngData}},
		{Label: "Plain", ID: 2},
		{Label: "Sub", Icon: driver.MenuIcon{SVG: monoSVG}, Items: []driver.MenuEntry{{Label: "Inside", ID: 3}}},
	})
	defer gObjectUnref(sub)

	section := getItemLink(sub, 0, cstr("section"))
	if section == 0 {
		t.Fatal("submenu has no section")
	}
	defer gObjectUnref(section)

	if v := getAttr(section, 0, cstr("icon"), 0); v == 0 {
		t.Error("item 0 (With PNG) has no icon attribute")
	} else {
		gVariantUnref(v)
	}
	if v := getAttr(section, 1, cstr("icon"), 0); v != 0 {
		gVariantUnref(v)
		t.Error("item 1 (Plain) unexpectedly has an icon attribute")
	}
	if v := getAttr(section, 2, cstr("icon"), 0); v == 0 {
		t.Error("item 2 (Submenu with SVG) has no icon attribute")
	} else {
		gVariantUnref(v)
	}

	// dbusmenu icon-data property for Linux tray menus.
	trayBind()
	withIcon := &driver.MenuEntry{Label: "Open", Icon: driver.MenuIcon{Data: pngData}}
	if v := itemProperty(withIcon, false, "icon-data"); v == 0 {
		t.Error("dbusmenu itemProperty icon-data returned 0 for item with icon")
	} else {
		gVariantUnref(v)
	}
	plain := &driver.MenuEntry{Label: "Plain"}
	if v := itemProperty(plain, false, "icon-data"); v != 0 {
		gVariantUnref(v)
		t.Error("dbusmenu itemProperty icon-data returned non-zero for plain item")
	}
}
