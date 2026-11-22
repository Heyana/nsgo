package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

// This fixture is installed under its own unsigned test identity. It exercises
// the public API only in a real MSIX, so an unpackaged run never writes a Run key.
func main() {
	var size uint32
	status, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentPackageFullName").Call(uintptr(unsafe.Pointer(&size)), 0)
	if status != 122 { // ERROR_INSUFFICIENT_BUFFER means this process has package identity.
		fmt.Fprintln(os.Stderr, "startup fixture must run from its MSIX package")
		os.Exit(2)
	}
	nsgo.Run(App, nsgo.StartHidden())
}

func App(cx *core.Scope) {
	desktop.SetShutdownMode(cx, desktop.ShutdownOnExplicit)
	var lines []string
	finish := func() {
		path := filepath.Join(cx.CacheDir(), "startup-proof.txt")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err == nil {
			_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600)
		}
		cx.Quit()
	}
	timer := time.AfterFunc(20*time.Second, func() { cx.Post(func() { lines = append(lines, "timeout=true"); finish() }) })
	cx.OnQuit(func(context.Context) { timer.Stop() })
	lines = append(lines, fmt.Sprintf("launched-at-login=%v", desktop.LaunchedAtLogin(cx)))
	desktop.LaunchAtLogin(cx, func(initial bool) {
		lines = append(lines, fmt.Sprintf("initial=%v", initial))
		desktop.SetLaunchAtLogin(cx, true)
		desktop.LaunchAtLogin(cx, func(enabled bool) {
			lines = append(lines, fmt.Sprintf("enabled=%v", enabled))
			desktop.SetLaunchAtLogin(cx, false)
			desktop.LaunchAtLogin(cx, func(disabled bool) {
				lines = append(lines, fmt.Sprintf("disabled=%v", disabled))
				finish()
			})
		})
	})
}
