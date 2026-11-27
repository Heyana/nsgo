//go:build windows && nsgotest

package windows

import (
	"errors"
	"os"
	"runtime"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The test host: nsgo test -target windows runs each test in a window of
// its own. The mouse is the window's own messages, posted as the system
// posts them; keys go in at the backend's key entry points, with the
// thread's keyboard state holding the modifiers, as GetKeyState reads
// them.

// TestHost returns the Windows test host. internal/backend registers it so
// packages that import windows through nsgotest for windowless rendering stay
// headless unless they also link github.com/nsgo-dev/nsgo.
func TestHost() testhost.Host { return host{} }

type host struct{}

// Main runs the tests here: each window runs its message loop on a
// thread of its own.
func (host) Main(run func() int) { os.Exit(run()) }

func (host) OpensWindows(app bool) bool { return app }

func (host) Open(app func(cx *core.Scope) core.Node, o testhost.Options) (testhost.Surface, error) {
	test := &testOpen{options: o, ready: make(chan *window, 1), windows: map[uint32]*window{}}
	go func() {
		runtime.LockOSThread()
		Run(app, Options{Title: "NSGo test", Width: o.Size.Width, Height: o.Size.Height, Hidden: o.Windows, test: test})
	}()
	select {
	case w := <-test.ready:
		return &testSurface{w: w, test: test}, nil
	case <-time.After(20 * time.Second):
		return nil, errors.New("the window did not open")
	}
}

// Several implements testhost.Host: a desktop shows several windows.
func (host) Several() bool { return true }
