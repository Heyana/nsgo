//go:build ios

package ios

import "github.com/ebitengine/purego/objc"

// testInputFilter is installed only by the native test host (testhost.go), on
// the main thread. Ordinary applications and live automation (nsgoautomation)
// leave it nil so real user input is never blocked.
var testInputFilter func(window, event objc.ID) bool

// iosInputAllowed keeps non-input UIKit events and nil events while a test
// accepts only its scripted touch, pointer, wheel, key and gesture events.
func iosInputAllowed(window, event objc.ID) bool {
	if testInputFilter == nil || event == 0 {
		return true
	}
	if !isPhysicalUIEventType(objc.Send[int](event, sel("type"))) {
		return true
	}
	return testInputFilter(window, event)
}
