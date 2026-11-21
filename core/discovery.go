package core

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// LocalService is a DNS-SD instance. Name, Type, Port and TXT describe an
// advertisement. A discovered instance also has its DNS hostname or IP address in Host.
type LocalService driver.LocalService

// ServiceEvent reports a discovered service, its removal, or a browse failure.
type ServiceEvent struct {
	Service LocalService
	Removed bool
	Err     error
}

// ErrDiscoveryUnavailable means the target does not provide DNS-SD.
var ErrDiscoveryUnavailable = errors.New("nsgo: local service discovery unavailable")

type discoveryOperation struct {
	owner *Scope
	reply func(driver.DiscoveryReply)
}

// AdvertiseService publishes service on the local network while the current
// scope is alive. Call stop on the engine goroutine to end the advertisement. A later
// platform failure reaches onError on the engine goroutine, if supplied.
func (cx *Scope) AdvertiseService(service LocalService, onError func(error)) (stop func(), err error) {
	typ, err := localServiceType(service.Type)
	if err != nil {
		return nil, err
	}
	if service.Name == "" || len(service.Name) > 63 || !utf8.ValidString(service.Name) || strings.ContainsRune(service.Name, 0) || service.Port < 1 || service.Port > 65535 {
		return nil, errors.New("nsgo: invalid local service name or port")
	}
	if _, err := driver.DiscoveryTXT(service.TXT); err != nil {
		return nil, fmt.Errorf("nsgo: local service: %w", err)
	}
	service.Type, service.Host, service.TXT = typ, "", maps.Clone(service.TXT)
	return cx.startDiscovery(driver.DiscoveryAdvertise, service, func(r driver.DiscoveryReply) {
		if onError != nil && r.Error != "" {
			onError(errors.New(r.Error))
		}
	})
}

// BrowseServices reports the services of serviceType as they appear or
// disappear. A failure ends browsing and reports an event with Err set.
// Call stop on the engine goroutine. Disposal of the current scope also stops browsing.
func (cx *Scope) BrowseServices(serviceType string, found func(ServiceEvent)) (stop func(), err error) {
	typ, err := localServiceType(serviceType)
	if err != nil {
		return nil, err
	}
	return cx.startDiscovery(driver.DiscoveryBrowse, LocalService{Type: typ}, func(r driver.DiscoveryReply) {
		if found == nil {
			return
		}
		if r.Error != "" {
			found(ServiceEvent{Err: errors.New(r.Error)})
			return
		}
		found(ServiceEvent{Service: LocalService(r.Service), Removed: r.Removed})
	})
}

func (cx *Scope) startDiscovery(op driver.DiscoveryOp, service LocalService, reply func(driver.DiscoveryReply)) (func(), error) {
	if err := cx.gate(serviceDiscovery, service.Type); err != nil {
		return nil, err
	}
	t, owner := cx.tree(), cx.current()
	if t == nil || owner.disposed || !Untracked(cx, cx.Env().CanDiscoverServices) {
		return nil, ErrDiscoveryUnavailable
	}
	if t.discovery == nil {
		t.discovery = map[uint32]discoveryOperation{}
	}
	t.alertID++
	id := t.alertID
	t.discovery[id] = discoveryOperation{owner, reply}
	t.sendDiscovery(id, driver.DiscoveryRequest{Op: op, Service: driver.LocalService(service)})
	stop := func() {
		if _, live := t.discovery[id]; live {
			delete(t.discovery, id)
			t.sendDiscovery(id, driver.DiscoveryRequest{Op: driver.DiscoveryStop})
		}
	}
	owner.addCleanup(stop)
	return stop, nil
}

func (t *Tree) sendDiscovery(id uint32, req driver.DiscoveryRequest) {
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemDiscovery, Count: id, Discovery: &req})
	t.changed = true
}

func (t *Tree) discoveryEvent(ev driver.Event) {
	op, live := t.discovery[ev.Count]
	report, ok := ev.Data.(*driver.DiscoveryReply)
	if !live || op.owner.disposed || !ok || report == nil {
		return
	}
	if report.Error != "" {
		delete(t.discovery, ev.Count)
	}
	t.within(op.owner, func() { op.reply(*report) })
}

func localServiceType(raw string) (string, error) {
	typ := strings.ToLower(raw)
	label, transport, _ := strings.Cut(typ, ".")
	if (transport == "_tcp" || transport == "_udp") && len(label) >= 2 && len(label) <= 16 && label[0] == '_' {
		name := label[1:]
		letter, valid := false, name[0] != '-' && name[len(name)-1] != '-' && !strings.Contains(name, "--")
		for _, c := range name {
			alpha := c >= 'a' && c <= 'z'
			letter = letter || alpha
			valid = valid && (alpha || c >= '0' && c <= '9' || c == '-')
		}
		if letter && valid {
			return typ, nil
		}
	}
	return "", fmt.Errorf("nsgo: invalid DNS-SD service type %q", raw)
}
