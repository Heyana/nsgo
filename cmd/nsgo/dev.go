package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// session is how dev builds and shows one target. start runs after each
// successful build and returns the running app.
type session struct {
	build func(ctx context.Context, p *project) error
	start func(ctx context.Context, p *project) (app, error)
}

// app is one started dev app. stop ends it, and wait returns once it has
// exited, reporting a failure. The web target only serves, so its wait is
// nil.
type app struct {
	stop func()
	wait func(context.Context) error
}

// devOptions are nsgo dev's flags that do not name a value in the project.
// The rest of the flags have replaced nsgo.yaml's values already, and apply
// replaces them again after each reload.
type devOptions struct {
	apply  func(*project)
	port   int    // the web target's
	open   bool   // whether to open the web target in the browser
	device string // the iPhone the iOS target runs on, by name, UDID, serial or ECID
	team   string // the Apple developer team the iOS target signs with
	fps    bool   // whether the performance monitor shows over the app
}

// dev builds and runs the app. By default it runs the app once and exits
// when the app exits. With nsgo.yaml's hot reload it rebuilds and restarts
// the app whenever a Go file it compiles from, go.mod or nsgo.yaml changes,
// and keeps the running app when a build fails.
func dev(ctx context.Context, p *project, opts devOptions) error {
	if opts.fps && (runtime.GOOS != "darwin" || p.Dev.Target != "native") {
		return fmt.Errorf("-fps needs the native target on macOS (the other hosts have no performance monitor yet)")
	}
	p.Perf = opts.fps
	s, err := sessionFor(p, opts)
	if err != nil {
		return err
	}

	stop := func() {}
	defer func() { stop() }()
	if !p.hotReload() {
		if err := s.build(ctx, p); err != nil {
			return err
		}
		running, err := s.start(ctx, p)
		if err != nil {
			return err
		}
		stop = running.stop
		if running.wait == nil {
			<-ctx.Done()
			return nil
		}
		return running.wait(ctx)
	}

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
				stop = next.stop
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
		// The flags and the rest of nsgo.yaml may have changed too.
		if next, err := load(p.Dir); err == nil {
			next.Perf = opts.fps
			opts.apply(next)
			*p = *next
		} else {
			step("%v", err)
		}
	}
}

// sessionFor picks how dev builds and shows nsgo.yaml's target.
func sessionFor(p *project, opts devOptions) (session, error) {
	switch p.Dev.Target {
	case "native":
		switch runtime.GOOS {
		case "darwin":
			return macSession(), nil
		case "windows":
			return windowsSession(), nil
		case "linux":
			return linuxSession(), nil
		}
		return session{}, fmt.Errorf("native development runs on macOS, Windows and Linux; use -target web")
	case "web":
		return webSession(opts), nil
	case "ios":
		return iosDeviceSession(opts), nil
	case "ios_simulator":
		return iosSession(), nil
	case "android":
		return androidSession(), nil
	default:
		return session{}, fmt.Errorf("unknown dev target %q; use native, web, ios, ios_simulator or android", p.Dev.Target)
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
		start: func(ctx context.Context, p *project) (app, error) {
			bin := filepath.Join(p.cache("dev", "macos"), p.Name+".app", "Contents", "MacOS", p.Name)
			stop, wait, err := startProcess(bin)
			if err != nil {
				return app{}, err
			}
			return app{stop: stop, wait: wait}, nil
		},
	}
}

// windowsSession builds the app as a Windows executable and runs it in
// the foreground.
func windowsSession() session {
	return session{
		build: func(ctx context.Context, p *project) error {
			dir := p.cache("dev", "windows")
			os.RemoveAll(dir)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			return buildWindows(ctx, p, dir, runtime.GOARCH)
		},
		start: func(ctx context.Context, p *project) (app, error) {
			bin := filepath.Join(p.cache("dev", "windows"), p.Name+".exe")
			stop, wait, err := startProcess(bin)
			if err != nil {
				return app{}, err
			}
			return app{stop: stop, wait: wait}, nil
		},
	}
}

// linuxSession builds the app as a Linux executable and runs it in the
// foreground.
func linuxSession() session {
	return session{
		build: func(ctx context.Context, p *project) error {
			dir := p.cache("dev", "linux")
			os.RemoveAll(dir)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			return buildLinux(ctx, p, dir, runtime.GOARCH)
		},
		start: func(ctx context.Context, p *project) (app, error) {
			bin := filepath.Join(p.cache("dev", "linux"), p.Name)
			stop, wait, err := startProcess(bin)
			if err != nil {
				return app{}, err
			}
			return app{stop: stop, wait: wait}, nil
		},
	}
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
		start: func(ctx context.Context, p *project) (app, error) {
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
					name, args := openCommand(url)
					exec.Command(name, args...).Start()
				}
			})
			if err != nil {
				return app{}, err
			}
			mu.Lock()
			for ch := range clients {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
			mu.Unlock()
			return app{stop: func() {}}, nil
		},
	}
}

// startProcess runs a command with this terminal's output. stop ends the
// process; wait returns its error once it exits, or nil when stop or ctx
// ended it.
func startProcess(name string, args ...string) (stop func(), wait func(context.Context) error, err error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			select {
			case <-done:
				return
			default:
			}
			if runtime.GOOS == "windows" {
				// Ask the app's windows to close, so it saves and exits;
				// a process without one is killed below.
				exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
			} else {
				cmd.Process.Signal(os.Interrupt)
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				cmd.Process.Kill()
				<-done
			}
		})
	}
	wait = func(ctx context.Context) error {
		select {
		case <-done:
			return waitErr
		case <-ctx.Done():
			stop()
			return nil
		}
	}
	return stop, wait, nil
}
