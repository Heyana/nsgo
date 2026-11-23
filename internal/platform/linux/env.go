//go:build linux && !android

package linux

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/speechd"
)

// speakAvailable is Environment.Speak, read once: speech-dispatcher listens or is installed.
var speakAvailable = sync.OnceValue(speechd.Available)

const (
	gtkTextDirLTR = 1
	gtkTextDirRTL = 2
	// gtk-xft-dpi is in 1024ths of a point per inch.
	xftDPIUnit = 1024
)

// environment reads what GTK and the session say about where the app
// runs. There is no software keyboard, so its inset stays 0.
func (w *window) environment() driver.Environment {
	dark, reduce, contrast, dpi := false, false, false, int32(96*xftDPIUnit)
	theme := ""
	if settings := gtkSettingsGetDefault(); settings != 0 {
		if colorScheme() {
			dark = settingInt(settings, "gtk-interface-color-scheme") == schemeDark
		} else {
			dark = settingBool(settings, "gtk-application-prefer-dark-theme")
		}
		reduce = !settingBool(settings, "gtk-enable-animations")
		dpi = settingInt(settings, "gtk-xft-dpi")
		theme = settingString(settings, "gtk-theme-name")
		if colorScheme() {
			contrast = settingInt(settings, "gtk-interface-contrast") == contrastMore
		}
	}
	if strings.Contains(strings.ToLower(theme), "dark") || strings.Contains(strings.ToLower(os.Getenv("GTK_THEME")), "dark") {
		dark = true
	}
	// GTK 4.20 and later have the session's contrast as a setting. Before it, GNOME's High Contrast is read where its schema is installed, and other desktops say it with a high-contrast theme.
	contrast = contrast || gnomeHighContrast()
	for _, name := range []string{theme, os.Getenv("GTK_THEME")} {
		name = strings.ToLower(name)
		if strings.Contains(name, "highcontrast") || strings.Contains(name, "high-contrast") {
			contrast = true
		}
	}
	scale := 1.0
	if surface := gtkNativeGetSurface(gtkWidgetGetNative(w.surface)); surface != 0 {
		scale = gdkSurfaceGetScale(surface)
	} else if f := gtkWidgetGetScaleFactor(w.surface); f > 0 {
		scale = float64(f)
	}
	if scale <= 0 {
		scale = 1
	}
	fontScale := float32(1)
	if dpi > 0 {
		fontScale = float32(float64(dpi) / float64(96*xftDPIUnit))
	}
	locale := sessionLocale()
	deviceName, deviceModel := deviceIdentity()
	var frame driver.WindowFrame
	if w.win != 0 {
		var width, height int32
		gtkWindowGetDefaultSize(w.win, &width, &height)
		frame.Width, frame.Height = driver.Points(width), driver.Points(height)
		if position, ok := w.framePosition(); ok {
			frame.X, frame.Y, frame.Move = position.X, position.Y, true
		}
	}
	minimized := false
	if w.surface != 0 {
		if top := gtkNativeGetSurface(gtkWidgetGetNative(w.surface)); top != 0 {
			minimized = gdkToplevelGetState(top)&toplevelMinimized != 0
		}
	}
	front, followed := frontmostApp()
	return driver.Environment{
		LaunchedAtLogin:  w.opts.LaunchedAtLogin,
		DeviceName:       deviceName,
		DeviceModel:      deviceModel,
		Dark:             dark,
		FontScale:        fontScale,
		ReduceMotion:     reduce,
		IncreaseContrast: contrast,
		ScreenColor:      w.screenColorAvailable(),
		RTL:              systemDirection() == gtkTextDirRTL || rightToLeft(locale),
		PixelRatio:       float32(scale),
		Locale:           locale,
		KeyCodeNames:     w.keyboardCodeNames(),
		LocaleSettings:   localeSettings(),
		Displays:         displays(),
		DisplayID:        w.currentDisplayID(),
		OS:               driver.OSLinux,
		CanLockPointer:   w.canLockPointer(),
		HasWebView:       true,
		CanSendInput:     w.canSendWebInput(),
		OSVersion:        osVersion(),
		Idiom:            driver.IdiomDesktop,
		TitleBar:         w.titleBarArea(),
		Maximized:        w.win != 0 && gtkWindowIsMaximized(w.win) != 0,
		FullScreen:       w.chrome.fullScreen,
		Minimized:        minimized,
		WindowFrame:      frame,
		CanDockPanels:    w.canDockPanels(),
		Lifecycle:        w.lifecycle(),
		LowPowerMode:     lowPowerMode(),
		Speak:            speakAvailable(),
		ScanCode:         scanCameraAvailable(),
		Camera:           scanCameraAvailable(),
		Gamepads:         true,
		Audio:            loadSound(),
		DiscoverServices: true,
		AudioSchedule:    loadSound(),
		GPU:              loadGPU(),

		AudioPlaybackControls: loadSound(),
		CanQuickLook:          w.canQuickLook(),

		// org.freedesktop.Notifications has no schedule, so scheduled notifications are kept while the app runs.
		ScheduledNotifications: true,

		// X11 positions windows and asks its window manager for stacking; a Wayland compositor chooses both for an ordinary toplevel.
		CanPlaceWindows:        w.canPlace(),
		CanSetWindowLevel:      w.canSetLevel(),
		CanSetWindowWorkspaces: w.canSetWorkspaces(),

		CanMakeWindowsTransparent: w.ownWindow() && displayComposites(),
		CanSkipTaskbar:            w.ownWindow() && skipTaskbarHint(),
		FrontmostApp:              front,

		// Only an X11 session says which window is in front.
		CanReadFrontmostApp: followed,
		CustomCursor:        true,

		// A window or panel cleared of its background shows through only while the display composites (look.go).
		WindowTransparent: w.transparent && displayComposites(),
	}
}

