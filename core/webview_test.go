package core

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestEmptyWebSourceLoadsBlank(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		name := "unrestricted"
		if restricted {
			name = "restricted"
		}
		t.Run(name, func(t *testing.T) {
			var ref Ref
			var source *Signal[WebSource]
			h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
				if restricted {
					cx.ProvideAccess(hostsAccess{})
				}
				source = NewSignal(cx, WebSource{})
				return WebView{SourceFunc: source.Get, Ref: &ref}
			})
			check := func(want WebSource) {
				t.Helper()
				var sources []WebSource
				for _, item := range h.commit().Items {
					if item.Kind == driver.ItemWeb {
						sources = append(sources, item.Web.Source)
					}
				}
				if !slices.Equal(sources, []WebSource{want}) {
					t.Fatalf("sent sources %v, want %v", sources, want)
				}
			}
			blank := WebSrc("about:blank")
			check(blank)
			page := WebHTML("<p>private</p>", "")
			source.Set(page)
			check(page)
			source.Set(WebSource{})
			check(blank)
			if kinds(h.commit())[driver.ItemWeb] != 0 {
				t.Fatal("an unchanged blank source loaded again")
			}
			// A repeated explicit blank load navigates again, and a base URL with nothing to resolve changes nothing.
			for _, src := range []WebSource{{}, {}, {BaseURL: "https://app.example/"}} {
				ref.Load(src)
				b := h.commit()
				if len(b.Items) != 1 || b.Items[0].WebCommand != driver.WebLoad {
					t.Fatalf("explicit blank load sent %v", b.Items)
				}
				if got := b.Items[0].Data.(*driver.WebSource); *got != blank {
					t.Fatalf("explicit blank load sent %+v", got)
				}
			}
		})
	}
}

func TestBlankWebNavigationHasNoSyntheticTitle(t *testing.T) {
	var got WebNavigation
	h := newHost(t, Size{Width: 100, Height: 100}, func(*Scope) Node {
		return WebView{TestID: "web", OnLoad: func(nav WebNavigation) { got = nav }}
	})
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventLoad, ID: h.find("web").id,
		Navigation: &driver.WebNavigation{URL: "about:blank", Title: "about:blank"}})
	if got.URL != "about:blank" || got.Title != "" {
		t.Fatalf("blank navigation = %+v, want an empty title", got)
	}
}

