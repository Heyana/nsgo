//go:build windows

package windows

import (
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf8"
	"unsafe"

	win "golang.org/x/sys/windows"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	dnsAPI               = win.NewLazySystemDLL("dnsapi.dll")
	dnsServiceConstruct  = dnsAPI.NewProc("DnsServiceConstructInstance")
	dnsServiceFree       = dnsAPI.NewProc("DnsServiceFreeInstance")
	dnsServiceRegister   = dnsAPI.NewProc("DnsServiceRegister")
	dnsServiceDeregister = dnsAPI.NewProc("DnsServiceDeRegister")
	dnsServiceBrowse     = dnsAPI.NewProc("DnsServiceBrowse")
	dnsBrowseCancel      = dnsAPI.NewProc("DnsServiceBrowseCancel")
	dnsServiceResolve    = dnsAPI.NewProc("DnsServiceResolve")
	dnsResolveCancel     = dnsAPI.NewProc("DnsServiceResolveCancel")
	dnsBrowseCallback    = syscall.NewCallback(dnsBrowseCompleted)
	dnsResolveCallback   = syscall.NewCallback(dnsResolveCompleted)
	dnsRegisterCallback  = syscall.NewCallback(dnsRegisterCompleted)

	dnsAvailable = sync.OnceValue(func() bool {
		for _, proc := range []*win.LazyProc{dnsServiceConstruct, dnsServiceFree, dnsServiceRegister,
			dnsServiceDeregister, dnsServiceBrowse, dnsBrowseCancel, dnsServiceResolve, dnsResolveCancel} {
			if proc.Find() != nil {
				return false
			}
		}
		return true
	})
)

// The DNS-SD structures in windns.h for Windows' 64-bit targets.
type dnsServiceInstance struct {
	name, host             *uint16
	ip4, ip6               unsafe.Pointer
	port, priority, weight uint16
	propertyCount          uint32
	keys, values           **uint16
	interfaceIndex         uint32
}

type dnsRegisterRequest struct {
	version, interfaceIndex uint32
	instance                *dnsServiceInstance
	callback, token         uintptr
	credentials             uintptr
	unicastEnabled          uint32
}

type dnsQueryRequest struct {
	version, interfaceIndex uint32
	name                    *uint16
	callback, token         uintptr
}

const dnsRequestVersion = 1

// The DNS worker threads receive integer tokens. Browse records and resolve
// results are copied before those threads release their native allocations.
var (
	nextDNSToken     atomic.Uintptr
	dnsQueries       sync.Map // uintptr -> *dnsQuery
	dnsRegistrations sync.Map // uintptr -> chan dnsRegistrationResult
)

type dnsQuery struct {
	request    dnsQueryRequest
	cancel     uintptr // DNS_SERVICE_CANCEL
	pin        runtime.Pinner
	ended      atomic.Bool
	cancelProc *win.LazyProc
	browse     func(syscall.Errno, []dnsPTR)
	resolve    func(driver.LocalService)
}

type dnsPTR struct {
	name    string
	removed bool
}

func (q *dnsQuery) start(name string, proc *win.LazyProc, callback uintptr) error {
	q.request = dnsQueryRequest{version: dnsRequestVersion, name: wide(name), callback: callback, token: nextDNSToken.Add(1)}
	// Keep DNSAPI's arguments pinned through completion or cancellation.
	q.pin.Pin(&q.request)
	q.pin.Pin(q.request.name)
	dnsQueries.Store(q.request.token, q)
	status, _, _ := proc.Call(uintptr(unsafe.Pointer(&q.request)), uintptr(unsafe.Pointer(&q.cancel)))
	if status != uintptr(win.DNS_REQUEST_PENDING) {
		q.finish()
		return dnsDiscoveryError(proc.Name, syscall.Errno(status))
	}
	return nil
}

func (q *dnsQuery) finish() {
	if q.ended.CompareAndSwap(false, true) {
		dnsQueries.Delete(q.request.token)
		q.pin.Unpin()
	}
}

func (q *dnsQuery) stop() {
	if q == nil || q.ended.Load() {
		return
	}
	status, _, _ := q.cancelProc.Call(uintptr(unsafe.Pointer(&q.cancel)))
	switch syscall.Errno(status) {
	case 0, win.ERROR_CANCELLED, win.ERROR_INVALID_PARAMETER:
		q.finish()
	}
}

func dnsBrowseCompleted(status uint32, token uintptr, records *win.DNSRecord) uintptr {
	if records != nil {
		const freeRecordList = 1 // DnsFreeRecordList
		defer win.DnsRecordListFree(records, freeRecordList)
	}
	value, ok := dnsQueries.Load(token)
	if !ok {
		return 0
	}
	q := value.(*dnsQuery)
	if syscall.Errno(status) == win.ERROR_CANCELLED {
		q.finish()
		return 0
	}
	var results []dnsPTR
	for r := records; r != nil; r = r.Next {
		if r.Type == win.DNS_TYPE_PTR {
			data := (*win.DNSPTRData)(unsafe.Pointer(&r.Data))
			results = append(results, dnsPTR{name: wideString(data.Host), removed: r.Ttl == 0})
		}
	}
	q.browse(syscall.Errno(status), results)
	return 0
}

