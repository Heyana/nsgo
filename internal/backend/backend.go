// Package backend runs an app on the platform it is built for, which build tags pick (run_*.go, view_*.go): what nsgo.Run and nsgo.NewView do once they have gathered their options.
package backend

import (
	"log"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/dev"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/restart"
)

// name and id are the app's name, "NSGo" where the tool set none, and ID.
var name, id = appName(), core.App().ID

func appName() string {
	if n := core.App().Name; n != "" {
		return n
	}
	return "NSGo"
}

// Options are what nsgo.Run's and nsgo.NewView's options set.
type Options struct {
	Components core.NativeComponents
	// Fonts are the font files to register, under the app's resources.
	Fonts []string
	// Monitor shows the performance monitor over the app: --nsgo-fps.
	Monitor bool
	// Material is the material behind the main window's content, and Width and Height its size.
	Material      core.WindowMaterial
	Width, Height core.Points
	// Frame is where the main window opens, when it moves.
	Frame core.WindowFrame
	// Constraints limit the main window.
	Constraints core.WindowConstraints
	// Appearance is the appearance the app starts in.
	Appearance core.Appearance
	// Simulate is the OS a web build runs as: --nsgo-os. Other platforms ignore it.
	Simulate core.OS
	// InstanceKey scopes the Windows and Linux instance lock; empty keeps one per app ID.
	InstanceKey string
	// HideDock starts the app out of the Dock (macOS).
	HideDock bool
	// Transparent, NoShadow and SkipTaskbar are the main window's (core.WindowOptions).
	Transparent, NoShadow, SkipTaskbar bool
	// Hidden opens the main window without showing it (nsgo.StartHidden).
	Hidden bool
	// Parent is the window NewView's view is made in (Windows).
	Parent uintptr
}

// Run runs root, a root view, or app, the app's function, and the platform's event loop, under the launch flags and nsgo dev's client, then starts the app again if it asked to restart (desktop.Restart).
func Run(root engine.App, app func(*core.Scope), o Options) {
	restart.Await()
	stopCrashReports := startCrashReports()
	launch.apply(&o)
	o.Constraints = o.Constraints.Normalized()
	if o.Frame.Width > 0 && o.Frame.Height > 0 {
		o.Width, o.Height = o.Frame.Width, o.Frame.Height
	}
	size := o.Constraints.Clamp(core.Size{Width: o.Width, Height: o.Height})
	o.Width, o.Height = size.Width, size.Height
	dev.Start()
	if app != nil {
		run(nil, dev.WrapApp(launch.localizedApp(app)), o)
	} else {
		run(dev.Wrap(launch.localized(root)), nil, o)
	}
	// nsgo dev ends with the app it started, so a restart there only quits.
	if core.Restarting() && !dev.Build {
		if restart.Looping() {
			log.Printf("nsgo: not restarting: the app restarted again within %v of its restart", restart.LoopWindow)
		} else if err := restart.Start(launchArgs); err != nil {
			log.Printf("nsgo: restarting: %v", err)
		}
	}
	// A panic unwinding Run still needs the output file when the runtime prints it.
	stopCrashReports()
}

// View is a backend's embedded view, which nsgo.View wraps.
type View interface {
	Handle() uintptr
	SetHostEnvironment(engine.HostEnvironment)
	SetTokens(core.Tokens)
	Close()
}

// NewView makes the platform's view showing app, for a host that runs the event loop itself.
func NewView(app engine.App, o Options, h engine.HostOptions) View { return newView(app, o, h) }

// appOf returns the app's function of what Run runs: app, or one showing
// root as its main view.
func appOf(root engine.App, app func(*core.Scope)) func(*core.Scope) {
	if app != nil {
		return app
	}
	return func(cx *core.Scope) { cx.SetMainView(root) }
}

// rootOf returns the root view of what Run runs on a platform that opens no windows: root, or app's main view.
func rootOf(root engine.App, app func(*core.Scope)) engine.App {
	if app != nil {
		return core.AppRoot(app)
	}
	return root
}

// hiddenMain returns what Run runs with o.Hidden on Windows and Linux, which open a hidden main window only from the app's function: root as the app's main view.
func hiddenMain(root engine.App, app func(*core.Scope), o Options) (engine.App, func(*core.Scope)) {
	if o.Hidden && app == nil {
		return nil, appOf(root, nil)
	}
	return root, app
}
