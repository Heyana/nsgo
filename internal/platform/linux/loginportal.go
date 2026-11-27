//go:build linux && !android

package linux

import (
	"errors"
	"log"
	"os"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const backgroundPortal = "org.freedesktop.portal.Background"

type portalLoginCommand struct {
	owner   *window
	id      uint32
	set, on bool
}

// portalLogin serializes requests from all surfaces. The Background portal has
// no read-only autostart query: known is only this run's last confirmed reply,
// never a persisted preference or a guess from background-consent permissions.
type portalLogin struct {
	root           *window
	commands       []portalLoginCommand
	busy           bool
	known, enabled bool
	bus            uintptr
	path           string
	sub            uint32
	serial         uint64
	watch          uint32
	parentOwner    *window
	parentFallback func()
	closed         atomic.Bool
}

var (
	cbLoginPortalResponse uintptr
	cbLoginPortalOwner    uintptr
)

func init() {
	cbLoginPortalResponse = newCallback(loginPortalResponse)
	cbLoginPortalOwner = newCallback(loginPortalOwnerChanged)
}

func (w *window) launchAtLoginItem(it *driver.MountItem) {
	if InstalledBy() != "Flatpak" {
		if it.Kind == driver.ItemSetLaunchAtLogin {
			setLaunchAtLogin(it.On, w.opts.ID, w.opts.Name)
		} else {
			w.engine.Dispatch(driver.Event{Kind: driver.EventLaunchAtLogin, Count: it.Count, Finished: launchesAtLogin(w.opts.ID, w.opts.Name)})
		}
		return
	}
	root := w
	if root.opener != nil {
		root = root.opener
	}
	if root.login == nil {
		root.login = &portalLogin{root: root}
	}
	root.login.commands = append(root.login.commands, portalLoginCommand{owner: w, id: it.Count, set: it.Kind == driver.ItemSetLaunchAtLogin, on: it.On})
	root.login.next()
}

func (c *portalLogin) next() {
	if c.busy || c.closed.Load() {
		return
	}
	for len(c.commands) > 0 && !c.commands[0].set {
		r := c.commands[0]
		c.commands = c.commands[1:]
		if !r.owner.closing && r.owner.engine != nil {
			r.owner.engine.Dispatch(driver.Event{Kind: driver.EventLaunchAtLogin, Count: r.id, Finished: c.known && c.enabled})
		}
	}
	if len(c.commands) == 0 {
		return
	}
	c.busy = true
	c.serial++
	serial := c.serial
	r := c.commands[0]
	bindPermissions()
	bindSession()
	appID := currentLinuxSandbox(c.root.opts.ID).appID
	sent := false
	send := func(parent string) {
		if sent {
			return
		}
		sent = true
		c.parentOwner, c.parentFallback = nil, nil
		if c.closed.Load() {
			return
		}
		portalQueue() <- func() {
			if c.closed.Load() {
				return
			}
			bus := portalBus(appID)
			if bus == 0 {
				c.fail(serial, errors.New("no session bus"))
				return
			}
			executable, err := os.Executable()
			if err != nil {
				c.fail(serial, err)
				return
			}
			token := portalToken("nsgo_login")
			path := portalRequestPath(bus, token)
			c.root.post(func() {
				if c.closed.Load() {
					return
				}
				c.bus, c.path = bus, path
				c.subscribe()
				if c.sub == 0 {
					c.finish(serial, errors.New("could not subscribe to the Background portal"))
					return
				}
				if c.watch == 0 {
					c.watch = gDBusConnectionSignalSubscribe(bus, cstr("org.freedesktop.DBus"), cstr("org.freedesktop.DBus"), cstr("NameOwnerChanged"), cstr("/org/freedesktop/DBus"), cstr(portalDesktopName), 0, cbLoginPortalOwner, c.root.handle, 0)
				}
				portalQueue() <- func() {
					if c.closed.Load() {
						return
					}
					options := menuDict([]uintptr{
						menuDictEntry("handle_token", gVariantNewString(cstr(token))),
						menuDictEntry("autostart", gVariantNewBoolean(boolInt(r.on))),
						menuDictEntry("commandline", menuArray("s", []uintptr{gVariantNewString(cstr(executable)), gVariantNewString(cstr("--nsgo-launched-at-login"))})),
					})
					reply, err := portalTryTimeout(bus, portalDesktopName, portalDesktopPath, backgroundPortal, "RequestBackground", 10000,
						gVariantNewString(cstr(parent)), options)
					if err != nil || reply == 0 {
						if err == nil {
							err = errors.New("empty Background portal reply")
						}
						c.fail(serial, err)
						return
					}
					actual := portalTupleText(reply, 0)
					gVariantUnref(reply)
					c.root.post(func() {
						if c.closed.Load() || !c.busy || c.serial != serial || c.path != path || actual == path {
							return
						}
						if actual == "" {
							c.finish(serial, errors.New("no Background portal request handle"))
							return
						}
						gDBusConnectionSignalUnsubscribe(c.bus, c.sub)
						c.path = actual
						c.subscribe()
						if c.sub == 0 {
							c.finish(serial, errors.New("could not subscribe to the Background portal"))
						}
					})
				}
			})
		}
	}
	if r.owner == nil || r.owner.closing {
		send("")
		return
	}
	c.parentOwner = r.owner
	c.parentFallback = func() { send("") }
	r.owner.withPortalParent(send)
}

func (c *portalLogin) subscribe() {
	c.sub = gDBusConnectionSignalSubscribe(c.bus, cstr(portalDesktopName), cstr(portalRequestIface), cstr("Response"),
		cstr(c.path), nil, 0, cbLoginPortalResponse, c.root.handle, 0)
}

func (c *portalLogin) fail(serial uint64, err error) {
	c.root.post(func() {
		if !c.closed.Load() {
			c.finish(serial, err)
		}
	})
}

func (c *portalLogin) finish(serial uint64, err error) {
	if !c.busy || c.serial != serial || len(c.commands) == 0 {
		return
	}
	if c.sub != 0 {
		gDBusConnectionSignalUnsubscribe(c.bus, c.sub)
	}
	c.sub, c.path = 0, ""
	if err != nil {
		log.Printf("nsgo: launch at login: %v", err)
	}
	c.commands = c.commands[1:]
	c.busy = false
	c.next()
}

func loginPortalResponse(conn, sender, path, iface, signal, params, data uintptr) {
	root := windowOf(data)
	if root == nil || root.login == nil {
		return
	}
	c := root.login
	if c.closed.Load() || !c.busy || goString(path) != c.path {
		return
	}
	var err error
	if portalTupleUint32(params, 0) != 0 {
		err = errors.New("Background portal request was not granted")
	} else {
		results := gVariantGetChildValue(params, 1)
		value := gVariantLookupValue(results, cstr("autostart"), 0)
		gVariantUnref(results)
		if value == 0 || goString(gVariantGetTypeString(value)) != "b" {
			err = errors.New("Background portal returned no autostart state")
		} else {
			c.enabled, c.known = permVariantGetBool(value) != 0, true
		}
		gVariantUnrefNonZero(value)
	}
	c.finish(c.serial, err)
}

func (c *portalLogin) ownerClosed(w *window) {
	if c == nil || c.closed.Load() || c.parentOwner != w || c.parentFallback == nil {
		return
	}
	c.parentFallback()
}

func (c *portalLogin) close() {
	if c == nil {
		return
	}
	c.closed.Store(true)
	c.parentOwner, c.parentFallback = nil, nil
	if c.sub != 0 {
		gDBusConnectionSignalUnsubscribe(c.bus, c.sub)
	}
	if c.watch != 0 {
		gDBusConnectionSignalUnsubscribe(c.bus, c.watch)
	}
	if c.bus != 0 && c.path != "" {
		sessionCall(c.bus, c.path, portalRequestIface, "Close")
	}
	c.commands = nil
}

func loginPortalOwnerChanged(conn, sender, path, iface, signal, params, data uintptr) {
	root := windowOf(data)
	if root == nil || root.login == nil || variantString(params, 1) == "" {
		return
	}
	c := root.login
	if c.closed.Load() {
		return
	}
	c.known = false
	c.finish(c.serial, errors.New("Background portal disappeared"))
}
