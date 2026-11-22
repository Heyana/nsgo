//go:build darwin && !ios && nsgotest

package macos

import (
	"os"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The test host: nsgo test -target macos runs each test's app as Run does, its own surface opening its windows, and acts on its main window with events posted to the application's queue, so they take the path a user's do, its key and mouse monitors included.

func init() { testhost.Register(host{}) }

type host struct{}

func (host) Main(run func() int) {
	nsApp := startApp(Options{Title: "NSGo test"}, nil)
	testInputFilter = scripted.contains
	go func() {
		code := run()
		os.Exit(code)
	}()
	nsApp.Send(sel("run"))
}

func (host) Open(app func(cx *core.Scope) core.Node, o testhost.Options) (testhost.Surface, error) {
	var s *surface
	test := &testWindow{options: o}
	onMainSync(func() {
		s = startTest(app, Options{Title: "NSGo test", Width: o.Size.Width, Height: o.Size.Height, Perf: o.FPS}, test)
	})
	return &testSurface{s: s, own: s, test: test}, nil
}

// Several implements testhost.Host: a desktop shows several windows.
func (host) Several() bool { return true }

// OpensWindows implements testhost.WindowHost: the app opens its windows itself, as Run's does.
func (host) OpensWindows(bool) bool { return true }

func (t *testSurface) RequestQuit(id uint32, forced bool) {
	t.own.engine.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: id, Forced: forced})
}

// startTest starts a test's app as startOwn starts Run's, on an own surface with no window, which opens the app's windows at the test's size, the main window's default without one; test is what the test opens it with.
func startTest(app engine.App, opts Options, test *testWindow) *surface {
	if opts.Width <= 0 || opts.Height <= 0 {
		opts.Width, opts.Height = 480, 640
	}
	o := opts
	o.Width, o.Height = 0, 0
	s := newSurface(o, false)
	s.opts = opts
	s.test, s.alerts = test, map[uint32]shownAlert{}
	s.env = s.environment()
	var monitor *engine.PerfMonitor
	if opts.Perf {
		// The first window the app opens shows it (attach).
		monitor = engine.NewPerfMonitor()
		ownPerf = monitor
	}
	eopts := engine.Options{ReadAsset: imageload.ReadAsset, Environment: s.env, NetworkWatcher: apple.NetworkWatcher{}, Perf: monitor, Thread: apple.RaiseThread, Appearance: opts.Appearance,
		Secrets: apple.Keychain{}, Windows: &engine.WindowOptions{Title: opts.Title, Width: opts.Width, Height: opts.Height}}
	eopts.SetDirs(engine.AppDirs(opts.ID))
	eopts.UseTest(test.options, &test.pin)
	s.envPin = eopts.Pin
	imageload.UseDiskCache(eopts.CacheDir)
	s.engine = engine.Start(s, app, eopts)
	observeApp()
	return s
}
