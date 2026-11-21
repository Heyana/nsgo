//go:build linux && !android && !nsgodev

package linux

import "github.com/nsgo-dev/nsgo/internal/driver"

type devWindowFrame struct{}

func (*devWindowFrame) restore(*Options)                   {}
func (*devWindowFrame) start(*window)                      {}
func (*devWindowFrame) close()                             {}
func (*devWindowFrame) record(*window, driver.Environment) {}
