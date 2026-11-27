//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"reflect"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

func TestWebMountSendsFullConfig(t *testing.T) {
	m := newMounter(nil)
	m.item(&driver.MountBatch{}, &driver.MountItem{
		Kind: driver.ItemCreate,
		ID:   4,
		View: driver.KindWeb,
	})
	m.o.reset()
	item := driver.MountItem{
		Kind: driver.ItemWeb,
		ID:   4,
		Web: &driver.WebConfig{
			Source:                          driver.WebSource{Src: "https://example.test/", HTML: "<p>hi</p>", BaseURL: "https://example.test/"},
			NoJavaScript:                    true,
			InjectedJavaScriptBeforeContent: "window.before = \"start\";",
			InjectedJavaScript:              "window.after = 'end';\n",
			UserAgent:                       "Custom/1.0",
			UserAgentSuffix:                 "Suffix/2.0",
			Headers:                         map[string]string{"X-Token": "abc", "Accept-Language": "en"},
			Incognito:                       true,
			NoScroll:                        true,
			Muted:                           true,
			PinchZoom:                       true,
			DownloadDir:                     "/data/downloads",
			Restricted:                      true,
			Hosts:                           "example.test *.cdn.test",
			Schemes:                         "data blob",
			Media:                           driver.WebMicrophone | driver.WebCamera,
		},
	}
	m.item(&driver.MountBatch{}, &item)
	var got [][]any
	if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0]) != 3 || got[0][0] != "web" || got[0][1] != float64(4) {
		t.Fatalf("web operations = %v, want one web op for id 4", got)
	}
	raw, ok := got[0][2].(string)
	if !ok {
		t.Fatalf("web op payload = %T, want string", got[0][2])
	}
	var cfg harmonyWebConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	want := harmonyWebConfig{
		Src:             "https://example.test/",
		HTML:            "<p>hi</p>",
		BaseURL:         "https://example.test/",
		Before:          item.Web.InjectedJavaScriptBeforeContent,
		After:           item.Web.InjectedJavaScript,
		UserAgent:       "Custom/1.0",
		UserAgentSuffix: "Suffix/2.0",
		Headers: []harmonyWebHeader{
			{Key: "Accept-Language", Value: "en"},
			{Key: "X-Token", Value: "abc"},
		},
		Incognito:    true,
		NoScroll:     true,
		NoJavaScript: true,
		Muted:        true,
		PinchZoom:    true,
		DownloadDir:  "/data/downloads",
		Restricted:   true,
		Hosts:        []string{"example.test", "*.cdn.test"},
		Schemes:      []string{"data", "blob"},
		Media:        uint8(driver.WebMicrophone | driver.WebCamera),
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("decoded web config =\n%+v\nwant:\n%+v", cfg, want)
	}
	if m.views[4].web == nil || !m.views[4].web.Restricted {
		t.Fatalf("view did not retain cloned WebConfig: %+v", m.views[4].web)
	}
}

