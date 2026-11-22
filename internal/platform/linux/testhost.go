//go:build linux && !android && nsgotest

package linux

import (
	"os"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The test host: nsgo test -target linux runs each test in a GTK window.
// GTK 4 takes no made-up events, so input goes in at the backend's own
// entry points, where GTK's controllers hand it over, and text through
// the focused field's own signals.

func init() { testhost.Register(&host{opens: make(chan openRequest)}) }

type openRequest struct {
	app   func(cx *core.Scope) core.Node
	opts  Options
	reply chan openReply
}

type openReply struct {
	w   *window
	err error
}

type host struct{ opens chan openRequest }

// Main waits on this thread for the test's app, then runs GTK there.
func (h *host) Main(run func() int) {
	go func() { os.Exit(run()) }()
	req := <-h.opens
	w, err := start(req.app, req.opts)
	req.reply <- openReply{w, err}
	if err != nil {
		select {}
	}
	// A test's later apps open their windows on the first one's loop.
	go func() {
		for req := range h.opens {
			w.post(func() {
				o, err := start(req.app, req.opts)
				req.reply <- openReply{o, err}
			})
		}
	}()
	w.run()
	select {}
}

func (*host) OpensWindows(app bool) bool { return app }

func (h *host) Open(app func(cx *core.Scope) core.Node, o testhost.Options) (testhost.Surface, error) {
	test := &testOpen{options: o, windows: map[uint32]*window{}}
	reply := make(chan openReply, 1)
	h.opens <- openRequest{app: app, opts: Options{Title: "NSGo test", Width: o.Size.Width, Height: o.Size.Height, Hidden: o.Windows, test: test}, reply: reply}
	r := <-reply
	if r.err != nil {
		return nil, r.err
	}
	return &testSurface{w: r.w}, nil
}

// Several implements testhost.Host: a desktop shows several windows.
func (*host) Several() bool { return true }
