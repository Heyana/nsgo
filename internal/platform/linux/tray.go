//go:build linux && !android

package linux

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Linux tray icons use the StatusNotifierItem protocol under the org.kde names that its hosts use, KDE Plasma's and GNOME's AppIndicator extension, which reach an item only through org.kde.StatusNotifierItem rather than the freedesktop draft's name. The watcher owns the visual presentation; the app owns one item per tray ID, each on a session bus connection of its own so that it has the protocol's /StatusNotifierItem path and its bus name to itself, and receives Activate, SecondaryActivate and ContextMenu calls. Its menu is a com.canonical.dbusmenu object at /Menu on the same connection.

const (
	trayServicePrefix = "org.kde.StatusNotifierItem-"
	trayItemInterface = "org.kde.StatusNotifierItem"
	trayMenuInterface = "com.canonical.dbusmenu"
	trayItemPath      = "/StatusNotifierItem"
	trayMenuPath      = "/Menu"
	trayWatcher       = "org.kde.StatusNotifierWatcher"
	trayWatcherAlt    = "org.freedesktop.StatusNotifierWatcher"
	trayWatcherPath   = "/StatusNotifierWatcher"
	trayWatcherIface  = "org.kde.StatusNotifierWatcher"
	// trayMenuWait is how long, in milliseconds, a host's AboutToShow for the menu waits for the app's menu before it is answered with the one the tray has.
	trayMenuWait = 250
	// trayIconSize is the longest side of the pixmap the tray publishes.
	trayIconSize = 64
)

// GDBus answers org.freedesktop.DBus.Properties itself, from the properties declared here and the vtable's GetProperty, with an error for a property the interface lacks.
const trayIntrospection = `
<node>
  <interface name="org.kde.StatusNotifierItem">
    <method name="Activate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
    <method name="SecondaryActivate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
    <method name="ContextMenu"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
    <method name="Scroll"><arg name="delta" type="i" direction="in"/><arg name="orientation" type="s" direction="in"/></method>
    <signal name="NewTitle"/><signal name="NewIcon"/><signal name="NewAttentionIcon"/><signal name="NewOverlayIcon"/><signal name="NewToolTip"/><signal name="NewStatus"><arg name="status" type="s"/></signal>
    <property name="Category" type="s" access="read"/>
    <property name="Id" type="s" access="read"/>
    <property name="Title" type="s" access="read"/>
    <property name="Status" type="s" access="read"/>
    <property name="WindowId" type="i" access="read"/>
    <property name="IconName" type="s" access="read"/>
    <property name="IconPixmap" type="a(iiay)" access="read"/>
    <property name="ToolTip" type="(sa(iiay)ss)" access="read"/>
    <property name="ItemIsMenu" type="b" access="read"/>
    <property name="Menu" type="o" access="read"/>
  </interface>
  <interface name="com.canonical.dbusmenu">
    <method name="GetLayout"><arg name="parentId" type="i" direction="in"/><arg name="recursionDepth" type="i" direction="in"/><arg name="propertyNames" type="as" direction="in"/><arg name="revision" type="u" direction="out"/><arg name="layout" type="(ia{sv}av)" direction="out"/></method>
    <method name="GetGroupProperties"><arg name="ids" type="ai" direction="in"/><arg name="propertyNames" type="as" direction="in"/><arg name="properties" type="a(ia{sv})" direction="out"/></method>
    <method name="GetProperty"><arg name="id" type="i" direction="in"/><arg name="name" type="s" direction="in"/><arg name="value" type="v" direction="out"/></method>
    <method name="Event"><arg name="id" type="i" direction="in"/><arg name="eventId" type="s" direction="in"/><arg name="data" type="v" direction="in"/><arg name="timestamp" type="u" direction="in"/></method>
    <method name="AboutToShow"><arg name="id" type="i" direction="in"/><arg name="needUpdate" type="b" direction="out"/></method>
    <method name="AboutToShowGroup"><arg name="ids" type="ai" direction="in"/><arg name="updatesNeeded" type="ai" direction="out"/><arg name="idErrors" type="ai" direction="out"/></method>
    <signal name="ItemsPropertiesUpdated"><arg name="updatedProps" type="a(ia{sv})"/><arg name="removedProps" type="a(ias)"/></signal>
    <signal name="LayoutUpdated"><arg name="revision" type="u"/><arg name="parentId" type="i"/></signal>
    <property name="Version" type="u" access="read"/>
    <property name="TextDirection" type="s" access="read"/>
    <property name="Status" type="s" access="read"/>
    <property name="IconThemePath" type="as" access="read"/>
  </interface>
</node>`

