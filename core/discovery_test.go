package core

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestDiscoveryLifetime(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	if _, err := cx.BrowseServices("_example._tcp", nil); !errors.Is(err, ErrDiscoveryUnavailable) {
		t.Fatalf("without discovery: %v", err)
	}
	h.tree.SetEnvironment(Environment{DiscoverServices: true})
	h.commit()
	if _, err := cx.BrowseServices("local.", nil); err == nil {
		t.Fatal("accepted a domain in place of a service type")
	}
	var replies []ServiceEvent
	stop, err := cx.BrowseServices("_EXAMPLE._TCP", func(e ServiceEvent) { replies = append(replies, e) })
	if err != nil {
		t.Fatal(err)
	}
	it := command(h.commit(), driver.ItemDiscovery)
	if it == nil || it.Discovery.Op != driver.DiscoveryBrowse || it.Discovery.Service.Type != "_example._tcp" {
		t.Fatalf("browse request = %+v", it)
	}
	report := func(reply driver.DiscoveryReply) {
		h.tree.Dispatch(driver.Event{Kind: driver.EventDiscovery, Count: it.Count, Data: &reply})
	}
	report(driver.DiscoveryReply{Service: driver.LocalService{Name: "Other", Host: "other.local.", Port: 8000}})
	report(driver.DiscoveryReply{Service: driver.LocalService{Name: "Other"}, Removed: true})
	stop()
	stop()
	if cancel := command(h.commit(), driver.ItemDiscovery); cancel == nil || cancel.Count != it.Count || cancel.Discovery.Op != driver.DiscoveryStop {
		t.Fatalf("stop request = %+v", cancel)
	}
	report(driver.DiscoveryReply{Error: "late reply"})
	if len(replies) != 2 || replies[0].Removed || replies[0].Service.Host != "other.local." || !replies[1].Removed {
		t.Fatalf("browse got %+v", replies)
	}
}
