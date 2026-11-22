//go:build windows

package windows

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestSetWebCookiesReachesThePagesLoadedAfter sets an HttpOnly session cookie, an HttpOnly one with an expiry and one of another path before any web view exists, which a hidden web view writes, then shows a web view and an Incognito one of a local server's pages: the server must hear the first two from the web view and none from the Incognito one, and the page's script must not see the HttpOnly ones. A second run of the app, with the same profile, sends the cookie with an expiry but not the session one. Each run is a process of its own, as WebView2 keeps one environment per profile and thread.
func TestSetWebCookiesReachesThePagesLoadedAfter(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	if phase := os.Getenv("NSGO_TEST_WEBVIEW2_PROCESS"); strings.HasPrefix(phase, t.Name()+":") {
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
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBVIEW2_PROCESS="+t.Name()+":"+phase, "NSGO_TEST_SERVER="+server.URL,
			"NSGO_TEST_VALUE="+value, "APPDATA="+dir, "LOCALAPPDATA="+dir)
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
	run("expire")
	mu.Lock()
	expired := heard["/expire"]
	mu.Unlock()
	if strings.Contains(expired, "keep=") {
		t.Errorf("an expired write left the persistent cookie in Cookie %q", expired)
	}
}

// cookieApp is a run of TestSetWebCookiesReachesThePagesLoadedAfter's app: "set" sets the cookies and loads /page and /incognito, "again" loads /again, and "expire" removes the persistent cookie before loading /expire; the window's title says when it is done.
func cookieApp(t *testing.T, phase string) {
	server, value := os.Getenv("NSGO_TEST_SERVER"), os.Getenv("NSGO_TEST_VALUE")
	host := strings.TrimPrefix(server, "http://")
	host = host[:strings.LastIndexByte(host, ':')]
	title := "nsgo-web-cookies-" + phase
	app := func(cx *core.Scope) core.Node {
		ready := core.NewSignal(cx, phase == "again")
		loaded := core.NewSignal(cx, 0)
		failure, script := core.NewSignal(cx, ""), core.NewSignal(cx, "")
		cx.Effect(func() {
			want := 2
			if phase != "set" {
				want = 1
			}
			switch {
			case failure.Get() != "":
				cx.SetTitle(title + "|" + failure.Get())
			case loaded.Get() >= want && (phase != "set" || script.Get() != ""):
				cx.SetTitle(title + "|done")
			}
		})
		load := func(core.WebNavigation) { loaded.Update(func(n int) int { return n + 1 }) }
		if phase == "set" {
			cx.SetWebCookies([]core.WebCookie{
				{Name: "sid", Value: value, Domain: host, HTTPOnly: true, SameSite: core.WebSameSiteLax},
				{Name: "keep", Value: value, Domain: host, Path: "/", Expires: time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC), HTTPOnly: true},
				{Name: "elsewhere", Value: value, Domain: host, Path: "/elsewhere"},
			}, func(err error) {
				if err != nil {
					failure.Set(err.Error())
					return
				}
				ready.Set(true)
			})
		}
		if phase == "expire" {
			cx.SetWebCookies([]core.WebCookie{{Name: "keep", Value: value, Domain: host, Path: "/",
				Expires: time.Date(1601, time.January, 1, 0, 0, 0, 0, time.UTC)}}, func(err error) {
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
				if phase != "set" {
					return core.WebView{Source: core.WebSrc(server + "/" + phase), Style: size, OnLoad: load}
				}
				return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
					core.WebView{Source: core.WebSrc(server + "/page"), Style: size, OnLoad: load, OnMessage: message},
					core.WebView{Source: core.WebSrc(server + "/incognito"), Style: size, Incognito: true, OnLoad: load},
				}}
			}},
		}}
	}
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 320, Height: 120})
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
	deadline := time.Now().Add(30 * time.Second)
	for got := windowText(hwnd); got != title+"|done"; got = windowText(hwnd) {
		if time.Now().After(deadline) || strings.HasPrefix(got, title+"|") {
			t.Fatalf("window title = %q, want %q", got, title+"|done")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestWebCookiesListsAndClearWebsiteDataClears has a local server's page set an HttpOnly cookie, store a value in localStorage and make an IndexedDB database, with SetWebCookies writing cookies of example.test and other.test beside it. Once the web view is gone, WebCookies must list, through a hidden web view, the page's cookie by its URL and the example.test ones by their domain, every field filled in. A second run, with the same profile and no web view, must list them again, then ClearWebsiteData must leave the store empty and a page loaded after it must send no cookie and find neither the value nor the database. Each run is a process of its own.
func TestWebCookiesListsAndClearWebsiteDataClears(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if _, err := findWebView2(); err != nil {
		t.Skip(err)
	}
	if phase := os.Getenv("NSGO_TEST_WEBVIEW2_PROCESS"); strings.HasPrefix(phase, t.Name()+":") {
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
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEBVIEW2_PROCESS="+t.Name()+":"+phase, "NSGO_TEST_SERVER="+server.URL,
			"NSGO_TEST_VALUE="+value, "NSGO_TEST_EXPIRES="+expires, "APPDATA="+dir, "LOCALAPPDATA="+dir)
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
	title := "nsgo-web-cookie-list-" + phase
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
						page.Set("/check")
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
	finished := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(finished)
		Run(app, Options{Title: title, Width: 200, Height: 150})
	}()
	hwnd := waitForWindow(t, title, finished)
	if hwnd == 0 {
		return
	}
	defer func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("the window did not close")
		}
	}()
	// The first start of the browser process can take a while.
	deadline := time.Now().Add(30 * time.Second)
	for got := windowText(hwnd); got != title+"|done"; got = windowText(hwnd) {
		if time.Now().After(deadline) {
			t.Fatalf("window title = %q, want %q", got, title+"|done")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
