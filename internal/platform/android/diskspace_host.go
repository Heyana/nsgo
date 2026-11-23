//go:build nsgoandroidhost && !android

package android

import "github.com/nsgo-dev/nsgo/internal/driver"

func (s *surface) requestDiskSpace(from, count uint32, path string) {
	// DataDir is on the development computer in this mode.
	go func() {
		space, err := hostDiskSpace(path)
		s.dispatchTo(from, driver.DiskSpaceEvent(count, space, err))
	}()
}
