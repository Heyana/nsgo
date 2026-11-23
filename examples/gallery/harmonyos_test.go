//go:build nsgoharmony

package main

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestHarmonyOSPages opens every demo the HarmonyOS build lists, from its tab's index under the drawn tab bar, and the settings, on the emulator: nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery.
func TestHarmonyOSPages(t *testing.T) {
	app := nsgotest.Start(t, Root)
	// The device keeps the demo an earlier run left open, which the gallery opens again.
	if d := shell.shownDemo(); d != "" {
		app.Back()
		app.WaitForGone("id:screen-"+d, 5*time.Second)
	}
	for i, tab := range tabs {
		app.Press("id:tab-bar >> role:tab >> nth=" + itoa(i))
		for name := range omitted {
			if app.Exists("id:nav-" + name) {
				t.Fatalf("the %s tab lists %s, which this build leaves out", tab.id, name)
			}
		}
		for _, g := range tab.groups {
			for _, d := range g.demos {
				app.Press("id:nav-" + d.name)
				app.Find("id:screen-" + d.name)
				app.Back()
				app.WaitForGone("id:screen-"+d.name, 5*time.Second)
			}
		}
	}
	app.Press("id:tab-bar >> role:tab >> nth=" + itoa(settingsTab()))
	app.Find("id:settings-page")
}

// TestHarmonyOSWebViewScripts reads the values the page and both injected scripts wrote, then changes only the before-content script and reads the next load.
func TestHarmonyOSWebViewScripts(t *testing.T) {
	const page = `<!doctype html><title>initial</title><script>
window.seenByPage = window.before || "missing";
window.endedBeforePage = window.after || "pending";
</script><p id="ready">ready</p>`
	source := core.WebSrc("data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(page)))
	titles := make(chan string, 4)
	var before *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		before = core.NewSignal(cx, "window.before = 'first';")
		return core.WebView{
			Source:                              source,
			Style:                               core.Style{Width: core.Pt(240), Height: core.Pt(180)},
			InjectedJavaScriptBeforeContentFunc: before.Get,
			InjectedJavaScript: `window.after = (window.after || 0) + 1;
document.title = [window.seenByPage, window.endedBeforePage, window.after,
  document.getElementById("ready").textContent].join(":");`,
			OnLoad:  func(nav core.WebNavigation) { titles <- nav.Title },
			OnError: func(err error) { titles <- err.Error() },
		}
	})
	expect := func(want string) {
		t.Helper()
		select {
		case got := <-titles:
			if got != want {
				t.Fatalf("page title = %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no page load reported title %q", want)
		}
	}
	expect("first:pending:1:ready")
	app.Post(func() { before.Set("window.before = 'second';") })
	app.Settle()
	expect("second:pending:1:ready")
}
