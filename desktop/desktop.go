// Package desktop holds what only desktops have: the window, its title
// bar and the menus, as NSH's nsh/desktop. It builds on every platform,
// so one app runs everywhere: on phones and tablets its functions do
// nothing, and IsAvailable says whether they act.
//
// A title bar of the app's own calls SetTitleBar once, marks its
// background with core.View's Desktop.WindowDrag and keeps clear of the edges
// TitleBar reports; SetWindowControls moves or hides the system's
// controls in it. A window over a material leaves transparent what
// should show it while ShowsMaterial.
package desktop

import "github.com/nsgo-dev/nsgo/core"

type (
	// TitleBarStyle is whether the system draws the window's title bar.
	TitleBarStyle = core.TitleBarStyle
	// WindowFrame is where a window moves and how large it becomes, in
	// points from the main screen's top left.
	WindowFrame = core.WindowFrame
	// WindowLevel is where a window stacks among the others.
	WindowLevel = core.WindowLevel
	// WindowWorkspaces are the workspaces a window shows on: macOS's Spaces, the virtual desktops of Windows and Linux.
	WindowWorkspaces = core.WindowWorkspaces
	// WindowPointerEvents is where a window takes the pointer: everywhere, nowhere, or only where it draws something.
	WindowPointerEvents = core.WindowPointerEvents
	// WindowConstraints limit a window's size and what the user may do
	// to it.
	WindowConstraints = core.WindowConstraints
	// MenuBarMode says when Windows and Linux show the menu bar.
	MenuBarMode = core.MenuBarMode
	// WindowControls is how a hidden title bar shows the system's window
	// controls: hidden for the app to draw its own, or macOS's traffic
	// lights moved.
	WindowControls = core.WindowControls
	// WindowControlsStyle chooses the look of the caption buttons NSGo draws on Windows.
	WindowControlsStyle = core.WindowControlsStyle
	// WindowMaterial is a translucent material behind the window's
	// content: one of macOS's vibrancy materials, or of Windows 11's
	// system backdrops, each platform showing its closest.
	WindowMaterial = core.WindowMaterial
)

const (
	TitleBarDefault = core.TitleBarDefault
	TitleBarHidden  = core.TitleBarHidden
)

const (
	// WindowControlsDefault draws NSGo's compact rounded caption buttons on Windows.
	WindowControlsDefault = core.WindowControlsDefault
	// WindowControlsSystem draws Windows 11's full-height caption buttons, flush with the window's edge.
	WindowControlsSystem = core.WindowControlsSystem
)

const (
	WindowLevelNormal   = core.WindowLevelNormal
	WindowLevelFloating = core.WindowLevelFloating
	WindowLevelDesktop  = core.WindowLevelDesktop
)

const (
	// WindowWorkspacesCurrent keeps a window on the workspace it opened on.
	WindowWorkspacesCurrent = core.WindowWorkspacesCurrent
	// WindowWorkspacesAll shows it on every workspace.
	WindowWorkspacesAll = core.WindowWorkspacesAll
)

const (
	// WindowPointerAuto takes the pointer everywhere in the window, as windows do.
	WindowPointerAuto = core.WindowPointerAuto
	// WindowPointerNone lets every press, scroll and hover through to what is below, as a heads-up display does.
	WindowPointerNone = core.WindowPointerNone
	// WindowPointerPainted takes the pointer only over the views that draw something, such as a background, a border, a text or an image, or that hear it, such as a pressable, a context menu or a window drag handle; elsewhere it goes through, as around a desktop pet.
	WindowPointerPainted = core.WindowPointerPainted
)

const (
	MenuBarShown    = core.MenuBarShown
	MenuBarAutoHide = core.MenuBarAutoHide
)