func dnsResolveCompleted(status uint32, token uintptr, instance *dnsServiceInstance) uintptr {
	defer freeDNSInstance(instance)
	value, ok := dnsQueries.Load(token)
	if !ok {
		return 0
	}
	q := value.(*dnsQuery)
	q.finish()
	var service driver.LocalService
	if status == 0 && instance != nil {
		service.Host = wideString(instance.host)
		service.Port = int(instance.port)
		if n := instance.propertyCount; n > 0 && n <= 65535 && instance.keys != nil && instance.values != nil {
			service.TXT = make(map[string]string, n)
			keys, values := unsafe.Slice(instance.keys, n), unsafe.Slice(instance.values, n)
			for i, name := range keys {
				key := strings.ToLower(wideString(name))
				if _, found := service.TXT[key]; key != "" && !found {
					service.TXT[key] = wideString(values[i])
				}
			}
		}
	}
	q.resolve(service)
	return 0
}

type dnsRegistrationResult struct {
	status   syscall.Errno
	instance *dnsServiceInstance
}

func dnsRegisterCompleted(status uint32, token uintptr, instance *dnsServiceInstance) uintptr {
	if value, ok := dnsRegistrations.Load(token); ok {
		value.(chan dnsRegistrationResult) <- dnsRegistrationResult{syscall.Errno(status), instance}
	} else {
		freeDNSInstance(instance)
	}
	return 0
}

func freeDNSInstance(instance *dnsServiceInstance) {
	if instance != nil {
		dnsServiceFree.Call(uintptr(unsafe.Pointer(instance)))
	}
}

func newDNSInstance(s driver.LocalService) (*dnsServiceInstance, error) {
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	var keys, values []*uint16
	for _, key := range slices.Sorted(maps.Keys(s.TXT)) {
		value := s.TXT[key]
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("nsgo: Windows DNS-SD TXT attribute %q must be UTF-8 text without NUL", key)
		}
		keys, values = append(keys, wide(key)), append(values, wide(value))
	}
	name := wide(s.Name + "." + s.Type + ".local")
	hostname := wide(host + ".local")
	p, _, err := dnsServiceConstruct.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(hostname)), 0, 0,
		uintptr(s.Port), 0, 0, uintptr(len(keys)), uintptr(unsafe.Pointer(unsafe.SliceData(keys))),
		uintptr(unsafe.Pointer(unsafe.SliceData(values))))
	if p == 0 {
		return nil, fmt.Errorf("nsgo: construct Windows DNS-SD instance: %w", err)
	}
	// DNSAPI owns the memory behind this address.
	return *(**dnsServiceInstance)(unsafe.Pointer(&p)), nil
}

func runDNSRegistration(service driver.LocalService, stopped <-chan struct{}) error {
	select {
	case <-stopped:
		return nil
	default:
	}
	instance, err := newDNSInstance(service)
	if err != nil {
		return err
	}
	defer freeDNSInstance(instance)
	key := nextDNSToken.Add(1)
	results := make(chan dnsRegistrationResult, 1)
	dnsRegistrations.Store(key, results)
	defer dnsRegistrations.Delete(key)
	request := dnsRegisterRequest{version: dnsRequestVersion, instance: instance, callback: dnsRegisterCallback, token: key}
	var pin runtime.Pinner
	pin.Pin(&request)
	defer pin.Unpin()
	status, _, _ := dnsServiceRegister.Call(uintptr(unsafe.Pointer(&request)), 0)
	if status != uintptr(win.DNS_REQUEST_PENDING) {
		return dnsDiscoveryError(dnsServiceRegister.Name, syscall.Errno(status))
	}
	result := <-results
	if result.instance != nil && result.instance != instance {
		defer freeDNSInstance(result.instance)
	}
	if result.status != 0 {
		return dnsDiscoveryError(dnsServiceRegister.Name, result.status)
	}
	// A stop may arrive while registration is pending. Use the instance that
	// Windows registered, including any name change due to a collision.
	<-stopped
	if result.instance != nil {
		request.instance = result.instance
	}
	status, _, _ = dnsServiceDeregister.Call(uintptr(unsafe.Pointer(&request)), 0)
	if status == uintptr(win.DNS_REQUEST_PENDING) {
		result := <-results
		if result.instance != instance && result.instance != request.instance {
			freeDNSInstance(result.instance)
		}
	}
	return nil
}