type gDBusInterfaceVTable struct {
	MethodCall  uintptr
	GetProperty uintptr
	SetProperty uintptr
	Padding     [8]uintptr
}

type linuxTray struct {
	cfg    driver.TrayConfig
	bus    uintptr // the tray's own session bus connection
	name   string
	owner  uint32
	itemID uint32
	menuID uint32
	// pixmap is the icon as the item's ARGB32 pixels, pixW by pixH, nil without a PNG.
	pixmap     []byte
	pixW, pixH int
	menu       trayMenu
	popupID    uint32
	// asked are the hosts' AboutToShow calls for the menu, which wait for the app's menu (ItemTrayMenu) or until waitID's timeout.
	asked  []uintptr
	waitID uint32
}

// trayMenu is a tray's menu as dbusmenu numbers it: every entry has an ID of its own, which the menus the tray shows never reuse, so that an event for an entry of an older layout finds none. 0 is the root, whose children root lists.
type trayMenu struct {
	items    map[int32]*trayItem
	root     []int32
	last     int32
	revision uint32
}

type trayItem struct {
	entry    driver.MenuEntry // Items left out: children has them
	children []int32
}

// set numbers entries as the menu's new layout.
func (m *trayMenu) set(entries []driver.MenuEntry) {
	m.items = map[int32]*trayItem{}
	m.root = m.add(entries)
	m.revision++
}

func (m *trayMenu) add(entries []driver.MenuEntry) []int32 {
	ids := make([]int32, len(entries))
	for i, e := range entries {
		m.last++
		id := m.last
		ids[i] = id
		it := &trayItem{entry: e}
		it.entry.Items = nil
		m.items[id] = it
		it.children = m.add(e.Items)
	}
	return ids
}

// node returns the entry id names, nil for the root, and its children's IDs; ok is false for an ID the layout lacks.
func (m *trayMenu) node(id int32) (entry *driver.MenuEntry, children []int32, ok bool) {
	if id == 0 {
		return nil, m.root, true
	}
	it := m.items[id]
	if it == nil {
		return nil, nil, false
	}
	return &it.entry, it.children, true
}

// itemProperties are the dbusmenu properties the menu's entries have.
var itemProperties = []string{"type", "label", "enabled", "visible", "children-display", "toggle-type", "toggle-state"}

// itemProperty is an entry's property, 0 where it has the default; entry is nil for the root.
func itemProperty(entry *driver.MenuEntry, parent bool, name string) uintptr {
	if name == "children-display" {
		if parent {
			return gVariantNewString(cstr("submenu"))
		}
		return 0
	}
	if entry == nil {
		return 0
	}
	switch name {
	case "type":
		if entry.Separator {
			return gVariantNewString(cstr("separator"))
		}
	case "label":
		if !entry.Separator {
			return gVariantNewString(cstr(menuLabel(entry.Label)))
		}
	case "enabled":
		return gVariantNewBoolean(boolInt(!entry.Disabled))
	case "visible":
		return gVariantNewBoolean(1)
	case "toggle-type":
		if entry.Radio {
			return gVariantNewString(cstr("radio"))
		}
		if entry.Checkable {
			return gVariantNewString(cstr("checkmark"))
		}
	case "toggle-state":
		if entry.Checkable {
			return gVariantNewInt32(boolInt(entry.Checked))
		}
	}
	return 0
}

// menuLabel escapes a label's underscores, which dbusmenu reads as access keys.
func menuLabel(label string) string { return strings.ReplaceAll(label, "_", "__") }