const (
	WindowMaterialNone = core.WindowMaterialNone
	// The materials of window areas.
	WindowMaterialWindow      = core.WindowMaterialWindow
	WindowMaterialContent     = core.WindowMaterialContent
	WindowMaterialSidebar     = core.WindowMaterialSidebar
	WindowMaterialHeader      = core.WindowMaterialHeader
	WindowMaterialTitlebar    = core.WindowMaterialTitlebar
	WindowMaterialUnderWindow = core.WindowMaterialUnderWindow
	WindowMaterialUnderPage   = core.WindowMaterialUnderPage
	// The materials of transient surfaces, Acrylic on Windows.
	WindowMaterialMenu         = core.WindowMaterialMenu
	WindowMaterialPopover      = core.WindowMaterialPopover
	WindowMaterialHUD          = core.WindowMaterialHUD
	WindowMaterialSheet        = core.WindowMaterialSheet
	WindowMaterialTooltip      = core.WindowMaterialTooltip
	WindowMaterialSelection    = core.WindowMaterialSelection
	WindowMaterialFullScreenUI = core.WindowMaterialFullScreenUI
	// Windows 11's system backdrops: Mica, Acrylic and Mica Alt.
	WindowMaterialMica    = core.WindowMaterialMica
	WindowMaterialAcrylic = core.WindowMaterialAcrylic
	WindowMaterialTabbed  = core.WindowMaterialTabbed
)

// IsAvailable reports whether the app runs on macOS, Windows or Linux, where this package's functions act; a web build that runs as one (core.Env.Simulated) does not.
func IsAvailable(cx *core.Scope) bool {
	switch cx.Env().OS() {
	case core.OSMacOS, core.OSWindows, core.OSLinux:
		return !cx.Env().Simulated()
	}
	return false
}

// CanLookUp reports whether LookUp shows the system's dictionary panel: in a window on macOS. Windows, Linux and the web have no such panel, so it is false there, as for the app's own scope and a tray icon's, which show in no window.
func CanLookUp(cx *core.Scope) bool { return cx.Env().CanLookUp() }

// LookUp shows the system's dictionary panel for text, as a three-finger tap or Ctrl-Cmd-D does on macOS, with the text drawn over at, a box in the view in refers to (the surface's when in is nil), such as a TextSelection's Text and Bounds with its Selectable view's Ref. It reports whether the panel was asked for: false for blank text, an in that is not mounted, where CanLookUp is false, and for a confined view the user is not using (core.Scope.Confine).
func LookUp(cx *core.Scope, text string, in *core.Ref, at core.Bounds) bool {
	return cx.LookUp(text, in, at)
}

// CanQuickLook reports whether QuickLook shows the system's file preview panel: in a window on macOS. Windows, Linux and the web have no such panel, so it is false there, as for a view a host embeds, whose app the panel asks for its files, and for the app's own scope and a tray icon's, which show in no window.
func CanQuickLook(cx *core.Scope) bool { return cx.Env().CanQuickLook() }

// QuickLook shows the system's file preview panel on paths, starting at index, as Space does in the Finder; called again while the panel shows, it shows the new paths, as the Finder's panel follows its selection. The user closes it with Space or Escape. It reports whether the panel was asked for: false for no paths, a path that is not a regular file, where CanQuickLook is false, and for a confined view the user is not using (core.Scope.Confine). The panel reads the files while it shows them.
func QuickLook(cx *core.Scope, paths []string, index int) bool {
	return cx.QuickLook(paths, index)
}

// SetTitle sets the window's title on desktops and the page's on the web.
func SetTitle(cx *core.Scope, title string) { cx.SetTitle(title) }

// SetTitleBar hides the system's title bar so the app can draw its own height points tall (0: the platform's height), or restores it. The platform centers its window controls in that height. TitleBar then reports the area they keep. A window with a hidden title bar has no menu bar on Windows and Linux; on Windows, Alt and F10 open the application menu as a popup from the window's top corner instead, and Alt with a top-level item's first letter opens that item's menu.
func SetTitleBar(cx *core.Scope, style TitleBarStyle, height core.Points) {
	cx.SetTitleBar(style, height)
}

// TitleBar returns what a hidden title bar covers: Top is its height,
// Left and Right the widths the system's window controls take at its
// ends. It is zero while the system draws the title bar, in full screen
// and off desktops.
func TitleBar(cx *core.Scope) core.Edges { return cx.Env().TitleBar() }

