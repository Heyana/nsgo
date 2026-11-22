//go:build darwin && !ios

package macos

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func testIconPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
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

func TestMenuItemIcons(t *testing.T) {
	onMain(func() { menuItemIcons(t) })
}

var (
	nativeMenuIconProbe      func()
	nativeMenuIconProbeClass objc.Class
)

// Render a real popup while AppKit is tracking it, rather than merely checking that NSMenuItem retains an NSImage (which AppKit 27 can leave hidden).
func TestMenuIconDrawsInNativePopup(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a native menu")
	}
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		s := &surface{}
		s.ensureMenuTarget()
		defer delete(menuTargets, s.menuTarget)
		menu := s.submenu("", []driver.MenuEntry{{Label: "Native icon", ID: 1, Icon: driver.MenuIcon{
			Data: testIconPNG(t, 16, 16, color.NRGBA{R: 0xf0, G: 0x20, B: 0x60, A: 0xff}),
		}}})
		defer menu.Send(selRelease)
		app := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
		old := map[objc.ID]bool{}
		windows := app.Send(sel("windows"))
		for i, n := 0, objc.Send[int](windows, sel("count")); i < n; i++ {
			old[windows.Send(sel("objectAtIndex:"), i)] = true
		}
		var shot image.Image
		probed := false
		nativeMenuIconProbe = func() {
			defer menu.Send(sel("cancelTracking"))
			probed = true
			windows := app.Send(sel("windows"))
			for i, n := 0, objc.Send[int](windows, sel("count")); i < n; i++ {
				win := windows.Send(sel("objectAtIndex:"), i)
				if old[win] || !objc.Send[bool](win, sel("isVisible")) {
					continue
				}
				view := win.Send(sel("contentView"))
				bounds := objc.Send[apple.CGRect](view, selBounds)
				rep := view.Send(sel("bitmapImageRepForCachingDisplayInRect:"), bounds)
				if rep == 0 {
					continue
				}
				view.Send(sel("cacheDisplayInRect:toBitmapImageRep:"), bounds, rep)
				data := rep.Send(sel("representationUsingType:properties:"), uint(4), objc.ID(apple.Class("NSDictionary")).Send(sel("dictionary")))
				if data != 0 {
					shot, _ = png.Decode(bytes.NewReader(apple.Bytes(data)))
				}
				if shot != nil {
					break
				}
			}
		}
		defer func() { nativeMenuIconProbe = nil }()
		if nativeMenuIconProbeClass == 0 {
			nativeMenuIconProbeClass = apple.RegisterClass("NsgoTestMenuIconProbe", apple.Class("NSObject"), nil,
				[]objc.MethodDef{{Cmd: sel("probe:"), Fn: func(objc.ID, objc.SEL, objc.ID) {
					if nativeMenuIconProbe != nil {
						nativeMenuIconProbe()
					}
				}}})
		}
		target := objc.ID(nativeMenuIconProbeClass).Send(selNew)
		defer target.Send(selRelease)
		timer := objc.ID(apple.Class("NSTimer")).Send(sel("timerWithTimeInterval:target:selector:userInfo:repeats:"),
			0.3, target, sel("probe:"), objc.ID(0), false)
		objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")).Send(sel("addTimer:forMode:"), timer, apple.CommonModes())
		defer timer.Send(sel("invalidate"))
		menu.Send(sel("popUpMenuPositioningItem:atLocation:inView:"), objc.ID(0), apple.CGPoint{X: 200, Y: 300}, objc.ID(0))
		if !probed {
			t.Fatal("native popup did not run its tracking timer")
		}
		if shot == nil {
			t.Fatal("native popup did not expose a rendered menu window")
		}
		colored := 0
		for y := shot.Bounds().Min.Y; y < shot.Bounds().Max.Y; y++ {
			for x := shot.Bounds().Min.X; x < shot.Bounds().Max.X; x++ {
				r, g, b, a := shot.At(x, y).RGBA()
				if r > 0xd000 && g < 0x5000 && b > 0x4000 && b < 0x9000 && a > 0xf000 {
					colored++
				}
			}
		}
		t.Logf("native popup %v drew %d icon-colored pixels", shot.Bounds(), colored)
		if colored < 100 {
			t.Fatalf("native popup drew %d icon-colored pixels, want at least 100", colored)
		}
	})
}

