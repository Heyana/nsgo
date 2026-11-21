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

func TestChildWindowUsesItsOwnAppParent(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		newApp := func() *surface {
			s := newSurface(Options{}, false)
			s.engine = engine.Start(s, func(*core.Scope) core.Node { return core.View{} }, engine.Options{})
			return s
		}
		app, foreign := newApp(), newApp()
		defer app.destroy()
		defer foreign.destroy()
		open := func(owner *surface, title string, id, parent uint32) *surface {
			title = t.Name() + "/" + title
			owner.openSurface(driver.SurfaceConfig{ID: id, Parent: parent, Width: 200, Height: 120, Title: title, Hidden: true})
			for s := range surfaces {
				if s.id == id && s.window != 0 && apple.GoString(s.window.Send(sel("title"))) == title {
					return s
				}
			}
			t.Fatalf("surface %d did not open", id)
			return nil
		}
		foreignParent := open(foreign, "foreign", 1, 0)
		defer foreignParent.destroy()
		child := open(app, "no parent", 2, 1)
		defer child.destroy()
		if child.windowParent != nil {
			t.Fatal("the child resolved its parent ID in another app")
		}
		parent := open(app, "parent", 1, 0)
		defer parent.destroy()
		secondChild := open(app, "child", 3, 1)
		defer secondChild.destroy()
		if secondChild.windowParent != parent {
			t.Fatal("the child did not resolve its parent in the same app")
		}
	})
}

func TestChildWindowFollowsParent(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		window := func(width, height float64) objc.ID {
			w := objc.ID(classWindow).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
				apple.Rect(0, 0, width, height), uint(styleTitled|styleMiniaturizable), uint(2), false)
			w.Send(sel("setReleasedWhenClosed:"), false)
			w.Send(sel("setAnimationBehavior:"), 2) // NSWindowAnimationBehaviorNone
			return w
		}
		p := &surface{window: window(320, 240)}
		c := &surface{window: window(160, 100), windowParent: p}
		p.windowChildren = []*surface{c}
		defer func() {
			p.releaseChildWindows()
			for _, w := range []objc.ID{c.window, p.window} {
				w.Send(sel("close"))
				w.Send(selRelease)
			}
		}()
		p.window.Send(sel("center"))
		c.centerOverParent()
		pf := objc.Send[apple.CGRect](p.window, sel("frame"))
		cf := objc.Send[apple.CGRect](c.window, sel("frame"))
		if math.Abs(pf.Origin.X+pf.Size.Width/2-cf.Origin.X-cf.Size.Width/2) > 1 ||
			math.Abs(pf.Origin.Y+pf.Size.Height/2-cf.Origin.Y-cf.Size.Height/2) > 1 {
			t.Fatalf("child frame %v is not centered over %v", cf, pf)
		}
		p.window.Send(sel("orderFrontRegardless"))
		if !c.showChildWindow() {
			t.Fatal("a visible parent keeps its child hidden")
		}
		c.window.Send(sel("orderFrontRegardless"))
		p.window.Send(sel("orderFrontRegardless"))
		p.childWindowEvent("NSWindowDidBecomeMainNotification")
		if child, parent := objc.Send[int](c.window, sel("orderedIndex")), objc.Send[int](p.window, sel("orderedIndex")); child >= parent {
			t.Fatalf("ordered child at %d, parent at %d", child, parent)
		}
		p.command(&driver.WindowCommand{Action: driver.WindowHide})
		if c.showsChildren() || c.showChildWindow() {
			t.Fatal("the child shows while its parent is hidden")
		}
		p.show(0)
		if !c.showsChildren() {
			t.Fatal("showing the parent did not restore its child")
		}
		p.command(&driver.WindowCommand{Action: driver.WindowMinimize})
		loop := objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop"))
		waitMinimized := func(want bool) bool {
			deadline := time.Now().Add(2 * time.Second)
			for objc.Send[bool](p.window, sel("isMiniaturized")) != want && time.Now().Before(deadline) {
				until := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.01)
				loop.Send(sel("runMode:beforeDate:"), apple.NSString("kCFRunLoopDefaultMode"), until)
			}
			return objc.Send[bool](p.window, sel("isMiniaturized")) == want
		}
		if !waitMinimized(true) {
			t.Fatal("AppKit did not minimize the parent")
		}
		p.childWindowEvent("NSWindowDidMiniaturizeNotification")
		if c.showsChildren() {
			t.Fatal("minimizing the parent did not hide its child")
		}
		p.window.Send(sel("deminiaturize:"), objc.ID(0))
		if !waitMinimized(false) {
			t.Fatal("AppKit did not restore the parent")
		}
		p.childWindowEvent("NSWindowDidDeminiaturizeNotification")
		if !c.showsChildren() {
			t.Fatal("restoring the parent did not restore its child")
		}
		p.childWindowEvent("NSWindowWillEnterFullScreenNotification")
		if c.window.Send(sel("parentWindow")) != p.window || objc.Send[uint](c.window, sel("collectionBehavior"))&fullScreenAuxiliary == 0 {
			t.Fatal("the child did not join its parent's full-screen window")
		}
		p.childWindowEvent("NSWindowDidExitFullScreenNotification")
		if c.window.Send(sel("parentWindow")) != 0 || objc.Send[uint](c.window, sel("collectionBehavior"))&fullScreenAuxiliary != 0 {
			t.Fatal("the child remained attached after leaving full screen")
		}
		c.command(&driver.WindowCommand{Action: driver.WindowHide})
		p.command(&driver.WindowCommand{Action: driver.WindowHide})
		p.show(0)
		if c.showsChildren() {
			t.Fatal("showing the parent revealed a child the app had hidden")
		}
	})
}