// deviceIdentity returns the name GNOME's and KDE's settings show, machine-info's PRETTY_HOSTNAME or else the hostname, read on every environment refresh so a rename reaches the app, and the hardware's model.
func deviceIdentity() (name, model string) {
	if data, err := os.ReadFile("/etc/machine-info"); err == nil {
		name = prettyHostname(string(data))
	}
	if name == "" {
		name, _ = os.Hostname()
	}
	return name, hardwareModel()
}

// prettyHostname reads PRETTY_HOSTNAME from machine-info, an environment-like file whose values may be quoted as a shell quotes them.
func prettyHostname(machineInfo string) string {
	for line := range strings.Lines(machineInfo) {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(key) != "PRETTY_HOSTNAME" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			quote := value[0]
			value = value[1 : len(value)-1]
			if quote == '"' {
				value = strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\$`, `$`, "\\`", "`").Replace(value)
			}
		}
		return value
	}
	return ""
}

// hardwareModel is the firmware's product name, or a device tree's model on boards without DMI, such as a Raspberry Pi; empty when neither names one. It never changes while the app runs.
var hardwareModel = sync.OnceValue(func() string {
	if data, err := os.ReadFile("/sys/devices/virtual/dmi/id/product_name"); err == nil {
		if model := firmwareModel(string(data)); model != "" {
			return model
		}
	}
	if data, err := os.ReadFile("/proc/device-tree/model"); err == nil {
		return firmwareModel(string(data))
	}
	return ""
})

// firmwareModel trims a firmware string and drops the placeholders board makers leave in it, which name no model.
func firmwareModel(s string) string {
	s = strings.TrimSpace(strings.TrimRight(s, "\x00"))
	switch strings.ToLower(s) {
	case "to be filled by o.e.m.", "system product name", "default string", "not applicable", "not specified", "none", "unknown", "0123456789":
		return ""
	}
	return s
}

// displays reads GDK's monitors in the same logical coordinate space as
// WindowFrame. GTK exposes no primary-monitor flag, so the first monitor is
// the primary one, matching GTK's ordering.
func displays() []driver.DisplayInfo {
	display := gdkDisplayGetDefault()
	if display == 0 {
		return nil
	}
	model := gdkDisplayGetMonitors(display)
	if model == 0 {
		return nil
	}
	watchDisplays(model)
	count := gListModelGetNItems(model)
	out := make([]driver.DisplayInfo, 0, count)
	for i := uint32(0); i < count; i++ {
		monitor := gListModelGetItem(model, i)
		if monitor == 0 {
			continue
		}
		watchMonitor(monitor)
		var frame gdkMonitorRectangle
		gdkMonitorGetGeometry(monitor, &frame)
		// Wayland reports no work area, so there the whole monitor is the visible one.
		visible := frame
		if gdkX11MonitorGetWorkarea != nil && gTypeIsA(monitor, gdkX11MonitorGetType()) != 0 {
			gdkX11MonitorGetWorkarea(monitor, &visible)
		}
		// The fractional scale, as PixelRatio's: the scale factor is its ceiling.
		var scale float32
		if gdkMonitorGetScale != nil {
			scale = float32(gdkMonitorGetScale(monitor))
		} else {
			scale = float32(gdkMonitorGetScaleFactor(monitor))
		}
		if scale <= 0 {
			scale = 1
		}
		point := func(r gdkMonitorRectangle) driver.Bounds {
			return driver.Bounds{Origin: driver.Point{X: driver.Points(float32(r.X)), Y: driver.Points(float32(r.Y))}, Size: driver.Size{Width: driver.Points(float32(r.Width)), Height: driver.Points(float32(r.Height))}}
		}
		out = append(out, driver.DisplayInfo{ID: monitorDisplayID(monitor), Frame: point(frame), VisibleFrame: point(visible), Scale: scale, Main: i == 0,
			RefreshRate: float64(max(0, gdkMonitorGetRefreshRate(monitor))) / 1000})
		gObjectUnref(monitor)
	}
	return out
}

// The connector names the display across runs. Only a connected monitor
// keeps its object address as a fallback ID.
func monitorDisplayID(monitor uintptr) string {
	if connector := gdkMonitorGetConnector(monitor); connector != 0 {
		if id := goString(connector); id != "" {
			return id
		}
	}
	return fmt.Sprintf("%x", monitor)
}

func (w *window) currentDisplayID() string {
	var surface uintptr
	if !w.closing && w.surface != 0 && !(w.primary && w.opts.Hidden) && w.kind != driver.SurfaceTrayIcon {
		if native := gtkWidgetGetNative(w.surface); native != 0 {
			surface = gtkNativeGetSurface(native)
		}
	}
	w.watchMonitorSurface(surface)
	display := gdkDisplayGetDefault()
	if display == 0 || surface == 0 {
		return ""
	}
	if monitor := gdkDisplayGetMonitorAtSurface(display, surface); monitor != 0 {
		return monitorDisplayID(monitor)
	}
	return ""
}

// Keep the host surface alive until its handlers can be disconnected when the
// view closes or changes hosts.
func (w *window) watchMonitorSurface(surface uintptr) {
	if surface == w.monitorSurface {
		return
	}
	if w.monitorSurface != 0 {
		for _, id := range w.monitorHandlers {
			gSignalHandlerDisconnect(w.monitorSurface, id)
		}
		gObjectUnref(w.monitorSurface)
	}
	w.monitorSurface = surface
	w.monitorHandlers = [2]uint64{}
	if surface != 0 {
		gObjectRef(surface)
		w.monitorHandlers = [2]uint64{
			gSignalConnect(surface, "enter-monitor", cbCurrentDisplayChanged, w.handle),
			gSignalConnect(surface, "leave-monitor", cbCurrentDisplayChanged, w.handle),
		}
	}
}

func currentDisplayChangedCallback(_, _, data uintptr) {
	if w := windowOf(data); w != nil && !w.closing {
		w.updateEnvironment()
	}
}

var watchedDisplays uintptr

func watchDisplays(model uintptr) {
	if watchedDisplays == model {
		return
	}
	watchedDisplays = model
	gSignalConnect(model, "items-changed", cbDisplaysChanged, 0)
}

// watchMonitor rereads the displays when monitor moves, resizes or changes its scale, which GDK reports on the monitor rather than as a change to the model's items. The mark it leaves on the monitor goes with it.
func watchMonitor(monitor uintptr) {
	key := cstr("nsgo-displays-watched")
	if gObjectGetData(monitor, key) != 0 {
		return
	}
	gObjectSetData(monitor, key, 1)
	for _, signal := range []string{"notify::geometry", "notify::scale", "notify::scale-factor", "notify::refresh-rate", "invalidate"} {
		gSignalConnect(monitor, signal, cbDisplaysChanged, 0)
	}
}

func displaysChangedCallback(_, _, _, _ uintptr) {
	windowHandles.Range(func(_, value any) bool {
		if w := value.(*window); !w.closing {
			w.updateEnvironment()
		}
		return true
	})
}

// GdkToplevelState's flags that hide a window: minimized, and suspended
// (on another workspace or covered, where the compositor says).
const toplevelMinimized, toplevelSuspended = 1 << 0, 1 << 16

// lifecycle is whether the app is in use: in the background while the
// surface is unmapped or its window minimized or suspended, active while
// one of the app's windows has the focus.
func (w *window) lifecycle() driver.Lifecycle {
	if w.primary && w.opts.Hidden {
		return appLifecycle()
	}
	if w.surface == 0 || gtkWidgetGetMapped(w.surface) == 0 {
		return driver.LifecycleBackground
	}
	if top := gtkNativeGetSurface(gtkWidgetGetNative(w.surface)); top != 0 &&
		gdkToplevelGetState(top)&(toplevelMinimized|toplevelSuspended) != 0 {
		return driver.LifecycleBackground
	}
	active := false
	windowHandles.Range(func(_, v any) bool {
		if other := v.(*window); !other.closing && windowActive(other.rootWindow()) {
			active = true
		}
		return !active
	})
	if active {
		return driver.LifecycleActive
	}
	return driver.LifecycleInactive
}

// appLifecycle is the lifecycle of the app's own surface, which shows in no window (Options.Hidden): its windows', active while one has the focus and in the background while none shows.
func appLifecycle() driver.Lifecycle {
	l := driver.LifecycleBackground
	windowHandles.Range(func(_, v any) bool {
		w := v.(*window)
		if w.closing || w.primary && w.opts.Hidden {
			return true
		}
		switch w.lifecycle() {
		case driver.LifecycleActive:
			l = driver.LifecycleActive
			return false
		case driver.LifecycleInactive:
			l = driver.LifecycleInactive
		}
		return true
	})
	return l
}

// updateAppSurface reads the environment of the app's own surface again as one of its windows shows, hides or goes.
func updateAppSurface() {
	windowHandles.Range(func(_, v any) bool {
		if w := v.(*window); w.primary && w.opts.Hidden && !w.closing {
			w.updateEnvironment()
		}
		return true
	})
}

// windowActive reports whether win is a shown window with the keyboard focus. GTK keeps a hidden window's is-active set until the window manager moves the focus, which may never happen, so a window that is not mapped is never active.
func windowActive(win uintptr) bool {
	return win != 0 && gtkWidgetGetMapped(win) != 0 && gtkWindowIsActive(win) != 0
}

// rootWindow is the GtkWindow the surface shows in, the host's for an
// embedded one; 0 while it is in none.
func (w *window) rootWindow() uintptr {
	if w.win != 0 {
		return w.win
	}
	if w.surface == 0 {
		return 0
	}
	return gtkWidgetGetRoot(w.surface)
}

// mappedCallback follows the surface's map and unmap, and on the first
// map its window's focus and its toplevel's state, which exist only once
// it is in a window.
func mappedCallback(widget, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	if gtkWidgetGetMapped(w.surface) != 0 && w.watchedRoot == 0 {
		if root := w.rootWindow(); root != 0 {
			w.watchedRoot = root
			// The host's window may outlive this surface. GTK disconnects these broadcasts when either endpoint goes.
			gSignalConnectObj(root, cstr("notify::is-active"), cbActivity, w.surface, 0)
			if top := gtkNativeGetSurface(gtkWidgetGetNative(w.surface)); top != 0 {
				gSignalConnectObj(top, cstr("notify::state"), cbActivity, w.surface, 0)
			}
		}
	}
	if !w.canHoldPointer() {
		w.releasePointer(true)
	}
	w.syncOwnedVisibility()
	w.updateEnvironment()
	if !w.primary {
		updateAppSurface()
	}
}

// activityCallback reads every window's environment again as one's focus
// or toplevel state changes, the app being active while any is.
func activityCallback(object, pspec, data uintptr) {
	windowHandles.Range(func(_, v any) bool {
		if w := v.(*window); !w.closing {
			if lockedPointer.owner == w && !w.canHoldPointer() {
				w.releasePointer(true)
			}
			w.syncOwnedVisibility()
			active := windowActive(w.win)
			if active {
				w.focused = true
			}
			// Keys and mouse buttons held come up in another window once this one is no longer active: an embedded surface's is the host's.
			if !windowActive(w.rootWindow()) {
				w.releaseKeys()
				w.releaseMouseButtons()
			}
			if w.kind == driver.SurfacePanel && w.hideOnBlur && w.visible && w.focused && !active {
				w.visible = false
				gtkWidgetSetVisible(w.win, 0)
				if w.engine != nil {
					w.engine.Dispatch(driver.Event{Kind: driver.EventWindowHidden})
				}
			}
			w.updateEnvironment()
		}
		return true
	})
}

// updateEnvironment reads the environment again and reports it when it
// changed.
func (w *window) updateEnvironment() {
	env := w.environment()
	w.devFrame.record(w, env)
	if w.engine != nil {
		w.reportFrameWindow(env)
	}
	if env.Equal(w.env) {
		return
	}
	redock := w.dock.Edge != driver.WindowPlaceDefault && w.visible && !slices.Equal(env.Displays, w.env.Displays)
	w.env = env
	if w.engine != nil {
		w.engine.SetEnvironment(env)
	}
	if redock {
		w.placeDockedPanel()
	}
}

func (w *window) reportFrameWindow(env driver.Environment) {
	root := w.rootWindow()
	visible := root != 0 && w.surface != 0 && w.kind != driver.SurfaceTrayIcon && gtkWidgetGetMapped(w.surface) != 0 &&
		env.Lifecycle != driver.LifecycleBackground
	w.engine.SetFrameWindow(visible, visible && windowActive(root))
}

// settingBool reads a gboolean property of settings.
func settingBool(settings uintptr, name string) bool {
	var v int32
	gObjectGet(settings, uintptr(unsafe.Pointer(cstr(name))), uintptr(unsafe.Pointer(&v)), 0)
	return v != 0
}

// settingInt reads an int property of settings.
func settingInt(settings uintptr, name string) int32 {
	var v int32
	gObjectGet(settings, uintptr(unsafe.Pointer(cstr(name))), uintptr(unsafe.Pointer(&v)), 0)
	return v
}

// settingString reads a string property of settings; the copy GTK makes
// is freed.
func settingString(settings uintptr, name string) string {
	var p uintptr
	gObjectGet(settings, uintptr(unsafe.Pointer(cstr(name))), uintptr(unsafe.Pointer(&p)), 0)
	s := goString(p)
	if p != 0 {
		gFree(p)
	}
	return s
}

// settingsChangedCallback rereads the environment and the UI font after a GTK setting changed.
func settingsChangedCallback(settings, pspec, data uintptr) {
	if readUIFont() {
		uiFontChanged()
	}
	if w := windowOf(data); w != nil && !w.closing {
		w.updateEnvironment()
	}
}

// uiFontChanged has every app prepare text again in GTK's new UI font, reloads editors' resolved families and redraws canvases.
func uiFontChanged() {
	windowHandles.Range(func(_, v any) bool {
		w := v.(*window)
		if w.closing {
			return true
		}
		if w.engine != nil {
			w.engine.FontsChanged()
		}
		for _, cv := range w.views {
			if cv == nil {
				continue
			}
			// A system family, also one in a fallback list, was resolved to the old UI font in the editor's CSS. Only those editors reload: each provider is the display's, so reloading one restyles every widget. Unset families inherit GTK's own changing default.
			if cv.entry != 0 && namesUIFont(cv.input.Style.Font.Family) {
				w.styleEntry(cv)
			}
			if cv.drawing != nil && cv.widget != 0 {
				gtkWidgetQueueDraw(cv.widget)
			}
		}
		return true
	})
}

// windowStateCallback rereads the environment after the window is
// maximized, restored or made full screen, by the app or the window
// manager.
func windowStateCallback(window, pspec, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	if fullScreen := gtkWindowIsFullscreen(w.win) != 0; fullScreen != w.chrome.fullScreen {
		w.chrome.fullScreen = fullScreen
		w.chrome.fullScreenRequest = nil
		w.showMenuBar()
	}
	w.updateEnvironment()
}

// sessionLocale returns the user's locale as a BCP 47 tag, such as
// "en-US".
func sessionLocale() string {
	locale := ""
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			locale = v
			break
		}
	}
	// Drop the encoding and modifier: "de_DE.UTF-8@euro" is "de-DE".
	if i := strings.IndexAny(locale, ".@"); i >= 0 {
		locale = locale[:i]
	}
	return strings.ReplaceAll(locale, "_", "-")
}

// localeSettings is what the session sets apart from the language: the
// region of LC_TIME's formats, LC_MEASUREMENT's system, and GNOME's
// clock-format over the hours.
func localeSettings() driver.LocaleSettings {
	region := func(category string) string {
		for _, key := range []string{"LC_ALL", category, "LANG"} {
			if v := os.Getenv(key); v != "" {
				v, _, _ = strings.Cut(v, ".")
				v, _, _ = strings.Cut(v, "@")
				if _, r, ok := strings.Cut(v, "_"); ok && len(r) == 2 {
					return strings.ToUpper(r)
				}
				return ""
			}
		}
		return ""
	}
	s := driver.LocaleSettings{Region: region("LC_TIME")}
	switch region("LC_MEASUREMENT") {
	case "":
	case "US", "LR", "MM":
		s.MeasurementSystem = "us"
	case "GB":
		s.MeasurementSystem = "uk"
	default:
		s.MeasurementSystem = "metric"
	}
	switch gnomeClockFormat() {
	case "24h":
		s.HourCycle = "h23"
	case "12h":
		s.HourCycle = "h12"
	}
	return s
}

// gnomeClockFormat is GNOME's clock-format setting, "" where its schema
// is not installed.
func gnomeClockFormat() string {
	const schema = "org.gnome.desktop.interface"
	gnomeSettingsOnce.Do(func() {
		var lookup, source, settingsNew uintptr
		var err error
		if source, err = purego.Dlsym(gioLib, "g_settings_schema_source_get_default"); err != nil {
			return
		}
		if lookup, err = purego.Dlsym(gioLib, "g_settings_schema_source_lookup"); err != nil {
			return
		}
		if settingsNew, err = purego.Dlsym(gioLib, "g_settings_new"); err != nil {
			return
		}
		src, _, _ := purego.SyscallN(source)
		if src == 0 {
			return
		}
		if found, _, _ := purego.SyscallN(lookup, src, uintptr(unsafe.Pointer(cstr(schema))), 1); found == 0 {
			return
		}
		gnomeSettings, _, _ = purego.SyscallN(settingsNew, uintptr(unsafe.Pointer(cstr(schema))))
		gnomeGetString, _ = purego.Dlsym(gioLib, "g_settings_get_string")
	})
	if gnomeSettings == 0 || gnomeGetString == 0 {
		return ""
	}
	p, _, _ := purego.SyscallN(gnomeGetString, gnomeSettings, uintptr(unsafe.Pointer(cstr("clock-format"))))
	s := goString(p)
	if p != 0 {
		gFree(p)
	}
	return s
}

var (
	gnomeSettingsOnce             sync.Once
	gnomeSettings, gnomeGetString uintptr
)

var (
	gnomeA11yOnce                      sync.Once
	gnomeA11ySettings, gnomeGetBoolean uintptr
)

// gnomeHighContrast reads GNOME's optional accessibility setting without linking to GSettings at build time. Other GTK desktops use their theme name, handled by environment.
func gnomeHighContrast() bool {
	gnomeA11yOnce.Do(func() {
		var lookup, source, settingsNew uintptr
		var err error
		if source, err = purego.Dlsym(gioLib, "g_settings_schema_source_get_default"); err != nil {
			return
		}
		if lookup, err = purego.Dlsym(gioLib, "g_settings_schema_source_lookup"); err != nil {
			return
		}
		if settingsNew, err = purego.Dlsym(gioLib, "g_settings_new"); err != nil {
			return
		}
		src, _, _ := purego.SyscallN(source)
		if src == 0 {
			return
		}
		if found, _, _ := purego.SyscallN(lookup, src, uintptr(unsafe.Pointer(cstr("org.gnome.desktop.a11y.interface"))), 1); found == 0 {
			return
		}
		gnomeA11ySettings, _, _ = purego.SyscallN(settingsNew, uintptr(unsafe.Pointer(cstr("org.gnome.desktop.a11y.interface"))))
		gnomeGetBoolean, _ = purego.Dlsym(gioLib, "g_settings_get_boolean")
	})
	if gnomeA11ySettings == 0 || gnomeGetBoolean == 0 {
		return false
	}
	v, _, _ := purego.SyscallN(gnomeGetBoolean, gnomeA11ySettings, uintptr(unsafe.Pointer(cstr("high-contrast"))))
	return v != 0
}

func gnomeA11ySettingsForWatch() uintptr {
	gnomeHighContrast()
	return gnomeA11ySettings
}

// rightToLeft reports whether locale's language reads right to left.
func rightToLeft(locale string) bool {
	language, _, _ := strings.Cut(strings.ToLower(locale), "-")
	switch language {
	case "ar", "arc", "ckb", "dv", "fa", "he", "ks", "ku", "ps", "sd", "ug", "ur", "yi":
		return true
	}
	return false
}

// motionReduced uses a test's pinned preference for the renderer too; the system's environment is kept intact for future preference changes.
func (w *window) motionReduced() bool { return driver.Pinned(w.env, w.envPin).ReduceMotion }
