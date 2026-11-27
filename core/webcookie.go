package core

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// WebCookie is a cookie of the store the app's web views share: its name and value, the Domain and Path it goes to, when it Expires (zero for a session cookie, which goes as the app ends; Apple platforms, Windows and Android keep one at most 400 days, as browsers do), whether it goes only over HTTPS (Secure) and stays out of page scripts' reach (HTTPOnly), and its SameSite attribute. Domain is a host, which alone takes the cookie, or with a leading dot (".example.com") the host and every host under it, as a Set-Cookie Domain attribute makes it; Path "" is "/".
type WebCookie = driver.WebCookie

// WebSameSite is a WebCookie's SameSite attribute.
type WebSameSite = driver.WebSameSite

// The SameSite attributes of a WebCookie.
const (
	// WebSameSiteDefault leaves the attribute out, so the platform's default for a cookie without one applies, which differs: Chromium's web views, on Windows and Android, take it as Lax.
	WebSameSiteDefault = driver.WebSameSiteDefault
	WebSameSiteLax     = driver.WebSameSiteLax
	WebSameSiteStrict  = driver.WebSameSiteStrict
	// WebSameSiteNone sends the cookie with requests from other sites too, which needs Secure.
	WebSameSiteNone = driver.WebSameSiteNone
)

// SetWebCookies adds cookies to the store every web view of the app shares, replacing those of the same name, domain and path, HttpOnly ones included, which page scripts can neither read nor write, and calls done on the engine's goroutine once the store holds them, so that a page loaded after it sends them: an app carries a sign-in into its web views this way. An Expires in the past removes the same name, domain and path. It works whether or not a web view is shown. The store is the persistent one: a cookie with an Expires outlives the app, and an Incognito web view never sees these, but on Android, which keeps one cookie store per app. Apple platforms write WKWebsiteDataStore's default store, Windows the WebView2 profile the app's web views share, through a hidden web view while none shows, Linux the app's WebKitGTK network session and Android CookieManager. Where the platform has no store, as on HarmonyOS, on the web and where Env.HasWebView is false, done gets an Unavailable error, and a cookie no platform would take, such as one without a name or a domain, SameSite None without Secure, or a "__Secure-" or "__Host-" name without what its prefix needs, fails before any is written; on Linux, libsoup also refuses domains on the Public Suffix List (including its private section) before any cookie in the batch is written. Code an Access holds needs the webview capability's cookies (AccessCookies) for each cookie's URL.
func (cx *Scope) SetWebCookies(cookies []WebCookie, done func(err error)) {
	answer := func(err error) {
		if done != nil {
			done(err)
		}
	}
	t := cx.tree()
	if t == nil {
		return
	}
	r := driver.WebCookiesRequest{Cookies: make([]WebCookie, len(cookies))}
	for i, c := range cookies {
		c, err := checkWebCookie(c)
		if err != nil {
			answer(fmt.Errorf("core: web cookie %d (%q): %w", i, c.Name, err))
			return
		}
		if cx.denied(serviceWebCookies, c.URL()) {
			answer(AccessDenied(AccessWebView, c.URL()))
			return
		}
		r.Cookies[i] = c
	}
	if len(r.Cookies) == 0 {
		answer(nil)
		return
	}
	cx.webCookies(t, r, func(_ driver.Event, err error) { answer(err) })
}

// WebCookieFilter picks cookies of the store the app's web views share: those a request to URL, an http or https URL, would send, by their domain, path and Secure, or those of Domain, a host, and every host under it. The zero filter picks all; a filter takes one or the other.
type WebCookieFilter = driver.WebCookieFilter

// WebCookies lists the cookies of the store every web view of the app shares that f picks, HttpOnly ones included, which page scripts cannot read, and calls done with them on the engine's goroutine, in the order of their host, path and name; an app reads the cookies of a sign-in made in its web views this way, to call the site from Go or to show where the user is signed in. It works whether or not a web view is shown, and reads the persistent store alone, so an Incognito web view's cookies are never among them. Apple platforms, Windows and Linux list each cookie with every field, filtering in Go; Android's CookieManager answers only the names and values a URL would send, so there a URL filter fills Domain with the URL's host and leaves the other fields empty, and a Domain filter or the zero one gets an Unavailable error. Where the platform has no store, as on HarmonyOS, on the web and where Env.HasWebView is false, done gets an Unavailable error too. Code an Access holds needs the webview capability's cookies (AccessCookies) for the filter's URL, for "https://" and the domain, or for "" to list the whole store.
func (cx *Scope) WebCookies(f WebCookieFilter, done func(cookies []WebCookie, err error)) {
	answer := func(cookies []WebCookie, err error) {
		if done != nil {
			done(cookies, err)
		}
	}
	t := cx.tree()
	if t == nil {
		return
	}
	f, resource, err := checkWebCookieFilter(f)
	if err != nil {
		answer(nil, err)
		return
	}
	if cx.denied(serviceWebCookies, resource) {
		answer(nil, AccessDenied(AccessWebView, resource))
		return
	}
	cx.webCookies(t, driver.WebCookiesRequest{Op: driver.WebCookiesList, Filter: f}, func(ev driver.Event, err error) {
		cookies, _ := ev.Data.([]WebCookie)
		if err != nil {
			cookies = nil
		} else if cookies == nil {
			cookies = []WebCookie{}
		}
		answer(cookies, err)
	})
}

