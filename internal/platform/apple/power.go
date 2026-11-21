//go:build darwin

package apple

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ProcessPower reads Foundation's current thermal state and Low Power Mode.
func ProcessPower() (driver.ThermalState, driver.LowPowerMode) {
	info := objc.ID(Class("NSProcessInfo")).Send(sel("processInfo"))
	thermal, low := driver.ThermalUnknown, driver.LowPowerUnknown
	state := sel("thermalState")
	if MsgBool(info, sel("respondsToSelector:"), uintptr(state)) {
		if value := objc.Send[int](info, state); value >= 0 && value <= 3 {
			thermal = driver.ThermalNominal + driver.ThermalState(value)
		}
	}
	mode := sel("isLowPowerModeEnabled")
	if MsgBool(info, sel("respondsToSelector:"), uintptr(mode)) {
		low = driver.LowPowerOff
		if MsgBool(info, mode) {
			low = driver.LowPowerOn
		}
	}
	return thermal, low
}

// ObserveProcessPower calls changed on the main thread whenever Foundation reports a power constraint changed, until the returned function stops it.
func ObserveProcessPower(changed func()) func() {
	// Foundation starts reporting thermal changes after reading thermalState.
	ProcessPower()
	center := objc.ID(Class("NSNotificationCenter")).Send(sel("defaultCenter"))
	refresh := func(objc.ID) { changed() }
	stopThermal := Observe(center, Global(foundationPath, "NSProcessInfoThermalStateDidChangeNotification"), refresh)
	stopPower := Observe(center, Global(foundationPath, "NSProcessInfoPowerStateDidChangeNotification"), refresh)
	return func() {
		stopThermal()
		stopPower()
	}
}
