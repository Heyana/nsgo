package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAppClipOverlayScopeAndDismissal(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	h.tree.SetEnvironment(Environment{OS: OSIOS, AppClip: true})
	h.commit()
	first, second := cx.Child(), cx.Child()
	var dismissFirst func()
	first.Run(func() { dismissFirst = first.ShowAppClipOverlay(AppClipOverlayBottom) })
	one := command(h.commit(), driver.ItemAppClipOverlay)
	if one == nil || !one.On || one.Count == 0 || one.Value != "bottom" {
		t.Fatalf("first request=%+v", one)
	}
	second.Run(func() { second.ShowAppClipOverlay(AppClipOverlayBottomRaised) })
	two := command(h.commit(), driver.ItemAppClipOverlay)
	if two == nil || !two.On || two.Count == one.Count || two.Value != "bottom-raised" {
		t.Fatalf("replacement request=%+v", two)
	}
	dismissFirst()
	h.drain()
	old := command(h.commit(), driver.ItemAppClipOverlay)
	if old == nil || old.On || old.Count != one.Count {
		t.Fatalf("dismissal lost ownership: %+v", old)
	}
	dismissFirst()
	first.Dispose()
	h.drain()
	if it := command(h.commit(), driver.ItemAppClipOverlay); it != nil {
		t.Fatalf("repeat cleanup sent another dismissal: %+v", it)
	}
	second.Dispose()
	last := command(h.commit(), driver.ItemAppClipOverlay)
	if last == nil || last.On || last.Count != two.Count {
		t.Fatalf("scope cleanup did not dismiss its own request: %+v", last)
	}
}

func TestAppClipOverlayUnavailableAndConfined(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	for _, env := range []Environment{
		{OS: OSIOS}, {OS: OSAndroid, AppClip: true}, {OS: OSMacOS, AppClip: true},
		{OS: OSIOS, AppClip: true, Simulated: true},
	} {
		h.tree.SetEnvironment(env)
		h.commit()
		cx.ShowAppClipOverlay(AppClipOverlayBottom)()
		h.drain()
		if it := command(h.commit(), driver.ItemAppClipOverlay); it != nil {
			t.Fatalf("unsupported environment sent %+v", it)
		}
	}
	h.tree.SetEnvironment(Environment{OS: OSIOS, AppClip: true})
	h.commit()
	cx.ShowAppClipOverlay(AppClipOverlayPosition(255))()
	h.drain()
	if it := command(h.commit(), driver.ItemAppClipOverlay); it != nil {
		t.Fatalf("invalid position sent %+v", it)
	}
	confined := cx.Child()
	confined.Run(func() { confined.Confine(); confined.ShowAppClipOverlay(AppClipOverlayBottom) })
	if it := command(h.commit(), driver.ItemAppClipOverlay); it != nil {
		t.Fatalf("confined scope presented without activation: %+v", it)
	}
	dead := cx.Child()
	dead.Dispose()
	dead.ShowAppClipOverlay(AppClipOverlayBottom)()
	h.drain()
	if it := command(h.commit(), driver.ItemAppClipOverlay); it != nil {
		t.Fatalf("disposed scope sent %+v", it)
	}
}

func TestAppClipOverlayConfinedUserActivation(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		return Func(func(owner *Scope) Node {
			owner.Confine()
			return View{TestID: "offer", Style: Style{Width: Pt(100), Height: Pt(40)}, OnPress: func(PressEvent) {
				owner.ShowAppClipOverlay(AppClipOverlayBottom)
			}}
		})
	})
	h.tree.SetEnvironment(Environment{OS: OSIOS, AppClip: true})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventPress, ID: h.find("offer").id})
	if it := command(h.commit(), driver.ItemAppClipOverlay); it == nil || !it.On {
		t.Fatalf("explicit confined user action did not present: %+v", it)
	}
}