func TestWebMountSendsUpdatedInjectedScripts(t *testing.T) {
	m := newMounter(nil)
	m.item(&driver.MountBatch{}, &driver.MountItem{Kind: driver.ItemCreate, ID: 9, View: driver.KindWeb})
	m.o.reset()

	decodeOp := func() harmonyWebConfig {
		t.Helper()
		var ops [][]any
		if err := json.Unmarshal(m.o.bytes(), &ops); err != nil {
			t.Fatal(err)
		}
		if len(ops) != 1 || ops[0][0] != "web" || ops[0][1] != float64(9) {
			t.Fatalf("ops = %v, want one web op for id 9", ops)
		}
		var cfg harmonyWebConfig
		if err := json.Unmarshal([]byte(ops[0][2].(string)), &cfg); err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	m.item(&driver.MountBatch{}, &driver.MountItem{
		Kind: driver.ItemWeb,
		ID:   9,
		Web: &driver.WebConfig{
			Source:                          driver.WebSource{Src: "https://example.test/"},
			InjectedJavaScriptBeforeContent: "window.before = 'v1';",
			InjectedJavaScript:              "window.after = 'v1';",
		},
	})
	first := decodeOp()
	if first.Before != "window.before = 'v1';" || first.After != "window.after = 'v1';" {
		t.Fatalf("initial scripts = %+v", first)
	}

	m.o.reset()
	m.item(&driver.MountBatch{}, &driver.MountItem{
		Kind: driver.ItemWeb,
		ID:   9,
		Web: &driver.WebConfig{
			Source:                          driver.WebSource{Src: "https://example.test/"},
			InjectedJavaScriptBeforeContent: "window.before = 'v2';",
			InjectedJavaScript:              "window.after = 'v2';",
		},
	})
	second := decodeOp()
	if second.Before != "window.before = 'v2';" || second.After != "window.after = 'v2';" {
		t.Fatalf("updated scripts = %+v", second)
	}
}

func TestWebCommandSerialization(t *testing.T) {
	m := newMounter(nil)
	loadSrc := driver.WebSource{HTML: "<b>loaded</b>", BaseURL: "https://example.test/"}
	for _, it := range []driver.MountItem{
		{Kind: driver.ItemWebCommand, ID: 7, WebCommand: driver.WebReload},
		{Kind: driver.ItemWebCommand, ID: 7, WebCommand: driver.WebGoBack},
		{Kind: driver.ItemWebCommand, ID: 7, WebCommand: driver.WebGoForward},
		{Kind: driver.ItemWebCommand, ID: 7, WebCommand: driver.WebStopLoading},
		{Kind: driver.ItemWebCommand, ID: 7, WebCommand: driver.WebPostMessage, Value: "hello"},
		{Kind: driver.ItemWebCommand, ID: 7, WebCommand: driver.WebInjectJavaScript, Value: "1+1"},
		{Kind: driver.ItemWebCommand, ID: 7, WebCommand: driver.WebLoad, Data: &loadSrc},
		{Kind: driver.ItemWebCommand, ID: 7, WebCommand: driver.WebSendInput, Data: &driver.WebInput{Kind: driver.WebInputClick}},
	} {
		m.item(&driver.MountBatch{}, &it)
	}
	var got [][]any
	if err := json.Unmarshal(m.o.bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 {
		t.Fatalf("got %d ops (%v), want 7 (WebSendInput omitted)", len(got), got)
	}
	if got[4][2] != float64(driver.WebPostMessage) || got[4][3] != driver.WebMessageScript("hello") {
		t.Fatalf("WebPostMessage op = %v, want script %q", got[4], driver.WebMessageScript("hello"))
	}
	if got[5][2] != float64(driver.WebInjectJavaScript) || got[5][3] != "1+1" {
		t.Fatalf("WebInjectJavaScript op = %v", got[5])
	}
	if got[6][2] != float64(driver.WebLoad) {
		t.Fatalf("WebLoad op = %v", got[6])
	}
	var loaded harmonyWebLoad
	if err := json.Unmarshal([]byte(got[6][3].(string)), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded != (harmonyWebLoad{HTML: "<b>loaded</b>", BaseURL: "https://example.test/"}) {
		t.Fatalf("WebLoad payload = %+v", loaded)
	}
}

func TestWebEventRouting(t *testing.T) {
	bridge := js.Global().Get("Object").New()
	noop := js.FuncOf(func(js.Value, []js.Value) any { return nil })
	t.Cleanup(noop.Release)
	bridge.Set("submit", noop)
	bridge.Set("tick", noop)
	s := &surface{bridge: bridge}
	s.m = newMounter(s)

	var webID driver.NodeID
	ready := make(chan func(func()), 1)
	var loadStarts, loads, navs []core.WebNavigation
	var messages []core.WebMessage
	var downloads []core.WebDownload
	var openWins []core.WebWindowRequest
	var errs []*core.WebError

	s.engine = engine.Start(s, func(cx *core.Scope) core.Node {
		ready <- cx.Post
		return core.WebView{
			Source:                  core.WebSrc("https://example.test/page"),
			OnLoadStart:             func(n core.WebNavigation) { loadStarts = append(loadStarts, n) },
			OnLoad:                  func(n core.WebNavigation) { loads = append(loads, n) },
			OnNavigationStateChange: func(n core.WebNavigation) { navs = append(navs, n) },
			OnMessageFrom:           func(m core.WebMessage) { messages = append(messages, m) },
			OnDownload:              func(d core.WebDownload) { downloads = append(downloads, d) },
			OnOpenWindow:            func(r core.WebWindowRequest) { openWins = append(openWins, r) },
			OnError: func(err error) {
				if we, ok := err.(*core.WebError); ok {
					errs = append(errs, we)
				}
			},
		}
	}, engine.Options{
		Environment: driver.Environment{HasWebView: true},
	})
	t.Cleanup(s.engine.Close)

	var post func(func())
	select {
	case post = <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not start")
	}
	// Wait for the initial mount batch to commit to s.m.views.
	mounted := make(chan struct{})
	post(func() { close(mounted) })
	select {
	case <-mounted:
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not mount initial batch")
	}

	for id, v := range s.m.views {
		if v.kind == driver.KindWeb {
			webID = id
		}
	}
	if webID == 0 {
		t.Fatal("no KindWeb view mounted")
	}

	fire := func(args ...any) {
		jsArgs := make([]js.Value, len(args))
		for i, a := range args {
			jsArgs[i] = js.ValueOf(a)
		}
		s.event(jsArgs)
	}

	// Load start + finish + title/same-document navigation change.
	fire("webloadstart", int(webID), 1, 0, "https://example.test/page", "")
	fire("webload", int(webID), 3, 0, "https://example.test/page", "Page Title")
	fire("webnav", int(webID), 1, 0, "https://example.test/page#sec", "Updated Title")
	// Bridge message from allowed source origin.
	fire("webmessage", int(webID), 0, 0, "ping", "https://example.test/page")
	// Download finished.
	fire("webdownload", int(webID), 0, 0, `{"state":1,"id":"d1","url":"https://example.test/f.txt","filename":"f.txt","path":"/tmp/d1/f.txt","bytes":12,"message":""}`)
	// Open window (unrestricted view allows it).
	fire("webopenwindow", int(webID), 1, 0, "https://other.test/popup")
	// Native error with domain/code and refused navigation without domain/code.
	fire("weberror", int(webID), -105, 1, "https://missing.test/", "ERR_NAME_NOT_RESOLVED")
	fire("weberror", int(webID), 0, 0, "https://blocked.test/", driver.RefusedNavigation("https://blocked.test/"))

	// Restricted view openWindow refusal check.
	s.m.views[webID].web.Restricted = true
	s.m.views[webID].web.Hosts = "example.test"
	fire("webopenwindow", int(webID), 1, 0, "https://blocked.test/popup")

	// Synchronize with engine goroutine.
	done := make(chan struct{})
	post(func() { close(done) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not drain web events")
	}

	if len(loadStarts) != 1 || !loadStarts[0].Loading || !loadStarts[0].CanGoBack || loadStarts[0].CanGoForward {
		t.Fatalf("loadStarts = %+v", loadStarts)
	}
	if len(loads) != 1 || loads[0].Loading || loads[0].Title != "Page Title" || !loads[0].CanGoBack || !loads[0].CanGoForward {
		t.Fatalf("loads = %+v", loads)
	}
	if len(navs) != 3 || navs[2].URL != "https://example.test/page#sec" || navs[2].Title != "Updated Title" {
		t.Fatalf("navs = %+v", navs)
	}
	if len(messages) != 1 || messages[0].Data != "ping" || messages[0].Origin != "https://example.test" {
		t.Fatalf("messages = %+v", messages)
	}
	if len(downloads) != 1 || downloads[0].State != core.WebDownloadFinished || downloads[0].Bytes != 12 || downloads[0].Filename != "f.txt" {
		t.Fatalf("downloads = %+v", downloads)
	}
	if len(openWins) != 1 || openWins[0].URL != "https://other.test/popup" || !openWins[0].UserInitiated {
		t.Fatalf("openWins = %+v", openWins)
	}
	if len(errs) != 3 {
		t.Fatalf("errs = %+v, want 3", errs)
	}
	if errs[0].Domain != "ohos.web.webview" || errs[0].Code != -105 || errs[0].Message != "ERR_NAME_NOT_RESOLVED" {
		t.Fatalf("native error = %+v", errs[0])
	}
	if errs[1].Domain != "" || errs[1].Code != 0 || errs[1].URL != "https://blocked.test/" {
		t.Fatalf("refused error = %+v", errs[1])
	}
	if errs[2].Domain != "" || errs[2].Code != 0 || errs[2].URL != "https://blocked.test/popup" {
		t.Fatalf("restricted openWindow error = %+v", errs[2])
	}
}

func TestWebCookieStoreAnswersUnavailable(t *testing.T) {
	bridge := js.Global().Get("Object").New()
	submit := js.FuncOf(func(js.Value, []js.Value) any { return nil })
	t.Cleanup(submit.Release)
	bridge.Set("submit", submit)
	bridge.Set("tick", submit)
	s := &surface{bridge: bridge}
	s.m = newMounter(s)

	type result struct {
		name string
		err  error
	}
	results := make(chan result, 5)
	answer := func(name string) func(error) {
		return func(err error) { results <- result{name, err} }
	}
	answers := make(chan driver.Event, 5)
	s.engine = engine.Start(s, func(cx *core.Scope) core.Node {
		cx.SetWebCookies([]core.WebCookie{{Name: "session", Domain: "example.test", Value: "test"}}, answer("set"))
		for name, filter := range map[string]core.WebCookieFilter{
			"all":    {},
			"domain": {Domain: "example.test"},
			"url":    {URL: "https://example.test/"},
		} {
			cx.WebCookies(filter, func(cookies []core.WebCookie, err error) {
				if cookies != nil {
					t.Errorf("%s listed %v, want nil cookies", name, cookies)
				}
				answer(name)(err)
			})
		}
		cx.ClearWebsiteData(answer("clear"))
		return core.View{}
	}, engine.Options{
		Environment: driver.Environment{HasWebView: true},
		Event: func(_ uint32, ev driver.Event) {
			if ev.Kind == driver.EventWebCookies {
				answers <- ev
			}
		},
	})
	t.Cleanup(s.engine.Close)

	pending := map[string]bool{"set": true, "all": true, "domain": true, "url": true, "clear": true}
	counts := map[uint32]bool{}
	deadline := time.After(2 * time.Second)
	for range 5 {
		select {
		case got := <-results:
			if !pending[got.name] || core.ErrorCode(got.err) != core.CodeUnavailable {
				t.Errorf("%s callback = %v, want one Unavailable answer", got.name, got.err)
			}
			delete(pending, got.name)
		case <-deadline:
			t.Fatalf("web cookie callbacks still pending: %v", pending)
		}
		select {
		case ev := <-answers:
			if ev.Count == 0 || counts[ev.Count] {
				t.Errorf("duplicate or missing request ID in %+v", ev)
			}
			counts[ev.Count] = true
		case <-deadline:
			t.Fatal("web cookie callback had no backend event")
		}
	}
}

func TestWebCookiesBridgeRoundTrip(t *testing.T) {
	bridge := js.Global().Get("Object").New()
	noop := js.FuncOf(func(js.Value, []js.Value) any { return nil })
	t.Cleanup(noop.Release)
	bridge.Set("submit", noop)
	bridge.Set("tick", noop)

	s := &surface{bridge: bridge}
	s.m = newMounter(s)

	var (
		stored       []harmonyWebCookieWire
		failSet      bool
		useHeader    bool
		clears       int
		failClear    bool
		bridgeCalled int
	)

	fire := func(args ...any) {
		jsArgs := make([]js.Value, len(args))
		for i, a := range args {
			jsArgs[i] = js.ValueOf(a)
		}
		s.event(jsArgs)
	}

	webcookiesFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		bridgeCalled++
		id := args[0].Int()
		var req harmonyWebCookiesRequest
		if err := json.Unmarshal([]byte(args[1].String()), &req); err != nil {
			fire("webcookies", id, 0, 0, "", err.Error())
			return nil
		}
		switch driver.WebCookiesOp(req.Op) {
		case driver.WebCookiesSet:
			if failSet {
				fire("webcookies", id, 0, 0, "", "HarmonyOS WebCookieManager refused the cookie SID: invalid")
				return nil
			}
			for _, c := range req.Cookies {
				stored = append(stored, harmonyWebCookieWire{
					Name:     c.Name,
					Value:    "val-" + c.Name,
					Domain:   ".example.test",
					Path:     "/",
					Expires:  "Wed, 02 Jan 2030 03:04:05 GMT",
					Secure:   true,
					HTTPOnly: true,
					SameSite: uint8(driver.WebSameSiteNone),
				})
			}
			fire("webcookies", id, 0, 0, "", "")
		case driver.WebCookiesList:
			if useHeader {
				if req.URL == "" {
					fire("webcookies", id, 0, 0, "", driver.NoWebCookieDomains)
					return nil
				}
				fire("webcookies", id, harmonyWebCookiesModeHeader, 0, "SID=val-SID; theme=dark", "")
				return nil
			}
			raw, _ := json.Marshal(stored)
			fire("webcookies", id, harmonyWebCookiesModeAll, 0, string(raw), "")
		case driver.WebCookiesClear:
			if failClear {
				fire("webcookies", id, 0, 0, "", "clear failed")
				return nil
			}
			clears++
			stored = nil
			fire("webcookies", id, 0, 0, "", "")
		}
		return nil
	})
	t.Cleanup(webcookiesFn.Release)
	bridge.Set("webcookies", webcookiesFn)

	scopeCh := make(chan *core.Scope, 1)
	s.engine = engine.Start(s, func(cx *core.Scope) core.Node {
		scopeCh <- cx
		return core.View{}
	}, engine.Options{
		Environment: driver.Environment{HasWebView: true},
	})
	t.Cleanup(s.engine.Close)

	var cx *core.Scope
	select {
	case cx = <-scopeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not start")
	}

	run := func(fn func(*core.Scope)) {
		t.Helper()
		done := make(chan struct{})
		cx.Post(func() {
			fn(cx)
			close(done)
		})
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("post timed out")
		}
	}

	// 1. SetWebCookies succeeds and writes via bridge.
	setErr := make(chan error, 1)
	run(func(cx *core.Scope) {
		cx.SetWebCookies([]core.WebCookie{{
			Name:     "SID",
			Value:    "secret",
			Domain:   ".example.test",
			Expires:  time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
			Secure:   true,
			HTTPOnly: true,
			SameSite: core.WebSameSiteNone,
		}}, func(err error) { setErr <- err })
	})
	if err := <-setErr; err != nil {
		t.Fatalf("SetWebCookies err = %v", err)
	}

	// 2. WebCookies (API 23+ mode 1) lists full metadata for Domain, URL, and zero filters.
	type listRes struct {
		cookies []core.WebCookie
		err     error
	}
	listCh := make(chan listRes, 3)
	run(func(cx *core.Scope) {
		cx.WebCookies(core.WebCookieFilter{Domain: "example.test"}, func(c []core.WebCookie, err error) { listCh <- listRes{c, err} })
		cx.WebCookies(core.WebCookieFilter{URL: "https://app.example.test/"}, func(c []core.WebCookie, err error) { listCh <- listRes{c, err} })
		cx.WebCookies(core.WebCookieFilter{URL: "http://app.example.test/"}, func(c []core.WebCookie, err error) { listCh <- listRes{c, err} })
	})
	rDomain := <-listCh
	rHTTPS := <-listCh
	rHTTP := <-listCh
	if rDomain.err != nil || len(rDomain.cookies) != 1 || rDomain.cookies[0].Name != "SID" || !rDomain.cookies[0].HTTPOnly || rDomain.cookies[0].SameSite != core.WebSameSiteNone {
		t.Fatalf("Domain list = %+v", rDomain)
	}
	if rHTTPS.err != nil || len(rHTTPS.cookies) != 1 || rHTTPS.cookies[0].Name != "SID" {
		t.Fatalf("HTTPS URL list = %+v", rHTTPS)
	}
	if rHTTP.err != nil || len(rHTTP.cookies) != 0 {
		t.Fatalf("HTTP URL list should exclude Secure cookie, got %+v", rHTTP)
	}

	// 3. Fallback header mode (API < 23): URL lists names/values, Domain answers Unavailable.
	useHeader = true
	run(func(cx *core.Scope) {
		cx.WebCookies(core.WebCookieFilter{URL: "https://app.example.test/"}, func(c []core.WebCookie, err error) { listCh <- listRes{c, err} })
		cx.WebCookies(core.WebCookieFilter{Domain: "example.test"}, func(c []core.WebCookie, err error) { listCh <- listRes{c, err} })
	})
	rFallbackURL := <-listCh
	rFallbackDomain := <-listCh
	if rFallbackURL.err != nil || len(rFallbackURL.cookies) != 2 || rFallbackURL.cookies[0].Name != "SID" || rFallbackURL.cookies[0].Domain != "app.example.test" || rFallbackURL.cookies[1].Name != "theme" {
		t.Fatalf("fallback URL list = %+v", rFallbackURL)
	}
	if core.ErrorCode(rFallbackDomain.err) != core.CodeUnavailable || rFallbackDomain.cookies != nil {
		t.Fatalf("fallback Domain list = %+v, want Unavailable", rFallbackDomain)
	}
	useHeader = false

	// 4. ClearWebsiteData clears store and answers nil.
	clearErr := make(chan error, 1)
	run(func(cx *core.Scope) {
		cx.ClearWebsiteData(func(err error) { clearErr <- err })
	})
	if err := <-clearErr; err != nil || clears != 1 {
		t.Fatalf("ClearWebsiteData err = %v, clears = %d", err, clears)
	}

	// 5. Error propagation on SetWebCookies and ClearWebsiteData (done always called).
	failSet = true
	failClear = true
	run(func(cx *core.Scope) {
		cx.SetWebCookies([]core.WebCookie{{Name: "SID", Value: "s", Domain: "example.test"}}, func(err error) { setErr <- err })
		cx.ClearWebsiteData(func(err error) { clearErr <- err })
	})
	if err := <-setErr; err == nil {
		t.Fatal("failed SetWebCookies should return error")
	}
	if err := <-clearErr; err == nil {
		t.Fatal("failed ClearWebsiteData should return error")
	}
	if bridgeCalled < 8 {
		t.Fatalf("expected bridge.webcookies calls, got %d", bridgeCalled)
	}
	if len(s.m.cookieFilters) != 0 {
		t.Fatalf("leaked pending cookieFilters: %+v", s.m.cookieFilters)
	}
}
