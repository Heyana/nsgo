package linux

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSoupCookieMaxAge(t *testing.T) {
	now := time.Date(2026, time.November, 27, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		expires time.Time
		want    int32
	}{
		{"session", time.Time{}, -1},
		{"past", time.Date(1601, time.January, 1, 0, 0, 0, 0, time.UTC), 0},
		{"now", now, 0},
		{"subsecond future", now.Add(250 * time.Millisecond), 1},
		{"future", now.Add(time.Hour), 3600},
		{"far future", time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC), 1<<31 - 1},
	} {
		if got := soupCookieMaxAge(tc.expires, now); got != tc.want {
			t.Errorf("%s: soupCookieMaxAge = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestPrepareSoupCookiesRejectsBeforeWriting(t *testing.T) {
	var next uintptr = 100
	var freed []uintptr
	build := func(c driver.WebCookie) uintptr {
		if c.Name == "bad" {
			return 0
		}
		next++
		return next
	}
	free := func(h uintptr) { freed = append(freed, h) }
	isIP := func(d string) bool { return d == "127.0.0.1" || d == "::1" }
	isPublicSuffix := func(d string) bool {
		h := strings.TrimPrefix(d, ".")
		return h == "httpbin.org" || h == "com" || h == "127.0.0.1"
	}

	// A public suffix after a valid cookie fails and frees the first cookie without writing either.
	handles, failure := prepareSoupCookies([]driver.WebCookie{
		{Name: "ok", Value: "1", Domain: ".example.com"},
		{Name: "psl", Value: "2", Domain: ".httpbin.org"},
	}, isIP, isPublicSuffix, build, free)
	if handles != nil || !strings.Contains(failure, "public suffix") || !strings.Contains(failure, "psl") {
		t.Fatalf("public suffix = %v, %q; want public suffix failure", handles, failure)
	}
	if !slices.Equal(freed, []uintptr{101}) {
		t.Fatalf("freed %v, want [101]", freed)
	}

	// A build refusal after a valid cookie also frees the earlier cookie.
	freed = nil
	handles, failure = prepareSoupCookies([]driver.WebCookie{
		{Name: "ok", Value: "1", Domain: "example.com"},
		{Name: "bad", Value: "2", Domain: "example.com"},
	}, isIP, isPublicSuffix, build, free)
	if handles != nil || !strings.Contains(failure, "bad") {
		t.Fatalf("build failure = %v, %q", handles, failure)
	}
	if !slices.Equal(freed, []uintptr{102}) {
		t.Fatalf("freed %v, want [102]", freed)
	}

	// IP addresses bypass the public-suffix check, as libsoup does.
	freed = nil
	handles, failure = prepareSoupCookies([]driver.WebCookie{
		{Name: "ip", Value: "1", Domain: "127.0.0.1"},
		{Name: "ok", Value: "2", Domain: "www.httpbin.org"},
	}, isIP, isPublicSuffix, build, free)
	if failure != "" || len(handles) != 2 || len(freed) != 0 {
		t.Fatalf("valid batch = %v, %q, freed %v", handles, failure, freed)
	}
}
