package moduletest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// unlinked lists what an app that shows one text must not link, by
// package, for each target ("" for all): what only an app's own use
// brings in, or what a backend leaves to the system
// (docs/architecture/layers.md, What an app links).
var unlinked = map[string][]string{
	"": {
		// Images download through the system's HTTP client (imageload.HTTP).
		"net/http", "crypto/tls", "crypto/x509",
		// Shaders and GPU views compile when an app makes one: no package variable, init or element case of its own reaches the compilers, and each target's writer is listed below.
		"github.com/nsgo-dev/nsgo/internal/shader", "github.com/nsgo-dev/nsgo/internal/shader/wgsl", "go/parser",
		// Only nsgo dev -automation's builds link the agent and what tests run on.
		"testing", "github.com/nsgo-dev/nsgo/nsgotest", "github.com/nsgo-dev/nsgo/internal/automation",
		// Only builds tagged nsgotui (nsgo -target tui) link the terminal backend and its cell library.
		"github.com/nsgo-dev/nsgo/internal/platform/tui", "github.com/gdamore/tcell/v3",
	},
	// The page decodes and encodes images with the browser's codecs, and links its 2D canvas player only when an app draws a canvas or icon.
	"js": {
		"image/png", "image/jpeg", "image/gif", "golang.org/x/image/webp", "golang.org/x/image/draw",
		"github.com/nsgo-dev/nsgo/internal/platform/canvas2d",
		"github.com/nsgo-dev/nsgo/internal/shaderir/glsl",
	},
	// Android's DNS resolver links only when an app uses Go networking of its own.
	"android": {"net", "github.com/nsgo-dev/nsgo/internal/shaderir/glsl"},
	// Only Android reads its resources from a zip, the APK.
	"darwin":  {"archive/zip", "github.com/nsgo-dev/nsgo/internal/shaderir/msl"},
	"windows": {"archive/zip", "github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"},
	"linux":   {"archive/zip", "github.com/nsgo-dev/nsgo/internal/shaderir/glsl"},
}

// featureSymbols names each backend's entry points for shaders, canvases, videos, web views, native components and GPU views. Minimal apps must link none of them; testdata/features must link all of them.
var featureSymbols = map[string][]string{
	"darwin": {
		"github.com/nsgo-dev/nsgo/internal/platform/apple.NewShaderLayer",
		"github.com/nsgo-dev/nsgo/internal/platform/apple.PlayOver",
		"github.com/nsgo-dev/nsgo/internal/platform/apple.(*Video).Configure",
		"github.com/nsgo-dev/nsgo/internal/platform/apple.(*Web).Configure",
		"github.com/nsgo-dev/nsgo/internal/platform/macos.(*surface).configureNative",
		"github.com/nsgo-dev/nsgo/internal/platform/macos.(*surface).SubmitGPU",
		"github.com/nsgo-dev/nsgo/internal/platform/apple.NewGPULayer",
	},
	"windows": {
		"github.com/nsgo-dev/nsgo/internal/platform/windows.(*window).setShader",
		"github.com/nsgo-dev/nsgo/internal/platform/windows.(*painter).canvas",
		"github.com/nsgo-dev/nsgo/internal/platform/windows.(*video).configure",
		"github.com/nsgo-dev/nsgo/internal/platform/windows.(*window).newWeb",
		"github.com/nsgo-dev/nsgo/internal/platform/windows.(*window).setNative",
		"github.com/nsgo-dev/nsgo/internal/platform/windows.(*window).SubmitGPU",
		"github.com/nsgo-dev/nsgo/internal/platform/windows.(*window).configureGPU",
		"github.com/nsgo-dev/nsgo/internal/platform/windows.(*painter).drawGPU",
	},
	"linux": {
		"github.com/nsgo-dev/nsgo/internal/platform/linux.(*window).setShader",
		"github.com/nsgo-dev/nsgo/internal/platform/linux.(*window).snapshotCanvas",
		"github.com/nsgo-dev/nsgo/internal/platform/linux.(*window).configureVideo",
		"github.com/nsgo-dev/nsgo/internal/platform/linux.(*window).configureWeb",
		"github.com/nsgo-dev/nsgo/internal/platform/linux.(*window).configureNative",
		"github.com/nsgo-dev/nsgo/internal/platform/linux.(*window).SubmitGPU",
		"github.com/nsgo-dev/nsgo/internal/platform/linux.(*window).setGPU",
	},
	"android": {
		"github.com/nsgo-dev/nsgo/internal/platform/android.writeShader",
		"github.com/nsgo-dev/nsgo/internal/platform/android.writeDrawing",
		"github.com/nsgo-dev/nsgo/internal/platform/android.writeVideo",
		"github.com/nsgo-dev/nsgo/internal/platform/android.writeWebConfig",
		"github.com/nsgo-dev/nsgo/internal/platform/android.writeNative",
		"github.com/nsgo-dev/nsgo/internal/platform/android.(*surface).SubmitGPU",
		"github.com/nsgo-dev/nsgo/internal/platform/android.writeGPUViewConfig",
	},
	"js": {
		"github.com/nsgo-dev/nsgo/internal/platform/web.(*surface).setShader",
		"github.com/nsgo-dev/nsgo/internal/platform/web.(*surface).paint",
		"github.com/nsgo-dev/nsgo/internal/platform/web.(*surface).setVideo",
		"github.com/nsgo-dev/nsgo/internal/platform/web.(*surface).setWeb",
		"github.com/nsgo-dev/nsgo/internal/platform/web.(*surface).configureNative",
		"github.com/nsgo-dev/nsgo/internal/platform/web.(*surface).SubmitGPU",
		"github.com/nsgo-dev/nsgo/internal/platform/web.(*surface).createGPU",
		"github.com/nsgo-dev/nsgo/internal/platform/web.(*surface).configureGPU",
		"github.com/nsgo-dev/nsgo/internal/platform/web.(*surface).initPendingGPUs",
	},
}

