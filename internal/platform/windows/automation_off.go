//go:build windows && !nsgoautomation

package windows

func trackLiveWindow(uint32, *window)   {}
func untrackLiveWindow(uint32, *window) {}
