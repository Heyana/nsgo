package nsgotest

import (
	"maps"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// LocalServices supplies the DNS-SD instances a headless app can browse.
func LocalServices(services ...core.LocalService) Option {
	return func(a *App) {
		a.localServices = make([]core.LocalService, len(services))
		for i, service := range services {
			service.TXT = maps.Clone(service.TXT)
			a.localServices[i] = service
		}
	}
}

func (a *App) discover(it driver.MountItem) {
	if a.target != nil || it.Discovery == nil || it.Discovery.Op != driver.DiscoveryBrowse {
		return
	}
	app := a
	if a.owner != nil {
		app = a.owner
	}
	for _, service := range app.localServices {
		if service.Type == it.Discovery.Service.Type {
			service.TXT = maps.Clone(service.TXT)
			a.post(driver.Event{Kind: driver.EventDiscovery, Count: it.Count, Data: &driver.DiscoveryReply{Service: driver.LocalService(service)}})
		}
	}
}
