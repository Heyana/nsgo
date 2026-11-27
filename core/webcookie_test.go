package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// cookieAccess allows the cookies of hosts and nothing else.
type cookieAccess struct{ hosts []string }

func (a cookieAccess) Allows(req core.AccessRequest) bool {
	if req.Permission != core.AccessWebView || req.Method != core.AccessCookies {
		return false
	}
	for _, h := range a.hosts {
		if strings.HasPrefix(req.Resource, "https://"+h+"/") || strings.HasPrefix(req.Resource, "http://"+h+"/") {
			return true
		}
	}
	return false
}

func (cookieAccess) WebHosts() []string { return nil }

func startCookies(t *testing.T, access core.Access) (*nsgotest.App, *core.Scope) {
	t.Helper()
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		if access != nil {
			cx.ProvideAccess(access)
		}
		return core.View{}
	})
	return app, scope
}

func TestSetWebCookiesKeepsThemInTheStore(t *testing.T) {
	app, cx := startCookies(t, nil)
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	calls := 0
	var got error
	set := func(cookies ...core.WebCookie) {
		cx.SetWebCookies(cookies, func(err error) { got = err; calls++ })
		app.Settle()
	}
	set(core.WebCookie{Name: "SID", Value: "a", Domain: ".Example.com", HTTPOnly: true, Secure: true, SameSite: core.WebSameSiteNone, Expires: expires},
		core.WebCookie{Name: "pref", Value: "x", Domain: "www.example.com", Path: "/watch"})
	set(core.WebCookie{Name: "SID", Value: "b", Domain: ".example.com", Path: "/", HTTPOnly: true})
	if calls != 2 || got != nil {
		t.Fatalf("done ran %d times, last with %v", calls, got)
	}
	want := []core.WebCookie{
		{Name: "pref", Value: "x", Domain: "www.example.com", Path: "/watch"},
		{Name: "SID", Value: "b", Domain: ".example.com", Path: "/", HTTPOnly: true},
	}
	if !slices.Equal(app.WebCookies, want) {
		t.Fatalf("store %+v, want %+v", app.WebCookies, want)
	}
	// Writing a past expiry removes only the matching name, domain and path.
	set(core.WebCookie{Name: "SID", Domain: ".example.com", Path: "/", Expires: time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)})
	if calls != 3 || got != nil || !slices.Equal(app.WebCookies, want[:1]) {
		t.Fatalf("after expiry: calls %d, err %v, store %+v, want %+v", calls, got, app.WebCookies, want[:1])
	}
}

func TestSetWebCookiesRefusesWhatNoPlatformTakes(t *testing.T) {
	app, cx := startCookies(t, nil)
	for _, c := range []core.WebCookie{
		{Value: "v", Domain: "example.com"},
		{Name: "a b", Domain: "example.com"},
		{Name: "a", Value: "x;y", Domain: "example.com"},
		{Name: "a", Domain: ""},
		{Name: "a", Domain: "."},
		{Name: "a", Domain: "https://example.com"},
		{Name: "a", Domain: "example.com:443"},
		{Name: "a", Domain: ".127.0.0.1"},
		{Name: "a", Domain: "example.com", Path: "watch"},
		{Name: "a", Domain: "example.com", SameSite: core.WebSameSiteNone},
		{Name: "__Secure-a", Domain: "example.com"},
		{Name: "__Host-a", Domain: ".example.com", Secure: true},
		{Name: "__Host-a", Domain: "example.com", Path: "/x", Secure: true},
	} {
		var got error
		ran := false
		// A valid cookie before it goes nowhere either.
		cx.SetWebCookies([]core.WebCookie{{Name: "ok", Domain: "example.com"}, c}, func(err error) { got, ran = err, true })
		if !ran || got == nil {
			t.Errorf("%+v: done ran %v with %v, want an error at once", c, ran, got)
		}
	}
	app.Settle()
	if len(app.WebCookies) != 0 {
		t.Errorf("store %+v, want nothing", app.WebCookies)
	}
	// What the platforms take.
	for _, c := range []core.WebCookie{
		{Name: "a", Domain: "127.0.0.1"},
		{Name: "a", Domain: "::1"},
		{Name: "__Host-a", Domain: "example.com", Secure: true},
		{Name: "a", Value: "with spaces, commas and \"quotes\"", Domain: "localhost"},
	} {
		var got error
		cx.SetWebCookies([]core.WebCookie{c}, func(err error) { got = err })
		app.Settle()
		if got != nil {
			t.Errorf("%+v: %v", c, got)
		}
	}
}

func TestSetWebCookiesReportsThePlatform(t *testing.T) {
	app, cx := startCookies(t, nil)
	app.SetWebCookies = func([]core.WebCookie) error { return errors.New("the store is locked") }
	var got error
	cx.SetWebCookies([]core.WebCookie{{Name: "a", Domain: "example.com"}}, func(err error) { got = err })
	app.Settle()
	if got == nil || got.Error() != "the store is locked" || len(app.WebCookies) != 0 {
		t.Fatalf("done heard %v, store %+v", got, app.WebCookies)
	}
	ran := false
	cx.SetWebCookies(nil, func(err error) { ran = err == nil })
	if !ran {
		t.Error("no cookies did not answer at once")
	}
}

