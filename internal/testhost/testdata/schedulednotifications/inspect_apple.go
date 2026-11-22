//go:build darwin

package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// The center returns only this fixture's requests, never another app's.
func inspect() string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	apple.WatchNotifications()
	center := objc.ID(apple.Class("UNUserNotificationCenter")).Send(objc.RegisterName("currentNotificationCenter"))
	var lines []string
	for _, kind := range []string{"pending", "delivered"} {
		result := make(chan []string, 1)
		callback := objcblock.New(func(_ objc.Block, items objc.ID) {
			var entries []string
			for i := uint(0); i < objc.Send[uint](items, objc.RegisterName("count")); i++ {
				item := items.Send(objc.RegisterName("objectAtIndex:"), i)
				request := item
				if kind == "delivered" {
					request = item.Send(objc.RegisterName("request"))
				}
				id := apple.GoString(request.Send(objc.RegisterName("identifier")))
				title := apple.GoString(request.Send(objc.RegisterName("content")).Send(objc.RegisterName("title")))
				entries = append(entries, fmt.Sprintf("%s %s %s", kind, id, title))
			}
			result <- entries
		})
		method := "getPendingNotificationRequestsWithCompletionHandler:"
		if kind == "delivered" {
			method = "getDeliveredNotificationsWithCompletionHandler:"
		}
		center.Send(objc.RegisterName(method), callback)
		callback.Release()
		select {
		case entries := <-result:
			lines = append(lines, entries...)
		case <-time.After(10 * time.Second):
			return "FAIL: query timed out"
		}
	}
	return strings.Join(lines, "\n")
}
