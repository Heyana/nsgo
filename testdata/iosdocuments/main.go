// Command iosdocuments records real system document opens for the iOS
// packaging test. The delayed handler exercises launch-file queuing.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() { nsgo.Run(root) }

func root(cx *core.Scope) core.Node {
	dir := cx.DataDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		panic(err)
	}
	record := func(kind, path, body string) {
		event := struct {
			Kind, Path, Body, SHA256 string
			PID                      int
		}{Kind: kind, Path: path, Body: body, PID: os.Getpid()}
		if kind == "file" {
			sum := sha256.Sum256([]byte(body))
			event.SHA256 = hex.EncodeToString(sum[:])
		}
		data, err := json.Marshal(event)
		if err != nil {
			panic(err)
		}
		file, err := os.OpenFile(filepath.Join(dir, "opened.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		_, err = file.Write(append(data, '\n'))
		file.Close()
		if err != nil {
			panic(err)
		}
	}
	record("started", "", "")
	cx.OnOpenLinks(func(links []string) {
		for _, link := range links {
			record("link", link, "")
		}
	})
	count := core.NewSignal(cx, 0)
	go func() {
		time.Sleep(800 * time.Millisecond)
		cx.Post(func() {
			cx.OnOpenFiles(func(paths []string) {
				for _, path := range paths {
					data, err := os.ReadFile(path)
					if err != nil {
						record("error", path, err.Error())
						continue
					}
					record("file", path, string(data))
					count.Update(func(n int) int { return n + 1 })
				}
			})
		})
	}()
	return core.Text{ContentFunc: func() string { return fmt.Sprintf("Opened documents: %d", count.Get()) }}
}
