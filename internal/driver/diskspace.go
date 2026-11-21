package driver

import "errors"

// DiskSpace is the byte counts answered by ItemDiskSpace.
type DiskSpace struct {
	// Total is the capacity the system reports.
	Total int64
	// Available is what the app could write for a user-requested operation.
	Available int64
	// Opportunistic is what the app could use for prefetching or caches.
	Opportunistic int64
}

var (
	ErrDiskSpaceNotDeclared = errors.New("nsgo: disk space requires NSPrivacyAccessedAPICategoryDiskSpace in apple.privacy.accessed_apis")
	ErrDiskSpaceUnavailable = errors.New("nsgo: disk space is unavailable")
)

// DiskSpaceResult is an EventDiskSpace's data.
type DiskSpaceResult struct {
	Space DiskSpace
	Err   error
}

// DiskSpaceEvent answers ItemDiskSpace count.
func DiskSpaceEvent(count uint32, space DiskSpace, err error) Event {
	return Event{Kind: EventDiskSpace, Count: count, Data: DiskSpaceResult{Space: space, Err: err}}
}
