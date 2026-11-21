//go:build darwin && !ios

package macos

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// TestWebMessagesComeOnlyFromTheSourcesPage loads a page with a frame of another origin, which posts to WebKit's handler itself and relays a message through the page, and which then goes to a page of a third origin. A web view reaches the app only from its source's page by default, one taking any origin from every page it shows, and neither from the frame; HTML without a base URL posts as "null". Run ends a process's app for good, so the test runs in a process of its own.
func TestWebMessagesComeOnlyFromTheSourcesPage(t *testing.T) {
	if os.Getenv("NSGO_TEST_WEB_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEB_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	pages := map[string]string{
		"nsgotest://main/index.html": `<!doctype html><script>
addEventListener('message', (e) => {
  if (e.data !== 'relay') return;
  window.nsgo.postMessage('relayed from ' + e.origin);
  location.href = 'nsgotest://other/next.html';
});
window.nsgo.postMessage('main');
</script><iframe src="nsgotest://frame/frame.html"></iframe>`,
		"nsgotest://frame/frame.html": `<!doctype html><script>
window.webkit.messageHandlers.nsgo.postMessage('frame');
parent.postMessage('relay', '*');
</script>`,
		"nsgotest://other/next.html": `<!doctype html><script>window.nsgo.postMessage('other');</script>`,
	}
	var mu sync.Mutex
	heard := map[string][]string{}
	var anyLoaded, ownLoaded, htmlHeard bool
	app := func(cx *core.Scope) core.Node {
		if err := cx.ServeScheme("nsgotest", func(url string) []byte { return []byte(pages[url]) }); err != nil {
			t.Error(err)
		}
		done := func() {
			if anyLoaded && ownLoaded && htmlHeard {
				cx.Quit()
			}
		}
		record := func(view string) func(core.WebMessage) {
			return func(m core.WebMessage) {
				mu.Lock()
				heard[view] = append(heard[view], m.Data+" @ "+m.Origin)
				mu.Unlock()
				if view == "html" {
					htmlHeard = true
					done()
				}
			}
		}
		loaded := func(flag *bool) func(core.WebNavigation) {
			return func(n core.WebNavigation) {
				if strings.HasSuffix(n.URL, "next.html") {
					*flag = true
					done()
				}
			}
		}
		size := core.Style{Width: core.Pt(200), Height: core.Pt(150)}
		return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
			core.WebView{Source: core.WebSrc("nsgotest://main/index.html"), Style: size,
				OnMessageFrom: record("own"), OnLoad: loaded(&ownLoaded)},
			core.WebView{Source: core.WebSrc("nsgotest://main/index.html"), Style: size, MessageOrigins: []string{"*"},
				OnMessageFrom: record("any"), OnLoad: loaded(&anyLoaded)},
			core.WebView{Source: core.WebHTML(`<script>window.nsgo.postMessage('html');</script>`, ""), Style: size,
				OnMessageFrom: record("html")},
		}}
	}
	watchdog := time.AfterFunc(30*time.Second, func() { panic("the web views did not load their pages") })
	defer watchdog.Stop()
	onMain(func() { Run(app, Options{Title: "Web messages", Width: 600, Height: 150}) })
	mu.Lock()
	defer mu.Unlock()
	for view, want := range map[string][]string{
		"own":  {"main @ nsgotest://main", "relayed from nsgotest://frame @ nsgotest://main"},
		"any":  {"main @ nsgotest://main", "relayed from nsgotest://frame @ nsgotest://main", "other @ nsgotest://other"},
		"html": {"html @ null"},
	} {
		if !slices.Equal(heard[view], want) {
			t.Errorf("the %s view heard %q, want %q", view, heard[view], want)
		}
	}
}

// TestWebSchemeRequestAnswers verifies a served scheme's requests reach its
// handler with their method and answer it with the status, headers and body
// its WebSchemeResponse gives, as a page's fetch sees them: a POST answered
// 405 without body, a GET answered 201 with a header the page reads back.
// Run ends a process's app for good, so the test runs in a process of its own.
func TestWebSchemeRequestAnswers(t *testing.T) {
	if os.Getenv("NSGO_TEST_WEB_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEB_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	page := `<!doctype html><script>
async function check() {
  let report = [];
  let r = await fetch('nsgotest://main/data', { method: 'POST', body: 'x' });
  report.push('post:' + r.status + ':' + r.headers.get('Allow') + ':' + await r.text());
  r = await fetch('nsgotest://main/ok.txt', { headers: { 'X-Probe': 'probe' } });
  report.push('get:' + r.status + ':' + r.headers.get('X-Served') + ':' + r.headers.get('Content-Type') + ':' + await r.text());
  r = await fetch('nsgotest://main/missing');
  report.push('missing:' + r.status);
  window.nsgo.postMessage(report.join('|'));
}
check().catch((e) => window.nsgo.postMessage('error:' + e.message));
</script>`
	var mu sync.Mutex
	var result string
	app := func(cx *core.Scope) core.Node {
		err := cx.ServeSchemeRequest("nsgotest", func(r *core.WebSchemeRequest) *core.WebSchemeResponse {
			if r.Method != "GET" {
				return &core.WebSchemeResponse{Status: 405, Headers: map[string]string{"Allow": "GET"}}
			}
			switch {
			case strings.HasSuffix(r.URL, "index.html"):
				return &core.WebSchemeResponse{Body: []byte(page)}
			case strings.HasSuffix(r.URL, "missing"):
				return nil
			default:
				return &core.WebSchemeResponse{Status: 201, Headers: map[string]string{"X-Served": "yes", "content-type": "text/x-nsgo"}, Body: []byte("bytes " + r.Header("x-probe"))}
			}
		})
		if err != nil {
			t.Error(err)
		}
		return core.WebView{Source: core.WebSrc("nsgotest://main/index.html"),
			Style: core.Style{Width: core.Pt(200), Height: core.Pt(150)},
			OnMessage: func(m string) {
				mu.Lock()
				result = m
				mu.Unlock()
				cx.Quit()
			},
		}
	}
	watchdog := time.AfterFunc(30*time.Second, func() { panic("the scheme requests did not answer") })
	defer watchdog.Stop()
	onMain(func() { Run(app, Options{Title: "Scheme requests", Width: 200, Height: 150}) })
	mu.Lock()
	defer mu.Unlock()
	if want := "post:405:GET:|get:201:yes:text/x-nsgo:bytes probe|missing:404"; result != want {
		t.Errorf("the page reported %q, want %q", result, want)
	}
}
