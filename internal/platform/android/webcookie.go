//go:build android || nsgoandroidhost

package android

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// withoutWebCookies hands the cookie requests of surface id's batch b to the runtime's CookieManager, which every web view of the app shares, Incognito ones included, as msgCookies, and returns b without them. Java answers each with msgCookiesSet once it set and flushed the cookies, read those of a URL or cleared the store, carrying the surface and the request's count back. CookieManager reads cookies only by URL, so a list by domain, or of the whole store, is answered here as unavailable.
func (s *surface) withoutWebCookies(id uint32, b *driver.MountBatch) *driver.MountBatch {
	cookies := func(it driver.MountItem) bool { return it.Kind == driver.ItemWebCookies }
	if !slices.ContainsFunc(b.Items, cookies) {
		return b
	}
	for _, it := range b.Items {
		if !cookies(it) {
			continue
		}
		if r := it.Cookies; r.Op == driver.WebCookiesList && r.Filter.URL == "" {
			s.dispatchTo(id, driver.Event{Kind: driver.EventWebCookies, Count: it.Count, Text: driver.NoWebCookieDomains})
			continue
		}
		s.send(msgCookies, func(w *writer) { writeWebCookies(w, id, it.Count, it.Cookies) })
	}
	out := *b
	out.Items = slices.DeleteFunc(slices.Clone(b.Items), cookies)
	return &out
}

// webCookiesSet dispatches a msgCookiesSet to the surface whose request it answers.
func (s *surface) webCookiesSet(r *reader) {
	surface, ev := readWebCookiesSet(r)
	s.dispatchTo(surface, ev)
}
