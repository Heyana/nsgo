//go:build darwin && !ios

package macos

import (
	"math"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func withFramePanel(t *testing.T, opts core.PanelOptions, content core.View, run func(*surface)) {
	t.Helper()
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		screens := screenFrames("visibleFrame")
		if len(screens) == 0 {
			t.Skip("no native display")
		}
		opts.NoActivate = true
		owner := newSurface(Options{Width: 320, Height: 200}, false)
		owner.engine = engine.Start(owner, core.AppRoot(func(cx *core.Scope) {
			cx.SetShutdownMode(core.ShutdownOnExplicit)
			cx.OpenPanel(opts, func(*core.Scope) core.Node { return content })
		}), engine.Options{Size: driver.Size{Width: 320, Height: 200}, Environment: owner.environment(), Windows: &engine.WindowOptions{Width: 320, Height: 200}})
		defer owner.destroy()
		var panel *surface
		defer func() {
			if panel != nil {
				panel.destroy()
			}
		}()
		if !panelTestPump(func() bool {
			for s := range surfaces {
				if s.windowOwner == owner && s.kind == driver.SurfacePanel {
					panel = s
					return int(s.root) < len(s.views) && s.views[s.root] != nil && s.views[s.root].framed
				}
			}
			return false
		}, 3*time.Second) {
			t.Fatal("the app's native panel did not lay out")
		}
		run(panel)
	})
}

func nativePanelFrame(s *surface) apple.CGRect {
	return objc.Send[apple.CGRect](s.window, sel("frame"))
}

func TestPanelFrameRestoresNativeChrome(t *testing.T) {
	for _, tc := range []struct {
		name        string
		transparent bool
	}{{"opaque", false}, {"transparent", true}} {
		t.Run(tc.name, func(t *testing.T) {
			content := core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xd02030)}}
			withFramePanel(t, core.PanelOptions{Width: 6, Height: 100, Transparent: tc.transparent}, content, func(s *surface) {
				w := s.window
				if !tc.transparent {
					s.showPanel(0, driver.Bounds{}, driver.WindowPlaceDefault)
				}
				start := nativePanelFrame(s)
				check := func(width, height, radius float64, titled bool) {
					t.Helper()
					frame := nativePanelFrame(s)
					bounds := objc.Send[apple.CGRect](s.native, selBounds)
					want := apple.CGSize{Width: width, Height: height}
					if frame.Size != want || bounds.Size != want || math.Abs(frame.Origin.Y+frame.Size.Height-start.Origin.Y-start.Size.Height) > 0.5 {
						t.Fatalf("frame %+v, content %+v; want size %+v at the original top edge", frame, bounds, want)
					}
					style := uint(styleNonactivating)
					if titled {
						style |= panelTitleStyle
						if !objc.Send[bool](w, sel("titlebarAppearsTransparent")) || objc.Send[int](w, sel("titleVisibility")) != titleHidden {
							t.Fatal("restored titlebar did not remain hidden")
						}
						for i := range 3 {
							if button := w.Send(selStandardButton, i); button != 0 && !objc.Send[bool](button, sel("isHidden")) {
								t.Fatal("a restored traffic light became visible")
							}
						}
					}
					if mask := objc.Send[uint](w, selStyleMask) & (styleNonactivating | panelTitleStyle); mask != style {
						t.Fatalf("native style mask = %#x, want %#x", mask, style)
					}
					layer := s.native.Send(selLayer)
					if got := objc.Send[float64](layer, sel("cornerRadius")); got != radius {
						t.Errorf("native corner radius = %g, want %g", got, radius)
					}
					if objc.Send[bool](w, sel("isOpaque")) == tc.transparent ||
						objc.Send[bool](w, sel("hasShadow")) == tc.transparent {
						t.Error("resizing changed the window's transparency or shadow")
					}
					if objc.Send[bool](w, sel("canBecomeKeyWindow")) || objc.Send[bool](w, sel("isKeyWindow")) {
						t.Error("nonactivating panel became eligible for keyboard focus")
					}
				}
				checkCorners := func(square bool) {
					t.Helper()
					var captured bool
					if panelTestPump(func() bool {
						img := captureView(s.native)
						if img == nil {
							return false
						}
						captured = true
						b := img.Bounds()
						red := func(x, y int) bool {
							r, g, _, a := img.At(x, y).RGBA()
							return a > 0xe000 && r > 0x9000 && g < r/2
						}
						return red(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2) &&
							red(b.Min.X+1, b.Min.Y+1) == square && red(b.Max.X-2, b.Min.Y+1) == square
					}, time.Second) {
						return
					}
					if captured {
						t.Errorf("composited panel corners were not square=%t", square)
					} else {
						t.Log("WindowServer capture unavailable; checked native chrome instead")
					}
				}
				radius := 12.0
				if tc.transparent {
					radius = 0
				}
				check(6, 100, 0, false)
				if !tc.transparent {
					checkCorners(true)
				}
				s.setWindowFrame(driver.WindowFrame{Width: 320, Height: 100})
				check(320, 100, radius, !tc.transparent)
				if !tc.transparent {
					checkCorners(false)
				}
				s.setWindowFrame(driver.WindowFrame{Height: 6})
				check(320, 6, 0, false)
				s.fitPanel(driver.WindowFrame{Width: 200, Height: 80})
				check(200, 80, radius, !tc.transparent)
				s.fitPanel(driver.WindowFrame{Width: 6, Height: 80})
				check(6, 80, 0, false)
			})
		})
	}
}

