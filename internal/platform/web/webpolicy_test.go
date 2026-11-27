package web

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebFrameSandbox(t *testing.T) {
	open := &driver.WebConfig{}
	restricted := &driver.WebConfig{Restricted: true, Hosts: "allowed.example"}

	// Unrestricted HTML keeps existing sandbox tokens without allow-same-origin or allow-top-navigation.
	got := webFrameSandbox(open, driver.WebSource{HTML: "<p>hi</p>"}, "https://app.example", true)
	for _, want := range []string{"allow-forms", "allow-popups", "allow-popups-to-escape-sandbox", "allow-modals", "allow-downloads", "allow-scripts"} {
		if !strings.Contains(got, want) {
			t.Errorf("unrestricted html sandbox %q missing %q", got, want)
		}
	}
	for _, forbid := range []string{"allow-same-origin", "allow-top-navigation"} {
		if strings.Contains(got, forbid) {
			t.Errorf("unrestricted html sandbox %q unexpectedly grants %q", got, forbid)
		}
	}

	// Restricted HTML omits allow-popups, allow-popups-to-escape-sandbox, allow-downloads, allow-same-origin, allow-top-navigation.
	got = webFrameSandbox(restricted, driver.WebSource{HTML: "<p>hi</p>"}, "https://app.example", true)
	for _, want := range []string{"allow-forms", "allow-modals", "allow-scripts"} {
		if !strings.Contains(got, want) {
			t.Errorf("restricted html sandbox %q missing %q", got, want)
		}
	}
	for _, forbid := range []string{"allow-popups", "allow-popups-to-escape-sandbox", "allow-downloads", "allow-same-origin", "allow-top-navigation"} {
		if strings.Contains(got, forbid) {
			t.Errorf("restricted html sandbox %q unexpectedly grants %q", got, forbid)
		}
	}

	// Restricted cross-origin Src keeps allow-same-origin for site storage but strips popups, downloads and top-navigation.
	got = webFrameSandbox(restricted, driver.WebSource{Src: "https://allowed.example/page"}, "https://app.example", false)
	if !strings.Contains(got, "allow-same-origin") {
		t.Errorf("restricted cross-origin src sandbox %q missing allow-same-origin", got)
	}
	for _, forbid := range []string{"allow-popups", "allow-popups-to-escape-sandbox", "allow-downloads", "allow-top-navigation"} {
		if strings.Contains(got, forbid) {
			t.Errorf("restricted cross-origin src sandbox %q unexpectedly grants %q", got, forbid)
		}
	}

	// Same-origin Src and about: / data: stay opaque even on restricted views.
	for _, src := range []string{"https://app.example/inner", "about:blank", "data:text/plain,hi"} {
		got = webFrameSandbox(restricted, driver.WebSource{Src: src}, "https://app.example", false)
		if strings.Contains(got, "allow-same-origin") {
			t.Errorf("sandbox for %q (%q) unexpectedly grants allow-same-origin", src, got)
		}
	}

	// NoJavaScript omits allow-scripts.
	noJS := &driver.WebConfig{Restricted: true, NoJavaScript: true}
	got = webFrameSandbox(noJS, driver.WebSource{HTML: "<p>hi</p>"}, "https://app.example", true)
	if strings.Contains(got, "allow-scripts") {
		t.Errorf("NoJavaScript sandbox %q unexpectedly grants allow-scripts", got)
	}
}

