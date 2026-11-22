//go:build linux && !nsgoembed && !nsgotui

package backend

func captureNativeCrashes(func(kind, trace string)) func() { return func() {} }
