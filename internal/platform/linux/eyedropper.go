//go:build linux && !android

package linux

import (
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Screen color picking through the XDG Desktop Portal Screenshot interface
// (org.freedesktop.portal.Screenshot.PickColor, version >= 2). Availability is
// probed once off the GTK main thread on portalQueue so startup never blocks
// waiting for portal activation, and windows refresh their environment once
// the portal confirms support.

const (
	dbusPropertiesIface         = "org.freedesktop.DBus.Properties"
	screenshotPortalProbeMillis = 2000
)

var (
	gVariantGetDouble func(v uintptr) float64
	bindScreenColor   = sync.OnceFunc(func() {
		bindGlobalHotKeys()
		purego.RegisterLibFunc(&gVariantGetDouble, glibLib, "g_variant_get_double")
	})

	screenColorPortalReady atomic.Bool
	screenColorProbeOnce   sync.Once
)

type portalColorPick struct {
	id      uint32
	bus     uintptr
	request string
	sub     uint32
	done    bool
}

func (w *window) screenColorAvailable() bool {
	probeScreenColorPortal(w.appID)
	return screenColorPortalReady.Load()
}

func probeScreenColorPortal(appID string) {
	screenColorProbeOnce.Do(func() {
		bindScreenColor()
		portalQueue() <- func() {
			bus := portalBus(appID)
			if bus == 0 {
				return
			}
			ver, ok := queryScreenshotPortalVersion(bus)
			if !screenshotPortalSupportsColor(ver, ok) {
				return
			}
			screenColorPortalReady.Store(true)
			windowHandles.Range(func(_, value any) bool {
				if w, ok := value.(*window); ok {
					w.post(func() {
						if !w.closing {
							w.updateEnvironment()
						}
					})
				}
				return true
			})
		}
	})
}

func queryScreenshotPortalVersion(bus uintptr) (uint32, bool) {
	reply := portalCallTimeout(bus, globalShortcutPath, dbusPropertiesIface, "Get", screenshotPortalProbeMillis,
		gVariantNewString(cstr(screenshotPortalIface)), gVariantNewString(cstr("version")))
	if reply == 0 {
		return 0, false
	}
	defer gVariantUnref(reply)
	return portalPropertyUint32(reply)
}

func portalCallTimeout(bus uintptr, path, iface, method string, timeoutMillis int32, args ...uintptr) uintptr {
	ok := true
	for i, arg := range args {
		if arg == 0 {
			ok = false
			continue
		}
		args[i] = gVariantTakeRef(arg)
	}
	defer func() {
		for _, arg := range args {
			gVariantUnrefNonZero(arg)
		}
	}()
	if !ok {
		return 0
	}
	var params uintptr
	if len(args) > 0 {
		params = gVariantNewTuple(&args[0], uintptr(len(args)))
	} else {
		params = gVariantNewTuple(nil, 0)
	}
	var err uintptr
	reply := gDBusConnectionCallSync(bus, cstr(globalShortcutName), cstr(path), cstr(iface), cstr(method),
		params, 0, 0, timeoutMillis, 0, &err)
	if err != 0 {
		gErrorFree(err)
		return 0
	}
	return reply
}

func portalPropertyUint32(reply uintptr) (uint32, bool) {
	if reply == 0 || gVariantNChildren(reply) < 1 {
		return 0, false
	}
	v := gVariantGetChildValue(reply, 0)
	defer func() { gVariantUnrefNonZero(v) }()
	for v != 0 && goString(gVariantGetTypeString(v)) == "v" {
		inner := gVariantGetVariant(v)
		gVariantUnref(v)
		v = inner
	}
	if v == 0 || goString(gVariantGetTypeString(v)) != "u" {
		return 0, false
	}
	return gVariantGetUint32(v), true
}

// pickScreenColor invokes org.freedesktop.portal.Screenshot.PickColor and
// answers id when Request::Response arrives.
func (w *window) pickScreenColor(id uint32) {
	if w.closing || w.engine == nil {
		return
	}
	if w.colorPick != nil {
		w.cancelColorPick()
	}
	bindScreenColor()
	pick := &portalColorPick{id: id}
	w.colorPick = pick
	appID := w.appID
	w.withPortalParent(func(parent string) {
		portalQueue() <- func() {
			bus := portalBus(appID)
			if bus == 0 {
				w.post(func() { w.finishColorPick(pick, 0, false) })
				return
			}
			w.post(func() {
				if w.closing || w.colorPick != pick || pick.done {
					return
				}
				pick.bus = bus
				token := portalToken("nsgo_color")
				pick.request = portalRequestPathForSender(goString(gDBusConnectionGetUniqueName(bus)), token)
				if pick.request == "" {
					w.finishColorPick(pick, 0, false)
					return
				}
				pick.sub = subscribeColorPickResponse(bus, pick.request, w.handle)
				if pick.sub == 0 {
					w.finishColorPick(pick, 0, false)
					return
				}
				awaited := pick.request
				portalQueue() <- func() {
					reply := portalCall(bus, globalShortcutPath, screenshotPortalIface, "PickColor",
						gVariantNewString(cstr(parent)), portalOptions(token))
					request := ""
					if reply != 0 {
						request = portalTupleText(reply, 0)
						gVariantUnref(reply)
					}
					if request == awaited {
						return
					}
					w.post(func() {
						if w.closing || w.colorPick != pick || pick.done || pick.request != awaited {
							return
						}
						if pick.sub != 0 {
							gDBusConnectionSignalUnsubscribe(pick.bus, pick.sub)
							pick.sub = 0
						}
						if request == "" {
							w.finishColorPick(pick, 0, false)
							return
						}
						pick.request = request
						pick.sub = subscribeColorPickResponse(pick.bus, pick.request, w.handle)
						if pick.sub == 0 {
							w.finishColorPick(pick, 0, false)
						}
					})
				}
			})
		}
	})
}

func subscribeColorPickResponse(bus uintptr, request string, handle uintptr) uint32 {
	return gDBusConnectionSignalSubscribe(bus, cstr(globalShortcutName), cstr(globalRequestIface), cstr("Response"),
		cstr(request), nil, 0, cbColorPickResponse, handle, 0)
}

func colorPickResponse(conn, sender, path, iface, signal, params, data uintptr) {
	w := windowOf(data)
	if w == nil || w.colorPick == nil || w.colorPick.done || goString(path) != w.colorPick.request {
		return
	}
	pick := w.colorPick
	c, ok := parsePortalColorResponse(params)
	w.finishColorPick(pick, c, ok)
}

func parsePortalColorResponse(params uintptr) (driver.Color, bool) {
	if params == 0 || gVariantNChildren(params) < 2 {
		return 0, false
	}
	code := portalTupleUint32(params, 0)
	if !portalColorResponseOK(code) {
		return 0, false
	}
	results := gVariantGetChildValue(params, 1)
	defer gVariantUnref(results)
	val := gVariantLookupValue(results, cstr("color"), 0)
	if val == 0 {
		return 0, false
	}
	defer func() { gVariantUnrefNonZero(val) }()
	for val != 0 && goString(gVariantGetTypeString(val)) == "v" {
		inner := gVariantGetVariant(val)
		gVariantUnref(val)
		val = inner
	}
	if val == 0 || goString(gVariantGetTypeString(val)) != "(ddd)" || gVariantNChildren(val) != 3 {
		return 0, false
	}
	part := func(i uintptr) float64 {
		c := gVariantGetChildValue(val, i)
		defer gVariantUnref(c)
		return gVariantGetDouble(c)
	}
	return portalColorFromRGB(code, part(0), part(1), part(2))
}

func (w *window) finishColorPick(pick *portalColorPick, c driver.Color, ok bool) {
	if pick == nil || pick.done {
		return
	}
	pick.done = true
	if w.colorPick == pick {
		w.colorPick = nil
	}
	if pick.sub != 0 {
		gDBusConnectionSignalUnsubscribe(pick.bus, pick.sub)
		pick.sub = 0
	}
	if w.engine != nil && !w.closing {
		w.engine.Dispatch(portalColorResultEvent(pick.id, c, ok))
	}
}

func (w *window) cancelColorPick() {
	pick := w.colorPick
	if pick == nil {
		return
	}
	bus, path := pick.bus, pick.request
	w.finishColorPick(pick, 0, false)
	if bus != 0 && path != "" {
		portalQueue() <- func() { gVariantUnrefNonZero(portalCall(bus, path, globalRequestIface, "Close")) }
	}
}
