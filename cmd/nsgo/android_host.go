package main

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
)

type androidHostDev struct {
	opts              devOptions
	device            device
	endpoint, token   string
	dir, bin, apk     string
	inputs, installed string
	packageID, port   string
}

func androidHostSession(opts devOptions) (session, error) {
	switch runtime.GOOS {
	case "darwin", "windows", "linux":
	default:
		return session{}, fmt.Errorf("-android-host is unavailable on %s", runtime.GOOS)
	}
	id, err := newToken()
	if err != nil {
		return session{}, err
	}
	s := &androidHostDev{opts: opts, endpoint: "nsgo-dev-" + id[:32], token: id[32:]}
	return session{build: s.build, start: s.start, watch: androidHostInputs, close: s.close}, nil
}

func (s *androidHostDev) build(ctx context.Context, p *project) error {
	if s.device.serial == "" {
		d, err := adbDevice(ctx, s.opts.device)
		if err != nil {
			return err
		}
		s.device = d
	}
	inputs := androidHostInputs(p)
	root := p.cache("dev", "android-host")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(root, "build-")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
		}
	}()
	bin := filepath.Join(dir, "app")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if err := goBuildWith(ctx, p, bin, []string{"GOOS=" + runtime.GOOS, "GOARCH=" + runtime.GOARCH},
		buildExtra{tags: []string{"nsgoandroidhost"}}); err != nil {
		return err
	}
	var path string
	if inputs != s.installed || s.packageID != p.ID {
		files, err := androidRuntimeFiles(p)
		if err != nil {
			return err
		}
		// Later Go builds can read fold geometry without reinstalling the runtime.
		path, err = writeAndroidPackage(ctx, p, dir, ".apk", files, []string{windowExtensions}, s.endpoint+"\n"+s.token, apk.Write)
		if err != nil {
			return err
		}
	}
	keep = true
	s.dir, s.bin, s.apk, s.inputs = dir, bin, path, inputs
	return nil
}

func (s *androidHostDev) start(ctx context.Context, p *project) (app, error) {
	dir := s.dir
	defer func() {
		if s.dir == dir { // No process claimed this build.
			os.RemoveAll(dir)
			s.dir = ""
		}
	}()
	if s.apk != "" {
		s.stopActivity()
		event("install on %s", s.device.serial)
		if err := s.device.run(ctx, "install", "-r", s.apk); err != nil {
			return app{}, err
		}
		s.packageID = p.ID
		port, err := s.device.output(ctx, "forward", "tcp:0", "localabstract:"+s.endpoint)
		if err != nil {
			return app{}, err
		}
		s.port = strings.TrimSpace(port)
		if n, err := strconv.Atoi(s.port); err != nil || n <= 0 || n > 65535 {
			return app{}, fmt.Errorf("adb forward returned invalid port %q", port)
		}
		if err := s.device.run(ctx, "shell", "am", "start", "-W", "-S", "-n", p.ID+androidComponent); err != nil {
			return app{}, err
		}
		s.installed = s.inputs
	}
	env := append(devEnv(s.opts),
		"NSGO_ANDROID_HOST_ADDRESS="+net.JoinHostPort("127.0.0.1", s.port),
		"NSGO_ANDROID_HOST_TOKEN="+s.token,
		"NSGO_DATA_DIR="+p.cache("dev", "android-host", "data"),
		"NSGO_CACHE_DIR="+p.cache("dev", "android-host", "cache"))
	stop, wait, err := startProcessIn(p.Dir, env, s.bin, s.opts.appFlags()...)
	if err != nil {
		return app{}, err
	}
	s.dir = ""
	return app{stop: func() {
		stop()
		os.RemoveAll(dir)
	}, wait: wait}, nil
}

func (s *androidHostDev) stopActivity() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if s.port != "" {
		s.device.run(ctx, "forward", "--remove", "tcp:"+s.port)
		s.port = ""
	}
	if s.packageID != "" {
		s.device.run(ctx, "shell", "am", "force-stop", s.packageID)
		s.packageID = ""
	}
	s.installed = ""
}

func (s *androidHostDev) close() {
	s.stopActivity()
	if s.dir != "" {
		os.RemoveAll(s.dir)
	}
}

// androidHostInputs covers the project files installed on the device. Go sources and files only embedded in Go are inputs to the host process.
func androidHostInputs(p *project) string {
	var files strings.Builder
	add := func(path string) {
		if path == "" {
			return
		}
		if info, err := os.Stat(path); err == nil {
			fmt.Fprintf(&files, "%s %d %d\n", path, info.Size(), info.ModTime().UnixNano())
		}
	}
	add(filepath.Join(p.Dir, "nsgo.yaml"))
	add(filepath.Join(p.Dir, assetsDir, "Icon.png"))
	add(filepath.Join(p.Dir, assetsDir, "Icon-dev.png"))
	add(p.Icon.PNG)
	if a := p.Icon.Android; a != nil {
		add(a.Foreground)
		add(a.Background)
		add(a.Monochrome)
	}
	for _, path := range p.Android.Classes {
		add(p.resolve(path))
	}
	root := filepath.Join(p.Dir, resourcesDir)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		add(path)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(&files, "%v\n", err)
	}
	return files.String()
}
