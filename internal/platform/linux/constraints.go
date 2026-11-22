//go:build linux && !android

package linux

import (
	"math"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type windowLimits struct {
	header, native   uintptr
	watched, maximum bool
	rewatchState     bool
}

var (
	gtkHeaderBarNew                          func() uintptr
	gtkHeaderBarSetDecorationLayout          func(uintptr, *byte)
	gtkWindowSetTitlebar                     func(uintptr, uintptr)
	gtkWidgetGetRealized                     func(uintptr) int32
	gtkWidgetUnrealize, gtkWidgetMap         func(uintptr)
	gtkWidgetActionSetEnabled                func(uintptr, *byte, int32)
	gdkSurfaceGetWidth, gdkSurfaceGetHeight  func(uintptr) int32
	gdkSurfaceGetFrameClock                  func(uintptr) uintptr
	gtkWindowGetModal, gtkWindowGetResizable func(uintptr) int32
	gtkWindowGetTransientFor                 func(uintptr) uintptr
	gdkToplevelGetCapabilities               func(uintptr) uint32 // GTK 4.20+, optional
	bindWindowLimits                         = sync.OnceFunc(func() {
		purego.RegisterLibFunc(&gtkHeaderBarNew, gtkLib, "gtk_header_bar_new")
		purego.RegisterLibFunc(&gtkHeaderBarSetDecorationLayout, gtkLib, "gtk_header_bar_set_decoration_layout")
		purego.RegisterLibFunc(&gtkWindowSetTitlebar, gtkLib, "gtk_window_set_titlebar")
		purego.RegisterLibFunc(&gtkWidgetGetRealized, gtkLib, "gtk_widget_get_realized")
		purego.RegisterLibFunc(&gtkWidgetUnrealize, gtkLib, "gtk_widget_unrealize")
		purego.RegisterLibFunc(&gtkWidgetMap, gtkLib, "gtk_widget_map")
		purego.RegisterLibFunc(&gtkWidgetActionSetEnabled, gtkLib, "gtk_widget_action_set_enabled")
		purego.RegisterLibFunc(&gtkWindowGetModal, gtkLib, "gtk_window_get_modal")
		purego.RegisterLibFunc(&gtkWindowGetResizable, gtkLib, "gtk_window_get_resizable")
		purego.RegisterLibFunc(&gtkWindowGetTransientFor, gtkLib, "gtk_window_get_transient_for")
		if symbol, err := purego.Dlsym(gtkLib, "gdk_toplevel_get_capabilities"); err == nil {
			purego.RegisterFunc(&gdkToplevelGetCapabilities, symbol)
		}
		purego.RegisterLibFunc(&gdkSurfaceGetWidth, gtkLib, "gdk_surface_get_width")
		purego.RegisterLibFunc(&gdkSurfaceGetHeight, gtkLib, "gdk_surface_get_height")
		purego.RegisterLibFunc(&gdkSurfaceGetFrameClock, gtkLib, "gdk_surface_get_frame_clock")
	})
	cbLimitsMapped            uintptr
	cbLimitsLayout            uintptr
	cbDecorationLayoutChanged uintptr
)

func init() {
	cbLimitsMapped = newCallback(limitsMapped)
	cbLimitsLayout = newCallback(limitsLayout)
	cbDecorationLayoutChanged = newCallback(decorationLayoutChanged)
}

// decorationLayout keeps the session's button placement while removing controls the window does not allow.
func decorationLayout(layout string, c driver.WindowConstraints) string {
	sides := strings.Split(layout, ":")
	for i, side := range sides {
		var keep []string
		for _, button := range strings.Split(side, ",") {
			switch strings.TrimSpace(button) {
			case "minimize":
				if c.NoMinimize {
					continue
				}
			case "maximize":
				if c.NoMaximize || c.NoResize {
					continue
				}
			}
			keep = append(keep, button)
		}
		sides[i] = strings.Join(keep, ",")
	}
	return strings.Join(sides, ":")
}

func (w *window) updateWindowLimits() {
	bindWindowLimits()
	if !w.limits.watched {
		w.limits.watched = true
		gSignalConnect(w.win, "map", cbLimitsMapped, w.handle)
		if settings := gtkSettingsGetDefault(); settings != 0 {
			w.settingsHandlers = append(w.settingsHandlers, gSignalConnect(settings, "notify::gtk-decoration-layout", cbDecorationLayoutChanged, w.handle))
		}
	}
	c := w.chrome.constraints
	if w.limits.header == 0 && (c.NoMinimize || c.NoMaximize || c.NoResize) {
		w.limits.header = gtkHeaderBarNew()
		// GTK requires changing the titlebar while unrealized. Keeping this header afterward avoids further remaps when flags change.
		mapped := gtkWidgetGetMapped(w.win) != 0
		if gtkWidgetGetRealized(w.win) != 0 {
			w.limits.rewatchState = w.watchedRoot != 0
			w.limits.native = 0
			gtkWidgetUnrealize(w.win)
		}
		gtkWindowSetTitlebar(w.win, w.limits.header)
		if mapped {
			gtkWidgetMap(w.win)
		}
	}
	w.updateDecorationLayout()
	w.applyMaximumHints()
}

func (w *window) updateDecorationLayout() {
	c := w.chrome.constraints
	sovereign := gtkWindowGetModal(w.win) == 0 && gtkWindowGetTransientFor(w.win) == 0
	canMinimize, canMaximize := true, true
	if native := gtkNativeGetSurface(w.win); native != 0 {
		if w.limits.rewatchState {
			// The GtkWindow survives the titlebar change, but its old GdkSurface and the state signal watched by mappedCallback do not.
			gSignalConnectObj(native, cstr("notify::state"), cbActivity, w.surface, 0)
			w.limits.rewatchState = false
		}
		if native != w.limits.native {
			w.limits.native = native
			gSignalConnect(native, "notify::state", cbDecorationLayoutChanged, w.handle)
			// GDK rewrites WM_NORMAL_HINTS from GTK's geometry, without a maximum, when its frame clock lays the surface out after a size request or resizability change, which is after setConstraints returns. The surface connected to its clock's layout signal when it was created, so this handler runs after that write.
			if clock := gdkSurfaceGetFrameClock(native); clock != 0 {
				gSignalConnect(clock, "layout", cbLimitsLayout, w.handle)
			}
			if gdkToplevelGetCapabilities != nil {
				gSignalConnect(native, "notify::capabilities", cbDecorationLayoutChanged, w.handle)
			}
		}
		if gdkToplevelGetCapabilities != nil {
			capabilities := gdkToplevelGetCapabilities(native)
			canMinimize, canMaximize = capabilities&(1<<6) != 0, capabilities&(1<<4) != 0
		}
	}
	gtkWidgetActionSetEnabled(w.win, cstr("window.minimize"), boolInt(sovereign && canMinimize && !c.NoMinimize))
	gtkWidgetActionSetEnabled(w.win, cstr("window.toggle-maximized"), boolInt(sovereign && canMaximize && gtkWindowGetResizable(w.win) != 0 && !c.NoMaximize && !c.NoResize))
	if w.limits.header == 0 {
		return
	}
	layout := ":minimize,maximize,close"
	if settings := gtkSettingsGetDefault(); settings != 0 {
		var text uintptr
		gObjectGet(settings, cstr("gtk-decoration-layout"), &text, uintptr(0))
		if text != 0 {
			layout = goString(text)
			gFree(text)
		}
	}
	gtkHeaderBarSetDecorationLayout(w.limits.header, cstr(decorationLayout(layout, w.chrome.constraints)))
}

func limitsMapped(_, data uintptr) {
	if w := windowOf(data); w != nil && !w.closing {
		w.updateDecorationLayout()
		w.applyMaximumHints()
	}
}

// limitsLayout puts back the maximum that GDK's layout dropped from WM_NORMAL_HINTS.
func limitsLayout(_, data uintptr) {
	if w := windowOf(data); w != nil && !w.closing {
		if c := w.chrome.constraints; c.MaxWidth > 0 || c.MaxHeight > 0 {
			w.applyMaximumHints()
		}
	}
}

func decorationLayoutChanged(_, _, data uintptr) {
	if w := windowOf(data); w != nil && !w.closing {
		w.updateDecorationLayout()
	}
}

// x11SizeHints has Xlib's XSizeHints layout on Linux's 64-bit targets.
type x11SizeHints struct {
	flags                                          int
	x, y, width, height                            int32
	minWidth, minHeight, maxWidth, maxHeight       int32
	widthInc, heightInc                            int32
	minAspectX, minAspectY, maxAspectX, maxAspectY int32
	baseWidth, baseHeight, winGravity              int32
}

var (
	x11GetWMNormalHints func(uintptr, uintptr, *x11SizeHints, *int) int32
	x11SetWMNormalHints func(uintptr, uintptr, *x11SizeHints)
	bindX11SizeHints    = sync.OnceValue(func() bool {
		lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return false
		}
		purego.RegisterLibFunc(&x11GetWMNormalHints, lib, "XGetWMNormalHints")
		purego.RegisterLibFunc(&x11SetWMNormalHints, lib, "XSetWMNormalHints")
		return true
	})
)