var (
	gDBusNodeInfoNewForXML               func(xml *byte, err *uintptr) uintptr
	gDBusNodeInfoLookupInterface         func(info uintptr, name *byte) uintptr
	gDBusConnectionRegisterObject        func(conn uintptr, path *byte, info uintptr, vtable *gDBusInterfaceVTable, data, destroy uintptr, err *uintptr) uint32
	gDBusConnectionUnregisterObject      func(conn uintptr, registration uint32) int32
	gDBusConnectionCloseSync             func(conn, cancellable uintptr, err *uintptr) int32
	gBusOwnNameOnConnection              func(conn uintptr, name *byte, flags uint32, acquired, lost, data, destroy uintptr) uint32
	gBusUnownName                        func(owner uint32)
	gDBusMethodInvocationReturnValue     func(invocation, value uintptr)
	gDBusMethodInvocationReturnDBusError func(invocation uintptr, name, message *byte)
	gVariantNewFixedArray                func(elementType uintptr, data unsafe.Pointer, n, elementSize uintptr) uintptr
	trayBind                             = sync.OnceFunc(func() {
		bindInputSources()
		bindNotifications()
		bindLauncherEntry()
		purego.RegisterLibFunc(&gDBusNodeInfoNewForXML, gioLib, "g_dbus_node_info_new_for_xml")
		purego.RegisterLibFunc(&gDBusNodeInfoLookupInterface, gioLib, "g_dbus_node_info_lookup_interface")
		purego.RegisterLibFunc(&gDBusConnectionRegisterObject, gioLib, "g_dbus_connection_register_object")
		purego.RegisterLibFunc(&gDBusConnectionUnregisterObject, gioLib, "g_dbus_connection_unregister_object")
		purego.RegisterLibFunc(&gDBusConnectionCloseSync, gioLib, "g_dbus_connection_close_sync")
		purego.RegisterLibFunc(&gDBusAddressGetForBusSync, gioLib, "g_dbus_address_get_for_bus_sync")
		purego.RegisterLibFunc(&gBusOwnNameOnConnection, gioLib, "g_bus_own_name_on_connection")
		purego.RegisterLibFunc(&gBusUnownName, gioLib, "g_bus_unown_name")
		purego.RegisterLibFunc(&gDBusMethodInvocationReturnValue, gioLib, "g_dbus_method_invocation_return_value")
		purego.RegisterLibFunc(&gDBusMethodInvocationReturnDBusError, gioLib, "g_dbus_method_invocation_return_dbus_error")
		purego.RegisterLibFunc(&gVariantNewFixedArray, glibLib, "g_variant_new_fixed_array")
		purego.RegisterLibFunc(&gVariantNewObjectPath, glibLib, "g_variant_new_object_path")
	})
	trayNodeInfo      uintptr
	trayVTable        = gDBusInterfaceVTable{}
	cbTrayMethod      uintptr
	cbTrayGetProperty uintptr
	cbTraySetProperty uintptr
	cbTrayAcquired    uintptr
	cbTrayMenuWait    uintptr
)

func init() {
	cbTrayMethod = newCallback(trayMethodCall)
	cbTrayGetProperty = newCallback(trayGetProperty)
	cbTraySetProperty = newCallback(traySetProperty)
	cbTrayAcquired = newCallback(trayNameAcquired)
	cbTrayMenuWait = newCallback(trayMenuWaitCallback)
	trayVTable = gDBusInterfaceVTable{MethodCall: cbTrayMethod, GetProperty: cbTrayGetProperty, SetProperty: cbTraySetProperty}
}

func (w *window) applyTray(cfg driver.TrayConfig) {
	trayBind()
	tr := w.trays[cfg.ID]
	if cfg.Removed {
		if tr != nil {
			w.removeTray(tr)
		}
		return
	}
	if tr != nil {
		was := tr.cfg
		tr.setConfig(cfg)
		// Hosts read the properties again only as these signals say.
		if !bytes.Equal(was.Icon, cfg.Icon) {
			tr.emit(trayItemPath, trayItemInterface, "NewIcon", 0)
		}
		if was.Title != cfg.Title {
			tr.emit(trayItemPath, trayItemInterface, "NewTitle", 0)
		}
		if was.Tooltip != cfg.Tooltip || !bytes.Equal(was.Icon, cfg.Icon) {
			tr.emit(trayItemPath, trayItemInterface, "NewToolTip", 0)
		}
		return
	}
	if trayNodeInfo == 0 {
		var err uintptr
		trayNodeInfo = gDBusNodeInfoNewForXML(cstr(trayIntrospection), &err)
		if err != 0 {
			gErrorFree(err)
			trayNodeInfo = 0
		}
	}
	if trayNodeInfo == 0 {
		return
	}
	itemInfo := gDBusNodeInfoLookupInterface(trayNodeInfo, cstr(trayItemInterface))
	menuInfo := gDBusNodeInfoLookupInterface(trayNodeInfo, cstr(trayMenuInterface))
	if itemInfo == 0 || menuInfo == 0 {
		return
	}
	bus := trayConnection()
	if bus == 0 {
		return
	}
	tr = &linuxTray{bus: bus, name: fmt.Sprintf("%s%d-%d", trayServicePrefix, os.Getpid(), cfg.ID)}
	tr.setConfig(cfg)
	var err uintptr
	tr.itemID = gDBusConnectionRegisterObject(bus, cstr(trayItemPath), itemInfo, &trayVTable, w.handle, 0, &err)
	if err != 0 {
		gErrorFree(err)
		err = 0
	}
	if tr.itemID == 0 {
		closeTrayConnection(bus)
		return
	}
	tr.menuID = gDBusConnectionRegisterObject(bus, cstr(trayMenuPath), menuInfo, &trayVTable, w.handle, 0, &err)
	if err != 0 {
		gErrorFree(err)
	}
	w.trays[cfg.ID] = tr
	tr.owner = gBusOwnNameOnConnection(bus, cstr(tr.name), 0, cbTrayAcquired, 0, w.handle, 0)
}

