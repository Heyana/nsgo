//go:build ios && !tvos && nsgotest

package ios_test

import (
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A native iOS tab bar reports its natural height again when its placement or safe area changes, including when only an ancestor moves, and restores its base height when moved clear of the bottom safe area (#1529).
func TestTabBarNativeInsetsFollowPlacement(t *testing.T) {
	var move func(int)
	var safe atomic.Uint32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		placement := core.NewSignal(cx, 0)
		move = func(at int) { cx.Post(func() { placement.Set(at) }) }
		env := cx.Env()
		return core.View{TestID: "root", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}, Children: []core.Node{
			core.View{StyleFunc: func() core.Style {
				bottom := env.SafeArea().Bottom
				safe.Store(math.Float32bits(float32(bottom)))
				st := core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0)}
				switch placement.Get() {
				case 1:
					st.Bottom = core.Pt(0)
				case 2:
					st.Bottom = core.Pt(bottom)
				case 3:
					st.Bottom = core.Pt(bottom / 2)
				default:
					st.Top = core.Pt(60)
				}
				return st
			}, Children: []core.Node{core.TabBar{TestID: "bar", Items: []core.TabBarItem{
				{Title: "Home", Icon: "house"},
				{Title: "Search", Icon: "magnifyingglass"},
			}}}},
		}}
	})
	app.Settle()
	base := app.Find("id:bar").Bounds().Size.Height
	if base <= 0 {
		t.Fatalf("base tab bar height = %v, want > 0", base)
	}
	inset := core.Points(math.Float32frombits(safe.Load()))
	if inset <= 0 {
		t.Skip("requires a bottom safe area inset")
	}
	// A classic UITabBar grows by the bottom safe area it overlaps; iOS 26's floating bar answers sizeThatFits: with one height at every placement, so there the bar keeps its base height. Either way the laid-out height is UIKit's natural height at the new placement.
	grows := false
	for _, at := range []int{1, 2, 3, 0} {
		move(at)
		app.Settle()
		inset = core.Points(math.Float32frombits(safe.Load()))
		if at == 1 {
			app.WaitUntil(5*time.Second, func() bool {
				fit := core.Points(ios.NativeTabBarFit())
				return fit > 0 && near(app.Find("id:bar").Bounds().Size.Height, fit)
			})
			grows = app.Find("id:bar").Bounds().Size.Height > base+0.5
		}
		want := base
		if grows {
			switch at {
			case 1:
				want += inset
			case 3:
				want += inset / 2
			}
		}
		app.WaitUntil(5*time.Second, func() bool {
			h := app.Find("id:bar").Bounds().Size.Height
			return near(h, want) && near(h, core.Points(ios.NativeTabBarFit()))
		})
	}
}

func near(a, b core.Points) bool { return math.Abs(float64(a-b)) < 0.51 }