// SetWindowControls hides the system's window controls in a hidden
// title bar, or moves macOS's traffic lights in it, from any time on:
//
//	desktop.SetWindowControls(cx, desktop.WindowControls{TrafficLights: core.Point{X: 20, Y: 18}})
//
// Offset moves them from there, as a transition does each frame. TitleBar
// follows where they are. The zero WindowControls restores the
// platform's.
func SetWindowControls(cx *core.Scope, c WindowControls) { cx.SetWindowControls(c) }

// SetWindowMaterial puts m behind the window's content, or makes the
// window opaque again with WindowMaterialNone; nsgo.WindowMaterial
// starts the window with one. macOS changes it any time; Windows 11 22H2
// and later only in a window started with a material, others staying
// opaque.
func SetWindowMaterial(cx *core.Scope, m WindowMaterial) { cx.SetWindowMaterial(m) }

// SetWindowTransparent makes the window transparent, without a background or frame, so what is behind it shows wherever the app draws nothing, or gives it back both; WindowOptions.Transparent and nsgo.WindowTransparent open a window so. It takes effect where CanMakeWindowsTransparent is true: macOS and Linux change it any time, and Windows only in a window opened transparent, whose composition holds from its creation; such a window is opaque, white behind the app's views, while framed.
func SetWindowTransparent(cx *core.Scope, on bool) { cx.SetWindowTransparent(on) }

// SetWindowShadow gives the window the system's shadow, or takes it away; WindowOptions.NoShadow and nsgo.WindowNoShadow open a window without it. Windows draws none around a transparent window and keeps it around a framed one.
func SetWindowShadow(cx *core.Scope, on bool) { cx.SetWindowShadow(on) }

// SetSkipTaskbar keeps the window off the Windows taskbar and Alt+Tab, off the task list on X11, and out of macOS's Window menu and Cmd+`, or puts it back; WindowOptions.SkipTaskbar and nsgo.SkipTaskbar open a window so. On macOS the Dock shows the app rather than its windows, and SetDockVisible takes it out. CanSkipTaskbar says whether it takes effect.
func SetSkipTaskbar(cx *core.Scope, on bool) { cx.SetSkipTaskbar(on) }

// CanMakeWindowsTransparent reports whether the scope's window shows what is behind it while transparent: on macOS; on Windows in a window opened transparent where DirectComposition draws it, the only one SetWindowTransparent changes; on Linux where the display composites with an RGBA visual, as Wayland always does and X11 with a compositing manager. It is false elsewhere, where a transparent window stays opaque, and for a view a host embeds, the app's own scope, a panel or a tray icon, which show in no window of their own. It is what the window could do, not what it does: Transparent says whether the window or a transparent panel draws nothing of its own now.
func CanMakeWindowsTransparent(cx *core.Scope) bool { return cx.Env().CanMakeWindowsTransparent() }

// CanSkipTaskbar reports whether SkipTaskbar and SetSkipTaskbar keep the scope's window off the taskbar: on Windows, on Linux under X11, and on macOS, where they keep it out of the Window menu and Cmd+` and SetDockVisible is the Dock's. Wayland has no way for an app to, and it is false there, on phones and the web, and for a view a host embeds, the app's own scope, a panel or a tray icon.
func CanSkipTaskbar(cx *core.Scope) bool { return cx.Env().CanSkipTaskbar() }

// ShowsMaterial reports whether a material shows behind the window's
// content, where the app leaves it transparent. Where it is false, give
// those parts a background of their own.
func ShowsMaterial(cx *core.Scope) bool {
	return cx.Env().WindowMaterial() != core.WindowMaterialNone
}

// Transparent reports whether the scope's window or panel draws nothing of its own now, no background, frame or shadow, so what is behind it shows wherever the content draws nothing: a panel opened with PanelOptions.Transparent, or a window opened with WindowOptions.Transparent or nsgo.WindowTransparent or made so by SetWindowTransparent, where the platform could. CanMakeWindowsTransparent says whether a window could be transparent, Transparent whether it is: it turns false as SetWindowTransparent gives the window its frame back, and stays false where the platform could not clear it, as on Linux without a compositor or on Windows in a window not opened transparent. Where it is false, give the content a background of its own.
func Transparent(cx *core.Scope) bool { return cx.Env().WindowTransparent() }

