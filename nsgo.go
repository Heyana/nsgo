// Package nsgo runs an NSGo app on the platform it is built for, either
// a root view, which shows in the app's main window:
//
//	func main() { nsgo.Run(Root) }
//
// or the app's function, which opens its windows, panels and tray icons
// itself (package desktop), as an Avalonia application does:
//
//	func main() { nsgo.Run(App) }
//
//	func App(app *core.Scope) {
//		desktop.SetShutdownMode(app, desktop.ShutdownOnExplicit)
//		desktop.NewTray(app, desktop.TrayOptions{Icon: icon, Panel: desktop.OpenPanel(app, desktop.PanelOptions{}, Panel)})
//	}
//
// Build tags pick the backend. The nsgo tool builds the app's identity
// into it (core.App), and its updates where nsgo.yaml configures them
// (package updater), from nsgo.yaml. The engine's own command-line
// flags, such as --nsgo-fps, leave os.Args before main runs (launchFlags).
package nsgo

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/backend"
	"github.com/nsgo-dev/nsgo/internal/dev"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// appearance is nsgo.yaml's, set by the nsgo tool with -ldflags -X: "light" or "dark" starts the app in it.
var appearance = ""

// startAppearance is the appearance the app starts in before its options: nsgo.yaml's.
func startAppearance() core.Appearance {
	return map[string]core.Appearance{"light": core.AppearanceLight, "dark": core.AppearanceDark}[appearance]
}

// App builds an app's root element once.
type App = engine.App

// Main is what Run runs: a root view, or the app's function.
type Main interface {
	func(cx *core.Scope) core.Node | func(app *core.Scope)
}

// Run runs main and the platform's event loop. Call it from main.
//
// A root view shows in the app's main window, or fills the app on phones and the web. The app's function runs once, in the app's own scope, and opens what the app shows: on macOS, Windows and Linux its windows, panels and tray icons (package desktop); everywhere the view SetMainView sets. Where the platform opens no windows of the app's (phones and the web, Windows when Run is given a root view, and Linux when it is given a root view and no automatic update checks) desktop.OpenWindow returns nil.
//
// On desktops Run returns once the app ends, by its shutdown mode
// (desktop.SetShutdownMode), core.Scope.Quit or desktop.Restart, its
// storage written, so main can clean up; on iOS, Android and the web it
// never returns. After desktop.Restart it has started the app again,
// which waits for this process to exit.
func Run[M Main](main M, opts ...Option) {
	o := options{platform: backend.Options{Width: 480, Height: 640, Appearance: startAppearance()}}
	for _, fn := range opts {
		fn(&o)
	}
	switch m := any(main).(type) {
	case func(cx *core.Scope) core.Node:
		backend.Run(m, nil, o.platform)
	case func(app *core.Scope):
		backend.Run(nil, m, o.platform)
	}
}

// SetMainView shows view as the app's main view: in its main window on
// desktops, which it opens, else filling the app. A later call replaces
// the view.
func SetMainView(cx *core.Scope, view func(cx *core.Scope) core.Node) {
	if view != nil {
		view = dev.Wrap(view)
	}
	cx.SetMainView(view)
}

// Option configures Run and NewView.
type Option func(*options)

type options struct {
	// platform is what the backend takes.
	platform backend.Options
	// What NewView takes besides: the data directory, the error handler
	// and the host channel.
	dataDir string
	onError func(error)
	host    *core.Host
}

// Appearance starts the app in appearance whatever the system's, its
// window and native chrome included, as core.Scope.SetAppearance later
// changes it.
func Appearance(appearance core.Appearance) Option {
	return func(o *options) { o.platform.Appearance = appearance }
}

// WindowSize sizes the main window's content, in points: 480 by 640 by
// default. desktop.OpenWindow's windows take it where they set no size.
func WindowSize(width, height core.Points) Option {
	return func(o *options) { o.platform.Width, o.platform.Height = width, height }
}

// InstanceKey scopes the single-instance lock on Windows and Linux. Runs with the same app ID and key hand their launches to one another; a different key has its own process and handoff channel. An empty key keeps the default one instance for the app ID. macOS and other targets ignore it.
func InstanceKey(key string) Option {
	return func(o *options) { o.platform.InstanceKey = key }
}

