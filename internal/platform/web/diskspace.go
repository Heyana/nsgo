//go:build js && wasm

package web

import (
	"errors"
	"math"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (s *surface) diskSpace(count uint32) {
	answer := func(space driver.DiskSpace, err error) {
		s.engine.Dispatch(driver.DiskSpaceEvent(count, space, err))
	}
	storage := js.Global().Get("navigator").Get("storage")
	if !storage.Truthy() || storage.Get("estimate").Type() != js.TypeFunction {
		answer(driver.DiskSpace{}, driver.ErrDiskSpaceUnavailable)
		return
	}
	then(storage.Call("estimate"), func(estimate js.Value) {
		quota, usage := estimate.Get("quota"), estimate.Get("usage")
		if quota.Type() != js.TypeNumber || usage.Type() != js.TypeNumber {
			answer(driver.DiskSpace{}, errors.New("storage estimate has no quota or usage"))
			return
		}
		total, used := quota.Float(), usage.Float()
		if math.IsNaN(total) || math.IsNaN(used) || math.IsInf(total, 0) || math.IsInf(used, 0) || total < 0 || used < 0 {
			answer(driver.DiskSpace{}, errors.New("storage estimate has invalid byte counts"))
			return
		}
		available := storageBytes(max(total-used, 0))
		answer(driver.DiskSpace{Total: storageBytes(total), Available: available, Opportunistic: available}, nil)
	}, func(failure js.Value) {
		answer(driver.DiskSpace{}, errors.New("storage estimate: "+failure.String()))
	})
}

func storageBytes(n float64) int64 {
	if n >= math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(n)
}
