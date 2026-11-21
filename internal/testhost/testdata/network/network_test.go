package network_test

import (
	"testing"

	_ "github.com/nsgo-dev/nsgo"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestNetworkEnvironmentPins(t *testing.T) {
	changes := make(chan core.Network, 4)
	offline := core.Network{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.OnNetworkChange(func(value core.Network) { changes <- value })
		return core.Text{TestID: "network", ContentFunc: func() string {
			state := cx.Env().Network()
			switch {
			case !state.Online:
				return "offline"
			case state.Expensive:
				return "metered"
			default:
				return "online"
			}
		}}
	}, nsgotest.Environment(core.Environment{Network: &offline}))
	if got := app.Find("id:network").Text; got != "offline" {
		t.Fatalf("initial label = %q, want offline", got)
	}
	metered := core.Network{Online: true, Expensive: true}
	app.SetEnvironment(core.Environment{Network: &metered})
	if got := app.Find("id:network").Text; got != "metered" {
		t.Fatalf("label after changing the network = %q, want metered", got)
	}
	select {
	case got := <-changes:
		if got != metered {
			t.Fatalf("network callback = %+v, want %+v", got, metered)
		}
	default:
		t.Fatal("the network callback did not run")
	}
	app.SetEnvironment(core.Environment{Network: &metered})
	select {
	case extra := <-changes:
		t.Fatalf("duplicate network callback: %+v", extra)
	default:
	}
}
