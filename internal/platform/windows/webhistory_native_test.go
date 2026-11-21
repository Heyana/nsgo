//go:build windows

package windows

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

type webHistoryAccess struct{}

func (webHistoryAccess) Allows(core.AccessRequest) bool { return true }
func (webHistoryAccess) WebHosts() []string             { return []string{"*"} }
func (webHistoryAccess) WebSchemes() []string           { return nil }

// inWebHistoryProcess gives each native WebView2 test its own UI process.
func inWebHistoryProcess(t *testing.T) bool {
	t.Helper()
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("NSGO_TEST_WEB_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEB_PROCESS="+t.Name())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return false
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	return true
}

func TestWebOwnPagesReloadAndReturn(t *testing.T) {
	if !inWebHistoryProcess(t) {
		return
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<p>next</p>")
	}))
	defer server.Close()
	finished := 0
	app := func(cx *core.Scope) core.Node {
		cx.ProvideAccess(webHistoryAccess{})
		var views []core.Node
		for i, src := range []core.WebSource{core.WebHTML("<p>own</p>", ""), core.WebSrc("data:text/html,<p>own</p>")} {
			var ref core.Ref
			phase := 0
			views = append(views, core.WebView{Source: src, Ref: &ref,
				Style: core.Style{Width: core.Pt(150), Height: core.Pt(100)},
				OnLoad: func(n core.WebNavigation) {
					phase++
					t.Logf("view %d phase %d: %s", i, phase, n.URL)
					if phase > 7 {
						return
					} // Restoration is not an extra test step.
					if (phase == 3 || phase == 6) && !n.CanGoBack {
						t.Errorf("view %d phase %d cannot go back", i, phase)
					}
					if (phase == 4 || phase == 5) && !n.CanGoForward {
						t.Errorf("view %d cannot go forward after going back", i)
					}
					ref.Evaluate("document.body.textContent.trim()", func(body string, err error) {
						want := "\"own\""
						if phase == 3 || phase == 6 {
							want = "\"next\""
						}
						if err != nil || body != want {
							t.Errorf("view %d phase %d body = %s, %v; want %s", i, phase, body, err, want)
							cx.Quit()
							return
						}
						switch phase {
						case 1:
							ref.Reload()
						case 2:
							ref.InjectJavaScript(fmt.Sprintf("location.href = %q", server.URL+"/next"))
						case 3, 6:
							ref.GoBack()
						case 4:
							ref.Reload()
						case 5:
							ref.GoForward()
						case 7:
							ref.InjectJavaScript(`{
const f = document.createElement('iframe');
let reloaded = false;
f.onload = () => {
  if (!reloaded) { reloaded = true; f.contentWindow.location.reload(); return; }
  location.href = 'about:blank';
};
f.src = 'about:blank';
document.body.append(f);
}`)
						}
					})
				},
				OnError: func(err error) {
					e, ok := err.(*core.WebError)
					if phase != 7 || !ok || e.URL != "about:blank" {
						t.Errorf("view %d phase %d: %v", i, phase, err)
						cx.Quit()
						return
					}
					finished++
					if finished == 2 {
						cx.Quit()
					}
				},
			})
		}
		return core.View{Style: core.Style{Direction: core.Row}, Children: views}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: "Web own history", Width: 300, Height: 100})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the own pages did not reload and traverse history")
	}
	if finished != 2 {
		t.Errorf("%d views completed, want 2", finished)
	}
}

// TestWebNavigationStateTracksPageEdits checks the state WebView2 reports
// after same-document changes and the two snapshots of a complete load.
func TestWebNavigationStateTracksPageEdits(t *testing.T) {
	if !inWebHistoryProcess(t) {
		return
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		title := "First"
		if r.URL.Path == "/next" {
			title = "Next"
		}
		fmt.Fprintf(w, "<!doctype html><title>%s</title>", title)
	}))
	defer server.Close()

	steps := []struct {
		path, title   string
		back, forward bool
		script        string
	}{
		{"/first", "First", false, false, `history.pushState({}, "", "/pushed")`},
		{"/pushed", "First", true, false, `document.title = "Renamed"`},
		{"/pushed", "Renamed", true, false, `history.replaceState({}, "", "/replaced")`},
		{"/replaced", "Renamed", true, false, `location.hash = "section"`},
		{"/replaced#section", "Renamed", true, false, `history.back()`},
		{"/replaced", "Renamed", true, true, `history.forward()`},
		{"/replaced#section", "Renamed", true, false, `history.back()`},
		{"/replaced", "Renamed", true, true, `history.pushState({}, "", location.href)`},
		{"/replaced", "Renamed", true, false, ""},
		{"/next", "Next", true, false, ""},
	}
	var states []core.WebNavigation
	at := 0
	app := func(cx *core.Scope) core.Node {
		var ref core.Ref
		return core.WebView{
			Source: core.WebSrc(server.URL + "/first"), Ref: &ref,
			Style: core.Style{Width: core.Pt(300), Height: core.Pt(170)},
			OnNavigationStateChange: func(nav core.WebNavigation) {
				states = append(states, nav)
				t.Logf("navigation: %+v", nav)
				if at == len(steps) {
					return
				}
				step := steps[at]
				want := core.WebNavigation{
					URL: server.URL + step.path, Title: step.title,
					CanGoBack: step.back, CanGoForward: step.forward,
				}
				if nav != want {
					return
				}
				at++
				switch {
				case step.script != "":
					ref.InjectJavaScript(step.script)
				case at < len(steps):
					ref.Load(core.WebSrc(server.URL + "/next"))
				default:
					// Give any trailing browser notifications time to arrive.
					cx.AfterFunc(100*time.Millisecond, cx.Quit)
				}
			},
			OnError: func(err error) {
				t.Error(err)
				cx.Quit()
			},
		}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: "Web navigation state", Width: 300, Height: 170})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the web view did not report all navigation states")
	}
	if at != len(steps) {
		t.Fatalf("matched %d of %d navigation snapshots", at, len(steps))
	}
	for _, path := range []string{"/first", "/next"} {
		var loading []bool
		for _, n := range states {
			if n.URL == server.URL+path {
				loading = append(loading, n.Loading)
			}
		}
		if len(loading) != 2 || !loading[0] || loading[1] {
			t.Errorf("navigation snapshots for %s have Loading = %v, want [true false]", path, loading)
		}
	}
}
