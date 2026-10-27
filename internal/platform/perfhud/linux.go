//go:build linux

package perfhud

import (
	"bytes"
	"os"
	"strconv"
)

// memory is the resident set: the second field of /proc/self/statm, in
// pages.
func memory() (uint64, bool) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}
	fields := bytes.Fields(data)
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(string(fields[1]), 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * uint64(os.Getpagesize()), true
}
