//go:build linux && !android

package linux

import (
	"os"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// TestGlobalShortcutPortalWithPermission is an opt-in acceptance check against
// the desktop's real GlobalShortcuts portal. The operator grants the binding
// and presses it while another window or the shell has the keyboard focus.
func TestGlobalShortcutPortalWithPermission(t *testing.T) {
	appID := os.Getenv("NSGO_TEST_GLOBAL_SHORTCUT_PORTAL")
	if appID == "" {
		t.Skip("requires a real portal, an installed desktop ID, permission and a global key press")
	}
	const title = "nsgo-portal-hotkey"
	fired := make(chan struct{}, 1)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		h := cx.RegisterHotKey("ctrl+alt+k", func() {
			select {
			case fired <- struct{}{}:
			default:
			}
		})
		cx.Effect(func() { cx.SetTitle(title + "|" + h.State().String()) })
		return core.Text{Content: "Grant Ctrl+Alt+K, then press it outside this window."}
	}, Options{ID: appID, Title: title, Width: 600, Height: 150})
	deadline := time.Now().Add(60 * time.Second)
	for w.titleOf(t) != title+"|registered" {
		if got := w.titleOf(t); got == title+"|unsupported" || got == title+"|taken" {
			t.Fatalf("the portal refused the shortcut: %s", got)
		}
		if time.Now().After(deadline) {
			t.Fatalf("the portal did not grant the shortcut: %s", w.titleOf(t))
		}
		time.Sleep(50 * time.Millisecond)
	}
	var portal bool
	w.onLoop(t, func() { portal = w.hotKeys != nil && w.hotKeys.session != "" && w.hotKeys.activatedSub != 0 })
	if !portal {
		t.Fatal("the shortcut did not use the portal; run this check in a Wayland session")
	}
	select {
	case <-fired:
		var active bool
		w.onLoop(t, func() { active = gtkWindowIsActive(w.win) != 0 })
		if active {
			t.Fatal("the shortcut fired with NSGo focused; press it from another application")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the granted shortcut did not fire")
	}
}
