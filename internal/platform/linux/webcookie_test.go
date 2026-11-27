//go:build linux && !android

package linux

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestSetWebCookiesReachesThePagesLoadedAfter sets an HttpOnly session cookie, an HttpOnly one with an expiry and one of another path before any web view exists, then shows a web view and an Incognito one of a local server's pages: the server must hear the first two from the web view and none from the Incognito one, and the page's script must not see the HttpOnly ones. A second run of the app, with the same website data, sends the cookie with an expiry but not the session one. WebKit keeps the first thread that uses it, so each run is a process of its own; it skips where WebKitGTK 6.0 is missing.
func TestSetWebCookiesReachesThePagesLoadedAfter(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if phase := os.Getenv("NSGO_TEST_WEBKIT_PROCESS"); strings.HasPrefix(phase, t.Name()+":") {
		cookieApp(t, strings.TrimPrefix(phase, t.Name()+":"))
		return
	}
	var mu sync.Mutex
	heard := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		heard[r.URL.Path] = r.Header.Get("Cookie")
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><script>window.nsgo.postMessage('script sees ' + document.cookie);</script>`))
	}))
	defer server.Close()
	dir, value := t.TempDir(), rand.Text()
	run := func(phase string) string {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name()+":"+phase, "NSGO_TEST_SERVER="+server.URL, "NSGO_TEST_VALUE="+value,
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", phase, err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return string(out)
	}
	out := run("set")
	mu.Lock()
	page, incognito := heard["/page"], heard["/incognito"]
	mu.Unlock()
	for _, want := range []string{"sid=" + value, "keep=" + value} {
		if !strings.Contains(page, want) {
			t.Errorf("the web view's request sent Cookie %q, want %s in it", page, want)
		}
	}
	if strings.Contains(page, "elsewhere=") {
		t.Errorf("the web view's request sent Cookie %q, with the cookie of another path", page)
	}
	if incognito != "" {
		t.Errorf("the Incognito web view's request sent Cookie %q, want none", incognito)
	}
	// The value is in no other line the app logs.
	if !strings.Contains(out, "script sees") || strings.Contains(out, value) {
		t.Errorf("the page's script did not report, or saw the HttpOnly cookies:\n%s", out)
	}
	run("again")
	mu.Lock()
	again := heard["/again"]
	mu.Unlock()
	if !strings.Contains(again, "keep="+value) {
		t.Errorf("after a restart the web view sent Cookie %q, want keep=%s in it", again, value)
	}
	if strings.Contains(again, "sid=") {
		t.Errorf("after a restart the web view sent Cookie %q, with the session cookie", again)
	}
}

// cookieApp is a run of TestSetWebCookiesReachesThePagesLoadedAfter's app: "set" sets the cookies and loads /page and /incognito, "again" loads /again; the window's title says when it is done.
func cookieApp(t *testing.T, phase string) {
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	server, value := os.Getenv("NSGO_TEST_SERVER"), os.Getenv("NSGO_TEST_VALUE")
	host := strings.TrimPrefix(server, "http://")
	host = host[:strings.LastIndexByte(host, ':')]
	title := "nsgo-native-cookies-" + phase
	app := func(cx *core.Scope) core.Node {
		ready := core.NewSignal(cx, phase == "again")
		loaded := core.NewSignal(cx, 0)
		failure, script := core.NewSignal(cx, ""), core.NewSignal(cx, "")
		cx.Effect(func() {
			if failure.Get() != "" {
				cx.SetTitle(title + "|" + failure.Get())
				return
			}
			want := 2
			if phase == "again" {
				want = 1
			}
			if loaded.Get() >= want && (phase == "again" || script.Get() != "") {
				cx.SetTitle(title + "|done")
			}
		})
		load := func(core.WebNavigation) { loaded.Update(func(n int) int { return n + 1 }) }
		if phase == "set" {
			cx.SetWebCookies([]core.WebCookie{
				{Name: "sid", Value: value, Domain: host, HTTPOnly: true, SameSite: core.WebSameSiteLax},
				{Name: "keep", Value: value, Domain: host, Path: "/", Expires: time.Now().Add(time.Hour), HTTPOnly: true},
				{Name: "elsewhere", Value: value, Domain: host, Path: "/elsewhere"},
			}, func(err error) {
				if err != nil {
					failure.Set(err.Error())
					return
				}
				ready.Set(true)
			})
		}
		size := core.Style{Width: driver.Pt(150), Height: driver.Pt(100)}
		message := func(m string) {
			t.Logf("%s", m)
			script.Set(m)
		}
		return core.View{Style: core.Style{Width: driver.Pt(320), Height: driver.Pt(120), Direction: core.Row}, Children: []core.Node{
			core.Show{When: ready.Get, Then: func(*core.Scope) core.Node {
				if phase == "again" {
					return core.WebView{Source: core.WebSrc(server + "/again"), Style: size, OnLoad: load}
				}
				return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
					core.WebView{Source: core.WebSrc(server + "/page"), Style: size, OnLoad: load, OnMessage: message},
					core.WebView{Source: core.WebSrc(server + "/incognito"), Style: size, Incognito: true, OnLoad: load},
				}}
			}},
		}}
	}
	w := startTestWindow(t, app, Options{ID: "nsgo-test-web-cookies", Title: title, Width: 320, Height: 120})
	waitForTitle(t, w, title+"|done")
}

// TestWebCookiesListsAndClearWebsiteDataClears has a local server's page set an HttpOnly cookie, store a value in localStorage and make an IndexedDB database, with SetWebCookies writing cookies of example.test and other.test beside it. Once the web view is gone, WebCookies must list the page's cookie by its URL and the example.test ones by their domain, every field filled in. A second run, with the same website data and no web view, must list them again, then ClearWebsiteData must leave the store empty and a page loaded after it must send no cookie and find neither the value nor the database. Each run is a process of its own; it skips where WebKitGTK 6.0 is missing.
func TestWebCookiesListsAndClearWebsiteDataClears(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if phase := os.Getenv("NSGO_TEST_WEBKIT_PROCESS"); strings.HasPrefix(phase, t.Name()+":") {
		cookieListApp(t, strings.TrimPrefix(phase, t.Name()+":"))
		return
	}
	var mu sync.Mutex
	heard := map[string]string{}
	value := rand.Text()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		heard[r.URL.Path] = r.Header.Get("Cookie")
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		// The page reports what it found and what it did once its storage settled.
		script := `const open = indexedDB.open('nsgo', 1); let made = false;
open.onupgradeneeded = () => { made = true; open.result.createObjectStore('s'); };
open.onsuccess = () => { open.result.close(); `
		if r.URL.Path == "/set" {
			w.Header().Set("Set-Cookie", "page="+value+"; HttpOnly; Path=/; Max-Age=120")
			script += `localStorage.setItem('k', '` + value + `'); window.nsgo.postMessage('stored'); };`
		} else {
			script += `window.nsgo.postMessage('storage ' + localStorage.getItem('k') + ' database ' + (made ? 'new' : 'kept')); };`
		}
		w.Write([]byte(`<!doctype html><script>` + script + `</script>`))
	}))
	defer server.Close()
	dir := t.TempDir()
	// Both runs set and look for the same expiry.
	expires := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	run := func(phase string) string {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBKIT_PROCESS="+t.Name()+":"+phase, "NSGO_TEST_SERVER="+server.URL, "NSGO_TEST_VALUE="+value, "NSGO_TEST_EXPIRES="+expires,
			"XDG_DATA_HOME="+filepath.Join(dir, "data"), "XDG_CACHE_HOME="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", phase, err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("%s", out)
		}
		return string(out)
	}
	run("set")
	out := run("clear")
	mu.Lock()
	check := heard["/check"]
	mu.Unlock()
	if check != "" {
		t.Errorf("after ClearWebsiteData the page's request sent Cookie %q, want none", check)
	}
	if !strings.Contains(out, "storage null database new") {
		t.Errorf("after ClearWebsiteData the page did not find empty storage:\n%s", out)
	}
}

// cookieListApp is a run of TestWebCookiesListsAndClearWebsiteDataClears's app: "set" loads /set, sets the example.test cookies, unmounts the web view and lists; "clear" lists, clears, lists and loads /check. The window's title says when it is done.
func cookieListApp(t *testing.T, phase string) {
	if err := openWebKit(); err != nil {
		t.Skip(err)
	}
	server, value := os.Getenv("NSGO_TEST_SERVER"), os.Getenv("NSGO_TEST_VALUE")
	host := strings.TrimPrefix(server, "http://")
	host = host[:strings.LastIndexByte(host, ':')]
	at, _ := strconv.ParseInt(os.Getenv("NSGO_TEST_EXPIRES"), 10, 64)
	expires := time.Unix(at, 0).UTC()
	set := []core.WebCookie{
		{Name: "d", Value: value, Domain: ".example.test", Path: "/", Expires: expires, Secure: true, HTTPOnly: true, SameSite: core.WebSameSiteStrict},
		{Name: "w", Value: value, Domain: "www.example.test", Path: "/watch", Expires: expires, SameSite: core.WebSameSiteLax},
		{Name: "o", Value: value, Domain: "other.test", Path: "/", Expires: expires, SameSite: core.WebSameSiteLax},
	}
	title := "nsgo-native-cookie-list-" + phase
	app := func(cx *core.Scope) core.Node {
		page := core.NewSignal(cx, "")
		done := func() { cx.SetTitle(title + "|done") }
		// checkListed holds the store's listings to what the page and SetWebCookies wrote, and calls next once both answered.
		checkListed := func(next func()) {
			cx.WebCookies(core.WebCookieFilter{URL: server + "/any"}, func(cookies []core.WebCookie, err error) {
				if err != nil || len(cookies) != 1 {
					t.Errorf("the page's URL listed %+v, %v; want its one cookie", cookies, err)
					done()
					return
				}
				c := cookies[0]
				if c.Name != "page" || c.Value != value || c.Domain != host || c.Path != "/" || !c.HTTPOnly || c.Secure || time.Until(c.Expires) < time.Minute || time.Until(c.Expires) > 3*time.Minute {
					t.Errorf("the page's cookie listed as %+v", c)
				}
				cx.WebCookies(core.WebCookieFilter{Domain: "example.test"}, func(cookies []core.WebCookie, err error) {
					if err != nil || len(cookies) != 2 || cookies[0] != set[0] || cookies[1] != set[1] {
						t.Errorf("example.test listed %+v, %v; want %+v", cookies, err, set[:2])
					}
					next()
				})
			})
		}
		switch phase {
		case "set":
			cx.SetWebCookies(set, func(err error) {
				if err != nil {
					t.Errorf("SetWebCookies: %v", err)
					done()
					return
				}
				page.Set("/set")
			})
		case "clear":
			checkListed(func() {
				cx.ClearWebsiteData(func(err error) {
					if err != nil {
						t.Errorf("ClearWebsiteData: %v", err)
					}
					cx.WebCookies(core.WebCookieFilter{}, func(cookies []core.WebCookie, err error) {
						if err != nil || len(cookies) != 0 {
							t.Errorf("after ClearWebsiteData the store listed %+v, %v", cookies, err)
						}
						// A public suffix libsoup drops must fail before any cookie in the batch is written.
						cx.SetWebCookies([]core.WebCookie{
							{Name: "before", Value: "1", Domain: ".example.test", Path: "/"},
							{Name: "pub", Value: "2", Domain: ".httpbin.org", Path: "/"},
						}, func(err error) {
							if err == nil {
								t.Errorf("SetWebCookies with public suffix .httpbin.org succeeded, want error")
							}
							cx.WebCookies(core.WebCookieFilter{}, func(cookies []core.WebCookie, err error) {
								if err != nil || len(cookies) != 0 {
									t.Errorf("after refused public-suffix batch the store listed %+v, %v", cookies, err)
								}
								page.Set("/check")
							})
						})
					})
				})
			})
		}
		message := func(m string) {
			t.Logf("%s", m)
			if m != "stored" {
				done()
				return
			}
			// The page that set the cookie goes before the store is listed.
			page.Set("")
			checkListed(done)
		}
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(150)}, Children: []core.Node{
			core.Show{When: func() bool { return page.Get() != "" }, Then: func(*core.Scope) core.Node {
				return core.WebView{Source: core.WebSrc(server + page.Get()), Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(150)}, OnMessage: message}
			}},
		}}
	}
	w := startTestWindow(t, app, Options{ID: "nsgo-test-web-cookie-list", Title: title, Width: 200, Height: 150})
	waitForTitle(t, w, title+"|done")
}
