package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

// androidComponent is the activity the runtime starts for an app.
const androidComponent = "/dev.oneo.nsgo.NsgoActivity"

// androidAutomationExtra is the boolean launch intent extra that tells NsgoActivity to create TestAgent in a debuggable build (TestAgent.java).
const androidAutomationExtra = "nsgo.automation"

// androidAPI is the manifest's minSdkVersion, so the NDK's clang targets
// the same API as the runtime.
const androidAPI = 28

// androidPlayTargetSDK is the target API Google Play requires for new apps
// and updates from August 31, 2026.
const androidPlayTargetSDK = 36

func androidTarget(target string) bool {
	return target == "android_arm64" || target == "android_x64" || target == "android_universal"
}

func androidTargetNotice(p *project, targets []string) string {
	if p.Android.TargetSDK == 0 || p.Android.TargetSDK >= androidPlayTargetSDK || !slices.ContainsFunc(targets, androidTarget) {
		return ""
	}
	return fmt.Sprintf("android.target_sdk %d is below Google Play's required targetSdkVersion %d; set android.target_sdk to %d or higher, or omit it to use NSGo's default", p.Android.TargetSDK, androidPlayTargetSDK, androidPlayTargetSDK)
}

// buildAndroidBinary builds the app's executable for goarch in tmp and
// returns it with its ABI's name.
func buildAndroidBinary(ctx context.Context, p *project, tmp, goarch string) ([]byte, string, error) {
	abi, err := androidABI(goarch)
	if err != nil {
		return nil, "", err
	}
	binary := filepath.Join(tmp, "libnsgo.so")
	env := []string{"GOOS=android", "GOARCH=" + goarch}
	if goarch == "arm64" {
		env = append(env, "CGO_ENABLED=0")
	} else {
		cc, err := androidClang(goarch)
		if err != nil {
			return nil, "", err
		}
		env = append(env, "CGO_ENABLED=1", "CC="+cc)
	}
	if err := goBuild(ctx, p, binary, env); err != nil {
		return nil, "", err
	}
	app, err := os.ReadFile(binary)
	return app, abi, err
}

// buildAndroidAPK writes <dir>/<name>.apk for one architecture. The app is
// a Go executable the runtime starts, and the archive, manifest and
// signature are written here rather than by the Android SDK's tools. Go
// links arm64 without cgo; amd64 needs the NDK's clang, as Go supports no
// other external linker for it.
func buildAndroidAPK(ctx context.Context, p *project, dir, goarch string) (string, error) {
	return buildAndroidPackage(ctx, p, dir, ".apk", apk.Write, goarch)
}

// buildAndroidAAB writes one signed Android App Bundle for the selected ABIs.
func buildAndroidAAB(ctx context.Context, p *project, dir string, goarches ...string) (string, error) {
	packs, err := p.storeAssetPacks()
	if err != nil {
		return "", err
	}
	write := func(w io.Writer, template []byte, m apk.Manifest, files []apk.File, key *apk.Key) error {
		m.AssetPacks = packs
		return apk.WriteBundle(w, template, m, files, key)
	}
	return buildAndroidPackage(ctx, p, dir, ".aab", write, goarches...)
}

func releaseAndroidBundle(ctx context.Context, p *project, dir, target string, goarches ...string) error {
	aab, err := buildAndroidAAB(ctx, p, dir, goarches...)
	if err != nil {
		return err
	}
	return p.releaseCopy(aab, p.releaseName(target, ".aab"))
}

// buildAndroidPackage combines the selected executables with one copy of the
// runtime, manifest and resources, then signs the resulting APK or App Bundle.
func buildAndroidPackage(ctx context.Context, p *project, dir, ext string,
	write func(io.Writer, []byte, apk.Manifest, []apk.File, *apk.Key) error, goarches ...string) (string, error) {
	files, err := androidRuntimeFiles(p)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "nsgo-android-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	var libraries []string
	for _, goarch := range goarches {
		app, abi, err := buildAndroidBinary(ctx, p, tmp, goarch)
		if err != nil {
			return "", err
		}
		files = append(files, apk.File{Name: "lib/" + abi + "/libnsgo.so", Data: app})
		if ghostty, err := androidGhosttyLibrary(ctx, p, goarch, abi); err != nil {
			return "", err
		} else if ghostty.Data != nil {
			files = append(files, ghostty)
		}
		required, err := androidLibraries(ctx, p, goarch, app)
		if err != nil {
			return "", err
		}
		for _, library := range required {
			if !slices.Contains(libraries, library) {
				libraries = append(libraries, library)
			}
		}
	}
	return writeAndroidPackage(ctx, p, dir, ext, files, libraries, "", write)
}

