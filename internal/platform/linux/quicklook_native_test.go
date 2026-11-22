//go:build linux && !android

package linux

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
)

// TestQuickLookWithSushi runs in a private Xvfb/D-Bus session with Sushi. Its PNGs have disjoint colors: reading the native window's pixels proves the requested file rendered, then changed, and the panel really closed.
func TestQuickLookWithSushi(t *testing.T) {
	if os.Getenv("NSGO_TEST_SUSHI") != "1" {
		t.Skip("needs Sushi in a private X11 and D-Bus session")
	}
	paths := []string{filepath.Join(t.TempDir(), "first #日本.png"), filepath.Join(t.TempDir(), "second.png")}
	for i, path := range paths {
		im := image.NewRGBA(image.Rect(0, 0, 96, 96))
		colors := [2]color.RGBA{{R: 255, A: 255}, {B: 255, A: 255}}
		if i == 1 {
			colors = [2]color.RGBA{{G: 255, A: 255}, {R: 255, G: 255, A: 255}}
		}
		for y := 0; y < 96; y++ {
			for x := 0; x < 96; x++ {
				im.SetRGBA(x, y, colors[x/48])
			}
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, im)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	ready := make(chan *core.Scope, 1)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		ready <- cx
		return core.Text{Content: "NSGo file preview"}
	}, Options{Title: "NSGo preview test", Width: 240, Height: 120})
	cx := <-ready
	w.onLoop(t, func() {
		if !w.environment().CanQuickLook {
			t.Error("the running previewer is unavailable")
		}
	})
	show := func(index int) {
		t.Helper()
		asked := make(chan bool, 1)
		cx.Post(func() { asked <- cx.QuickLook(paths, index) })
		select {
		case ok := <-asked:
			if !ok {
				t.Fatal("core refused the regular preview files")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the engine did not request the preview")
		}
	}
	waitPixels := func(name string, second, closed bool) {
		t.Helper()
		for deadline := time.Now().Add(40 * time.Second); ; time.Sleep(25 * time.Millisecond) {
			im := captureSushi(t)
			var counts [4]int
			if im != nil {
				for y := 0; y < im.Bounds().Dy(); y++ {
					for x := 0; x < im.Bounds().Dx(); x++ {
						switch im.RGBAAt(x, y) {
						case color.RGBA{R: 255, A: 255}:
							counts[0]++
						case color.RGBA{B: 255, A: 255}:
							counts[1]++
						case color.RGBA{G: 255, A: 255}:
							counts[2]++
						case color.RGBA{R: 255, G: 255, A: 255}:
							counts[3]++
						}
					}
				}
			}
			ok := counts[0] >= 1000 && counts[1] >= 1000 && counts[2] < 1000
			if second {
				ok = counts[2] >= 1000 && counts[3] >= 1000 && counts[0] < 1000
			}
			if closed {
				ok = im == nil
			}
			if ok {
				t.Logf("%s native preview pixel counts [red blue green yellow]: %v", name, counts)
				if dir := os.Getenv("NSGO_PREVIEW_SCREENSHOTS"); dir != "" && im != nil {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					f, err := os.Create(filepath.Join(dir, name+".png"))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, im)
					f.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s native preview pixel counts = %v, closed=%v", name, counts, im == nil)
			}
		}
	}
	show(1)
	waitPixels("selected", true, false)
	// Activate Sushi's own keyboard action. It emits SelectionEvent with the service's real wire type, which the backend must attribute.
	bindNotifications()
	v := gVariantTypeNew(cstr("v"))
	dict := gVariantTypeNew(cstr("{sv}"))
	reply, err := dbusTry(sessionBus(), previewName, previewPath+"/window/1", "org.gtk.Actions", "Activate",
		gVariantNewString(cstr("select-left")), gVariantNewArray(v, nil, 0), gVariantNewArray(dict, nil, 0))
	gVariantTypeFree(v)
	gVariantTypeFree(dict)
	gVariantUnrefNonZero(reply)
	if err != nil {
		t.Fatal(err)
	}
	waitPixels("previous", false, false)
	show(1)
	waitPixels("updated", true, false)
	gVariantUnrefNonZero(dbusCall(sessionBus(), previewName, previewPath, previewV2, "Close"))
	waitPixels("closed", false, true)
	show(0)
	waitPixels("reopened", false, false)
	w.onLoop(t, w.closeQuickLook)
	waitPixels("owner-released", false, true)
	show(0)
	waitPixels("before-takeover", false, false)
	// A different app may reuse this process-global service. An unparented ShowFile simulates its request without inventing a foreign window ID.
	previews.Lock()
	protocol := previews.active.protocol
	previews.Unlock()
	args := []uintptr{gVariantNewString(cstr(previewFileURI(paths[1]))),
		gVariantNewString(cstr("")), gVariantNewBoolean(0)}
	if protocol.signature == "ssbs" {
		args = append(args, gVariantNewString(cstr("")))
	}
	gVariantUnrefNonZero(dbusCall(sessionBus(), previewName, previewPath, previewV2, "ShowFile", args...))
	waitPixels("taken-over", true, false)
	w.onLoop(t, w.closeQuickLook)
	waitPixels("other-owner-preserved", true, false)
	gVariantUnrefNonZero(dbusCall(sessionBus(), previewName, previewPath, previewV2, "Close"))
	waitPixels("takeover-closed", false, true)
}

