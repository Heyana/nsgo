//go:build linux && !android

package linux

import (
	"io"
	"net/http"
	"net/http/httptest"
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<!doctype html><p>Web audio mute</p>")
	}))
	t.Cleanup(server.Close)
	start, next := server.URL+"/start", server.URL+"/next"
	loads := make(chan string, 2)
	app := func(*core.Scope) core.Node {
		return core.WebView{Source: core.WebSrc(start), Muted: true, Incognito: true,
			Style:  core.Style{Width: core.Pt(200), Height: core.Pt(100)},
			OnLoad: func(page core.WebNavigation) { loads <- page.URL }}
	}
	w := startTestWindow(t, app, Options{Title: "Web audio mute", Width: 200, Height: 100})
	waitLoad := func(url string) {
		t.Helper()
		deadline := time.After(15 * time.Second)
		for {
			select {
			case loaded := <-loads:
				if loaded == url {
					return
				}
			case <-deadline:
				t.Fatalf("the web view did not load %q", url)
			}
		}
	}
	waitLoad(start)
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
	check(true, func(c *driver.WebConfig) { c.Source = core.WebSrc(next) })
	waitLoad(next)
	check(true, nil)
	check(false, func(c *driver.WebConfig) { c.Muted = false })
}
