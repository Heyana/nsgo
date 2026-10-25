//go:build darwin

package apple

import (
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	dispatchAsyncF   func(queue, context, work uintptr)
	poolPush         func() uintptr
	poolPop          func(uintptr)
	mainQueue        uintptr
	mainWork         uintptr
	msgSend          uintptr
	setQOS           func(class uint32, priority int32) int32
	mainMu           sync.Mutex
	mainFuncs        []func()
	selAddObserver   = sel("addObserverForName:object:queue:usingBlock:")
	selMainQueue     = sel("mainQueue")
	selCount         = sel("count")
	selObjectAtIndex = sel("objectAtIndex:")
)

func loadSystem() {
	lib := load("/usr/lib/libSystem.B.dylib")
	purego.RegisterLibFunc(&dispatchAsyncF, lib, "dispatch_async_f")
	mainQueue = symbol(lib, "_dispatch_main_q")
	mainWork = purego.NewCallback(func(uintptr) { runMain() })
	runtimeLib := load("/usr/lib/libobjc.A.dylib")
	purego.RegisterLibFunc(&poolPush, runtimeLib, "objc_autoreleasePoolPush")
	purego.RegisterLibFunc(&poolPop, runtimeLib, "objc_autoreleasePoolPop")
	msgSend = symbol(runtimeLib, "objc_msgSend")
	purego.RegisterLibFunc(&setQOS, lib, "pthread_set_qos_class_self_np")
}

// qosUserInteractive is QOS_CLASS_USER_INTERACTIVE, the main thread's.
const qosUserInteractive = 0x21

// RaiseThread gives the calling OS thread the main thread's quality of
// service, as NSH gives its engine thread. The engine's thread turns input
// into frames; at the default class the system may run it on efficiency
// cores, and the quotes benchmark's commits took about 15% longer and
// varied with the machine's load.
func RaiseThread() { setQOS(qosUserInteractive, 0) }

// Msg sends sel to id with up to six arguments and a result that are
// objects, selectors, pointers, integers or booleans, through
// purego.SyscallN. objc.Send and ID.Send work each call out with
// reflection, which costs several times the message itself; what runs for
// every changed view of every frame sends with Msg.
func Msg(id objc.ID, sel objc.SEL, args ...uintptr) uintptr {
	var all [8]uintptr
	if len(args) > len(all)-2 {
		panic("apple: too many arguments to Msg")
	}
	all[0], all[1] = uintptr(id), uintptr(sel)
	n := 2 + copy(all[2:], args)
	r, _, _ := purego.SyscallN(msgSend, all[:n]...)
	return r
}

// MsgBool is Msg for a BOOL result.
func MsgBool(id objc.ID, sel objc.SEL, args ...uintptr) bool {
	return byte(Msg(id, sel, args...)) != 0
}

// OnMain runs fn on the main thread after what is queued there already.
// It never waits, so any goroutine may call it.
func OnMain(fn func()) {
	mainMu.Lock()
	mainFuncs = append(mainFuncs, fn)
	first := len(mainFuncs) == 1
	mainMu.Unlock()
	if first {
		dispatchAsyncF(mainQueue, 0, mainWork)
	}
}

func runMain() {
	mainMu.Lock()
	fns := mainFuncs
	mainFuncs = nil
	mainMu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// withPool runs fn on one OS thread inside an autorelease pool, for
// Objective-C calls off the main thread.
func withPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := poolPush()
	defer poolPop(pool)
	fn()
}

// WithoutAnimation runs fn with Core Animation's implicit animations off,
// so layers a backend adds itself change at once.
func WithoutAnimation(fn func()) {
	tx := objc.ID(objc.GetClass("CATransaction"))
	tx.Send(sel("begin"))
	tx.Send(sel("setDisableActions:"), true)
	fn()
	tx.Send(sel("commit"))
}

// Observe calls fn on the main thread each time center posts name, until
// stop is called.
func Observe(center, name objc.ID, fn func(note objc.ID)) (stop func()) {
	if center == 0 || name == 0 {
		return func() {}
	}
	block := objc.NewBlock(func(_ objc.Block, note objc.ID) { fn(note) })
	queue := objc.ID(objc.GetClass("NSOperationQueue")).Send(selMainQueue)
	token := center.Send(selAddObserver, name, objc.ID(0), queue, block).Send(sel("retain"))
	block.Release()
	return func() {
		if token != 0 {
			center.Send(sel("removeObserver:"), token)
			token.Send(sel("release"))
			token = 0
		}
	}
}

// Global reads the global object, such as an NSString constant, that
// framework at path exports as name; 0 when it has none.
func Global(path, name string) objc.ID {
	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0
	}
	addr, err := purego.Dlsym(lib, name)
	if err != nil || addr == 0 {
		return 0
	}
	return **(**objc.ID)(unsafe.Pointer(&addr))
}

// CommonModes returns NSRunLoopCommonModes. Run loop sources compare
// modes by identity, so an equal string of another object is a mode of
// its own that never runs.
func CommonModes() objc.ID {
	commonModesOnce.Do(func() {
		commonModes = Global("/System/Library/Frameworks/Foundation.framework/Foundation", "NSRunLoopCommonModes")
	})
	return commonModes
}

var (
	commonModesOnce sync.Once
	commonModes     objc.ID
)

// Locale returns the user's locale as a BCP 47 tag, and whether their
// first preferred language is written right to left.
func Locale() (tag string, rtl bool) {
	nsLocale := objc.ID(objc.GetClass("NSLocale"))
	id := GoString(nsLocale.Send(sel("currentLocale")).Send(sel("localeIdentifier")))
	if i := strings.IndexByte(id, '@'); i >= 0 {
		id = id[:i]
	}
	tag = strings.ReplaceAll(id, "_", "-")
	langs := nsLocale.Send(sel("preferredLanguages"))
	if objc.Send[uint](langs, selCount) > 0 {
		const rightToLeft = 2
		first := langs.Send(selObjectAtIndex, uint(0))
		rtl = objc.Send[int](nsLocale, sel("characterDirectionForLanguage:"), first) == rightToLeft
	}
	return tag, rtl
}

// ForEach calls fn with each object of an NSArray or NSSet.
func ForEach(collection objc.ID, fn func(objc.ID)) {
	if objc.Send[bool](collection, sel("isKindOfClass:"), objc.GetClass("NSSet")) {
		collection = collection.Send(sel("allObjects"))
	}
	n := objc.Send[uint](collection, selCount)
	for i := range n {
		fn(collection.Send(selObjectAtIndex, i))
	}
}

// KeyNames returns the names core.NormalizeKey gives a key press: the key
// named name with the modifier flags AppKit and UIKit share, first with
// Command as "mod", then as "meta".
func KeyNames(name string, flags uint) []string {
	const shift, control, option, command = 1 << 17, 1 << 18, 1 << 19, 1 << 20
	var mods []string
	for _, m := range []struct {
		flag uint
		name string
	}{{control, "ctrl"}, {option, "alt"}, {shift, "shift"}} {
		if flags&m.flag != 0 {
			mods = append(mods, m.name)
		}
	}
	if flags&command == 0 {
		return []string{strings.Join(append(mods, name), "+")}
	}
	return []string{
		strings.Join(append(append([]string{"mod"}, mods...), name), "+"),
		strings.Join(append(append([]string{}, mods...), "meta", name), "+"),
	}
}
