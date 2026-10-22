package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/template"
)

// buildAll writes one directory per target: <output>/<name>-<version>-<target>.
func buildAll(ctx context.Context, p *project) error {
	p.Release = true
	for _, target := range p.Build.Targets {
		if !slices.Contains(targets, target) {
			return fmt.Errorf("unknown target %q; use one of %s", target, strings.Join(targets, ", "))
		}
	}
	for _, target := range p.Build.Targets {
		dir := filepath.Join(p.Build.Output, fmt.Sprintf("%s-%s-%s", p.Name, p.Version, target))
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		step("build %s", target)
		if err := buildTarget(ctx, p, target, dir); err != nil {
			os.RemoveAll(dir)
			return fmt.Errorf("%s: %w", target, err)
		}
		step("  -> %s", rel(dir))
	}
	if p.openOutput() {
		openDir(p.Build.Output)
	}
	return nil
}

// openDir shows dir in the host's file manager. A build is complete
// without it, so failing to open is only a warning.
func openDir(dir string) {
	step("open %s", rel(dir))
	opener := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		opener = "open"
	case "windows":
		opener = "explorer"
	}
	if err := exec.Command(opener, dir).Start(); err != nil {
		step("could not open %s: %v", rel(dir), err)
	}
}

// openCommand returns the command that shows a URL in the user's browser.
func openCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	}
	return "xdg-open", []string{url}
}

// targets are the names nsgo.yaml and -target accept, each one package.
var targets = []string{
	"macos_arm64", "macos_x64",
	"windows_arm64", "windows_x64",
	"linux_arm64", "linux_x64",
	"ios", "ios_simulator",
	"android_arm64",
	"web",
}

func buildTarget(ctx context.Context, p *project, target, dir string) error {
	osName, arch, _ := strings.Cut(target, "_")
	switch target {
	case "macos_arm64", "macos_x64":
		_, err := buildMacApp(ctx, p, dir, goArch(arch))
		return err
	case "windows_arm64", "windows_x64":
		if err := goBuild(ctx, p, filepath.Join(dir, p.Name+".exe"),
			[]string{"GOOS=" + osName, "GOARCH=" + goArch(arch), "CGO_ENABLED=0"}, "-H=windowsgui"); err != nil {
			return err
		}
		return copyAssets(p, dir)
	case "ios":
		_, err := buildIOSApp(ctx, p, dir, false)
		return err
	case "ios_simulator":
		_, err := buildIOSApp(ctx, p, dir, true)
		return err
	case "web":
		return buildWeb(ctx, p, dir, false)
	case "linux_arm64", "linux_x64":
		return fmt.Errorf("there is no Linux backend yet")
	case "android_arm64":
		return fmt.Errorf("APK packaging is not available yet")
	}
	return fmt.Errorf("unknown target; use one of %s", strings.Join(targets, ", "))
}

// goArch maps x64, the name packages use, to Go's amd64.
func goArch(arch string) string {
	if arch == "x64" {
		return "amd64"
	}
	return arch
}

