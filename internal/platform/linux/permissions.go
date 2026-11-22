//go:build linux && !android

package linux

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Sandbox permissions on Linux. Outside Flatpak and Snap every permission is
// not gated. Inside Flatpak or Snap, camera access is checked and requested
// through org.freedesktop.impl.portal.PermissionStore and
// org.freedesktop.portal.Camera (OpenPipeWireRemote / AccessCamera), and
// microphone access through PermissionStore, org.freedesktop.portal.Device
// (AccessDevice), and PipeWire's native registry visibility (Audio/Source).

const permissionCallTimeoutMillis = 2000

var (
	permVariantGetBool func(v uintptr) int32

	bindPermissions = sync.OnceFunc(func() {
		bindShare()
		purego.RegisterLibFunc(&permVariantGetBool, glibLib, "g_variant_get_boolean")
	})

	cbPermissionPortalResponse = sync.OnceValue(func() uintptr {
		return purego.NewCallback(permissionPortalResponse)
	})
)

type linuxPortalPermissionRequest struct {
	key     uintptr
	w       *window
	id      uint32
	name    string
	ctrl    *linuxPermissionController
	bus     uintptr
	request string
	sub     uint32
}

var (
	linuxPermSeq     atomic.Uintptr
	linuxPermReqMu   sync.Mutex
	linuxPermReqs    = map[uintptr]*linuxPortalPermissionRequest{}
	linuxPermCtrlMu  sync.Mutex
	linuxPermCtrl    *linuxPermissionController
	linuxPermSandbox linuxSandbox
)

// currentLinuxSandbox inspects /.flatpak-info and Snap environment variables
// to identify the running sandbox and portal app ID.
func currentLinuxSandbox(fallbackAppID string) linuxSandbox {
	data, err := os.ReadFile("/.flatpak-info")
	hasFlatpak := err == nil
	if !hasFlatpak {
		if _, statErr := os.Stat("/.flatpak-info"); statErr == nil {
			hasFlatpak = true
		}
	}
	return detectLinuxSandbox(data, hasFlatpak, os.Getenv("SNAP"), os.Getenv("SNAP_NAME"), fallbackAppID)
}

func getLinuxPermissionController(appID string) *linuxPermissionController {
	sb := currentLinuxSandbox(appID)
	linuxPermCtrlMu.Lock()
	defer linuxPermCtrlMu.Unlock()
	if linuxPermCtrl == nil || linuxPermSandbox.kind != sb.kind || linuxPermSandbox.appID != sb.appID {
		linuxPermSandbox = sb
		linuxPermCtrl = newLinuxPermissionController(sb, &linuxRealPermissionBackend{appID: appID})
	}
	return linuxPermCtrl
}

// permission carries out an ItemPermission numbered id on Linux.
func (w *window) permission(id uint32, r driver.PermissionRequest) {
	if w.closing || w.engine == nil {
		return
	}
	switch r.Command {
	case driver.PermissionStopGuide:
		return
	case driver.PermissionOpenSettings:
		w.engine.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id})
		return
	}

	ctrl := getLinuxPermissionController(w.appID)
	if !ctrl.needsSandboxQuery(r.Name) {
		w.engine.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: driver.PermissionNotGated})
		return
	}

	bindPermissions()
	name := r.Name
	appID := w.appID
	if r.Command != driver.PermissionAsk {
		// PermissionCheck and PermissionGuide (Linux has no settings page to guide to).
		portalQueue() <- func() {
			status := ctrl.check(name)
			w.dispatchPermission(id, status)
		}
		return
	}

	// PermissionAsk: run portal prompt off the GTK main thread, subscribing to
	// Request::Response on the GTK main thread before invoking the portal call.
	portalQueue() <- func() {
		if name == driver.PermissionMicrophone {
			if status, skip := ctrl.shouldSkipMicrophonePrompt(); skip {
				w.dispatchPermission(id, status)
				return
			}
		}
		bus := portalBus(appID)
		if bus == 0 {
			w.dispatchPermission(id, resolvePermissionAskError(ctrl, name, errors.New("no session bus")))
			return
		}
		token := portalToken("nsgo_perm")
		expected := portalRequestPath(bus, token)
		if expected == "" {
			w.dispatchPermission(id, resolvePermissionAskError(ctrl, name, errors.New("no portal request path")))
			return
		}
		w.post(func() {
			if w.closing {
				return
			}
			pending := registerLinuxPermission(w, id, name, ctrl, bus, expected)
			if pending == nil {
				portalQueue() <- func() {
					w.dispatchPermission(id, resolvePermissionAskError(ctrl, name, errors.New("could not subscribe to portal response")))
				}
				return
			}
			portalQueue() <- func() {
				var (
					reply uintptr
					err   error
				)
				if name == driver.PermissionCamera {
					reply, err = portalTryTimeout(bus, portalDesktopName, portalDesktopPath, portalCameraIface, "AccessCamera", -1,
						portalOptions(token))
				} else {
					reply, err = portalCallAccessDevice(bus, uint32(os.Getpid()), permissionIDMicrophone, token)
				}
				if err != nil || reply == 0 {
					w.post(func() { cancelLinuxPermission(pending.key, false) })
					if err == nil {
						err = errors.New("empty portal reply")
					}
					w.dispatchPermission(id, resolvePermissionAskError(ctrl, name, err))
					return
				}
				actual := portalTupleText(reply, 0)
				gVariantUnref(reply)
				if actual != "" && actual != expected {
					w.post(func() { rebindLinuxPermission(pending.key, actual) })
				}
			}
		})
	}
}

