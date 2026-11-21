package crashreport

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lock(file *os.File) (bool, error) {
	// Lock a byte beyond the trace, leaving writes through the runtime's
	// duplicated handle and app reads of the diagnostic text alone.
	position := windows.Overlapped{OffsetHigh: 1 << 30}
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &position)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}
