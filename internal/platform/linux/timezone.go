//go:build linux && !android

package linux

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ebitengine/purego"
)

// Go reads the local zone from /etc/localtime once on Linux, at the first use of time.Local. acquireTimeZoneWatcher watches /etc for changes to localtime and timezone via inotify and subscribes to org.freedesktop.timedate1's Timezone property on the system bus, updating time.Local whenever the system zone changes while the process follows /etc/localtime (TZ unset or pointing at /etc/localtime). Times read after a change are in the new zone; the app is not rendered again for it.

var linuxTimeZone = linuxZoneTracker{
	localtimePath: defaultLocaltimePath,
	timezonePath:  defaultTimezonePath,
	lookupEnv:     os.LookupEnv,
	loadLocation:  time.LoadLocation,
	apply: func(loc *time.Location) {
		// Go has no other way to change the zone: the store races with readers of time.Local, but it is one pointer, which none sees torn.
		time.Local = loc
	},
	nowYear: func() int { return time.Now().UTC().Year() },
}

var (
	tzWatchMu     sync.Mutex
	tzWatchRefs   int
	tzWatchGen    uintptr
	tzStopInotify func()
	tzDBusCancel  uintptr
	tzDBusProxy   uintptr
	tzDBusHandler uint64

	tzDBusProxyNewForBus         func(bus int32, flags uint32, info uintptr, name, path, iface *byte, cancel, callback, data uintptr)
	tzDBusProxyNewForBusFinish   func(result uintptr, err *uintptr) uintptr
	tzDBusProxyGetCachedProperty func(proxy uintptr, name *byte) uintptr
	tzCancellableNew             func() uintptr
	tzCancellableCancel          func(uintptr)
	tzSignalHandlerDisconnect    func(uintptr, uint64)

	bindTimeZoneDBus = sync.OnceFunc(func() {
		bindInputSources()
		purego.RegisterLibFunc(&tzDBusProxyNewForBus, gioLib, "g_dbus_proxy_new_for_bus")
		purego.RegisterLibFunc(&tzDBusProxyNewForBusFinish, gioLib, "g_dbus_proxy_new_for_bus_finish")
		purego.RegisterLibFunc(&tzDBusProxyGetCachedProperty, gioLib, "g_dbus_proxy_get_cached_property")
		purego.RegisterLibFunc(&tzCancellableNew, gioLib, "g_cancellable_new")
		purego.RegisterLibFunc(&tzCancellableCancel, gioLib, "g_cancellable_cancel")
		purego.RegisterLibFunc(&tzSignalHandlerDisconnect, gobjectLib, "g_signal_handler_disconnect")
	})

	cbTimeDateReady   = sync.OnceValue(func() uintptr { return purego.NewCallback(timeDateReady) })
	cbTimeDateChanged = sync.OnceValue(func() uintptr { return purego.NewCallback(timeDateChanged) })
)

func init() {
	linuxTimeZone.seed()
}

// acquireTimeZoneWatcher starts watching system time zone changes when the first window or embedded surface opens.
func acquireTimeZoneWatcher() {
	tzWatchMu.Lock()
	tzWatchRefs++
	if tzWatchRefs > 1 {
		tzWatchMu.Unlock()
		return
	}
	tzWatchGen++
	gen := tzWatchGen
	tzWatchMu.Unlock()

	linuxTimeZone.refresh()

	stop, err := startInotifyTimeZoneWatcher(
		defaultLocaltimePath,
		[]string{filepath.Base(defaultLocaltimePath), filepath.Base(defaultTimezonePath)},
		func() { linuxTimeZone.refresh() },
	)

	var cancel uintptr
	if gioLib != 0 && gobjectLib != 0 {
		bindTimeZoneDBus()
		cancel = tzCancellableNew()
		const systemBus = 1
		const (
			doNotAutoStart           = 1 << 2
			getInvalidatedProperties = 1 << 3
		)
		tzDBusProxyNewForBus(
			systemBus,
			doNotAutoStart|getInvalidatedProperties,
			0,
			cstr("org.freedesktop.timedate1"),
			cstr("/org/freedesktop/timedate1"),
			cstr("org.freedesktop.timedate1"),
			cancel,
			cbTimeDateReady(),
			gen,
		)
	}

	tzWatchMu.Lock()
	if tzWatchGen == gen && tzWatchRefs > 0 {
		if err == nil {
			tzStopInotify = stop
		}
		tzDBusCancel = cancel
		tzWatchMu.Unlock()
		return
	}
	tzWatchMu.Unlock()

	if cancel != 0 {
		tzCancellableCancel(cancel)
		gObjectUnref(cancel)
	}
	if err == nil && stop != nil {
		stop()
	}
}