func TestWebViewConfiguresLoadsAndReports(t *testing.T) {
	var ref Ref
	var src *Signal[WebSource]
	var before *Signal[string]
	var muted *Signal[bool]
	var loaded []WebNavigation
	var messages []string
	h := newHost(t, Size{Width: 300, Height: 400}, func(cx *Scope) Node {
		src = NewSignal(cx, WebSrc("https://example.com"))
		before = NewSignal(cx, "before")
		muted = NewSignal(cx, true)
		return WebView{
			SourceFunc:                          src.Get,
			InjectedJavaScriptBeforeContentFunc: before.Get,
			InjectedJavaScript:                  "1",
			Headers:                             map[string]string{"X-Test": "one"},
			Incognito:                           true,
			PinchZoom:                           true,
			Mobile:                              true,
			MutedFunc:                           muted.Get,
			UserAgentSuffix:                     "TestBrowser/1.0",
			Ref:                                 &ref,
			TestID:                              "web",
			Style:                               Style{Width: Pt(300), Height: Pt(200)},
			OnLoad:                              func(n WebNavigation) { loaded = append(loaded, n) },
			OnMessage:                           func(d string) { messages = append(messages, d) },
		}
	})
	b := h.commit()
	var cfg *driver.WebConfig
	var mask driver.EventMask
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemWeb:
			cfg = it.Web
		case driver.ItemEvents:
			if it.ID == h.find("web").id {
				mask = it.Events
			}
		}
	}
	if n := h.find("web"); n.kind != driver.KindWeb {
		t.Fatalf("kind %v, want KindWeb", n.kind)
	}
	if cfg == nil || cfg.Source.Src != "https://example.com" || cfg.InjectedJavaScriptBeforeContent != "before" || cfg.InjectedJavaScript != "1" || cfg.Headers["X-Test"] != "one" || cfg.UserAgentSuffix != "TestBrowser/1.0" || !cfg.Incognito || !cfg.PinchZoom || !cfg.Mobile || !cfg.Muted {
		t.Fatalf("config %+v", cfg)
	}
	if !mask.Has(driver.EventLoad) || !mask.Has(driver.EventMessage) || mask.Has(driver.EventLoadStart) {
		t.Fatalf("mask %b", mask)
	}
	if k := kinds(h.commit()); k[driver.ItemWeb] != 0 {
		t.Fatalf("resent an unchanged config: %v", k)
	}

	before.Set("changed")
	b = h.commit()
	if len(b.Items) != 1 || b.Items[0].Kind != driver.ItemWeb || b.Items[0].Web.InjectedJavaScriptBeforeContent != "changed" {
		t.Fatalf("script change sent %v", b.Items)
	}

	src.Set(WebHTML("<p>hi</p>", ""))
	b = h.commit()
	if len(b.Items) != 1 || b.Items[0].Kind != driver.ItemWeb || !b.Items[0].Web.Muted {
		t.Fatalf("source change sent %+v", b.Items)
	}

	muted.Set(false)
	b = h.commit()
	if len(b.Items) != 1 || b.Items[0].Kind != driver.ItemWeb || b.Items[0].ID != h.find("web").id || b.Items[0].Web.Muted {
		t.Fatalf("unmuting sent %+v", b.Items)
	}

	ref.PostMessage("ping")
	ref.GoBack()
	b = h.commit()
	var cmds []driver.WebCommand
	for _, it := range b.Items {
		if it.Kind == driver.ItemWebCommand {
			cmds = append(cmds, it.WebCommand)
			if it.WebCommand == driver.WebPostMessage && it.Value != "ping" {
				t.Fatalf("message %q", it.Value)
			}
		}
	}
	if len(cmds) != 2 || cmds[0] != driver.WebPostMessage || cmds[1] != driver.WebGoBack {
		t.Fatalf("commands %v", cmds)
	}

	ref.Load(WebSrc("https://example.com"))
	b = h.commit()
	if len(b.Items) != 1 || b.Items[0].WebCommand != driver.WebLoad {
		t.Fatalf("load command %v", b.Items)
	}
	loadedSource, ok := b.Items[0].Data.(*driver.WebSource)
	if !ok || loadedSource.Src != "https://example.com" {
		t.Fatalf("load source %#v", b.Items[0].Data)
	}

	id := h.find("web").id
	h.tree.Dispatch(driver.Event{Kind: driver.EventLoad, ID: id, Navigation: &WebNavigation{URL: "about:srcdoc", CanGoBack: true}})
	h.tree.Dispatch(driver.Event{Kind: driver.EventMessage, ID: id, Text: "pong", Origin: "null", URL: "about:srcdoc", MainFrame: true})
	if len(loaded) != 1 || !loaded[0].CanGoBack || len(messages) != 1 || messages[0] != "pong" {
		t.Fatalf("loaded %v, messages %v", loaded, messages)
	}
}

// TestWebMessagesComeFromTheSourcesOrigin posts from the loaded page, from a frame within it and from pages of other origins, as backends report them, and checks that only the page reaches the app by default, the origins MessageOrigins lists when it is set, and never a frame; of the pages without an origin of their own ("null") only the source's.
func TestWebMessagesComeFromTheSourcesOrigin(t *testing.T) {
	type post struct {
		url       string
		mainFrame bool
	}
	posts := []post{
		{"https://app.example/index.html", true},
		{"https://app.example/frame.html", false}, // a frame of the page's own origin
		{"https://ads.example/", false},           // a third party's frame
		{"https://other.example/", true},          // a page the view navigated to
		{"about:blank", true},                     // HTML without a base URL
		{"data:text/html,<p>", true},              // a data: URL a frame sent the view to
		{"", true},                                // a backend that told no origin
		{"https://app.example:8443/", true},
	}
	for _, c := range []struct {
		name    string
		source  WebSource
		origins []string
		want    []string
	}{
		{"url", WebSrc("https://APP.example:443/index.html"), nil, []string{"https://app.example/index.html"}},
		{"html with a base", WebHTML("<p>", "https://app.example/app/"), nil, []string{"https://app.example/index.html"}},
		{"html without a base", WebHTML("<p>", ""), nil, []string{"about:blank"}},
		{"html without a base, listed", WebHTML("<p>", ""), []string{"null", "https://other.example"}, []string{"https://other.example/", "about:blank"}},
		{"data: url", WebSrc("data:text/html,<p>"), nil, []string{"data:text/html,<p>"}},
		{"listed", WebSrc("https://app.example/"), []string{"https://other.example/", "example.com"}, []string{"https://other.example/"}},
		{"listed null", WebSrc("https://app.example/"), []string{"null"}, nil},
		{"listed, none an origin", WebSrc("https://app.example/"), []string{"example.com"}, []string{"https://app.example/index.html"}},
		{"any", WebSrc("https://app.example/"), []string{"*"}, []string{"https://app.example/index.html", "https://other.example/", "about:blank", "data:text/html,<p>", "https://app.example:8443/"}},
		{"empty source", WebSource{}, nil, []string{"about:blank"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var strs []string
			var got []string
			h := newHost(t, Size{Width: 100, Height: 100}, func(*Scope) Node {
				return WebView{Source: c.source, MessageOrigins: c.origins, TestID: "web",
					OnMessage: func(d string) { strs = append(strs, d) },
					OnMessageFrom: func(m WebMessage) {
						if m.Origin != driver.WebOrigin(m.Data) {
							t.Errorf("%s posted as %s", m.Data, m.Origin)
						}
						got = append(got, m.Data)
					}}
			})
			h.commit()
			id := h.find("web").id
			for _, p := range posts {
				h.tree.Dispatch(driver.Event{Kind: driver.EventMessage, ID: id, Text: p.url, Origin: driver.WebOrigin(p.url), URL: p.url, MainFrame: p.mainFrame})
			}
			if !slices.Equal(got, c.want) || !slices.Equal(strs, c.want) {
				t.Errorf("OnMessageFrom heard %q and OnMessage %q, want %q", got, strs, c.want)
			}
		})
	}
}

