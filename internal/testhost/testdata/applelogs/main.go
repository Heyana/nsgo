//go:build darwin

// AppleLogs checks diagnostics from a packaged app with no terminal attached.
package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() {
	log.Print("WARN applelogs1348: 中文 100% %@ %s")
	log.Print("ERROR applelogs1348: before\x00after")
	go func() {
		time.Sleep(3 * time.Second)
		report := verify()
		path := filepath.Join(os.TempDir(), "applelogs1348.txt")
		if runtime.GOOS == "ios" {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, "Documents", "applelogs1348.txt")
		}
		if err := os.WriteFile(path, []byte(report), 0600); err != nil {
			log.Print(err)
		}
	}()
	nsgo.Run(func(cx *core.Scope) core.Node { return core.Text{Content: "Apple log verification"} })
}
