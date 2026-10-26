package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/template"
	"time"
)

// buildAll writes one directory per target: <output>/<name>-<version>-<target>.
func buildAll(ctx context.Context, p *project) error {
	p.Release = true
	p.Perf = p.Build.FPS
	buildTargets, err := expandTargets(p.Build.Targets)
	if err != nil {
		return err
	}
	if p.Build.embed {
		if buildTargets, err = embedOnly(buildTargets); err != nil {
			return err
		}
	}
	var total int64
	started := time.Now()
	header(p.Name, p.Version, strings.Join(buildTargets, ", "))
	for _, target := range buildTargets {
		dir := filepath.Join(p.Build.Output, fmt.Sprintf("%s-%s-%s", p.Name, p.Version, target))
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		progress := begin("building %s", target)
		if err := buildTarget(ctx, p, target, dir); err != nil {
			progress.fail()
			os.RemoveAll(dir)
			return fmt.Errorf("%s: %w", target, err)
		}
		size := dirSize(dir)
		total += size
		progress.done(fmt.Sprintf("built %s", target), formatSize(size))
		note("%s", rel(dir))
	}
	buildSummary(p, len(buildTargets), total, time.Since(started))
	if p.openOutput() {
		openDir(p.Build.Output)
	}
	if p.Build.run {
		for _, target := range buildTargets {
			if runnable(target) {
				return runPackage(ctx, p, target, filepath.Join(p.Build.Output, fmt.Sprintf("%s-%s-%s", p.Name, p.Version, target)))
			}
		}
		return fmt.Errorf("-run: none of %s runs on this machine", strings.Join(buildTargets, ", "))
	}
	return nil
}

// buildSummary closes a build of several targets with their number, time
// and space; one target's line already says its own.
func buildSummary(p *project, targets int, size int64, took time.Duration) {
	if targets < 2 {
		return
	}
	done("built %d targets in %s (%s, %s)", targets, rel(p.Build.Output), formatDuration(took), formatSize(size))
}

// dirSize is how much space the files under dir take.
func dirSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// openDir shows dir in the host's file manager. A build is complete
// without it, so failing to open is only a warning.
func openDir(dir string) {
	note("open %s", rel(dir))
	opener := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		opener = "open"
	case "windows":
		opener = "explorer"
	}
	if err := exec.Command(opener, dir).Start(); err != nil {
		warn("could not open %s: %v", rel(dir), err)
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
	"android_arm64", "android_x64",
	"web",
}

// expandTargets replaces "all" with every target that needs only Go and
// Xcode, checks every name and drops duplicates, so each package is built
// once. android_x64 stays out of "all" because it needs an NDK.
func expandTargets(names []string) ([]string, error) {
	var out []string
	for _, name := range names {
		expanded := []string{name}
		if name == "all" {
			expanded = goAndXcodeTargets()
		} else if !slices.Contains(targets, name) {
			return nil, fmt.Errorf("unknown target %q; use all or one of %s", name, strings.Join(targets, ", "))
		}
		for _, target := range expanded {
			if !slices.Contains(out, target) {
				out = append(out, target)
			}
		}
	}
	return out, nil
}

// goAndXcodeTargets are the packages a Mac with Go and Xcode can build.
// android_x64 is omitted: Go links android/amd64 only with cgo, so it needs an NDK.
func goAndXcodeTargets() []string {
	out := make([]string, 0, len(targets)-1)
	for _, target := range targets {
		if target != "android_x64" {
			out = append(out, target)
		}
	}
	return out
}

func buildTarget(ctx context.Context, p *project, target, dir string) error {
	_, arch, _ := strings.Cut(target, "_")
	if p.Build.embed {
		switch target {
		case "ios", "ios_simulator":
			// One framework holds both.
			return buildIOSFramework(ctx, p, dir)
		case "android_arm64", "android_x64":
			return buildAndroidAAR(ctx, p, dir, goArch(arch))
		}
		return fmt.Errorf("-embed builds for %s; desktop hosts embed apps with nsgo.NewView", strings.Join(embedTargets, ", "))
	}
	switch target {
	case "macos_arm64", "macos_x64":
		_, err := buildMacApp(ctx, p, dir, goArch(arch))
		return err
	case "windows_arm64", "windows_x64":
		return buildWindows(ctx, p, dir, goArch(arch))
	case "ios":
		app, err := buildIOSApp(ctx, p, dir, false)
		if err != nil {
			return err
		}
		if err := signForStore(ctx, p, app); err != nil {
			return err
		}
		return writeIPA(app, filepath.Join(dir, p.Name+".ipa"))
	case "ios_simulator":
		_, err := buildIOSApp(ctx, p, dir, true)
		return err
	case "web":
		return buildWeb(ctx, p, dir, false)
	case "linux_arm64", "linux_x64":
		return buildLinux(ctx, p, dir, goArch(arch))
	case "android_arm64", "android_x64":
		_, err := buildAndroidAPK(ctx, p, dir, goArch(arch))
		return err
	}
	return fmt.Errorf("unknown target %q; use all or one of %s", target, strings.Join(targets, ", "))
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
	return goBuildWith(ctx, p, out, env, nil, ldflags...)
}

