package core

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestNetworkStartsOnDemandAndReportsChanges(t *testing.T) {
	var pending []func()
	var report func(driver.Network)
	started, stopped := 0, 0
	root := NewRoot(RootOptions{
		Post: func(fn func()) { pending = append(pending, fn) },
		WatchNetwork: func(changed func(driver.Network)) (driver.Network, func()) {
			started++
			report = changed
			return driver.Network{Online: true, Expensive: true}, func() { stopped++ }
		},
	})
	tree := NewTree(root, nil)
	pinned := Network{Online: false}
	tree.SetEnvironment(Environment{Network: &pinned})
	if started != 0 {
		t.Fatal("reading no network state opened a watcher")
	}
	if got := root.Env().Network(); got != pinned || started != 0 {
		t.Fatalf("pinned state = %+v, watchers started = %d", got, started)
	}

	child := root.Child()
	var changes []Network
	initial := root.Env().Network()
	child.OnNetworkChange(func(state Network) { changes = append(changes, state) })
	if len(changes) != 0 {
		t.Fatalf("registered a callback with an initial event: %v", changes)
	}
	tree.SetEnvironment(Environment{})
	metered := Network{Online: true, Expensive: true}
	if got := root.Env().Network(); got != metered || started != 1 || initial != pinned {
		t.Fatalf("live state = %+v, watchers started = %d, pinned = %+v", got, started, initial)
	}

	emit := func(state Network) {
		t.Helper()
		report(state)
		for _, fn := range pending {
			fn()
		}
		pending = nil
	}
	emit(metered)
	emit(Network{})
	emit(Network{})
	online := Network{Online: true}
	emit(online)
	child.Dispose()
	emit(Network{})
	if want := []Network{metered, {}, online}; !slices.Equal(changes, want) {
		t.Fatalf("changes = %+v, want %+v", changes, want)
	}
	root.Dispose()
	if stopped != 1 {
		t.Fatalf("watcher stopped %d times", stopped)
	}
}

func TestNetworkDefaultsOnline(t *testing.T) {
	root := NewRoot(RootOptions{})
	defer root.Dispose()
	if got := root.Env().Network(); got != (Network{Online: true}) {
		t.Fatalf("default network = %+v", got)
	}
}