// trayConnection opens a session bus connection for one tray, 0 without a session bus.
func trayConnection() uintptr {
	const busSession = 2
	const authClient, messageBus = 1 << 0, 1 << 3
	var err uintptr
	address := gDBusAddressGetForBusSync(busSession, 0, &err)
	if err != 0 {
		gErrorFree(err)
		return 0
	}
	bus := gDBusConnectionNewForAddressSync(cstr(goString(address)), authClient|messageBus, 0, 0, &err)
	gFree(address)
	if err != 0 {
		gErrorFree(err)
		return 0
	}
	return bus
}

// closeTrayConnection closes a tray's connection, which takes its bus name and objects away and so has the watcher drop the item.
func closeTrayConnection(bus uintptr) {
	var err uintptr
	gDBusConnectionCloseSync(bus, 0, &err)
	if err != 0 {
		gErrorFree(err)
	}
	gObjectUnref(bus)
}

// setConfig takes cfg and its icon's pixels.
func (tr *linuxTray) setConfig(cfg driver.TrayConfig) {
	if tr.pixmap == nil || !bytes.Equal(tr.cfg.Icon, cfg.Icon) {
		tr.pixmap, tr.pixW, tr.pixH = trayIcon(cfg.Icon)
	}
	tr.cfg = cfg
}

// emit sends one of the tray's signals; params may be 0 for none.
func (tr *linuxTray) emit(path, iface, signal string, params uintptr) {
	var err uintptr
	gDBusConnectionEmitSignal(tr.bus, nil, cstr(path), cstr(iface), cstr(signal), params, &err)
	if err != 0 {
		gErrorFree(err)
	}
}

func (w *window) removeTray(tr *linuxTray) {
	tr.answerAsked(false)
	if tr.menuID != 0 {
		gDBusConnectionUnregisterObject(tr.bus, tr.menuID)
	}
	if tr.itemID != 0 {
		gDBusConnectionUnregisterObject(tr.bus, tr.itemID)
	}
	if tr.owner != 0 {
		gBusUnownName(tr.owner)
	}
	closeTrayConnection(tr.bus)
	delete(w.trays, tr.cfg.ID)
}

func (w *window) removeTrays() {
	for _, tr := range w.trays {
		w.removeTray(tr)
	}
}

// trayMenuShown takes the menu the app gives for a tray's AboutToShow or click (ItemTrayMenu): hosts that wait for it hear it changed, and the others are told so.
func (w *window) trayMenuShown(tr *linuxTray, entries []driver.MenuEntry, popup uint32) {
	tr.menu.set(entries)
	tr.popupID = popup
	params := []uintptr{gVariantNewUint32(tr.menu.revision), gVariantNewInt32(0)}
	tr.emit(trayMenuPath, trayMenuInterface, "LayoutUpdated", gVariantNewTuple(&params[0], uintptr(len(params))))
	tr.answerAsked(true)
}

// answerAsked answers the AboutToShow calls waiting for the menu.
func (tr *linuxTray) answerAsked(update bool) {
	if tr.waitID != 0 {
		gSourceRemove(tr.waitID)
		tr.waitID = 0
	}
	for _, invocation := range tr.asked {
		v := gVariantNewBoolean(boolInt(update))
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&v, 1))
	}
	tr.asked = nil
}

