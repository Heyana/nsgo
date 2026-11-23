package android

import "github.com/nsgo-dev/nsgo/internal/driver"

func (s *surface) requestDiskSpace(from, count uint32, path string) {
	if err := s.send(msgDiskSpace, func(w *writer) {
		w.u32(from)
		w.u32(count)
		w.str(path)
	}); err != nil {
		go s.dispatchTo(from, driver.DiskSpaceEvent(count, driver.DiskSpace{}, err))
	}
}
