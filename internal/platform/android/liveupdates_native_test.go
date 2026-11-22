//go:build android && nsgotest

package android

import (
	"os"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
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
		if (got == "true") != c.same {
			t.Errorf("%+v: same=%s", c, got)
		}
	}
}
