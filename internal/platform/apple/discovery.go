//go:build darwin

package apple

import (
	"fmt"
	"maps"
	"math/bits"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Discovery holds the live Bonjour operations of a surface. Its requests,
// callbacks and deallocation all run on the native main queue.
type Discovery struct {
	live map[uint32]*bonjourOperation
}

type bonjourOperation struct {
	owner     *Discovery
	id        uint32
	ref       uintptr
	dispatch  func(driver.Event)
	instances map[bonjourInstanceKey]*bonjourInstance
	visible   map[bonjourEndpoint]int
}

type bonjourInstanceKey struct {
	name, typ, domain string
	index             uint32
}

type bonjourInstance struct {
	browse  *bonjourOperation
	ref     uintptr
	key     bonjourInstanceKey
	service driver.LocalService
}

type bonjourEndpoint struct {
	name, typ, host string
	port            int
}

var (
	bonjourRegister                          func(*uintptr, uint32, uint32, string, string, string, *byte, uint16, uint16, *byte, uintptr, uintptr) int32
	bonjourBrowse                            func(*uintptr, uint32, uint32, string, string, uintptr, uintptr) int32
	bonjourResolve                           func(*uintptr, uint32, uint32, string, string, string, uintptr, uintptr) int32
	bonjourQueue                             func(uintptr, uintptr) int32
	bonjourRelease                           func(uintptr)
	registerReply, browseReply, resolveReply uintptr
	bonjourOperations                        = map[uintptr]*bonjourOperation{}
	bonjourResolutions                       = map[uintptr]*bonjourInstance{}
)

func loadBonjour() {
	if bonjourBrowse != nil {
		return
	}
	lib := load("/usr/lib/libSystem.B.dylib")
	purego.RegisterLibFunc(&bonjourRegister, lib, "DNSServiceRegister")
	purego.RegisterLibFunc(&bonjourBrowse, lib, "DNSServiceBrowse")
	purego.RegisterLibFunc(&bonjourResolve, lib, "DNSServiceResolve")
	purego.RegisterLibFunc(&bonjourQueue, lib, "DNSServiceSetDispatchQueue")
	purego.RegisterLibFunc(&bonjourRelease, lib, "DNSServiceRefDeallocate")
	registerReply = purego.NewCallback(bonjourRegistered)
	browseReply = purego.NewCallback(bonjourBrowsed)
	resolveReply = purego.NewCallback(bonjourResolved)
}

// Handle starts or stops the requested DNS-SD operation on the main queue.
func (d *Discovery) Handle(it *driver.MountItem, dispatch func(driver.Event)) {
	if it.Discovery == nil {
		return
	}
	id, req := it.Count, *it.Discovery
	d.stop(id)
	if req.Op == driver.DiscoveryStop {
		return
	}
	loadBonjour()
	op := &bonjourOperation{owner: d, id: id, dispatch: dispatch}
	const local = "local."
	var status int32
	switch req.Op {
	case driver.DiscoveryAdvertise:
		txt, err := driver.DiscoveryTXT(req.Service.TXT)
		if err != nil {
			op.report(driver.DiscoveryReply{Error: err.Error()})
			return
		}
		// Both Apple Go targets are little-endian; the ABI takes network byte order.
		port := bits.ReverseBytes16(uint16(req.Service.Port))
		status = bonjourRegister(&op.ref, 0, 0, req.Service.Name, req.Service.Type, local, nil, port, uint16(len(txt)), unsafe.SliceData(txt), registerReply, 0)
	case driver.DiscoveryBrowse:
		op.instances = map[bonjourInstanceKey]*bonjourInstance{}
		op.visible = map[bonjourEndpoint]int{}
		status = bonjourBrowse(&op.ref, 0, 0, req.Service.Type, local, browseReply, 0)
	default:
		return
	}
	if status == 0 {
		status = bonjourQueue(op.ref, mainQueue)
		if status != 0 {
			bonjourRelease(op.ref)
		}
	}
	if status != 0 {
		op.report(driver.DiscoveryReply{Error: bonjourError(status)})
		return
	}
	if d.live == nil {
		d.live = map[uint32]*bonjourOperation{}
	}
	d.live[id], bonjourOperations[op.ref] = op, op
}

func (d *Discovery) stop(id uint32) {
	op := d.live[id]
	if op == nil {
		return
	}
	delete(d.live, id)
	delete(bonjourOperations, op.ref)
	for _, service := range op.instances {
		service.cancelResolution()
	}
	bonjourRelease(op.ref)
}

// Close releases the Bonjour registrations, browsers and resolvers of the surface.
func (d *Discovery) Close() {
	for id := range d.live {
		d.stop(id)
	}
}

func (op *bonjourOperation) report(reply driver.DiscoveryReply) {
	reply.Service.TXT = maps.Clone(reply.Service.TXT)
	op.dispatch(driver.Event{Kind: driver.EventDiscovery, Count: op.id, Data: &reply})
}

func (op *bonjourOperation) fail(status int32) {
	op.owner.stop(op.id)
	op.report(driver.DiscoveryReply{Error: bonjourError(status)})
}

func bonjourError(status int32) string {
	const policyDenied = -65570
	if status == policyDenied {
		return "nsgo: local network access denied"
	}
	return fmt.Sprintf("nsgo: Bonjour failed (%d)", status)
}

func bonjourRegistered(ref uintptr, _ uint32, status int32, _, _, _ *byte, _ uintptr) {
	if op := bonjourOperations[ref]; op != nil && status != 0 {
		op.fail(status)
	}
}

func bonjourBrowsed(ref uintptr, flags, index uint32, status int32, name, typ, domain *byte, _ uintptr) {
	op := bonjourOperations[ref]
	if op == nil {
		return
	}
	if status != 0 {
		op.fail(status)
		return
	}
	key := bonjourInstanceKey{bonjourString(name), bonjourString(typ), bonjourString(domain), index}
	const added = 1 << 1
	if flags&added != 0 {
		op.resolve(key)
	} else {
		op.remove(key)
	}
}

func (op *bonjourOperation) resolve(key bonjourInstanceKey) {
	if op.instances[key] != nil {
		return
	}
	inst := &bonjourInstance{browse: op, key: key}
	if bonjourResolve(&inst.ref, 0, key.index, key.name, key.typ, key.domain, resolveReply, 0) != 0 {
		return
	}
	if bonjourQueue(inst.ref, mainQueue) != 0 {
		bonjourRelease(inst.ref)
		return
	}
	op.instances[key], bonjourResolutions[inst.ref] = inst, inst
}

func bonjourResolved(ref uintptr, _, _ uint32, status int32, _, host *byte, port, txtLen uint16, txt *byte, _ uintptr) {
	inst := bonjourResolutions[ref]
	if inst == nil {
		return
	}
	if status == 0 {
		hostname := bonjourString(host)
		if hostname != "" && hostname != "." && port != 0 {
			var records map[string]string
			if txt != nil {
				records = driver.ParseDiscoveryTXT(unsafe.Slice(txt, int(txtLen)))
			}
			inst.service = driver.LocalService{
				Name: inst.key.name, Type: strings.TrimSuffix(inst.key.typ, "."),
				Host: hostname, Port: int(bits.ReverseBytes16(port)), TXT: records,
			}
		}
	}
	inst.cancelResolution()
	if inst.service.Host == "" {
		delete(inst.browse.instances, inst.key)
		return
	}
	key := inst.endpoint()
	inst.browse.visible[key]++
	if inst.browse.visible[key] == 1 {
		inst.browse.report(driver.DiscoveryReply{Service: inst.service})
	}
}

func (op *bonjourOperation) remove(key bonjourInstanceKey) {
	inst := op.instances[key]
	if inst == nil {
		return
	}
	delete(op.instances, key)
	inst.cancelResolution()
	if inst.service.Host != "" {
		endpoint := inst.endpoint()
		op.visible[endpoint]--
		if op.visible[endpoint] == 0 {
			delete(op.visible, endpoint)
			op.report(driver.DiscoveryReply{Service: inst.service, Removed: true})
		}
	}
}

func (inst *bonjourInstance) endpoint() bonjourEndpoint {
	s := inst.service
	return bonjourEndpoint{s.Name, s.Type, s.Host, s.Port}
}

func (inst *bonjourInstance) cancelResolution() {
	if inst.ref != 0 {
		delete(bonjourResolutions, inst.ref)
		bonjourRelease(inst.ref)
		inst.ref = 0
	}
}

func bonjourString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}