func menuItemIcons(t *testing.T) {
	loadAppKit()
	defer clearMenuIconCache()

	png16 := testIconPNG(t, 16, 16, color.NRGBA{R: 0x20, G: 0x80, B: 0xf0, A: 0xff})
	png64 := testIconPNG(t, 64, 32, color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff})
	monoSVG, err := driver.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	colorSVG, err := driver.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8" fill="#ff3300"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	pix := image.NewRGBA(image.Rect(0, 0, 16, 16))
	pix.SetRGBA(8, 8, color.RGBA{R: 0x10, G: 0xc0, B: 0x40, A: 0xff})

	s := &surface{}
	s.ensureMenuTarget()
	defer delete(menuTargets, s.menuTarget)

	checkImage := func(name string, it objc.ID, wantHas, wantTemplate bool, wantReps int) objc.ID {
		t.Helper()
		img := it.Send(sel("image"))
		if !wantHas {
			if img != 0 {
				t.Errorf("%s: got image %v, want 0", name, img)
			}
			return 0
		}
		if img == 0 {
			t.Fatalf("%s: image is 0", name)
		}
		if objc.Send[bool](it, sel("respondsToSelector:"), sel("preferredImageVisibility")) {
			if got := objc.Send[int](it, sel("preferredImageVisibility")); got != 1 {
				t.Errorf("%s: preferred image visibility = %d, want visible (1)", name, got)
			}
		}
		sz := objc.Send[apple.CGSize](img, sel("size"))
		if sz.Width != menuIconPoints || sz.Height != menuIconPoints {
			t.Errorf("%s: image size = %v, want %dx%d", name, sz, menuIconPoints, menuIconPoints)
		}
		if got := objc.Send[bool](img, sel("isTemplate")); got != wantTemplate {
			t.Errorf("%s: isTemplate = %v, want %v", name, got, wantTemplate)
		}
		if wantReps > 0 {
			reps := img.Send(sel("representations"))
			if n := objc.Send[int](reps, sel("count")); n != wantReps {
				t.Errorf("%s: representation count = %d, want %d", name, n, wantReps)
			}
		}
		return img
	}

	bar := s.submenu("", []driver.MenuEntry{
		{Label: "File", Items: []driver.MenuEntry{
			{Label: "Color PNG", ID: 1, Icon: driver.MenuIcon{Data: png16}},
			{Label: "Template PNG", ID: 2, Icon: driver.MenuIcon{Data: png64, Template: true}},
			{Label: "Mono SVG", ID: 3, Icon: driver.MenuIcon{SVG: monoSVG}},
			{Label: "Color SVG", ID: 4, Icon: driver.MenuIcon{SVG: colorSVG}},
			{Label: "Pixels", ID: 5, Icon: driver.MenuIcon{Source: driver.ImageSource{Pixels: pix, Version: 1}}},
			{Label: "Sub", Icon: driver.MenuIcon{SVG: monoSVG}, Items: []driver.MenuEntry{{Label: "Inside", ID: 6}}},
			{Label: "Plain", ID: 7},
			{Label: "Invalid", ID: 8, Icon: driver.MenuIcon{Data: []byte("not an image")}},
			{Label: "Remote", ID: 9, Icon: driver.MenuIcon{Source: driver.ImageSource{URI: "https://example.invalid/icon.png"}}},
		}},
	})
	defer bar.Send(selRelease)

	fileSub := bar.Send(selItemAtIndex, 0).Send(sel("submenu"))
	imgPNG := checkImage("Color PNG", fileSub.Send(selItemAtIndex, 0), true, false, 1)
	checkImage("Template PNG", fileSub.Send(selItemAtIndex, 1), true, true, 1)
	imgMono := checkImage("Mono SVG", fileSub.Send(selItemAtIndex, 2), true, true, 2)
	checkImage("Color SVG", fileSub.Send(selItemAtIndex, 3), true, false, 2)
	checkImage("Pixels", fileSub.Send(selItemAtIndex, 4), true, false, 1)
	checkImage("Submenu holder", fileSub.Send(selItemAtIndex, 5), true, true, 2)
	checkImage("Plain", fileSub.Send(selItemAtIndex, 6), false, false, 0)
	checkImage("Invalid", fileSub.Send(selItemAtIndex, 7), false, false, 0)
	checkImage("Remote", fileSub.Send(selItemAtIndex, 8), false, false, 0)

	// In-place sync updates changed icons, clears removed icons, and adds new icons without replacing items.
	firstItem := fileSub.Send(selItemAtIndex, 0)
	plainItem := fileSub.Send(selItemAtIndex, 6)
	next := s.submenu("", []driver.MenuEntry{
		{Label: "File", Items: []driver.MenuEntry{
			{Label: "Color PNG", ID: 1, Icon: driver.MenuIcon{SVG: monoSVG}}, // changed
			{Label: "Template PNG", ID: 2},                                   // removed
			{Label: "Mono SVG", ID: 3, Icon: driver.MenuIcon{SVG: monoSVG}},  // unchanged
			{Label: "Color SVG", ID: 4, Icon: driver.MenuIcon{SVG: colorSVG}},
			{Label: "Pixels", ID: 5, Icon: driver.MenuIcon{Source: driver.ImageSource{Pixels: pix, Version: 1}}},
			{Label: "Sub", Icon: driver.MenuIcon{SVG: monoSVG}, Items: []driver.MenuEntry{{Label: "Inside", ID: 6}}},
			{Label: "Plain", ID: 7, Icon: driver.MenuIcon{Data: png16}}, // added
			{Label: "Invalid", ID: 8},
			{Label: "Remote", ID: 9},
		}},
	})
	syncMenu(bar, next)
	next.Send(selRelease)

	if fileSub.Send(selItemAtIndex, 0) != firstItem || fileSub.Send(selItemAtIndex, 6) != plainItem {
		t.Fatal("syncMenu replaced existing NSMenuItems")
	}
	if got := firstItem.Send(sel("image")); got != imgMono {
		t.Errorf("synced changed icon = %v, want cached mono SVG %v", got, imgMono)
	}
	if got := fileSub.Send(selItemAtIndex, 1).Send(sel("image")); got != 0 {
		t.Errorf("synced removed icon = %v, want 0", got)
	}
	if got := plainItem.Send(sel("image")); got != imgPNG {
		t.Errorf("synced added icon = %v, want cached PNG %v", got, imgPNG)
	}
	checkImage("synced added icon", plainItem, true, false, 1)
	if objc.Send[bool](firstItem, sel("respondsToSelector:"), sel("preferredImageVisibility")) {
		if got := objc.Send[int](fileSub.Send(selItemAtIndex, 1), sel("preferredImageVisibility")); got != 0 {
			t.Errorf("removed icon visibility = %d, want automatic (0)", got)
		}
	}

	// Dock menu items and submenus also receive icons.
	s.setDockMenu([]driver.MenuEntry{
		{Label: "New", ID: 10, Icon: driver.MenuIcon{SVG: monoSVG}},
		{Label: "More", Icon: driver.MenuIcon{Data: png16}, Items: []driver.MenuEntry{{Label: "Inner", ID: 11}}},
	})
	defer clearDockMenu()
	checkImage("Dock New", dockMenu.menu.Send(selItemAtIndex, 0), true, true, 2)
	checkImage("Dock More", dockMenu.menu.Send(selItemAtIndex, 1), true, false, 1)
}
