//go:build windows && !nsgodev

package windows

import "github.com/nsgo-dev/nsgo/internal/driver"

type devWindowFrame struct{}

func (*devWindowFrame) restore(f driver.WindowFrame) (driver.WindowFrame, bool) { return f, false }
func (*devWindowFrame) start(*window)                                           {}
func (*devWindowFrame) close()                                                  {}
func (*devWindowFrame) changed(*window)                                         {}