func TestPanelFrameMotionUsesElapsedTimeAndCancels(t *testing.T) {
	withFramePanel(t, core.PanelOptions{Width: 100, Height: 80}, core.View{}, func(s *surface) {
		reduce := false
		s.envPin = func(env driver.Environment) driver.Environment {
			env.ReduceMotion = reduce
			return env
		}
		s.showPanel(0, driver.Bounds{}, driver.WindowPlaceDefault)
		if !objc.Send[bool](s.window, sel("isVisible")) {
			t.Fatal("the nonactivating panel did not show")
		}
		assertFrame := func(want apple.CGRect) {
			t.Helper()
			if got := nativePanelFrame(s); got.Size != want.Size || math.Abs(got.Origin.X-want.Origin.X) > 0.5 || math.Abs(got.Origin.Y-want.Origin.Y) > 0.5 {
				t.Fatalf("native frame = %+v, want %+v", got, want)
			}
		}
		start := nativePanelFrame(s)
		target := apple.Rect(start.Origin.X+24, start.Origin.Y-20, 300, 100)
		// Step the same code the display link uses without depending on the
		// display's refresh rate or the time AppKit spends applying a frame.
		now := time.Now()
		s.setPanelFrame(target, true, now)
		assertFrame(start)
		s.stepPanelFrame(now.Add(60 * time.Millisecond))
		mid := nativePanelFrame(s)
		if mid.Size.Width <= start.Size.Width+0.3*(target.Size.Width-start.Size.Width) || mid.Size.Width >= target.Size.Width ||
			mid.Origin.X <= start.Origin.X || mid.Origin.X >= target.Origin.X {
			t.Fatalf("native frame %+v did not ease out from %+v to %+v", mid, start, target)
		}
		if math.Abs(mid.Origin.Y+mid.Size.Height-start.Origin.Y-start.Size.Height) > 1 {
			t.Fatalf("the intermediate frame did not keep the native top edge: %+v", mid)
		}
		s.setPanelFrame(target, true, now.Add(60*time.Millisecond))
		s.stepPanelFrame(now.Add(200 * time.Millisecond))
		assertFrame(target)

		now = now.Add(time.Second)
		s.setPanelFrame(start, true, now)
		s.stepPanelFrame(now.Add(40 * time.Millisecond))
		before := nativePanelFrame(s)
		now = now.Add(40 * time.Millisecond)
		s.setPanelFrame(target, true, now)
		s.stepPanelFrame(now)
		assertFrame(before)
		s.stepPanelFrame(now.Add(200 * time.Millisecond))
		assertFrame(target)

		now = now.Add(time.Second)
		s.setPanelFrame(start, true, now)
		s.setWindowFrame(driver.WindowFrame{Width: 6, Height: 100})
		immediate := target
		immediate.Size.Width = 6
		assertFrame(immediate)
		s.stepPanelFrame(now.Add(200 * time.Millisecond))
		assertFrame(immediate)

		now = now.Add(time.Second)
		s.setPanelFrame(target, true, now)
		s.command(&driver.WindowCommand{Action: driver.WindowFitSize, Frame: driver.WindowFrame{Width: 6, Height: 100}})
		s.stepPanelFrame(now.Add(200 * time.Millisecond))
		assertFrame(immediate)

		now = now.Add(time.Second)
		s.setPanelFrame(target, true, now)
		reduce = true
		s.stepPanelFrame(now)
		assertFrame(target)
		s.setWindowFrame(driver.WindowFrame{Width: 180, Animated: true})
		target.Size.Width = 180
		assertFrame(target)

		now = now.Add(time.Second)
		reduce = false
		s.command(&driver.WindowCommand{Action: driver.WindowSetFrame, Frame: driver.WindowFrame{Width: 220, Animated: true}})
		assertFrame(target)
		s.stepPanelFrame(time.Now().Add(200 * time.Millisecond))
		target.Size.Width = 220
		assertFrame(target)

		s.setPanelFrame(start, true, now)
		s.hidePanel(false)
		s.stepPanelFrame(now.Add(200 * time.Millisecond))
		assertFrame(target)
		s.setWindowFrame(driver.WindowFrame{Width: 90, Animated: true})
		target.Size.Width = 90
		assertFrame(target)
	})
}

