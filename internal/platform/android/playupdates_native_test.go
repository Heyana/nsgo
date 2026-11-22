//go:build android && nsgotest

package android

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestPlayUpdatesNativeSDKFailure exercises the real compiled Play SDK through
// the runtime pipe with an owned test app that Play does not publish. No store
// account or update is installed: the service must report its native rejection,
// rather than failing to load the SDK or silently treating the request as direct.
func TestPlayUpdatesNativeSDKFailure(t *testing.T) {
	if os.Getenv("NSGO_PLAY_NATIVE") != "1" {
		t.Skip("set NSGO_PLAY_NATIVE=1 on an owned Android emulator without a published test app")
	}
	old := updateConfig
	updateConfig = updates.Config{App: "Play update test", Version: "1.0", Channel: "store", Store: "com.android.vending"}
	t.Cleanup(func() { updateConfig = old })
	answers := make(chan core.UpdateCheck, 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.Button{TestID: "check", OnPress: func(core.PressEvent) { cx.CheckForUpdates(false, func(answer core.UpdateCheck) { answers <- answer }) }, Children: []core.Node{core.Text{Content: "Check"}}}
	})
	app.Press("id:check")
	select {
	case answer := <-answers:
		if answer.Shown || answer.Err == nil || !strings.HasPrefix(answer.Err.Error(), "Play update:") || strings.Contains(answer.Err.Error(), "NoClassDefFoundError") {
			t.Fatalf("native Play SDK failure = %+v", answer)
		}
		t.Logf("real Play SDK returned the expected native check rejection: %v", answer.Err)
	case <-time.After(40 * time.Second):
		t.Fatal("Play SDK failed to answer through the runtime pipe")
	}
}
