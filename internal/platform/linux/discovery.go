//go:build linux && !android

package linux

import (
	"maps"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	avahiName        = "org.freedesktop.Avahi"
	avahiServer      = avahiName + ".Server"
	avahiServer2     = avahiName + ".Server2"
	avahiEntryGroup  = avahiName + ".EntryGroup"
	avahiBrowser     = avahiName + ".ServiceBrowser"
	avahiLocal       = "local"
	avahiUnspecified = -1
	avahiNoCookie    = 32 // AVAHI_PUBLISH_NO_COOKIE
	avahiMulticast   = 2  // AVAHI_LOOKUP_USE_MULTICAST
	avahiNoAddress   = 8  // AVAHI_LOOKUP_NO_ADDRESS: DNS-SD reports the hostname
)

// avahiDiscovery owns the daemon objects of a surface. GDBus calls and
// subscriptions complete on GTK's main context, where the mount items run.
type avahiDiscovery struct {
	live map[uint32]*avahiOperation
}

type avahiOperation struct {
	owner    *avahiDiscovery
	dispatch func(driver.Event)
	id       uint32
	token    uintptr
	watch    uint32

	bus          uintptr // held by the name watch
	daemon       string  // unique bus name, so a restart cannot reuse our paths
	path, iface  string
	subscription uint32
	request      driver.DiscoveryRequest
	instances    map[avahiInstance]*driver.LocalService
	visible      map[avahiEndpoint]int
}

type avahiInstance struct {
	index, protocol   int32
	name, typ, domain string
}

type avahiEndpoint struct {
	name, typ, host string
	port            int
}

type avahiReply func(bus, reply uintptr, message string)

var (
	gBusWatchName             func(busType int32, name *byte, flags uint32, appeared, vanished, data, destroy uintptr) uint32
	gBusUnwatchName           func(watcher uint32)
	gDBusConnectionCallFinish func(bus, result uintptr, err *uintptr) uintptr
	gVariantNewUint16         func(uint16) uintptr
	gVariantGetUint16         func(uintptr) uint16

	avahiToken      uintptr
	avahiOperations = map[uintptr]*avahiOperation{}
	avahiReplies    = map[uintptr]avahiReply{}
	avahiAppeared   uintptr
	avahiVanished   uintptr
	avahiSignal     uintptr
	avahiCallReady  uintptr

	bindAvahi = sync.OnceFunc(func() {
		bindSession()
		bindSecrets()
		purego.RegisterLibFunc(&gBusWatchName, gioLib, "g_bus_watch_name")
		purego.RegisterLibFunc(&gBusUnwatchName, gioLib, "g_bus_unwatch_name")
		purego.RegisterLibFunc(&gDBusConnectionCallFinish, gioLib, "g_dbus_connection_call_finish")
		purego.RegisterLibFunc(&gVariantNewUint16, glibLib, "g_variant_new_uint16")
		purego.RegisterLibFunc(&gVariantGetUint16, glibLib, "g_variant_get_uint16")
		avahiAppeared = newCallback(avahiNameAppeared)
		avahiVanished = newCallback(avahiNameVanished)
		avahiSignal = newCallback(avahiObjectSignal)
		avahiCallReady = newCallback(avahiCallFinished)
	})
)

func (d *avahiDiscovery) handle(item *driver.MountItem, dispatch func(driver.Event)) {
	if item.Discovery == nil {
		return
	}
	id, req := item.Count, *item.Discovery
	d.stop(id)
	if req.Op != driver.DiscoveryAdvertise && req.Op != driver.DiscoveryBrowse {
		return
	}
	bindAvahi()
	avahiToken++
	op := &avahiOperation{owner: d, dispatch: dispatch, id: id, token: avahiToken, request: req}
	if d.live == nil {
		d.live = map[uint32]*avahiOperation{}
	}
	d.live[id], avahiOperations[op.token] = op, op
	const systemBus = 1
	op.watch = gBusWatchName(systemBus, cstr(avahiName), 0, avahiAppeared, avahiVanished, op.token, 0)
}

func (d *avahiDiscovery) stop(id uint32) {
	op := d.live[id]
	if op == nil {
		return
	}
	delete(d.live, id)
	delete(avahiOperations, op.token)
	if op.subscription != 0 {
		gDBusConnectionSignalUnsubscribe(op.bus, op.subscription)
		avahiCall(op.bus, op.daemon, op.path, op.iface, "Free", "", nil)
	}
	gBusUnwatchName(op.watch)
}

func (d *avahiDiscovery) close() {
	for id := range d.live {
		d.stop(id)
	}
}

func (op *avahiOperation) active() bool { return op.owner.live[op.id] == op }

