package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/cmd/nsgo/apk"
	androidruntime "github.com/oneo-me/nsgo/platform/android/runtime"
)

// androidComponent is the activity the runtime starts for an app.
const androidComponent = "/dev.oneo.nsgo.NsgoActivity"

// buildAndroidAPK writes <dir>/<name>.apk for 64-bit devices. The app is a
// Go executable the runtime starts, so packaging needs no NDK, and the
// archive, manifest and signature are written here rather than by the
// Android SDK's tools.
func buildAndroidAPK(ctx context.Context, p *project, dir string) (string, error) {
	tmp, err := os.MkdirTemp("", "nsgo-android-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	binary := filepath.Join(tmp, "libnsgo.so")
	if err := goBuild(ctx, p, binary, []string{"GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0"}); err != nil {
		return "", err
	}
	app, err := os.ReadFile(binary)
	if err != nil {
		return "", err
	}
	key, err := debugKey(p)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, p.Name+".apk")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	err = apk.Write(f, androidruntime.Manifest, apk.Manifest{
		Package:     p.ID,
		Label:       p.Name,
		VersionName: p.Version,
		VersionCode: apk.VersionCode(p.Version),
	}, []apk.File{
		{Name: "classes.dex", Data: androidruntime.ClassesDex},
		{Name: "lib/arm64-v8a/libnsgo.so", Data: app},
	}, key)
	if err != nil {
		return "", err
	}
	return path, nil
}

// debugKey returns the key the project's builds are signed with, making
// one on first use. Android rejects an update signed by another key, so
// the key stays with the project's development files.
func debugKey(p *project) (*apk.Key, error) {
	path := p.cache("android", "debug.pem")
	data, err := os.ReadFile(path)
	if err == nil {
		return parseKey(data)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key, err := apk.NewKey()
	if err != nil {
		return nil, err
	}
	data = append(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key.Private)}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: key.Cert.Raw})...)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func parseKey(data []byte) (*apk.Key, error) {
	key := &apk.Key{}
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		switch block.Type {
		case "RSA PRIVATE KEY":
			private, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return nil, err
			}
			key.Private = private
		case "CERTIFICATE":
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			key.Cert = cert
		}
	}
	if key.Private == nil || key.Cert == nil {
		return nil, errors.New("the Android debug key is not a private key and a certificate; delete .nsgo/android/debug.pem to make a new one")
	}
	return key, nil
}

// androidSession installs the app on the connected device each build and
// streams its log here until it exits.
func androidSession() session {
	return session{
		build: func(ctx context.Context, p *project) error {
			dir := p.cache("dev", "android")
			os.RemoveAll(dir)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			_, err := buildAndroidAPK(ctx, p, dir)
			return err
		},
		start: func(ctx context.Context, p *project) (app, error) {
			d, err := adbDevice(ctx)
			if err != nil {
				return app{}, err
			}
			path := filepath.Join(p.cache("dev", "android"), p.Name+".apk")
			step("install on %s", d.serial)
			if err := d.run(ctx, "install", "-r", path); err != nil {
				return app{}, err
			}
			// Start the log before the app, so its first lines arrive.
			stopLog, _, err := startProcess(d.adb, "-s", d.serial, "logcat", "-s", "nsgo:I", "AndroidRuntime:E")
			if err != nil {
				return app{}, err
			}
			if err := d.run(ctx, "shell", "am", "start", "-S", "-n", p.ID+androidComponent); err != nil {
				stopLog()
				return app{}, err
			}
			return app{
				stop: func() {
					stopLog()
					d.run(context.Background(), "shell", "am", "force-stop", p.ID)
				},
				wait: func(ctx context.Context) error {
					defer stopLog()
					for {
						select {
						case <-ctx.Done():
							return nil
						case <-time.After(time.Second):
						}
						alive, err := d.running(ctx, p.ID)
						if err != nil {
							return err
						}
						if !alive {
							return nil
						}
					}
				},
			}, nil
		},
	}
}

// device is one adb target.
type device struct {
	adb    string
	serial string
}

// run runs an adb command on the device.
func (d device) run(ctx context.Context, args ...string) error {
	return run(ctx, d.adb, append([]string{"-s", d.serial}, args...)...)
}

// running reports whether the app's process is alive.
func (d device) running(ctx context.Context, id string) (bool, error) {
	out, err := exec.CommandContext(ctx, d.adb, "-s", d.serial, "shell", "pidof", id).Output()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return false, fmt.Errorf("adb shell pidof: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// adbDevice finds adb and the device to run on. ANDROID_SERIAL names one
// when several are connected.
func adbDevice(ctx context.Context) (device, error) {
	adb, err := findAdb()
	if err != nil {
		return device{}, err
	}
	out, err := output(ctx, adb, "devices")
	if err != nil {
		return device{}, err
	}
	var serials []string
	for _, line := range strings.Split(out, "\n")[1:] {
		if f := strings.Fields(line); len(f) >= 2 && f[1] == "device" {
			serials = append(serials, f[0])
		}
	}
	serial := os.Getenv("ANDROID_SERIAL")
	switch {
	case serial != "":
		found := false
		for _, s := range serials {
			found = found || s == serial
		}
		if !found {
			return device{}, fmt.Errorf("ANDROID_SERIAL names %s, which is not connected; adb devices lists %s", serial, list(serials))
		}
	case len(serials) == 0:
		return device{}, errors.New("no Android device: connect one, turn on USB debugging, accept the prompt and run adb devices")
	case len(serials) > 1:
		return device{}, fmt.Errorf("several Android devices are connected: set ANDROID_SERIAL to one of %s", list(serials))
	default:
		serial = serials[0]
	}
	return device{adb: adb, serial: serial}, nil
}

// list joins device serials for a message.
func list(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

// findAdb finds the Android SDK's adb: on PATH, or in an SDK that
// ANDROID_HOME or the usual install directory names.
func findAdb() (string, error) {
	name := "adb"
	if runtime.GOOS == "windows" {
		name = "adb.exe"
	}
	if path, err := exec.LookPath("adb"); err == nil {
		return path, nil
	}
	var sdkDirs []string
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if dir := os.Getenv(env); dir != "" {
			sdkDirs = append(sdkDirs, dir)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		sdkDirs = append(sdkDirs,
			filepath.Join(home, "Android"),
			filepath.Join(home, "Library", "Android", "sdk"),
			filepath.Join(home, "AppData", "Local", "Android", "Sdk"))
	}
	for _, dir := range sdkDirs {
		path := filepath.Join(dir, "platform-tools", name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", errors.New("Android development needs adb: install the Android SDK's platform-tools and put them on PATH")
}
