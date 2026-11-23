//go:build darwin && !ios

package macos

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Read real NSPanel frames after app commands, including content resizing.
func TestEdgeDockedPanelUsesNativeScreenFrames(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		all := displays()
		if len(all) == 0 {
			t.Skip("no native display")
		}
		t.Logf("native displays: %d", len(all))
		type panelCase struct {
			panel *core.Window
			cx    *core.Scope
			dock  driver.PanelDock
		}
		ready := make(chan []panelCase, 1)
		owner := newSurface(Options{Width: 320, Height: 200}, false)
		owner.engine = engine.Start(owner, core.AppRoot(func(cx *core.Scope) {
			cx.SetShutdownMode(core.ShutdownOnExplicit)
			var cases []panelCase
			for _, display := range all {
				for _, edge := range []core.Placement{core.PlaceTop, core.PlaceBottom, core.PlaceLeft, core.PlaceRight} {
					c := panelCase{dock: driver.PanelDock{Edge: macDockEdge(edge), Display: display.ID, Margin: 10}}
					c.panel = cx.OpenPanel(core.PanelOptions{Width: 120, Height: 60, NoActivate: true, Transparent: true,
						Dock: core.PanelDock{Edge: edge, Display: display.ID, Margin: 10}}, func(cx *core.Scope) core.Node {
						c.cx = cx
						return core.View{}
					})
					cases = append(cases, c)
				}
			}
			ready <- cases
		}), engine.Options{Size: driver.Size{Width: 320, Height: 200}, Environment: owner.environment(), Windows: &engine.WindowOptions{Width: 320, Height: 200}})
		defer owner.destroy()
		cases := <-ready
		settle := func(fn func() bool) bool {
			for deadline := time.Now().Add(3 * time.Second); ; {
				if fn() {
					return true
				}
				if time.Now().After(deadline) {
					return false
				}
				until := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.01)
				objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")).Send(sel("runMode:beforeDate:"), apple.NSString("kCFRunLoopDefaultMode"), until)
			}
		}
		for _, c := range cases {
			title := fmt.Sprintf("%s/%d", c.dock.Display, c.dock.Edge)
			var panel *surface
			if !settle(func() bool {
				for s := range surfaces {
					if s.windowOwner == owner && s.dock == c.dock {
						panel = s
					}
				}
				return panel != nil
			}) {
				t.Fatal("the app's native panel did not open")
			}
			defer panel.destroy()
			c.cx.Post(c.panel.Show)
			check := func(width, height float64) {
				t.Helper()
				var frame apple.CGRect
				var want driver.WindowFrame
				if !settle(func() bool {
					// The screens as they are now: a runner's Dock and menu bar settle their sizes while the test runs, and the panel follows the visible frame they leave.
					want, _ = driver.DockWindow(driver.Size{Width: driver.Points(width), Height: driver.Points(height)}, displays(), c.dock)
					frame = objc.Send[apple.CGRect](panel.window, sel("frame"))
					main := screenFrames("frame")[0]
					x, y := frame.Origin.X-main.Origin.X, main.Origin.Y+main.Size.Height-frame.Origin.Y-frame.Size.Height
					return objc.Send[bool](panel.window, sel("isVisible")) && math.Abs(x-float64(want.X)) <= 1 && math.Abs(y-float64(want.Y)) <= 1 && frame.Size.Width == width && frame.Size.Height == height
				}) {
					t.Fatalf("%s native frame%+v visible%v, want desktop%+v", title, frame, objc.Send[bool](panel.window, sel("isVisible")), want)
				}
				if objc.Send[bool](panel.window, sel("isKeyWindow")) || objc.Send[bool](panel.window, sel("canBecomeKeyWindow")) {
					t.Errorf("%s became a key panel", title)
				}
			}
			check(120, 60)
			for _, size := range []core.Size{{Width: 160, Height: 80}, {Width: 80, Height: 40}} {
				c.cx.Post(func() { c.cx.SetWindowFrame(core.WindowFrame{Width: size.Width, Height: size.Height}) })
				check(float64(size.Width), float64(size.Height))
			}
			c.cx.Post(c.panel.Hide)
			if !settle(func() bool { return !objc.Send[bool](panel.window, sel("isVisible")) }) {
				t.Fatal("the native panel stayed visible after Hide")
			}
			c.cx.Post(c.panel.Show)
			check(80, 40)
			panel.dock.Display = "nsgo-test-missing-output"
			c.cx.Post(c.panel.Show)
			if !settle(func() bool { return !objc.Send[bool](panel.window, sel("isVisible")) }) {
				t.Error("a panel on an unavailable output stayed visible")
			}
		}
	})
}

func macDockEdge(edge core.Placement) driver.WindowPlacement {
	switch edge {
	case core.PlaceTop:
		return driver.WindowPlaceTop
	case core.PlaceBottom:
		return driver.WindowPlaceBottom
	case core.PlaceLeft:
		return driver.WindowPlaceLeft
	default:
		return driver.WindowPlaceRight
	}
}
