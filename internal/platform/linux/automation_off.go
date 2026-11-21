//go:build linux && !android && !nsgoautomation

package linux

func registerLiveWindow(*window)   {}
func unregisterLiveWindow(*window) {}
