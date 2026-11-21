package core

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestServeScheme(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		if err := cx.ServeScheme("App:", func(string) []byte { return []byte("ok") }); err != nil {
			t.Fatal(err)
		}
		return WebView{Source: WebSrc("app://index.html"), TestID: "web"}
	})
	b := h.commit()
	var got *driver.WebConfig
	for _, item := range b.Items {
		if item.Kind == driver.ItemWeb {
			got = item.Web
		}
	}
	if got == nil || got.SchemeHandlers["app"] == nil || string(got.SchemeHandlers["app"](&driver.WebSchemeRequest{URL: "app://index.html"}).Body) != "ok" {
		t.Fatalf("scheme handlers %+v", got)
	}
	for _, scheme := range []string{"", "http", "1app", "bad scheme", "WS", "wss:", "javascript", "ftp", "applewebdata", "webkit-fake-url", "blob", "about"} {
		if err := h.tree.root.ServeScheme(scheme, func(string) []byte { return nil }); err == nil {
			t.Errorf("ServeScheme(%q) accepted", scheme)
		}
	}
}

func TestServeSchemeRefusesNoHandler(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node { return View{} })
	for _, err := range []error{
		h.tree.root.ServeScheme("app", nil),
		h.tree.root.ServeSchemeRequest("app", nil),
	} {
		if err == nil {
			t.Error("a scheme without a handler was accepted")
		}
	}
	if servesWebScheme(h.tree.root, "app") {
		t.Error("a scheme without a handler was registered")
	}
}

func TestServeSchemeStaysInItsSubtree(t *testing.T) {
	answer := func(body string) func(string) []byte { return func(string) []byte { return []byte(body) } }
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		if err := cx.ServeScheme("app", answer("outer")); err != nil {
			t.Fatal(err)
		}
		return View{Children: []Node{
			Func(func(cx *Scope) Node {
				if err := cx.ServeScheme("app", answer("inner")); err != nil {
					t.Fatal(err)
				}
				if err := cx.ServeScheme("own", answer("own")); err != nil {
					t.Fatal(err)
				}
				return WebView{Source: WebSrc("own://index.html"), TestID: "served"}
			}),
			Func(func(cx *Scope) Node {
				cx.ProvideAccess(hostsAccess{})
				return WebView{Source: WebSrc("own://index.html"), TestID: "sibling"}
			}),
		}}
	})
	configs := map[driver.NodeID]*driver.WebConfig{}
	for _, item := range h.commit().Items {
		if item.Kind == driver.ItemWeb {
			configs[item.ID] = item.Web
		}
	}
	served, sibling := configs[h.find("served").id], configs[h.find("sibling").id]
	if served == nil || sibling == nil {
		t.Fatalf("web configs %v", configs)
	}
	if got := served.SchemeHandlers["app"](&driver.WebSchemeRequest{URL: "app://x"}); string(got.Body) != "inner" || served.SchemeHandlers["own"] == nil {
		t.Errorf("the subtree's schemes: app answers %q, own %v", got.Body, served.SchemeHandlers["own"] != nil)
	}
	if got := sibling.SchemeHandlers["app"](&driver.WebSchemeRequest{URL: "app://x"}); string(got.Body) != "outer" || sibling.SchemeHandlers["own"] != nil {
		t.Errorf("a sibling's schemes: app answers %q, own %v", got.Body, sibling.SchemeHandlers["own"] != nil)
	}
	// A restricted sibling may not load a scheme only another subtree serves.
	if sibling.Source.Src != "about:blank" || sibling.Navigable("own://index.html") {
		t.Errorf("a sibling loads another subtree's scheme: %+v", sibling.Source)
	}
}

func TestServeSchemeStaysInItsWindow(t *testing.T) {
	w := newWindows(t, func(cx *Scope) {
		cx.SetMainView(func(cx *Scope) Node {
			if err := cx.ServeScheme("app", func(string) []byte { return []byte("ok") }); err != nil {
				t.Fatal(err)
			}
			return WebView{Source: WebSrc("app://index.html")}
		})
		cx.OpenWindow(WindowOptions{Title: "Other"}, func(cx *Scope) Node {
			return WebView{Source: WebSrc("app://index.html")}
		})
	})
	main := w.take(1, driver.ItemWeb)
	other := w.take(2, driver.ItemWeb)
	if len(main) != 1 || len(other) != 1 {
		t.Fatalf("web items %d and %d", len(main), len(other))
	}
	if main[0].Web.SchemeHandlers["app"] == nil {
		t.Error("the main window's web view does not serve its scheme")
	}
	if schemes := other[0].Web.SchemeHandlers; schemes != nil {
		t.Errorf("another window's web view serves %v", schemes)
	}
}

func TestServeSchemeRequest(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		if err := cx.ServeSchemeRequest("app", func(r *WebSchemeRequest) *WebSchemeResponse {
			if r.Method != "GET" && r.Method != "" {
				return &WebSchemeResponse{Status: 405, Headers: map[string]string{"Allow": "GET"}}
			}
			return &WebSchemeResponse{Status: 201, Headers: map[string]string{"Access-Control-Allow-Origin": "app://page"}, Body: []byte(r.URL + " via " + r.Header("Accept"))}
		}); err != nil {
			t.Fatal(err)
		}
		if err := cx.ServeScheme("bytes", func(url string) []byte { return []byte(url) }); err != nil {
			t.Fatal(err)
		}
		return WebView{Source: WebSrc("app://index.html"), TestID: "web"}
	})
	var got *driver.WebConfig
	for _, item := range h.commit().Items {
		if item.Kind == driver.ItemWeb {
			got = item.Web
		}
	}
	if got == nil {
		t.Fatal("no web config")
	}
	resp := got.SchemeHandlers["app"](&driver.WebSchemeRequest{URL: "app://a", Method: "POST"})
	if resp == nil || resp.Status != 405 || resp.Header("allow") != "GET" || len(resp.Body) != 0 {
		t.Errorf("POST answers %+v", resp)
	}
	req := &driver.WebSchemeRequest{URL: "app://a", Method: "GET", Headers: map[string]string{"accept": "text/*"}}
	resp = got.SchemeHandlers["app"](req)
	if resp == nil || resp.Status != 201 || resp.Header("Access-Control-Allow-Origin") != "app://page" ||
		string(resp.Body) != "app://a via text/*" {
		t.Errorf("GET answers %+v", resp)
	}
	// ServeScheme's bytes handler answers every request's method 200.
	resp = got.SchemeHandlers["bytes"](&driver.WebSchemeRequest{URL: "bytes://a", Method: "POST"})
	if resp == nil || resp.Status != 0 || string(resp.Body) != "bytes://a" {
		t.Errorf("bytes answers %+v", resp)
	}
	if err := h.tree.root.ServeSchemeRequest("http", func(*WebSchemeRequest) *WebSchemeResponse { return nil }); err == nil {
		t.Error("ServeSchemeRequest(http) accepted")
	}
	if err := h.tree.root.ServeSchemeRequest("gone", nil); err == nil {
		t.Error("ServeSchemeRequest with no handler accepted")
	}
}
