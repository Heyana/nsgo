// Command installedupdates exercises the public updater from an installed app.
// Its loopback-only witness records progress and the version/PID after restart.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/nsgo-dev/nsgo/updater"
)

//go:embed version.txt
var version string

func main() { nsgo.Run(Root, nsgo.WindowSize(480, 320)) }

type record struct {
	Version    string
	PID        int
	At         time.Time
	State      updater.State
	StageBytes int64
}

func Root(cx *core.Scope) core.Node {
	version = strings.TrimSpace(version)
	dir := filepath.Join(cx.CacheDir(), "installed-update-witness")
	if err := os.MkdirAll(dir, 0700); err != nil {
		panic(err)
	}
	logFile := filepath.Join(dir, "events.jsonl")
	var latest atomic.Value
	cx.Effect(func() {
		s := updater.Status(cx)
		var staged int64
		entries, _ := os.ReadDir(filepath.Join(cx.CacheDir(), "nsgo.updater"))
		for _, entry := range entries {
			if info, err := entry.Info(); err == nil && !info.IsDir() {
				staged += info.Size()
			}
		}
		r := record{Version: version, PID: os.Getpid(), At: time.Now().UTC(), State: s, StageBytes: staged}
		latest.Store(r)
		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		err = json.NewEncoder(file).Encode(r)
		file.Close()
		if err != nil {
			panic(err)
		}
	})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(latest.Load())
	})
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, logFile) })
	mux.HandleFunc("POST /check", func(w http.ResponseWriter, r *http.Request) {
		cx.Post(func() { updater.Check(cx) })
		w.WriteHeader(http.StatusAccepted)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	cx.Cleanup(func() { server.Close() })
	go func() {
		// A restarted process may begin just before its predecessor releases the port.
		var listener net.Listener
		var err error
		for range 50 {
			listener, err = net.Listen("tcp", "127.0.0.1:47820")
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 16, Padding: core.All(24)}, Children: []core.Node{
		core.Text{Content: "Installed update fixture " + version, TestID: "version"},
		ui.Button{Text: "Check for updates", OnPress: func() { updater.Check(cx) }, TestID: "check"},
		core.Text{ContentFunc: func() string {
			s := updater.Status(cx)
			return fmt.Sprintf("Phase %d: %d / %d bytes", s.Phase, s.Loaded, s.Total)
		}, TestID: "progress"},
		ui.Progress{Style: core.Style{Width: core.Pt(300)}, Label: "Update download", Value: func() float32 { return float32(updater.Status(cx).Fraction() * 100) }},
	}}
}
