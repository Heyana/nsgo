//go:build windows

package windows

// The web views' cookie store (core.Scope.SetWebCookies, WebCookies and ClearWebsiteData). WebView2 reaches a profile's cookies and browsing data only through a web view of it (ICoreWebView2_2::get_CookieManager, ICoreWebView2_13::get_Profile), so a request uses one of the window's that shows pages of the app's profile, and while there is none, a hidden web view of the window's own, made for it in the profile's environment (starting it, with the schemes the request names, as a web view's first batch would). The hidden view stays until a web view of the profile shows pages or the window closes: WebView2's processes, and the session cookies they hold, end with the profile's last web view. Its cookie calls apply after they return, so a request setting cookies answers once GetCookies lists them, failing after webCookieSettle. The vtable indexes follow WebView2.idl, as webview2.go's do.

import (
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	iidGetCookiesCompleted       = parseGUID("5a4f5069-5c15-47c3-8646-f4de1c116670")
	iidWeb13                     = parseGUID("f75f09a8-667e-4983-88d6-c8773f315e84")
	iidProfile2                  = parseGUID("fa740d4b-5eae-4344-a8ad-74be31925397")
	iidClearBrowsingDataComplete = parseGUID("e9710a06-1d1d-49b2-8234-226f35846ae5")
)

// Vtable indexes.
const (
	// ICoreWebView2_2, after ICoreWebView2's.
	web2GetCookieManager = 66

	// ICoreWebView2CookieManager
	cookieManagerCreateCookie      = 3
	cookieManagerGetCookies        = 5
	cookieManagerAddOrUpdateCookie = 6
	cookieManagerDeleteByKey       = 9 // DeleteCookiesWithDomainAndPath

	// ICoreWebView2_13, after ICoreWebView2_12's.
	web13GetProfile = 105

	// ICoreWebView2Profile2, after ICoreWebView2Profile's.
	profileClearBrowsingDataAll = 12

	// ICoreWebView2Cookie
	cookieGetName       = 3
	cookieGetValue      = 4
	cookieGetDomain     = 6
	cookieGetPath       = 7
	cookieGetExpires    = 8
	cookiePutExpires    = 9
	cookieGetIsHttpOnly = 10
	cookiePutIsHttpOnly = 11
	cookieGetSameSite   = 12
	cookiePutSameSite   = 13
	cookieGetIsSecure   = 14
	cookiePutIsSecure   = 15
	cookieGetIsSession  = 16

	// ICoreWebView2CookieList
	cookieListGetCount        = 3
	cookieListGetValueAtIndex = 4
)

// COREWEBVIEW2_COOKIE_SAME_SITE_KIND
const (
	cookieSameSiteNone   = 0
	cookieSameSiteLax    = 1
	cookieSameSiteStrict = 2
)

// webCookieSettle is how long a request waits for GetCookies to list its cookies, and for WebView2 to answer at all.
const webCookieSettle = 5 * time.Second

// webDataClearWait is how long a ClearWebsiteData request waits for ClearBrowsingDataAll to answer.
const webDataClearWait = time.Minute

// cookieView is a window's hidden web view for its cookie requests.
type cookieView struct {
	comp, controller, web *comObject
	creating              bool
	waiting               []func(*comObject, error)
	// requests is how many requests use it; it closes only once none does.
	requests int
}

