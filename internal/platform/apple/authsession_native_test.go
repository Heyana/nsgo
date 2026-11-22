//go:build darwin

package apple

import (
	"runtime"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The system's callback matcher can be tested without a sign-in sheet, a real
// provider, an associated-domain installation or any account credentials.
func TestHTTPSAuthCallbackMatchesThePreparedPath(t *testing.T) {
	if !loadAuthenticationServices() || Class("ASWebAuthenticationSessionCallback") == 0 {
		t.Skip("requires macOS 14.4 or iOS 17.4")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(Class("NSAutoreleasePool")).Send(sel("new"))
	defer pool.Send(sel("drain"))
	for _, path := range []string{"/oauth", "/before/../戻る/%2F"} {
		request := driver.AuthRequest{Scheme: "https", Host: "auth.example.com", Path: path}
		callback := httpsAuthCallback(request)
		uri := objc.ID(Class("NSURL")).Send(sel("URLWithString:"), NSString(driver.AuthRedirectURI(request)))
		if callback == 0 || uri == 0 || !objc.Send[bool](callback, sel("matchesURL:"), uri) {
			t.Errorf("the native HTTPS callback did not match the prepared path %q", path)
		}
		other := objc.ID(Class("NSURL")).Send(sel("URLWithString:"), NSString("https://auth.example.com/elsewhere"))
		if objc.Send[bool](callback, sel("matchesURL:"), other) {
			t.Error("the native HTTPS callback matched another path")
		}
	}
}
