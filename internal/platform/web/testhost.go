//go:build js && wasm && nsgotest

package web

import (
	"errors"
	"os"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The test host: nsgo test -target web runs each test in a page of a
// headless browser it drives over the DevTools protocol. The page asks it
// for trusted input through the nsgoTest binding, which a script of the
// tool answers with nsgoTestReply.

func init() { testhost.Register(host{}) }

type host struct{}

func (host) Main(run func() int) {
	go func() { os.Exit(run()) }()
	select {}
}

func (host) Open(app func(cx *core.Scope) core.Node, o testhost.Options) (testhost.Surface, error) {
	if js.Global().Get("nsgoTest").Type() != js.TypeFunction {
		return nil, errors.New("the page has no nsgoTest binding: run the tests with nsgo test -target web")
	}
	t := &testSurface{}
	if o.Size.Width > 0 {
		if _, err := t.ask(map[string]any{"op": "viewport", "width": o.Size.Width, "height": o.Size.Height}); err != nil {
			return nil, err
		}
	}
	test := &testOpen{options: o}
	t.s, t.test = open(app, Options{}, test), test
	return t, nil
}

// Several implements testhost.Host: it shows one app at a time.
func (host) Several() bool { return false }