// ClearWebsiteData removes everything the store every web view of the app shares holds, cookies, local and session storage, IndexedDB, service workers and caches, and calls done on the engine's goroutine once it is gone, so that a page loaded after it starts signed out and with empty storage: an app's "sign out of every site" or "clear browsing data". A page a web view shows keeps what it already read until it loads again. It works whether or not a web view is shown, and leaves Incognito web views alone, which keep their data in memory. Apple platforms remove all of WKWebsiteDataStore's default store's website data types, Windows clears the WebView2 profile's browsing data (ClearBrowsingDataAll), through a hidden web view while none shows, Linux clears the app's WebKitGTK network session's website data manager, and Android removes CookieManager's cookies and WebStorage's data, which leaves the HTTP cache of web views. Where the platform has no store, as on HarmonyOS, on the web and where Env.HasWebView is false, done gets an Unavailable error. Code an Access holds needs the webview capability's cookies (AccessCookies) for "".
func (cx *Scope) ClearWebsiteData(done func(err error)) {
	answer := func(err error) {
		if done != nil {
			done(err)
		}
	}
	t := cx.tree()
	if t == nil {
		return
	}
	if cx.denied(serviceWebCookies, "") {
		answer(AccessDenied(AccessWebView, ""))
		return
	}
	cx.webCookies(t, driver.WebCookiesRequest{Op: driver.WebCookiesClear}, func(_ driver.Event, err error) { answer(err) })
}

// webCookies sends r, with the schemes the current scope serves, and calls answer with the platform's event and its error: Unavailable where the platform has no store or no listing by domain.
func (cx *Scope) webCookies(t *Tree, r driver.WebCookiesRequest, answer func(driver.Event, error)) {
	t.enableWebView()
	r.Schemes = slices.Sorted(maps.Keys(webSchemesOf(cx.current())))
	t.request(cx, driver.MountItem{Kind: driver.ItemWebCookies, Cookies: &r}, func(ev driver.Event) {
		switch ev.Text {
		case "":
			answer(ev, nil)
		case driver.NoWebCookieStore, driver.NoWebCookieDomains:
			answer(ev, Unavailable(ev.Text))
		default:
			answer(ev, errors.New(ev.Text))
		}
	})
}

// checkWebCookieFilter returns f as platforms take it, a URL's scheme and host and a domain in lower case, and the resource Access is asked for, or why it picks nothing.
func checkWebCookieFilter(f WebCookieFilter) (WebCookieFilter, string, error) {
	switch {
	case f.URL != "" && f.Domain != "":
		return f, "", errors.New("core: a web cookie filter takes a URL or a domain, not both")
	case f.URL != "":
		u, err := url.Parse(f.URL)
		// Parse writes the scheme in lower case.
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return f, "", fmt.Errorf("core: a web cookie filter's URL is an http or https URL with a host, not %q", f.URL)
		}
		u.Host = strings.ToLower(u.Host)
		f.URL = u.String()
		return f, f.URL, nil
	case f.Domain != "":
		d := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(f.Domain), "."))
		if d == "" || strings.ContainsAny(d, "/;, []") || strings.HasSuffix(d, ".") {
			return f, "", fmt.Errorf("core: a web cookie filter's domain is a host, not %q", f.Domain)
		}
		f.Domain = d
		return f, WebCookie{Domain: d, Secure: true}.URL(), nil
	}
	return f, "", nil
}

// checkWebCookie returns c as the platforms take it, its domain in lower case, or why none would.
func checkWebCookie(c WebCookie) (WebCookie, error) {
	bad := func(s, also string) bool {
		return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f || strings.ContainsRune(also, r) })
	}
	switch {
	case c.Name == "":
		return c, errors.New("a cookie has a name")
	case bad(c.Name, "=;, \t"):
		return c, errors.New("a cookie's name has no '=', ';', ',', spaces or control characters")
	case bad(c.Value, ";"):
		return c, errors.New("a cookie's value has no ';' or control characters")
	case c.Path != "" && (!strings.HasPrefix(c.Path, "/") || bad(c.Path, "; ")):
		return c, errors.New("a cookie's path starts with '/' and has no ';', spaces or control characters")
	}
	c.Domain = strings.ToLower(strings.TrimSpace(c.Domain))
	host := c.Host()
	if host == "" || bad(host, "/;, []") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return c, fmt.Errorf("a cookie's domain is a host, or a leading dot and a host, not %q", c.Domain)
	}
	ipv4 := strings.Trim(host, "0123456789.") == ""
	ipv6 := strings.Count(host, ":") >= 2 && strings.Trim(host, "0123456789abcdef:.") == ""
	switch {
	case (ipv4 || ipv6) && !c.HostOnly():
		return c, errors.New("an IP address takes no leading dot: the cookie goes to that address alone")
	case !ipv6 && strings.Contains(host, ":"):
		return c, fmt.Errorf("a cookie's domain has no port: %q", c.Domain)
	}
	switch {
	case c.SameSite == WebSameSiteNone && !c.Secure:
		return c, errors.New("SameSite None needs Secure")
	case c.SameSite > WebSameSiteNone:
		return c, fmt.Errorf("no SameSite attribute %d", c.SameSite)
	case strings.HasPrefix(c.Name, "__Secure-") && !c.Secure:
		return c, errors.New("a __Secure- cookie needs Secure")
	case strings.HasPrefix(c.Name, "__Host-") && (!c.Secure || !c.HostOnly() || c.CookiePath() != "/"):
		return c, errors.New("a __Host- cookie needs Secure, the path \"/\" and a domain without a leading dot")
	}
	return c, nil
}
