package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestLocalServicesAnswersBrowse(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless DNS-SD model")
	}
	var got []core.ServiceEvent
	Start(t, func(cx *core.Scope) core.Node {
		if _, err := cx.BrowseServices("_example._tcp", func(e core.ServiceEvent) { got = append(got, e) }); err != nil {
			t.Fatal(err)
		}
		return core.View{}
	}, LocalServices(
		core.LocalService{Name: "Other", Type: "_example._udp"},
		core.LocalService{Name: "Desk", Type: "_example._tcp", Host: "desk.local.", Port: 4567, TXT: map[string]string{"version": "1"}},
	))
	if len(got) != 1 || got[0].Err != nil || got[0].Service.Host != "desk.local." || got[0].Service.TXT["version"] != "1" {
		t.Fatalf("got %+v", got)
	}
}
