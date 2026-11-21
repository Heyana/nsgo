//go:build darwin && !ios && !nsgodev

package macos

import "github.com/nsgo-dev/nsgo/internal/driver"

type devWindowFrame struct{}

func (*devWindowFrame) restore(f driver.WindowFrame) driver.WindowFrame { return f }
func (*devWindowFrame) start(*surface)                                  {}
func (*devWindowFrame) record(driver.Environment)                       {}
