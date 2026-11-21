//go:build !darwin && !linux && !windows

package crashreport

import "os"

// These targets do not create crash reports.
func lock(*os.File) (bool, error) { return false, nil }
