// Package nsgo runs an NSGo app on the platform it is built for:
//
//	func main() { nsgo.Run(Root) }
//
// Build tags pick the backend. The nsgo tool sets the app's name, ID and
// window size from nsgo.yaml.
package nsgo

import (
	"strconv"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/internal/engine"
)

// Set by the nsgo tool with -ldflags -X.
var (
	name   = "NSGo"
	id     = ""
	width  = "480"
	height = "640"
	fps    = "0"
	// appearance is nsgo.yaml's: "light" or "dark" starts the app in it.
	appearance = ""
)

// App builds an app's root element once.
type App = engine.App

// Run shows app and runs the platform's event loop. Call it from main. On
// macOS, Windows and Linux it returns once the app ends, its window
// closed or core.Scope.Quit called, its storage written, so main can
// clean up; on iOS, Android and the web it never returns.
func Run(app App, opts ...Option) {
	o := options{monitor: fps == "1", appearance: map[string]core.Appearance{
		"light": core.AppearanceLight, "dark": core.AppearanceDark}[appearance]}
	for _, fn := range opts {
		fn(&o)
	}
	run(devApp(app), o)
}

// Option configures Run and NewView.
type Option func(*options)

type options struct {
	components core.NativeComponents
	// fonts are the font files to register, under the app's resources.
	fonts []string
	// monitor shows the performance monitor over the app (macOS).
	monitor bool
	// material is the material behind the window's content.
	material core.WindowMaterial
	// appearance is the appearance the app starts in.
	appearance core.Appearance
	// keepAlive keeps the app running when its window closes (macOS).
	keepAlive bool
	// What NewView takes: the parent window, the data directory, the
	// error handler and the host channel.
	parent  uintptr
	dataDir string
	onError func(error)
	host    *core.Host
}

// Appearance starts the app in appearance whatever the system's, its
// window and native chrome included, as core.Scope.SetAppearance later
// changes it.
func Appearance(appearance core.Appearance) Option {
	return func(o *options) { o.appearance = appearance }
}

// KeepAliveOnClose keeps the app running when the user closes its
// window, on macOS: the window hides, keeping its state, and clicking
// the app in the Dock shows it again. Quitting, from the app menu or
// with core.Scope.Quit, still ends it. Other platforms end the app with
// its window.
func KeepAliveOnClose(on bool) Option {
	return func(o *options) { o.keepAlive = on }
}

// FPS shows (or hides) the performance monitor over the app: frame rate,
// frame and commit costs, CPU, GPU and memory. `nsgo dev` turns it on for
// development runs; `nsgo build` never does.
func FPS(on bool) Option {
	return func(o *options) { o.monitor = on }
}

// WindowMaterial sets the material behind the window's content, where the
// app draws nothing opaque: a system vibrancy material on macOS and a
// system backdrop on Windows 11 22H2 or later. Other targets ignore it.
// The window keeps it for its life.
func WindowMaterial(material core.WindowMaterial) Option {
	return func(o *options) { o.material = material }
}

// Fonts registers the font files at paths, slash-separated under the
// app's resources directory (TrueType, OpenType or collections), before
// the first view: text names them by their families, as Font.Family or a
// Style's FontFamily.
func Fonts(paths ...string) Option {
	return func(o *options) { o.fonts = append(o.fonts, paths...) }
}

// Component registers a native component the app's Native elements name:
// factory makes the platform view for each one. Register the components
// of each platform in files built for it.
func Component(name string, factory core.NativeFactory) Option {
	return func(o *options) {
		if o.components == nil {
			o.components = core.NativeComponents{}
		}
		o.components[name] = factory
	}
}

func windowSize() (core.Points, core.Points) {
	w, _ := strconv.ParseFloat(width, 32)
	h, _ := strconv.ParseFloat(height, 32)
	return core.Points(w), core.Points(h)
}