func TestPanelFrameRepicksStationaryNativeHover(t *testing.T) {
	events := make(chan driver.EventKind, 8)
	withFramePanel(t, core.PanelOptions{Width: 6, Height: 100}, core.View{
		TestID:       "panel-hover",
		Style:        core.Style{FlexGrow: 1, Background: core.RGB(0x335588)},
		OnMouseEnter: func(core.MouseEvent) { events <- driver.EventMouseEnter },
		OnMouseLeave: func(core.MouseEvent) { events <- driver.EventMouseLeave },
	}, func(s *surface) {
		// Same window-local point throughout: the left and bottom edges
		// stay fixed. No test NSEvent or physical mouse movement is needed.
		at := apple.CGPoint{X: 70, Y: 45}
		s.testPointer = &at
		var target *view
		for _, v := range s.views {
			if v != nil && v.props.TestID == "panel-hover" {
				target = v
				break
			}
		}
		if target == nil {
			t.Fatal("the panel's hover target did not mount")
		}
		s.showPanel(0, driver.Bounds{}, driver.WindowPlaceDefault)
		point := objc.Send[apple.CGPoint](s.window, sel("convertPointToScreen:"), at)
		front := func() int {
			return objc.Send[int](objc.ID(apple.Class("NSWindow")), sel("windowNumberAtPoint:belowWindowWithWindowNumber:"), point, 0)
		}
		cover := newPanel(driver.SurfaceConfig{Width: 100, Height: 80, NoActivate: true})
		cover.Send(sel("setReleasedWhenClosed:"), false)
		defer cover.Send(selRelease)
		defer cover.Send(sel("close"))
		cover.Send(sel("setLevel:"), levelStatus+1)
		cover.Send(sel("setFrameOrigin:"), apple.CGPoint{X: point.X - 50, Y: point.Y - 40})
		cover.Send(sel("orderFrontRegardless"))
		coverNumber := objc.Send[int](cover, sel("windowNumber"))
		if !panelTestPump(func() bool { return front() == coverNumber }, time.Second) {
			t.Skip("window server did not expose the nonactivating cover at the test point")
		}
		s.setWindowFrame(driver.WindowFrame{Width: 200})
		if !panelTestPump(func() bool { return s.panelRepickAfter == 0 && covers(target, s.hitAt(at)) }, time.Second) {
			t.Fatal("the expanded panel did not lay out and re-pick at the test point")
		}
		if target.over || len(events) != 0 {
			t.Fatal("covered panel hovered through the window above it")
		}
		s.setWindowFrame(driver.WindowFrame{Width: 6})
		cover.Send(sel("orderOut:"), objc.ID(0))
		// AppKit can still route hits to a panel while it orders out.
		if !panelTestPump(func() bool { return front() != coverNumber }, time.Second) {
			t.Fatal("the window server did not remove the cover from hit testing")
		}
		wantEvent := func(want driver.EventKind) {
			t.Helper()
			if !panelTestPump(func() bool { return len(events) > 0 }, time.Second) {
				t.Fatalf("stationary pointer did not report %v: window=%+v front=%d panel=%d target=%t",
					want, nativePanelFrame(s), front(), objc.Send[int](s.window, sel("windowNumber")), covers(target, s.hitAt(at)))
			}
			if got := <-events; got != want {
				t.Fatalf("stationary pointer reported %v, want %v", got, want)
			}
		}
		s.envPin = func(env driver.Environment) driver.Environment { env.ReduceMotion = false; return env }
		now := time.Now()
		wide := nativePanelFrame(s)
		wide.Size.Width = 200
		s.setPanelFrame(wide, true, now)
		s.stepPanelFrame(now.Add(200 * time.Millisecond))
		wantEvent(driver.EventMouseEnter)
		s.setPointerEvents(driver.WindowPointerNone)
		wantEvent(driver.EventMouseLeave)
		s.setPointerEvents(driver.WindowPointerPainted)
		wantEvent(driver.EventMouseEnter)
		s.setWindowFrame(driver.WindowFrame{Width: 6})
		wantEvent(driver.EventMouseLeave)
	})
}
