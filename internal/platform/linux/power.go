//go:build linux && !android

package linux

import (
	"sync"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	gDBusProxyNewForBus         func(bus int32, flags uint32, info uintptr, name, path, iface *byte, cancel, callback, data uintptr)
	gDBusProxyNewForBusFinish   func(result uintptr, err *uintptr) uintptr
	gDBusProxyGetCachedProperty func(proxy uintptr, name *byte) uintptr
	powerProfiles               [2]struct {
		proxy uintptr
		mode  driver.LowPowerMode
	}
)

// GDBusProxy follows both names used by power-profiles-daemon on the system bus. It caches ActiveProfile and drops it if the service goes away. Creation and notification callbacks run on GTK's thread.
var followPowerProfiles = sync.OnceFunc(func() {
	bindInputSources()
	purego.RegisterLibFunc(&gDBusProxyNewForBus, gioLib, "g_dbus_proxy_new_for_bus")
	purego.RegisterLibFunc(&gDBusProxyNewForBusFinish, gioLib, "g_dbus_proxy_new_for_bus_finish")
	purego.RegisterLibFunc(&gDBusProxyGetCachedProperty, gioLib, "g_dbus_proxy_get_cached_property")
	const systemBus = 1
	const (
		doNotAutoStart           = 1 << 2
		getInvalidatedProperties = 1 << 3
	)
	for i, endpoint := range [...]struct{ name, path string }{
		{"org.freedesktop.UPower.PowerProfiles", "/org/freedesktop/UPower/PowerProfiles"},
		{"net.hadess.PowerProfiles", "/net/hadess/PowerProfiles"},
	} {
		gDBusProxyNewForBus(systemBus, doNotAutoStart|getInvalidatedProperties, 0, cstr(endpoint.name), cstr(endpoint.path), cstr(endpoint.name), 0,
			cbPowerProfilesReady, uintptr(i))
	}
})

var cbPowerProfilesReady = purego.NewCallback(func(source, result, index uintptr) {
	var err uintptr
	proxy := gDBusProxyNewForBusFinish(result, &err)
	if err != 0 {
		gErrorFree(err)
	}
	if proxy == 0 {
		return
	}
	powerProfiles[index].proxy = proxy
	gSignalConnect(proxy, "g-properties-changed", cbPowerProfileChanged, index)
	gSignalConnect(proxy, "notify::g-name-owner", cbPowerProfileOwner, index)
	updatePowerProfile(index)
})

var cbPowerProfileChanged = purego.NewCallback(func(proxy, changed, invalidated, index uintptr) {
	updatePowerProfile(index)
})

var cbPowerProfileOwner = purego.NewCallback(func(proxy, pspec, index uintptr) {
	updatePowerProfile(index)
})

func lowPowerMode() driver.LowPowerMode {
	for _, profile := range powerProfiles {
		if profile.mode != driver.LowPowerUnknown {
			return profile.mode
		}
	}
	return driver.LowPowerUnknown
}

func updatePowerProfile(index uintptr) {
	before := lowPowerMode()
	profile := &powerProfiles[index]
	profile.mode = driver.LowPowerUnknown
	if p := gDBusProxyGetCachedProperty(profile.proxy, cstr("ActiveProfile")); p != 0 {
		if goString(gVariantGetTypeString(p)) == "s" {
			switch goString(gVariantGetString(p, 0)) {
			case "power-saver":
				profile.mode = driver.LowPowerOn
			case "balanced", "performance":
				profile.mode = driver.LowPowerOff
			}
		}
		gVariantUnref(p)
	}
	if before != lowPowerMode() {
		windowHandles.Range(func(_, v any) bool {
			if w := v.(*window); !w.closing {
				w.updateEnvironment()
			}
			return true
		})
	}
}
