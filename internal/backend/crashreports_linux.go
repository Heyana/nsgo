//go:build linux && !nsgoembed

package backend

func captureNativeCrashes(func(kind, trace string)) func() { return func() {} }
