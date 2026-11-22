package desktop

import "github.com/nsgo-dev/nsgo/core"

// An app run with nsgo.Run's app function opens what it shows itself, as
// an Avalonia application does: windows, panels, tray icons and global
// shortcuts, on macOS. Each belongs to the owner that opened it and goes
// with it; the windows share the app's signals. Where the platform opens
// no windows of the app's, the functions that open one return nil, whose
// methods do nothing, and the app shows its main view
// (nsgo.SetMainView).

type (
	// Window is a window or a panel the app opened.
	Window = core.Window
	// WindowOptions are a window's title, size, constraints, key and where it takes the pointer.
	WindowOptions = core.WindowOptions
	// PanelOptions are a panel's size, whether it takes its content's, whether it hides as it loses the focus, whether it never takes it, whether it draws anything of its own, and where it takes the pointer.
	PanelOptions = core.PanelOptions
	// WindowClosing is a close the user asked for, which
	// Window.OnClosing may cancel.
	WindowClosing = core.WindowClosing
	// ShutdownMode says when the app ends by itself.
	ShutdownMode = core.ShutdownMode
	// QuitRequest is a user or system request to quit. Forced reports a system session ending.
	QuitRequest = core.QuitRequest
	// Tray is a tray icon: in the macOS menu bar, or a Linux StatusNotifierItem.
	Tray = core.Tray
	// TrayOptions are a tray icon's image or view, and what its clicks
	// show.
	TrayOptions = core.TrayOptions
)

const (
	ShutdownOnLastWindowClose = core.ShutdownOnLastWindowClose
	ShutdownOnMainWindowClose = core.ShutdownOnMainWindowClose
	ShutdownOnExplicit        = core.ShutdownOnExplicit
)

// OpenWindow opens a window showing content, which belongs to cx and closes
// when cx goes — not the batch a build owns while it runs this code, so a
// window opened from a click outlives the rebuild that click causes. A window with
// opts.Key already open shows and takes the focus instead:
//
//	settings := desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Settings", Key: "settings"}, Settings)
func OpenWindow(cx *core.Scope, opts WindowOptions, content func(cx *core.Scope) core.Node) *Window {
	return cx.OpenWindow(opts, content)
}

// OpenPanel opens a hidden panel showing content: a borderless window above the others, which takes the keyboard without activating the app, for Window.Toggle, a global shortcut or a tray icon to show, as a launcher or a tray icon's panel does. Like OpenWindow, the panel belongs to cx itself rather than the running owner. With PanelOptions.NoActivate it never takes the keyboard, for Window.ShowAt to place beside another app's text caret, as an input method's candidate window does:
//
//	candidates := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 320, Height: 40, NoActivate: true}, Candidates)
//	candidates.ShowAt(caret, core.PlaceBottom)
//
// A popup card draws itself on a Transparent panel, which shows nothing around it, sized to it with FitContent, up to Width and Height:
//
//	lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, FitContent: true, Transparent: true, HideOnBlur: true}, Card)
func OpenPanel(cx *core.Scope, opts PanelOptions, content func(cx *core.Scope) core.Node) *Window {
	return cx.OpenPanel(opts, content)
}

// CurrentWindow returns the window cx is in, nil in the app's own scope.
func CurrentWindow(cx *core.Scope) *Window { return cx.CurrentWindow() }

// SetShutdownMode says when the app ends by itself: as its last window
// closes (the default), as its main window closes, or only by
// core.Scope.Quit, as a tray app does.
func SetShutdownMode(cx *core.Scope, mode ShutdownMode) { cx.SetShutdownMode(mode) }

// SetMainWindow makes w the main window: the one
// ShutdownOnMainWindowClose follows and reopening the app shows. The
// window nsgo.SetMainView opens is main.
func SetMainWindow(cx *core.Scope, w *Window) { cx.SetMainWindow(w) }

// MainWindow returns the main window, nil once it closed.
func MainWindow(cx *core.Scope) *Window { return cx.MainWindow() }

// OnReopen runs fn when the user opens the app again while it runs: by
// clicking it in the Dock or launching it a second time. Without a
// handler the main window shows and takes the focus.
func OnReopen(cx *core.Scope, fn func()) { cx.OnReopen(fn) }