// applyMaximumHints augments GTK's minimum and resize hints with the app's nonfixed maximum. X11 sizes include client decorations and use physical pixels; NSGo's constraints name the content in logical points.
func (w *window) applyMaximumHints() {
	if w.win == 0 || gtkWidgetGetMapped(w.win) == 0 || !bindX11SizeHints() {
		return
	}
	display, xid, ok := w.x11Window()
	if !ok {
		return
	}
	c := w.chrome.constraints
	if c.NoResize || c.MaxWidth > 0 && c.MaxWidth == c.MinWidth && c.MaxHeight > 0 && c.MaxHeight == c.MinHeight {
		return
	}
	var hints x11SizeHints
	var supplied int
	if x11GetWMNormalHints(display, xid, &hints, &supplied) == 0 {
		return
	}
	before := hints
	const maximum = 1 << 5 // PMaxSize
	if (c.MaxWidth > 0 || c.MaxHeight > 0) && gtkWindowIsFullscreen(w.win) == 0 {
		surface := gtkNativeGetSurface(w.win)
		scale := max(gtkWidgetGetScaleFactor(w.win), 1)
		width := max(gdkSurfaceGetWidth(surface)-gtkWidgetGetWidth(w.surface), 0)
		height := max(gdkSurfaceGetHeight(surface)-gtkWidgetGetHeight(w.surface), 0)
		pixels := func(limit driver.Points, decoration int32) int32 {
			if limit <= 0 {
				return math.MaxInt32
			}
			return int32(min(math.Floor((float64(limit)+float64(decoration))*float64(scale)), math.MaxInt32))
		}
		hints.flags |= maximum
		hints.maxWidth, hints.maxHeight = pixels(c.MaxWidth, width), pixels(c.MaxHeight, height)
		w.limits.maximum = true
	} else if w.limits.maximum {
		hints.flags &^= maximum
		w.limits.maximum = false
	}
	if hints != before {
		x11SetWMNormalHints(display, xid, &hints)
		x11Flush(display)
	}
}