// TestWebLoadUsesAccessRefusal loads a source Access refuses, twice, which reaches OnError each time and sends nothing, and HTML whose base URL it refuses, which loads without the base.
func TestWebLoadUsesAccessRefusal(t *testing.T) {
	var ref Ref
	var errors []error
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		cx.ProvideAccess(hostsAccess{hosts: []string{"ok.com"}})
		return WebView{Ref: &ref, Source: WebSrc("https://ok.com/"), OnError: func(err error) { errors = append(errors, err) }}
	})
	h.commit()
	for range 2 {
		ref.Load(WebSrc("https://evil.io/"))
		if b := h.commit(); len(b.Items) != 0 {
			t.Fatalf("a refused load sent %v", b.Items)
		}
	}
	h.drain()
	if len(errors) != 2 {
		t.Fatalf("refusal errors %v", errors)
	}
	if webErr, ok := errors[0].(*WebError); !ok || webErr.Domain != "" || webErr.Code != 0 || webErr.URL != "https://evil.io/" {
		t.Fatalf("refusal error %#v", errors[0])
	}
	ref.Load(WebHTML("<p>", "https://evil.io/"))
	b := h.commit()
	if len(b.Items) != 1 || b.Items[0].WebCommand != driver.WebLoad {
		t.Fatalf("html load %v", b.Items)
	}
	if src := b.Items[0].Data.(*driver.WebSource); *src != (driver.WebSource{HTML: "<p>"}) {
		t.Fatalf("html load sent %#v, want it without the refused base", src)
	}
	ref.Load(WebSrc("https://ok.com/next"))
	if b := h.commit(); len(b.Items) != 1 || b.Items[0].Data.(*driver.WebSource).Src != "https://ok.com/next" {
		t.Fatalf("allowed load %v", b.Items)
	}
}

// TestWebLoadLeavesTheSource loads another page, which navigates the view and leaves its source alone: nothing but the command is sent, and messages still come from the source's origin by default.
func TestWebLoadLeavesTheSource(t *testing.T) {
	var ref Ref
	var heard []string
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		return WebView{Ref: &ref, Source: WebSrc("https://app.example/"), TestID: "web",
			OnMessage: func(d string) { heard = append(heard, d) }}
	})
	h.commit()
	ref.Load(WebSrc("https://other.example/"))
	if b := h.commit(); len(b.Items) != 1 || b.Items[0].Kind != driver.ItemWebCommand {
		t.Fatalf("load sent %v", b.Items)
	}
	n := h.find("web")
	if n.web.config.Source.Src != "https://app.example/" || n.web.sent.Source.Src != "https://app.example/" {
		t.Fatalf("Load changed the source: %v, sent %v", n.web.config.Source, n.web.sent.Source)
	}
	for _, url := range []string{"https://other.example/", "https://app.example/"} {
		h.tree.Dispatch(driver.Event{Kind: driver.EventMessage, ID: n.id, Text: url, Origin: driver.WebOrigin(url), URL: url, MainFrame: true})
	}
	if !slices.Equal(heard, []string{"https://app.example/"}) {
		t.Errorf("heard %q, want only the source's origin", heard)
	}
}