// OnQuitRequest runs fn when the user chooses Quit from a desktop menu, or the system ends the session. On Windows and Linux, closing a window also asks when its close would end the app. Handlers run newest first and may cancel or defer a user quit. Check QuitRequest.Forced before asking the user for a decision. core.Scope.Quit and Restart quit without asking.
//
//	desktop.OnQuitRequest(cx, func(q *desktop.QuitRequest) {
//		if q.Forced() || !dirty.Get() {
//			return
//		}
//		done := q.Defer()
//		base.ShowAlertDialog(cx, discard, func(label string) { done(label == "Discard") })
//	})
func OnQuitRequest(cx *core.Scope, fn func(q *QuitRequest)) { cx.OnQuitRequest(fn) }

// RequestQuit asks the app to quit as a desktop Quit menu item does.
func RequestQuit(cx *core.Scope) {
	if IsAvailable(cx) {
		cx.RequestQuit()
	}
}

// OnOpenFiles runs fn with the paths of files the system asks the app to
// open: the file types nsgo.yaml's file_types declares, opened from the
// Finder, the Dock or Open With, on Windows and Linux the files named
// on the command line, and on iOS temporary copies imported through the
// scene delegate. Move iOS imports into cx.DataDir() to preserve them. Files the
// app launched to open wait for the first handler.
func OnOpenFiles(cx *core.Scope, fn func(paths []string)) { cx.OnOpenFiles(fn) }

// OnOpenLinks runs fn with links of the app's own URL schemes, which nsgo.yaml's url_schemes declares and the macOS, Windows and Linux packages register, that the system asks the app to open: clicked in a browser or another app, or opened with open, start or xdg-open. They reach the running app, on Windows and Linux through its one-instance claim, which brings its window to the front if it shows and does not run OnReopen; the link the app launched to open waits for the first handler. On Windows and Linux any link the app is started with arrives, as a development build registers no scheme, so check the scheme; on Windows a launch that names a link opens no files with it, as the shell does not escape a link's quotes. A link carries whatever the page that made it chose, so treat its contents as untrusted input and confirm with the user before acting on it.
func OnOpenLinks(cx *core.Scope, fn func(links []string)) { cx.OnOpenLinks(fn) }

// NewTray adds a tray icon, which cx keeps: an image, or a
// view that draws it and may change, a panel its primary click shows
// under it, and a menu its secondary click shows. Title shows beside
// either image or view on macOS. Read Tray.ClickBounds in OnClick or
// Menu to place a custom panel with Window.ShowAt where bounds are available.
func NewTray(cx *core.Scope, opts TrayOptions) *Tray { return cx.NewTray(opts) }

// HotKeyState is what the platform answered a global shortcut's registration.
type HotKeyState = core.HotKeyState

const (
	// HotKeyPending waits for the platform's answer.
	HotKeyPending = core.HotKeyPending
	// HotKeyRegistered is a shortcut the platform fires.
	HotKeyRegistered = core.HotKeyRegistered
	// HotKeyTaken is a combination another registration holds: the app's own, or another app's on Windows and X11; on macOS another app's only when it registered exclusively; on Wayland one the compositor or the user did not bind.
	HotKeyTaken = core.HotKeyTaken
	// HotKeyUnsupported is a key the platform cannot make a global shortcut, or a platform or session with none.
	HotKeyUnsupported = core.HotKeyUnsupported
)

// HotKey is a global shortcut's registration, whose State follows the platform's answers.
type HotKey = core.HotKey

// RegisterHotKey runs fn when key, in core.Scope.OnKey's form such as "alt+space", is pressed whichever app has the focus, while the current owner lives. The handle's State is Pending until the platform answers, then Registered, Taken or Unsupported, and Pending again while a change to the app's shortcuts registers them anew. Global shortcuts work on macOS, on Linux and, for an app run with nsgo.Run(App), on Windows; elsewhere, nsgo.NewView's hosts included, the handle is Unsupported. Linux uses X11 passive grabs, and its compositor's global-shortcut portal on Wayland, where the shortcut is Registered once the user accepts the binding, and Taken when the binding leaves it out.
func RegisterHotKey(cx *core.Scope, key string, fn func()) *HotKey { return cx.RegisterHotKey(key, fn) }

// SetDockVisible shows the app in the Dock and the app switcher, or
// leaves it to its tray icon and windows, on macOS; nsgo.HideDock starts
// it hidden.
func SetDockVisible(cx *core.Scope, on bool) { cx.SetDockVisible(on) }

