//go:build nsgoembed || (!darwin && !linux && !windows)

package backend

func startCrashReports() func() { return func() {} }
