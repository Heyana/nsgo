//go:build darwin && !ios && (nsgotest || nsgoautomation)

package macos

import (
	"errors"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func (t *testSurface) WindowState() (testhost.WindowState, error) {
	var state testhost.WindowState
	var err error
	onMainSync(func() {
		t.follow()
		if t.s.window == 0 || t.s.embedded {
			err = errors.New("the surface has no desktop window")
			return
		}
		state = testhost.WindowState{
			Frame:   t.s.windowFrame(),
			Visible: objc.Send[bool](t.s.window, sel("isVisible")),
			Focused: objc.Send[bool](t.s.window, sel("isKeyWindow")),
		}
	})
	return state, err
}