func androidRuntimeFiles(p *project) ([]apk.File, error) {
	if err := p.androidWidgetRuntime(androidruntime.ClassesDex); err != nil {
		return nil, err
	}
	classes, err := androidClassFiles(p)
	if err != nil {
		return nil, err
	}
	return append([]apk.File{{Name: "classes.dex", Data: androidruntime.ClassesDex}}, classes...), nil
}

func writeAndroidPackage(ctx context.Context, p *project, dir, ext string, files []apk.File, libraries []string, hostConfig string,
	write func(io.Writer, []byte, apk.Manifest, []apk.File, *apk.Key) error) (string, error) {
	if ext == ".aab" {
		stage(ctx, "packaging the App Bundle")
	} else {
		stage(ctx, "packaging the APK")
	}
	key, err := releaseKey(ctx, p)
	if err != nil {
		return "", err
	}
	if hostConfig != "" {
		files = append(files, apk.File{Name: "assets/__nsgo_android_host", Data: []byte(hostConfig)})
	}
	// Resources go in the APK's assets, where the app reads them.
	err = walkMobileResources(p, func(rel string, data []byte) error {
		if hostConfig != "" && rel == "__nsgo_android_host" {
			return errors.New("resources/__nsgo_android_host is reserved by the Android development runtime")
		}
		files = append(files, apk.File{Name: "assets/" + rel, Data: data})
		return nil
	})
	if err != nil {
		return "", err
	}
	img, err := p.androidIconImage()
	if err != nil {
		return "", err
	}
	var icon []byte
	if img != nil {
		icon = scaledPNG(img, apk.IconSize)
	}
	adaptive, legacy, err := p.androidAdaptiveIcon()
	if err != nil {
		return "", err
	}
	if icon == nil && legacy != nil {
		icon = scaledPNG(legacy, apk.IconSize)
	}
	launch, err := p.androidLaunchScreen()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, p.Name+ext)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	names, err := p.appNames()
	if err != nil {
		return "", err
	}
	widgets, err := p.androidWidgets()
	if err != nil {
		return "", err
	}
	err = write(f, androidruntime.Manifest, apk.Manifest{
		Widgets:             widgets,
		Package:             p.ID,
		Label:               p.Name,
		Names:               names.android(),
		VersionName:         p.Version,
		URLSchemes:          p.URLSchemes,
		AppLinks:            p.AppLinks,
		ShareTypes:          p.Android.ShareTypes,
		VersionCode:         p.versionCode(),
		Icon:                icon,
		Adaptive:            adaptive,
		Theme:               androidThemes[p.Appearance],
		Launch:              launch,
		Debuggable:          androidDebuggable(p) || hostConfig != "",
		CrashReportBuild:    crashReportBuild(p),
		Permissions:         p.androidPermissions(),
		TargetSDK:           p.Android.TargetSDK,
		LoopbackCleartext:   p.Android.LoopbackCleartext,
		Cleartext:           p.Android.UsesCleartextTraffic,
		Overlay:             p.androidOverlay(),
		Recording:           p.Android.Recording,
		Motion:              p.Android.BackgroundMotion,
		Location:            p.androidLocation(),
		Libraries:           libraries,
		Game:                p.Game,
		Gamepad:             p.Gamepad,
		BackgroundDownloads: p.Android.BackgroundDownloads,
	}, files, key)
	if err != nil {
		return "", err
	}
	return path, f.Close()
}

func crashReportBuild(p *project) string {
	if p.Release {
		return p.BundleVersion()
	}
	return ""
}

// windowExtensions is the device's library of its folds and hinges, which the runtime reads by reflection (Folds.java).
const windowExtensions = "androidx.window.extensions"