// Maximized reports whether the window is maximized or zoomed, so a
// maximize button of the app's own can show restore.
func Maximized(cx *core.Scope) bool { return cx.Env().Maximized() }

// FullScreen reports whether the window is in full screen.
func FullScreen(cx *core.Scope) bool { return cx.Env().FullScreen() }

// Minimized reports whether the window is minimized, so an app can stop work the user cannot see and resume it once the window is restored. It is always false on phones and the web, and on Wayland, which reports no minimized state.
func Minimized(cx *core.Scope) bool { return cx.Env().Minimized() }

// Frame returns where the window is and how large, as SetWindowFrame and
// nsgo.WindowFrame take them, changing as it moves and resizes. While
// the window is maximized or in full screen it is that frame, so an app
// remembering a window keeps the one it last had otherwise. Linux knows
// only the size GTK keeps, which ignores those states, and Move says no
// position came; zero without a window.
func Frame(cx *core.Scope) WindowFrame { return cx.Env().WindowFrame() }

// Monitors returns the displays and their bounds in desktop screen coordinates.
func Monitors(cx *core.Scope) []core.DisplayInfo { return cx.Env().Displays() }

// CursorPoint calls done with the pointer position, even outside the app's
// windows, in the same points and coordinate space as Monitors and Window.ShowAt.
// The bool is false when the position is unavailable, including on Wayland.
// Poll by calling CursorPoint again when another sample is needed.
func CursorPoint(cx *core.Scope, done func(point core.Point, available bool)) {
	cx.CursorPoint(done)
}

// MinimizeWindow minimizes the window.
func MinimizeWindow(cx *core.Scope) { cx.MinimizeWindow() }

// ToggleMaximize maximizes (zooms) the window, or restores it.
func ToggleMaximize(cx *core.Scope) { cx.ToggleMaximize() }

// ToggleFullScreen enters or leaves full screen.
// On the web, call it from a press handler. Browser refusals go to the log.
func ToggleFullScreen(cx *core.Scope) { cx.ToggleFullScreen() }

// SetFullScreen enters or leaves full screen. A display ID from Env.Displays
// selects a desktop display. An omitted ID uses the window's current display.
// On the web, call it from a press or another user gesture. The browser chooses
// the display and Env.FullScreen follows changes, including leaving with Esc.
func SetFullScreen(cx *core.Scope, on bool, displayID ...string) { cx.SetFullScreen(on, displayID...) }

// CloseWindow closes the window as its close button does: the window's
// OnClosing handlers may keep it, and the app ends by its shutdown mode.
func CloseWindow(cx *core.Scope) { cx.CloseWindow() }

// SetWindowFrame moves and sizes the window: f's top left corner is in points from the main screen's top left on every platform, and fields it leaves out keep their values, so a window given only a new height keeps its top edge. macOS animates the change when f asks. Linux sets the size GTK keeps for the window and positions it on X11; a Wayland compositor places an ordinary toplevel itself; CanPlaceWindows says whether the position moves the window.
func SetWindowFrame(cx *core.Scope, f WindowFrame) { cx.SetWindowFrame(f) }

// SetWindowLevel puts the window at level among the others. macOS floats it at NSFloatingWindowLevel, or sinks it to the desktop's icons on every space; Windows makes it topmost, or puts it at the bottom of the z-order; Linux on X11 asks the window manager for _NET_WM_STATE_ABOVE or _NET_WM_STATE_BELOW, again each time the window shows. A Wayland compositor stacks its windows itself, and where CanSetWindowLevel is false the level does nothing.
func SetWindowLevel(cx *core.Scope, level WindowLevel) { cx.SetWindowLevel(level) }

// CanSetWindowLevel reports whether SetWindowLevel stacks the scope's window: on macOS, Windows and Linux on X11, but not for a view a host embeds, the app's own scope or a tray icon's, which show in no window, a Linux panel no window manager manages, or on Wayland.
func CanSetWindowLevel(cx *core.Scope) bool { return cx.Env().CanSetWindowLevel() }

