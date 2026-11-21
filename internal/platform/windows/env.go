//go:build windows

package windows

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// displayPlacement is a monitor as monitorLayout lays it out: origin is its top left in points.
type displayPlacement struct {
	monitor uintptr
	info    monitorInfoEx
	scale   float64
	origin  driver.Point
	display driver.DisplayInfo
}

var (
	dwmapi = syscall.NewLazyDLL("dwmapi.dll")
	shcore = syscall.NewLazyDLL("shcore.dll")

	systemParametersInfoW    = user32.NewProc("SystemParametersInfoW")
	getUserDefaultLocaleName = kernel32.NewProc("GetUserDefaultLocaleName")
	getLocaleInfoEx          = kernel32.NewProc("GetLocaleInfoEx")
	dwmSetWindowAttribute    = dwmapi.NewProc("DwmSetWindowAttribute")
	enumDisplayMonitors      = user32.NewProc("EnumDisplayMonitors")
	getDpiForMonitor         = shcore.NewProc("GetDpiForMonitor")

	isWindowVisible          = user32.NewProc("IsWindowVisible")
	getForegroundWindow      = user32.NewProc("GetForegroundWindow")
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
)

const (
	spiGetScreenReader        = 0x0046
	spiGetHighContrast        = 0x0042
	spiGetClientAreaAnimation = 0x1042
	localeIReadingLayout      = 0x0070
	localeIFirstDayOfWeek     = 0x100C // 0 Monday to 6 Sunday
	localeSTimeFormat         = 0x1003
	localeIMeasure            = 0x000D // 0 metric, 1 U.S.
	localeReturnNumber        = 0x20000000
	localeNameMaxLength       = 85
	hcfHighContrastOn         = 0x00000001

	dwmwaUseImmersiveDarkMode    = 20
	dwmwaUseImmersiveDarkModeOld = 19 // before Windows 10 20H1
)

type monitorInfoEx struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
	Device  [32]uint16
}

type highContrast struct {
	Size          uint32
	Flags         uint32
	DefaultScheme *uint16
}