func TestWebErrorCarriesFailure(t *testing.T) {
	var got *WebError
	h := newHost(t, Size{Width: 100, Height: 100}, func(*Scope) Node {
		return WebView{Ref: &Ref{}, OnError: func(err error) { got = err.(*WebError) }, TestID: "web"}
	})
	h.commit()
	id := h.find("web").id
	for _, tc := range []struct {
		url, msg, domain string
		code             int
	}{
		{"https://example.com", "failed", "test", 7},
		{"https://missing.invalid/", "net::ERR_NAME_NOT_RESOLVED", "android.webkit.WebViewClient", -2},
	} {
		got = nil
		h.tree.Dispatch(driver.Event{
			Kind:       driver.EventError,
			ID:         id,
			Text:       tc.msg,
			Navigation: &WebNavigation{URL: tc.url},
			Data:       &driver.WebFailure{Domain: tc.domain, Code: tc.code},
		})
		if got == nil || got.Domain != tc.domain || got.Code != tc.code || got.URL != tc.url || got.Message != tc.msg {
			t.Fatalf("error %#v, want domain=%q code=%d url=%q msg=%q", got, tc.domain, tc.code, tc.url, tc.msg)
		}
	}
}

func TestSendInputUsesEnvironmentCapability(t *testing.T) {
	var ref Ref
	h := newHost(t, Size{Width: 100, Height: 100}, func(*Scope) Node {
		return WebView{Ref: &ref, Source: WebHTML("<button>go</button>", "")}
	})
	h.commit()
	ref.SendInput(WebClick(10, 10))
	if b := h.commit(); len(b.Items) != 0 {
		t.Fatalf("unsupported input sent %v", b.Items)
	}
	h.tree.SetEnvironment(Environment{CanSendInput: true})
	ref.SendInput(WebClick(10, 10))
	b := h.commit()
	if len(b.Items) != 1 || b.Items[0].Kind != driver.ItemWebCommand || b.Items[0].WebCommand != driver.WebSendInput {
		t.Fatalf("supported input sent %v", b.Items)
	}
}

func TestEnvironmentWebViewCapability(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		if cx.Env().HasWebView() || cx.Env().CanSendInput() {
			t.Fatal("test environment unexpectedly has web capabilities")
		}
		return View{}
	})
	h.commit()
	h.tree.SetEnvironment(Environment{HasWebView: true, CanSendInput: true})
	if !h.root.Env().HasWebView() || !h.root.Env().CanSendInput() {
		t.Fatalf("environment capabilities: web=%v input=%v", h.root.Env().HasWebView(), h.root.Env().CanSendInput())
	}
}

func TestJSString(t *testing.T) {
	for in, want := range map[string]string{
		`a"b\c` + "\nd": `"a\"b\\c\nd"`,
		"\x01":          `"\u0001"`,
		"\u2028é":       `"\u2028é"`,
	} {
		if got := driver.JSString(in); got != want {
			t.Errorf("JSString(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestWebWindowRequestsUseThePagesNavigationPolicy(t *testing.T) {
	var requests []WebWindowRequest
	var refused []string
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		cx.ProvideAccess(hostsAccess{hosts: []string{"allowed.example"}})
		if err := cx.ServeSchemeRequest("nsgo-popup", func(*WebSchemeRequest) *WebSchemeResponse { return nil }); err != nil {
			t.Fatal(err)
		}
		return WebView{TestID: "web",
			OnOpenWindow: func(r WebWindowRequest) { requests = append(requests, r) },
			OnError: func(err error) {
				e, ok := err.(*WebError)
				if !ok {
					t.Errorf("refused request returned %v, want a web error", err)
					return
				}
				refused = append(refused, e.URL)
			},
		}
	})
	batch := h.commit()
	id := h.find("web").id
	var config *driver.WebConfig
	var mask driver.EventMask
	for _, it := range batch.Items {
		if it.ID != id {
			continue
		}
		switch it.Kind {
		case driver.ItemWeb:
			config = it.Web
		case driver.ItemEvents:
			mask = it.Events
		}
	}
	if config == nil || !mask.Has(driver.EventWebOpenWindow) {
		t.Fatalf("web config = %+v, event mask = %b", config, mask)
	}
	allowed := []WebWindowRequest{
		{URL: "https://allowed.example/path", UserInitiated: true},
		{URL: "nsgo-popup://main/path"},
	}
	denied := []string{"https://denied.example/path", "other-popup://main/path"}
	for _, request := range append(allowed, WebWindowRequest{URL: denied[0]}, WebWindowRequest{URL: denied[1]}) {
		ev := config.OpenWindowEvent(request)
		ev.ID = id
		h.tree.Dispatch(ev)
	}
	if !slices.Equal(requests, allowed) {
		t.Errorf("web window requests = %v, want %v", requests, allowed)
	}
	if !slices.Equal(refused, denied) {
		t.Errorf("refused URLs = %v, want %v", refused, denied)
	}
}