// trayMenuWaitCallback answers the AboutToShow calls the app has not given a menu for in time, as for a tray without one: the host shows the menu it has.
func trayMenuWaitCallback(data uintptr) int32 {
	if w := windowOf(data); w != nil {
		for _, tr := range w.trays {
			if tr.waitID != 0 {
				tr.waitID = 0
				tr.answerAsked(true)
			}
		}
	}
	return 0
}

func trayNameAcquired(bus, name, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing || w.trayOn(bus) == nil {
		return
	}
	// The watcher finds the item at /StatusNotifierItem of the service it names. A missing watcher is normal on GNOME without its AppIndicator extension.
	service := goString(name)
	if reply := dbusCall(bus, trayWatcher, trayWatcherPath, trayWatcherIface, "RegisterStatusNotifierItem", gVariantNewString(cstr(service))); reply != 0 {
		gVariantUnref(reply)
	} else {
		gVariantUnrefNonZero(dbusCall(bus, trayWatcherAlt, trayWatcherPath, trayWatcherAlt, "RegisterStatusNotifierItem", gVariantNewString(cstr(service))))
	}
}

// trayOn returns the tray whose connection bus is, nil for none.
func (w *window) trayOn(bus uintptr) *linuxTray {
	for _, tr := range w.trays {
		if tr.bus == bus {
			return tr
		}
	}
	return nil
}

func trayMethodCall(conn, sender, path, iface, method, params, invocation, data uintptr) {
	w := windowOf(data)
	var tr *linuxTray
	if w != nil && w.engine != nil && !w.closing {
		tr = w.trayOn(conn)
	}
	if tr == nil {
		trayError(invocation, "org.freedesktop.DBus.Error.UnknownObject", "no such tray")
		return
	}
	if goString(iface) == trayMenuInterface {
		w.trayMenuMethod(tr, goString(method), params, invocation)
		return
	}
	switch goString(method) {
	case "Activate":
		w.engine.Dispatch(driver.Event{Kind: driver.EventTray, Count: tr.cfg.ID, Index: 0})
	case "SecondaryActivate", "ContextMenu":
		w.engine.Dispatch(driver.Event{Kind: driver.EventTray, Count: tr.cfg.ID, Index: 1})
	}
	returnTray(invocation)
}

func trayGetProperty(conn, sender, path, iface, property uintptr, err, data uintptr) uintptr {
	var tr *linuxTray
	id := ""
	if w := windowOf(data); w != nil {
		tr, id = w.trayOn(conn), w.opts.ID
	}
	if tr == nil {
		// GDBus asks only for declared properties and needs a value or an error for each.
		tr = &linuxTray{}
	}
	if goString(iface) == trayMenuInterface {
		return menuProperty(goString(property))
	}
	return trayProperty(tr, id, goString(property))
}

func traySetProperty(conn, sender, path, iface, property, value, err, data uintptr) uintptr {
	return 0
}

func returnTray(invocation uintptr) {
	if invocation != 0 {
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(nil, 0))
	}
}

func trayError(invocation uintptr, name, message string) {
	if invocation != 0 {
		gDBusMethodInvocationReturnDBusError(invocation, cstr(name), cstr(message))
	}
}

// trayProperty is a declared StatusNotifierItem property of tr, appID naming the app.
func trayProperty(tr *linuxTray, appID string, property string) uintptr {
	cfg := tr.cfg
	// A host shows a named icon from the theme ahead of the pixmap, so an icon the app gives leaves the name out; the app's own installed icon shows without one.
	iconName := appID
	if tr.pixmap != nil {
		iconName = ""
	}
	switch property {
	case "Category":
		return gVariantNewString(cstr("ApplicationStatus"))
	case "Id":
		return gVariantNewString(cstr(appID))
	case "Title":
		return gVariantNewString(cstr(cfg.Title))
	case "Status":
		return gVariantNewString(cstr("Active"))
	case "WindowId":
		return gVariantNewInt32(0)
	case "IconName":
		return gVariantNewString(cstr(iconName))
	case "IconPixmap":
		return tr.pixmapVariant()
	case "ToolTip":
		children := []uintptr{gVariantNewString(cstr(iconName)), tr.pixmapVariant(), gVariantNewString(cstr(cfg.Tooltip)), gVariantNewString(cstr(""))}
		return gVariantNewTuple(&children[0], uintptr(len(children)))
	case "ItemIsMenu":
		return gVariantNewBoolean(0)
	case "Menu":
		return gVariantNewObjectPath(cstr(trayMenuPath))
	}
	return gVariantNewString(cstr(""))
}

