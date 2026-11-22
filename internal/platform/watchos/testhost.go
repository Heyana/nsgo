//go:build ios && nsgotest

package watchos

import (
	"fmt"
	"os"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The test host: nsgo test -target watchos_simulator runs each test in
// the app, launched in a watch simulator, and drives it through
// testSurface (testsurface.go).

func init() { testhost.Register(host{}) }

type host struct{}

func (host) Main(run func() int) {
	hostReady = make(chan struct{})
	ready := hostReady
	s = newSurface(nil, Options{ID: "dev.nsgo.test"})
	s.inputGate.isolated = true
	token, marked := os.LookupEnv("NSGO_TEST_RUN")
	if marked {
		fmt.Printf("nsgo-test-start %s\n", token)
	}
	go func() {
		<-ready
		code := run()
		if marked {
			fmt.Printf("nsgo-test-exit %s %d\n", token, code)
		}
		os.Exit(code)
	}()
	runHost()
}

func (host) Open(root func(cx *core.Scope) core.Node, o testhost.Options) (testhost.Surface, error) {
	test := &testOpen{options: o}
	onMainSync(func() {
		s.app = root
		s.started = true
		s.start(test)
	})
	return &testSurface{test: test}, nil
}

// Several implements testhost.Host: a watch shows one app.
func (host) Several() bool { return false }
