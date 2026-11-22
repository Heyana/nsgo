package windows

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Cookie expiry is Unix seconds in WebView2, including subsecond precision.
func cookieExpirySeconds(at time.Time) float64 {
	return float64(at.Unix()) + float64(at.Nanosecond())/1e9
}
func cookieExpiryTime(seconds float64) time.Time {
	whole := int64(seconds)
	return time.Unix(whole, int64((seconds-float64(whole))*1e9)).UTC()
}

// holdsWebCookies reports whether stored holds each of want that the store keeps, by its name, value, domain and path, and no longer holds one written expired, which DeleteCookiesWithDomainAndPath removes; a cookie a later one of its name, domain and path replaces is skipped. A live cookie's domain compares without its leading dot, which WebView2 may write either way for a domain cookie; an expired one's compares with it, as the deletion's key does, so a domain cookie of the same name and path does not hold up a host-only one's removal.
func holdsWebCookies(stored, want []driver.WebCookie) bool {
	same := func(a, b driver.WebCookie) bool {
		return a.Name == b.Name && a.Host() == b.Host() && a.CookiePath() == b.CookiePath()
	}
	sameKey := func(a, b driver.WebCookie) bool { return same(a, b) && a.HostOnly() == b.HostOnly() }
	now := time.Now()
	for i, c := range want {
		replaced := false
		for _, later := range want[i+1:] {
			replaced = replaced || same(c, later)
		}
		if replaced {
			continue
		}
		if !c.Expires.IsZero() && !c.Expires.After(now) {
			for _, s := range stored {
				if sameKey(s, c) {
					return false
				}
			}
			continue
		}
		found := false
		for _, s := range stored {
			if same(s, c) && s.Value == c.Value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