func (op *avahiOperation) report(reply driver.DiscoveryReply) {
	reply.Service.TXT = maps.Clone(reply.Service.TXT)
	op.dispatch(driver.Event{Kind: driver.EventDiscovery, Count: op.id, Data: &reply})
}

func (op *avahiOperation) fail(message string) {
	op.owner.stop(op.id)
	op.report(driver.DiscoveryReply{Error: "nsgo: Avahi: " + message})
}

func avahiNameAppeared(bus, _, owner, token uintptr) {
	op := avahiOperations[token]
	if op == nil {
		return
	}
	op.bus, op.daemon = bus, goString(owner)
	if op.request.Op == driver.DiscoveryAdvertise {
		op.iface = avahiEntryGroup
		op.create(avahiServer, "EntryGroupNew", op.advertise)
		return
	}
	op.iface = avahiBrowser
	op.instances = map[avahiInstance]*driver.LocalService{}
	op.visible = map[avahiEndpoint]int{}
	// Prepare, subscribe, then start so cached entries cannot arrive before
	// the receiver is listening.
	op.create(avahiServer2, "ServiceBrowserPrepare", op.browse, gVariantNewInt32(avahiUnspecified),
		gVariantNewInt32(avahiUnspecified), gVariantNewString(cstr(op.request.Service.Type)),
		gVariantNewString(cstr(avahiLocal)), gVariantNewUint32(avahiMulticast))
}

func avahiNameVanished(_, _ uintptr, token uintptr) {
	if op := avahiOperations[token]; op != nil {
		op.fail("local service discovery unavailable")
	}
}

func (op *avahiOperation) create(iface, method string, start func(), args ...uintptr) {
	avahiCall(op.bus, op.daemon, "/", iface, method, "(o)", func(bus, reply uintptr, message string) {
		if message != "" {
			if op.active() {
				op.fail(message)
			}
			return
		}
		pathValue := gVariantGetChildValue(reply, 0)
		path := goString(gVariantGetString(pathValue, 0))
		gVariantUnref(pathValue)
		if !op.active() {
			// A stop can arrive while the daemon is allocating the object.
			avahiCall(bus, op.daemon, path, op.iface, "Free", "", nil)
			return
		}
		op.path = path
		op.subscription = gDBusConnectionSignalSubscribe(bus, cstr(op.daemon), cstr(op.iface),
			nil, cstr(path), nil, 0, avahiSignal, op.token, 0)
		start()
	}, args...)
}

func (op *avahiOperation) advertise() {
	s := op.request.Service
	txt, err := driver.DiscoveryTXT(s.TXT)
	if err != nil {
		op.fail(err.Error())
		return
	}
	op.command("AddService", func() { op.command("Commit", nil) },
		gVariantNewInt32(avahiUnspecified), gVariantNewInt32(avahiUnspecified), gVariantNewUint32(avahiNoCookie),
		gVariantNewString(cstr(s.Name)), gVariantNewString(cstr(s.Type)), gVariantNewString(cstr(avahiLocal)),
		gVariantNewString(cstr("")), gVariantNewUint16(uint16(s.Port)), avahiTXTVariant(txt))
}

func (op *avahiOperation) browse() { op.command("Start", nil) }

func (op *avahiOperation) command(method string, done func(), args ...uintptr) {
	avahiCall(op.bus, op.daemon, op.path, op.iface, method, "()", func(_, _ uintptr, message string) {
		if !op.active() {
			return
		}
		if message != "" {
			op.fail(message)
			return
		}
		if done != nil {
			done()
		}
	}, args...)
}

func avahiObjectSignal(_, _, _, _, signal, params, token uintptr) {
	op := avahiOperations[token]
	if op == nil {
		return
	}
	name := goString(signal)
	switch name {
	case "StateChanged":
		// AvahiEntryGroupState reports collisions and failures after Commit.
		const collision, failed = 3, 4
		if goString(gVariantGetTypeString(params)) != "(is)" {
			return
		}
		switch variantInt32(params, 0) {
		case collision:
			op.fail("service name is already in use")
		case failed:
			message := variantString(params, 1)
			if message == "" {
				message = "advertisement failed"
			}
			op.fail(message)
		}
	case "Failure":
		if goString(gVariantGetTypeString(params)) == "(s)" {
			op.fail(variantString(params, 0))
		}
	case "ItemNew", "ItemRemove":
		if goString(gVariantGetTypeString(params)) != "(iisssu)" {
			return
		}
		key := avahiInstance{
			index: variantInt32(params, 0), protocol: variantInt32(params, 1),
			name: variantString(params, 2), typ: variantString(params, 3), domain: variantString(params, 4),
		}
		if name == "ItemNew" {
			op.resolve(key)
		} else {
			op.remove(key)
		}
	}
}

