//go:build linux && !android

package linux

import (
	"math"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// GTK 4 leaves where a toplevel goes to the window manager, and on Wayland to the compositor, which no client can ask for a position. On X11 a panel is placed through its X window with Xlib, as global shortcuts are grabbed, and one that never takes the focus is left unmanaged (override-redirect), as menus and tooltips are: no window manager decorates, places, lists or focuses it, so the app the user types in keeps the keyboard.
var (
	gtkWidgetRealize          func(widget uintptr)
	gdkX11DisplayGetType      func() uintptr
	gdkX11PanelXDisplay       func(display uintptr) uintptr
	gdkX11SurfaceGetXID       func(surface uintptr) uintptr
	x11MoveWindow             func(display, window uintptr, x, y int32) int32
	x11ChangeWindowAttributes func(display, window, mask uintptr, attributes *x11WindowAttributes) int32
	x11Flush                  func(display uintptr) int32
	x11PanelsReady            bool
	bindX11Panels             = sync.OnceFunc(func() {
		// GDK's display, not the environment, says whether it draws on X11: an app run with GDK_BACKEND=x11 in a Wayland session has WAYLAND_DISPLAY set and still draws on Xwayland, where its panels can be placed.
		for _, symbol := range []string{"gdk_x11_display_get_type", "gdk_x11_display_get_xdisplay", "gdk_x11_surface_get_xid"} {
			if _, err := purego.Dlsym(gtkLib, symbol); err != nil {
				return
			}
		}
		lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			if lib, err = purego.Dlopen("libX11.so", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
				return
			}
		}
		purego.RegisterLibFunc(&gtkWidgetRealize, gtkLib, "gtk_widget_realize")
		purego.RegisterLibFunc(&gdkX11DisplayGetType, gtkLib, "gdk_x11_display_get_type")
		purego.RegisterLibFunc(&gdkX11PanelXDisplay, gtkLib, "gdk_x11_display_get_xdisplay")
		purego.RegisterLibFunc(&gdkX11SurfaceGetXID, gtkLib, "gdk_x11_surface_get_xid")
		purego.RegisterLibFunc(&x11MoveWindow, lib, "XMoveWindow")
		purego.RegisterLibFunc(&x11ChangeWindowAttributes, lib, "XChangeWindowAttributes")
		purego.RegisterLibFunc(&x11Flush, lib, "XFlush")
		x11PanelsReady = true
	})
)

// x11WindowAttributes is Xlib's XSetWindowAttributes, of which XChangeWindowAttributes reads the fields its mask names.
type x11WindowAttributes struct {
	backgroundPixmap, backgroundPixel, borderPixmap, borderPixel uintptr
	bitGravity, winGravity, backingStore                         int32
	backingPlanes, backingPixel                                  uintptr
	saveUnder                                                    int32
	eventMask, doNotPropagateMask                                int
	overrideRedirect                                             int32
	colormap, cursor                                             uintptr
}

// x11CWOverrideRedirect is XChangeWindowAttributes' mask for overrideRedirect.
const x11CWOverrideRedirect = 1 << 9

// x11Window returns the Xlib display and the X window of the panel's GtkWindow, realizing it, or false where GDK does not draw on X11.
func (w *window) x11Window() (display, xid uintptr, ok bool) {
	if bindX11Panels(); !x11PanelsReady {
		return 0, 0, false
	}
	gdk := gdkDisplayGetDefault()
	if gdk == 0 || gTypeIsA(gdk, gdkX11DisplayGetType()) == 0 {
		return 0, 0, false
	}
	if display = gdkX11PanelXDisplay(gdk); display == 0 {
		return 0, 0, false
	}
	gtkWidgetRealize(w.win)
	surface := gtkNativeGetSurface(w.win)
	if surface == 0 {
		return 0, 0, false
	}
	if xid = gdkX11SurfaceGetXID(surface); xid == 0 {
		return 0, 0, false
	}
	return display, xid, true
}

