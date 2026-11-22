//go:build linux && !android

package linux

import (
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// dbusMenu is a menu as dbusmenu numbers it: every entry has an ID of its own, which later layouts never reuse, so that an event for an entry of an older layout finds none. 0 is the root, whose children root lists.
type dbusMenu struct {
	items    map[int32]*dbusMenuItem
	root     []int32
	last     int32
	revision uint32
}

type dbusMenuItem struct {
	entry    driver.MenuEntry // Items left out: children has them
	children []int32
}

// set numbers entries as the menu's new layout.
func (m *dbusMenu) set(entries []driver.MenuEntry) {
	m.items = map[int32]*dbusMenuItem{}
	m.root = m.add(entries)
	m.revision++
}

func (m *dbusMenu) add(entries []driver.MenuEntry) []int32 {
	ids := make([]int32, len(entries))
	for i, e := range entries {
		m.last++
		id := m.last
		ids[i] = id
		it := &dbusMenuItem{entry: e}
		it.entry.Items = nil
		m.items[id] = it
		it.children = m.add(e.Items)
	}
	return ids
}

// node returns the entry id names, nil for the root, and its children's IDs; ok is false for an ID the layout lacks.
func (m *dbusMenu) node(id int32) (entry *driver.MenuEntry, children []int32, ok bool) {
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

// readMethod answers the read operations shared by tray menus and launcher quicklists.
func (m *dbusMenu) readMethod(method string, params, invocation uintptr) bool {
	switch method {
	case "GetLayout":
		parent, depth := variantInt32(params, 0), variantInt32(params, 1)
		if _, _, ok := m.node(parent); !ok {
			trayError(invocation, "org.freedesktop.DBus.Error.InvalidArgs", "no such menu item")
			return true
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
			return true
		}
		v := gVariantNewVariant(value)
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&v, 1))
	default:
		return false
	}
	return true
}

// layout is the (ia{sv}av) of the item id with its children depth levels down, all of them for a negative depth.
func (m *dbusMenu) layout(id int32, depth int32) uintptr {
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

// eventMethod serves both individual events and the batches libdbusmenu clients send. Unknown IDs are reported by EventGroup instead of reaching old actions.
func (m *dbusMenu) eventMethod(method string, params, invocation uintptr, event func(int32, string)) bool {
	switch method {
	case "Event":
		id := variantInt32(params, 0)
		if _, _, ok := m.node(id); ok {
			event(id, variantString(params, 1))
		}
		returnTray(invocation)
	case "EventGroup":
		events := gVariantGetChildValue(params, 0)
		defer gVariantUnref(events)
		var errors []uintptr
		for i := uintptr(0); i < gVariantNChildren(events); i++ {
			item := gVariantGetChildValue(events, i)
			id := variantInt32(item, 0)
			if _, _, ok := m.node(id); ok {
				event(id, variantString(item, 1))
			} else {
				errors = append(errors, gVariantNewInt32(id))
			}
			gVariantUnref(item)
		}
		value := menuArray("i", errors)
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&value, 1))
	default:
		return false
	}
	return true
}