// featurePackages names the optional packages testdata/features must link per target.
var featurePackages = map[string][]string{
	"darwin":  {"github.com/nsgo-dev/nsgo/internal/shaderir/msl"},
	"windows": {"github.com/nsgo-dev/nsgo/internal/shaderir/hlsl"},
	"linux":   {"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"},
	"android": {"github.com/nsgo-dev/nsgo/internal/shaderir/glsl"},
	"js":      {"github.com/nsgo-dev/nsgo/internal/shaderir/glsl", "github.com/nsgo-dev/nsgo/internal/platform/canvas2d"},
}

// linkTargets are the targets the link tests build for.
var linkTargets = [][2]string{{"darwin", "arm64"}, {"windows", "amd64"}, {"linux", "amd64"}, {"android", "arm64"}, {"js", "wasm"}}

// A text view without RenderMath, RenderDiagram or EmojiPicker links no formulas, diagrams or emoji data: Untie and its font come with base/formula alone, limu with base/diagram, and emoji data with EmojiPicker alone.
func TestTextViewAppLinks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	linked := linkedPackagesOf(t, moduleRoot+"/testdata/textview", "darwin", "arm64", "")
	if !linked["github.com/nsgo-dev/nsgo/base/textview"] {
		t.Fatal("the text view app links no text view")
	}
	for pkg := range linked {
		if strings.HasPrefix(pkg, "github.com/caelyreth/untie") || pkg == "github.com/nsgo-dev/nsgo/base/formula" ||
			strings.HasPrefix(pkg, "github.com/firede/limu") || pkg == "github.com/nsgo-dev/nsgo/base/diagram" ||
			pkg == "github.com/nsgo-dev/nsgo/base/internal/emoji" {
			t.Errorf("an app showing a text view without formulas, diagrams or emoji picker links %s", pkg)
		}
	}
}

func TestMinimalAppLinks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	for _, target := range linkTargets {
		t.Run(target[0]+"_"+target[1], func(t *testing.T) {
			t.Parallel()
			linked, syms := linkedPackagesAndSymbolsOf(t, moduleRoot+"/testdata/minimal", target[0], target[1], "")
			for _, pkg := range append(unlinked[""], unlinked[target[0]]...) {
				if linked[pkg] {
					t.Errorf("an app showing one text links %s", pkg)
				}
			}
			for _, sym := range featureSymbols[target[0]] {
				if syms[sym] {
					t.Errorf("an app showing one text links %s", sym)
				}
			}
		})
	}
}

