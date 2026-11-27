package harmonyos

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestEncodeWebConfigAllFields(t *testing.T) {
	cfg := &driver.WebConfig{
		Source: driver.WebSource{
			Src:     "https://example.test/app",
			HTML:    "<h1>Hello</h1>",
			BaseURL: "https://example.test/",
		},
		InjectedJavaScriptBeforeContent: "window.before = 1;",
		InjectedJavaScript:              "window.after = 2;",
		UserAgent:                       "CustomUA/1.0",
		UserAgentSuffix:                 "App/2.0",
		Headers: map[string]string{
			"X-Zebra": "z",
			"Accept":  "text/html",
		},
		Incognito: true,
		SchemeHandlers: map[string]driver.WebSchemeHandler{
			"MyApp": func(*driver.WebSchemeRequest) *driver.WebSchemeResponse { return nil },
			"asset": func(*driver.WebSchemeRequest) *driver.WebSchemeResponse { return nil },
		},
		NoScroll:            true,
		NoJavaScript:        true,
		Muted:               true,
		PinchZoom:           true,
		BackForwardGestures: true,
		Mobile:              true,
		DownloadDir:         "/data/downloads",
		Restricted:          true,
		Hosts:               "example.test  *.sub.test",
		Schemes:             "DATA  blob",
		Media:               driver.WebMicrophone | driver.WebCamera,
	}

	raw, err := encodeWebConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got harmonyWebConfig
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	want := harmonyWebConfig{
		Src:             "https://example.test/app",
		HTML:            "<h1>Hello</h1>",
		BaseURL:         "https://example.test/",
		Before:          "window.before = 1;",
		After:           "window.after = 2;",
		UserAgent:       "CustomUA/1.0",
		UserAgentSuffix: "App/2.0",
		Headers: []harmonyWebHeader{
			{Key: "Accept", Value: "text/html"},
			{Key: "X-Zebra", Value: "z"},
		},
		Incognito:    true,
		NoScroll:     true,
		NoJavaScript: true,
		Muted:        true,
		PinchZoom:    true,
		DownloadDir:  "/data/downloads",
		Restricted:   true,
		Hosts:        []string{"example.test", "*.sub.test"},
		Schemes:      []string{"data", "blob"},
		AppSchemes:   []string{"asset", "myapp"},
		Media:        uint8(driver.WebMicrophone | driver.WebCamera),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("encoded config =\n%+v\nwant:\n%+v", got, want)
	}
}

