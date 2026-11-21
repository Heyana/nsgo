package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// DiskSpace gives the capacity and available space in bytes. On the web,
// it estimates the origin's storage quota and the portion still available.
type DiskSpace = driver.DiskSpace

// ErrNotDeclared is returned by DiskSpace on Apple targets unless the
// app declares NSPrivacyAccessedAPICategoryDiskSpace in its privacy settings.
var ErrNotDeclared = driver.ErrDiskSpaceNotDeclared

// ErrDiskSpaceUnavailable is returned where the platform has no disk space service.
var ErrDiskSpaceUnavailable = driver.ErrDiskSpaceUnavailable

// DiskSpace reports the room on the volume containing DataDir. Available
// counts space the system can free on demand where the platform reports it.
// Opportunistic equals Available where there is no separate figure for caches
// and prefetching. The query is asynchronous and done runs on the engine
// goroutine.
func (cx *Scope) DiskSpace(done func(DiskSpace, error)) {
	if t := cx.tree(); t != nil && done != nil {
		t.request(cx, driver.MountItem{Kind: driver.ItemDiskSpace, Value: cx.DataDir()}, func(ev driver.Event) {
			result := ev.Data.(driver.DiskSpaceResult)
			done(result.Space, result.Err)
		})
	}
}