func TestNetworkWatchersLinkOnUse(t *testing.T) {
	for _, target := range []struct{ os, arch, backend, loader string }{
		{"darwin", "arm64", "apple", "loadNetworkAPI"},
		{"windows", "amd64", "windows", "networkInterfaceChanged"},
		{"linux", "amd64", "linux", "loadLinuxNetwork"},
		{"js", "wasm", "web", "networkWatcher.WatchNetwork"},
	} {
		t.Run(target.os+"_"+target.arch, func(t *testing.T) {
			t.Parallel()
			prefix := modulePath + "/internal/platform/" + target.backend + "."
			for _, fixture := range []string{"minimal", "network"} {
				_, symbols := linkedPackagesAndSymbolsOf(t, moduleRoot+"/testdata/"+fixture, target.os, target.arch, "")
				linked := false
				for symbol := range symbols {
					if strings.HasPrefix(symbol, prefix) && strings.HasSuffix(symbol, ".WatchNetwork") {
						linked = true
					}
					if fixture == "minimal" && symbol == prefix+target.loader {
						t.Errorf("an app without a network read links %s", symbol)
					}
				}
				if want := fixture == "network"; linked != want {
					t.Errorf("%s app links its native network watcher = %t, want %t", fixture, linked, want)
				}
			}
		})
	}
}

// TestAutomationAppLinks verifies that calling automation.Register in ordinary
// app source brings no automation code into the release.
func TestAutomationAppLinks(t *testing.T) {
	linked := linkedPackagesOf(t, moduleRoot+"/testdata/automation", "darwin", "arm64", "")
	for _, pkg := range []string{
		"github.com/nsgo-dev/nsgo/automation",
		"github.com/nsgo-dev/nsgo/internal/automation",
		"github.com/nsgo-dev/nsgo/nsgotest",
	} {
		if linked[pkg] {
			t.Errorf("a release app that registers an automation tool links %s", pkg)
		}
	}
}

// TestFeatureAppLinks verifies that an app constructing a shader, canvas, video, web view, native component and GPU view links each backend's implementation for all of them.
func TestFeatureAppLinks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	for _, target := range linkTargets {
		t.Run(target[0]+"_"+target[1], func(t *testing.T) {
			t.Parallel()
			linked, syms := linkedPackagesAndSymbolsOf(t, moduleRoot+"/testdata/features", target[0], target[1], "")
			for _, pkg := range featurePackages[target[0]] {
				if !linked[pkg] {
					t.Errorf("feature app does not link %s", pkg)
				}
			}
			for _, sym := range featureSymbols[target[0]] {
				if !syms[sym] {
					t.Errorf("feature app does not link %s", sym)
				}
			}
		})
	}
}

// TestAndroidDNSLinks verifies that building for Android with nsgo_android_dns (which nsgo build adds when an app imports net) links Android's DNS resolver.
func TestAndroidDNSLinks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	linked := linkedPackages(t, "android", "arm64", "nsgo_android_dns")
	if !linked["net"] {
		t.Error("android build with nsgo_android_dns does not link net")
	}
}

// TestDevBuildsLink builds the minimal app as nsgo dev does, with the dev client (nsgodev), for each target, which TestReleaseBuildsLinkNoDevClient does for this machine's alone, and checks that the client, which reaches the dev server over HTTP, links.
func TestDevBuildsLink(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	for _, target := range linkTargets {
		t.Run(target[0]+"_"+target[1], func(t *testing.T) {
			t.Parallel()
			if !linkedPackages(t, target[0], target[1], "nsgodev")["net/http"] {
				t.Error("a dev build does not link the dev client")
			}
		})
	}
	t.Run("miniprogram_release", func(t *testing.T) {
		t.Parallel()
		linked := linkedPackages(t, "js", "wasm", "nsgominiprogram,nsgo_noshader")
		for _, pkg := range []string{"net/http", "github.com/nsgo-dev/nsgo/internal/dev/locales"} {
			if linked[pkg] {
				t.Errorf("a release mini program links %s", pkg)
			}
		}
	})
	t.Run("miniprogram_dev", func(t *testing.T) {
		t.Parallel()
		linked := linkedPackages(t, "js", "wasm", "nsgominiprogram,nsgo_noshader,nsgodev")
		if !linked["github.com/nsgo-dev/nsgo/internal/dev/locales"] {
			t.Error("a dev mini program does not link the dev client")
		}
		for _, pkg := range []string{"net/http", "runtime/pprof", "runtime/trace"} {
			if linked[pkg] {
				t.Errorf("a dev mini program links %s, past the 2 MB package budget", pkg)
			}
		}
	})
}