// environment reads what Windows says about where the app runs, with
// what the hidden title bar's window controls cover. There is no
// software keyboard, so its inset stays 0.
func (w *window) environment() driver.Environment {
	locale := userLocale()
	deviceName, deviceModel := deviceIdentity()
	displays, displayID := w.displays()
	return driver.Environment{
		LaunchedAtLogin:       w.opts.LaunchedAtLogin,
		DeviceName:            deviceName,
		DeviceModel:           deviceModel,
		Dark:                  regDWORD(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme", 1) == 0,
		FontScale:             float32(regDWORD(`Software\Microsoft\Accessibility`, "TextScaleFactor", 100)) / 100,
		ReduceMotion:          !systemFlag(spiGetClientAreaAnimation, true),
		IncreaseContrast:      highContrastEnabled(),
		ScreenReader:          systemFlag(spiGetScreenReader, false),
		ScreenColor:           true,
		RTL:                   rightToLeft(locale),
		PixelRatio:            float32(w.scale),
		Locale:                locale,
		LocaleSettings:        localeSettings(),
		KeyCodeNames:          w.keyboardCodeNames(),
		Displays:              displays,
		DisplayID:             displayID,
		OS:                    driver.OSWindows,
		HasWebView:            true,
		CanSendInput:          true,
		CanLockPointer:        true,
		OSVersion:             osVersion(),
		Idiom:                 driver.IdiomDesktop,
		TitleBar:              w.titleBarArea(),
		WindowMaterial:        w.material,
		Maximized:             w.zoomed(),
		FullScreen:            w.chrome.fullScreen,
		Minimized:             w.minimized(),
		WindowFrame:           w.windowFrame(),
		CanDockPanels:         !w.embedded,
		Lifecycle:             w.lifecycle(),
		LowPowerMode:          lowPowerMode(),
		Speech:                speechAvailable(),
		Speak:                 speakAvailable(),
		ScanCode:              scanCameraAvailable(),
		Camera:                scanCameraAvailable(),
		Gamepads:              true,
		Audio:                 audioAvailable(),
		AudioPlaybackControls: audioAvailable(),
		FrontmostApp:          frontmostApp(),
		GPU:                   gpuAvailable(),
		DiscoverServices:      dnsAvailable(),
		ClipboardItems:        true,

		// Only a window opened transparent has the composition that shows what is behind it (look.go); a transparent panel's is its own.
		CanMakeWindowsTransparent: w.canTransparent && !w.embedded && w.kind == driver.SurfaceWindow,
		CanSkipTaskbar:            w.kind == driver.SurfaceWindow && !w.embedded && !(w.primary && w.opts.Hidden),

		// The foreground window says which app is in front to every app.
		CanReadFrontmostApp: true,
		CustomCursor:        true,

		// The toast notifier's schedule, with daily and weekly repeats chained while the app runs and rearmed on launch.
		ScheduledNotifications: true,

		// An embedding host owns its window, and the app's own surface and a tray icon's show in none, as on macOS.
		CanQuickLook:      w.showsOwnWindow(),
		CanSetWindowLevel: w.showsOwnWindow(),
		CanPlaceWindows:   w.showsOwnWindow(),

		// A window or a panel opened transparent with DirectComposition draws nothing of its own until SetWindowTransparent frames the window (look.go).
		WindowTransparent: w.transparent,
	}
}

// deviceIdentity returns the device name Settings' About page shows, the computer's physical DNS host name as os.Hostname reads it, and the firmware's product name. Either may be unavailable on a restricted host.
func deviceIdentity() (name, model string) {
	name, _ = os.Hostname()
	return name, hardwareModel()
}

// hardwareModel is the firmware's product name, empty when the board maker left a placeholder there. It never changes while the app runs.
var hardwareModel = sync.OnceValue(func() string {
	return firmwareModel(regString(syscall.HKEY_LOCAL_MACHINE, 0, `HARDWARE\DESCRIPTION\System\BIOS`, "SystemProductName"))
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

func highContrastEnabled() bool {
	hc := highContrast{Size: uint32(unsafe.Sizeof(highContrast{}))}
	r, _, _ := systemParametersInfoW.Call(spiGetHighContrast, uintptr(hc.Size), uintptr(unsafe.Pointer(&hc)), 0)
	return r != 0 && hc.Flags&hcfHighContrastOn != 0
}

// displays reads the monitors in the virtual desktop, in the points monitorLayout lays them out in, as WindowFrame uses.
func (w *window) displays() ([]driver.DisplayInfo, string) {
	placements := monitorLayout()
	out := make([]driver.DisplayInfo, 0, len(placements))
	current := w.currentMonitor()
	currentID := ""
	for _, placement := range placements {
		display := placement.display
		display.RefreshRate, display.MaxRefreshRate = displayRates(placement.info.Device)
		if placement.monitor == current {
			currentID = display.ID
		}
		out = append(out, display)
	}
	return out, currentID
}

// currentMonitor follows the surface's top-level window, including a host's.
func (w *window) currentMonitor() uintptr {
	if hwnd := w.owner(); hwnd != 0 && !(w.primary && w.opts.Hidden) && w.kind != driver.SurfaceTrayIcon {
		monitor, _, _ := monitorFromWindow.Call(hwnd, monitorDefaultNear)
		return monitor
	}
	return 0
}

func (w *window) currentDisplayID() string {
	info := monitorInfoEx{Size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
	if monitor := w.currentMonitor(); monitor != 0 {
		if ok, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
			return monitorDisplayID(monitor, info.Device)
		}
	}
	return ""
}

// monitorLayout reads the monitors, in physical pixels, with their DPI, and lays them out in points (layoutMonitors).
func monitorLayout() []displayPlacement {
	monitorsFound.mu.Lock()
	monitorsFound.all = nil
	enumDisplayMonitors.Call(0, 0, monitorFound, 0)
	all := monitorsFound.all
	monitorsFound.all = nil
	monitorsFound.mu.Unlock()
	scales := make([]float64, len(all))
	for i, item := range all {
		scales[i] = 1
		var dpi, dpiY uint32
		if r, _, _ := getDpiForMonitor.Call(item.monitor, 0, uintptr(unsafe.Pointer(&dpi)), uintptr(unsafe.Pointer(&dpiY))); r == 0 && dpi > 0 {
			scales[i] = float64(dpi) / 96
		}
	}
	return layoutMonitors(all, scales)
}

// layoutMonitors gives each monitor its place in one point space. A monitor's top left is its pixel offset from the primary monitor's top left divided by the smallest scale of all monitors, and within a monitor a point is that monitor's own scale's. Monitors never overlap in pixels, and no monitor's scale is below the smallest, so their frames never overlap in points either, and at one shared scale they tile as their pixels do. Dividing every offset by its own monitor's scale would not: a 200% monitor right of a 100% one would start halfway across it.
func layoutMonitors(all []foundMonitor, scales []float64) []displayPlacement {
	if len(all) == 0 {
		return nil
	}
	primary := 0
	for i := range all {
		if all[i].info.Flags&1 != 0 {
			primary = i
			break
		}
	}
	base := all[primary].info.Monitor
	least := scales[0]
	for _, scale := range scales {
		least = min(least, scale)
	}
	out := make([]displayPlacement, 0, len(all))
	for i, item := range all {
		p := displayPlacement{monitor: item.monitor, info: item.info, scale: scales[i]}
		p.origin = driver.Point{X: driver.Points(float64(item.info.Monitor.Left-base.Left) / least), Y: driver.Points(float64(item.info.Monitor.Top-base.Top) / least)}
		bounds := func(r rect) driver.Bounds {
			return driver.Bounds{Origin: p.point(r.Left, r.Top), Size: driver.Size{Width: driver.Points(float64(r.Right-r.Left) / p.scale), Height: driver.Points(float64(r.Bottom-r.Top) / p.scale)}}
		}
		p.display = driver.DisplayInfo{ID: monitorDisplayID(item.monitor, item.info.Device), Frame: bounds(item.info.Monitor), VisibleFrame: bounds(item.info.Work), Scale: float32(p.scale), Main: item.info.Flags&1 != 0}
		out = append(out, p)
	}
	return out
}

func monitorDisplayID(monitor uintptr, device [32]uint16) string {
	if id := syscall.UTF16ToString(device[:]); id != "" {
		return id
	}
	return fmt.Sprintf("%x", monitor)
}

// point converts a virtual desktop pixel position to points through the monitor.
func (p displayPlacement) point(x, y int32) driver.Point {
	return driver.Point{X: p.origin.X + driver.Points(float64(x-p.info.Monitor.Left)/p.scale), Y: p.origin.Y + driver.Points(float64(y-p.info.Monitor.Top)/p.scale)}
}

// pixel converts a position in points to the virtual desktop's pixels through the monitor, as point's inverse.
func (p displayPlacement) pixel(x, y driver.Points) (int32, int32) {
	return p.info.Monitor.Left + int32(math.Round(float64(x-p.origin.X)*p.scale)), p.info.Monitor.Top + int32(math.Round(float64(y-p.origin.Y)*p.scale))
}

// px converts a length in points to the monitor's pixels.
func (p displayPlacement) px(v driver.Points) int32 { return int32(math.Round(float64(v) * p.scale)) }

// monitorsFound collects what EnumDisplayMonitors reports, through the one callback monitorFound: Go makes at most about 2000 callbacks in a process, and windows read the layout on every move.
var monitorsFound struct {
	mu  sync.Mutex
	all []foundMonitor
}

type foundMonitor struct {
	monitor uintptr
	info    monitorInfoEx
}

var monitorFound = syscall.NewCallback(func(monitor, _, _, _ uintptr) uintptr {
	var info monitorInfoEx
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
		monitorsFound.all = append(monitorsFound.all, foundMonitor{monitor: monitor, info: info})
	}
	return 1
})

func displayContains(d driver.Bounds, p driver.Point) bool {
	return p.X >= d.Origin.X && p.X < d.Origin.X+d.Size.Width && p.Y >= d.Origin.Y && p.Y < d.Origin.Y+d.Size.Height
}

// placementForFrame chooses the monitor that owns the frame's center. A frame copied from Env.Displays therefore uses that display's scale when it is passed back to SetWindowFrame, even before the window has moved there.
func placementForFrame(f driver.WindowFrame) (displayPlacement, bool) {
	return choosePlacement(monitorLayout(), f)
}

// choosePlacement is placementForFrame on placements: the one whose frame holds f's center, which layoutMonitors keeps to at most one, or else the nearest.
func choosePlacement(placements []displayPlacement, f driver.WindowFrame) (displayPlacement, bool) {
	p := driver.Point{X: f.X + f.Width/2, Y: f.Y + f.Height/2}
	for _, placement := range placements {
		if displayContains(placement.display.Frame, p) {
			return placement, true
		}
	}
	if len(placements) == 0 {
		return displayPlacement{}, false
	}
	// If a frame is outside every display, keep the nearest monitor's scale.
	best, distance := placements[0], math.Inf(1)
	for _, placement := range placements {
		dx := max(max(float64(placement.display.Frame.Origin.X-p.X), float64(p.X-(placement.display.Frame.Origin.X+placement.display.Frame.Size.Width))), 0)
		dy := max(max(float64(placement.display.Frame.Origin.Y-p.Y), float64(p.Y-(placement.display.Frame.Origin.Y+placement.display.Frame.Size.Height))), 0)
		if d := dx*dx + dy*dy; d < distance {
			best, distance = placement, d
		}
	}
	return best, true
}

// appActive is whether the app is the active one, as the last
// WM_ACTIVATEAPP said, which Windows sends per thread; before the first,
// whether the foreground window is the process's.
var appActive = sync.OnceValue(func() *bool {
	active := false
	if fg, _, _ := getForegroundWindow.Call(); fg != 0 {
		var pid uint32
		getWindowThreadProcessID.Call(fg, uintptr(unsafe.Pointer(&pid)))
		active = pid == uint32(syscall.Getpid())
	}
	return &active
})

// lifecycle is whether the app is in use: in the background while the window, an embedded view's root window, is minimized or hidden, active while the app is the active one. The app's own surface, the hidden root window nsgo.Run(App) runs on, takes the lifecycle of the windows and panels it opened, as on macOS and Linux: in the background only while none of them shows.
func (w *window) lifecycle() driver.Lifecycle {
	l := w.windowLifecycle()
	if w.primary && w.opts.Hidden {
		for _, other := range windows {
			if other.hostWindow == w && other.kind != driver.SurfaceTrayIcon && other.hwnd != 0 && !other.destroying {
				l = min(l, other.windowLifecycle())
			}
		}
	}
	return l
}

// updateAppSurface reads the environment of the app's own surface again after one of the windows it opened changed or went, as its lifecycle follows theirs.
func (w *window) updateAppSurface() {
	if h := w.hostWindow; h != nil && h != w && h.primary && h.opts.Hidden && h.engine != nil && h.hwnd != 0 {
		h.updateEnvironment()
	}
}

// minimized reports whether the window, or an embedded view's ancestor root window, is minimized.
func (w *window) minimized() bool {
	if w.embedded {
		if state, ok := embeddedRoots.stateOf(w); ok {
			return state.minimized
		}
		root := w.embeddedAncestorRoot()
		return root != 0 && isIconicWindow(root)
	}
	return isIconicWindow(w.hwnd)
}

// windowLifecycle is the lifecycle of the window itself, an embedded view's root window's.
func (w *window) windowLifecycle() driver.Lifecycle {
	if w.embedded {
		if state, ok := embeddedRoots.stateOf(w); ok {
			return state.lifecycle()
		}
		return readEmbeddedRootState(w.embeddedAncestorRoot()).lifecycle()
	}
	root := w.owner()
	if visible, _, _ := isWindowVisible.Call(root); visible == 0 || isIconicWindow(root) {
		return driver.LifecycleBackground
	}
	if *appActive() {
		return driver.LifecycleActive
	}
	return driver.LifecycleInactive
}

// updateEnvironment reads the environment again, after Windows broadcast
// a settings change, and reports it when it changed.
func (w *window) updateEnvironment() {
	env := w.environment()
	w.reportFrameWindow()
	if env.Equal(w.env) {
		return
	}
	w.devFrame.changed(w)
	dark := w.shownDark()
	redock := w.dock.Edge != driver.WindowPlaceDefault && w.visible && !slices.Equal(env.Displays, w.env.Displays)
	w.env = env
	if w.shownDark() != dark {
		w.darkTitleBar(w.shownDark())
		if w.chrome.bar != 0 && menuHasTemplateIcon(w.chrome.menu) {
			w.showMenuBar()
		}
	}
	w.engine.SetEnvironment(env)
	w.updateAppSurface()
	if redock {
		w.placeDockedPanel()
	}
}

func (w *window) reportFrameWindow() {
	if w.engine == nil || w.hwnd == 0 {
		return
	}
	foreground, _, _ := getForegroundWindow.Call()
	visible := w.kind != driver.SurfaceTrayIcon && w.windowLifecycle() != driver.LifecycleBackground
	w.engine.SetFrameWindow(visible, visible && foreground == w.owner())
}

// darkTitleBar matches the title bar to the appearance, where DWM can.
func (w *window) darkTitleBar(dark bool) {
	if dwmSetWindowAttribute.Find() != nil {
		return
	}
	on := int32(boolArg(dark))
	if r, _, _ := dwmSetWindowAttribute.Call(w.hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&on)), 4); r != 0 {
		dwmSetWindowAttribute.Call(w.hwnd, dwmwaUseImmersiveDarkModeOld, uintptr(unsafe.Pointer(&on)), 4)
	}
}