// geometryReader is the function an app reads its fold geometry with. Its name is in the executable's function table only when the linker keeps the function: as the app's code reaches it, or as reflection by names that are not constants, as tsx's, keeps every exported method of the types in interfaces.
const geometryReader = "github.com/nsgo-dev/nsgo/core.(*Env).Geometry"

// nsgoInternal is the import path prefix of NSGo's internal packages.
const nsgoInternal = "github.com/nsgo-dev/nsgo/internal/"

// androidGhosttyLibrary returns the prebuilt libghostty-vt for an app that links base/terminal. Android's loader searches the APK's ABI directory, so the library is a sibling of libnsgo.so rather than a Go-embedded payload.
func androidGhosttyLibrary(ctx context.Context, p *project, goarch, abi string) (apk.File, error) {
	dir, err := linkedDir(ctx, p, []string{"GOOS=android", "GOARCH=" + goarch, "CGO_ENABLED=0"}, ghosttyPackage)
	if err != nil {
		return apk.File{}, err
	}
	if dir == "" {
		return apk.File{}, nil
	}
	return androidGhosttyFile(dir, goarch, abi)
}

func androidGhosttyFile(dir, goarch, abi string) (apk.File, error) {
	path := filepath.Join(dir, "lib", "android-"+goarch, "libghostty-vt.so")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		// Until #1452 commits an Android prebuild the package has none for this ABI; the AAR builds without it, as before, rather than failing every embedded app that links the terminal.
		return apk.File{}, nil
	}
	if err != nil {
		return apk.File{}, fmt.Errorf("base/terminal: read Android libghostty-vt: %w", err)
	}
	return apk.File{Name: "lib/" + abi + "/libghostty-vt.so", Data: data}, nil
}

// androidLibraries are the device's libraries app, the app's linked executable for goarch, uses where the device has them: the window extensions for an app that reads its fold geometry, so no other app declares them. The name in the function table alone does not tell, as reflection may have kept it, so the app's sources must also select a Geometry.
// androidGhosttyLibrary returns the prebuilt libghostty-vt for an app that
// links base/terminal. Android's loader searches the APK's ABI directory, so
// the library is a sibling of libnsgo.so rather than a Go-embedded payload.
func androidGhosttyLibrary(ctx context.Context, p *project, goarch, abi string) (apk.File, error) {
	if goarch != "arm64" {
		return apk.File{}, nil
	}
	dir, err := linkedDir(ctx, p, []string{"GOOS=android", "GOARCH=" + goarch, "CGO_ENABLED=0"}, ghosttyPackage)
	if err != nil {
		return apk.File{}, err
	}
	if dir == "" {
		return apk.File{}, nil
	}
	return androidGhosttyFile(dir, goarch, abi)
}

func androidGhosttyFile(dir, goarch, abi string) (apk.File, error) {
	path := filepath.Join(dir, "lib", "android-"+goarch, "libghostty-vt.so")
	data, err := os.ReadFile(path)
	if err != nil {
		return apk.File{}, fmt.Errorf("base/terminal: read Android libghostty-vt: %w", err)
	}
	return apk.File{Name: "lib/" + abi + "/libghostty-vt.so", Data: data}, nil
}

func androidLibraries(ctx context.Context, p *project, goarch string, app []byte) ([]string, error) {
	if !bytes.Contains(app, []byte(geometryReader)) {
		return nil, nil
	}
	cgo := "CGO_ENABLED=0"
	if goarch != "arm64" {
		cgo = "CGO_ENABLED=1"
	}
	reads, err := selectsGeometry(ctx, p, []string{"GOOS=android", "GOARCH=" + goarch, cgo})
	if err != nil || !reads {
		return nil, err
	}
	return []string{windowExtensions}, nil
}

