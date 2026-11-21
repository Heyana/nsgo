//go:build darwin || ios

package apple

import (
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

var networkAPI struct {
	create                 func() uintptr
	queue                  func(uintptr, uintptr)
	setup                  func(uintptr, objc.Block)
	start, cancel, release func(uintptr)
	status                 func(uintptr) int32
	expensive              func(uintptr) bool
	newQueue               func(string, uintptr) uintptr
	releaseQueue           func(uintptr)
}

var bindNetwork = sync.OnceValue(func() bool {
	lib, err := purego.Dlopen("/System/Library/Frameworks/Network.framework/Network", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return false
	}
	for name, into := range map[string]any{
		"nw_path_monitor_create":             &networkAPI.create,
		"nw_path_monitor_set_queue":          &networkAPI.queue,
		"nw_path_monitor_set_update_handler": &networkAPI.setup,
		"nw_path_monitor_start":              &networkAPI.start,
		"nw_path_monitor_cancel":             &networkAPI.cancel,
		"nw_release":                         &networkAPI.release,
		"nw_path_get_status":                 &networkAPI.status,
		"nw_path_is_expensive":               &networkAPI.expensive,
	} {
		symbol, err := purego.Dlsym(lib, name)
		if err != nil {
			purego.Dlclose(lib)
			return false
		}
		purego.RegisterFunc(into, symbol)
	}
	purego.RegisterLibFunc(&networkAPI.newQueue, purego.RTLD_DEFAULT, "dispatch_queue_create")
	purego.RegisterLibFunc(&networkAPI.releaseQueue, purego.RTLD_DEFAULT, "dispatch_release")
	return true
})

// WatchNetwork reads and follows the system's default Network.framework path.
func WatchNetwork(changed func(driver.Network)) (driver.Network, func()) {
	initial := driver.Network{Online: true}
	if !bindNetwork() {
		return initial, nil
	}
	monitor := networkAPI.create()
	if monitor == 0 {
		return initial, nil
	}
	queue := networkAPI.newQueue("org.nsgo.network", 0)
	if queue == 0 {
		networkAPI.release(monitor)
		return initial, nil
	}
	first := make(chan driver.Network, 1)
	handler := objcblock.New(func(_ objc.Block, path uintptr) {
		const satisfied = 1 // nw_path_status_satisfied
		online := networkAPI.status(path) == satisfied
		value := driver.Network{Online: online, Expensive: online && networkAPI.expensive(path)}
		select {
		case first <- value:
		default:
		}
		changed(value)
	})
	networkAPI.queue(monitor, queue)
	networkAPI.setup(monitor, handler)
	handler.Release()
	networkAPI.start(monitor)
	// Network.framework sends its first path asynchronously. The graph
	// can read it before building the first view under normal conditions.
	select {
	case initial = <-first:
	case <-time.After(250 * time.Millisecond):
	}
	return initial, func() {
		networkAPI.cancel(monitor)
		networkAPI.release(monitor)
		networkAPI.releaseQueue(queue)
	}
}