// LaunchFiles replaces Run's command-line file scan on Windows and Linux.
// By default the scan takes existing regular files (including local file
// URLs) before the first "--" and delivers their absolute paths to
// desktop.OnOpenFiles. It does not interpret the app's flags or subcommands.
//
// When there are arguments to classify, parse gets a copy of the arguments after the engine removes its flags, excluding the executable name, "--" and everything after it. Local file URLs, as a Linux desktop entry passes them, are already paths; links among the arguments are included on Linux.
//
// Return the filesystem paths to deliver, not URLs. Relative paths are resolved against the launching process's working directory.
//
// A nil parser disables command-line file delivery, including Open With and recent-file choices on Windows and Linux, and a parser returning nil opens no files for that launch. The launching process makes this choice even when it hands the result to a running instance.
//
// Links are still read on both sides of "--". On Windows, a launch with
// a link opens no files and never calls parse. Other platforms ignore this
// option.
func LaunchFiles(parse func(args []string) []string) Option {
	if parse == nil {
		parse = func([]string) []string { return nil }
	}
	return func(o *options) { o.platform.LaunchFiles = parse }
}

// WindowFrame opens the main window with frame f, as desktop.Frame
// reported it, instead of centered at WindowSize: an app restores its
// window with it. A window no screen would show, as on a screen that
// went away, opens centered on the main screen. Linux takes only the size.
func WindowFrame(f core.WindowFrame) Option {
	return func(o *options) { o.platform.Frame = f }
}

// StartHidden opens the main window on macOS, Windows and Linux without showing it, as a tray app or one launched at login (desktop.LaunchedAtLogin) does: the app shows it with core.Scope.MainWindow's Show. Phones and the web show the app as they launch it.
func StartHidden() Option {
	return func(o *options) { o.platform.Hidden = true }
}

// WindowConstraints limits the main window's content size and what the
// user may do to it: desktop.SetWindowConstraints says how, and changes
// them later. desktop.OpenWindow's windows take their own from
// WindowOptions.
func WindowConstraints(c core.WindowConstraints) Option {
	return func(o *options) { o.platform.Constraints = c }
}

// HideDock starts the app out of the Dock and the app switcher, on
// macOS, as a tray or background app runs: set before the app starts,
// its icon never shows there. desktop.SetDockVisible changes it later.
func HideDock() Option {
	return func(o *options) { o.platform.HideDock = true }
}

// WindowMaterial starts the main window with material behind its content,
// where the app draws nothing opaque: a system vibrancy material on
// macOS and a system backdrop on Windows 11 22H2 or later. Other targets
// ignore it. desktop.SetWindowMaterial changes it later, which on
// Windows needs a window started with one.
func WindowMaterial(material core.WindowMaterial) Option {
	return func(o *options) { o.platform.Material = material }
}

// WindowTransparent opens the main window without a background or frame, so what is behind it shows wherever the app draws nothing, on macOS, Windows and Linux: desktop.WindowOptions.Transparent says how, desktop.SetWindowTransparent changes it later, and desktop.CanMakeWindowsTransparent says whether what is behind the window shows.
func WindowTransparent() Option {
	return func(o *options) { o.platform.Transparent = true }
}

// WindowNoShadow opens the main window without the system's shadow (desktop.WindowOptions.NoShadow); desktop.SetWindowShadow changes it later.
func WindowNoShadow() Option {
	return func(o *options) { o.platform.NoShadow = true }
}

// SkipTaskbar keeps the main window off the Windows taskbar and Alt+Tab, off the task list on X11, and out of macOS's Window menu and Cmd+` (desktop.WindowOptions.SkipTaskbar); desktop.SetSkipTaskbar changes it later, and desktop.CanSkipTaskbar says whether it takes effect.
func SkipTaskbar() Option {
	return func(o *options) { o.platform.SkipTaskbar = true }
}

// Fonts registers the font files at paths, slash-separated under the
// app's resources directory (TrueType, OpenType or collections), before
// the first view: text names them by their families, as Font.Family or a
// Style's FontFamily.
func Fonts(paths ...string) Option {
	return func(o *options) { o.platform.Fonts = append(o.platform.Fonts, paths...) }
}

// Component registers a native component the app's Native elements name:
// factory makes the platform view for each one. Register the components
// of each platform in files built for it.
func Component(name string, factory core.NativeFactory) Option {
	return func(o *options) {
		if o.platform.Components == nil {
			o.platform.Components = core.NativeComponents{}
		}
		o.platform.Components[name] = factory
	}
}
