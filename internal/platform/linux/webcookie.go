//go:build linux && !android

package linux

import (
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The cookie store's calls, bound with the rest of WebKitGTK (bindWebCookies).
var (
	webkitCookieManagerAddCookie       func(manager, cookie, cancellable, callback, data uintptr)
	webkitCookieManagerAddCookieFinish func(manager, result, gerr uintptr) int32
	soupCookieNew                      func(name, value, domain, path *byte, maxAge int32) uintptr
	soupCookieSetSecure                func(cookie uintptr, on int32)
	soupCookieSetHTTPOnly              func(cookie uintptr, on int32)
	soupCookieSetSameSitePolicy        func(cookie uintptr, policy int32)
	soupCookieFree                     func(cookie uintptr)
	soupCookieGetName                  func(cookie uintptr) uintptr
	soupCookieGetValue                 func(cookie uintptr) uintptr
	soupCookieGetDomain                func(cookie uintptr) uintptr
	soupCookieGetPath                  func(cookie uintptr) uintptr
	soupCookieGetExpires               func(cookie uintptr) uintptr
	soupCookieGetSecure                func(cookie uintptr) int32
	soupCookieGetHTTPOnly              func(cookie uintptr) int32
	soupCookieGetSameSitePolicy        func(cookie uintptr) int32
	soupCookieSetExpires               func(cookie, dateTime uintptr)
	soupTLDIsPublicSuffix              func(domain *byte) int32
	gHostnameIsIPAddress               func(hostname *byte) int32
	gDateTimeToUnix                    func(dateTime uintptr) int64
	gDateTimeNewFromUnixUTC            func(unix int64) uintptr
	gDateTimeUnref                     func(dateTime uintptr)
	webkitNetworkSessionGetDataManager func(session uintptr) uintptr
	webkitWebsiteDataManagerClear      func(manager uintptr, types uint32, timespan int64, cancellable, callback, data uintptr)
	webkitWebsiteDataManagerClearDone  func(manager, result, gerr uintptr) int32
	// webkit_cookie_manager_get_all_cookies came with WebKitGTK 2.42, after 6.0's first release, 2.40, so they stay nil without it.
	webkitCookieManagerGetAllCookies       func(manager, cancellable, callback, data uintptr)
	webkitCookieManagerGetAllCookiesFinish func(manager, result, gerr uintptr) uintptr
)

func bindWebCookies(webkit, soup uintptr) {
	if _, err := purego.Dlsym(webkit, "webkit_cookie_manager_get_all_cookies"); err == nil {
		purego.RegisterLibFunc(&webkitCookieManagerGetAllCookies, webkit, "webkit_cookie_manager_get_all_cookies")
		purego.RegisterLibFunc(&webkitCookieManagerGetAllCookiesFinish, webkit, "webkit_cookie_manager_get_all_cookies_finish")
	}
	purego.RegisterLibFunc(&webkitNetworkSessionGetDataManager, webkit, "webkit_network_session_get_website_data_manager")
	purego.RegisterLibFunc(&webkitWebsiteDataManagerClear, webkit, "webkit_website_data_manager_clear")
	purego.RegisterLibFunc(&webkitWebsiteDataManagerClearDone, webkit, "webkit_website_data_manager_clear_finish")
	purego.RegisterLibFunc(&soupCookieGetName, soup, "soup_cookie_get_name")
	purego.RegisterLibFunc(&soupCookieGetValue, soup, "soup_cookie_get_value")
	purego.RegisterLibFunc(&soupCookieGetDomain, soup, "soup_cookie_get_domain")
	purego.RegisterLibFunc(&soupCookieGetPath, soup, "soup_cookie_get_path")
	purego.RegisterLibFunc(&soupCookieGetExpires, soup, "soup_cookie_get_expires")
	purego.RegisterLibFunc(&soupCookieGetSecure, soup, "soup_cookie_get_secure")
	purego.RegisterLibFunc(&soupCookieGetHTTPOnly, soup, "soup_cookie_get_http_only")
	purego.RegisterLibFunc(&soupCookieGetSameSitePolicy, soup, "soup_cookie_get_same_site_policy")
	purego.RegisterLibFunc(&soupCookieSetExpires, soup, "soup_cookie_set_expires")
	purego.RegisterLibFunc(&soupTLDIsPublicSuffix, soup, "soup_tld_domain_is_public_suffix")
	purego.RegisterLibFunc(&gHostnameIsIPAddress, glibLib, "g_hostname_is_ip_address")
	purego.RegisterLibFunc(&gDateTimeToUnix, glibLib, "g_date_time_to_unix")
	purego.RegisterLibFunc(&gDateTimeNewFromUnixUTC, glibLib, "g_date_time_new_from_unix_utc")
	purego.RegisterLibFunc(&gDateTimeUnref, glibLib, "g_date_time_unref")
	purego.RegisterLibFunc(&webkitCookieManagerAddCookie, webkit, "webkit_cookie_manager_add_cookie")
	purego.RegisterLibFunc(&webkitCookieManagerAddCookieFinish, webkit, "webkit_cookie_manager_add_cookie_finish")
	purego.RegisterLibFunc(&soupCookieNew, soup, "soup_cookie_new")
	purego.RegisterLibFunc(&soupCookieSetSecure, soup, "soup_cookie_set_secure")
	purego.RegisterLibFunc(&soupCookieSetHTTPOnly, soup, "soup_cookie_set_http_only")
	purego.RegisterLibFunc(&soupCookieSetSameSitePolicy, soup, "soup_cookie_set_same_site_policy")
	purego.RegisterLibFunc(&soupCookieFree, soup, "soup_cookie_free")
}

// webCookies carries out an ItemWebCookies in the app's network session, which every web view but an Incognito one uses and which keeps cookies in its SQLite file (webSession), so it needs no web view: it sets cookies or lists them in its cookie manager, or clears every type of data its website data manager keeps, and answers once WebKit has.
func (w *window) webCookies(id uint32, r driver.WebCookiesRequest) {
	answer := func(ev driver.Event) {
		if !w.closing {
			ev.Kind, ev.Count = driver.EventWebCookies, id
			w.engine.Dispatch(ev)
		}
	}
	if err := openWebKit(); err != nil {
		answer(driver.Event{Text: "nsgo: " + err.Error()})
		return
	}
	session := w.webSession()
	if r.Op == driver.WebCookiesClear {
		clearWebsiteData(session, answer)
		return
	}
	manager := webkitNetworkSessionGetCookieManager(session)
	if manager == 0 {
		answer(driver.Event{Text: "nsgo: WebKitGTK's network session has no cookie manager"})
		return
	}
	if r.Op == driver.WebCookiesList {
		listWebCookies(manager, r.Filter, answer)
		return
	}
	setWebCookies(manager, r.Cookies, answer)
}

// setWebCookies adds cookies in manager and answers once WebKit has added them all, rejecting any cookie libsoup would refuse or silently drop before writing any.
func setWebCookies(manager uintptr, cookies []driver.WebCookie, answer func(driver.Event)) {
	isIP := func(d string) bool { return gHostnameIsIPAddress(cstr(d)) != 0 }
	isPublicSuffix := func(d string) bool { return soupTLDIsPublicSuffix(cstr(d)) != 0 }
	built, failure := prepareSoupCookies(cookies, isIP, isPublicSuffix, soupCookie, soupCookieFree)
	if failure != "" {
		answer(driver.Event{Text: failure})
		return
	}
	if len(built) == 0 {
		answer(driver.Event{})
		return
	}
	pending := len(built)
	finish := func(text string) {
		if failure == "" {
			failure = text
		}
		if pending--; pending == 0 {
			answer(driver.Event{Text: failure})
		}
	}
	for i, cookie := range built {
		name := cookies[i].Name
		webkitCookieManagerAddCookie(manager, cookie, 0, cbAsync, async(func(source, result uintptr) {
			var gerr uintptr
			if webkitCookieManagerAddCookieFinish(source, result, uintptr(unsafe.Pointer(&gerr))) != 0 {
				finish("")
				return
			}
			text := "nsgo: WebKitGTK did not add the cookie " + name
			if err := dialogError(gerr); err != nil {
				text = "nsgo: " + err.Error()
			}
			finish(text)
		}))
		// WebKit copies the cookie as it is added.
		soupCookieFree(cookie)
	}
}

// listWebCookies answers with the cookies of manager that f picks, which webkit_cookie_manager_get_all_cookies lists whole for FilterWebCookies to filter.
func listWebCookies(manager uintptr, f driver.WebCookieFilter, answer func(driver.Event)) {
	if webkitCookieManagerGetAllCookies == nil {
		answer(driver.Event{Text: "nsgo: listing web view cookies needs WebKitGTK 2.42 or later"})
		return
	}
	webkitCookieManagerGetAllCookies(manager, 0, cbAsync, async(func(source, result uintptr) {
		var gerr uintptr
		list := webkitCookieManagerGetAllCookiesFinish(source, result, uintptr(unsafe.Pointer(&gerr)))
		if gerr != 0 {
			text := "nsgo: WebKitGTK did not list the cookies"
			if err := dialogError(gerr); err != nil {
				text = "nsgo: " + err.Error()
			}
			answer(driver.Event{Text: text})
			return
		}
		var cookies []driver.WebCookie
		// The list and its cookies are the caller's (transfer full).
		for node := list; node != 0; {
			n := (*gListNode)(cPointer(node))
			cookies = append(cookies, webCookie(n.data))
			soupCookieFree(n.data)
			node = n.next
		}
		gListFree(list)
		answer(driver.Event{Data: driver.FilterWebCookies(cookies, f, time.Now())})
	}))
}

// gListNode is a GList's node.
type gListNode struct {
	data, next, prev uintptr
}

// webCookie reads a SoupCookie, whose domain has the leading dot of a cookie every host under it shares.
func webCookie(cookie uintptr) driver.WebCookie {
	c := driver.WebCookie{Name: goString(soupCookieGetName(cookie)), Value: goString(soupCookieGetValue(cookie)), Domain: goString(soupCookieGetDomain(cookie)),
		Path: goString(soupCookieGetPath(cookie)), Secure: soupCookieGetSecure(cookie) != 0, HTTPOnly: soupCookieGetHTTPOnly(cookie) != 0}
	// libsoup 3's expiry is a GDateTime it keeps, NULL for a session cookie.
	if at := soupCookieGetExpires(cookie); at != 0 {
		c.Expires = time.Unix(gDateTimeToUnix(at), 0).UTC()
	}
	switch soupCookieGetSameSitePolicy(cookie) {
	case soupSameSiteLax:
		c.SameSite = driver.WebSameSiteLax
	case soupSameSiteStrict:
		c.SameSite = driver.WebSameSiteStrict
	case soupSameSiteNone:
		c.SameSite = driver.WebSameSiteNone
	}
	return c
}

// webkitWebsiteDataAll is WEBKIT_WEBSITE_DATA_ALL as WebKitGTK 2.56's 6.0 API has it, with file system storage; an older one ignores the types it has not.
const webkitWebsiteDataAll = 1<<13 - 1

// clearWebsiteData removes every type of website data session keeps, over all time (a timespan of 0), and answers once WebKit has.
func clearWebsiteData(session uintptr, answer func(driver.Event)) {
	manager := webkitNetworkSessionGetDataManager(session)
	if manager == 0 {
		answer(driver.Event{Text: "nsgo: WebKitGTK's network session has no website data manager"})
		return
	}
	webkitWebsiteDataManagerClear(manager, webkitWebsiteDataAll, 0, 0, cbAsync, async(func(source, result uintptr) {
		var gerr uintptr
		if webkitWebsiteDataManagerClearDone(source, result, uintptr(unsafe.Pointer(&gerr))) != 0 {
			answer(driver.Event{})
			return
		}
		text := "nsgo: WebKitGTK did not clear the website data"
		if err := dialogError(gerr); err != nil {
			text = "nsgo: " + err.Error()
		}
		answer(driver.Event{Text: text})
	}))
}

// SoupSameSitePolicy
const (
	soupSameSiteNone   = 0
	soupSameSiteLax    = 1
	soupSameSiteStrict = 2
)

// soupCookie returns c as a SoupCookie the caller frees, 0 when libsoup refuses it. libsoup's domain takes the leading dot of a cookie every host under it shares, as c's does, and a max age of -1 makes a session cookie.
func soupCookie(c driver.WebCookie) uintptr {
	maxAge := soupCookieMaxAge(c.Expires, time.Now())
	cookie := soupCookieNew(cstr(c.Name), cstr(c.Value), cstr(c.Domain), cstr(c.CookiePath()), maxAge)
	if cookie == 0 {
		return 0
	}
	if maxAge > 0 {
		// The max age counts from now, so it would end the cookie up to a second early; the expiry itself is the second asked for.
		if at := gDateTimeNewFromUnixUTC(c.Expires.Unix()); at != 0 {
			soupCookieSetExpires(cookie, at)
			gDateTimeUnref(at)
		}
	}
	soupCookieSetSecure(cookie, boolInt(c.Secure))
	soupCookieSetHTTPOnly(cookie, boolInt(c.HTTPOnly))
	switch c.SameSite {
	case driver.WebSameSiteLax:
		soupCookieSetSameSitePolicy(cookie, soupSameSiteLax)
	case driver.WebSameSiteStrict:
		soupCookieSetSameSitePolicy(cookie, soupSameSiteStrict)
	case driver.WebSameSiteNone:
		soupCookieSetSameSitePolicy(cookie, soupSameSiteNone)
	}
	return cookie
}
