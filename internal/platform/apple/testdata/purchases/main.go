// Command purchases hosts the isolated native StoreKit acceptance tests.
package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() {
	nsgo.Run(func(*core.Scope) core.Node { return core.Text{Content: "StoreKit native fixture"} })
}