// regDWORD reads a DWORD under HKEY_CURRENT_USER, or returns fallback.
func regDWORD(path, name string, fallback uint32) uint32 {
	return regDWORDIn(syscall.HKEY_CURRENT_USER, path, name, fallback)
}

// regDWORDIn reads a DWORD under root, or returns fallback.
func regDWORDIn(root syscall.Handle, path, name string, fallback uint32) uint32 {
	var key syscall.Handle
	if syscall.RegOpenKeyEx(root, wide(path), 0, syscall.KEY_READ, &key) != nil {
		return fallback
	}
	defer syscall.RegCloseKey(key)
	var value, kind uint32
	size := uint32(unsafe.Sizeof(value))
	if syscall.RegQueryValueEx(key, wide(name), nil, &kind, (*byte)(unsafe.Pointer(&value)), &size) != nil ||
		kind != syscall.REG_DWORD {
		return fallback
	}
	return value
}

// systemFlag reads a BOOL system parameter, or returns fallback.
func systemFlag(action uintptr, fallback bool) bool {
	var on int32
	if r, _, _ := systemParametersInfoW.Call(action, 0, uintptr(unsafe.Pointer(&on)), 0); r == 0 {
		return fallback
	}
	return on != 0
}

// userLocale returns the user's locale as a BCP 47 tag, such as "en-US".
func userLocale() string {
	var buf [localeNameMaxLength]uint16
	if n, _, _ := getUserDefaultLocaleName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); n == 0 {
		return ""
	}
	// An alternate sort order follows an underscore, as in "de-DE_phoneb".
	name, _, _ := strings.Cut(syscall.UTF16ToString(buf[:]), "_")
	return name
}

