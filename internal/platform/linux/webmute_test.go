//go:build linux && !android

package linux

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebAudioMutePersistsAcrossNavigation(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEBKIT_PROCESS") != t.Name() {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return
	}
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	var isMuted func(uintptr) int32
	purego.RegisterLibFunc(&isMuted, purego.RTLD_DEFAULT, "webkit_web_view_get_is_muted")
	loads := make(chan string, 2)
	app := func(*core.Scope) core.Node {
		return core.WebView{Source: webMutePage("start"), Muted: true, Incognito: true,
			Style:     core.Style{Width: core.Pt(200), Height: core.Pt(100)},
			OnMessage: func(page string) { loads <- page }}
	}
	w := startTestWindow(t, app, Options{Title: "Web audio mute", Width: 200, Height: 100})
	waitLoad := func(title string) {
		t.Helper()
		deadline := time.After(15 * time.Second)
		for {
			select {
			case loaded := <-loads:
				if loaded == title {
					return
				}
			case <-deadline:
				t.Fatalf("the web view did not load %q", title)
			}
		}
	}
	waitLoad("start")
	var widget uintptr
	check := func(want bool, change func(*driver.WebConfig)) {
		t.Helper()
		var got bool
		var native uintptr
		w.onLoop(t, func() {
			for _, v := range w.views {
				if v == nil || v.web == nil || v.web.widget == 0 {
					continue
				}
				if change != nil {
					cfg := v.web.config
					change(&cfg)
					w.configureWeb(v, &cfg)
				}
				native = v.web.widget
				got = isMuted(native) != 0
				break
			}
		})
		if native == 0 || widget != 0 && native != widget || got != want {
			t.Fatalf("WebKitGTK view %x has mute %v, want the same view with mute %v", native, got, want)
		}
		widget = native
	}
	check(true, func(c *driver.WebConfig) { c.Source = webMutePage("next") })
	waitLoad("next")
	check(true, nil)
	check(false, func(c *driver.WebConfig) { c.Muted = false })
}

// The page's reply identifies the new document even if its title arrives after the platform's load notification.
func webMutePage(name string) core.WebSource {
	return core.WebHTML("<script>window.nsgo.postMessage('"+name+"')</script>", "")
}
