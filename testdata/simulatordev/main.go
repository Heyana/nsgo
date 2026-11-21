package main

import (
	"os"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

var marker = "first"

func main() {
	if path := os.Getenv("NSGO_TEST_MARKER"); path != "" {
		if err := os.WriteFile(path, []byte(marker), 0o600); err != nil {
			panic(err)
		}
	}
	nsgo.Run(func(*core.Scope) core.Node { return core.Text{Content: marker} })
}