// CanPlaceWindows reports whether SetWindowFrame moves the scope's window to the position it gives: on macOS, Windows and X11, and for an Android overlay (mobile.OpenOverlay), which it keeps on the screen, but not for a view a host embeds, the app's own scope or a tray icon's, which show in no window, or an ordinary Wayland toplevel.
func CanPlaceWindows(cx *core.Scope) bool { return cx.Env().CanPlaceWindows() }

// SetWindowWorkspaces shows the window on every workspace, as a desktop pet, a HUD or a timer does, or back on one, the one the user is on; WindowOptions.Workspaces opens a window so. On macOS a window on every Space also shows over other apps' full screen spaces and stays out of Cmd-` cycling, and SetWindowLevel keeps it on every Space; a level that floats keeps it above the windows there, and the desktop's level behind them, out of full screen spaces. Linux puts it on every virtual desktop on X11, where the window manager supports _NET_WM_DESKTOP. Windows, which has no public way to pin a window to every virtual desktop, and Wayland leave it on the one it opened on; CanSetWindowWorkspaces says whether it takes effect.
func SetWindowWorkspaces(cx *core.Scope, w WindowWorkspaces) { cx.SetWindowWorkspaces(w) }

// CanSetWindowWorkspaces reports whether SetWindowWorkspaces puts the scope's window on the workspaces it names: on macOS, and on Linux on X11 where the window manager lists _NET_WM_DESKTOP among the hints it supports (_NET_SUPPORTED), as it says when the window opens. It is false on Windows, on Wayland, on phones and the web, and for a view a host embeds, the app's own scope or a tray icon's, which show in no window, or a Linux panel no window manager manages.
func CanSetWindowWorkspaces(cx *core.Scope) bool { return cx.Env().CanSetWindowWorkspaces() }

// SetWindowPointerEvents says where the window takes the pointer, from its next frame: everywhere, the default, nowhere, or only where it draws something, the pointer going through to the windows below elsewhere. WindowOptions.PointerEvents and PanelOptions.PointerEvents open a window so. A window that takes none hears no presses, so an app that lets one through entirely switches it back from elsewhere, such as a global shortcut or a tray icon; one that takes the pointer where it draws hears it there:
//
//	desktop.SetWindowPointerEvents(cx, desktop.WindowPointerPainted)
//
// Code held to an Access needs it to grant core.AccessOverlay, else the window takes the pointer. A view whose background fills the window makes a painted window take it everywhere, and the title bar the system draws, which is no view, lets it through.
func SetWindowPointerEvents(cx *core.Scope, p WindowPointerEvents) { cx.SetWindowPointerEvents(p) }

// SetWindowConstraints limits the window's content size and says whether
// it resizes, minimizes and maximizes; the zero WindowConstraints limits
// nothing. The window moves into its new sizes, and MinimizeWindow and
// ToggleMaximize do nothing where it may not, as its buttons:
//
//	desktop.SetWindowConstraints(cx, desktop.WindowConstraints{MinWidth: 480, MinHeight: 320, NoMaximize: true})
//
// A window sets them as it opens with WindowOptions.Constraints, and the main window with nsgo.WindowConstraints. Linux applies maximum sizes through X11's window manager and filters GTK's native titlebar buttons. Wayland supports fixed sizes, but nonfixed maximums remain unsupported.
func SetWindowConstraints(cx *core.Scope, c WindowConstraints) { cx.SetWindowConstraints(c) }

// OrderFront shows the window, restoring it when minimized or hidden,
// brings it to the front of its level and activates the app.
func OrderFront(cx *core.Scope) { cx.OrderFront() }

// SetMenuBar says when Windows and Linux show the menu bar the window draws for the application menu, which a window with a hidden title bar has none of (SetTitleBar); macOS's is the system's and always shows.
func SetMenuBar(cx *core.Scope, mode MenuBarMode) { cx.SetMenuBar(mode) }
