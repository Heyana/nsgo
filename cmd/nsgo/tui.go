package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// tuiTarget names a console binary separately from the same OS's GUI app.
func tuiTarget(goos, arch string) string {
	if goos == "darwin" {
		goos = "macos"
	}
	return "tui_" + goos + "_" + goArchName(arch)
}
func tuiEnv(target string) ([]string, error) {
	if target == "tui" {
		target = tuiTarget(runtime.GOOS, runtime.GOARCH)
	}
	parts := strings.Split(target, "_")
	if len(parts) != 3 || parts[0] != "tui" {
		return nil, fmt.Errorf("invalid terminal target %q", target)
	}
	osname := parts[1]
	if osname == "macos" {
		osname = "darwin"
	}
	if osname != "darwin" && osname != "linux" && osname != "windows" {
		return nil, fmt.Errorf("invalid terminal OS %q", parts[1])
	}
	if parts[2] != "arm64" && parts[2] != "x64" {
		return nil, fmt.Errorf("invalid terminal architecture %q", parts[2])
	}
	return []string{"GOOS=" + osname, "GOARCH=" + goArch(parts[2]), "CGO_ENABLED=0"}, nil
}
func tuiBinary(p *project, dir, target string) string {
	extension := ""
	if strings.Contains(target, "_windows_") {
		extension = ".exe"
	}
	return filepath.Join(dir, p.Name+extension)
}

// buildTUI makes a console executable and its resources, with no GUI
// subsystem, platform toolkit, window metadata or installer.
func buildTUI(ctx context.Context, p *project, dir, target string) error {
	env, err := tuiEnv(target)
	if err != nil {
		return err
	}
	if err = goBuildWith(ctx, p, tuiBinary(p, dir, target), env, buildExtra{tags: []string{"nsgotui"}, target: target}); err != nil {
		return err
	}
	if err := copyResources(p, dir); err != nil {
		return err
	}
	return addTUILibraries(ctx, p, dir, target)
}

func tuiSession(opts devOptions) session {
	target := tuiTarget(runtime.GOOS, runtime.GOARCH)
	return session{
		build: func(ctx context.Context, p *project) error {
			dir := p.cache("dev", "tui")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			return buildTUI(ctx, p, dir, target)
		},
		start: func(ctx context.Context, p *project) (app, error) {
			if opts.debugger != nil {
				return app{}, fmt.Errorf("terminal development does not support -debug")
			}
			logPath := p.cache("tui.log")
			if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
				return app{}, err
			}
			log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				return app{}, err
			}
			note("terminal diagnostics: %s; Ctrl+C exits", rel(logPath))
			out.Lock()
			out.clear()
			out.terminalLog = log
			out.Unlock()
			var once sync.Once
			restore := func() { once.Do(func() { out.Lock(); out.terminalLog = nil; out.Unlock(); log.Close() }) }
			cmd := exec.Command(tuiBinary(p, p.cache("dev", "tui"), target), opts.appFlags()...)
			cmd.Dir = p.Dir
			cmd.Env = append(os.Environ(), devEnv(opts)...)
			cmd.Stdin = os.Stdin
			cmd.Stdout, cmd.Stderr = log, log
			setupTUIProcess(cmd)
			if err := cmd.Start(); err != nil {
				restore()
				return app{}, err
			}
			done := make(chan struct{})
			var waitErr error
			go func() { waitErr = cmd.Wait(); close(done) }()
			stop := func() {
				select {
				case <-done:
				default:
					stopTUIProcess(cmd)
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						cmd.Process.Kill()
						<-done
					}
				}
				restore()
			}
			wait := func(ctx context.Context) error {
				select {
				case <-done:
					restore()
					return waitErr
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return app{stop: stop, wait: wait}, nil
		},
	}
}

func tuiTestRunner(ctx context.Context, p *project, dir string) (targetRunner, error) {
	target := tuiTarget(runtime.GOOS, runtime.GOARCH)
	if err := buildTUI(ctx, p, dir, target); err != nil {
		return targetRunner{}, err
	}
	return targetRunner{run: func(ctx context.Context, args, env []string, out io.Writer) (int, error) {
		// The backend draws on /dev/tty or CONOUT$, while the test protocol
		// is buffered until Fini has restored the caller's terminal.
		var output bytes.Buffer
		code, err := runLocal(ctx, tuiBinary(p, dir, target), args, env, &output)
		if _, writeErr := io.Copy(out, &output); err == nil {
			err = writeErr
		}
		return code, err
	}, close: func() {}}, nil
}

func runTUIExecutable(ctx context.Context, bin string) error {
	cmd := exec.Command(bin)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	setupTUIProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		stopTUIProcess(cmd)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
			<-done
		}
		return ctx.Err()
	}
}