// releaseTimeZoneWatcher tears down the inotify and D-Bus watchers when the last window or embedded surface closes.
func releaseTimeZoneWatcher() {
	tzWatchMu.Lock()
	if tzWatchRefs == 0 {
		tzWatchMu.Unlock()
		return
	}
	tzWatchRefs--
	if tzWatchRefs > 0 {
		tzWatchMu.Unlock()
		return
	}
	tzWatchGen++
	stop := tzStopInotify
	cancel := tzDBusCancel
	proxy := tzDBusProxy
	handler := tzDBusHandler
	tzStopInotify = nil
	tzDBusCancel = 0
	tzDBusProxy = 0
	tzDBusHandler = 0
	tzWatchMu.Unlock()

	if cancel != 0 {
		tzCancellableCancel(cancel)
		gObjectUnref(cancel)
	}
	if proxy != 0 {
		if handler != 0 {
			tzSignalHandlerDisconnect(proxy, handler)
		}
		gObjectUnref(proxy)
	}
	if stop != nil {
		stop()
	}
}

func timeDateReady(source, result, gen uintptr) {
	var err uintptr
	proxy := tzDBusProxyNewForBusFinish(result, &err)
	if err != 0 {
		gErrorFree(err)
	}
	if proxy == 0 {
		return
	}
	tzWatchMu.Lock()
	if gen != tzWatchGen || tzWatchRefs == 0 {
		tzWatchMu.Unlock()
		gObjectUnref(proxy)
		return
	}
	tzDBusProxy = proxy
	tzDBusHandler = gSignalConnect(proxy, "g-properties-changed", cbTimeDateChanged(), 0)
	tzWatchMu.Unlock()

	if name := timedateProxyZone(proxy); name != "" {
		linuxTimeZone.applyNamed(name)
	}
}

func timeDateChanged(proxy, changed, invalidated, data uintptr) {
	if name := timedateProxyZone(proxy); name != "" {
		linuxTimeZone.applyNamed(name)
		return
	}
	linuxTimeZone.refresh()
}

func timedateProxyZone(proxy uintptr) string {
	p := tzDBusProxyGetCachedProperty(proxy, cstr("Timezone"))
	if p == 0 {
		return ""
	}
	defer gVariantUnref(p)
	if goString(gVariantGetTypeString(p)) != "s" {
		return ""
	}
	return goString(gVariantGetString(p, 0))
}

// startInotifyTimeZoneWatcher watches the directory containing targetPath via Linux inotify (pure syscall) and calls onChange whenever any entry matching watchNames is created, replaced, modified or moved into place. Calling the returned stop function closes the inotify descriptor and waits for the watcher goroutine to exit.
func startInotifyTimeZoneWatcher(targetPath string, watchNames []string, onChange func()) (func(), error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	const mask = syscall.IN_MOVED_TO |
		syscall.IN_CREATE |
		syscall.IN_CLOSE_WRITE |
		syscall.IN_MODIFY |
		syscall.IN_ATTRIB |
		syscall.IN_DELETE
	dir := filepath.Dir(targetPath)
	if _, err := syscall.InotifyAddWatch(fd, dir, mask); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "inotify-timezone")
	done := make(chan struct{})
	names := append([]string(nil), watchNames...)
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			if n > 0 && parseInotifyEvents(buf[:n], names...) {
				onChange()
			}
			if err != nil {
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = f.Close()
			<-done
		})
	}, nil
}
