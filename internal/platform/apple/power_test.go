//go:build darwin && !ios

package apple

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestProcessPowerObservesFoundationNotifications(t *testing.T) {
	onMain(func() {
		thermal, low := ProcessPower()
		if thermal < driver.ThermalNominal || thermal > driver.ThermalCritical {
			t.Errorf("Foundation thermal state = %v", thermal)
		}
		if low != driver.LowPowerOff && low != driver.LowPowerOn {
			t.Errorf("Foundation low-power mode = %v", low)
		}
		var changes int
		stop := ObserveProcessPower(func() { changes++ })
		defer stop()
		center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
		post := func(name string) {
			center.Send(sel("postNotificationName:object:"), Global(foundationPath, name),
				objc.ID(Class("NSProcessInfo")).Send(sel("processInfo")))
		}
		post("NSProcessInfoThermalStateDidChangeNotification")
		post("NSProcessInfoPowerStateDidChangeNotification")
		if changes != 2 {
			t.Errorf("observed %d Foundation notifications, want 2", changes)
		}
		stop()
		post("NSProcessInfoPowerStateDidChangeNotification")
		if changes != 2 {
			t.Errorf("received a notification after stopping")
		}
	})
}
