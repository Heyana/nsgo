package sensorsettings_test

import (
	"os"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// TestOpenSensorSettings exercises the public API without asking for sensor access. The hold lets an observer verify the real Settings destination; callback success alone does not prove the pane.
func TestOpenSensorSettings(t *testing.T) {
	if target := nsgotest.Target(); target != "macos" && target != "android" {
		t.Skip("native destination acceptance requires -target macos or -target android")
	}
	permission := core.Permission(os.Getenv("NSGO_TEST_SENSOR_SETTINGS"))
	switch permission {
	case core.PermissionLocation, core.PermissionBackgroundLocation, core.PermissionMotion:
	default:
		t.Skip("set NSGO_TEST_SENSOR_SETTINGS to location, background-location or motion for native destination acceptance")
	}
	var answered, opened bool
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{TestID: "open-settings", Style: core.Style{Width: core.Pt(200), Height: core.Pt(80)}, OnPress: func(core.PressEvent) {
			cx.OpenPermissionSettings(permission, func(ok bool) { answered, opened = true, ok })
		}}
	})
	app.Press("id:open-settings")
	app.WaitUntil(10*time.Second, func() bool { return answered })
	if !opened {
		t.Fatalf("%s settings did not open", permission)
	}
	t.Logf("%s settings opened; inspect the native destination during the 15-second hold", permission)
	time.Sleep(15 * time.Second)
}
