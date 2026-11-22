//go:build darwin

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Only this process's entries are queried; no other app's logs are read.
func verify() string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	if _, err := purego.Dlopen("/System/Library/Frameworks/OSLog.framework/OSLog", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return err.Error()
	}
	var stat syscall.Stat_t
	syscall.Fstat(int(os.Stderr.Fd()), &stat)
	var null syscall.Stat_t
	syscall.Stat(os.DevNull, &null)
	result := fmt.Sprintf("stderr /dev/null: %v\n", stat.Mode&syscall.S_IFMT == syscall.S_IFCHR && stat.Rdev == null.Rdev)
	for attempt := 0; attempt < 12; attempt++ {
		var failure objc.ID
		store := objc.ID(objc.GetClass("OSLogStore")).Send(objc.RegisterName("storeWithScope:error:"), int(1), &failure)
		if store == 0 {
			return result + "store: " + apple.GoString(failure.Send(objc.RegisterName("localizedDescription")))
		}
		entries := store.Send(objc.RegisterName("entriesEnumeratorWithOptions:position:predicate:error:"), uint(0), objc.ID(0), objc.ID(0), &failure)
		if entries == 0 {
			return result + "enumerate: " + apple.GoString(failure.Send(objc.RegisterName("localizedDescription")))
		}
		var matches []string
		for entry := entries.Send(objc.RegisterName("nextObject")); entry != 0; entry = entries.Send(objc.RegisterName("nextObject")) {
			if !objc.Send[bool](entry, objc.RegisterName("isKindOfClass:"), objc.GetClass("OSLogEntryLog")) {
				continue
			}
			if apple.GoString(entry.Send(objc.RegisterName("subsystem"))) != "dev.nsgo.test.applelogs1348" || apple.GoString(entry.Send(objc.RegisterName("category"))) != "nsgo" {
				continue
			}
			text := apple.GoString(entry.Send(objc.RegisterName("composedMessage")))
			if strings.Contains(text, "applelogs1348:") || strings.Contains(text, "--nsgo-lang") {
				matches = append(matches, text)
			}
		}
		joined := strings.Join(matches, "\n")
		if strings.Contains(joined, "WARN applelogs1348: 中文 100% %@ %s") && strings.Contains(joined, "ERROR applelogs1348: before\\0after") {
			return result + "PASS\n" + joined
		}
		time.Sleep(time.Second)
	}
	return result + "FAIL: stored fixture messages missing"
}
