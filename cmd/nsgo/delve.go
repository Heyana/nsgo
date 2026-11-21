package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sync"
	"time"
)

func checkDevDebug(p *project, opts devOptions) error {
	if !opts.debug {
		return nil
	}
	switch {
	case opts.release:
		return errors.New("-debug needs a development build; remove -release")
	case p.Dev.Target != "native":
		return errors.New("-debug runs desktop apps under Delve; use -target native")
	case runtime.GOOS != "darwin" && runtime.GOOS != "windows" && runtime.GOOS != "linux":
		return errors.New("-debug runs on macOS, Windows and Linux")
	}
	return nil
}

// devDebugger keeps the loopback address of one development run, including
// any rebuilds. Each build takes a new path so Windows can keep the previous
// executable open until its replacement builds.
type devDebugger struct {
	path, address  string
	root, buildDir string
}

func newDevDebugger(p *project) (*devDebugger, error) {
	path, err := exec.LookPath("dlv")
	if err != nil {
		return nil, errors.New("-debug needs Delve (dlv on PATH): https://github.com/go-delve/delve")
	}
	cache := p.cache("dev")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(cache, "debug-")
	if err != nil {
		return nil, err
	}
	return &devDebugger{path: path, root: root}, nil
}

func (d *devDebugger) nextBuild() error {
	var err error
	d.buildDir, err = os.MkdirTemp(d.root, "app-")
	return err
}

func (o devOptions) desktopBuildDir(p *project, target string) string {
	if o.debugger != nil {
		return o.debugger.buildDir
	}
	return p.cache("dev", target)
}

func (d *devDebugger) start(workDir, appDir string, env []string, bin string, flags []string) (app, error) {
	if d.address == "" {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return app{}, err
		}
		d.address = l.Addr().String()
		if err := l.Close(); err != nil {
			return app{}, err
		}
	}
	address := d.address
	args := []string{"exec", "--headless", "--api-version=2", "--accept-multiclient", "--continue", "--listen=" + address, bin, "--"}
	stopProcess, waitProcess, err := startProcessIn(workDir, env, d.path, append(args, flags...)...)
	if err != nil {
		return app{}, fmt.Errorf("start Delve: %w", err)
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			// Detach ends the server and its debuggee on all three hosts,
			// including Windows, where Delve ignores console interrupts.
			delveCall(address, "Detach", struct{ Kill bool }{true}, &struct{}{})
			stopProcess()
			os.RemoveAll(appDir)
		})
	}
	wait := func(ctx context.Context) error {
		exited := make(chan error, 1)
		go func() { exited <- waitProcess(ctx) }()
		tick := time.NewTicker(300 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case err := <-exited:
				return err
			case <-tick.C:
				// A multiclient Delve server keeps running when the
				// program exits. Preserve dev's app-exit behavior.
				err := delveCall(address, "State", struct{ NonBlocking bool }{true}, &struct{}{})
				if err != nil {
					if m := delveExited.FindStringSubmatch(err.Error()); m != nil {
						if m[1] == "0" {
							return nil
						}
						return fmt.Errorf("the app exited with status %s", m[1])
					}
				}
			}
		}
	}
	return app{stop: stop, wait: wait}, nil
}

var delveExited = regexp.MustCompile(`^Process \d+ has exited with status (-?\d+)$`)

func delveCall(address, method string, in, out any) error {
	const timeout = 300 * time.Millisecond
	conn, err := net.DialTimeout("tcp4", address, timeout)
	if err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		return err
	}
	c := rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn))
	defer c.Close()
	return c.Call("RPCServer."+method, in, out)
}
