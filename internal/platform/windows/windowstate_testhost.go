//go:build windows && nsgotest

package windows

import (
	"errors"

	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func (t *testSurface) WindowState() (testhost.WindowState, error) {
	type result struct {
		state testhost.WindowState
		err   error
	}
	done := make(chan result, 1)
	t.onMain(func() {
		w := t.w
		if w.hwnd == 0 || w.embedded || w.opts.Hidden && w.primary {
			done <- result{err: errors.New("the surface has no desktop window")}
			return
		}
		visible, _, _ := isWindowVisible.Call(w.hwnd)
		foreground, _, _ := getForegroundWindow.Call()
		done <- result{state: testhost.WindowState{Frame: w.windowFrame(), Visible: visible != 0, Focused: foreground == w.hwnd}}
	})
	select {
	case r := <-done:
		return r.state, r.err
	default:
		return testhost.WindowState{}, errors.New("the window thread did not answer")
	}
}
