package engine

import (
	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/testhost"
)

func testhostOptions(observe func(*driver.MountBatch)) testhost.Options {
	return testhost.Options{Observe: observe}
}
