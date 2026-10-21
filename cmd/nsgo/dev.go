package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// session is how dev builds and shows one target. start runs after each
// successful build and returns what stops the running app.
type session struct {
	build func(ctx context.Context, p *project) error
	start func(ctx context.Context, p *project) (stop func(), err error)
}

// dev builds and runs the app, then rebuilds and restarts it whenever a
// Go file it compiles from, go.mod or nsgo.yaml changes. A failed build
// keeps the running app.
type devOptions struct {
	target string // overrides nsgo.yaml's dev.target
	port   int    // the web target's
	open   bool   // whether to open the web target in the browser
}

func dev(ctx context.Context, p *project, opts devOptions) error {
	if opts.target != "" {
		p.Dev.Target = opts.target
	}
	var s session
	switch p.Dev.Target {
	case "native":
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("native development runs on macOS; use -target web")
		}
		s = macSession()
	case "web":
		s = webSession(opts)
	case "ios_simulator":
		s = iosSession()
	default:
		return fmt.Errorf("unknown dev target %q; use native, web or ios_simulator", p.Dev.Target)
	}

	stop := func() {}
	defer func() { stop() }()
	fingerprint := ""
	for {
		dirs, err := sourceDirs(ctx, p.Dir)
		if err != nil {
			return err
		}
		fingerprint = scan(dirs)
		step("build %s", p.Dev.Target)
		started := time.Now()
		if err := s.build(ctx, p); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			step("build failed; waiting for changes")
		} else {
			stop()
			stop = func() {}
			step("run (built in %s)", time.Since(started).Round(time.Millisecond))
			if next, err := s.start(ctx, p); err != nil {
				step("run failed: %v", err)
			} else {
				stop = next
			}
		}
		// Wait for a change, then for the editor to finish writing.
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(300 * time.Millisecond):
			}
			if scan(dirs) != fingerprint {
				time.Sleep(100 * time.Millisecond)
				break
			}
		}
		// The target stays; the rest of nsgo.yaml may change.
		if next, err := load(p.Dir); err == nil {
			next.Dev.Target = p.Dev.Target
			*p = *next
		} else {
			step("%v", err)
		}
	}
}

// sourceDirs lists the project's directory and those of every non-standard
// package it compiles, NSGo's own included.
func sourceDirs(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{if not .Standard}}{{.Dir}}{{end}}", ".")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// A project that does not compile still has its own files to watch.
		return []string{dir}, nil
	}
	dirs := []string{dir}
	for _, d := range strings.Fields(string(out)) {
		dirs = append(dirs, d)
	}
	return dirs, nil
}

// scan returns a fingerprint of the watched files' names, sizes and times.
func scan(dirs []string) string {
	var b strings.Builder
	for _, dir := range dirs {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !(strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum" || name == "nsgo.yaml") {
				continue
			}
			if info, err := e.Info(); err == nil {
				fmt.Fprintf(&b, "%s/%s %d %d\n", dir, name, info.Size(), info.ModTime().UnixNano())
			}
		}
	}
	return b.String()
}

// macSession runs the app's bundle binary in the foreground, its output
// in this terminal.
func macSession() session {
	return session{
		build: func(ctx context.Context, p *project) error {
			dir := p.cache("dev", "macos")
			os.RemoveAll(dir)
			_, err := buildMacApp(ctx, p, dir, runtime.GOARCH)
			return err
		},
		start: func(ctx context.Context, p *project) (func(), error) {
			bin := filepath.Join(p.cache("dev", "macos"), p.Name+".app", "Contents", "MacOS", p.Name)
			return startProcess(bin)
		},
	}
}