// selectsGeometry reports whether a Go file of the packages the app's build for env links selects Geometry from anything but an imported package, as reading core.Env.Geometry does; core.Geometry, the type, does not count, nor do NSGo's internal packages, which pass the geometry on but never read it for an app. It reads the files build constraints leave out as well, so a tag the build adds cannot hide one. Without types it counts a field or method of another type named Geometry too, declaring the optional library where it was not needed rather than leaving it out where it was.
func selectsGeometry(ctx context.Context, p *project, env []string) (bool, error) {
	files := "{{range .GoFiles}}\t{{.}}{{end}}{{range .CgoFiles}}\t{{.}}{{end}}{{range .IgnoredGoFiles}}\t{{.}}{{end}}"
	args := []string{"list", "-deps"}
	var extra []string
	if p.testBinary {
		files += "{{range .TestGoFiles}}\t{{.}}{{end}}{{range .XTestGoFiles}}\t{{.}}{{end}}"
		args = append(args, "-test")
		extra = append(extra, "nsgotest")
	}
	tags, err := goTags(ctx, p, env, extra...)
	if err != nil {
		return false, err
	}
	args = append(args, tagsArgs(tags)...)
	args = append(args, "-f", "{{if not .Standard}}{{.ImportPath}}\t{{.Name}}\t{{.Dir}}"+files+"{{end}}", ".")
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("go list: %w\n%s", err, stderr.Bytes())
	}
	names := map[string]string{}
	var pkgs [][]string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		// A test's variant of a package, "p [p.test]", is p to what imports it.
		path, _, _ := strings.Cut(fields[0], " ")
		names[path] = fields[1]
		if !strings.HasPrefix(path, nsgoInternal) {
			pkgs = append(pkgs, fields[2:])
		}
	}
	fset := token.NewFileSet()
	for _, pkg := range pkgs {
		for _, name := range pkg[1:] {
			src, err := os.ReadFile(filepath.Join(pkg[0], name))
			if err != nil {
				return false, err
			}
			if !bytes.Contains(src, []byte("Geometry")) {
				continue
			}
			f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
			if err != nil {
				// A file left out of the build need not parse; one in it has, as the app built.
				continue
			}
			if fileSelectsGeometry(f, names) {
				return true, nil
			}
		}
	}
	return false, nil
}

