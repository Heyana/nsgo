package harmonyos

import (
	"encoding/json"
	"reflect"
	"testing"

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