func TestWebRestrictedCSP(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  driver.WebConfig
		want string
	}{
		{
			name: "no grants",
			cfg:  driver.WebConfig{Restricted: true},
			want: "form-action 'none'; base-uri 'none'",
		},
		{
			name: "specific host and wildcard",
			cfg:  driver.WebConfig{Restricted: true, Hosts: "Allowed.Example. *.cdn.example"},
			want: "form-action http://allowed.example:* https://allowed.example:* http://*.cdn.example:* https://*.cdn.example:*; base-uri http://allowed.example:* https://allowed.example:* http://*.cdn.example:* https://*.cdn.example:*",
		},
		{
			name: "star host with page and custom schemes",
			cfg: driver.WebConfig{
				Restricted:     true,
				Hosts:          "* ok.example",
				Schemes:        "about blob",
				SchemeHandlers: map[string]driver.WebSchemeHandler{"app": nil},
			},
			want: "form-action http: https: about: blob: app:; base-uri http: https: about: blob: app:",
		},
		{
			name: "malformed host cannot inject directives",
			cfg:  driver.WebConfig{Restricted: true, Hosts: "ok.example;script-src 'unsafe-inline' good.example"},
			want: "form-action http://good.example:* https://good.example:*; base-uri http://good.example:* https://good.example:*",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := webRestrictedCSP(&tc.cfg); got != tc.want {
				t.Errorf("webRestrictedCSP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeRestrictedDataHTML(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
		ok   bool
	}{
		{"data:text/html,<a href='https://evil.example/'>bad</a>", "<a href='https://evil.example/'>bad</a>", true},
		{"data:text/html;charset=utf-8,%3Cp%3Ehi%3C/p%3E", "<p>hi</p>", true},
		{"data:text/html;base64,PHA+aGk8L3A+", "<p>hi</p>", true},
		{"DATA:APPLICATION/XHTML+XML;base64,PHA+PC9wPg==", "<p></p>", true},
		{"data:text/plain,hello", "", false},
		{"data:image/png;base64,AAAA", "", false},
		{"https://example.com/", "", false},
	} {
		got, ok := decodeRestrictedDataHTML(tc.raw)
		if ok != tc.ok || got != tc.want {
			t.Errorf("decodeRestrictedDataHTML(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParseMetaRefresh(t *testing.T) {
	for _, tc := range []struct {
		content string
		delayMS int
		want    string
		ok      bool
	}{
		{"0;url=https://evil.example/", 0, "https://evil.example/", true},
		{"5; URL='https://evil.example/a'", 5000, "https://evil.example/a", true},
		{`0.5; url = "https://evil.example/b"`, 500, "https://evil.example/b", true},
		{"0;https://evil.example/direct", 0, "https://evil.example/direct", true},
		{"0,url=https://evil.example/comma", 0, "https://evil.example/comma", true},
		{"5", 0, "", false},
		{"0;", 0, "", false},
		{"", 0, "", false},
	} {
		delayMS, got, ok := parseMetaRefresh(tc.content)
		if ok != tc.ok || got != tc.want || delayMS != tc.delayMS {
			t.Errorf("parseMetaRefresh(%q) = (%d, %q, %v), want (%d, %q, %v)", tc.content, delayMS, got, ok, tc.delayMS, tc.want, tc.ok)
		}
	}
}

func TestBuildRestrictedSrcdoc(t *testing.T) {
	cfg := &driver.WebConfig{
		Restricted: true,
		Hosts:      "allowed.example *.sub.example",
		Schemes:    "about",
	}
	raw := `<!doctype html><html><head><title>Demo</title>` +
		`<meta http-equiv="refresh" content="0;url=https://evil.example/bounce">` +
		`<meta http-equiv="refresh" content="0;url=https://allowed.example/ok">` +
		`</head><body><script>if (1 < 2 && 3 > 1) {}</script></body></html>`

	doc, title, refused := buildRestrictedSrcdoc(raw, "", "tok123", true, cfg)
	if title != "Demo" {
		t.Errorf("title = %q, want Demo", title)
	}
	if len(refused) != 1 || refused[0] != "https://evil.example/bounce" {
		t.Errorf("refused meta = %v, want [https://evil.example/bounce]", refused)
	}
	if strings.Contains(doc, "evil.example/bounce") {
		t.Errorf("disallowed meta refresh remained in output:\n%s", doc)
	}
	if !strings.Contains(doc, "allowed.example/ok") {
		t.Errorf("allowed meta refresh was stripped:\n%s", doc)
	}
	if !strings.Contains(doc, `http-equiv="Content-Security-Policy"`) || !strings.Contains(doc, `form-action http://allowed.example:*`) {
		t.Errorf("CSP meta missing from output:\n%s", doc)
	}
	if !strings.Contains(doc, `if (1 < 2 && 3 > 1) {}`) {
		t.Errorf("script contents were escaped or altered:\n%s", doc)
	}
	// CSP meta and guard script must precede any page script.
	cspIdx := strings.Index(doc, `Content-Security-Policy`)
	guardIdx := strings.Index(doc, `new MessageChannel()`)
	pageScriptIdx := strings.Index(doc, `if (1 < 2`)
	if !(cspIdx > 0 && guardIdx > cspIdx && pageScriptIdx > guardIdx) {
		t.Errorf("ordering wrong: csp=%d guard=%d pageScript=%d", cspIdx, guardIdx, pageScriptIdx)
	}

	// With NoJavaScript: disallowed links and forms are statically stripped, allowed ones stay.
	noJS := &driver.WebConfig{
		Restricted:   true,
		NoJavaScript: true,
		Hosts:        "allowed.example",
	}
	htmlNoJS := `<a id="ok" href="https://allowed.example/p">ok</a>` +
		`<a id="bad" href="https://evil.example/p" download>bad</a>` +
		`<a id="top" href="https://allowed.example/p" target="_top">top</a>` +
		`<a id="frag" href="#sec">sec</a>` +
		`<form action="https://evil.example/post"><button formaction="https://evil.example/btn">go</button></form>`
	docNoJS, _, _ := buildRestrictedSrcdoc(htmlNoJS, "", "", false, noJS)
	if !strings.Contains(docNoJS, `href="https://allowed.example/p"`) {
		t.Errorf("allowed link removed in NoJavaScript output:\n%s", docNoJS)
	}
	if strings.Contains(docNoJS, `https://evil.example/p`) || strings.Contains(docNoJS, `download`) {
		t.Errorf("disallowed link or download kept in NoJavaScript output:\n%s", docNoJS)
	}
	if strings.Contains(docNoJS, `target="_top"`) {
		t.Errorf("target=_top kept in NoJavaScript output:\n%s", docNoJS)
	}
	if !strings.Contains(docNoJS, `href="#sec"`) {
		t.Errorf("same-document fragment removed in NoJavaScript output:\n%s", docNoJS)
	}
	if strings.Contains(docNoJS, `https://evil.example/post`) || strings.Contains(docNoJS, `https://evil.example/btn`) {
		t.Errorf("disallowed form action kept in NoJavaScript output:\n%s", docNoJS)
	}
}

func TestSameWebFrameChecksRestrictions(t *testing.T) {
	a := &driver.WebConfig{Source: driver.WebSource{HTML: "<p>x</p>"}, Restricted: true, Hosts: "a.example", Schemes: "about"}
	b := *a
	if !sameWebFrame(a, &b) {
		t.Fatal("identical configs reported different")
	}
	b.Hosts = "b.example"
	if sameWebFrame(a, &b) {
		t.Fatal("changed Hosts reported same")
	}
	b = *a
	b.Schemes = ""
	if sameWebFrame(a, &b) {
		t.Fatal("changed Schemes reported same")
	}
	b = *a
	b.Restricted = false
	if sameWebFrame(a, &b) {
		t.Fatal("changed Restricted reported same")
	}
}
