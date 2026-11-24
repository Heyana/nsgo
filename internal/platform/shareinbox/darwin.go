//go:build darwin

package shareinbox

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

const GroupKey = "NSGoShareAppGroup"
const TypesKey = "NSGoShareTypes"
const NotificationKey = "NSGoShareNotification"

var native = sync.OnceFunc(func() {
	lib, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	if _, err = purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&darwinCenter, lib, "CFNotificationCenterGetDarwinNotifyCenter")
	purego.RegisterLibFunc(&addObserver, lib, "CFNotificationCenterAddObserver")
	purego.RegisterLibFunc(&removeObserver, lib, "CFNotificationCenterRemoveObserver")
	purego.RegisterLibFunc(&postNotification, lib, "CFNotificationCenterPostNotification")
})

var darwinCenter func() uintptr
var addObserver func(uintptr, uintptr, uintptr, objc.ID, uintptr, int)
var removeObserver func(uintptr, uintptr, objc.ID, uintptr)
var postNotification func(uintptr, objc.ID, uintptr, uintptr, bool)
var observers sync.Map
var nextObserver atomic.Uint64
var notificationCallback = sync.OnceValue(func() uintptr {
	return purego.NewCallback(func(_, observer uintptr, _ objc.ID, _, _ uintptr) {
		if fn, ok := observers.Load(observer); ok {
			fn.(func())()
		}
	})
})

func String(value string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), value)
}
func GoString(value objc.ID) string {
	if value == 0 {
		return ""
	}
	p := objc.Send[*byte](value, objc.RegisterName("UTF8String"))
	if p == nil {
		return ""
	}
	n := objc.Send[uint](value, objc.RegisterName("lengthOfBytesUsingEncoding:"), uint(4))
	return string(unsafe.Slice(p, n))
}
func Info(key string) objc.ID {
	native()
	return objc.ID(objc.GetClass("NSBundle")).Send(objc.RegisterName("mainBundle")).Send(objc.RegisterName("objectForInfoDictionaryKey:"), String(key))
}
func GroupPath() string {
	group := Info(GroupKey)
	if group == 0 {
		return ""
	}
	manager := objc.ID(objc.GetClass("NSFileManager")).Send(objc.RegisterName("defaultManager"))
	url := manager.Send(objc.RegisterName("containerURLForSecurityApplicationGroupIdentifier:"), group)
	return GoString(url.Send(objc.RegisterName("path")))
}
func Notify() {
	native()
	if name := Info(NotificationKey); name != 0 {
		postNotification(darwinCenter(), name, 0, 0, true)
	}
}

// Observe receives a payload-free Darwin notification. The inbox remains the source of truth; a notification can be coalesced or arrive after activation.
func Observe(fn func()) func() {
	native()
	name := Info(NotificationKey)
	if name == 0 {
		return func() {}
	}
	key := uintptr(nextObserver.Add(1))
	observers.Store(key, fn)
	addObserver(darwinCenter(), key, notificationCallback(), name, 0, 4)
	// The registration retains the name; recreate it for removal rather than retaining an autoreleased Foundation object across activations.
	return func() { removeObserver(darwinCenter(), key, Info(NotificationKey), 0); observers.Delete(key) }
}