// menuProperty is a declared property of the dbusmenu object.
func menuProperty(name string) uintptr {
	switch name {
	case "Version":
		return gVariantNewUint32(3)
	case "TextDirection":
		return gVariantNewString(cstr("ltr"))
	case "Status":
		return gVariantNewString(cstr("normal"))
	case "IconThemePath":
		return menuArray("s", nil)
	}
	return gVariantNewString(cstr(""))
}

func (w *window) trayMenuMethod(tr *linuxTray, method string, params, invocation uintptr) {
	m := &tr.menu
	switch method {
	case "GetLayout":
		parent, depth := variantInt32(params, 0), variantInt32(params, 1)
		if _, _, ok := m.node(parent); !ok {
			trayError(invocation, "org.freedesktop.DBus.Error.InvalidArgs", "no such menu item")
			return
		}
		children := []uintptr{gVariantNewUint32(m.revision), m.layout(parent, depth)}
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&children[0], uintptr(len(children))))
	case "GetGroupProperties":
		ids := variantInt32s(params, 0)
		if len(ids) == 0 {
			for id := range m.items {
				ids = append(ids, id)
			}
			slices.Sort(ids)
		}
		values := make([]uintptr, 0, len(ids))
		for _, id := range ids {
			if entry, children, ok := m.node(id); ok {
				item := []uintptr{gVariantNewInt32(id), menuEntryProperties(entry, len(children) > 0)}
				values = append(values, gVariantNewTuple(&item[0], uintptr(len(item))))
			}
		}
		v := menuArray("(ia{sv})", values)
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&v, 1))
	case "GetProperty":
		entry, children, ok := m.node(variantInt32(params, 0))
		var value uintptr
		if ok {
			value = itemProperty(entry, len(children) > 0, variantString(params, 1))
		}
		if value == 0 {
			trayError(invocation, "org.freedesktop.DBus.Error.InvalidArgs", "no such menu item property")
			return
		}
		v := gVariantNewVariant(value)
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&v, 1))
	case "Event":
		id := variantInt32(params, 0)
		switch variantString(params, 1) {
		case "clicked":
			if entry, _, ok := m.node(id); ok && entry != nil && !entry.Disabled {
				if entry.ID != 0 {
					w.engine.Dispatch(driver.Event{Kind: driver.EventMenu, Count: entry.ID})
				} else if entry.Role == driver.MenuQuit {
					w.requestQuit()
				}
			}
			w.trayMenuClosed(tr)
		case "closed":
			// Qt hosts close the menu before they report the click; core still finds the chosen item.
			if id == 0 {
				w.trayMenuClosed(tr)
			}
		}
		returnTray(invocation)
	case "AboutToShow":
		id := variantInt32(params, 0)
		if _, _, ok := m.node(id); !ok {
			trayError(invocation, "org.freedesktop.DBus.Error.InvalidArgs", "no such menu item")
			return
		}
		if id != 0 {
			v := gVariantNewBoolean(0)
			gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&v, 1))
			return
		}
		// The app builds its menu as it is shown: the answer waits for it.
		tr.asked = append(tr.asked, invocation)
		w.askTrayMenu(tr)
	case "AboutToShowGroup":
		var errs []uintptr
		for _, id := range variantInt32s(params, 0) {
			if id == 0 {
				w.askTrayMenu(tr)
			} else if _, _, ok := m.node(id); !ok {
				errs = append(errs, gVariantNewInt32(id))
			}
		}
		values := []uintptr{menuArray("i", nil), menuArray("i", errs)}
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&values[0], uintptr(len(values))))
	default:
		returnTray(invocation)
	}
}

// askTrayMenu has the app build the tray's menu, as a secondary click does, unless it is building it already.
func (w *window) askTrayMenu(tr *linuxTray) {
	if tr.waitID != 0 {
		return
	}
	tr.waitID = gTimeoutAdd(trayMenuWait, cbTrayMenuWait, w.handle)
	w.engine.Dispatch(driver.Event{Kind: driver.EventTray, Count: tr.cfg.ID, Index: 1})
}