func resolvePermissionAskError(ctrl *linuxPermissionController, name string, err error) int {
	if name == driver.PermissionCamera {
		return ctrl.resolveCameraAskResponse(0, err)
	}
	return ctrl.resolveMicrophoneAskResponse(0, err)
}

func (w *window) dispatchPermission(id uint32, status int) {
	w.post(func() {
		if !w.closing && w.engine != nil {
			w.engine.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: status})
		}
	})
}

func (w *window) closePermissions() {
	linuxPermReqMu.Lock()
	var owned []*linuxPortalPermissionRequest
	for k, req := range linuxPermReqs {
		if req.w == w {
			owned = append(owned, req)
			delete(linuxPermReqs, k)
		}
	}
	linuxPermReqMu.Unlock()
	for _, req := range owned {
		if req.sub != 0 {
			gDBusConnectionSignalUnsubscribe(req.bus, req.sub)
			req.sub = 0
		}
		if req.bus != 0 && req.request != "" {
			bus, path := req.bus, req.request
			portalQueue() <- func() { gVariantUnrefNonZero(portalCall(bus, path, portalRequestIface, "Close")) }
		}
	}
}

func registerLinuxPermission(w *window, id uint32, name string, ctrl *linuxPermissionController, bus uintptr, requestPath string) *linuxPortalPermissionRequest {
	key := linuxPermSeq.Add(1)
	req := &linuxPortalPermissionRequest{key: key, w: w, id: id, name: name, ctrl: ctrl, bus: bus, request: requestPath}
	linuxPermReqMu.Lock()
	linuxPermReqs[key] = req
	linuxPermReqMu.Unlock()
	sub := gDBusConnectionSignalSubscribe(bus, cstr(portalDesktopName), cstr(portalRequestIface), cstr("Response"),
		cstr(requestPath), nil, 0, cbPermissionPortalResponse(), key, 0)
	if sub == 0 {
		linuxPermReqMu.Lock()
		delete(linuxPermReqs, key)
		linuxPermReqMu.Unlock()
		return nil
	}
	req.sub = sub
	return req
}

func cancelLinuxPermission(key uintptr, closeRemote bool) {
	linuxPermReqMu.Lock()
	req := linuxPermReqs[key]
	delete(linuxPermReqs, key)
	linuxPermReqMu.Unlock()
	if req == nil {
		return
	}
	if req.sub != 0 {
		gDBusConnectionSignalUnsubscribe(req.bus, req.sub)
		req.sub = 0
	}
	if closeRemote && req.bus != 0 && req.request != "" {
		bus, path := req.bus, req.request
		portalQueue() <- func() { gVariantUnrefNonZero(portalCall(bus, path, portalRequestIface, "Close")) }
	}
}

func rebindLinuxPermission(key uintptr, actual string) {
	linuxPermReqMu.Lock()
	req := linuxPermReqs[key]
	linuxPermReqMu.Unlock()
	if req == nil || req.w.closing {
		return
	}
	if req.sub != 0 {
		gDBusConnectionSignalUnsubscribe(req.bus, req.sub)
	}
	req.request = actual
	req.sub = gDBusConnectionSignalSubscribe(req.bus, cstr(portalDesktopName), cstr(portalRequestIface), cstr("Response"),
		cstr(actual), nil, 0, cbPermissionPortalResponse(), key, 0)
}

