//go:build windows

package windows

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	ipHelper                = syscall.NewLazyDLL("iphlpapi.dll")
	notifyIPInterfaceChange = ipHelper.NewProc("NotifyIpInterfaceChange")
	cancelIPNotification    = ipHelper.NewProc("CancelMibChangeNotify2")
	uninitializeNetworkCOM  = ole32.NewProc("CoUninitialize")
	networkNotification     = sync.OnceValue(func() uintptr { return syscall.NewCallback(networkInterfaceChanged) })
	networkObservers        sync.Map
	nextNetworkObserver     atomic.Uintptr
)

func networkInterfaceChanged(id, _, _ uintptr) uintptr {
	if value, ok := networkObservers.Load(id); ok {
		select {
		case value.(chan struct{}) <- struct{}{}:
		default:
		}
	}
	return 0
}

func watchNetwork(changed func(driver.Network)) (driver.Network, func()) {
	first := make(chan driver.Network, 1)
	done := make(chan struct{})
	go runNetworkMonitor(first, done, changed)
	initial := driver.Network{Online: true}
	select {
	case initial = <-first:
	case <-time.After(250 * time.Millisecond):
	}
	return initial, func() { close(done) }
}

func runNetworkMonitor(first chan<- driver.Network, done <-chan struct{}, changed func(driver.Network)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const multithreaded = 0
	hr, _, _ := coInitializeEx.Call(0, multithreaded)
	if failed(hr) {
		first <- driver.Network{Online: true}
		return
	}
	defer uninitializeNetworkCOM.Call()
	clsid := parseGUID("dcb00c01-570f-4a9b-8d69-199fdba5723b")           // CLSID_NetworkListManager
	connectivityIID := parseGUID("dcb00000-570f-4a9b-8d69-199fdba5723b") // IID_INetworkListManager
	costIID := parseGUID("dcb00008-570f-4a9b-8d69-199fdba5723b")         // IID_INetworkCostManager
	const localServer = 4                                                // CLSCTX_LOCAL_SERVER
	var manager *comObject
	hr, _, _ = coCreateInstance.Call(uintptr(unsafe.Pointer(&clsid)), 0, clsctxInprocServer|localServer,
		uintptr(unsafe.Pointer(&connectivityIID)), uintptr(unsafe.Pointer(&manager)))
	if failed(hr) || manager == nil {
		first <- driver.Network{Online: true}
		return
	}
	defer manager.release()
	cost := manager.query(&costIID)
	defer cost.release()

	updates := make(chan struct{}, 1)
	id := nextNetworkObserver.Add(1)
	networkObservers.Store(id, updates)
	defer networkObservers.Delete(id)
	var notification uintptr
	status, _, _ := notifyIPInterfaceChange.Call(0, networkNotification(), id, 0, uintptr(unsafe.Pointer(&notification)))
	if status == 0 {
		defer cancelIPNotification.Call(notification)
	}
	// Interface notifications arrive immediately. Recheck cost and NLM's
	// Internet validation, which can change on the same interface.
	tick := time.NewTicker(750 * time.Millisecond)
	defer tick.Stop()
	last := windowsNetworkState(manager, cost, driver.Network{Online: true})
	first <- last
	changed(last) // the initial query may have finished after the caller's timeout
	for {
		select {
		case <-done:
			return
		case <-updates:
		case <-tick.C:
		}
		if value := windowsNetworkState(manager, cost, last); value != last {
			last = value
			changed(value)
		}
	}
}

func windowsNetworkState(manager, cost *comObject, previous driver.Network) driver.Network {
	const getConnectivity = 13    // INetworkListManager::GetConnectivity
	const internet = 0x40 | 0x400 // NLM_CONNECTIVITY_IPV4_INTERNET, NLM_CONNECTIVITY_IPV6_INTERNET
	var flags uint32
	if failed(manager.call(getConnectivity, uintptr(unsafe.Pointer(&flags)))) {
		return previous
	}
	online := flags&internet != 0
	if !online || cost == nil {
		return driver.Network{Online: online}
	}
	const getCost = 3 // INetworkCostManager::GetCost
	const expensive = 0x2 | 0x4 | 0x10000 | 0x40000 | 0x80000
	var amount uint32
	if failed(cost.call(getCost, uintptr(unsafe.Pointer(&amount)), 0)) {
		return driver.Network{Online: true}
	}
	return driver.Network{Online: true, Expensive: amount&expensive != 0}
}