// linkedPackages returns the packages the linker keeps in testdata/minimal for goos and goarch, built with tags, for what other packages reach: those its dependency dump has an edge into from another package's symbol. A package's own init and the tables it fills, and literals the linker shares by content, do not count.
func linkedPackages(t *testing.T, goos, goarch, tags string) map[string]bool {
	t.Helper()
	return linkedPackagesOf(t, moduleRoot+"/testdata/minimal", goos, goarch, tags)
}

// linkedPackagesOf is linkedPackages for the app in dir.
func linkedPackagesOf(t *testing.T, dir, goos, goarch, tags string) map[string]bool {
	t.Helper()
	packages, _ := linkedPackagesAndSymbolsOf(t, dir, goos, goarch, tags)
	return packages
}

// linkedPackagesAndSymbolsOf also keeps concrete linker symbols for optional backend methods.
func linkedPackagesAndSymbolsOf(t *testing.T, dir, goos, goarch, tags string) (map[string]bool, map[string]bool) {
	t.Helper()
	cmd := exec.Command("go", "build", "-trimpath", "-tags="+tags, "-ldflags=-dumpdep", "-o", os.DevNull, dir)
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=")
	var dump bytes.Buffer
	cmd.Stderr = &dump
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v\n%s", err, dump.Bytes())
	}
	linked := map[string]bool{}
	symbols := map[string]bool{}
	sc := bufio.NewScanner(&dump)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		from, to, ok := strings.Cut(sc.Text(), " -> ")
		if !ok {
			continue
		}
		to, _, _ = strings.Cut(to, " ")
		if pseudoSymbol(from) || pseudoSymbol(to) {
			continue
		}
		symbols[to] = true
		if pkg := symbolPackage(to); pkg != symbolPackage(from) {
			linked[pkg] = true
		}
	}
	return linked, symbols
}

// pseudoSymbol reports whether sym is no code of its package's: the
// runner of inits, an init's task, or data the compiler names after
// where it first appeared, such as a string literal or a frame's layout.
func pseudoSymbol(sym string) bool {
	if sym == "go:main.inittasks" || strings.HasSuffix(sym, "..inittask") {
		return true
	}
	for _, s := range []string{"..stmp_", ".stkobj", ".opendefer", ".arginfo", ".argliveinfo", "..gobytes.", "..dict."} {
		if strings.Contains(sym, s) {
			return true
		}
	}
	return false
}

// symbolPackage returns the package of a linker symbol, such as
// "net/http" of "net/http.(*Client).Do" or of "type:*net/http.Client".
func symbolPackage(sym string) string {
	for _, prefix := range []string{"type:.eq.", "type:.hash.", "type:", "go:itab.", "go:"} {
		if rest, ok := strings.CutPrefix(sym, prefix); ok {
			sym = rest
			break
		}
	}
	sym = strings.TrimLeft(sym, "*(")
	slash := strings.LastIndexByte(sym, '/')
	if dot := strings.IndexByte(sym[slash+1:], '.'); dot >= 0 {
		return sym[:slash+1+dot]
	}
	return sym
}

