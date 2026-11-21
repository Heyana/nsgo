//go:build android

package android

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (s *surface) diskSpace(from, count uint32, path string) {
	if path == "" {
		go s.dispatchTo(from, driver.DiskSpaceEvent(count, driver.DiskSpace{}, driver.ErrDiskSpaceUnavailable))
		return
	}
	if err := s.send(msgDiskSpace, func(w *writer) {
		w.u32(from)
		w.u32(count)
		w.str(path)
	}); err != nil {
		go s.dispatchTo(from, driver.DiskSpaceEvent(count, driver.DiskSpace{}, err))
	}
}

func (s *surface) diskSpaceReply(r *reader) {
	from, count, failure := r.u32(), r.u32(), r.str()
	total, available := int64(r.u64()), int64(r.u64())
	if failure != "" {
		s.dispatchTo(from, driver.DiskSpaceEvent(count, driver.DiskSpace{}, fmt.Errorf("android storage: %s", failure)))
		return
	}
	s.dispatchTo(from, driver.DiskSpaceEvent(count, driver.DiskSpace{Total: total, Available: available, Opportunistic: available}, nil))
}
