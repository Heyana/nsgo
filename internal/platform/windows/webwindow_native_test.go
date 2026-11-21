//go:build windows

package windows

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

const popupTestPage = `<!doctype html>
<style>
body, form { margin: 0; }
a, button { display: block; box-sizing: border-box; width: 220px; height: 32px; }
</style>
<a target="_blank" href="https://allowed.invalid/link">Link</a>
<button onclick="request('click')">Script</button>
<form target="_blank" method="post" action="https://allowed.invalid/form"><button>Form</button></form>
<a target="_blank" href="https://denied.invalid/link">Denied</a>
<script>
function request(kind) {
    const popup = window.open('https://allowed.invalid/' + kind);
    window.nsgo.postMessage(kind + ':' + Boolean(popup && !popup.closed));
}
setTimeout(() => request('timer'), 50);
</script>`

type popupTestAccess struct{}

func (popupTestAccess) Allows(core.AccessRequest) bool { return true }
func (popupTestAccess) WebHosts() []string             { return []string{"allowed.invalid"} }

func TestWebView2WindowRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a WebView2 window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	for _, c := range []struct {
		name   string
		listen bool
	}{{"reports", true}, {"suppresses", false}} {
		t.Run(c.name, func(t *testing.T) {
			if os.Getenv("NSGO_POPUP_TEST_PROCESS") == t.Name() {
				testWebView2Popup(t, c.listen)
				return
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestWebView2WindowRequests$/^"+c.name+"$", "-test.v")
			profile := webView2TestProfile(t)
			cmd.Env = append(os.Environ(), "NSGO_POPUP_TEST_PROCESS="+t.Name(), "WEBVIEW2_USER_DATA_FOLDER="+profile)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			if strings.Contains(string(out), "--- SKIP") {
				t.Skipf("%s", out)
			}
		})
	}
}

func webView2TestProfile(t *testing.T) string {
	t.Helper()
	profile, err := os.MkdirTemp("", "nsgo-webview2-test-")
	if err != nil {
		t.Fatal(err)
	}
	// WebView2's browser processes can still be closing when the test process exits.
	t.Cleanup(func() {
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(100 * time.Millisecond) {
			if err := os.RemoveAll(profile); err == nil {
				return
			} else if time.Now().After(deadline) {
				t.Logf("WebView2 test profile cleanup: %v", err)
				return
			}
		}
	})
	return profile
}

func testWebView2Popup(t *testing.T, listen bool) {
	requests := make(chan core.WebWindowRequest, 8)
	failures := make(chan error, 8)
	messages := make(chan string, 8)
	app := func(cx *core.Scope) core.Node {
		cx.ProvideAccess(popupTestAccess{})
		w := core.WebView{
			Source:    core.WebHTML(popupTestPage, ""),
			Style:     core.Style{Width: core.Pt(240), Height: core.Pt(160)},
			OnMessage: func(s string) { messages <- s },
			OnError:   func(err error) { failures <- err },
		}
		if listen {
			w.OnOpenWindow = func(r core.WebWindowRequest) { requests <- r }
		}
		return w
	}
	title := "NSGo popup test " + t.Name()
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 240, Height: 160})
	}()
	hwnd := waitForWindow(t, title, done)
	t.Cleanup(func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the popup test window did not close")
		}
	})
	expectMessage := func(want string) {
		t.Helper()
		select {
		case got := <-messages:
			if got != want {
				t.Fatalf("page message = %q, want %q", got, want)
			}
		case err := <-failures:
			t.Fatal(err)
		case <-time.After(20 * time.Second):
			t.Fatalf("no page message %q", want)
		}
	}
	expectMessage("timer:false")
	if !listen {
		clickPopupPage(t, hwnd, 48)
		expectMessage("click:false")
		return
	}
	for i, action := range []string{"link", "click", "form"} {
		clickPopupPage(t, hwnd, 16+32*i)
		want := "https://allowed.invalid/" + action
		deadline := time.After(10 * time.Second)
		for {
			select {
			case got := <-requests:
				if got.URL == "https://allowed.invalid/timer" {
					if got.UserInitiated {
						t.Error("the timer acquired a user gesture")
					}
					continue
				}
				if got.URL != want || !got.UserInitiated {
					t.Fatalf("request = %+v, want a user gesture to %s", got, want)
				}
			case err := <-failures:
				t.Fatal(err)
			case <-deadline:
				t.Fatalf("no request for %s", want)
			}
			break
		}
		if action == "click" {
			expectMessage("click:false")
		}
	}
	clickPopupPage(t, hwnd, 112)
	select {
	case err := <-failures:
		if web, ok := err.(*core.WebError); !ok || web.URL != "https://denied.invalid/link" {
			t.Errorf("denied window reported %v", err)
		}
	case r := <-requests:
		t.Errorf("denied window reached OnOpenWindow: %+v", r)
	case <-time.After(10 * time.Second):
		t.Error("no error for the denied window")
	}
}

// CDP delivers trusted mouse input inside the WebView2 session, even on a
// Windows desktop that does not let the harness move its physical cursor.
func clickPopupPage(t *testing.T, hwnd uintptr, y int) {
	t.Helper()
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		answer := make(chan error, 1)
		onMain(t, hwnd, func(w *window) {
			for web := range w.webs {
				params := fmt.Sprintf(`{"type":%q,"x":12,"y":%d,"button":"left","clickCount":1}`, kind, y)
				web.devTools("Input.dispatchMouseEvent", params, "dispatch mouse input", func(_ string, err error) { answer <- err })
				return
			}
			answer <- fmt.Errorf("no WebView2 browser")
		})
		select {
		case err := <-answer:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("WebView2 did not acknowledge input")
		}
	}
}
