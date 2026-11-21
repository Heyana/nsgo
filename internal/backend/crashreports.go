//go:build !nsgoembed && (darwin || linux || windows)

package backend

import (
	"log"
	"runtime"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/crashreport"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

func startCrashReports() func() {
	app := core.App()
	_, _, logs := engine.AppDirs(app.ID)
	if app.Development || logs == "" {
		return func() {}
	}
	stop, err := crashreport.Capture(logs, app.Version, app.Build)
	if err != nil {
		log.Printf("nsgo: cannot capture fatal Go diagnostics: %v", err)
		return func() {}
	}
	stopNative := captureNativeCrashes(func(kind, trace string) {
		info := crashreport.Header{Platform: runtime.GOOS, Version: app.Version, Build: app.Build, Kind: kind}
		crashreport.Record(logs, info, trace)
	})
	return func() {
		stopNative()
		stop()
	}
}
