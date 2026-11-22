//go:build android && nsgotest

package android

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// PendingIntent identity must retain the complete notification ID. Aa and BB
// have the same Java String hash; extras alone do not distinguish intents.
func TestNotificationPressIntentsKeepCollidingIDs(t *testing.T) {
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node { return core.Text{Content: "Notification intent identity"} }, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := surface.(*testSurface)
	s.Sync()
	for _, launcher := range []bool{false, true} {
		for _, pair := range [][2]string{{"Aa", "BB"}, {"Aa", "Aa"}, {"提醒:42", "提醒:43"}} {
			same, err := s.ask(map[string]any{"op": "notification-intents", "first": pair[0], "second": pair[1], "launcher": launcher})
			if err != nil {
				t.Fatal(err)
			}
			want := "false"
			if pair[0] == pair[1] {
				want = "true"
			}
			if same != want {
				t.Errorf("launcher %v, intents for %q and %q same=%s, want %s", launcher, pair[0], pair[1], same, want)
			}
		}
	}
}
