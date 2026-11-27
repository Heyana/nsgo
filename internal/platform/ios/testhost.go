//go:build ios && nsgotest

package ios

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The test host: nsgo test -target ios_simulator runs each test in the
// app, launched in a simulator, and drives it through testSurface
// (testsurface.go).

func init() { testhost.Register(host{}) }

type host struct{}

func (host) Main(run func() int) {
	sceneReady = make(chan struct{})
	runner = newSurface(nil, Options{ID: "dev.nsgo.test"})
	scenes = []*surface{runner}
	testInputFilter = scriptedTestInputAllowed
	// devicectl on an iPhone and simctl in a simulator bridge the app's output but not its exit code: the tool's token marks where this run's output starts and what it exits with, as Android's host marks its log, even when the app ends before simctl attaches to it.
	token, marked := os.LookupEnv("NSGO_TEST_RUN")
	if marked {
		fmt.Printf("nsgo-test-start %s\n", token)
	}
	go func() {
		code := run()
		if marked {
			fmt.Printf("nsgo-test-exit %s %d\n", token, code)
		}
		os.Exit(code)
	}()
	runUIKit(Options{})
}

func (host) Open(root func(cx *core.Scope) core.Node, o testhost.Options) (testhost.Surface, error) {
	select {
	case <-sceneReady:
	case <-time.After(30 * time.Second):
		return nil, errors.New("the scene did not connect")
	}
	test := &testOpen{options: o}
	onMainSync(func() {
		// A previous test may have resized its window. Each test starts with the scene's actual size again.
		space := runner.window.Send(sel("windowScene")).Send(sel("coordinateSpace"))
		runner.window.Send(selSetFrame, objc.Send[apple.CGRect](space, selBounds))
		runner.window.Send(sel("layoutIfNeeded"))
		runner.native.Send(sel("layoutIfNeeded"))
		runner.app = root
		runner.startEngine(test)
	})
	if err := loadHID(); err != nil {
		return nil, err
	}
	if err := awaitActive(); err != nil {
		return nil, err
	}
	surface := &testSurface{s: runner, test: test}
	if onTV() {
		return surface, nil
	}
	return &touchTestSurface{surface}, nil
}

// awaitActive waits for an Apple TV app to become active, as it does a
// moment after launching: only then does UIKit give a view the focus.
func awaitActive() error {
	if !onTV() {
		return nil
	}
	for range 100 {
		var active bool
		onMainSync(func() {
			const stateActive = 0
			app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
			active = objc.Send[int](app, sel("applicationState")) == stateActive
		})
		if active {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("the app did not become active")
}

// Several implements testhost.Host: it shows one app at a time.
func (host) Several() bool { return false }
