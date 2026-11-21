//go:build android && nsgotest

package android

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The test host: nsgo test -target android starts the app's activity for
// each test, the test's arguments and environment in its intent. The
// runtime's test agent (TestAgent.java) acts on the UI thread with
// MotionEvents and KeyEvents, asked over the pipe. The process prints to
// standard error, which the runtime logs, and says how it exits there.

func init() { testhost.Register(&host{}) }

type host struct {
	s    *surface
	test *testOpen
}

func (h *host) Main(run func() int) {
	h.s = connect(Options{})
	h.test = &testOpen{size: make(chan driver.Size, 1)}
	// The device's log keeps earlier runs: the tool reads from the start
	// of this one, which its token marks.
	token := os.Getenv("NSGO_TEST_RUN")
	fmt.Fprintf(os.Stderr, "nsgo-test-start %s\n", token)
	go func() {
		code := run()
		fmt.Fprintf(os.Stderr, "nsgo-test-exit %s %d\n", token, code)
		os.Exit(code)
	}()
	h.s.serve(nil, Options{}, h.test)
	os.Exit(1)
}

func (h *host) Open(app func(cx *core.Scope) core.Node, o testhost.Options) (testhost.Surface, error) {
	h.test.options = o
	var size driver.Size
	select {
	case size = <-h.test.size:
	case <-time.After(30 * time.Second):
		return nil, errors.New("the runtime did not start")
	}
	t := newTestSurface(h.s, h.test)
	h.s.startEngine(app, size, h.test.geometry, Options{}, h.test)
	return t, nil
}

// Several implements testhost.Host: it shows one app at a time.
func (*host) Several() bool { return false }
