//go:build linux && !android

package linux

import (
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// TestBackgroundLoginPortal uses actual GDBus connections and a private portal
// service. It tests the protocol and queue, without changing login preferences
// or requiring a compositor's consent UI or an installed Flatpak.
func TestBackgroundLoginPortal(t *testing.T) {
	if os.Getenv("NSGO_TEST_LOGIN_PORTAL") == "" {
		if _, err := exec.LookPath("dbus-run-session"); err != nil {
			t.Skip(err)
		}
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("dbus-run-session", "--", executable, "-test.run=^TestBackgroundLoginPortal$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_LOGIN_PORTAL=1", "XDG_DATA_HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "GIO_USE_VFS=local")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("private Background portal: %v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP") {
			t.Skipf("native libraries unavailable: %s", out)
		}
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	trayBind()
	bindPermissions()
	const schema = `<node>
<interface name="org.freedesktop.portal.Background"><method name="RequestBackground"><arg type="s" direction="in"/><arg type="a{sv}" direction="in"/><arg type="o" direction="out"/></method></interface>
<interface name="org.freedesktop.host.portal.Registry"><method name="Register"><arg type="s" direction="in"/><arg type="a{sv}" direction="in"/></method></interface>
<interface name="org.freedesktop.portal.Request"><method name="Close"/><signal name="Response"><arg type="u"/><arg type="a{sv}"/></signal></interface>
</node>`
	var nativeError uintptr
	info := gDBusNodeInfoNewForXML(cstr(schema), &nativeError)
	if info == 0 {
		t.Fatal("could not parse portal schema")
	}
	server := trayConnection()
	if server == 0 {
		t.Fatal("no private bus")
	}
	defer closeTrayConnection(server)
	type request struct {
		path    string
		on      bool
		command []string
	}
	var requests []request
	var registrations []uint32
	closed := 0
	var vtable gDBusInterfaceVTable
	response := func(path, results string) {
		params := portalVariant("(ua{sv})", "(uint32 0, "+results+")")
		var err uintptr
		gDBusConnectionEmitSignal(server, nil, cstr(path), cstr(portalRequestIface), cstr("Response"), params, &err)
		if err != 0 {
			gErrorFree(err)
			t.Error("could not emit portal response")
		}
	}
	method := newCallback(func(conn, sender, path, iface, name, params, invocation, data uintptr) {
		switch goString(name) {
		case "Register":
			returnTray(invocation)
		case "Close":
			closed++
			returnTray(invocation)
		case "RequestBackground":
			options := gVariantGetChildValue(params, 1)
			token := portalLookupText(options, "handle_token")
			value := gVariantLookupValue(options, cstr("autostart"), 0)
			on := permVariantGetBool(value) != 0
			gVariantUnref(value)
			command := gVariantLookupValue(options, cstr("commandline"), 0)
			args := variantStringArray(command)
			gVariantUnref(command)
			gVariantUnref(options)
			path := portalRequestPathForSender(goString(sender), token)
			requests = append(requests, request{path, on, args})
			registration := gDBusConnectionRegisterObject(server, cstr(path), gDBusNodeInfoLookupInterface(info, cstr(portalRequestIface)), &vtable, 0, 0, &nativeError)
			registrations = append(registrations, registration)
			if len(requests) == 2 {
				response(path, "{'autostart': <false>}")
			}
			if len(requests) == 3 {
				response(path, "{'autostart': <'wrong-type'>}")
			}
			value = gVariantNewObjectPath(cstr(path))
			gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&value, 1))
		}
	})
	vtable.MethodCall = method
	for _, iface := range []string{backgroundPortal, globalRegistryIface} {
		registration := gDBusConnectionRegisterObject(server, cstr(globalShortcutPath), gDBusNodeInfoLookupInterface(info, cstr(iface)), &vtable, 0, 0, &nativeError)
		if registration == 0 {
			t.Fatal("could not export fake portal")
		}
		registrations = append(registrations, registration)
	}
	defer func() {
		for _, registration := range registrations {
			gDBusConnectionUnregisterObject(server, registration)
		}
	}()
	owned := false
	acquired := newCallback(func(conn, name, data uintptr) { owned = true })
	name := gBusOwnNameOnConnection(server, cstr(portalDesktopName), 0, acquired, 0, 0, 0)
	defer func() {
		if name != 0 {
			gBusUnownName(name)
		}
	}()
	wait := func(test func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
			for gMainContextIteration(0, 0) != 0 {
			}
			if test() {
				return
			}
			if time.Now().After(end) {
				t.Fatal("portal did not reach expected state")
			}
		}
	}
	wait(func() bool { return owned })
	var events []driver.Event
	e := engine.Start(portalTestPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{Event: func(_ uint32, ev driver.Event) {
		if ev.Kind == driver.EventLaunchAtLogin {
			events = append(events, ev)
		}
	}})
	defer e.Close()
	w := &window{engine: e, opts: Options{ID: "dev.nsgo.loginfixture"}}
	w.handle = registerWindow(w)
	defer windowHandles.Delete(w.handle)
	c := &portalLogin{root: w}
	w.login = c
	defer func() { w.login.close() }()
	get := func(id uint32) portalLoginCommand { return portalLoginCommand{owner: w, id: id} }
	set := func(on bool) portalLoginCommand { return portalLoginCommand{owner: w, set: true, on: on} }
	c.commands = []portalLoginCommand{get(1), set(true), get(2), set(false), get(3)}
	c.next()
	wait(func() bool { return len(requests) == 1 })
	if len(events) != 1 || events[0].Finished {
		t.Fatalf("cold read or pending read changed state: %v", events)
	}
	executable, _ := os.Executable()
	if !requests[0].on || !reflect.DeepEqual(requests[0].command, []string{executable, "--nsgo-launched-at-login"}) {
		t.Fatalf("Background options %+v", requests[0])
	}
	response(requests[0].path, "{'autostart': <true>}")
	wait(func() bool { return len(events) == 3 })
	if !events[1].Finished || events[2].Finished || requests[1].on {
		t.Fatalf("serialized replies %v, requests %+v", events, requests)
	}
	// Stale responses and malformed booleans must not enable startup.
	response(requests[0].path, "{'autostart': <true>}")
	c.commands = append(c.commands, set(true), get(4))
	c.next()
	wait(func() bool { return len(events) == 4 })
	if events[3].Finished || c.enabled {
		t.Fatal("malformed or stale response enabled startup")
	}
	c.commands = append(c.commands, set(true), get(5))
	c.next()
	wait(func() bool { return len(requests) == 4 })
	c.close()
	wait(func() bool { return closed == 1 })
	response(requests[3].path, "{'autostart': <true>}")
	for gMainContextIteration(0, 0) != 0 {
	}
	if len(events) != 4 {
		t.Fatal("a read survived its app closing")
	}
	// Losing the portal while awaiting a grant finishes pending reads, with
	// no claim that an old reply is the current system setting.
	c = &portalLogin{root: w}
	w.login = c
	c.commands = []portalLoginCommand{set(true), get(6)}
	c.next()
	wait(func() bool { return len(requests) == 5 })
	gBusUnownName(name)
	name = 0
	wait(func() bool { return len(events) == 5 })
	if events[4].Finished || c.known {
		t.Fatal("a vanished portal left an enabled state")
	}
}
