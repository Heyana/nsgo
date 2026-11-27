package driver

import (
	"errors"
	"strings"
	"testing"
)

func TestWebOrigin(t *testing.T) {
	for raw, want := range map[string]string{
		"https://Example.COM/a?b#c":       "https://example.com",
		"https://example.com:443/":        "https://example.com",
		"http://example.com:8080/":        "http://example.com:8080",
		"http://user:pw@[::1]:80/x":       "http://[::1]",
		"app://bundle/index.html":         "app://bundle",
		"file:///Users/me/app/index.html": "file://",
		"blob:https://example.com/1234":   "https://example.com",
		"about:blank":                     "null",
		"about:srcdoc":                    "null",
		"data:text/html,<p>":              "null",
		"":                                "",
		"example.com":                     "null",
		"https://exa mple.com/":           "null",
	} {
		if got := WebOrigin(raw); got != want {
			t.Errorf("WebOrigin(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestWebUserAgent(t *testing.T) {
	if got := WebUserAgent("Mozilla/5.0", "Test/1"); got != "Mozilla/5.0 Test/1" {
		t.Fatalf("WebUserAgent = %q", got)
	}
	if got := WebUserAgent("", "Test/1"); got != "Test/1" {
		t.Fatalf("empty WebUserAgent = %q", got)
	}
}

func TestWebSourceShows(t *testing.T) {
	for _, c := range []struct {
		source WebSource
		url    string
		want   bool
	}{
		{WebSource{HTML: "<p>"}, "about:blank", true},
		{WebSource{HTML: "<p>"}, "about:blank#top", true},
		{WebSource{HTML: "<p>"}, "about:srcdoc", true},
		{WebSource{HTML: "<p>"}, "data:text/html,<p>", false},
		{WebSource{HTML: "<p>"}, "blob:null/1234", false},
		{WebSource{HTML: "<p>"}, "", false},
		{WebSource{HTML: "<p>", BaseURL: "app://bundle/"}, "app://bundle/", true},
		{WebSource{HTML: "<p>", BaseURL: "app://bundle/"}, "about:blank", false},
		{WebSource{Src: "data:text/html,<p>hi</p>"}, "data:text/html,%3Cp%3Ehi%3C/p%3E", true},
		{WebSource{Src: "data:text/html,<p>hi</p>"}, "data:text/html,<p>evil</p>", false},
		{WebSource{Src: "data:text/html,<p>hi</p>"}, "about:blank", false},
		{WebSource{}, "about:blank", true},
		{WebSource{}, "about:blank#top", true},
		{WebSource{}, "data:text/html,<p>", false},
		{WebSource{}, "about:srcdoc", false},
	} {
		if got := c.source.Shows(c.url); got != c.want {
			t.Errorf("%+v.Shows(%q) = %v, want %v", c.source, c.url, got, c.want)
		}
	}
}

func TestResolveWebSource(t *testing.T) {
	blank := WebSource{Src: "about:blank"}
	for _, c := range []struct{ source, want WebSource }{
		{WebSource{}, blank},
		{WebSource{BaseURL: "https://app.example/"}, blank},
		{WebSource{Src: "https://app.example/"}, WebSource{Src: "https://app.example/"}},
		{WebSource{HTML: "<p>", BaseURL: "app://bundle/"}, WebSource{HTML: "<p>", BaseURL: "app://bundle/"}},
	} {
		if got := ResolveWebSource(c.source); got != c.want {
			t.Errorf("ResolveWebSource(%+v) = %+v, want %+v", c.source, got, c.want)
		}
		if got := c.source.Origin(); got != c.want.Origin() {
			t.Errorf("%+v.Origin() = %q, want %q", c.source, got, c.want.Origin())
		}
	}
	if got := (WebSource{}).Origin(); got != "null" {
		t.Errorf("an empty source's origin is %q, want null", got)
	}
}

// A backend that has no page to run an Evaluate in answers it itself, from the script's marker.
func TestWebEvaluateMarker(t *testing.T) {
	for _, marker := range []string{"nsgo-evaluate:0123abcd:", `odd "quoted\" marker`} {
		got, ok := WebEvaluateMarker(WebEvaluateScript(`"nsgo-evaluate:ffff:" + 1`, marker))
		if !ok || got != marker {
			t.Errorf("the marker of a script after %q is %q, %v", marker, got, ok)
		}
		if _, err := WebEvaluateAnswer(WebEvaluateFailure(marker, "no page")[len(marker):]); err == nil || err.Error() != "no page" {
			t.Errorf("a failure answers %v", err)
		}
		if _, err := WebEvaluateAnswer(WebEvaluateUnsupported(marker, "no web view")[len(marker):]); !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("an unsupported failure answers %v, want errors.ErrUnsupported", err)
		}
	}
	for _, script := range []string{`window.nsgo.postMessage("nsgo-evaluate:ffff:")`, "", WebMessageScript("x")} {
		if marker, ok := WebEvaluateMarker(script); ok {
			t.Errorf("%q has a marker, %q", script, marker)
		}
	}
}

func TestNavigableHoldsPageSchemes(t *testing.T) {
	base := WebConfig{Restricted: true, Hosts: "ok.com", SchemeHandlers: map[string]WebSchemeHandler{"app": nil}}
	for _, c := range []struct {
		schemes string
		url     string
		page    bool // Navigable: a page's own navigation
		source  bool // SourceNavigable: the app's own load
	}{
		{"about blob data", "about:blank", true, true},
		{"about blob data", "data:text/html,x", true, true},
		{"blob", "blob:https://ok.com/id", true, true},
		{"blob", "about:blank", false, true},
		{"blob", "about:srcdoc", false, true},
		{"blob", "data:text/html,x", false, true},
		{"blob", "DATA:text/html,x", false, true},
		{"", "blob:https://ok.com/id", false, true},
		{"", "https://ok.com/", true, true},
		{"", "https://evil.io/", false, false},
		{"", "file:///etc/hosts", false, false},
		{"", "javascript:alert(1)", false, false},
		{"", "app://settings/index.html", true, true},
	} {
		cfg := base
		cfg.Schemes = c.schemes
		if got := cfg.Navigable(c.url); got != c.page {
			t.Errorf("Schemes %q: Navigable(%q) = %v, want %v", c.schemes, c.url, got, c.page)
		}
		if got := cfg.SourceNavigable(c.url); got != c.source {
			t.Errorf("Schemes %q: SourceNavigable(%q) = %v, want %v", c.schemes, c.url, got, c.source)
		}
	}
	var open WebConfig
	if !open.Navigable("data:text/html,x") || !open.SourceNavigable("https://evil.io/") {
		t.Error("an unrestricted view refuses a navigation")
	}
}

func TestWebSchemeHeader(t *testing.T) {
	resp := &WebSchemeResponse{Headers: map[string]string{"content-type": "text/html", "X-Allow": "yes"}}
	if got := resp.Header("Content-Type"); got != "text/html" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := resp.Header("x-allow"); got != "yes" {
		t.Errorf("x-allow = %q", got)
	}
	if got := resp.Header("Missing"); got != "" {
		t.Errorf("Missing = %q", got)
	}
	var nilResp *WebSchemeResponse
	if got := nilResp.Header("Any"); got != "" {
		t.Errorf("nil response's Any = %q", got)
	}
}

func TestIntersectSchemes(t *testing.T) {
	for _, c := range []struct {
		a, b []string
		want string
	}{
		{[]string{"data", "blob"}, []string{"blob", "about"}, "blob"},
		{[]string{"DATA", "Blob", "javascript"}, PageSchemes, "blob data"},
		{nil, PageSchemes, ""},
		{PageSchemes, PageSchemes, "about blob data"},
	} {
		if got := strings.Join(IntersectSchemes(c.a, c.b), " "); got != c.want {
			t.Errorf("IntersectSchemes(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}
