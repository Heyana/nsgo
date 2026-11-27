//go:build darwin

package apple

import (
	"fmt"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

const webKitPath = "/System/Library/Frameworks/WebKit.framework/WebKit"

// WebCookies carries out an ItemWebCookies in WKWebsiteDataStore's default store, whose WKHTTPCookieStore every web view but an Incognito one reads: it sets the cookies, lists them or removes every type of website data, and answers once WebKit has. The store exists without a web view, and WebKit writes it to disk, so a cookie with an expiry outlives the app. A target without WebKit, tvOS, has no store.
func WebCookies(id uint32, r driver.WebCookiesRequest, dispatch func(driver.Event)) {
	if _, err := purego.Dlopen(webKitPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		dispatch(driver.NoWebCookies(&driver.MountItem{Count: id}))
		return
	}
	loadWebKit()
	store := objc.ID(Class("WKWebsiteDataStore")).Send(sel("defaultDataStore"))
	switch r.Op {
	case driver.WebCookiesList:
		cookies := store.Send(sel("httpCookieStore"))
		launchNetworkProcess(store, func() { listWebCookies(id, cookies, r.Filter, dispatch) })
	case driver.WebCookiesClear:
		clearWebsiteData(id, store, dispatch)
	default:
		setWebCookies(id, store.Send(sel("httpCookieStore")), r.Cookies, dispatch)
	}
}

// prepareHTTPCookies builds every NSHTTPCookie before any is written to WebKit, failing the whole batch if Foundation refuses any cookie.
func prepareHTTPCookies(cookies []driver.WebCookie, build func(driver.WebCookie) objc.ID) ([]objc.ID, string) {
	built := make([]objc.ID, len(cookies))
	for i, c := range cookies {
		cookie := build(c)
		if cookie == 0 {
			return nil, fmt.Sprintf("nsgo: Foundation refused the cookie %q of %s", c.Name, c.Domain)
		}
		built[i] = cookie
	}
	return built, ""
}

// setWebCookies sets each of cookies in store and answers once WebKit has taken them all, building every cookie before writing any.
func setWebCookies(id uint32, store objc.ID, cookies []driver.WebCookie, dispatch func(driver.Event)) {
	built, failure := prepareHTTPCookies(cookies, httpCookie)
	if failure != "" {
		dispatch(driver.Event{Kind: driver.EventWebCookies, Count: id, Text: failure})
		return
	}
	if len(built) == 0 {
		dispatch(driver.Event{Kind: driver.EventWebCookies, Count: id})
		return
	}
	pending := len(built)
	finish := func() {
		if pending--; pending == 0 {
			dispatch(driver.Event{Kind: driver.EventWebCookies, Count: id})
		}
	}
	for _, cookie := range built {
		// WebKit calls the handler on the main thread; OnMain keeps the count there whatever it does.
		set := objcblock.New(func(objc.Block) { OnMain(finish) })
		store.Send(sel("setCookie:completionHandler:"), cookie, set)
		set.Release()
	}
}

// launchNetworkProcess calls next once store's network process, which holds the cookies WebKit kept on disk, runs. WebKit before 313745@main (https://webkit.org/b/315310), macOS 15's among it, answers getAllCookies: with no cookies while that process has not started, which it does with the app's first web view or a request that needs it, so a listing before any web view showed missed every cookie of an earlier run; fetching the store's cookie records starts it on a persistent store, and setCookie: and removeDataOfTypes: start it themselves.
func launchNetworkProcess(store objc.ID, next func()) {
	types := objc.ID(Class("NSSet")).Send(sel("setWithObject:"), Global(webKitPath, "WKWebsiteDataTypeCookies"))
	fetched := objcblock.New(func(objc.Block, objc.ID) { next() })
	store.Send(sel("fetchDataRecordsOfTypes:completionHandler:"), types, fetched)
	fetched.Release()
}

// listWebCookies answers with the cookies of store that f picks, which getAllCookies: lists whole for FilterWebCookies to filter.
func listWebCookies(id uint32, store objc.ID, f driver.WebCookieFilter, dispatch func(driver.Event)) {
	list := objcblock.New(func(_ objc.Block, all objc.ID) {
		// The array is WebKit's for the block's run alone, so its cookies are read here.
		n := objc.Send[uint](all, sel("count"))
		cookies := make([]driver.WebCookie, 0, n)
		for i := range n {
			cookies = append(cookies, webCookie(all.Send(sel("objectAtIndex:"), i)))
		}
		dispatch(driver.Event{Kind: driver.EventWebCookies, Count: id, Data: driver.FilterWebCookies(cookies, f, time.Now())})
	})
	store.Send(sel("getAllCookies:"), list)
	list.Release()
}

// clearWebsiteData removes every type of website data store holds, over all time, and answers once WebKit has.
func clearWebsiteData(id uint32, store objc.ID, dispatch func(driver.Event)) {
	types := objc.ID(Class("WKWebsiteDataStore")).Send(sel("allWebsiteDataTypes"))
	since := objc.ID(Class("NSDate")).Send(sel("distantPast"))
	done := objcblock.New(func(objc.Block) {
		dispatch(driver.Event{Kind: driver.EventWebCookies, Count: id})
	})
	store.Send(sel("removeDataOfTypes:modifiedSince:completionHandler:"), types, since, done)
	done.Release()
}

// webCookie reads an NSHTTPCookie. Foundation has a policy for SameSite Lax and Strict alone, so a cookie set with None, like one without the attribute, reads as WebSameSiteDefault.
func webCookie(cookie objc.ID) driver.WebCookie {
	str := func(name string) string { return GoString(cookie.Send(sel(name))) }
	flag := func(name string) bool { return objc.Send[bool](cookie, sel(name)) }
	c := driver.WebCookie{Name: str("name"), Value: str("value"), Domain: str("domain"), Path: str("path"), Secure: flag("isSecure"), HTTPOnly: flag("isHTTPOnly")}
	if !flag("isSessionOnly") {
		if at := cookie.Send(sel("expiresDate")); at != 0 {
			seconds := objc.Send[float64](at, sel("timeIntervalSince1970"))
			whole := int64(seconds)
			c.Expires = time.Unix(whole, int64((seconds-float64(whole))*1e9)).UTC()
		}
	}
	switch str("sameSitePolicy") {
	case "lax":
		c.SameSite = driver.WebSameSiteLax
	case "strict":
		c.SameSite = driver.WebSameSiteStrict
	}
	return c
}

// httpCookie returns c as an autoreleased NSHTTPCookie, 0 when Foundation refuses it. A session cookie is one to discard, and SameSite None leaves the policy out, as Foundation has none for it and WebKit restricts no cookie without one.
func httpCookie(c driver.WebCookie) objc.ID {
	props := objc.ID(Class("NSMutableDictionary")).Send(sel("dictionary"))
	set := func(key string, value objc.ID) {
		if k := Global(foundationPath, key); k != 0 && value != 0 {
			props.Send(sel("setObject:forKey:"), value, k)
		}
	}
	yes := NSString("TRUE")
	set("NSHTTPCookieName", NSString(c.Name))
	set("NSHTTPCookieValue", NSString(c.Value))
	set("NSHTTPCookieDomain", NSString(c.Domain))
	set("NSHTTPCookiePath", NSString(c.CookiePath()))
	if c.Expires.IsZero() {
		set("NSHTTPCookieDiscard", yes)
	} else {
		// Cookie dates extend beyond UnixNano's 1678–2262 range.
		seconds := float64(c.Expires.Unix()) + float64(c.Expires.Nanosecond())/1e9
		set("NSHTTPCookieExpires", objc.ID(Class("NSDate")).Send(sel("dateWithTimeIntervalSince1970:"), seconds))
	}
	if c.Secure {
		set("NSHTTPCookieSecure", yes)
	}
	if c.HTTPOnly {
		// Foundation exports no key for it; CFNetwork reads the attribute's name, as it parses Set-Cookie.
		props.Send(sel("setObject:forKey:"), yes, NSString("HttpOnly"))
	}
	switch c.SameSite {
	case driver.WebSameSiteLax:
		set("NSHTTPCookieSameSitePolicy", Global(foundationPath, "NSHTTPCookieSameSiteLax"))
	case driver.WebSameSiteStrict:
		set("NSHTTPCookieSameSitePolicy", Global(foundationPath, "NSHTTPCookieSameSiteStrict"))
	}
	return objc.ID(Class("NSHTTPCookie")).Send(sel("cookieWithProperties:"), props)
}
