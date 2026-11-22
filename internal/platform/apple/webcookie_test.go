//go:build darwin

package apple

import (
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestHTTPCookieCarriesEveryAttribute builds the NSHTTPCookie SetWebCookies hands WebKit and reads its attributes back, HttpOnly, which Foundation has no key for, among them.
func TestHTTPCookieCarriesEveryAttribute(t *testing.T) {
	// withPool keeps the goroutine on the pool's OS thread: a pool drained on another thread, after the scheduler moved the goroutine, crashes in Foundation.
	withPool(func() { testHTTPCookieCarriesEveryAttribute(t) })
}

func testHTTPCookieCarriesEveryAttribute(t *testing.T) {
	// Foundation keeps a cookie at most 400 days, as RFC 6265bis says.
	expires := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	for _, c := range []struct {
		cookie   driver.WebCookie
		session  bool
		sameSite string
	}{
		{driver.WebCookie{Name: "SID", Value: "v", Domain: ".example.com", Path: "/watch", Expires: expires, Secure: true, HTTPOnly: true, SameSite: driver.WebSameSiteStrict}, false, "strict"},
		{driver.WebCookie{Name: "pref", Value: "1", Domain: "www.example.com", SameSite: driver.WebSameSiteLax}, true, "lax"},
		{driver.WebCookie{Name: "x", Value: "2", Domain: "example.com", Secure: true, SameSite: driver.WebSameSiteNone}, true, ""},
	} {
		cookie := httpCookie(c.cookie)
		if cookie == 0 {
			t.Fatalf("%+v: Foundation refused it", c.cookie)
		}
		str := func(name string) string { return GoString(cookie.Send(sel(name))) }
		flag := func(name string) bool { return objc.Send[bool](cookie, sel(name)) }
		if str("name") != c.cookie.Name || str("value") != c.cookie.Value || str("domain") != c.cookie.Domain || str("path") != c.cookie.CookiePath() {
			t.Errorf("%+v: NSHTTPCookie %s=%s for %s%s", c.cookie, str("name"), str("value"), str("domain"), str("path"))
		}
		if flag("isSecure") != c.cookie.Secure || flag("isHTTPOnly") != c.cookie.HTTPOnly || flag("isSessionOnly") != c.session {
			t.Errorf("%+v: secure %v, HttpOnly %v, session %v", c.cookie, flag("isSecure"), flag("isHTTPOnly"), flag("isSessionOnly"))
		}
		if got := str("sameSitePolicy"); got != c.sameSite {
			t.Errorf("%+v: SameSite %q, want %q", c.cookie, got, c.sameSite)
		}
		if !c.session {
			at := objc.Send[float64](cookie.Send(sel("expiresDate")), sel("timeIntervalSince1970"))
			if int64(at) != expires.Unix() {
				t.Errorf("%+v: expires at %v", c.cookie, at)
			}
		}
	}
}

// TestWebCookieReadsWhatHTTPCookieWrites reads back the NSHTTPCookie SetWebCookies hands WebKit as WebCookies lists it: every field, but SameSite None, which Foundation has no policy for, reads as the default.
func TestWebCookieReadsWhatHTTPCookieWrites(t *testing.T) {
	withPool(func() {
		expires := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second).UTC()
		for _, c := range []driver.WebCookie{
			{Name: "SID", Value: "v", Domain: ".example.com", Path: "/watch", Expires: expires, Secure: true, HTTPOnly: true, SameSite: driver.WebSameSiteStrict},
			{Name: "pref", Value: "1", Domain: "www.example.com", Path: "/", SameSite: driver.WebSameSiteLax},
			{Name: "x", Value: "2", Domain: "example.com", Path: "/", Secure: true, SameSite: driver.WebSameSiteNone},
		} {
			want := c
			if want.SameSite == driver.WebSameSiteNone {
				want.SameSite = driver.WebSameSiteDefault
			}
			if got := webCookie(httpCookie(c)); got != want {
				t.Errorf("%+v read back as %+v", c, got)
			}
		}
	})
}

func TestHTTPCookieExpiryOutsideNanoseconds(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name    string
		expires time.Time
		future  bool
	}{
		{"expired", time.Date(1601, time.January, 1, 0, 0, 0, 0, time.UTC), false},
		{"persistent", time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withPool(func() {
				c := driver.WebCookie{Name: tc.name, Value: "fixture", Domain: ".example.com", Path: "/", Expires: tc.expires, Secure: true, HTTPOnly: true}
				cookie := httpCookie(c)
				if cookie == 0 {
					t.Fatal("Foundation refused a valid cookie expiry")
				}
				got := webCookie(cookie)
				// WebKit may cap a long-lived cookie's lifetime; its expiry must stay on the right side of now in both directions.
				if got.Expires.IsZero() || got.Expires.After(now) != tc.future {
					t.Fatalf("expiry %s became %s (want future %t)", tc.expires, got.Expires, tc.future)
				}
			})
		})
	}
}