// localeSettings reads the regional format with the user's changes:
// the first day of the week, the time format's hour letter and the
// measurement system.
func localeSettings() driver.LocaleSettings {
	var s driver.LocaleSettings
	number := func(kind uintptr) (uint32, bool) {
		var v uint32
		r, _, _ := getLocaleInfoEx.Call(0, kind|localeReturnNumber, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v)/2)
		return v, r != 0
	}
	if first, ok := number(localeIFirstDayOfWeek); ok && first <= 6 {
		s.FirstDayOfWeek = int(first) + 1
	}
	var buf [80]uint16
	if r, _, _ := getLocaleInfoEx.Call(0, localeSTimeFormat, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r != 0 {
		// The hours' letter, outside quoted text.
		quoted := false
		for _, c := range syscall.UTF16ToString(buf[:]) {
			if c == '\'' {
				quoted = !quoted
				continue
			}
			if c == 'H' && !quoted {
				s.HourCycle = "h23"
				break
			}
			if c == 'h' && !quoted {
				s.HourCycle = "h12"
				break
			}
		}
	}
	if m, ok := number(localeIMeasure); ok {
		s.MeasurementSystem = map[uint32]string{0: "metric", 1: "us"}[m]
	}
	return s
}

// rightToLeft reports whether locale's script reads right to left.
func rightToLeft(locale string) bool {
	var layout uint32 // 0 left to right, 1 right to left, 2 and 3 vertical
	r, _, _ := getLocaleInfoEx.Call(uintptr(unsafe.Pointer(wide(locale))), localeIReadingLayout|localeReturnNumber,
		uintptr(unsafe.Pointer(&layout)), unsafe.Sizeof(layout)/2) // in UTF-16 units
	return r != 0 && layout == 1
}

