package deviceidentity_test

import (
	"os"
	"strings"
	"sync"
	"testing"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// This fixture claims no device-name entitlement. A physical iPhone must
// keep its user-assigned name private while still providing its model.
func TestAppleDeviceIdentity(t *testing.T) {
	target := nsgotest.Target()
	prefix := "iPhone"
	switch target {
	case "ios", "ios_simulator":
	case "watchos", "watchos_simulator":
		prefix = "Watch"
	default:
		t.Skip("checks Apple mobile hardware identifiers")
	}
	var mu sync.Mutex
	var name, model string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		mu.Lock()
		name, model = cx.Env().DeviceName(), cx.Env().DeviceModel()
		mu.Unlock()
		return core.Text{TestID: "ready", Content: "Device identity"}
	})
	app.Settle()
	mu.Lock()
	defer mu.Unlock()
	if prefix == "iPhone" && (strings.HasPrefix(model, "iPad") || strings.HasPrefix(model, "iPod")) {
		prefix = strings.SplitN(model, ",", 2)[0]
	}
	if want := os.Getenv("NSGO_TEST_DEVICE_MODEL"); want != "" && model != want {
		t.Fatalf("device model = %q, want this device's %q", model, want)
	}
	if !strings.HasPrefix(model, prefix) || !strings.Contains(model, ",") {
		t.Fatalf("device model = %q, want a %s hardware identifier", model, prefix)
	}
	if target == "ios" && name != "" {
		t.Fatal("the unentitled iPhone app received a device name")
	}
	if strings.HasPrefix(target, "watchos") && name != "" {
		t.Fatal("the watch reports a user-assigned name without entitlement attribution")
	}
	t.Logf("hardware model %q; user-assigned name available: %v", model, name != "")
}