// AddRecentFile records path as a document the user opened, which the system offers again: in the Dock menu and File › Open Recent on macOS, for the types nsgo.yaml's file_types declares, and in the Recent category of the Jump List on Windows, for the types the app is registered to open. Choosing one reaches OnOpenFiles. Elsewhere it does nothing.
func AddRecentFile(cx *core.Scope, path string) { cx.AddRecentFile(path) }

// ClearRecentFiles forgets the documents AddRecentFile recorded, the app's alone.
func ClearRecentFiles(cx *core.Scope) { cx.ClearRecentFiles() }

// Restart ends the app as core.Scope.Quit does, and nsgo.Run starts it
// again with the arguments it started with once it has exited, on macOS,
// Windows and Linux, so a setting read at launch takes effect. Under nsgo
// dev, and in a view a host embeds, it only quits.
func Restart(cx *core.Scope) { cx.Restart() }

// RequestRestart asks the OnQuitRequest handlers before restarting. A
// canceled request leaves the app running; a deferred one waits for its answer.
func RequestRestart(cx *core.Scope) {
	if IsAvailable(cx) {
		cx.RequestRestart()
	}
}

// RevealFile opens the file manager with path selected.
func RevealFile(cx *core.Scope, path string, done func(error)) { cx.RevealFile(path, done) }

// TrashFile moves path to the user's recoverable Trash or Recycle Bin. It never deletes permanently: where the item cannot be recycled, done gets core.ErrNoTrash and the item stays.
func TrashFile(cx *core.Scope, path string, done func(trashedAt string, err error)) {
	cx.TrashFile(path, done)
}

// SetLaunchAtLogin has the system start the app as the user logs in, or no more. macOS takes only an app in a bundle, as nsgo build makes it: SMAppService registers it on macOS 13 and later, and a per-user LaunchAgent, which takes effect at the next login, on macOS 11 and 12. Windows uses the user's Run key, or StartupTask in an MSIX; Linux writes an XDG entry, uses Snap's declared autostart file, or requests Flatpak's Background portal. A portal may ask for consent, and Windows does not override a startup task disabled by the user or policy. A failure is logged.
func SetLaunchAtLogin(cx *core.Scope, on bool) { cx.SetLaunchAtLogin(on) }

// LaunchAtLogin calls done with whether the system starts the app at login: false where registration is unavailable, and on Windows while Startup apps or policy disables it. On Flatpak it reports only the last Background portal reply in this run, false before any reply: that portal has no read-only autostart query.
func LaunchAtLogin(cx *core.Scope, done func(on bool)) { cx.LaunchAtLogin(done) }

// LaunchedAtLogin reports whether the system started this run as a login item, as a tray app reads it to open no window then. It is known when the app's function first runs and stays the same for the run, reopens included; a restart (Restart) reports false. macOS reads the launch Apple event, or the LaunchAgent's marker on macOS 11 and 12, and Windows and Linux the marker their login entries start the app with, or the StartupTask activation kind in an MSIX; phones and the web report false. Tests set it with nsgotest.Environment(core.Environment{LaunchedAtLogin: true}).
func LaunchedAtLogin(cx *core.Scope) bool { return cx.Env().LaunchedAtLogin() }

// FrontmostAppInfo is the application in front on the desktop: its ID, its name, its process ID and whether its frontmost window is in full screen.
type FrontmostAppInfo = core.FrontmostAppInfo

// FrontmostApp returns the application in front on the desktop, the app itself included, tracked: an effect or binding reading it runs again when another app comes to the front or the one in front enters or leaves full screen.
//
//	front := desktop.FrontmostApp(cx)
//	coding := front.ID == "com.microsoft.VSCode" && !front.FullScreen
//
// It is zero where CanReadFrontmostApp is false. Tests set it with nsgotest.Environment(core.Environment{FrontmostApp: ..., CanReadFrontmostApp: true}).
func FrontmostApp(cx *core.Scope) FrontmostAppInfo { return cx.FrontmostApp() }

// CanReadFrontmostApp reports whether FrontmostApp follows the application in front, tracked: on macOS, Windows and Linux on X11, but not on Wayland, which has no portable way to tell, on phones, the web or mini programs, or for code whose Access refuses core.AccessFrontmostApp, as it says what else the user runs.
func CanReadFrontmostApp(cx *core.Scope) bool { return cx.CanReadFrontmostApp() }
