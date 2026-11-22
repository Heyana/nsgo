//go:build linux && !android

package linux

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
)

// TestLauncherQuicklistOverDBus exercises the actual exported object, including
// a menu owned by a child surface and stale events after another surface sets it.
func TestLauncherQuicklistOverDBus(t *testing.T) {
	if testing.Short() || os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("needs GTK and a session bus")
	}
	var rootScope, childScope *core.Scope
	var child *core.Window
	var childClicks, rootClicks atomic.Int32
	ready := make(chan struct{})
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		rootScope = cx
		child = cx.OpenWindow(core.WindowOptions{Title: "quicklist owner", Hidden: true}, func(cx *core.Scope) core.Node {
			childScope = cx
			cx.SetDockMenu([]core.MenuItem{
				{Label: "Open_1", Checked: true, OnSelect: func() { childClicks.Add(1) }},
				{Separator: true},
				{Label: "More", Items: []core.MenuItem{{Label: "Child", OnSelect: func() { childClicks.Add(1) }}}},
				{Label: "Disabled", Disabled: true, OnSelect: func() { childClicks.Add(100) }},
			})
			return core.View{}
		})
		close(ready)
	})
	w := startTestWindow(t, app, Options{Title: "nsgo-quicklist", ID: "dev.nsgo.quicklisttest", Width: 120, Height: 80, Hidden: true})
	defer w.onLoop(t, w.close)
	<-ready
	wait := func(test func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			if test() {
				return
			}
			if time.Now().After(end) {
				t.Fatal("quicklist did not reach expected state")
			}
		}
	}
	var path, sender string
	var old, nested, disabled int32
	var revision uint32
	wait(func() bool {
		found := false
		w.onLoop(t, func() {
			m := w.dockMenu
			if m == nil || len(m.menu.root) != 4 {
				return
			}
			var getName func(uintptr) uintptr
			purego.RegisterLibFunc(&getName, gioLib, "g_dbus_connection_get_unique_name")
			path, sender = m.path, goString(getName(m.bus))
			old, nested, disabled = m.menu.root[0], m.menu.items[m.menu.root[2]].children[0], m.menu.root[3]
			revision = m.menu.revision
			found = true
		})
		return found
	})
	// A launcher that appears after the app must receive the current quicklist.
	type update struct{ sender, uri, path string }
	updates := make(chan update, 16)
	callback := newCallback(func(conn, sender, path, iface, signal, params, data uintptr) {
		uri := variantString(params, 0)
		properties := gVariantGetChildValue(params, 1)
		defer gVariantUnref(properties)
		for i := uintptr(0); i < gVariantNChildren(properties); i++ {
			item := gVariantGetChildValue(properties, i)
			if variantString(item, 0) == "quicklist" {
				wrapped := gVariantGetChildValue(item, 1)
				value := gVariantGetVariant(wrapped)
				if got := goString(gVariantGetTypeString(value)); got != "s" {
					t.Errorf("quicklist wire type %q, want s", got)
				}
				u := update{goString(sender), uri, goString(gVariantGetString(value, 0))}
				select {
				case updates <- u:
				default:
				}
				gVariantUnref(value)
				gVariantUnref(wrapped)
			}
			gVariantUnref(item)
		}
	})
	var launcher uintptr
	var name, subscription uint32
	w.onLoop(t, func() {
		launcher = trayConnection()
		subscription = gDBusConnectionSignalSubscribe(launcher, nil, cstr("com.canonical.Unity.LauncherEntry"), cstr("Update"), nil, nil, 0, callback, 0, 0)
		name = gBusOwnNameOnConnection(launcher, cstr("com.canonical.Unity"), 0, 0, 0, 0, 0)
	})
	defer w.onLoop(t, func() {
		gDBusConnectionSignalUnsubscribe(launcher, subscription)
		gBusUnownName(name)
		closeTrayConnection(launcher)
	})
	select {
	case u := <-updates:
		if u.sender != sender || u.uri != launcherEntryURI(w.opts.ID) || u.path != path {
			t.Fatalf("launcher update %+v", u)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a late launcher received no quicklist")
	}
	bus := sessionBus()
	call := func(method string, args ...uintptr) uintptr {
		t.Helper()
		reply := dbusCall(bus, sender, path, trayMenuInterface, method, args...)
		if reply == 0 {
			t.Fatalf("quicklist %s failed", method)
		}
		return reply
	}
	click := func(id int32) {
		t.Helper()
		gVariantUnref(call("Event", gVariantNewInt32(id), gVariantNewString(cstr("clicked")), gVariantNewVariant(gVariantNewInt32(0)), gVariantNewUint32(0)))
	}
	property := func(id int32, name string) uintptr {
		t.Helper()
		reply := call("GetProperty", gVariantNewInt32(id), gVariantNewString(cstr(name)))
		defer gVariantUnref(reply)
		v := gVariantGetChildValue(reply, 0)
		defer gVariantUnref(v)
		return gVariantGetVariant(v)
	}
	label := property(old, "label")
	if got := goString(gVariantGetString(label, 0)); got != "Open__1" {
		t.Errorf("label %q", got)
	}
	gVariantUnref(label)
	checked := property(old, "toggle-state")
	if got := gVariantGetInt32(checked); got != 1 {
		t.Errorf("toggle-state %d", got)
	}
	gVariantUnref(checked)
	enabled := property(disabled, "enabled")
	if got := gVariantGetBoolean(enabled); got != 0 {
		t.Error("disabled item enabled")
	}
	gVariantUnref(enabled)
	reply := call("GetLayout", gVariantNewInt32(0), gVariantNewInt32(-1), menuArray("s", nil))
	layout := gVariantGetChildValue(reply, 1)
	children := gVariantGetChildValue(layout, 2)
	if got := gVariantNChildren(children); got != 4 {
		t.Errorf("layout has %d root children", got)
	}
	gVariantUnref(children)
	gVariantUnref(layout)
	gVariantUnref(reply)
	gVariantUnref(call("AboutToShow", gVariantNewInt32(0)))
	click(old)
	click(nested)
	click(disabled)
	wait(func() bool { return childClicks.Load() == 2 })
	// Exercise a real consumer when installed; libdbusmenu coalesces events
	// through EventGroup, as launcher clients do.
	expectedChildClicks := int32(2)
	if lib, err := purego.Dlopen("libdbusmenu-glib.so.4", purego.RTLD_NOW|purego.RTLD_LOCAL); err == nil {
		var clientNew func(name, path *byte) uintptr
		var clientRoot func(client uintptr) uintptr
		var findID func(root uintptr, id int32) uintptr
		var handleEvent func(item uintptr, name *byte, value uintptr, timestamp uint32)
		purego.RegisterLibFunc(&clientNew, lib, "dbusmenu_client_new")
		purego.RegisterLibFunc(&clientRoot, lib, "dbusmenu_client_get_root")
		purego.RegisterLibFunc(&findID, lib, "dbusmenu_menuitem_find_id")
		purego.RegisterLibFunc(&handleEvent, lib, "dbusmenu_menuitem_handle_event")
		var client, item uintptr
		w.onLoop(t, func() { client = clientNew(cstr(sender), cstr(path)) })
		wait(func() bool {
			w.onLoop(t, func() {
				if root := clientRoot(client); root != 0 {
					item = findID(root, nested)
				}
			})
			return item != 0
		})
		w.onLoop(t, func() { handleEvent(item, cstr("clicked"), 0, 0) })
		expectedChildClicks++
		wait(func() bool { return childClicks.Load() == expectedChildClicks })
		w.onLoop(t, func() { gObjectUnref(client) })
		// The library installs process-wide GTypes, so its code must stay loaded.
	} else {
		t.Log("libdbusmenu client unavailable; raw protocol assertions still ran")
	}

	rootScope.Post(func() {
		childScope.SetDockMenu([]core.MenuItem{{Label: "Superseded", OnSelect: func() { childClicks.Add(100) }}})
		rootScope.SetDockMenu([]core.MenuItem{{Label: "Root", OnSelect: func() { rootClicks.Add(1) }}})
	})
	var replacement int32
	wait(func() bool {
		found := false
		w.onLoop(t, func() {
			if m := w.dockMenu; m.menu.revision > revision && len(m.menu.root) == 1 {
				replacement, found = m.menu.root[0], true
			}
		})
		return found
	})
	click(old)
	click(nested)
	event := func(id int32) uintptr {
		values := []uintptr{gVariantNewInt32(id), gVariantNewString(cstr("clicked")), gVariantNewVariant(gVariantNewInt32(0)), gVariantNewUint32(0)}
		return gVariantNewTuple(&values[0], 4)
	}
	reply = call("EventGroup", menuArray("(isvu)", []uintptr{event(old), event(replacement)}))
	errors := variantInt32s(reply, 0)
	if len(errors) != 1 || errors[0] != old {
		t.Errorf("EventGroup errors %v", errors)
	}
	gVariantUnref(reply)
	wait(func() bool { return rootClicks.Load() == 1 })
	if got := childClicks.Load(); got != expectedChildClicks {
		t.Errorf("stale selections reached previous owner: %d", got)
	}
	// Closing a former owner must keep the replacement set by the root.
	rootScope.Post(child.Close)
	click(replacement)
	wait(func() bool { return rootClicks.Load() == 2 })
	rootScope.Post(func() { rootScope.SetDockMenu(nil) })
	wait(func() bool {
		empty := false
		w.onLoop(t, func() { empty = len(w.dockMenu.menu.root) == 0 })
		return empty
	})
	click(replacement)
	if replacement <= old {
		t.Error("wire IDs were reset")
	}
	if got := rootClicks.Load(); got != 2 {
		t.Errorf("cleared selection dispatched: %d", got)
	}
	// An owner that closes while its menu is current must clear the native
	// export even when its engine no longer sends its cleanup batch.
	rootScope.Post(func() {
		child = rootScope.OpenWindow(core.WindowOptions{Title: "latest quicklist owner", Hidden: true}, func(cx *core.Scope) core.Node {
			cx.SetDockMenu([]core.MenuItem{{Label: "Latest", OnSelect: func() { childClicks.Add(1) }}})
			return core.View{}
		})
	})
	var latest int32
	wait(func() bool {
		found := false
		w.onLoop(t, func() {
			if m := w.dockMenu; len(m.menu.root) == 1 && m.menu.root[0] > replacement {
				latest, found = m.menu.root[0], true
			}
		})
		return found
	})
	rootScope.Post(func() { child.Close() })
	wait(func() bool {
		empty := false
		w.onLoop(t, func() { empty = len(w.dockMenu.menu.root) == 0 })
		return empty
	})
	click(latest)
	if got := childClicks.Load(); got != expectedChildClicks {
		t.Errorf("closed owner received a selection: %d", got)
	}
	for len(updates) > 0 {
		<-updates
	}
	w.onLoop(t, w.closeDockMenu)
	select {
	case u := <-updates:
		if u.path != "" {
			t.Errorf("closed app quicklist %q", u.path)
		}
	case <-time.After(5 * time.Second):
		t.Error("closed app did not withdraw its quicklist")
	}
}