func TestSetWebCookiesHeldToAccess(t *testing.T) {
	app, cx := startCookies(t, cookieAccess{hosts: []string{"example.com"}})
	var got error
	cx.SetWebCookies([]core.WebCookie{{Name: "a", Domain: "example.com"}}, func(err error) { got = err })
	app.Settle()
	if got != nil || len(app.WebCookies) != 1 {
		t.Fatalf("allowed: %v, store %+v", got, app.WebCookies)
	}
	cx.SetWebCookies([]core.WebCookie{{Name: "b", Domain: "example.com"}, {Name: "c", Domain: ".other.com", Secure: true}}, func(err error) { got = err })
	app.Settle()
	if core.ErrorCode(got) != core.CodeAccessDenied || !strings.Contains(got.Error(), "https://other.com/") || len(app.WebCookies) != 1 {
		t.Fatalf("refused: %v, store %+v", got, app.WebCookies)
	}
}

func TestWebCookiesListsTheStore(t *testing.T) {
	app, cx := startCookies(t, nil)
	app.WebCookies = []core.WebCookie{
		{Name: "s", Value: "1", Domain: ".example.test", Path: "/", Secure: true, HTTPOnly: true, SameSite: core.WebSameSiteLax, Expires: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)},
		{Name: "www", Value: "2", Domain: "www.example.test", Path: "/watch"},
		{Name: "other", Value: "3", Domain: "other.test"},
	}
	list := func(f core.WebCookieFilter) ([]core.WebCookie, error) {
		var got []core.WebCookie
		var err error
		calls := 0
		cx.WebCookies(f, func(cookies []core.WebCookie, e error) { got, err = cookies, e; calls++ })
		app.Settle()
		if calls != 1 {
			t.Fatalf("%+v: done ran %d times", f, calls)
		}
		return got, err
	}
	got, err := list(core.WebCookieFilter{Domain: ".Example.TEST"})
	if err != nil || !slices.Equal(got, app.WebCookies[:2]) {
		t.Errorf("a domain listed %+v, %v", got, err)
	}
	got, err = list(core.WebCookieFilter{URL: "HTTPS://WWW.Example.test/watch/1"})
	if err != nil || !slices.Equal(got, app.WebCookies[:2]) {
		t.Errorf("a URL listed %+v, %v", got, err)
	}
	got, err = list(core.WebCookieFilter{URL: "http://www.example.test/"})
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("a URL that takes none listed %#v, %v", got, err)
	}
	got, err = list(core.WebCookieFilter{})
	if err != nil || len(got) != 3 {
		t.Errorf("the zero filter listed %+v, %v", got, err)
	}
	for _, f := range []core.WebCookieFilter{
		{URL: "https://example.test/", Domain: "example.test"},
		{URL: "ftp://example.test/"},
		{URL: "example.test"},
		{URL: "https:///path"},
		{Domain: "."},
		{Domain: "example.test/"},
		{Domain: "example.test."},
	} {
		ran := false
		cx.WebCookies(f, func(cookies []core.WebCookie, err error) {
			ran = true
			if err == nil || cookies != nil {
				t.Errorf("%+v answered %+v, %v, want an error", f, cookies, err)
			}
		})
		if !ran {
			t.Errorf("%+v did not answer at once", f)
		}
	}
}

func TestClearWebsiteDataEmptiesTheStore(t *testing.T) {
	app, cx := startCookies(t, nil)
	app.WebCookies = []core.WebCookie{{Name: "s", Value: "1", Domain: ".example.test"}}
	var got error
	calls := 0
	cx.ClearWebsiteData(func(err error) { got = err; calls++ })
	app.Settle()
	if calls != 1 || got != nil || len(app.WebCookies) != 0 || app.WebsiteDataClears != 1 {
		t.Fatalf("done ran %d times with %v; store %+v after %d clears", calls, got, app.WebCookies, app.WebsiteDataClears)
	}
	var cookies []core.WebCookie
	cx.WebCookies(core.WebCookieFilter{}, func(c []core.WebCookie, err error) { cookies, got = c, err })
	app.Settle()
	if got != nil || len(cookies) != 0 {
		t.Errorf("after clearing the store listed %+v, %v", cookies, got)
	}
}

func TestWebCookiesHeldToAccess(t *testing.T) {
	app, cx := startCookies(t, cookieAccess{hosts: []string{"example.test"}})
	app.WebCookies = []core.WebCookie{{Name: "s", Value: "1", Domain: ".example.test"}, {Name: "o", Value: "2", Domain: "other.test"}}
	for _, c := range []struct {
		filter core.WebCookieFilter
		denied bool
	}{
		{core.WebCookieFilter{Domain: "example.test"}, false},
		{core.WebCookieFilter{URL: "https://example.test/x"}, false},
		{core.WebCookieFilter{Domain: "other.test"}, true},
		{core.WebCookieFilter{URL: "https://other.test/"}, true},
		{core.WebCookieFilter{}, true},
	} {
		var got error
		cx.WebCookies(c.filter, func(_ []core.WebCookie, err error) { got = err })
		app.Settle()
		if denied := core.ErrorCode(got) == core.CodeAccessDenied; denied != c.denied {
			t.Errorf("%+v answered %v, want denied %v", c.filter, got, c.denied)
		}
	}
	var got error
	cx.ClearWebsiteData(func(err error) { got = err })
	app.Settle()
	if core.ErrorCode(got) != core.CodeAccessDenied || len(app.WebCookies) != 2 {
		t.Errorf("clearing answered %v, store %+v", got, app.WebCookies)
	}
}
