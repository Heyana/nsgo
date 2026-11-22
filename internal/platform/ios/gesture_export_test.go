//go:build ios && nsgotest

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// HoldPanForTest gives the pan gate of the view testID a followed touch and a scroll handoff, as a finger down on it holds, without touching the screen. The first closure removes the view's pan as an OnPan that goes away does; the second reports whether the gate still holds either. It reports nil closures when no view testID has a pan.
func HoldPanForTest(testID string) (func(), func() bool) {
	var v *view
	var g *panGate
	var scroll objc.ID
	onMainSync(func() {
		for _, w := range runner.views {
			if w != nil && w.pan != 0 && w.props.TestID == testID {
				v, g = w, gates[w.pan]
				break
			}
		}
		if g == nil {
			return
		}
		g.finger = objc.ID(apple.Class("NSObject")).Send(selNew)
		scroll = objc.ID(apple.Class("UIScrollView")).Send(selNew)
		g.scroll, g.consuming = scroll, true
		handoffs[scroll] = g
	})
	if g == nil {
		return nil, nil
	}
	remove := func() {
		onMainSync(func() {
			v.events &^= 1 << driver.EventPan
			v.setGestures(v.events)
		})
	}
	held := func() bool {
		var held bool
		onMainSync(func() { held = g.finger != 0 || g.scroll != 0 || handoffs[scroll] != nil || pinTop(scroll) })
		return held
	}
	return remove, held
}
