package linux

import (
	"fmt"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// soupCookieMaxAge returns the SoupCookie max_age for expires at now: -1 for a session cookie, 0 for one already expired (which removes the matching cookie), and at least 1 for a future expiry so a sub-second expiry does not truncate to expired before soup_cookie_set_expires runs.
func soupCookieMaxAge(expires, now time.Time) int32 {
	if expires.IsZero() {
		return -1
	}
	if !expires.After(now) {
		return 0
	}
	secs := expires.Sub(now) / time.Second
	if secs < 1 {
		return 1
	}
	return int32(min(secs, 1<<31-1))
}

// prepareSoupCookies validates and builds every cookie in cookies before any is passed to WebKitGTK. libsoup's SoupCookieJar silently drops non-IP cookies whose domain is a public suffix without reporting an error to webkit_cookie_manager_add_cookie_finish, so public suffixes are rejected here first; on any failure, every already-built SoupCookie is freed and no cookie is written.
func prepareSoupCookies(cookies []driver.WebCookie, isIP, isPublicSuffix func(string) bool, build func(driver.WebCookie) uintptr, free func(uintptr)) ([]uintptr, string) {
	built := make([]uintptr, 0, len(cookies))
	freeAll := func() {
		for _, handle := range built {
			free(handle)
		}
	}
	for _, c := range cookies {
		if isPublicSuffix != nil && (isIP == nil || !isIP(c.Domain)) && isPublicSuffix(c.Domain) {
			freeAll()
			return nil, fmt.Sprintf("nsgo: libsoup refused the cookie %q of %s: domain is a public suffix", c.Name, c.Domain)
		}
		handle := build(c)
		if handle == 0 {
			freeAll()
			return nil, fmt.Sprintf("nsgo: libsoup refused the cookie %q of %s", c.Name, c.Domain)
		}
		built = append(built, handle)
	}
	return built, ""
}
