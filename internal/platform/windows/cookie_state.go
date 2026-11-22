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

// holdsWebCookies checks each key's last write: a live cookie must be present
// with its value, and an expired one must be absent. WebView2 distinguishes
// host-only and domain cookies, so their keys keep that scope too.
func holdsWebCookies(stored, want []driver.WebCookie) bool {
	same := func(a, b driver.WebCookie) bool {
		return a.Name == b.Name && a.Host() == b.Host() && a.HostOnly() == b.HostOnly() && a.CookiePath() == b.CookiePath()
	}
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
				if same(s, c) {
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
