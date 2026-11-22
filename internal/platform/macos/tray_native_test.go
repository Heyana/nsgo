//go:build darwin && !ios

package macos

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Real status items must reserve separate space for a title and a drawn
// surface, and their AppKit actions must report current screen geometry.
func TestTrayTitleAndClickBoundsNative(t *testing.T) {
	// AppKit's launch and status-bar tracking own process state.
	if os.Getenv("NSGO_TEST_TRAY_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_TRAY_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 18, 18))
	for y := range 18 {
		for x := range 18 {
			pixels.SetNRGBA(x, y, color.NRGBA{G: 255, A: 255})
		}
	}
	var icon bytes.Buffer
	if err := png.Encode(&icon, pixels); err != nil {
		t.Fatal(err)
	}
	onMain(func() {
		loadAppKit()
		nsApp().Send(sel("finishLaunching"))
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		var event driver.Event
		var clickedButton objc.ID
		var nativeAtClick apple.CGRect
		owner := &surface{}
		owner.engine = engine.Start(actionEngine{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
			Event: func(_ uint32, e driver.Event) {
				event = e
				if e.Kind == driver.EventTray {
					nativeAtClick = objc.Send[apple.CGRect](clickedButton.Send(selWindow), selFrame)
				}
			},
		})
		defer owner.engine.Close()
		for _, drawn := range []bool{false, true} {
			cfg := driver.TrayConfig{ID: 1390, Icon: icon.Bytes()}
			if drawn {
				cfg.View = 1
			}
			owner.applyTray(cfg)
			tr := trays[cfg.ID]
			defer func() {
				if tr.item != 0 {
					tr.remove()
				}
			}()
			button := tr.item.Send(sel("button"))
			clickedButton = button
			var view objc.ID
			if drawn {
				view = objc.ID(apple.Class("NSView")).Send(selAlloc).Send(sel("initWithFrame:"), apple.Rect(0, 0, 32, 22))
				defer view.Send(selRelease)
				tr.show(&surface{native: view})
			}
			var emptyWidth float64
			for _, title := range []string{"", "NSGo 1390", "NSGo 1390 expanded title", ""} {
				cfg.Title = title
				owner.applyTray(cfg)
				button.Send(sel("layoutSubtreeIfNeeded"))
				until := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.05)
				objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")).Send(sel("runUntilDate:"), until)
				bounds := objc.Send[apple.CGRect](button, selBounds)
				if bounds.Size.Width <= 0 || button.Send(selWindow) == 0 {
					t.Fatal("status item has no native button window")
				}
				if title == "" {
					if emptyWidth != 0 && math.Abs(emptyWidth-bounds.Size.Width) > 1 {
						t.Errorf("drawn=%v clearing title: width %g, want %g", drawn, bounds.Size.Width, emptyWidth)
					}
					emptyWidth = bounds.Size.Width
				} else if bounds.Size.Width <= emptyWidth+20 {
					t.Errorf("drawn=%v title %q did not grow status item: width %g, empty %g", drawn, title, bounds.Size.Width, emptyWidth)
				}
				if got := apple.GoString(button.Send(sel("title"))); got != title {
					t.Errorf("title %q, want %q", got, title)
				}
				if drawn {
					frame := objc.Send[apple.CGRect](view, selFrame)
					text := objc.Send[apple.CGRect](button.Send(sel("cell")), sel("titleRectForBounds:"), bounds)
					if frame.Size.Width != 32 || frame.Origin.X < 0 || frame.Origin.X+32 > bounds.Size.Width ||
						title != "" && frame.Origin.X+32 > text.Origin.X {
						t.Errorf("drawn icon overlaps title or button: icon %+v, title %+v, button %+v", frame, text, bounds)
					}
				}
				deadline := time.Now().Add(2 * time.Second)
				for objc.Send[apple.CGRect](button.Send(selWindow), selFrame).Size.Height <= 0 && time.Now().Before(deadline) {
					until := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.01)
					e := nsApp().Send(sel("nextEventMatchingMask:untilDate:inMode:dequeue:"), ^uint64(0), until, apple.NSString("kCFRunLoopDefaultMode"), true)
					if e != 0 {
						nsApp().Send(sel("sendEvent:"), e)
					}
				}
				if objc.Send[apple.CGRect](button.Send(selWindow), selFrame).Size.Height <= 0 {
					t.Fatal("status-item window did not receive its screen frame")
				}
				event = driver.Event{}
				button.Send(sel("performClick:"), objc.ID(0))
				native := nativeAtClick
				main := screenFrames("frame")[0]
				want := driver.Bounds{Origin: driver.Point{X: driver.Points(native.Origin.X - main.Origin.X),
					Y: driver.Points(main.Origin.Y + main.Size.Height - native.Origin.Y - native.Size.Height)},
					Size: driver.Size{Width: driver.Points(native.Size.Width), Height: driver.Points(native.Size.Height)}}
				if got, ok := event.Data.(driver.Bounds); !ok || got != want || event.Kind != driver.EventTray || event.Count != cfg.ID || event.Index != 0 {
					t.Errorf("drawn=%v title %q action %+v, want bounds %+v", drawn, title, event, want)
				}
			}
			tr.remove()
		}
	})
}
