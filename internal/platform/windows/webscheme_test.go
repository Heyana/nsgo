//go:build windows

package windows

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// schemePage is the page TestWebSchemeRequestAnswers serves, which fetches the scheme and reports what each answer was: a POST answered 405, a GET answered 201 with a response header and its Content-Type, and a missing path answered 404.
const schemePage = `<!doctype html><script>
async function check() {
  let report = [];
  let r = await fetch('nsgotest://main/data', { method: 'POST', body: 'x' });
  report.push('post:' + r.status + ':' + r.headers.get('Allow') + ':' + await r.text());
  r = await fetch('nsgotest://main/ok');
  report.push('get:' + r.status + ':' + r.headers.get('X-Served') + ':' + r.headers.get('Content-Type') + ':' + await r.text());
  r = await fetch('nsgotest://main/missing');
  report.push('missing:' + r.status);
  window.nsgo.postMessage(report.join('|'));
}
check().catch((e) => window.nsgo.postMessage('error:' + e.message));
</script>`

// TestWebSchemeRequestAnswers has a page of a served scheme fetch it: the handler sees each request's method and answers its status, headers and body, as the page's fetch sees them. It runs in a process of its own, with a profile of its own.
func TestWebSchemeRequestAnswers(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	if os.Getenv("NSGO_TEST_WEBVIEW2_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBVIEW2_PROCESS="+t.Name(), "LOCALAPPDATA="+webView2TestProfile(t))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	var mu sync.Mutex
	result := ""
	title := "nsgo-web-scheme"
	app := func(cx *core.Scope) core.Node {
		err := cx.ServeSchemeRequest("nsgotest", func(r *core.WebSchemeRequest) *core.WebSchemeResponse {
			switch {
			case strings.HasSuffix(r.URL, "index.html"):
				return &core.WebSchemeResponse{Body: []byte(schemePage)}
			case r.Method != "GET":
				return &core.WebSchemeResponse{Status: 405, Headers: map[string]string{"Allow": "GET"}}
			case strings.HasSuffix(r.URL, "missing"):
				return nil
			default:
				return &core.WebSchemeResponse{Status: 201, Headers: map[string]string{"X-Served": "yes", "content-type": "text/x-nsgo"}, Body: []byte("bytes")}
			}
		})
		if err != nil {
			t.Error(err)
		}
		return core.WebView{Source: core.WebSrc("nsgotest://main/index.html"),
			Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(150)},
			OnMessage: func(m string) {
				mu.Lock()
				result = m
				mu.Unlock()
			}}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 200, Height: 150})
	}()
	hwnd := waitForWindow(t, title, done)
	if hwnd == 0 {
		return
	}
	defer func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the window did not close")
		}
	}()
	// The first start of the browser process can take a while.
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		mu.Lock()
		got := result
		mu.Unlock()
		if got != "" {
			if want := "post:405:GET:|get:201:yes:text/x-nsgo:bytes|missing:404"; got != want {
				t.Errorf("the page reported %q, want %q", got, want)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the page did not report")
		}
	}
}
