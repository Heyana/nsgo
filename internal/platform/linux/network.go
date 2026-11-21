//go:build linux && !android

package linux

import (
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

var linuxNetwork struct {
	monitor            func() uintptr
	available, metered func(uintptr) int32
	notify             uintptr
	next               atomic.Uintptr
	observers          sync.Map
}

var bindLinuxNetwork = sync.OnceValue(func() bool {
	for name, into := range map[string]any{
		"g_network_monitor_get_default":           &linuxNetwork.monitor,
		"g_network_monitor_get_network_available": &linuxNetwork.available,
		"g_network_monitor_get_network_metered":   &linuxNetwork.metered,
	} {
		symbol, err := purego.Dlsym(gioLib, name)
		if err != nil {
			return false
		}
		purego.RegisterFunc(into, symbol)
	}
	linuxNetwork.notify = newCallback(linuxNetworkChanged)
	return true
})

func linuxNetworkChanged(monitor, _, id uintptr) {
	if callback, ok := linuxNetwork.observers.Load(id); ok {
		callback.(func(driver.Network))(linuxNetworkState(monitor))
	}
}

func linuxNetworkState(monitor uintptr) driver.Network {
	online := linuxNetwork.available(monitor) != 0
	return driver.Network{Online: online, Expensive: online && linuxNetwork.metered(monitor) != 0}
}

func watchNetwork(changed func(driver.Network)) (driver.Network, func()) {
	if !bindLinuxNetwork() {
		return driver.Network{Online: true}, nil
	}
	monitor := linuxNetwork.monitor()
	if monitor == 0 {
		return driver.Network{Online: true}, nil
	}
	id := linuxNetwork.next.Add(1)
	linuxNetwork.observers.Store(id, changed)
	available := gSignalConnect(monitor, "notify::network-available", linuxNetwork.notify, id)
	metered := gSignalConnect(monitor, "notify::network-metered", linuxNetwork.notify, id)
	return linuxNetworkState(monitor), func() {
		linuxNetwork.observers.Delete(id)
		gSignalHandlerDisconnect(monitor, available)
		gSignalHandlerDisconnect(monitor, metered)
	}
}
