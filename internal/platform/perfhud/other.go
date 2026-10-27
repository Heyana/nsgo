//go:build !darwin && !linux && !windows

package perfhud

import "time"

func cpuTime() (time.Duration, bool) { return 0, false }

func memory() (uint64, bool) { return 0, false }