func TestEncodeWebConfigEmptySourceResolvesAboutBlank(t *testing.T) {
	raw, err := encodeWebConfig(&driver.WebConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var got harmonyWebConfig
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if got.Src != "about:blank" || got.HTML != "" || got.BaseURL != "" {
		t.Fatalf("empty source encoded as %+v, want Src=about:blank", got)
	}
}

func TestEncodeWebLoad(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  *driver.WebSource
		want harmonyWebLoad
	}{
		{
			name: "nil resolves to about:blank",
			src:  nil,
			want: harmonyWebLoad{Src: "about:blank"},
		},
		{
			name: "url",
			src:  &driver.WebSource{Src: "https://example.test/next"},
			want: harmonyWebLoad{Src: "https://example.test/next"},
		},
		{
			name: "html with base",
			src:  &driver.WebSource{HTML: "<p>ok</p>", BaseURL: "https://example.test/"},
			want: harmonyWebLoad{HTML: "<p>ok</p>", BaseURL: "https://example.test/"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := encodeWebLoad(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			var got harmonyWebLoad
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("encodeWebLoad = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDecodeWebNavigationAndDownload(t *testing.T) {
	nav := decodeWebNavigation("https://example.test/", "Home", 3, true)
	wantNav := driver.WebNavigation{
		URL:          "https://example.test/",
		Title:        "Home",
		Loading:      true,
		CanGoBack:    true,
		CanGoForward: true,
	}
	if *nav != wantNav {
		t.Fatalf("nav = %+v, want %+v", *nav, wantNav)
	}

	dl, ok := decodeWebDownload(`{"state":1,"id":"dl-1","url":"https://example.test/a.txt","filename":"a.txt","path":"/data/downloads/dl-1/a.txt","bytes":42,"message":""}`)
	if !ok {
		t.Fatal("decodeWebDownload failed")
	}
	wantDL := driver.WebDownload{
		State:    driver.WebDownloadFinished,
		ID:       "dl-1",
		URL:      "https://example.test/a.txt",
		Filename: "a.txt",
		Path:     "/data/downloads/dl-1/a.txt",
		Bytes:    42,
	}
	if *dl != wantDL {
		t.Fatalf("download = %+v, want %+v", *dl, wantDL)
	}
	if _, ok := decodeWebDownload("not-json"); ok {
		t.Fatal("malformed download JSON should fail")
	}
}

func TestEncodeWebCookiesRequest(t *testing.T) {
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	setReq := &driver.WebCookiesRequest{
		Op: driver.WebCookiesSet,
		Cookies: []driver.WebCookie{
			{Name: "SID", Value: "secret", Domain: ".example.test", Path: "/", Expires: expires, Secure: true, HTTPOnly: true, SameSite: driver.WebSameSiteNone},
			{Name: "pref", Value: "dark", Domain: "app.example.test", Path: "/ui", SameSite: driver.WebSameSiteLax},
		},
	}
	raw, err := encodeWebCookiesRequest(setReq)
	if err != nil {
		t.Fatal(err)
	}
	var got harmonyWebCookiesRequest
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	want := harmonyWebCookiesRequest{
		Op: uint8(driver.WebCookiesSet),
		Cookies: []harmonyWebCookieEntry{
			{URL: "https://example.test/", SetCookie: setReq.Cookies[0].SetCookie(), Name: "SID"},
			{URL: "http://app.example.test/ui", SetCookie: setReq.Cookies[1].SetCookie(), Name: "pref"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("set request =\n%+v\nwant:\n%+v", got, want)
	}

	listRaw, err := encodeWebCookiesRequest(&driver.WebCookiesRequest{
		Op:     driver.WebCookiesList,
		Filter: driver.WebCookieFilter{URL: "https://example.test/app"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotList harmonyWebCookiesRequest
	if err := json.Unmarshal([]byte(listRaw), &gotList); err != nil {
		t.Fatal(err)
	}
	if gotList.Op != uint8(driver.WebCookiesList) || gotList.URL != "https://example.test/app" || gotList.Domain != "" {
		t.Fatalf("list request = %+v", gotList)
	}

	clearRaw, err := encodeWebCookiesRequest(&driver.WebCookiesRequest{Op: driver.WebCookiesClear})
	if err != nil {
		t.Fatal(err)
	}
	var gotClear harmonyWebCookiesRequest
	if err := json.Unmarshal([]byte(clearRaw), &gotClear); err != nil {
		t.Fatal(err)
	}
	if gotClear.Op != uint8(driver.WebCookiesClear) || len(gotClear.Cookies) != 0 {
		t.Fatalf("clear request = %+v", gotClear)
	}

	if _, err := encodeWebCookiesRequest(nil); err == nil {
		t.Fatal("nil request should fail")
	}
	if _, err := encodeWebCookiesRequest(&driver.WebCookiesRequest{Op: 99}); err == nil {
		t.Fatal("unsupported op should fail")
	}
}

func TestDecodeWebCookiesEvent(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	// Status success and error.
	if ev := decodeWebCookiesEvent(3, harmonyWebCookiesModeStatus, "", "", driver.WebCookieFilter{}, now); ev.Kind != driver.EventWebCookies || ev.Count != 3 || ev.Text != "" {
		t.Fatalf("status ok = %+v", ev)
	}
	if ev := decodeWebCookiesEvent(4, harmonyWebCookiesModeStatus, "", "refused", driver.WebCookieFilter{}, now); ev.Kind != driver.EventWebCookies || ev.Count != 4 || ev.Text != "refused" {
		t.Fatalf("status err = %+v", ev)
	}
	if ev := decodeWebCookiesEvent(5, harmonyWebCookiesModeAll, "", driver.NoWebCookieDomains, driver.WebCookieFilter{}, now); ev.Text != driver.NoWebCookieDomains {
		t.Fatalf("unavailable err = %+v", ev)
	}

	// Full cookie list (mode 1) with URL, Domain, and zero filters + expiry + SameSite.
	wire, err := json.Marshal([]harmonyWebCookieWire{
		{Name: "SID", Value: "s1", Domain: ".Example.Test", Path: "/", Expires: "Wed, 02 Jan 2030 03:04:05 GMT", Secure: true, HTTPOnly: true, SameSite: uint8(driver.WebSameSiteNone)},
		{Name: "sess", Value: "s2", Domain: "app.example.test", Path: "/ui", Session: true, SameSite: uint8(driver.WebSameSiteLax)},
		{Name: "expired", Value: "old", Domain: ".example.test", Path: "/", Expires: "Mon, 31 Dec 2029 23:59:59 GMT"},
		{Name: "other", Value: "o1", Domain: "other.test", Path: "", Expires: "2030-06-01T00:00:00Z", SameSite: uint8(driver.WebSameSiteStrict)},
		{Name: "badss", Value: "x", Domain: "other.test", Path: "/", SameSite: 99},
	})
	if err != nil {
		t.Fatal(err)
	}

	evAll := decodeWebCookiesEvent(10, harmonyWebCookiesModeAll, string(wire), "", driver.WebCookieFilter{}, now)
	if evAll.Text != "" {
		t.Fatalf("all err = %q", evAll.Text)
	}
	gotAll, ok := evAll.Data.([]driver.WebCookie)
	if !ok || len(gotAll) != 4 {
		t.Fatalf("all cookies = %#v, want 4 non-expired", evAll.Data)
	}
	if gotAll[0].Name != "sess" || !gotAll[0].Expires.IsZero() || gotAll[0].SameSite != driver.WebSameSiteLax {
		t.Fatalf("first cookie = %+v", gotAll[0])
	}
	if gotAll[1].Name != "SID" || gotAll[1].Domain != ".example.test" || !gotAll[1].Secure || !gotAll[1].HTTPOnly || gotAll[1].SameSite != driver.WebSameSiteNone || !gotAll[1].Expires.Equal(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("second cookie = %+v", gotAll[1])
	}
	if gotAll[2].Name != "badss" || gotAll[2].SameSite != driver.WebSameSiteDefault {
		t.Fatalf("clamped sameSite cookie = %+v", gotAll[2])
	}
	if gotAll[3].Name != "other" || gotAll[3].Path != "/" || gotAll[3].SameSite != driver.WebSameSiteStrict {
		t.Fatalf("fourth cookie = %+v", gotAll[3])
	}

	evURL := decodeWebCookiesEvent(11, harmonyWebCookiesModeAll, string(wire), "", driver.WebCookieFilter{URL: "http://app.example.test/ui/home"}, now)
	gotURL, _ := evURL.Data.([]driver.WebCookie)
	if len(gotURL) != 1 || gotURL[0].Name != "sess" {
		t.Fatalf("http URL filter = %+v, want only non-secure sess", gotURL)
	}

	evDomain := decodeWebCookiesEvent(12, harmonyWebCookiesModeAll, string(wire), "", driver.WebCookieFilter{Domain: "example.test"}, now)
	gotDomain, _ := evDomain.Data.([]driver.WebCookie)
	if len(gotDomain) != 2 || gotDomain[0].Name != "sess" || gotDomain[1].Name != "SID" {
		t.Fatalf("domain filter = %+v, want sess and SID", gotDomain)
	}

	// Malformed JSON payload reports error rather than panicking.
	if evBad := decodeWebCookiesEvent(13, harmonyWebCookiesModeAll, "{bad", "", driver.WebCookieFilter{}, now); !strings.Contains(evBad.Text, "invalid web cookies payload") {
		t.Fatalf("malformed payload = %+v", evBad)
	}

	// Header fallback (mode 2) from fetchCookieSync.
	evHdr := decodeWebCookiesEvent(14, harmonyWebCookiesModeHeader, "b=two=three; a=1; bare", "", driver.WebCookieFilter{URL: "https://API.Example.Test/v1"}, now)
	if evHdr.Text != "" {
		t.Fatalf("header err = %q", evHdr.Text)
	}
	gotHdr, ok := evHdr.Data.([]driver.WebCookie)
	wantHdr := []driver.WebCookie{
		{Name: "", Value: "bare", Domain: "api.example.test"},
		{Name: "a", Value: "1", Domain: "api.example.test"},
		{Name: "b", Value: "two=three", Domain: "api.example.test"},
	}
	if !ok || !reflect.DeepEqual(gotHdr, wantHdr) {
		t.Fatalf("header cookies = %#v, want %#v", gotHdr, wantHdr)
	}

	evEmptyHdr := decodeWebCookiesEvent(15, harmonyWebCookiesModeHeader, "", "", driver.WebCookieFilter{URL: "https://example.test/"}, now)
	if got, ok := evEmptyHdr.Data.([]driver.WebCookie); !ok || got == nil || len(got) != 0 {
		t.Fatalf("empty header = %#v, want non-nil empty slice", evEmptyHdr.Data)
	}

	// Unknown mode returns error.
	if evUnk := decodeWebCookiesEvent(16, 99, "", "", driver.WebCookieFilter{}, now); !strings.Contains(evUnk.Text, "unknown web cookies reply mode") {
		t.Fatalf("unknown mode = %+v", evUnk)
	}
}

func TestParseHarmonyCookieExpires(t *testing.T) {
	want := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, raw := range []string{
		"Wed, 02 Jan 2030 03:04:05 GMT",
		"Wed, 02-Jan-2030 03:04:05 GMT",
		"2030-01-02T03:04:05Z",
		"2030-01-02 03:04:05",
		"1893553445",
		"1893553445000",
		"1893553445000000",
	} {
		if got := parseHarmonyCookieExpires(raw); !got.Equal(want) {
			t.Errorf("parseHarmonyCookieExpires(%q) = %v, want %v", raw, got, want)
		}
	}
	if got := parseHarmonyCookieExpires("not-a-date"); !got.IsZero() {
		t.Errorf("invalid date parsed as %v", got)
	}
}
