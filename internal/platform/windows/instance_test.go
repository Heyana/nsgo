//go:build windows

package windows

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/engine"
)

// deliveryWait bounds how long a handoff may take. The owner's goroutine reads the message as soon as it is in the pipe, so this is far longer than delivery needs on a loaded runner, yet a lost message still fails the test in seconds rather than at the binary's timeout.
const deliveryWait = 30 * time.Second

// TestInstanceHandsOffByKey keeps the default instance and two keyed ones running together; a later launch must send its files and links to the owner of its own key, even while the other owners run.
func TestInstanceHandsOffByKey(t *testing.T) {
	id := "dev.nsgo.instance-test-" + t.TempDir()
	owners := map[string]chan []string{}
	for _, key := range []string{"", "one", "two"} {
		owner, already := claimInstance(id, key, launch{})
		if already || owner == nil {
			t.Fatalf("key %q claim = (%v, %v), want an independent owner", key, owner, already)
		}
		t.Cleanup(owner.close)
		got := make(chan []string, 1)
		owners[key] = got
		owner.setDelivery(func(l launch) { got <- append(l.Files, l.Links...) })
	}
	for key, got := range owners {
		path, link := `C:\Documents\`+key+".txt", "umapp://open/"+key
		other, already := claimInstance(id, key, launch{Launch: engine.Launch{Files: []string{path}, Links: []string{link}}})
		if !already || other != nil {
			other.close()
			t.Fatalf("same key %q claim = (%v, %v), want handoff", key, other, already)
		}
		select {
		case paths := <-got:
			if len(paths) != 2 || paths[0] != path || paths[1] != link {
				t.Fatalf("key %q received %v; want %q and %q", key, paths, path, link)
			}
		case <-time.After(deliveryWait):
			t.Fatalf("key %q did not receive its files within %v", key, deliveryWait)
		}
	}
}

// TestInstanceReceivesAClosedSender sends a launch that connects, writes and closes before the owner calls ConnectNamedPipe, which then fails with ERROR_NO_DATA; the message must still be read rather than dropped.
func TestInstanceReceivesAClosedSender(t *testing.T) {
	_, name := instanceNames("dev.nsgo.instance-test-"+t.TempDir(), "closed")
	pipe, err := createInstancePipe(name)
	if pipe == instanceInvalidHandle {
		t.Fatalf("create pipe: %v", err)
	}
	defer closeHandle.Call(pipe)
	path, link := `C:\Documents\closed.txt`, "umapp://open/closed"
	if !sendInstance(name, launch{Launch: engine.Launch{Files: []string{path}, Links: []string{link}}, Task: "Open"}) {
		t.Fatal("sendInstance failed")
	}
	got, ok := receiveInstance(pipe)
	if !ok {
		t.Fatal("receiveInstance dropped the message of a sender that had already closed")
	}
	if len(got.Files) != 1 || got.Files[0] != path || len(got.Links) != 1 || got.Links[0] != link || got.Task != "Open" {
		t.Fatalf("received %+v; want %q, %q and task Open", got, path, link)
	}
}