// trayMenuClosed reports the tray's menu closed, once.
func (w *window) trayMenuClosed(tr *linuxTray) {
	if tr.popupID != 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: tr.popupID})
		tr.popupID = 0
	}
}

// layout is the (ia{sv}av) of the item id with its children depth levels down, all of them for a negative depth.
func (m *trayMenu) layout(id int32, depth int32) uintptr {
	entry, children, _ := m.node(id)
	var values []uintptr
	if depth != 0 {
		values = make([]uintptr, len(children))
		for i, child := range children {
			values[i] = gVariantNewVariant(m.layout(child, depth-1))
		}
	}
	node := []uintptr{gVariantNewInt32(id), menuEntryProperties(entry, len(children) > 0), menuArray("v", values)}
	return gVariantNewTuple(&node[0], uintptr(len(node)))
}

// menuEntryProperties is the a{sv} of an entry's properties, nil for the root.
func menuEntryProperties(entry *driver.MenuEntry, parent bool) uintptr {
	props := []uintptr{}
	for _, name := range itemProperties {
		if v := itemProperty(entry, parent, name); v != 0 {
			props = append(props, menuDictEntry(name, v))
		}
	}
	return menuDict(props)
}

// variantInt32 is the int32 child i of a tuple.
func variantInt32(tuple uintptr, i int) int32 {
	child := gVariantGetChildValue(tuple, uintptr(i))
	defer gVariantUnref(child)
	return gVariantGetInt32(child)
}

// variantInt32s is the ai child i of a tuple.
func variantInt32s(tuple uintptr, i int) []int32 {
	array := gVariantGetChildValue(tuple, uintptr(i))
	defer gVariantUnref(array)
	out := make([]int32, gVariantNChildren(array))
	for j := range out {
		out[j] = variantInt32(array, j)
	}
	return out
}

func menuDictEntry(key string, value uintptr) uintptr {
	return gVariantNewDictEntry(gVariantNewString(cstr(key)), gVariantNewVariant(value))
}

func menuDict(entries []uintptr) uintptr {
	return menuArray("{sv}", entries)
}

func menuArray(signature string, values []uintptr) uintptr {
	t := gVariantTypeNew(cstr(signature))
	var ptr *uintptr
	if len(values) != 0 {
		ptr = &values[0]
	}
	v := gVariantNewArray(t, ptr, uintptr(len(values)))
	gVariantTypeFree(t)
	return v
}

// trayIcon converts the PNG in TrayConfig to the StatusNotifierItem's pixmap: network-order ARGB32 with straight alpha, at most trayIconSize pixels on its longer side. It returns nil without a PNG.
func trayIcon(data []byte) (pix []byte, w, h int) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Empty() {
		return nil, 0, 0
	}
	b := img.Bounds()
	w, h = b.Dx(), b.Dy()
	if long := max(w, h); long > trayIconSize {
		w, h = max(1, w*trayIconSize/long), max(1, h*trayIconSize/long)
	}
	pix = make([]byte, w*h*4)
	for y := range h {
		sy := b.Min.Y + y*b.Dy()/h
		for x := range w {
			sx := b.Min.X + x*b.Dx()/w
			c := color.NRGBAModel.Convert(img.At(sx, sy)).(color.NRGBA)
			i := (y*w + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = c.A, c.R, c.G, c.B
		}
	}
	return pix, w, h
}

// pixmapVariant is the tray's a(iiay) pixmap; an empty array lets a host use IconName.
func (tr *linuxTray) pixmapVariant() uintptr {
	if tr.pixmap == nil {
		return menuArray("(iiay)", nil)
	}
	byteType := gVariantTypeNew(cstr("y"))
	bytesValue := gVariantNewFixedArray(byteType, unsafe.Pointer(&tr.pixmap[0]), uintptr(len(tr.pixmap)), 1)
	gVariantTypeFree(byteType)
	children := []uintptr{gVariantNewInt32(int32(tr.pixW)), gVariantNewInt32(int32(tr.pixH)), bytesValue}
	item := gVariantNewTuple(&children[0], uintptr(len(children)))
	return menuArray("(iiay)", []uintptr{item})
}
