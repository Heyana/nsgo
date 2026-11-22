//go:build linux && !android

package linux

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// launcherMenu is the app's quicklist on the same connection that emits its
// LauncherEntry updates. The root surface owns its export, while owner is the
// surface whose tree must hear EventDockMenu.
type launcherMenu struct {
	bus          uintptr
	path         string
	registration uint32
	watch        uint32
	owner        *window
	menu         dbusMenu
}

var (
	launcherMenuVTable = gDBusInterfaceVTable{}
	cbLauncherChanged  uintptr
)

func init() {
	launcherMenuVTable = gDBusInterfaceVTable{
		MethodCall:  newCallback(launcherMenuMethod),
		GetProperty: newCallback(launcherMenuProperty),
	}
	cbLauncherChanged = newCallback(launcherChanged)
}

func (w *window) launcherRoot() *window {
	for w.opener != nil {
		w = w.opener
	}
	return w
}

func (w *window) setDockMenu(entries []driver.MenuEntry) {
	root := w.launcherRoot()
	if root.closing || root.opts.ID == "" {
		return
	}
	if root.dockMenu == nil {
		trayBind()
		bus := sessionBus()
		if bus == 0 {
			return
		}
		var err uintptr
		if trayNodeInfo == 0 {
			trayNodeInfo = gDBusNodeInfoNewForXML(cstr(trayIntrospection), &err)
			if err != 0 {
				gErrorFree(err)
				return
			}
		}
		if trayNodeInfo == 0 {
			return
		}
		m := &launcherMenu{bus: bus, path: fmt.Sprintf("%s/Quicklist%d", launcherEntryPath(root.opts.ID), root.handle)}
		info := gDBusNodeInfoLookupInterface(trayNodeInfo, cstr(trayMenuInterface))
		m.registration = gDBusConnectionRegisterObject(bus, cstr(m.path), info, &launcherMenuVTable, root.handle, 0, &err)
		if err != 0 {
			gErrorFree(err)
		}
		if m.registration == 0 {
			return
		}
		// Launchers request the current properties by acquiring this name.
		m.watch = gDBusConnectionSignalSubscribe(bus, cstr("org.freedesktop.DBus"), cstr("org.freedesktop.DBus"),
			cstr("NameOwnerChanged"), cstr("/org/freedesktop/DBus"), cstr("com.canonical.Unity"), 0, cbLauncherChanged, root.handle, 0)
		root.dockMenu = m
	}
	m := root.dockMenu
	m.owner = w
	m.menu.set(entries)
	m.publish(root.opts.ID)
	m.layoutChanged()
}

func (m *launcherMenu) layoutChanged() {
	params := []uintptr{gVariantNewUint32(m.menu.revision), gVariantNewInt32(0)}
	var err uintptr
	gDBusConnectionEmitSignal(m.bus, nil, cstr(m.path), cstr(trayMenuInterface), cstr("LayoutUpdated"), gVariantNewTuple(&params[0], 2), &err)
	if err != 0 {
		gErrorFree(err)
	}
}

func (m *launcherMenu) publish(appID string) {
	params := []uintptr{gVariantNewString(cstr(launcherEntryURI(appID))), menuDict([]uintptr{
		menuDictEntry("quicklist", gVariantNewString(cstr(m.path))),
	})}
	var err uintptr
	gDBusConnectionEmitSignal(m.bus, nil, cstr(launcherEntryPath(appID)), cstr("com.canonical.Unity.LauncherEntry"),
		cstr("Update"), gVariantNewTuple(&params[0], 2), &err)
	if err != 0 {
		gErrorFree(err)
	}
}

func launcherChanged(conn, sender, path, iface, signal, params, data uintptr) {
	if root := windowOf(data); root != nil && !root.closing && root.dockMenu != nil && variantString(params, 2) != "" {
		root.dockMenu.publish(root.opts.ID)
	}
}

func (w *window) closeDockMenu() {
	root := w.launcherRoot()
	m := root.dockMenu
	if m == nil {
		return
	}
	if root != w {
		if m.owner == w {
			m.owner = nil
			m.menu.set(nil)
			m.layoutChanged()
		}
		return
	}
	// libunity advertises an empty string when there is no quicklist.
	m.path = ""
	m.publish(root.opts.ID)
	gDBusConnectionSignalUnsubscribe(m.bus, m.watch)
	gDBusConnectionUnregisterObject(m.bus, m.registration)
	root.dockMenu = nil
}

func launcherMenuProperty(conn, sender, path, iface, property uintptr, err, data uintptr) uintptr {
	return menuProperty(goString(property))
}

func launcherMenuMethod(conn, sender, path, iface, method, params, invocation, data uintptr) {
	root := windowOf(data)
	if root == nil || root.closing || root.dockMenu == nil {
		trayError(invocation, "org.freedesktop.DBus.Error.UnknownObject", "no such launcher menu")
		return
	}
	m := root.dockMenu
	if m.menu.readMethod(goString(method), params, invocation) {
		return
	}
	if m.menu.eventMethod(goString(method), params, invocation, func(id int32, name string) {
		if name == "clicked" {
			m.choose(id)
		}
	}) {
		return
	}
	switch goString(method) {
	case "AboutToShow":
		if _, _, ok := m.menu.node(variantInt32(params, 0)); !ok {
			trayError(invocation, "org.freedesktop.DBus.Error.InvalidArgs", "no such menu item")
			return
		}
		v := gVariantNewBoolean(0)
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&v, 1))
	case "AboutToShowGroup":
		var errors []uintptr
		for _, id := range variantInt32s(params, 0) {
			if _, _, ok := m.menu.node(id); !ok {
				errors = append(errors, gVariantNewInt32(id))
			}
		}
		values := []uintptr{menuArray("i", nil), menuArray("i", errors)}
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&values[0], 2))
	default:
		returnTray(invocation)
	}
}

func (m *launcherMenu) choose(id int32) bool {
	entry, children, ok := m.menu.node(id)
	if !ok || entry == nil || entry.Disabled || entry.Separator || len(children) != 0 || entry.ID == 0 || m.owner == nil || m.owner.closing {
		return false
	}
	m.owner.engine.Dispatch(driver.Event{Kind: driver.EventDockMenu, Count: entry.ID})
	return true
}
