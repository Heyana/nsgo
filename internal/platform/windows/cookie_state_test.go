package windows

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestHoldsWebCookiesSkipsWhatTheStoreDrops(t *testing.T) {
	stored := []driver.WebCookie{{Name: "a", Value: "1", Domain: ".example.com", Path: "/"}, {Name: "b", Value: "2", Domain: "example.com"}}
	for _, c := range []struct {
		want []driver.WebCookie
		held bool
	}{
		{[]driver.WebCookie{{Name: "a", Value: "1", Domain: ".example.com"}, {Name: "b", Value: "2", Domain: "example.com", Path: "/"}}, true},
		{[]driver.WebCookie{{Name: "a", Value: "1", Domain: "example.com"}}, false},
		{[]driver.WebCookie{{Name: "a", Value: "0", Domain: ".example.com"}}, false},
		{[]driver.WebCookie{{Name: "c", Value: "3", Domain: "example.com"}}, false},
		// Replaced by the next, and expired.
		{[]driver.WebCookie{{Name: "a", Value: "0", Domain: ".example.com"}, {Name: "a", Value: "1", Domain: ".example.com"}}, true},
		{[]driver.WebCookie{{Name: "c", Value: "3", Domain: "example.com", Expires: time.Unix(1, 0)}}, true},
	} {
		if got := holdsWebCookies(stored, c.want); got != c.held {
			t.Errorf("holdsWebCookies(%v) = %v, want %v", c.want, got, c.held)
		}
	}
}

func TestCookieExpiryOutsideNanoseconds(t *testing.T) {
	for _, at := range []time.Time{
		time.Date(1601, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2026, time.November, 22, 1, 2, 3, 625000000, time.UTC),
	} {
		got := cookieExpiryTime(cookieExpirySeconds(at))
		if !got.Equal(at) {
			t.Errorf("expiry %s became %s", at, got)
		}
	}
}

func TestExpiredCookieWaitsForDeletion(t *testing.T) {
	c := driver.WebCookie{Name: "sid", Value: "old", Domain: ".example.com", Path: "/"}
	expired := c
	expired.Expires = time.Date(1601, time.January, 1, 0, 0, 0, 0, time.UTC)
	if holdsWebCookies([]driver.WebCookie{c}, []driver.WebCookie{expired}) {
		t.Fatal("acknowledged expiry while the old cookie is still stored")
	}
	if !holdsWebCookies(nil, []driver.WebCookie{expired}) {
		t.Fatal("did not acknowledge the cookie's removal")
	}
	// Removing the host-only cookie must not wait for a domain cookie
	// with the same name and path, which is a different WebView2 key.
	expired.Domain = "example.com"
	if !holdsWebCookies([]driver.WebCookie{c}, []driver.WebCookie{expired}) {
		t.Fatal("a domain cookie blocked the host-only cookie's deletion")
	}
}
