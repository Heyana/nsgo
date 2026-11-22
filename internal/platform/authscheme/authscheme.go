// Package authscheme opens desktop sign-in sessions whose private callback scheme the app registers. The core receives the callback through OpenLinks; sessions without a scheme remain the platform's loopback flow.
package authscheme

import (
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Handle carries out r for a scheme session and reports whether it did. It owns no native resources: cancelling stops the core's callback claim, while a tab in the default browser remains the user's. open may answer on any goroutine, as GTK's URI launcher does.
func Handle(id uint32, r driver.AuthRequest, open func(string, func(bool)), dispatch func(driver.Event)) bool {
	if r.Scheme == "" {
		return false
	}
	switch r.Command {
	case driver.AuthStart:
		if strings.EqualFold(r.Scheme, "https") || strings.EqualFold(r.Scheme, "http") || r.Host != "" {
			dispatch(driver.AuthEnd(id, driver.AuthUnavailable, "the default browser cannot catch an HTTPS callback"))
		} else {
			dispatch(driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &driver.AuthEvent{
				Phase: driver.AuthRedirect, Redirect: driver.AuthRedirectURI(r), OpenLinks: true}})
		}
	case driver.AuthOpen:
		open(r.URL, func(opened bool) {
			if !opened {
				dispatch(driver.AuthEnd(id, driver.AuthFailed, "no browser opened the authorization URL"))
			}
		})
	}
	return true
}
