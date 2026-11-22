//go:build android && nsgotest

package android

import (
	"encoding/json"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestLiveUpdateActionsKeepOwnerAndFullIdentity(t *testing.T) {
	if target := os.Getenv("NSGO_TEST_TARGET"); target != "android" {
		t.Fatalf("requires nsgo test -target android, got %q", target)
	}
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		return core.Text{Content: "Live Update action identity"}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := surface.(*testSurface)
	s.Sync()
	for _, c := range []struct {
		owner, otherOwner, id, otherID, action, otherAction string
		same                                                bool
	}{
		{"one", "one", "delivery", "delivery", "pause", "pause", true},
		{"one", "two", "delivery", "delivery", "pause", "pause", false},
		{"one", "one", "Aa", "BB", "pause", "pause", false},
		{"one", "one", "delivery", "delivery", "Aa", "BB", false},
		{"one", "one", "提醒/42", "提醒/43", "暂停", "暂停", false},
	} {
		got, err := s.ask(map[string]any{"op": "live-action-intents", "owner": c.owner, "otherOwner": c.otherOwner,
			"first": c.id, "second": c.otherID, "action": c.action, "otherAction": c.otherAction})
		if err != nil {
			t.Fatal(err)
		}
		want := "false"
		if c.same {
			want = "true"
		}
		if got != want {
			t.Errorf("%+v: same=%s, want %s", c, got, want)
		}
	}
}

// A notification permission callback may immediately start an activity. Its cached Live Update availability must already reflect the permission reply.
func TestLiveUpdateStatePrecedesPermissionCallback(t *testing.T) {
	if target := os.Getenv("NSGO_TEST_TARGET"); target != "android" {
		t.Fatalf("requires nsgo test -target android, got %q", target)
	}
	var scope *core.Scope
	var reported atomic.Bool
	ordered := make(chan bool, 1)
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		scope = cx
		return core.Text{Content: "Live Update permission ordering"}
	}, testhost.Options{Event: func(_ uint32, ev driver.Event) {
		if ev.Kind == driver.EventNotification {
			ordered <- reported.Load()
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	surface.Sync()
	driver.HearLiveUpdates(func(data []byte) {
		var report struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &report) == nil && report.Type == "state" {
			reported.Store(true)
		}
	})
	// Ignore the startup state; this assertion observes the next permission reply.
	reported.Store(false)
	scope.Post(func() { scope.NotificationPermission(func(core.NotificationPermission) {}) })
	select {
	case ok := <-ordered:
		if !ok {
			t.Fatal("permission callback preceded Live Update availability")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no notification permission reply")
	}
}