// goBuild builds the project's main package to out.
func goBuild(ctx context.Context, p *project, out string, env []string, ldflags ...string) error {
	args := []string{"build", "-ldflags", p.ldflags(ldflags...), "-o", out}
	if p.Release {
		args = append(args, "-trimpath", "-buildvcs=false")
	}
	cmd := exec.CommandContext(ctx, "go", append(args, ".")...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func run(ctx context.Context, name string, args ...string) error {
	// Tools' chatter only matters when they fail.
	if out, err := exec.CommandContext(ctx, name, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, out)
	}
	return nil
}

func output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// buildMacApp writes <dir>/<name>.app for one architecture.
func buildMacApp(ctx context.Context, p *project, dir, arch string) (string, error) {
	app := filepath.Join(dir, p.Name+".app")
	bin := filepath.Join(app, "Contents", "MacOS", p.Name)
	if err := goBuild(ctx, p, bin, []string{"GOOS=darwin", "GOARCH=" + arch, "CGO_ENABLED=0"}); err != nil {
		return "", err
	}
	if err := writePlist(filepath.Join(app, "Contents", "Info.plist"), p, macPlist); err != nil {
		return "", err
	}
	if err := copyAssets(p, filepath.Join(app, "Contents", "MacOS")); err != nil {
		return "", err
	}
	return app, run(ctx, "codesign", "--force", "--sign", "-", app)
}

// buildIOSApp writes <dir>/<name>.app for the simulator or for devices.
// Device builds are left unsigned for the developer's own identity.
func buildIOSApp(ctx context.Context, p *project, dir string, simulator bool) (string, error) {
	sdkName, triple := "iphoneos", "arm64-apple-ios15.0"
	if simulator {
		sdkName, triple = "iphonesimulator", "arm64-apple-ios15.0-simulator"
	}
	sdk, err := output(ctx, "xcrun", "--sdk", sdkName, "--show-sdk-path")
	if err != nil {
		return "", fmt.Errorf("the iOS SDK needs Xcode: %w", err)
	}
	clang, err := output(ctx, "xcrun", "--sdk", sdkName, "-f", "clang")
	if err != nil {
		return "", err
	}
	// Go links iOS only through Xcode's linker; NSGo itself has no C.
	flags := "-isysroot " + sdk + " -target " + triple
	app := filepath.Join(dir, p.Name+".app")
	env := []string{"GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1",
		"CC=" + clang + " " + flags, "CGO_CFLAGS=" + flags, "CGO_LDFLAGS=" + flags}
	// Go's runtime objects target a newer iOS than 15.0; the linker's
	// warning about it is noise.
	if err := goBuild(ctx, p, filepath.Join(app, p.Name), env, "-extldflags=-Wl,-w"); err != nil {
		return "", err
	}
	if err := writePlist(filepath.Join(app, "Info.plist"), p, iosPlist); err != nil {
		return "", err
	}
	if err := copyAssets(p, app); err != nil {
		return "", err
	}
	if simulator {
		return app, run(ctx, "codesign", "--force", "--sign", "-", app)
	}
	return app, nil
}

// buildWeb writes index.html, wasm_exec.js and app.wasm to dir.
func buildWeb(ctx context.Context, p *project, dir string, reload bool) error {
	if err := goBuild(ctx, p, filepath.Join(dir, "app.wasm"), []string{"GOOS=js", "GOARCH=wasm"}); err != nil {
		return err
	}
	goroot, err := output(ctx, "go", "env", "GOROOT")
	if err != nil {
		return err
	}
	js, err := os.ReadFile(filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), js, 0o644); err != nil {
		return err
	}
	if err := copyAssets(p, dir); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()
	return indexHTML.Execute(f, map[string]any{"Name": p.Name, "Reload": reload})
}

var indexHTML = template.Must(template.New("index").Parse(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>{{.Name}}</title>
<style>html, body { margin: 0; height: 100%; }</style>
<script src="wasm_exec.js"></script>
</head>
<body>
<script>
const go = new Go();
WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject).then((r) => go.run(r.instance));
{{- if .Reload}}
new EventSource("/_nsgo/reload").onmessage = () => location.reload();
{{- end}}
</script>
</body>
</html>
`))

func writePlist(path string, p *project, t *template.Template) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, p)
}

const plistHead = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key><string>{{.Name}}</string>
	<key>CFBundleIdentifier</key><string>{{.ID}}</string>
	<key>CFBundleName</key><string>{{.Name}}</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>{{.Version}}</string>
	<key>CFBundleVersion</key><string>{{.Version}}</string>
`

var macPlist = template.Must(template.New("mac").Parse(plistHead + `	<key>LSMinimumSystemVersion</key><string>11.0</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
`))

// The SDK requires the scene life cycle; platform/ios registers
// NsgoSceneDelegate.
var iosPlist = template.Must(template.New("ios").Parse(plistHead + `	<key>MinimumOSVersion</key><string>15.0</string>
	<key>UILaunchScreen</key><dict/>
	<key>UIRequiresFullScreen</key><true/>
	<key>UIDeviceFamily</key><array><integer>1</integer><integer>2</integer></array>
	<key>UIApplicationSceneManifest</key>
	<dict>
		<key>UIApplicationSupportsMultipleScenes</key><false/>
		<key>UISceneConfigurations</key>
		<dict>
			<key>UIWindowSceneSessionRoleApplication</key>
			<array><dict>
				<key>UISceneConfigurationName</key><string>Default</string>
				<key>UISceneDelegateClassName</key><string>NsgoSceneDelegate</string>
			</dict></array>
		</dict>
	</dict>
</dict>
</plist>
`))

func step(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }

func rel(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	return path
}