// showsOwnWindow reports whether the surface shows in a top-level window of its own, which SetWindowLevel and SetWindowFrame reach: not a view a host embeds, the app's own surface (Options.Hidden), or a tray icon's.
func (w *window) showsOwnWindow() bool {
	return !w.embedded && !(w.primary && w.opts.Hidden) && w.kind != driver.SurfaceTrayIcon
}

// cursorPoint converts the native cursor pixels through the display that
// contains them, rather than through the app window's current DPI.
func cursorPoint() (driver.Point, bool) {
	var at point
	if ok, _, _ := getCursorPos.Call(uintptr(unsafe.Pointer(&at))); ok == 0 {
		return driver.Point{}, false
	}
	bounds := rect{Left: at.X, Right: at.X + 1, Top: at.Y, Bottom: at.Y + 1}
	monitor, _, _ := monitorFromRect.Call(uintptr(unsafe.Pointer(&bounds)), monitorDefaultNear)
	for _, p := range monitorLayout() {
		if p.monitor == monitor {
			return p.point(at.X, at.Y), true
		}
	}
	return driver.Point{}, false
}

// motionReduced uses a test's pinned preference for the renderer too; the system's environment is kept intact for future preference changes.
func (w *window) motionReduced() bool { return driver.Pinned(w.env, w.envPin).ReduceMotion }
