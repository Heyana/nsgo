package core

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"testing"
	"time"
)

func TestSharedDeliveryWaitsForAHandlerBeforeAcknowledging(t *testing.T) {
	var posted []func()
	root := testRoot(RootOptions{Post: func(fn func()) { posted = append(posted, fn) }, Clock: NewManualClock(time.Unix(0, 0))})
	tree := NewTree(root, ApproxTextEngine{})
	listen := NewSignal(root, false)
	handled, acknowledged := 0, 0
	tree.Mount(func(cx *Scope) Node {
		return Show{When: listen.Get, Then: func(cx *Scope) Node {
			for range 2 {
				cx.OnShared(func(items []SharedItem) {
					if len(items) != 1 || items[0].Text != "url" {
						t.Fatal(items)
					}
					handled++
				})
			}
			return nil
		}}
	})
	tree.Commit()
	tree.Dispatch(driver.Event{Kind: driver.EventShared, Data: driver.SharedDelivery{Items: []driver.SharedItem{{Text: "url"}}, Acknowledge: func() { acknowledged++ }}})
	if acknowledged != 0 {
		t.Fatal("share acknowledged before a handler registered")
	}
	listen.Set(true)
	tree.Commit()
	if acknowledged != 0 {
		t.Fatal("share acknowledged before its callback ran")
	}
	for len(posted) > 0 {
		fn := posted[0]
		posted = posted[1:]
		fn()
	}
	if handled != 1 || acknowledged != 1 {
		t.Fatalf("initial handling = %d, acknowledgments = %d", handled, acknowledged)
	}
	tree.Dispatch(driver.Event{Kind: driver.EventShared, Data: driver.SharedDelivery{Items: []driver.SharedItem{{Text: "url"}}, Acknowledge: func() { acknowledged++ }}})
	if handled != 3 || acknowledged != 2 {
		t.Fatalf("two handlers: handling = %d, acknowledgments = %d", handled, acknowledged)
	}
}
