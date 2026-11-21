//go:build linux && !android

package linux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
)

func TestLinuxSessionEndForcesQuit(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a GTK window")
	}
	const child = "NSGO_TEST_LINUX_QUIT_PORTAL"
	if os.Getenv(child) == "" {
		if _, err := exec.LookPath("dbus-run-session"); err != nil {
			t.Skip("dbus-run-session is needed for a private test bus")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dbus-run-session", "--", os.Args[0], "-test.run=^"+t.Name()+"$", "-test.timeout=35s", "-test.v")
		cmd.Env = append(os.Environ(), child+"=1")
		// Services activated on the test bus can outlive dbus-run-session.
		// Write to a file so an inherited descriptor cannot hold up Wait.
		log, err := os.CreateTemp(t.TempDir(), "session-*.log")
		if err != nil {
			t.Fatal(err)
		}
		defer log.Close()
		cmd.Stdout, cmd.Stderr = log, log
		runErr := cmd.Run()
		out, err := os.ReadFile(log.Name())
		if err != nil {
			t.Fatal(err)
		}
		if runErr != nil {
			t.Fatalf("%v\n%s", runErr, out)
		}
		if testing.Verbose() {
			t.Logf("%s", out)
		}
		return
	}

	const xml = `<node><interface name="org.freedesktop.portal.Inhibit">
		<method name="CreateMonitor"><arg type="s" direction="in"/><arg type="a{sv}" direction="in"/><arg type="o" direction="out"/></method>
		<method name="QueryEndResponse"><arg type="o" direction="in"/></method>
	</interface></node>`
	created, acknowledged := make(chan struct{}, 1), make(chan struct{}, 1)
	userRequest, cleanup := make(chan struct{}, 1), make(chan bool, 1)
	var forced atomic.Bool
	app := func(cx *core.Scope) core.Node {
		cx.OnQuitRequest(func(q *core.QuitRequest) {
			if !q.Forced() {
				q.Defer()
				userRequest <- struct{}{}
				return
			}
			q.Cancel()
			q.Defer()(false)
			forced.Store(!q.Canceled())
		})
		cx.OnQuit(func(context.Context) { cleanup <- forced.Load() })
		return core.Text{Content: "Session quit"}
	}
	w := startTestWindow(t, app, Options{Title: "Session quit", Width: 200, Height: 100})
	var bus uintptr
	var client, session string
	wait := func(ch <-chan struct{}, what string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
		}
	}
	w.onLoop(t, func() {
		trayBind()
		bindSession()
		bus = sessionBus()
		info := gDBusNodeInfoNewForXML(cstr(xml), nil)
		vtable := &gDBusInterfaceVTable{MethodCall: purego.NewCallback(func(conn, sender, path, iface, method, params, invocation, data uintptr) {
			switch goString(method) {
			case "CreateMonitor":
				client = goString(sender)
				session = w.session.path
				options := gVariantGetChildValue(params, 1)
				if token := portalLookupText(options, "session_handle_token"); token == "" {
					t.Error("CreateMonitor did not send a session handle token")
				}
				gVariantUnref(options)
				request := gVariantNewObjectPath(cstr(w.session.request))
				gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(&request, 1))
				created <- struct{}{}
			case "QueryEndResponse":
				if got := portalTupleText(params, 0); got != session {
					t.Errorf("acknowledged session %q, want %q", got, session)
				}
				returnTray(invocation)
				acknowledged <- struct{}{}
			}
		})}
		i := gDBusNodeInfoLookupInterface(info, cstr(inhibitPortal))
		if gDBusConnectionRegisterObject(bus, cstr(globalShortcutPath), i, vtable, 0, 0, nil) == 0 {
			t.Error("could not register the test portal")
		}
		acquired := purego.NewCallback(func(conn, name, data uintptr) { w.watchSession() })
		gBusOwnNameOnConnection(bus, cstr(globalShortcutName), 0, acquired, 0, 0, 0)
		w.requestQuit()
	})
	t.Cleanup(func() {
		if !forced.Load() {
			w.onLoop(t, w.close)
		}
	})
	wait(created, "CreateMonitor")
	wait(userRequest, "deferred user quit")
	emit := func(handle string, state uint32) {
		w.onLoop(t, func() {
			params := portalVariant("(oa{sv})", fmt.Sprintf("(objectpath %s, {'session-state': <uint32 %d>})", portalQuote(handle), state))
			if params == 0 {
				t.Error("could not create the session signal")
				return
			}
			if gDBusConnectionEmitSignal(bus, cstr(client), cstr(globalShortcutPath), cstr(inhibitPortal), cstr("StateChanged"), params, nil) == 0 {
				t.Error("could not emit the session signal")
			}
		})
	}
	emit(session+"_other", 3)
	emit(session, 2)
	wait(acknowledged, "QueryEndResponse")
	if forced.Load() || len(cleanup) != 0 {
		t.Fatal("Query End or a different session's signal quit the app")
	}
	emit(session, 3)
	select {
	case ok := <-cleanup:
		if !ok {
			t.Fatal("forced notification did not run before cleanup")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Ending did not run cleanup")
	}
}