func permissionPortalResponse(conn, sender, path, iface, signal, params, data uintptr) {
	linuxPermReqMu.Lock()
	req := linuxPermReqs[data]
	delete(linuxPermReqs, data)
	linuxPermReqMu.Unlock()
	if req == nil {
		return
	}
	if req.sub != 0 {
		gDBusConnectionSignalUnsubscribe(req.bus, req.sub)
		req.sub = 0
	}
	if req.w.closing || req.w.engine == nil {
		return
	}
	code := portalResponseOther
	if params != 0 && gVariantNChildren(params) >= 1 {
		code = portalTupleUint32(params, 0)
	}
	portalQueue() <- func() {
		var status int
		if req.name == driver.PermissionCamera {
			status = req.ctrl.resolveCameraAskResponse(code, nil)
		} else {
			status = req.ctrl.resolveMicrophoneAskResponse(code, nil)
		}
		req.w.dispatchPermission(req.id, status)
	}
}

// linuxRealPermissionBackend implements linuxPermissionBackend over GDBus and
// PipeWire's native Unix socket.
type linuxRealPermissionBackend struct {
	appID string
}

func (b *linuxRealPermissionBackend) lookupPermissionStore(table, id, appID string) permissionStoreResult {
	if appID == "" {
		return permissionStoreUnavailable
	}
	bus := sessionBus()
	if bus == 0 {
		return permissionStoreUnavailable
	}
	// Try PermissionStore v2 GetPermission(table, id, app) -> (as).
	reply, err := portalTryTimeout(bus, permissionStoreName, permissionStorePath, permissionStoreIface, "GetPermission",
		permissionCallTimeoutMillis,
		gVariantNewString(cstr(table)),
		gVariantNewString(cstr(id)),
		gVariantNewString(cstr(appID)))
	if err == nil && reply != 0 {
		defer gVariantUnref(reply)
		return parsePermissionStoreValues(variantTupleStrings(reply, 0))
	}
	if isPortalNotFoundError(err) {
		return permissionStoreNotFound
	}
	if !isPortalUnknownMethodError(err) {
		return permissionStoreUnavailable
	}

	// Fall back to PermissionStore v1 Lookup(table, id) -> (a{sas}, v).
	reply, err = portalTryTimeout(bus, permissionStoreName, permissionStorePath, permissionStoreIface, "Lookup",
		permissionCallTimeoutMillis,
		gVariantNewString(cstr(table)),
		gVariantNewString(cstr(id)))
	if err != nil || reply == 0 {
		if isPortalNotFoundError(err) {
			return permissionStoreNotFound
		}
		return permissionStoreUnavailable
	}
	defer gVariantUnref(reply)
	if gVariantNChildren(reply) < 1 {
		return permissionStoreUnavailable
	}
	tableDict := gVariantGetChildValue(reply, 0)
	defer gVariantUnref(tableDict)
	entry := gVariantLookupValue(tableDict, cstr(appID), 0)
	if entry == 0 {
		return permissionStoreNotFound
	}
	defer gVariantUnref(entry)
	return parsePermissionStoreValues(variantStringArray(entry))
}

func (b *linuxRealPermissionBackend) cameraPortalAvailable() (bool, bool) {
	bus := portalBus(b.appID)
	if bus == 0 {
		return false, false
	}
	reply, err := portalTryTimeout(bus, portalDesktopName, portalDesktopPath, dbusPropertiesIface, "Get",
		permissionCallTimeoutMillis,
		gVariantNewString(cstr(portalCameraIface)),
		gVariantNewString(cstr("IsCameraPresent")))
	if err != nil || reply == 0 {
		return false, false
	}
	defer gVariantUnref(reply)
	return portalPropertyBool(reply)
}

func (b *linuxRealPermissionBackend) cameraOpenPipeWireRemote() cameraRemoteProbe {
	bus := portalBus(b.appID)
	if bus == 0 {
		return cameraRemoteUnavailable
	}
	options := portalVariant("a{sv}", "{}")
	if options == 0 {
		return cameraRemoteUnavailable
	}
	arg := gVariantTakeRef(options)
	defer gVariantUnrefNonZero(arg)
	params := gVariantNewTuple(&arg, 1)
	var (
		outFDList uintptr
		gerr      uintptr
	)
	reply := gDBusConnectionCallWithUnixFDListSync(bus,
		cstr(portalDesktopName), cstr(portalDesktopPath), cstr(portalCameraIface), cstr("OpenPipeWireRemote"),
		params, 0, 0, permissionCallTimeoutMillis, 0, &outFDList, 0, &gerr)
	if outFDList != 0 {
		// Unreffing the GUnixFDList closes the PipeWire remote FD owned by it.
		gObjectUnref(outFDList)
	}
	if reply != 0 {
		gVariantUnref(reply)
	}
	if gerr != 0 {
		msg := goString((*gError)(cPointer(gerr)).message)
		gErrorFree(gerr)
		return classifyCameraRemoteError(errors.New(msg))
	}
	if reply == 0 {
		return cameraRemoteUnavailable
	}
	return cameraRemoteGranted
}

