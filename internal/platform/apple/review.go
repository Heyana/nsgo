//go:build darwin

package apple

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var storeReview = sync.OnceValues(func() (objc.ID, objc.SEL) {
	const framework = "/System/Library/Frameworks/StoreKit.framework/StoreKit"
	if _, err := purego.Dlopen(framework, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return 0, 0
	}
	controller := objc.ID(Class("SKStoreReviewController"))
	if runtime.GOOS == "ios" {
		if command := sel("requestReviewInScene:"); objc.Send[bool](controller, sel("respondsToSelector:"), command) {
			return controller, command
		}
	}
	if command := sel("requestReview"); objc.Send[bool](controller, sel("respondsToSelector:"), command) {
		return controller, command
	}
	return 0, 0
})

// ReviewAvailable reports whether this system has a StoreKit rating prompt.
func ReviewAvailable() bool {
	controller, _ := storeReview()
	return controller != 0
}

// RequestReview asks StoreKit to show a rating prompt on the main thread.
// Pass the calling surface's foreground-active scene on iOS and zero on macOS.
func RequestReview(scene objc.ID) {
	controller, command := storeReview()
	if controller == 0 {
		return
	}
	if command == sel("requestReviewInScene:") {
		if scene != 0 {
			controller.Send(command, scene)
		}
		return
	}
	controller.Send(command)
}
