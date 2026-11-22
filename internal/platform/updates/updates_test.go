package updates

import (
	"context"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		newer bool
	}{
		{"1.0.1", "1.0", true},
		{"1.0", "1.0.1", false},
		{"1.10", "1.9", true},
		{"2", "10", false},
		{"1.0", "1.0b1", true},
		{"1.0b2", "1.0b1", true},
		{"1.0b1", "1.0", false},
		{"1.0", "1.0", false},
		{"12", "9", true},
		{"100000000000000000000001", "100000000000000000000000", true},
		{"007", "7", false},
	} {
		if got := Newer(c.a, c.b); got != c.newer {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.newer)
		}
	}
}

const feed = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>Hello</title>
    <item>
      <title>1.1</title>
      <link>https://example.com/releases/1.1</link>
      <sparkle:version>11</sparkle:version>
      <sparkle:shortVersionString>1.1</sparkle:shortVersionString>
      <enclosure url="https://example.com/hello-1.1.deb" length="10" type="application/octet-stream"/>
    </item>
    <item>
      <title>1.2</title>
      <enclosure url="https://example.com/hello-1.2.deb" sparkle:version="12" sparkle:shortVersionString="1.2" length="10" type="application/octet-stream"/>
    </item>
    <item>
      <title>no link</title>
      <link>javascript:alert(1)</link>
      <sparkle:version>99</sparkle:version>
    </item>
  </channel>