// xPreviewAttributes matches XWindowAttributes on NSGo's 64-bit Linux targets. Captures select Sushi's own visible window by WM_CLASS, never unrelated windows or the whole desktop.
type xPreviewAttributes struct {
	X, Y, Width, Height, Border, Depth          int32
	Visual, Root                                uintptr
	Class, BitGravity, WinGravity, BackingStore int32
	BackingPlanes, BackingPixel                 uintptr
	SaveUnder                                   int32
	Colormap                                    uintptr
	MapInstalled, MapState                      int32
	AllEvents, YourEvents, DoNotPropagate       int64
	OverrideRedirect                            int32
	Screen                                      uintptr
}

func captureSushi(t *testing.T) *image.RGBA {
	t.Helper()
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(lib)
	var open func(*byte) uintptr
	var close func(uintptr) int32
	var root func(uintptr) uintptr
	var tree func(uintptr, uintptr, *uintptr, *uintptr, *uintptr, *uint32) int32
	var attrs func(uintptr, uintptr, *xPreviewAttributes) int32
	var hint func(uintptr, uintptr, *[2]uintptr) int32
	var free func(uintptr) int32
	var grab, ungrab func(uintptr) int32
	var syncDisplay func(uintptr, int32) int32
	var getImage func(uintptr, uintptr, int32, int32, uint32, uint32, uintptr, int32) uintptr
	var pixel func(uintptr, int32, int32) uintptr
	var destroy func(uintptr) int32
	for name, fn := range map[string]any{"XOpenDisplay": &open, "XCloseDisplay": &close, "XDefaultRootWindow": &root,
		"XQueryTree": &tree, "XGetWindowAttributes": &attrs, "XGetClassHint": &hint, "XFree": &free,
		"XGrabServer": &grab, "XUngrabServer": &ungrab, "XSync": &syncDisplay,
		"XGetImage": &getImage, "XGetPixel": &pixel, "XDestroyImage": &destroy} {
		purego.RegisterLibFunc(fn, lib, name)
	}
	d := open(nil)
	if d == 0 {
		t.Fatal("the native preview capture has no X display")
	}
	defer close(d)
	// Hold other clients only while resolving and copying this window, preventing a close between XQueryTree and XGetImage.
	grab(d)
	grabbed := true
	defer func() {
		if grabbed {
			ungrab(d)
			syncDisplay(d, 0)
		}
	}()
	var returnedRoot, parent, children uintptr
	var count uint32
	if tree(d, root(d), &returnedRoot, &parent, &children, &count) == 0 {
		return nil
	}
	defer free(children)
	var native uintptr
	var width, height int32
	for _, window := range unsafe.Slice((*uintptr)(cPointer(children)), int(count)) {
		var h [2]uintptr
		if hint(d, window, &h) == 0 {
			continue
		}
		match := goString(h[0]) == previewName || goString(h[1]) == previewName
		free(h[0])
		free(h[1])
		if !match {
			continue
		}
		var a xPreviewAttributes
		if attrs(d, window, &a) == 0 || a.MapState != 2 || a.Width < 96 || a.Height < 96 {
			continue
		}
		if a.Depth != 24 && a.Depth != 32 {
			t.Fatalf("preview capture needs a true-color X display, depth=%d", a.Depth)
		}
		native = getImage(d, window, 0, 0, uint32(a.Width), uint32(a.Height), ^uintptr(0), 2)
		width, height = a.Width, a.Height
		break
	}
	ungrab(d)
	syncDisplay(d, 0)
	grabbed = false
	if native == 0 {
		return nil
	}
	defer destroy(native)
	im := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	for y := int32(0); y < height; y++ {
		for x := int32(0); x < width; x++ {
			v := pixel(native, x, y)
			im.SetRGBA(int(x), int(y), color.RGBA{R: byte(v >> 16), G: byte(v >> 8), B: byte(v), A: 255})
		}
	}
	return im
}
