//go:build windows

package windows

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestDrawnTrayTracksItsView reads the native HICON made for a retained view,
// changes that view, updates metadata, re-adds only this app's icon as
// TaskbarCreated does, and removes it. Shell registration is checked separately;
// no process restarts Explorer.
func TestDrawnTrayTracksItsView(t *testing.T) {
	if testing.Short() {
		t.Skip("registers a native notification icon")
	}
	type controls struct {
		cx     *core.Scope
		change func()
		tray   *core.Tray
	}
	ready := make(chan controls, 1)
	red := nativeTrayPNG(t, color.NRGBA{R: 255, A: 255})
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		var change func()
		tray := cx.NewTray(core.TrayOptions{Icon: red, Tooltip: "NSGo 1382",
			View: func(cx *core.Scope) core.Node {
				shade := core.NewSignal(cx, core.RGB(0x00ff00))
				change = func() { shade.Set(core.RGB(0x0000ff)) }
				return core.View{StyleFunc: func() core.Style {
					return core.Style{
						Width: core.Pt(22), Height: core.Pt(22), CornerRadius: 11, Background: shade.Get(),
					}
				}}
			}})
		ready <- controls{cx, func() { change() }, tray}
	})
	hwnd := startAppWith(t, app, Options{Title: "nsgo-tray1382", Width: 100, Height: 100, Hidden: true})
	ctl := <-ready
	id := waitNativeTrayColor(t, hwnd, color.NRGBA{G: 255, A: 255})
	var original, changed uintptr
	onMain(t, hwnd, func(w *window) {
		original = w.trays[id].icon
		img := iconImage(original)
		if img == nil || img.NRGBAAt(0, 0).A > 32 {
			t.Error("the drawn icon lost its transparent corners")
		}
		for _, child := range windows {
			if child.hostWindow == w && child.kind == driver.SurfaceTrayIcon {
				visible, _, _ := isWindowVisible.Call(child.hwnd)
				style, _, _ := getWindowLongPtrW.Call(child.hwnd, gwlStyle)
				if visible != 0 || style&wsVisible != 0 || child.swap != nil {
					t.Error("the notification icon created a visible window or swap chain")
				}
			}
		}
	})
	ctl.cx.Post(ctl.change)
	waitNativeTrayColor(t, hwnd, color.NRGBA{B: 255, A: 255})
	onMain(t, hwnd, func(w *window) {
		changed = w.trays[id].icon
		if changed == original || nativeTrayIconAlive(original) {
			t.Error("the old drawn HICON was not replaced and released")
		}
	})
	ctl.cx.Post(func() { ctl.tray.SetTooltip("NSGo 1382 updated") })
	nativeTrayWait(t, "updated tooltip with the drawn icon preserved", func() bool {
		var ok bool
		onMain(t, hwnd, func(w *window) {
			tr := w.trays[id]
			ok = tr.cfg.Tooltip == "NSGo 1382 updated" && tr.icon == changed
		})
		return ok
	})
	onMain(t, hwnd, func(w *window) {
		// Remove just our shell registration, then use the production
		// Explorer-restart path to re-add its cached drawn icon.
		data := w.trayData(w.trays[id].cfg, changed)
		data.Size = uint32(unsafe.Sizeof(data))
		shellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
		w.readdTrays()
		tr := w.trays[id]
		if tr.icon != changed || !nativeTrayIconAlive(changed) {
			t.Error("re-adding a drawn icon discarded its view's HICON")
		}
	})
	waitNativeTrayColor(t, hwnd, color.NRGBA{B: 255, A: 255})
	t.Run("ShellRegistration", func(t *testing.T) {
		var added, located bool
		onMain(t, hwnd, func(w *window) {
			added = w.trays[id].added
			_, located = w.trayRect(id)
		})
		if !added {
			t.Skip("this Windows desktop has no notification-area host; bitmap/HICON checks still run")
		}
		if !located {
			t.Skip("the shell accepted the icon but exposes no notification rectangle in this session")
		}
	})
	ctl.cx.Post(ctl.tray.Remove)
	nativeTrayWait(t, "removal of the icon and its attached surface", func() bool {
		var gone bool
		onMain(t, hwnd, func(w *window) {
			gone = w.trays[id] == nil
			for _, child := range windows {
				if child.hostWindow == w && child.kind == driver.SurfaceTrayIcon {
					gone = false
				}
			}
		})
		return gone
	})
	if nativeTrayIconAlive(changed) {
		t.Error("removing the tray retained its private HICON")
	}
}

func TestDrawnTrayRefreshesAfterImageLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("registers a native notification icon")
	}
	started, release := make(chan struct{}, 1), make(chan struct{})
	releaseImage := sync.OnceFunc(func() { close(release) })
	green := nativeTrayPNG(t, color.NRGBA{G: 255, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(green)
	}))
	defer server.Close()
	defer releaseImage()
	red := nativeTrayPNG(t, color.NRGBA{R: 255, A: 255})
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		cx.NewTray(core.TrayOptions{Icon: red, View: func(*core.Scope) core.Node {
			return core.Image{Source: driver.ImageSource{URI: server.URL}, Style: core.Style{Width: core.Pt(22), Height: core.Pt(22)}}
		}})
	})
	hwnd := startAppWith(t, app, Options{Title: "nsgo-tray1382-image", Width: 100, Height: 100, Hidden: true})
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the tray's image did not start loading")
	}
	// Release the image after its first incomplete frame, not as part of an
	// engine commit. The native image completion must schedule the redraw.
	nativeTrayWait(t, "the first transparent image-loading frame", func() bool {
		var blank bool
		onMain(t, hwnd, func(w *window) {
			for _, tr := range w.trays {
				blank = blank || tr.icon != 0 && nativeTrayIconAlive(tr.icon) && iconImage(tr.icon) == nil
			}
		})
		return blank
	})
	releaseImage()
	waitNativeTrayColor(t, hwnd, color.NRGBA{G: 255, A: 255})
}

func nativeTrayPNG(t *testing.T, shade color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.SetNRGBA(x, y, shade)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func waitNativeTrayColor(t *testing.T, hwnd uintptr, want color.NRGBA) uint32 {
	t.Helper()
	var id uint32
	var last color.NRGBA
	nativeTrayWait(t, "the shell icon's pixels", func() bool {
		var matched bool
		onMain(t, hwnd, func(w *window) {
			for _, tr := range w.trays {
				img := iconImage(tr.icon)
				if img == nil {
					continue
				}
				last = img.NRGBAAt(img.Bounds().Dx()/2, img.Bounds().Dy()/2)
				if last == want {
					id, matched = tr.cfg.ID, true
				}
			}
		})
		return matched
	}, func() string { return fmt.Sprintf("last icon pixel: %v", last) })
	return id
}

func nativeTrayIconAlive(icon uintptr) bool {
	var info iconInfo
	ok, _, _ := getIconInfo.Call(icon, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return false
	}
	deleteObject.Call(info.Mask)
	deleteObject.Call(info.Color)
	return true
}

func nativeTrayWait(t *testing.T, what string, ready func() bool, details ...func() string) {
	t.Helper()
	for until := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if ready() {
			return
		}
		if time.Now().After(until) {
			if len(details) > 0 {
				t.Fatal(what, ": ", details[0]())
			}
			t.Fatal(what)
		}
	}
}

// This GDI-only check opens no window or notification icon.
func TestTrayPNGPreservesAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{})
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	icon := iconFromPNG(data.Bytes())
	if icon == 0 {
		t.Fatal("PNG did not create an HICON")
	}
	defer destroyIcon.Call(icon)
	got := iconImage(icon)
	if got == nil || got.NRGBAAt(16, 16) != (color.NRGBA{B: 255, A: 255}) || got.NRGBAAt(0, 0).A != 0 {
		t.Fatal("PNG pixels or transparency changed when converted to an HICON")
	}
}