</rss>`

func TestParseAppcast(t *testing.T) {
	item, err := ParseAppcast([]byte(feed))
	if err != nil {
		t.Fatal(err)
	}
	// The highest build with a link: the enclosure's older form, and its
	// download where the item has no page.
	want := Item{Version: "1.2", Build: "12", Link: "https://example.com/hello-1.2.deb", Download: "https://example.com/hello-1.2.deb", Size: 10}
	if item != want {
		t.Errorf("ParseAppcast = %+v, want %+v", item, want)
	}
	if _, err := ParseAppcast([]byte(`<rss><channel></channel></rss>`)); err == nil {
		t.Error("an appcast of no release parsed")
	}
}

// serve has the backend's HTTP client answer with body, or status alone.
func serve(t *testing.T, status int, body string) *[]string {
	var asked []string
	was := imageload.HTTP
	imageload.HTTP = func(_ context.Context, url string, _ imageload.Header, _ imageload.Progress) (*imageload.Response, error) {
		asked = append(asked, url)
		return &imageload.Response{Status: status, Body: []byte(body)}, nil
	}
	t.Cleanup(func() { imageload.HTTP = was })
	return &asked
}

func TestFind(t *testing.T) {
	serve(t, 200, feed)
	c := Config{App: "Hello", Version: "1.1", Build: "11", Feed: "https://example.com/appcast-linux_x64.xml"}
	got := Find(context.Background(), c)
	// An unsigned appcast offers the release's page alone, whether or not the app has a key.
	want := driver.Update{App: "Hello", Current: "1.1", Version: "1.2", URL: "https://example.com/hello-1.2.deb", Managed: driver.UpdateManagedSelf}
	if got != want {
		t.Errorf("Find = %+v, want %+v", got, want)
	}
	c.Key = "a2V5"
	if got := Find(context.Background(), c); got != want {
		t.Errorf("Find of an unsigned appcast with a key = %+v, want %+v", got, want)
	}
	c.Key = ""
	c.Build = "12"
	if got := Find(context.Background(), c); got.Version != "" || got.Err != "" {
		t.Errorf("Find of the newest build = %+v, want none newer", got)
	}
	// Without a build number the version is compared.
	c.Build, c.Version = "", "13"
	if got := Find(context.Background(), c); got.Version != "" {
		t.Errorf("Find of version 13 = %+v, want none newer", got)
	}
	if got := Find(context.Background(), Config{}); got.Err != "" || got.Managed != driver.UpdateManagedChannel {
		t.Error("Find without a feed did not report its disabled capability")
	}
	serve(t, 404, "")
	if got := Find(context.Background(), c); !strings.Contains(got.Err, "404") {
		t.Errorf("Find of a missing appcast = %+v, want its status", got)
	}
}

func TestFindOffersOnlySignedPackages(t *testing.T) {
	signed := strings.Replace(feed, `sparkle:version="12"`, `sparkle:version="12" sparkle:edSignature="c2ln" sparkle:sha256="abc"`, 1)
	serve(t, 200, signed)
	c := Config{App: "Hello", Version: "1.1", Build: "11", Feed: "https://example.com/appcast-linux_x64.xml", Key: "a2V5"}
	got := Find(context.Background(), c)
	want := driver.Update{App: "Hello", Current: "1.1", Version: "1.2", URL: "https://example.com/hello-1.2.deb",
		Download: "https://example.com/hello-1.2.deb", Signature: "c2ln", SHA256: "abc", Key: "a2V5", Size: 10, Managed: driver.UpdateManagedSelf}
	if got != want {
		t.Errorf("Find = %+v, want %+v", got, want)
	}
	// Without a key to check it against, a signature is worth nothing.
	c.Key = ""
	if got := Find(context.Background(), c); got.Download != "" || got.Version != "1.2" {
		t.Errorf("Find without a key = %+v, want the page alone", got)
	}
}

func TestFindInAppStore(t *testing.T) {
	asked := serve(t, 200, `{"resultCount":1,"results":[{"version":"2.0","trackViewUrl":"https://apps.apple.com/cn/app/hello/id123"}]}`)
	c := Config{App: "Hello", ID: "com.example.hello", Version: "1.9"}
	got := FindInAppStore(context.Background(), c, "CN")
	want := driver.Update{App: "Hello", Current: "1.9", Version: "2.0", URL: "https://apps.apple.com/cn/app/hello/id123", Managed: driver.UpdateManagedPage}
	if got != want {
		t.Errorf("FindInAppStore = %+v, want %+v", got, want)
	}
	if u := (*asked)[0]; u != "https://itunes.apple.com/lookup?bundleId=com.example.hello&country=cn" {
		t.Errorf("asked %s", u)
	}
	serve(t, 200, `{"resultCount":0,"results":[]}`)
	if got := FindInAppStore(context.Background(), c, ""); got.Version != "" || got.Err != "" {
		t.Errorf("FindInAppStore of an app the store lacks = %+v, want none newer", got)
	}
	// An answer naming no page, or one no browser opens, leaves the app's
	// page by its ID in the store.
	for _, body := range []string{
		`{"results":[{"version":"2.0","trackId":123}]}`,
		`{"results":[{"version":"2.0","trackId":123,"trackViewUrl":"javascript:alert(1)"}]}`,
	} {
		serve(t, 200, body)
		if got := FindInAppStore(context.Background(), c, ""); got.Version != "2.0" || got.URL != "https://apps.apple.com/app/id123" {
			t.Errorf("FindInAppStore of %s = %+v, want the app's page by its ID", body, got)
		}
	}
	// What the store could not answer is an error, not an app up to date.
	for status, body := range map[int]string{503: "", 200: "<html>"} {
		serve(t, status, body)
		if got := FindInAppStore(context.Background(), c, ""); got.Err == "" || got.Version != "" {
			t.Errorf("FindInAppStore of status %d and %q = %+v, want an error", status, body, got)
		}
	}
}

func TestAnswer(t *testing.T) {
	events := make(chan driver.Event, 1)
	Answer(7, func(context.Context) driver.Update { return driver.Update{Version: "2"} }, func(ev driver.Event) { events <- ev })
	ev := <-events
	if u, ok := ev.Data.(*driver.Update); ev.Kind != driver.EventUpdate || ev.Count != 7 || !ok || u.Version != "2" {
		t.Errorf("Answer dispatched %+v", ev)
	}
}

// An app a store installed checks nothing: the store updates it.
func TestStoreInstalls(t *testing.T) {
	for installer, store := range map[string]string{
		"": "", "com.android.shell": "", "com.google.android.packageinstaller": "", "com.miui.packageinstaller": "",
		"com.android.vending": "com.android.vending", "com.huawei.appmarket": "com.huawei.appmarket",
	} {
		if got := AndroidStore(installer); got != store {
			t.Errorf("AndroidStore(%q) = %q, want %q", installer, got, store)
		}
	}
	serve(t, 200, feed)
	c := Config{App: "Hello", ID: "com.example.hello", Version: "1.1", Build: "11", Feed: "https://example.com/appcast.xml", Store: "com.android.vending"}
	if c.Checks(false) || c.Checks(true) {
		t.Error("an app a store installed checks")
	}
	for _, got := range []driver.Update{Find(context.Background(), c), FindInAppStore(context.Background(), c, "")} {
		if got.Version != "" || got.Err != "" || got.Managed != driver.UpdateManagedStore || got.Store != "com.android.vending" {
			t.Errorf("a check of an app a store installed = %+v", got)
		}
	}
	c.Store = ""
	if !c.Checks(false) || (Config{}).Checks(false) || !(Config{}).Checks(true) {
		t.Error("Checks: an appcast or the App Store is not checked")
	}
}

func TestNotesText(t *testing.T) {
	for _, c := range []struct{ html, want string }{
		{"", ""},
		{"<p>Faster <code>sync</code>,\nand more.</p>", "Faster sync, and more."},
		{"<h2>New</h2>\n<ul>\n<li>A <strong>dark</strong> theme</li>\n<li><p>Tabs</p></li>\n</ul>\n<p>Fixes &amp; more</p>", "New\n• A dark theme\n• Tabs\nFixes & more"},
		{"Plain text", "Plain text"},
		{"<html><head><title>v2</title><style>p { color: red }</style></head><body><p>Fixes</p><script>track()</script></body></html>", "Fixes"},
	} {
		if got := NotesText(c.html); got != c.want {
			t.Errorf("NotesText(%q) = %q, want %q", c.html, got, c.want)
		}
	}
}

func TestParseAppcastItemsAndFindMatching(t *testing.T) {
	multiFeed := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>Hello</title>
    <item>
      <title>Hello 1.1</title>
      <link>https://example.com/releases/1.1</link>
      <sparkle:version>11</sparkle:version>
      <sparkle:shortVersionString>1.1</sparkle:shortVersionString>
      <enclosure url="https://example.com/hello-1.1-linux-x64.deb" length="10" sparkle:edSignature="old" sparkle:sha256="old"/>
    </item>
    <item>
      <title>Hello 1.2</title>
      <link>https://example.com/releases/1.2</link>
      <sparkle:version>12</sparkle:version>
      <sparkle:shortVersionString>1.2</sparkle:shortVersionString>
      <enclosure url="https://example.com/hello-1.2-linux-x64.deb" length="100" sparkle:edSignature="sig-deb" sparkle:sha256="sha-deb"/>
    </item>
    <item>
      <title>Hello 1.2</title>
      <link>https://example.com/releases/1.2</link>
      <sparkle:version>12</sparkle:version>
      <sparkle:shortVersionString>1.2</sparkle:shortVersionString>
      <enclosure url="https://example.com/hello-1.2-linux-x64.pkg.tar.gz" length="200" sparkle:edSignature="sig-arch" sparkle:sha256="sha-arch"/>
    </item>
    <item>
      <title>Hello 1.2</title>
      <link>https://example.com/releases/1.2</link>
      <sparkle:version>12</sparkle:version>
      <sparkle:shortVersionString>1.2</sparkle:shortVersionString>
      <enclosure url="https://example.com/hello-1.2-linux-x64.AppImage" length="300" sparkle:edSignature="sig-appimage" sparkle:sha256="sha-appimage"/>
    </item>
  </channel>
</rss>`
	items, err := ParseAppcastItems([]byte(multiFeed))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("ParseAppcastItems returned %d items, want 3", len(items))
	}
	if first, err := ParseAppcast([]byte(multiFeed)); err != nil || first != items[0] {
		t.Fatalf("ParseAppcast = %+v, %v; want first item %+v", first, err, items[0])
	}
	serve(t, 200, multiFeed)
	cfg := Config{App: "Hello", Version: "1.1", Build: "11", Feed: "https://example.com/appcast-linux_x64.xml", Key: "a2V5"}

	arch := FindMatching(context.Background(), cfg, func(it Item) bool {
		return LinuxPackageFormat(it.Download) == LinuxPackageArch
	})
	if arch.Download != "https://example.com/hello-1.2-linux-x64.pkg.tar.gz" || arch.Signature != "sig-arch" || arch.SHA256 != "sha-arch" || arch.Size != 200 {
		t.Fatalf("arch match = %+v", arch)
	}

	appImg := FindMatching(context.Background(), cfg, func(it Item) bool {
		return LinuxPackageFormat(it.Download) == LinuxPackageAppImage
	})
	if appImg.Download != "https://example.com/hello-1.2-linux-x64.AppImage" || appImg.Signature != "sig-appimage" || appImg.Size != 300 {
		t.Fatalf("appimage match = %+v", appImg)
	}

	unmanaged := FindMatching(context.Background(), cfg, func(Item) bool { return false })
	if unmanaged.Version != "1.2" || unmanaged.URL != "https://example.com/releases/1.2" || unmanaged.Download != "" {
		t.Fatalf("unmanaged match = %+v, want release page without download", unmanaged)
	}
}

func TestFindInMacAppStoreSelectsMacSoftware(t *testing.T) {
	asked := serve(t, 200, `{"results":[{"version":"2.0","trackId":123}]}`)
	c := Config{App: "Hello", ID: "com.example.hello.mac", Version: "1.9", Channel: "store"}
	got := FindInMacAppStore(t.Context(), c, "GB")
	if got.Version != "2.0" || got.URL != "macappstore://apps.apple.com/app/id123" || got.Managed != driver.UpdateManagedPage {
		t.Fatalf("Mac store update = %+v", got)
	}
	if len(*asked) != 1 || (*asked)[0] != "https://itunes.apple.com/lookup?bundleId=com.example.hello.mac&country=gb&entity=desktopSoftware" {
		t.Fatalf("Mac lookup = %v", *asked)
	}
	c.Development = true
	if got := FindInMacAppStore(t.Context(), c, "GB"); got.Managed != driver.UpdateManagedDevelopment || len(*asked) != 1 {
		t.Fatalf("development lookup = %+v, requests %v", got, *asked)
	}
}