// webCookies carries out an ItemWebCookies.
func (w *window) webCookies(id uint32, r driver.WebCookiesRequest) {
	finished := false
	var view *cookieView
	var page *webView
	finish := func(ev driver.Event) {
		if finished {
			return
		}
		finished = true
		if page != nil {
			delete(page.cookieRequests, id)
		}
		if view != nil {
			view.requests--
			w.closeCookieViewWhenUnneeded()
		}
		if w.engine != nil && w.hwnd != 0 {
			ev.Kind, ev.Count = driver.EventWebCookies, id
			w.engine.Dispatch(ev)
		}
	}
	fail := func(text string) { finish(driver.Event{Text: text}) }
	// A web view that closes in the middle of the request may never call its handlers. Clearing deletes the profile's caches from disk, which can take longer than a cookie call, so it waits longer before calling the request lost.
	wait := webCookieSettle + time.Second
	if r.Op == driver.WebCookiesClear {
		wait = webDataClearWait
	}
	time.AfterFunc(wait, func() {
		w.post(func() { fail("nsgo: WebView2 did not answer the cookie request") })
	})
	start := func(web *comObject) {
		if r.Op == driver.WebCookiesClear {
			clearBrowsingData(web, func(err error) {
				if err != nil {
					fail("nsgo: " + err.Error())
					return
				}
				finish(driver.Event{})
			})
			return
		}
		manager := cookieManagerOf(web)
		if manager == nil {
			fail("nsgo: WebView2 has no cookie manager (ICoreWebView2_2)")
			return
		}
		if r.Op == driver.WebCookiesList {
			listWebCookies(manager, func(stored []driver.WebCookie, err error) {
				manager.release()
				if err != nil {
					fail("nsgo: " + err.Error())
					return
				}
				finish(driver.Event{Data: driver.FilterWebCookies(stored, r.Filter, time.Now())})
			})
			return
		}
		for _, c := range r.Cookies {
			if err := addWebCookie(manager, c); err != nil {
				manager.release()
				fail("nsgo: " + err.Error())
				return
			}
		}
		deadline := time.Now().Add(webCookieSettle)
		var settle func()
		settle = func() {
			listWebCookies(manager, func(stored []driver.WebCookie, err error) {
				switch {
				case finished:
					manager.release()
				case err != nil:
					manager.release()
					fail("nsgo: " + err.Error())
				case holdsWebCookies(stored, r.Cookies):
					manager.release()
					finish(driver.Event{})
				case time.Now().After(deadline):
					manager.release()
					fail("nsgo: WebView2 did not keep the cookies")
				default:
					settle()
				}
			})
		}
		settle()
	}
	var run func()
	run = func() {
		if finished {
			return
		}
		// A view the batch that asked deletes closes after this, so the request starts again on another, or the hidden one, as it does.
		if page = w.pageWebView(); page != nil {
			if page.cookieRequests == nil {
				page.cookieRequests = map[uint32]func(){}
			}
			page.cookieRequests[id] = run
			start(page.web)
			return
		}
		// Counted before it may answer, which an error made at once does.
		if w.cookies == nil {
			w.cookies = &cookieView{}
		}
		view = w.cookies
		view.requests++
		w.ensureCookieView(r.Schemes, func(web *comObject, err error) {
			if finished {
				return
			}
			if err != nil {
				fail("nsgo: " + err.Error())
				return
			}
			start(web)
		})
	}
	run()
}

// pageWebView returns a web view of w that shows pages of the app's profile, nil for none.
func (w *window) pageWebView() *webView {
	for wv := range w.webs {
		if wv.ready && !wv.closed && !wv.failed && wv.web != nil && !wv.config.Incognito {
			return wv
		}
	}
	return nil
}

// ensureCookieView makes w's hidden web view, w.cookies, in the profile's environment unless it has one, and calls fn with its ICoreWebView2 once it does, or with why it could not be made.
func (w *window) ensureCookieView(schemes []string, fn func(*comObject, error)) {
	cv := w.cookies
	if cv.web != nil {
		fn(cv.web, nil)
		return
	}
	cv.waiting = append(cv.waiting, fn)
	if cv.creating {
		return
	}
	cv.creating = true
	ready := func(web *comObject, err error) {
		cv.creating = false
		waiting := cv.waiting
		cv.waiting = nil
		for _, fn := range waiting {
			fn(web, err)
		}
	}
	withWebEnvironment(w, w.profileFolder(), schemes, func(env *comObject, err error) {
		if err != nil {
			ready(nil, err)
			return
		}
		env3 := env.query(&iidEnvironment3)
		if env3 == nil {
			ready(nil, hresultError("ICoreWebView2Environment3", eNoInterface))
			return
		}
		defer env3.release()
		var h *comHandler
		h = newHandler(&iidCompositionControllerCompleted, func(hr uintptr, result unsafe.Pointer) {
			defer h.release()
			comp := (*comObject)(result)
			if failed(hr) || comp == nil {
				ready(nil, hresultError("CreateCoreWebView2CompositionController", hr))
				return
			}
			comp.addRef()
			controller := comp.query(&iidController)
			var web *comObject
			if controller != nil {
				controller.call(controllerPutIsVisible, 0)
				controller.call(controllerGetCoreWebView2, uintptr(unsafe.Pointer(&web)))
			}
			if web == nil || w.hwnd == 0 {
				if controller != nil {
					controller.call(controllerClose)
				}
				releaseAll(&web, &controller, &comp)
				ready(nil, hresultError("get_CoreWebView2", eFail))
				return
			}
			cv.comp, cv.controller, cv.web = comp, controller, web
			ready(web, nil)
		})
		if hr := env3.call(envCreateCompositionController, w.hwnd, h.ptr()); failed(hr) {
			h.release()
			ready(nil, hresultError("CreateCoreWebView2CompositionController", hr))
		}
	})
}

