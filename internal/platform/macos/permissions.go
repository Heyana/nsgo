//go:build darwin && !ios

package macos

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

// permission carries out an ItemPermission numbered id: privacy tells and asks, apple's probe asks for the local network, System Settings opens at the permission's pane, and the guide follows it (permissionguide.go).
func (s *surface) permission(id uint32, r driver.PermissionRequest) {
	e := s.engine
	answer := func(status int) { e.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: status}) }
	switch r.Command {
	case driver.PermissionCheck:
		answer(permissionStatus(r.Name))
	case driver.PermissionAsk:
		if r.Name == driver.PermissionLocalNetwork {
			apple.RequestLocalNetwork(answer)
			return
		}
		privacy.Request(r.Name, answer)
	case driver.PermissionOpenSettings:
		u := privacySettingsURL(r.Name)
		// Before macOS 15 the local network has no pane: nothing keeps it.
		if r.Name == driver.PermissionLocalNetwork && apple.OSVersion().Major < 15 {
			u = ""
		}
		e.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Finished: u != "" && openSystemURL(u)})
	case driver.PermissionGuide:
		s.guidePermission(id, r)
	case driver.PermissionStopGuide:
		stopGuide(s, id)
	}
}

// permissionStatus is privacy's status of the permission name, or what apple's probe knows of the local network, which privacy cannot tell. Any goroutine.
func permissionStatus(name string) int {
	if name == driver.PermissionLocalNetwork {
		return apple.LocalNetworkStatus()
	}
	return privacy.Status(name)
}
