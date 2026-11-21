//go:build darwin

package apple

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Set by nsgo when the project declares the disk-space category in its
// privacy manifest. Embedding builds carry it in the framework.
var diskSpaceDeclared = "0"

var diskSpaceKeys = sync.OnceValue(func() [4]objc.ID {
	return [4]objc.ID{
		Global(foundationPath, "NSURLVolumeTotalCapacityKey"),
		Global(foundationPath, "NSURLVolumeAvailableCapacityKey"),
		Global(foundationPath, "NSURLVolumeAvailableCapacityForImportantUsageKey"),
		Global(foundationPath, "NSURLVolumeAvailableCapacityForOpportunisticUsageKey"),
	}
})

// DiskSpace answers a volume capacity request. The mount loop starts this
// off the main thread. tvOS and watchOS offer one available-space figure.
func DiskSpace(path string, count uint32, separate bool, dispatch func(driver.Event)) {
	if diskSpaceDeclared != "1" {
		dispatch(driver.DiskSpaceEvent(count, driver.DiskSpace{}, driver.ErrDiskSpaceNotDeclared))
		return
	}
	space, err := volumeDiskSpace(path, separate)
	dispatch(driver.DiskSpaceEvent(count, space, err))
}

func volumeDiskSpace(path string, separate bool) (space driver.DiskSpace, err error) {
	if path == "" {
		return space, driver.ErrDiskSpaceUnavailable
	}
	withPool(func() {
		keys := diskSpaceKeys()
		usageKeys := []objc.ID{keys[0], keys[1]}
		if separate {
			usageKeys = []objc.ID{keys[0], keys[2], keys[3]}
		}
		for _, key := range usageKeys {
			if key == 0 {
				err = driver.ErrDiskSpaceUnavailable
				return
			}
		}
		nsKeys := objc.ID(Class("NSArray")).Send(sel("arrayWithObjects:count:"), unsafe.Pointer(&usageKeys[0]), uint(len(usageKeys)))
		url := objc.ID(Class("NSURL")).Send(sel("fileURLWithPath:isDirectory:"), NSString(path), true)
		var cause objc.ID
		values := url.Send(sel("resourceValuesForKeys:error:"), nsKeys, unsafe.Pointer(&cause))
		if values == 0 {
			err = errors.New(describeError(cause, "could not read disk space"))
			return
		}
		bytes := make([]int64, len(usageKeys))
		for i, key := range usageKeys {
			value := values.Send(sel("objectForKey:"), key)
			if value == 0 {
				err = errors.New("volume did not report its capacity")
				return
			}
			bytes[i] = objc.Send[int64](value, sel("longLongValue"))
		}
		space = driver.DiskSpace{Total: bytes[0], Available: bytes[1], Opportunistic: bytes[len(bytes)-1]}
	})
	return space, err
}