// closeCookieViewWhenUnneeded closes w's hidden web view once no request uses it and a web view of the profile shows pages, which keeps WebView2's processes, and the session cookies, alive. It closes from a message of its own: a request finishes in a completion handler of the hidden view's, and closing a web view inside one of its handlers ends the message loop.
func (w *window) closeCookieViewWhenUnneeded() {
	cv := w.cookies
	if cv == nil || cv.web == nil || cv.requests > 0 || w.pageWebView() == nil {
		return
	}
	w.post(func() {
		if w.cookies == cv && cv.requests == 0 && w.pageWebView() != nil {
			w.closeCookieView()
		}
	})
}

// closeCookieView closes w's hidden web view, as the window closes or once it is not needed.
func (w *window) closeCookieView() {
	cv := w.cookies
	if cv == nil {
		return
	}
	w.cookies = nil
	if cv.controller != nil {
		cv.controller.call(controllerClose)
	}
	releaseAll(&cv.web, &cv.controller, &cv.comp)
}

// cookieManagerOf returns web's profile's cookie manager, a reference the caller releases, nil without ICoreWebView2_2.
func cookieManagerOf(web *comObject) *comObject {
	web2 := web.query(&iidWeb2)
	if web2 == nil {
		return nil
	}
	defer web2.release()
	var manager *comObject
	if failed(web2.call(web2GetCookieManager, uintptr(unsafe.Pointer(&manager)))) {
		return nil
	}
	return manager
}

// addWebCookie adds or replaces c in manager's store, or removes its key when expired. A cookie without an expiry is a session one, as CreateCookie makes it.
func addWebCookie(manager *comObject, c driver.WebCookie) error {
	if !c.Expires.IsZero() && !c.Expires.After(time.Now()) {
		// WebView2 refuses negative expiry times (except -1, a session
		// cookie), so delete expired cookies by their key instead.
		hr := manager.call(cookieManagerDeleteByKey, uintptr(unsafe.Pointer(wide(c.Name))),
			uintptr(unsafe.Pointer(wide(c.Domain))), uintptr(unsafe.Pointer(wide(c.CookiePath()))))
		if failed(hr) {
			return hresultError("DeleteCookiesWithDomainAndPath "+c.Name, hr)
		}
		return nil
	}
	var cookie *comObject
	hr := manager.call(cookieManagerCreateCookie, uintptr(unsafe.Pointer(wide(c.Name))), uintptr(unsafe.Pointer(wide(c.Value))),
		uintptr(unsafe.Pointer(wide(c.Domain))), uintptr(unsafe.Pointer(wide(c.CookiePath()))), uintptr(unsafe.Pointer(&cookie)))
	if failed(hr) || cookie == nil {
		return hresultError("CreateCookie "+c.Name, hr)
	}
	defer cookie.release()
	if !c.Expires.IsZero() {
		if hr := cookie.callDouble(cookiePutExpires, cookieExpirySeconds(c.Expires)); failed(hr) {
			return hresultError("put_Expires", hr)
		}
	}
	if hr := cookie.call(cookiePutIsHttpOnly, boolArg(c.HTTPOnly)); failed(hr) {
		return hresultError("put_IsHttpOnly", hr)
	}
	if hr := cookie.call(cookiePutIsSecure, boolArg(c.Secure)); failed(hr) {
		return hresultError("put_IsSecure", hr)
	}
	policy := -1
	switch c.SameSite {
	case driver.WebSameSiteLax:
		policy = cookieSameSiteLax
	case driver.WebSameSiteStrict:
		policy = cookieSameSiteStrict
	case driver.WebSameSiteNone:
		policy = cookieSameSiteNone
	}
	if policy >= 0 {
		if hr := cookie.call(cookiePutSameSite, uintptr(policy)); failed(hr) {
			return hresultError("put_SameSite", hr)
		}
	}
	if hr := manager.call(cookieManagerAddOrUpdateCookie, cookie.ptr()); failed(hr) {
		return hresultError("AddOrUpdateCookie "+c.Name, hr)
	}
	return nil
}

