//go:build linux && !android && nsgotest

package linux

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

var translateWindowOrigin = sync.OnceValue(func() func(uintptr, uintptr, uintptr, int32, int32, *int32, *int32, *uintptr) int32 {
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil
	}
	var translate func(uintptr, uintptr, uintptr, int32, int32, *int32, *int32, *uintptr) int32
	purego.RegisterLibFunc(&translate, lib, "XTranslateCoordinates")
	return translate
})

func (t *testSurface) WindowState() (testhost.WindowState, error) {
	type result struct {
		state testhost.WindowState
		err   error
	}
	done := make(chan result, 1)
	t.onLoop(func() {
		w := t.w
		if w.win == 0 || w.embedded || w.primary && w.opts.Hidden {
			done <- result{err: errors.New("the surface has no desktop window")}
			return
		}
		var width, height int32
		gtkWindowGetDefaultSize(w.win, &width, &height)
		if allocated := gtkWidgetGetWidth(w.win); allocated > 0 {
			width = allocated
		}
		if allocated := gtkWidgetGetHeight(w.win); allocated > 0 {
			height = allocated
		}
		frame := driver.WindowFrame{Width: driver.Points(width), Height: driver.Points(height)}
		if drawsOnX11() {
			if display, xid, ok := w.x11Window(); ok {
				if translate := translateWindowOrigin(); translate != nil {
					var x, y int32
					var child uintptr
					if translate(display, xid, x11LevelRoot(display), 0, 0, &x, &y, &child) != 0 {
						scale := driver.Points(max(gtkWidgetGetScaleFactor(w.win), 1))
						frame.X, frame.Y, frame.Move = driver.Points(x)/scale, driver.Points(y)/scale, true
					}
				}
			}
		}
		done <- result{state: testhost.WindowState{Frame: frame, Visible: gtkWidgetGetVisible(w.win) != 0, Focused: gtkWindowIsActive(w.win) != 0}}
	})
	select {
	case r := <-done:
		return r.state, r.err
	default:
		return testhost.WindowState{}, errors.New("the GTK thread did not answer")
	}
}