// goBuildWith is goBuild with flags of its own for go build.
func goBuildWith(ctx context.Context, p *project, out string, env, flags []string, ldflags ...string) error {
	args := append([]string{"build"}, flags...)
	args = append(args, "-ldflags", p.ldflags(ldflags...), "-o", out)
	if p.testBinary {
		// nsgo test -target: the package's tests, with the backend's test
		// host, in place of the app.
		args = []string{"test", "-c", "-tags", "nsgotest", "-ldflags", p.ldflags(ldflags...), "-o", out}
	}
	if p.Release {
		args = append(args, "-trimpath", "-buildvcs=false")
	}
	if p.Dev.Client {
		args = append(args, "-tags", "nsgodev")
	}
	cmd := exec.CommandContext(ctx, "go", append(args, ".")...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), env...)
	// The tool's chatter only matters when it fails: the step's line says
	// what it did, and a failure shows the whole output under its error.
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go %s failed\n%s", args[0], strings.TrimSpace(output.String()))
	}
	return nil
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
	resources := filepath.Join(app, "Contents", "Resources")
	if err := copyResources(p, resources); err != nil {
		return "", err
	}
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return "", err
	}
	icon, err := writeMacIcon(ctx, p, resources)
	if err != nil {
		return "", err
	}
	if err := writePlist(filepath.Join(app, "Contents", "Info.plist"), p, macPlist, icon); err != nil {
		return "", err
	}
	return app, run(ctx, "codesign", "--force", "--sign", "-", app)
}

// buildWindows writes <dir>/<name>.exe with the icon linked in and the
// resources beside it.
func buildWindows(ctx context.Context, p *project, dir, arch string) error {
	removeIcon, err := writeWindowsIcon(p, arch)
	if err != nil {
		return err
	}
	err = goBuild(ctx, p, filepath.Join(dir, p.Name+".exe"),
		[]string{"GOOS=windows", "GOARCH=" + arch, "CGO_ENABLED=0"}, "-H=windowsgui")
	removeIcon()
	if err != nil {
		return err
	}
	return copyResources(p, dir)
}

// buildLinux writes <dir>/<name> with the resources and icon beside it.
func buildLinux(ctx context.Context, p *project, dir, arch string) error {
	if err := goBuild(ctx, p, filepath.Join(dir, p.Name),
		[]string{"GOOS=linux", "GOARCH=" + arch, "CGO_ENABLED=0"}); err != nil {
		return err
	}
	if err := copyResources(p, dir); err != nil {
		return err
	}
	return writeLinuxIcon(p, dir)
}