func (op *avahiOperation) resolve(key avahiInstance) {
	if op.instances[key] != nil {
		return
	}
	instance := new(driver.LocalService)
	op.instances[key] = instance
	avahiCall(op.bus, op.daemon, "/", avahiServer, "ResolveService", "(iissssisqaayu)",
		func(_, reply uintptr, message string) {
			if !op.active() || op.instances[key] != instance {
				return
			}
			if message != "" {
				delete(op.instances, key)
				return
			}
			portValue := gVariantGetChildValue(reply, 8)
			port := gVariantGetUint16(portValue)
			gVariantUnref(portValue)
			host := strings.TrimSuffix(variantString(reply, 5), ".")
			if host == "" || port == 0 {
				delete(op.instances, key)
				return
			}
			*instance = driver.LocalService{Name: key.name, Type: key.typ, Host: host + ".", Port: int(port), TXT: avahiTXTRecords(reply)}
			endpoint := avahiServiceEndpoint(*instance)
			op.visible[endpoint]++
			if op.visible[endpoint] == 1 {
				op.report(driver.DiscoveryReply{Service: *instance})
			}
		}, gVariantNewInt32(key.index), gVariantNewInt32(key.protocol), gVariantNewString(cstr(key.name)),
		gVariantNewString(cstr(key.typ)), gVariantNewString(cstr(key.domain)), gVariantNewInt32(avahiUnspecified),
		gVariantNewUint32(avahiMulticast|avahiNoAddress))
}

func (op *avahiOperation) remove(key avahiInstance) {
	instance := op.instances[key]
	delete(op.instances, key)
	if instance == nil || instance.Host == "" {
		return
	}
	endpoint := avahiServiceEndpoint(*instance)
	op.visible[endpoint]--
	if op.visible[endpoint] == 0 {
		delete(op.visible, endpoint)
		op.report(driver.DiscoveryReply{Service: *instance, Removed: true})
	}
}

func avahiServiceEndpoint(s driver.LocalService) avahiEndpoint {
	return avahiEndpoint{strings.ToLower(s.Name), strings.ToLower(s.Type), strings.ToLower(s.Host), s.Port}
}

// Avahi represents TXT data as an array of byte arrays. The driver uses
// the DNS wire representation so its key and value rules stay shared.
func avahiTXTVariant(txt []byte) uintptr {
	byteType := gVariantTypeNew(cstr("y"))
	defer gVariantTypeFree(byteType)
	var records []uintptr
	for len(txt) != 0 {
		size := int(txt[0])
		txt = txt[1:]
		records = append(records, gVariantNewFixedArray(byteType, unsafe.Pointer(unsafe.SliceData(txt)), uintptr(size), 1))
		txt = txt[size:]
	}
	recordType := gVariantTypeNew(cstr("ay"))
	defer gVariantTypeFree(recordType)
	return gVariantNewArray(recordType, unsafe.SliceData(records), uintptr(len(records)))
}

func avahiTXTRecords(reply uintptr) map[string]string {
	records := gVariantGetChildValue(reply, 9)
	defer gVariantUnref(records)
	var wire []byte
	for i := range gVariantNChildren(records) {
		record := gVariantGetChildValue(records, i)
		var size uintptr
		data := gVariantGetFixedArray(record, &size, 1)
		if data != 0 && size <= 255 && uintptr(len(wire))+size+1 <= 65535 {
			wire = append(wire, byte(size))
			wire = append(wire, unsafe.Slice((*byte)(cPointer(data)), size)...)
		}
		gVariantUnref(record)
	}
	return driver.ParseDiscoveryTXT(wire)
}

func avahiCall(bus uintptr, dest, path, iface, method, signature string, done avahiReply, args ...uintptr) {
	var token, callback, replyType uintptr
	if done != nil {
		avahiToken++
		token, callback = avahiToken, avahiCallReady
		avahiReplies[token] = done
		replyType = gVariantTypeNew(cstr(signature))
		defer gVariantTypeFree(replyType)
	}
	var params uintptr
	if len(args) != 0 {
		params = gVariantNewTuple(&args[0], uintptr(len(args)))
	}
	gDBusConnectionCall(bus, cstr(dest), cstr(path), cstr(iface), cstr(method), params, replyType, 0, 10000, 0, callback, token)
}

func avahiCallFinished(bus, result, token uintptr) {
	done := avahiReplies[token]
	delete(avahiReplies, token)
	var err uintptr
	reply := gDBusConnectionCallFinish(bus, result, &err)
	if err != 0 {
		defer gErrorFree(err)
		done(bus, 0, goString((*gError)(cPointer(err)).message))
		return
	}
	defer gVariantUnref(reply)
	done(bus, reply, "")
}
