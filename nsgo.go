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
	"github.com/oneo-me/nsgo/engine"
)

// Set by the nsgo tool with -ldflags -X.
var (
	name   = "NSGo"
	id     = ""
	width  = "480"
	height = "640"
)

// App builds an app's root element once.
type App = engine.App

// Run shows app and runs the platform's event loop. Call it from main; it
// does not return while the app runs.
func Run(app App, opts ...Option) {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	run(app, o)
}

// Option configures Run.
type Option func(*options)

type options struct {
	components core.NativeComponents
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
