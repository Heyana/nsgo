//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func waitForScreenWakeLock(t *testing.T, what string, ready func() bool) {
	t.Helper()
	limit := time.Now().Add(3 * time.Second)
	for !ready() {
		if time.Now().After(limit) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestScreenAwakeFollowsPageVisibility(t *testing.T) {
	g := js.Global()
	doc := g.Get("document")
	if !doc.Truthy() || !g.Get("navigator").Get("wakeLock").Truthy() {
		t.Skip("requires a browser with the Screen Wake Lock API")
	}
	setVisibility := func(value string) {
		g.Get("Object").Call("defineProperty", doc, "visibilityState", map[string]any{"value": value, "configurable": true})
		doc.Call("dispatchEvent", g.Get("Event").New("visibilitychange"))
	}
	defer g.Get("Reflect").Call("deleteProperty", doc, "visibilityState")
	a := &surface{doc: doc, animator: driver.NewAnimator(func(driver.Event) {})}
	b := &surface{doc: doc, animator: driver.NewAnimator(func(driver.Event) {})}
	defer a.clearDisplayHolds()
	defer b.clearDisplayHolds()
	mount := func(s *surface, kind driver.ItemKind, id uint32, display bool) {
		s.Mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: kind, Count: id, Awake: &driver.KeepAwake{Display: display}}}})
	}
	holding := func() bool { return screenAwake.lock.Truthy() && !screenAwake.lock.Get("released").Bool() }
	mount(a, driver.ItemKeepAwake, 1, false)
	if len(screenAwake.holds) != 0 || screenAwake.pending {
		t.Fatal("computer-only hold asked the browser for a lock")
	}
	setVisibility("hidden")
	mount(a, driver.ItemKeepAwake, 2, true)
	mount(b, driver.ItemKeepAwake, 2, true)
	if screenAwake.pending || holding() {
		t.Fatal("hidden page asked for the display lock")
	}
	setVisibility("visible")
	waitForScreenWakeLock(t, "the browser's first sentinel", holding)
	first := screenAwake.lock
	mount(a, driver.ItemReleaseAwake, 2, false)
	mount(a, driver.ItemReleaseAwake, 2, false)
	if !holding() || !screenAwake.lock.Equal(first) {
		t.Fatal("releasing one surface's hold ended the shared lock")
	}
	setVisibility("hidden")
	waitForScreenWakeLock(t, "release on hide", func() bool { return first.Get("released").Bool() })
	setVisibility("visible")
	waitForScreenWakeLock(t, "a different sentinel after visibility returns", func() bool { return holding() && !screenAwake.lock.Equal(first) })
	last := screenAwake.lock
	b.windowCommand(&driver.WindowCommand{Action: driver.WindowDestroy})
	waitForScreenWakeLock(t, "release on surface destruction", func() bool { return last.Get("released").Bool() })

	mount(a, driver.ItemKeepAwake, 3, true)
	if !screenAwake.pending {
		t.Fatal("expected a new promise while taking the hold")
	}
	mount(a, driver.ItemReleaseAwake, 3, false)
	waitForScreenWakeLock(t, "the canceled request to finish", func() bool { return !screenAwake.pending })
	if holding() {
		t.Fatal("a request that settled after its release kept the display awake")
	}
}