func dnsDiscoveryError(operation string, status syscall.Errno) error {
	return fmt.Errorf("nsgo: Windows DNS-SD %s: %w", operation, status)
}

// Only the window thread owns the service operations and their visible results.
type dnsDiscovery struct {
	live map[uint32]*dnsDiscoveryOperation
}

type dnsDiscoveryOperation struct {
	window    *window
	id        uint32
	typeName  string
	stopped   chan struct{}
	browser   *dnsQuery
	instances map[string]*dnsDiscoveredService
}

type dnsDiscoveredService struct {
	lookup  *dnsQuery
	service driver.LocalService
}

func (w *window) handleDiscovery(item *driver.MountItem) {
	if item.Discovery == nil {
		return
	}
	id, req := item.Count, *item.Discovery
	w.discovery.stop(id)
	if req.Op != driver.DiscoveryBrowse && req.Op != driver.DiscoveryAdvertise {
		return
	}
	op := &dnsDiscoveryOperation{window: w, id: id, typeName: req.Service.Type}
	if w.discovery.live == nil {
		w.discovery.live = map[uint32]*dnsDiscoveryOperation{}
	}
	w.discovery.live[id] = op
	if !dnsAvailable() {
		op.fail(fmt.Errorf("nsgo: Windows DNS-SD unavailable"))
		return
	}
	if req.Op == driver.DiscoveryAdvertise {
		op.stopped = make(chan struct{})
		go func() {
			if err := runDNSRegistration(req.Service, op.stopped); err != nil {
				w.post(func() {
					if op.active() {
						op.fail(err)
					}
				})
			}
		}()
		return
	}
	op.instances = map[string]*dnsDiscoveredService{}
	op.browser = &dnsQuery{cancelProc: dnsBrowseCancel, browse: func(status syscall.Errno, results []dnsPTR) {
		if (status == 0 && len(results) == 0) || status == win.DNS_INFO_NO_RECORDS {
			return
		}
		w.post(func() { op.browseResults(status, results) })
	}}
	if err := op.browser.start(req.Service.Type+".local", dnsServiceBrowse, dnsBrowseCallback); err != nil {
		op.fail(err)
	}
}

func (d *dnsDiscovery) stop(id uint32) {
	op := d.live[id]
	if op == nil {
		return
	}
	delete(d.live, id)
	if op.stopped != nil {
		close(op.stopped)
	}
	op.browser.stop()
	for _, entry := range op.instances {
		entry.lookup.stop()
	}
}

func (d *dnsDiscovery) close() {
	for id := range d.live {
		d.stop(id)
	}
}

func (op *dnsDiscoveryOperation) active() bool { return op.window.discovery.live[op.id] == op }

func (op *dnsDiscoveryOperation) report(reply driver.DiscoveryReply) {
	reply.Service.TXT = maps.Clone(reply.Service.TXT)
	op.window.engine.Dispatch(driver.Event{Kind: driver.EventDiscovery, Count: op.id, Data: &reply})
}

func (op *dnsDiscoveryOperation) fail(err error) {
	op.window.discovery.stop(op.id)
	op.report(driver.DiscoveryReply{Error: err.Error()})
}

func (op *dnsDiscoveryOperation) browseResults(status syscall.Errno, results []dnsPTR) {
	if !op.active() {
		return
	}
	if status != 0 {
		op.fail(dnsDiscoveryError(dnsServiceBrowse.Name, status))
		return
	}
	suffix := "." + op.typeName + ".local"
	for _, result := range results {
		fullName := strings.TrimSuffix(result.name, ".")
		key := strings.ToLower(fullName)
		if len(fullName) <= len(suffix) || !strings.HasSuffix(key, suffix) {
			continue
		}
		entry := op.instances[key]
		if result.removed {
			if entry != nil {
				delete(op.instances, key)
				entry.lookup.stop()
				if entry.service.Host != "" {
					op.report(driver.DiscoveryReply{Service: entry.service, Removed: true})
				}
			}
			continue
		}
		if entry == nil {
			op.resolve(key, fullName, fullName[:len(fullName)-len(suffix)])
		}
	}
}

func (op *dnsDiscoveryOperation) resolve(key, fullName, name string) {
	entry := &dnsDiscoveredService{}
	op.instances[key] = entry
	entry.lookup = &dnsQuery{cancelProc: dnsResolveCancel, resolve: func(service driver.LocalService) {
		op.window.post(func() {
			if !op.active() || op.instances[key] != entry {
				return
			}
			entry.lookup = nil
			if service.Host == "" || service.Port == 0 {
				delete(op.instances, key)
				return
			}
			service.Name, service.Type = name, op.typeName
			entry.service = service
			op.report(driver.DiscoveryReply{Service: service})
		})
	}}
	if err := entry.lookup.start(fullName, dnsServiceResolve, dnsResolveCallback); err != nil {
		delete(op.instances, key)
	}
}
