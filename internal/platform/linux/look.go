//go:build linux && !android

package linux

import (
	"sync"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// GTK 4 draws a window with an alpha channel wherever the display composites, an RGBA visual on X11 and any surface on Wayland, so a transparent window is an undecorated one whose theme background is gone: it loses the style class that paints it, and a style of NSGo's own clears it and the shadow GTK draws; its surface also leaves out the opaque fill behind the root (widget.go). NoShadow clears the shadow GTK draws around its own decorations; a window manager's, as on X11 with server-side ones, stays, as does the window manager's frame: NoShadow never trades it for GTK's client title bar. Without a compositor, as on X11 without a compositing manager, there is no alpha to show through and the window shows black where the app draws nothing, which the environment says (CanMakeWindowsTransparent). A transparent panel (SurfaceConfig.Transparent), whose content draws its own corners and shadow, is cleared the same way only where the display composites, keeping its background elsewhere; the environment's WindowTransparent says whether a window or panel shows through now. GTK 4 keeps a window off the task list only on X11, through its GdkX11Surface's hint, and Wayland has no such hint, which the environment says too (CanSkipTaskbar).

// lookCSS clears the theme's background, border and shadow from windows of the classes NSGo gives them, and from a transparent one's box.
const lookCSS = `
window.nsgo-transparent, window.nsgo-transparent > box { background: none; border: none; box-shadow: none; }
window.nsgo-no-shadow, window.nsgo-no-shadow.csd { box-shadow: none; }
`

var (
	gtkWidgetAddCSSClass               func(widget uintptr, class *byte)
	gtkWidgetRemoveCSSClass            func(widget uintptr, class *byte)
	gdkDisplayIsComposited             func(display uintptr) int32
	gdkDisplayIsRGBA                   func(display uintptr) int32
	gdkX11SurfaceSetSkipTaskbarHint    func(surface uintptr, skip int32)
	gdkX11SurfaceSetSkipTaskbarHintSet bool
	// bindLook binds what windows' looks need, once.
	bindLook = sync.OnceFunc(func() {
		purego.RegisterLibFunc(&gtkWidgetAddCSSClass, gtkLib, "gtk_widget_add_css_class")
		purego.RegisterLibFunc(&gtkWidgetRemoveCSSClass, gtkLib, "gtk_widget_remove_css_class")
		purego.RegisterLibFunc(&gdkDisplayIsComposited, gtkLib, "gdk_display_is_composited")
		purego.RegisterLibFunc(&gdkDisplayIsRGBA, gtkLib, "gdk_display_is_rgba")
		if _, err := purego.Dlsym(gtkLib, "gdk_x11_surface_set_skip_taskbar_hint"); err == nil {
			purego.RegisterLibFunc(&gdkX11SurfaceSetSkipTaskbarHint, gtkLib, "gdk_x11_surface_set_skip_taskbar_hint")
			gdkX11SurfaceSetSkipTaskbarHintSet = true
		}
	})
	// addLookCSS gives the display lookCSS, above the theme, once a window needs it.
	addLookCSS = sync.OnceFunc(func() {
		provider := gtkCssProviderNew()
		gtkCssProviderLoadFromString(provider, cstr(lookCSS))
		// GTK_STYLE_PROVIDER_PRIORITY_APPLICATION.
		gtkStyleContextAddForDisplay(gdkDisplayGetDefault(), provider, 600)
	})
)

// ownWindow reports whether the surface shows in a top-level window of its own, which turns transparent and leaves the task list: not a view a host embeds, the app's own surface (Options.Hidden), a panel or a tray icon's.
func (w *window) ownWindow() bool {
	return w.win != 0 && !w.embedded && !(w.primary && w.opts.Hidden) && w.kind == driver.SurfaceWindow
}

// displayComposites reports whether the display shows a window's alpha over what is behind it: a compositor with an RGBA visual, as every Wayland one and an X11 compositing manager are.
func displayComposites() bool {
	bindLook()
	display := gdkDisplayGetDefault()
	return display != 0 && gdkDisplayIsComposited(display) != 0 && gdkDisplayIsRGBA(display) != 0
}

// skipTaskbarHint reports whether GDK has the hint that keeps a window off the task list: only on X11, Xwayland included.
func skipTaskbarHint() bool {
	if bindLook(); !gdkX11SurfaceSetSkipTaskbarHintSet {
		return false
	}
	if bindX11Panels(); !x11PanelsReady {
		return false
	}
	display := gdkDisplayGetDefault()
	return display != 0 && gTypeIsA(display, gdkX11DisplayGetType()) != 0
}

// applyLook opens the window as opts ask: transparent, without a shadow, off the task list.
func (w *window) applyLook(opts Options) {
	if opts.Transparent {
		w.setTransparent(true)
	}
	if opts.NoShadow {
		w.setShadow(false)
	}
	if opts.SkipTaskbar {
		w.setSkipTaskbar(true)
	}
}

// setCSSClass gives the window class, or takes it away.
func (w *window) setCSSClass(class string, on bool) {
	bindLook()
	addLookCSS()
	if on {
		gtkWidgetAddCSSClass(w.win, cstr(class))
	} else {
		gtkWidgetRemoveCSSClass(w.win, cstr(class))
	}
}

// setTransparent makes the window undecorated and clear wherever the app draws nothing, or gives it back its theme's background and, with the default title bar, its decorations.
func (w *window) setTransparent(on bool) {
	if w.win == 0 {
		return
	}
	w.transparent = on
	w.setCSSClass("background", !on)
	w.setCSSClass("nsgo-transparent", on)
	gtkWindowSetDecorated(w.win, boolInt(!on && w.chrome.titleBar == driver.TitleBarDefault))
	w.showMenuBar()
	w.updateEnvironment()
}

// setShadow keeps the shadow GTK draws around the window, or clears it.
func (w *window) setShadow(on bool) {
	if w.win != 0 {
		w.setCSSClass("nsgo-no-shadow", !on)
	}
}

// setSkipTaskbar keeps the window off the X11 task list, or puts it back; elsewhere it does nothing, as CanSkipTaskbar says.
func (w *window) setSkipTaskbar(on bool) {
	if w.win == 0 || !skipTaskbarHint() {
		return
	}
	// The hint is the GdkSurface's, which the window has once realized.
	gtkWidgetRealize(w.win)
	if surface := gtkNativeGetSurface(w.win); surface != 0 {
		gdkX11SurfaceSetSkipTaskbarHint(surface, boolInt(on))
	}
}