// leaveUnmanaged makes a panel's X window override-redirect before it first maps, so that no window manager ever manages it; false where GDK does not draw on X11, as on Wayland, whose compositor focuses the panel as it shows.
func (w *window) leaveUnmanaged() bool {
	display, xid, ok := w.x11Window()
	if !ok {
		return false
	}
	x11ChangeWindowAttributes(display, xid, x11CWOverrideRedirect, &x11WindowAttributes{overrideRedirect: 1})
	x11Flush(display)
	return true
}

// showPanel shows a panel beside at on side when side is set (core.Window.ShowAt), presenting it for the keyboard unless it never takes it. Where the panel cannot be placed it stays hidden and the app hears why. An unmanaged panel goes where it is placed before it maps, so it never shows elsewhere first, and to the upper middle of the primary display when unplaced, where no window manager places it.
func (w *window) showPanel(at driver.Bounds, side driver.WindowPlacement) {
	if w.dock.Edge != driver.WindowPlaceDefault {
		w.showDockedPanel()
		return
	}
	w.anchorBounds, w.panelSide = at, side
	if (w.unmanaged || side != driver.WindowPlaceDefault && w.noActivate) && !w.placePanel() {
		w.placementFailed()
		return
	}
	w.visible = true
	gtkWidgetSetVisible(w.win, 1)
	if w.noActivate {
		return
	}
	gtkWindowPresent(w.win)
	// A window manager places a window it manages as it maps, so this one moves once it shows.
	if side != driver.WindowPlaceDefault && !w.placePanel() {
		w.placementFailed()
	}
}

// placePanel moves the panel's X window beside w.anchorBounds on w.panelSide's side, inside the visible frame of the display that holds the rectangle's center, else to the upper middle of the primary display; false where GDK does not draw on X11.
func (w *window) placePanel() bool {
	display, xid, ok := w.x11Window()
	if !ok {
		return false
	}
	all := displays()
	if len(all) == 0 {
		return false
	}
	var width, height int32
	gtkWindowGetDefaultSize(w.win, &width, &height)
	size := driver.Size{Width: driver.Points(width), Height: driver.Points(height)}
	var p driver.Point
	if w.panelSide == driver.WindowPlaceDefault {
		f := all[0].Frame
		p = driver.Point{X: f.Origin.X + (f.Size.Width-size.Width)/2, Y: f.Origin.Y + f.Size.Height/3 - size.Height/2}
	} else {
		at := w.anchorBounds
		center := driver.Point{X: at.Origin.X + at.Size.Width/2, Y: at.Origin.Y + at.Size.Height/2}
		area := all[0].VisibleFrame
		for _, d := range all {
			if d.Frame.Contains(center) {
				area = d.VisibleFrame
				break
			}
		}
		p, _ = driver.PlaceWindow(at, size, area, w.panelSide)
	}
	// GDK's displays and sizes are in its logical pixels, X's in the device pixels of GDK's whole scale.
	scale := float64(max(gtkWidgetGetScaleFactor(w.win), 1))
	x11MoveWindow(display, xid, int32(math.Round(float64(p.X)*scale)), int32(math.Round(float64(p.Y)*scale)))
	x11Flush(display)
	return true
}

// placementFailed hides the panel and tells the app that it could not be placed where it asked.
func (w *window) placementFailed() {
	w.visible = false
	gtkWidgetSetVisible(w.win, 0)
	if w.engine != nil {
		w.engine.Dispatch(driver.Event{Kind: driver.EventWindowPlacementFailed, Text: "Linux places a panel at a screen rectangle on X11 only; a Wayland compositor places its windows itself"})
	}
}

// fitPanel sizes a panel to width by height, the size its content takes (driver.WindowFitSize), rounded up to GTK's whole logical pixels so a size the core rounded to device pixels, a half point at scale 2, loses none of the content: GTK keeps its top left corner, and one that shows beside the rectangle ShowAt placed it at is placed there again at its new size, on X11.
func (w *window) fitPanel(width, height driver.Points) {
	gtkWindowSetDefaultSize(w.win, int32(math.Ceil(float64(width))), int32(math.Ceil(float64(height))))
	if w.visible && w.dock.Edge != driver.WindowPlaceDefault {
		w.placeDockedPanel()
	}
	if w.visible && w.panelSide != driver.WindowPlaceDefault {
		w.placePanel()
	}
}
