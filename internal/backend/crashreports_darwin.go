//go:build darwin && !nsgoembed

package backend

import "github.com/nsgo-dev/nsgo/internal/platform/apple"

func captureNativeCrashes(record func(kind, trace string)) func() {
	return apple.CaptureUncaughtExceptions(func(trace string) { record("objective-c", trace) })
}
