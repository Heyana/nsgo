//go:build ios && nsgotest

package ios_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestRotationPublishesSizeAndSafeAreaTogether observes every reactive update,
// including those during UIKit's transition, rather than only its final layout.
func TestRotationPublishesSizeAndSafeAreaTogether(t *testing.T) {
	type observation struct {
		size core.Size
		safe core.Edges
	}
	var mu sync.Mutex
	var observed []observation
	var scope *core.Scope
	var phone bool
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		phone = cx.Env().OS() == core.OSIOS && cx.Env().Idiom() == core.IdiomPhone
		cx.RequestOrientation(core.OrientationPortrait)
		cx.Effect(func() {
			value := observation{cx.Env().Window(), cx.Env().SafeArea()}
			mu.Lock()
			observed = append(observed, value)
			mu.Unlock()
		})
		return core.View{TestID: "probe", Style: core.Style{FlexGrow: 1}}
	})
	app.WaitFor("id:probe", 5*time.Second)
	if !phone {
		t.Skip("rotation acceptance needs an iPhone surface")
	}
	latest := func() observation {
		mu.Lock()
		defer mu.Unlock()
		return observed[len(observed)-1]
	}
	rotate := func(mask core.Orientation, portrait bool) observation {
		t.Helper()
		app.Post(func() { scope.RequestOrientation(mask) })
		app.WaitUntil(5*time.Second, func() bool {
			size := latest().size
			return size.Width > 0 && size.Height > 0 && (size.Height > size.Width) == portrait
		})
		// Let the native transition finish while the effect continues recording.
		until := time.Now().Add(500 * time.Millisecond)
		app.WaitUntil(time.Second, func() bool { return time.Now().After(until) })
		return latest()
	}
	portrait := rotate(core.OrientationPortrait, true)
	mu.Lock()
	observed = []observation{portrait}
	mu.Unlock()
	left := rotate(core.OrientationLandscapeLeft, false)
	rotate(core.OrientationPortrait, true)
	right := rotate(core.OrientationLandscapeRight, false)
	if portrait.safe == left.safe || portrait.safe == right.safe {
		t.Skip("this surface has no orientation-dependent safe area")
	}
	for range 2 {
		rotate(core.OrientationPortrait, true)
		rotate(core.OrientationLandscapeLeft, false)
		rotate(core.OrientationPortrait, true)
		rotate(core.OrientationLandscapeRight, false)
	}
	rotate(core.OrientationPortrait, true)
	mu.Lock()
	defer mu.Unlock()
	allowed := []observation{portrait, left, right}
	for i, value := range observed {
		if !slices.Contains(allowed, value) {
			t.Errorf("effect %d saw mixed size/safe area: %+v; settled observations: %+v", i, value, allowed)
		}
	}
	t.Logf("%d effect observations across 12 rotations; portrait=%+v, left=%+v, right=%+v", len(observed), portrait, left, right)
}