// iosSession installs the app on the booted simulator, booting one when
// none is, and streams its output here.
func iosSession() session {
	return session{
		build: func(ctx context.Context, p *project) error {
			dir := p.cache("dev", "ios_simulator")
			os.RemoveAll(dir)
			_, err := buildIOSApp(ctx, p, dir, true)
			return err
		},
		start: func(ctx context.Context, p *project) (func(), error) {
			if err := bootSimulator(ctx); err != nil {
				return nil, err
			}
			app := filepath.Join(p.cache("dev", "ios_simulator"), p.Name+".app")
			if err := run(ctx, "xcrun", "simctl", "install", "booted", app); err != nil {
				return nil, err
			}
			stop, err := startProcess("xcrun", "simctl", "launch", "--console-pty", "--terminate-running-process", "booted", p.ID)
			if err != nil {
				return nil, err
			}
			return func() {
				stop()
				exec.Command("xcrun", "simctl", "terminate", "booted", p.ID).Run()
			}, nil
		},
	}
}

var deviceID = regexp.MustCompile(`\(([0-9A-F-]{36})\)`)

func bootSimulator(ctx context.Context) error {
	booted, err := output(ctx, "xcrun", "simctl", "list", "devices", "booted")
	if err != nil {
		return fmt.Errorf("the iOS simulator needs Xcode: %w", err)
	}
	if !strings.Contains(booted, "Booted") {
		available, err := output(ctx, "xcrun", "simctl", "list", "devices", "available")
		if err != nil {
			return err
		}
		var device string
		for _, line := range strings.Split(available, "\n") {
			if m := deviceID.FindStringSubmatch(line); m != nil && strings.Contains(line, "iPhone") {
				device = m[1]
				break
			}
		}
		if device == "" {
			return fmt.Errorf("no iPhone simulator is installed; add one in Xcode")
		}
		step("boot simulator %s", device)
		if err := run(ctx, "xcrun", "simctl", "boot", device); err != nil {
			return err
		}
	}
	// Show the simulator's window; the app runs without it.
	if xcode, err := output(ctx, "xcode-select", "-p"); err == nil {
		exec.CommandContext(ctx, "open", filepath.Join(xcode, "Applications", "Simulator.app")).Run()
	}
	return nil
}

// webSession serves the build on localhost and reloads open pages after
// each rebuild.
func webSession(opts devOptions) session {
	var (
		once    sync.Once
		mu      sync.Mutex
		clients = map[chan struct{}]bool{}
	)
	return session{
		build: func(ctx context.Context, p *project) error {
			return buildWeb(ctx, p, p.cache("dev", "web"), true)
		},
		start: func(ctx context.Context, p *project) (func(), error) {
			var err error
			once.Do(func() {
				mux := http.NewServeMux()
				mux.Handle("/", http.FileServer(http.Dir(p.cache("dev", "web"))))
				mux.HandleFunc("/_nsgo/reload", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Cache-Control", "no-cache")
					w.(http.Flusher).Flush()
					ch := make(chan struct{}, 1)
					mu.Lock()
					clients[ch] = true
					mu.Unlock()
					defer func() {
						mu.Lock()
						delete(clients, ch)
						mu.Unlock()
					}()
					select {
					case <-ch:
						fmt.Fprint(w, "data: reload\n\n")
						w.(http.Flusher).Flush()
					case <-r.Context().Done():
					}
				})
				var ln net.Listener
				if ln, err = net.Listen("tcp", fmt.Sprintf("localhost:%d", opts.port)); err != nil {
					return
				}
				go http.Serve(ln, mux)
				url := fmt.Sprintf("http://localhost:%d/", opts.port)
				step("serving %s", url)
				if opts.open {
					exec.Command("open", url).Start()
				}
			})
			if err != nil {
				return nil, err
			}
			mu.Lock()
			for ch := range clients {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
			mu.Unlock()
			return func() {}, nil
		},
	}
}

// startProcess runs a command with this terminal's output and returns
// what stops it.
func startProcess(name string, args ...string) (func(), error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()
	return func() {
		select {
		case <-done:
			return
		default:
		}
		cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			cmd.Process.Kill()
			<-done
		}
	}, nil
}