func (b *linuxRealPermissionBackend) cameraAccess(done func(code uint32, err error)) (cancel func()) {
	// Async AccessCamera is driven directly on window.permission to coordinate
	// GLib signal subscriptions on the GTK main thread.
	if done != nil {
		done(portalResponseOther, errors.New("unsupported synchronous cameraAccess call"))
	}
	return func() {}
}

func (b *linuxRealPermissionBackend) devicePortalAvailable() bool {
	bus := portalBus(b.appID)
	if bus == 0 {
		return false
	}
	reply, err := portalTryTimeout(bus, portalDesktopName, portalDesktopPath, dbusPropertiesIface, "Get",
		permissionCallTimeoutMillis,
		gVariantNewString(cstr(portalDeviceIface)),
		gVariantNewString(cstr("version")))
	if err != nil || reply == 0 {
		return false
	}
	defer gVariantUnref(reply)
	ver, ok := portalPropertyUint32(reply)
	return ok && ver >= 1
}

func (b *linuxRealPermissionBackend) deviceAccess(device string, done func(code uint32, err error)) (cancel func()) {
	// Async AccessDevice is driven directly on window.permission to coordinate
	// GLib signal subscriptions on the GTK main thread.
	if done != nil {
		done(portalResponseOther, errors.New("unsupported synchronous deviceAccess call"))
	}
	return func() {}
}

func (b *linuxRealPermissionBackend) probePipeWireMicrophone() pipewireMicProbe {
	path := pipeWireSocketPath(os.Getenv)
	return probePipeWireMicrophonePath(path, pipeWireProbeTimeout)
}

func portalCallAccessDevice(bus uintptr, pid uint32, device, token string) (uintptr, error) {
	strType := gVariantTypeNew(cstr("s"))
	defer gVariantTypeFree(strType)
	item := gVariantNewString(cstr(device))
	devices := gVariantNewArray(strType, &item, 1)
	return portalTryTimeout(bus, portalDesktopName, portalDesktopPath, portalDeviceIface, "AccessDevice", -1,
		gVariantNewUint32(pid), devices, portalOptions(token))
}

// portalTryTimeout calls a D-Bus method with explicit destination, taking
// ownership of args (floating or non-floating) and returning any GError text.
func portalTryTimeout(bus uintptr, name, path, iface, method string, timeoutMillis int32, args ...uintptr) (uintptr, error) {
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
		return 0, errors.New("invalid dbus argument")
	}
	var params uintptr
	if len(args) > 0 {
		params = gVariantNewTuple(&args[0], uintptr(len(args)))
	} else {
		params = gVariantNewTuple(nil, 0)
	}
	var gerr uintptr
	reply := gDBusConnectionCallSync(bus, cstr(name), cstr(path), cstr(iface), cstr(method),
		params, 0, 0, timeoutMillis, 0, &gerr)
	if gerr != 0 {
		msg := goString((*gError)(cPointer(gerr)).message)
		gErrorFree(gerr)
		return 0, errors.New(msg)
	}
	return reply, nil
}

func portalPropertyBool(reply uintptr) (bool, bool) {
	if reply == 0 || gVariantNChildren(reply) < 1 {
		return false, false
	}
	v := gVariantGetChildValue(reply, 0)
	defer func() { gVariantUnrefNonZero(v) }()
	for v != 0 && goString(gVariantGetTypeString(v)) == "v" {
		inner := gVariantGetVariant(v)
		gVariantUnref(v)
		v = inner
	}
	if v == 0 || goString(gVariantGetTypeString(v)) != "b" {
		return false, false
	}
	return permVariantGetBool(v) != 0, true
}

func variantTupleStrings(tuple uintptr, index int) []string {
	if tuple == 0 || int(gVariantNChildren(tuple)) <= index {
		return nil
	}
	child := gVariantGetChildValue(tuple, uintptr(index))
	defer gVariantUnref(child)
	return variantStringArray(child)
}

func variantStringArray(arr uintptr) []string {
	if arr == 0 {
		return nil
	}
	n := int(gVariantNChildren(arr))
	if n == 0 {
		return nil
	}
	out := make([]string, 0, n)
	for i := range n {
		el := gVariantGetChildValue(arr, uintptr(i))
		if el != 0 {
			if goString(gVariantGetTypeString(el)) == "s" {
				out = append(out, goString(gVariantGetString(el, 0)))
			}
			gVariantUnref(el)
		}
	}
	return out
}
