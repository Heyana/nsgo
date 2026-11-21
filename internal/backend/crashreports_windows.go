//go:build windows && !nsgoembed

package backend

import "github.com/nsgo-dev/nsgo/internal/platform/windows"

func captureNativeCrashes(record func(kind, trace string)) func() {
	return windows.CaptureUnhandledExceptions(func(trace string) { record("seh", trace) })
}