// TestIOSPrivateAutomationLookupsAreGated verifies that iOS's private IOKit HID, BackBoardServices and UIKeyboardImpl lookups compile only under nsgotest or nsgoautomation, never in release or plain dev builds, and that watchOS's automation surface is gated the same way.
func TestIOSPrivateAutomationLookupsAreGated(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	listFiles := func(pkg, tags string) (dir string, files []string) {
		t.Helper()
		cmd := exec.Command("go", "list", "-tags="+tags, "-json=Dir,GoFiles", pkg)
		cmd.Dir = moduleRoot
		cmd.Env = append(os.Environ(), "GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1", "GOFLAGS=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list -tags=%s %s: %v\n%s", tags, pkg, err, out)
		}
		var p struct {
			Dir     string
			GoFiles []string
		}
		if err := json.Unmarshal(out, &p); err != nil {
			t.Fatal(err)
		}
		return p.Dir, p.GoFiles
	}
	privateTokens := []string{
		"IOHIDEvent",
		"BKSHIDEvent",
		"_enqueueHIDEvent:",
		"_contextId",
		"UIKeyboardImpl",
		"UIKeyboardInputMode",
	}
	for _, tags := range []string{"", "nsgodev"} {
		dir, files := listFiles("./internal/platform/ios", tags)
		for _, forbidden := range []string{"testhost.go", "testsurface.go", "deviceinput.go", "automation.go"} {
			if slices.Contains(files, forbidden) {
				t.Errorf("ios tags=%q compiled %s", tags, forbidden)
			}
		}
		if !slices.Contains(files, "automation_off.go") {
			t.Errorf("ios tags=%q did not compile automation_off.go", tags)
		}
		for _, name := range files {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			src := string(data)
			for _, tok := range privateTokens {
				if strings.Contains(src, tok) {
					t.Errorf("ios tags=%q file %s references private symbol %q", tags, name, tok)
				}
			}
		}
	}
	_, iosAuto := listFiles("./internal/platform/ios", "nsgodev,nsgoautomation")
	if !slices.Contains(iosAuto, "automation.go") || !slices.Contains(iosAuto, "testsurface.go") || !slices.Contains(iosAuto, "deviceinput.go") || slices.Contains(iosAuto, "testhost.go") || slices.Contains(iosAuto, "automation_off.go") {
		t.Errorf("ios automation files = %v", iosAuto)
	}
	_, iosTest := listFiles("./internal/platform/ios", "nsgotest")
	if !slices.Contains(iosTest, "testhost.go") || !slices.Contains(iosTest, "testsurface.go") || !slices.Contains(iosTest, "deviceinput.go") || !slices.Contains(iosTest, "automation_off.go") || slices.Contains(iosTest, "automation.go") {
		t.Errorf("ios test files = %v", iosTest)
	}

	for _, tags := range []string{"nsgowatch", "nsgowatch,nsgodev"} {
		_, files := listFiles("./internal/platform/watchos", tags)
		for _, forbidden := range []string{"testhost.go", "testsurface.go", "automation.go"} {
			if slices.Contains(files, forbidden) {
				t.Errorf("watchos tags=%q compiled %s", tags, forbidden)
			}
		}
	}
	_, watchAuto := listFiles("./internal/platform/watchos", "nsgowatch,nsgodev,nsgoautomation")
	if !slices.Contains(watchAuto, "automation.go") || !slices.Contains(watchAuto, "testsurface.go") || slices.Contains(watchAuto, "testhost.go") {
		t.Errorf("watchos automation files = %v", watchAuto)
	}
	_, watchTest := listFiles("./internal/platform/watchos", "nsgowatch,nsgotest")
	if !slices.Contains(watchTest, "testhost.go") || !slices.Contains(watchTest, "testsurface.go") || slices.Contains(watchTest, "automation.go") {
		t.Errorf("watchos test files = %v", watchTest)
	}
}

// TestMiniProgramAutomationIsGated verifies that mini-program automation hooks (automation.go and testsurface.go) compile only with nsgoautomation and stay absent from release and plain dev builds.
func TestMiniProgramAutomationIsGated(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	listFiles := func(pkg, tags string) []string {
		t.Helper()
		cmd := exec.Command("go", "list", "-tags="+tags, "-json=GoFiles", pkg)
		cmd.Dir = moduleRoot
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0", "GOFLAGS=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list -tags=%s %s: %v\n%s", tags, pkg, err, out)
		}
		var p struct{ GoFiles []string }
		if err := json.Unmarshal(out, &p); err != nil {
			t.Fatal(err)
		}
		return p.GoFiles
	}
	for _, tags := range []string{"nsgominiprogram,nsgo_noshader", "nsgominiprogram,nsgo_noshader,nsgodev"} {
		files := listFiles("./internal/platform/miniprogram", tags)
		for _, forbidden := range []string{"automation.go", "testsurface.go"} {
			if slices.Contains(files, forbidden) {
				t.Errorf("miniprogram tags=%q compiled %s", tags, forbidden)
			}
		}
		if !slices.Contains(files, "automation_off.go") {
			t.Errorf("miniprogram tags=%q did not compile automation_off.go", tags)
		}
	}
	autoFiles := listFiles("./internal/platform/miniprogram", "nsgominiprogram,nsgo_noshader,nsgodev,nsgoautomation")
	if !slices.Contains(autoFiles, "automation.go") || !slices.Contains(autoFiles, "testsurface.go") || slices.Contains(autoFiles, "automation_off.go") {
		t.Errorf("miniprogram automation files = %v", autoFiles)
	}
	linked := linkedPackages(t, "js", "wasm", "nsgominiprogram,nsgo_noshader")
	for _, pkg := range []string{"testing", "github.com/nsgo-dev/nsgo/nsgotest", "github.com/nsgo-dev/nsgo/internal/automation", "net/http", "image/png"} {
		if linked[pkg] {
			t.Errorf("release minimal mini-program build unexpectedly links %s", pkg)
		}
	}
}