// clearBrowsingData clears every kind of browsing data of web's profile, over all time, and calls done once WebView2 has.
func clearBrowsingData(web *comObject, done func(error)) {
	web13 := web.query(&iidWeb13)
	if web13 == nil {
		done(hresultError("ICoreWebView2_13", eNoInterface))
		return
	}
	defer web13.release()
	var profile *comObject
	if hr := web13.call(web13GetProfile, uintptr(unsafe.Pointer(&profile))); failed(hr) || profile == nil {
		done(hresultError("get_Profile", hr))
		return
	}
	defer profile.release()
	profile2 := profile.query(&iidProfile2)
	if profile2 == nil {
		done(hresultError("ICoreWebView2Profile2", eNoInterface))
		return
	}
	defer profile2.release()
	var h *comHandler
	// Its Invoke takes the HRESULT alone; the handler's second word goes unread.
	h = newHandler(&iidClearBrowsingDataComplete, func(hr uintptr, _ unsafe.Pointer) {
		defer h.release()
		if failed(hr) {
			done(hresultError("ClearBrowsingDataAll", hr))
			return
		}
		done(nil)
	})
	if hr := profile2.call(profileClearBrowsingDataAll, h.ptr()); failed(hr) {
		h.release()
		done(hresultError("ClearBrowsingDataAll", hr))
	}
}

// listWebCookies calls done with every cookie of manager's store, with every field.
func listWebCookies(manager *comObject, done func([]driver.WebCookie, error)) {
	var h *comHandler
	h = newHandler(&iidGetCookiesCompleted, func(hr uintptr, result unsafe.Pointer) {
		defer h.release()
		list := (*comObject)(result)
		if failed(hr) || list == nil {
			done(nil, hresultError("GetCookies", hr))
			return
		}
		var count uint32
		if hr := list.call(cookieListGetCount, uintptr(unsafe.Pointer(&count))); failed(hr) {
			done(nil, hresultError("ICoreWebView2CookieList::get_Count", hr))
			return
		}
		cookies := make([]driver.WebCookie, 0, count)
		for i := range count {
			var c *comObject
			if failed(list.call(cookieListGetValueAtIndex, uintptr(i), uintptr(unsafe.Pointer(&c)))) || c == nil {
				continue
			}
			cookies = append(cookies, webCookie(c))
			c.release()
		}
		done(cookies, nil)
	})
	// An empty URI lists every cookie of the profile.
	if hr := manager.call(cookieManagerGetCookies, uintptr(unsafe.Pointer(wide(""))), h.ptr()); failed(hr) {
		h.release()
		done(nil, hresultError("GetCookies", hr))
	}
}

// webCookie reads an ICoreWebView2Cookie, whose domain has the leading dot of a cookie every host under it shares, as Chromium keeps it. WebView2's SameSite has no kind for a cookie set without the attribute, and reads such a one as one of the three, Lax by its documentation.
func webCookie(cookie *comObject) driver.WebCookie {
	flag := func(method int) bool {
		var v int32
		return !failed(cookie.call(method, uintptr(unsafe.Pointer(&v)))) && v != 0
	}
	c := driver.WebCookie{Name: cookie.getString(cookieGetName), Value: cookie.getString(cookieGetValue), Domain: cookie.getString(cookieGetDomain),
		Path: cookie.getString(cookieGetPath), Secure: flag(cookieGetIsSecure), HTTPOnly: flag(cookieGetIsHttpOnly)}
	var expires float64
	if !flag(cookieGetIsSession) && !failed(cookie.call(cookieGetExpires, uintptr(unsafe.Pointer(&expires)))) && expires > 0 {
		c.Expires = cookieExpiryTime(expires)
	}
	var policy int32 = -1
	if !failed(cookie.call(cookieGetSameSite, uintptr(unsafe.Pointer(&policy)))) {
		switch policy {
		case cookieSameSiteLax:
			c.SameSite = driver.WebSameSiteLax
		case cookieSameSiteStrict:
			c.SameSite = driver.WebSameSiteStrict
		case cookieSameSiteNone:
			c.SameSite = driver.WebSameSiteNone
		}
	}
	return c
}
