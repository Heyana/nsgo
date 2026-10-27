//go:build darwin || linux

package perfhud

import (
	"syscall"
	"time"
)

// cpuTime is the user and system time the process used.
func cpuTime() (time.Duration, bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, false
	}
	return time.Duration(usage.Utime.Nano() + usage.Stime.Nano()), true
}