// fileSelectsGeometry reports whether f selects Geometry from anything but a package it imports, whose names are by import path in names.
func fileSelectsGeometry(f *ast.File, names map[string]string) bool {
	imported := map[string]bool{}
	for _, spec := range f.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if spec.Name != nil {
			imported[spec.Name.Name] = true
		} else if name, ok := names[path]; ok {
			imported[name] = true
		} else {
			imported[path[strings.LastIndex(path, "/")+1:]] = true
		}
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok && s.Sel.Name == "Geometry" {
			if id, ok := s.X.(*ast.Ident); !ok || !imported[id.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

// androidThemes are the activity's themes, so the window starts in the
// app's appearance before the first frame.
var androidThemes = map[string]string{
	"light": "@android:style/Theme.DeviceDefault.Light.NoActionBar",
	"dark":  "@android:style/Theme.DeviceDefault.NoActionBar",
}

// versionCode is Android's integer version: BuildNumber, else Version's.
func (p *project) versionCode() int {
	if p.BuildNumber > 0 {
		return p.BuildNumber
	}
	return apk.VersionCode(p.Version)
}

// debugKey returns the key the project's builds are signed with, making
// one on first use. Android rejects an update signed by another key, so
// the key stays with the project's development files.
func debugKey(p *project) (*apk.Key, error) {
	// Targets built at once sign with the one key the first makes.
	debugKeyMu.Lock()
	defer debugKeyMu.Unlock()
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

var debugKeyMu sync.Mutex

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

// androidDebuggable reports whether the Android package for p sets android:debuggable="true": test builds (nsgo test -target android) and automation dev builds (nsgo dev -target android -automation) need it so the runtime accepts their launch extras and starts TestAgent; release builds never do.
func androidDebuggable(p *project) bool {
	return p.testBinary || (p.Dev.Automation && !p.Release)
}

// androidExtras passes the engine's flags as an intent's extras, which
// the activity gives the app as flags again: --nsgo-fps as --ez nsgo-fps
// true, --nsgo-lang=de as --es nsgo-lang de.
func androidExtras(flags []string) []string {
	var extras []string
	for _, f := range flags {
		name, value, ok := strings.Cut(strings.TrimPrefix(f, "--"), "=")
		if ok {
			extras = append(extras, "--es", name, value)
		} else {
			extras = append(extras, "--ez", name, "true")
		}
	}
	return extras
}

// androidStartArgs returns the adb arguments that launch id's activity with opts' engine flags and, under -automation, the automation extra.
func androidStartArgs(id string, opts devOptions) []string {
	args := append([]string{"shell", "am", "start", "-S", "-n", id + androidComponent}, androidExtras(opts.appFlags())...)
	if opts.automation {
		args = append(args, "--ez", androidAutomationExtra, "true")
	}
	return args
}

// androidReverseSpec is the adb reverse socket spec for a local TCP port.
func androidReverseSpec(port int) string {
	return fmt.Sprintf("tcp:%d", port)
}

// androidSession installs the app on the connected device each build and
// streams its log here until it exits. It builds for the device's own ABI,
// so a device and an emulator need no flag.
func androidSession(opts devOptions) session {
	return session{
		build: func(ctx context.Context, p *project) error {
			// The app connects to the device's loopback, regardless of the host's bind address.
			p.devURL, p.devToken = opts.server.urlAt("127.0.0.1"), opts.server.token
			d, err := adbDevice(ctx, opts.device)
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
			d, err := adbDevice(ctx, opts.device)
			if err != nil {
				return app{}, err
			}
			addr := opts.server.addr
			port := androidReverseSpec(addr.Port)
			destination := port
			if !addr.IP.IsUnspecified() {
				// A port alone makes the ADB server dial localhost.
				destination = "tcp:" + addr.String()
			}
			if err := d.run(ctx, "reverse", port, destination); err != nil {
				return app{}, err
			}
			removeReverse := func() {
				// Only this run's rule: another app's dev server may use another port.
				d.run(context.Background(), "reverse", "--remove", port)
			}
			path := filepath.Join(p.cache("dev", "android"), p.Name+".apk")
			event("install on %s", d.serial)
			if err := d.run(ctx, "install", "-r", path); err != nil {
				removeReverse()
				return app{}, err
			}
			// Start the log before the app, so its first lines arrive.
			stopLog, _, err := startProcess(d.adb, "-s", d.serial, "logcat", "-s", "nsgo:I", "AndroidRuntime:E")
			if err != nil {
				removeReverse()
				return app{}, err
			}
			if err := d.run(ctx, androidStartArgs(p.ID, opts)...); err != nil {
				stopLog()
				removeReverse()
				return app{}, err
			}
			return app{
				stop: func() {
					stopLog()
					d.run(context.Background(), "shell", "am", "force-stop", p.ID)
					removeReverse()
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
	case "armeabi-v7a", "armeabi":
		// Go links android/arm only with cgo, which needs the NDK: NSGo builds every Android target but the x86_64 emulator's without it.
		return "", fmt.Errorf("the device reports ABI %q, 32-bit ARM, which NSGo does not package: Go links it only with cgo and the Android NDK (https://github.com/nsgo-dev/nsgo/issues/41); use an arm64 device or an x86_64 emulator", abi)
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

// adbDevice finds adb and the device to run on: the one wanted names, by
// serial or, for an emulator, its AVD's name; else ANDROID_SERIAL's, else
// the only one connected.
func adbDevice(ctx context.Context, wanted string) (device, error) {
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
	serial := wanted
	if serial == "" {
		serial = os.Getenv("ANDROID_SERIAL")
	}
	switch {
	case serial != "":
		if slices.Contains(serials, serial) {
			break
		}
		for _, s := range serials {
			if !strings.HasPrefix(s, "emulator-") {
				continue
			}
			if name, err := output(ctx, adb, "-s", s, "emu", "avd", "name"); err == nil && strings.TrimSpace(strings.Split(name, "\n")[0]) == serial {
				return device{adb: adb, serial: s}, nil
			}
		}
		return device{}, fmt.Errorf("no connected device is %s; adb devices lists %s", serial, list(serials))
	case len(serials) == 0:
		return device{}, errors.New("no Android device: connect one, turn on USB debugging, accept the prompt and run adb devices")
	case len(serials) > 1:
		return device{}, fmt.Errorf("several Android devices are connected: pass -device or set ANDROID_SERIAL to one of %s", list(serials))
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
