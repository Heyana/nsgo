//go:build linux && !android

package linux

import (
	"sync"

	"github.com/ebitengine/purego"
)

const inhibitPortal = "org.freedesktop.portal.Inhibit"

var (
	gDBusConnectionCall func(bus uintptr, name, path, iface, method *byte, params, replyType uintptr,
		flags uint32, timeout int32, cancellable, callback, data uintptr)
	bindSession = sync.OnceFunc(func() {
		bindGlobalHotKeys()
		purego.RegisterLibFunc(&gDBusConnectionCall, gioLib, "g_dbus_connection_call")
	})
	cbSessionState = sync.OnceValue(func() uintptr { return purego.NewCallback(sessionStateChanged) })
)

type sessionMonitor struct {
	bus           uintptr
	request, path string
	sub           uint32
}

// watchSession monitors the desktop session through the Inhibit portal when
// it is available. The monitor itself never inhibits logout or shutdown.
func (w *window) watchSession() {
	bindSession()
	portalQueue() <- func() {
		bus := portalBus(w.opts.ID)
		if bus == 0 {
			return
		}
		token := portalToken("nsgo_session")
		request := portalRequestPath(bus, token)
		path := portalHandlePath(bus, "session", token)
		if path == "" {
			return
		}
		w.post(func() {
			if w.closing {
				return
			}
			w.session = &sessionMonitor{bus: bus, request: request, path: path}
			// Subscribe before CreateMonitor so even the first state change can
			// be matched against the session handle the portal will derive.
			w.session.sub = gDBusConnectionSignalSubscribe(bus, cstr(globalShortcutName),
				cstr(inhibitPortal), cstr("StateChanged"), cstr(globalShortcutPath), nil, 0, cbSessionState(), w.handle, 0)
			sessionCall(bus, globalShortcutPath, inhibitPortal, "CreateMonitor",
				gVariantNewString(cstr("")), portalCreateOptions(token))
		})
	}
}

func (w *window) unwatchSession() {
	m := w.session
	if m == nil {
		return
	}
	w.session = nil
	gDBusConnectionSignalUnsubscribe(m.bus, m.sub)
	sessionCall(m.bus, m.request, globalRequestIface, "Close")
	sessionCall(m.bus, m.path, globalSessionIface, "Close")
}

// sessionCall sends the portal request without holding up the GTK thread.
func sessionCall(bus uintptr, path, iface, method string, args ...uintptr) {
	var params uintptr
	if len(args) > 0 {
		params = gVariantNewTuple(&args[0], uintptr(len(args)))
	}
	gDBusConnectionCall(bus, cstr(globalShortcutName), cstr(path), cstr(iface), cstr(method),
		params, 0, 0, -1, 0, 0, 0)
}

func sessionStateChanged(conn, sender, path, iface, signal, params, data uintptr) {
	w := windowOf(data)
	if w == nil || w.session == nil || w.closing {
		return
	}
	m := w.session
	if portalTupleText(params, 0) != m.path {
		return
	}
	dict := gVariantGetChildValue(params, 1)
	state := gVariantLookupValue(dict, cstr("session-state"), 0)
	gVariantUnref(dict)
	if state == 0 {
		return
	}
	value := gVariantGetUint32(state)
	gVariantUnref(state)
	switch value {
	case 2: // Query End. Acknowledge immediately, since the desktop may still cancel logout.
		sessionCall(m.bus, globalShortcutPath, inhibitPortal, "QueryEndResponse", gVariantNewObjectPath(cstr(m.path)))
	case 3: // Ending. The app has its bounded OnQuit interval to save its state.
		w.shutdown(true)
	}
}