// buildWeb writes the site to dir: index.html, wasm_exec.js, app.wasm
// and the resources beside them. A release names every file but the page
// and its manifest.webmanifest by its content, so hosts cache them for
// good, and the page tells the runtime the name each resource got; a dev
// build keeps the names and follows nsgo dev's server.
func buildWeb(ctx context.Context, p *project, dir string, dev bool) error {
	wasm := filepath.Join(dir, "app.wasm")
	if err := goBuild(ctx, p, wasm, []string{"GOOS=js", "GOARCH=wasm"}); err != nil {
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
	img, err := p.iconImage()
	if err != nil {
		return err
	}
	page := map[string]any{"Name": p.Name, "Reload": dev, "Wasm": "app.wasm", "Exec": "wasm_exec.js",
		"Color": appearanceColor(p.Appearance), "Appearance": p.Appearance}
	// write puts data in dir, by its content's name in a release, and
	// returns the name.
	write := func(name string, data []byte) (string, error) {
		if !dev {
			name = hashedName(name, data)
		}
		return name, os.WriteFile(filepath.Join(dir, name), data, 0o644)
	}
	if page["Exec"], err = write("wasm_exec.js", js); err != nil {
		return err
	}
	if dev {
		if err := copyResources(p, dir); err != nil {
			return err
		}
	} else {
		data, err := os.ReadFile(wasm)
		if err != nil {
			return err
		}
		name := hashedName("app.wasm", data)
		if err := os.Rename(wasm, filepath.Join(dir, name)); err != nil {
			return err
		}
		page["Wasm"] = name
		names, err := copyHashedResources(p, dir)
		if err != nil {
			return err
		}
		if len(names) > 0 {
			data, err := json.Marshal(names)
			if err != nil {
				return err
			}
			page["Resources"] = string(data)
		}
	}
	if img != nil {
		if page["Icon"], err = write("icon.png", scaledPNG(img, 256)); err != nil {
			return err
		}
		if page["TouchIcon"], err = write("apple-touch-icon.png", scaledPNG(img, 180)); err != nil {
			return err
		}
	}
	if !dev {
		var icons []webIcon
		for _, size := range []int{192, 512} {
			if img == nil {
				break
			}
			name, err := write(fmt.Sprintf("icon-%d.png", size), scaledPNG(img, size))
			if err != nil {
				return err
			}
			icons = append(icons, webIcon{Src: name, Sizes: fmt.Sprintf("%dx%d", size, size), Type: "image/png"})
		}
		if err := writeWebManifest(p, dir, icons); err != nil {
			return err
		}
		page["Manifest"] = true
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()
	return indexHTML.Execute(f, page)
}

var indexHTML = template.Must(template.New("index").Parse(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>{{.Name}}</title>
{{- if .Appearance}}
<meta name="color-scheme" content="{{.Appearance}}">
{{- end}}
{{- if .Color}}
<meta name="theme-color" content="{{.Color}}">
{{- end}}
{{- if .Icon}}
<link rel="icon" href="{{.Icon}}">
<link rel="apple-touch-icon" href="{{.TouchIcon}}">
{{- end}}
{{- if .Manifest}}
<link rel="manifest" href="manifest.webmanifest">
{{- end}}
<style>html, body { margin: 0; height: 100%;{{if .Color}} background: {{.Color}};{{end}} }</style>
<script src="{{.Exec}}"></script>
</head>
<body>
<script>
{{- if .Resources}}
globalThis.nsgoResources = {{.Resources}};
{{- end}}
const go = new Go();
WebAssembly.instantiateStreaming(fetch("{{.Wasm}}"), go.importObject).then((r) => go.run(r.instance));
{{- if .Reload}}
// nsgo dev's server: each new build loads, a failed one shows over the app.
const events = new EventSource("/_nsgo/events");
let overlay;
events.onmessage = (e) => {
  const [kind, ...rest] = e.data.split(" ");
  if (kind === "reload") location.reload();
  if (kind === "ok" && overlay) { overlay.remove(); overlay = null; }
  if (kind !== "error") return;
  overlay?.remove();
  overlay = document.createElement("div");
  overlay.style.cssText = "position:fixed;inset:0;z-index:2147483647;background:rgba(17,17,17,.94);color:#f2f2f2;padding:48px 20px;overflow:auto;font:13px ui-monospace,monospace";
  const title = document.createElement("div");
  title.textContent = "Build failed";
  title.style.cssText = "color:#ff5c5c;font:bold 18px system-ui;margin-bottom:12px";
  const text = document.createElement("pre");
  text.textContent = JSON.parse(rest.join(" "));
  text.style.whiteSpace = "pre-wrap";
  const copy = document.createElement("button");
  copy.textContent = "Copy";
  copy.onclick = (e) => { e.stopPropagation(); navigator.clipboard.writeText(text.textContent); };
  overlay.append(title, text, copy);
  overlay.onclick = () => { overlay.remove(); overlay = null; };
  document.body.append(overlay);
};
{{- end}}
</script>
</body>
</html>
`))

// writePlist writes t's plist for p, with extra entries, such as the
// icon's, at the end of its top dictionary.
func writePlist(path string, p *project, t *template.Template, extra string) error {
	var out bytes.Buffer
	if err := t.Execute(&out, p); err != nil {
		return err
	}
	plist := out.String()
	if extra != "" {
		end := strings.LastIndex(plist, "</dict>")
		plist = plist[:end] + "\t" + strings.TrimSpace(extra) + "\n" + plist[end:]
	}
	return os.WriteFile(path, []byte(plist), 0o644)
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
	<key>CFBundleVersion</key><string>{{.BundleVersion}}</string>
`

var macPlist = template.Must(template.New("mac").Parse(plistHead + `	<key>LSMinimumSystemVersion</key><string>11.0</string>
	<key>NSHighResolutionCapable</key><true/>
{{- if eq .Appearance "light"}}
	<key>NSRequiresAquaSystemAppearance</key><true/>
{{- end}}
</dict>
</plist>
`))

func rel(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	return path
}
