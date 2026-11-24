//go:build ios

package apple

import "github.com/ebitengine/purego/objc"

// LocalNetworkGated says the system keeps the local network from the app, as iOS and tvOS do from 14, below every minimum NSGo builds for.
func LocalNetworkGated() bool { return true }

// appActive says the app is active, which it is not while a system prompt shows over it. Main thread.
func appActive() bool {
	const uiApplicationStateActive = 0
	return objc.Send[int](objc.ID(Class("UIApplication")).Send(sel("sharedApplication")), sel("applicationState")) == uiApplicationStateActive
}
