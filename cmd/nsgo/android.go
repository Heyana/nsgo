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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/cmd/nsgo/apk"
	androidruntime "github.com/oneo-me/nsgo/platform/android/runtime"
)

// androidComponent is the activity the runtime starts for an app.
const androidComponent = "/dev.oneo.nsgo.NsgoActivity"

// androidAPI is the manifest's minSdkVersion, so the NDK's clang targets
// the same API as the runtime.
const androidAPI = 28

// buildAndroidAPK writes <dir>/<name>.apk for one architecture. The app is
// a Go executable the runtime starts, and the archive, manifest and
// signature are written here rather than by the Android SDK's tools. Go
// links arm64 without cgo; amd64 needs the NDK's clang, as Go supports no
// other external linker for it.
func buildAndroidAPK(ctx context.Context, p *project, dir, goarch string) (string, error) {
	tmp, err := os.MkdirTemp("", "nsgo-android-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	abi, err := androidABI(goarch)
	if err != nil {
		return "", err
	}
	binary := filepath.Join(tmp, "libnsgo.so")
	env := []string{"GOOS=android", "GOARCH=" + goarch}
	if goarch == "arm64" {
		env = append(env, "CGO_ENABLED=0")
	} else {
		cc, err := androidClang(goarch)
		if err != nil {
			return "", err
		}
		env = append(env, "CGO_ENABLED=1", "CC="+cc)
	}
	if err := goBuild(ctx, p, binary, env); err != nil {
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
	files := []apk.File{
		{Name: "classes.dex", Data: androidruntime.ClassesDex},
		{Name: "lib/" + abi + "/libnsgo.so", Data: app},
	}
	// Resources go in the APK's assets, where the app reads them.
	err = walkResources(p, func(rel string, data []byte) error {
		files = append(files, apk.File{Name: "assets/" + rel, Data: data})
		return nil
	})
	if err != nil {
		return "", err
	}
	img, err := p.iconImage()
	if err != nil {
		return "", err
	}
	var icon []byte
	if img != nil {
		icon = scaledPNG(img, apk.IconSize)
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
		Icon:        icon,
	}, files, key)
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
// streams its log here until it exits. It builds for the device's own ABI,
// so a device and an emulator need no flag.
func androidSession() session {
	return session{
		build: func(ctx context.Context, p *project) error {
			d, err := adbDevice(ctx)
			if err != nil {
				return err
			}
			goarch, err := d.goarch(ctx)
			if err != nil {
				return err
			}
			dir := p.cache("dev", "android")
			os.RemoveAll(dir)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			_, err = buildAndroidAPK(ctx, p, dir, goarch)
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

// output runs an adb command on the device and returns its output.
func (d device) output(ctx context.Context, args ...string) (string, error) {
	return output(ctx, d.adb, append([]string{"-s", d.serial}, args...)...)
}

// goarch returns the Go architecture of the device's primary ABI.
func (d device) goarch(ctx context.Context) (string, error) {
	abi, err := d.output(ctx, "shell", "getprop", "ro.product.cpu.abi")
	if err != nil {
		return "", err
	}
	return goarchForABI(strings.TrimSpace(abi))
}

// goarchForABI maps Android's ABI names to the two NSGo packages.
func goarchForABI(abi string) (string, error) {
	switch abi {
	case "arm64-v8a":
		return "arm64", nil
	case "x86_64":
		return "amd64", nil
	}
	return "", fmt.Errorf("the device reports ABI %q, which NSGo does not package; use an arm64 device or an x86_64 emulator", abi)
}

// androidABI maps a Go architecture to the APK's native library directory.
func androidABI(goarch string) (string, error) {
	switch goarch {
	case "arm64":
		return "arm64-v8a", nil
	case "amd64":
		return "x86_64", nil
	}
	return "", fmt.Errorf("Android packages support arm64 and amd64, not %s", goarch)
}

// androidClang finds the NDK's clang for one architecture. Go links
// android/amd64 only with cgo, so the emulator target needs it; arm64
// builds with Go alone.
func androidClang(goarch string) (string, error) {
	ndk, err := findNDK()
	if err != nil {
		return "", err
	}
	name := androidTriple(goarch) + strconv.Itoa(androidAPI) + "-clang"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	prebuilt := filepath.Join(ndk, "toolchains", "llvm", "prebuilt")
	hosts, err := os.ReadDir(prebuilt)
	if err != nil {
		return "", err
	}
	for _, host := range hosts {
		path := filepath.Join(prebuilt, host.Name(), "bin", name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("the NDK at %s has no %s; install a recent NDK with sdkmanager", ndk, name)
}

// androidTriple is the NDK's target triple prefix for one architecture.
func androidTriple(goarch string) string {
	switch goarch {
	case "arm64":
		return "aarch64-linux-android"
	case "amd64":
		return "x86_64-linux-android"
	case "386":
		return "i686-linux-android"
	case "arm":
		return "armv7a-linux-androideabi"
	}
	return ""
}

// findNDK returns the newest NDK of the SDK or the one the environment
// names, so an x86_64 emulator build can find its clang.
func findNDK() (string, error) {
	for _, env := range []string{"ANDROID_NDK_HOME", "ANDROID_NDK_ROOT"} {
		if dir := os.Getenv(env); dir != "" {
			if isNDK(dir) {
				return dir, nil
			}
			return "", fmt.Errorf("%s is %s, which is not an NDK", env, dir)
		}
	}
	var roots []string
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if dir := os.Getenv(env); dir != "" {
			roots = append(roots, dir)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots,
			filepath.Join(home, "Android"),
			filepath.Join(home, "Library", "Android", "sdk"),
			filepath.Join(home, "AppData", "Local", "Android", "Sdk"))
	}
	for _, root := range roots {
		entries, err := os.ReadDir(filepath.Join(root, "ndk"))
		if err != nil {
			continue
		}
		var versions []string
		for _, entry := range entries {
			if entry.IsDir() {
				versions = append(versions, entry.Name())
			}
		}
		slices.SortFunc(versions, compareVersions)
		for i := len(versions) - 1; i >= 0; i-- {
			dir := filepath.Join(root, "ndk", versions[i])
			if isNDK(dir) {
				return dir, nil
			}
		}
	}
	return "", errors.New("the x86_64 emulator target needs an Android NDK: install one with sdkmanager \"ndk;27.2.12479018\" and set ANDROID_NDK_HOME; arm64 devices and emulators need none")
}

// isNDK reports whether dir holds the NDK's toolchain layout.
func isNDK(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "toolchains", "llvm", "prebuilt"))
	return err == nil && info.IsDir()
}

// compareVersions orders dotted numeric versions, so the newest NDK wins.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv int
		if i < len(as) {
			av, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(bs[i])
		}
		if av != bv {
			return av - bv
		}
	}
	return 0
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
