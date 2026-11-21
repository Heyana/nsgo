package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// Network is the device's connectivity as the system reports it. Online
// describes the network route and cannot predict whether a server will answer.
type Network = driver.Network

type networkState struct {
	owner    *Scope
	value    *Signal[Network]
	native   Network
	reported bool
	observed bool
}

func newNetworkState(owner *Scope) *networkState {
	defaultNetwork := Network{Online: true}
	return &networkState{owner: owner, native: defaultNetwork, value: NewSignal(owner, defaultNetwork)}
}

// Network reports the device's connectivity. A platform that cannot
// report it defaults to Online. Bindings that read it follow changes.
func (e *Env) Network() Network {
	e.network.observe()
	return e.network.value.Get()
}

// OnNetworkChange calls fn on the graph's goroutine only when the
// network state changes, until the scope is disposed.
func (cx *Scope) OnNetworkChange(fn func(Network)) {
	state := cx.Env().network
	state.observe()
	last := state.value.GetUntracked()
	cx.Effect(func() {
		if next := state.value.Get(); next != last {
			last = next
			cx.Untrack(func() { fn(next) })
		}
	})
}

func (n *networkState) observe() {
	n.observed = true
	n.start()
}

func (n *networkState) start() {
	watch := n.owner.rt.watchNetwork
	if watch == nil || n.reported || !n.observed {
		return
	}
	n.owner.rt.watchNetwork = nil
	initial, stop := watch(func(value Network) {
		n.owner.Post(func() {
			n.native = value
			if !n.reported {
				n.value.Set(value)
			}
		})
	})
	if stop != nil {
		n.owner.addCleanup(stop)
	}
	n.native = initial
	n.value.Set(initial)
}

func (n *networkState) setEnvironment(value *Network) {
	n.reported = value != nil
	if value != nil {
		n.value.Set(*value)
		return
	}
	n.start()
	n.value.Set(n.native)
}
