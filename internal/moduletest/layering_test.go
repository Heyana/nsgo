package moduletest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// layers lists the module packages a package may import
// (docs/architecture/layers.md), both as paths inside the module, "" for
// its root. A pattern ending in "/" matches a package and the packages
// below it; the first rule whose package matches applies.
var layers = []struct {
	pkg     string
	imports []string
}{
	{"internal/layout", nil},
	// Recording reaches Android's runtime, and nsgotest's fake, through the driver's channel outside any surface, asks for its permission as the backends do, and holds iOS's audio session with the backends' videos and speech.
	{"microphone", []string{"internal/audiosession", "internal/driver", "internal/platform/objcblock", "internal/platform/privacy"}},
	{"internal/audiosession", nil},
	{"internal/crashreport", nil},
	// The automation's protocol, which the nsgo tool and the agent in a running app speak, is the standard library's JSON alone.
	{"internal/automation", nil},
	// Widgets take core's colors, edges and fonts, and reach WidgetKit
	// through the files and the library the tool's extension shares, and
	// Android's runtime through the driver's Live Updates.
	{"widget", []string{"core", "internal/driver"}},
	// Platform game services reach GameKit through purego and objcblock on Apple platforms, Android's runtime through the driver's game services channel, and nsgotest's per-app fake through core.
	{"gameservices", []string{"core", "internal/driver", "internal/platform/objcblock"}},
	// Cloud storage reaches Apple's Foundation/iCloud APIs through purego and objcblock, Android's Play Games runtime bridge through internal/driver, and nsgotest's isolated in-memory store through core.Scope.CloudStore.
	{"cloud", []string{"core", "internal/driver", "internal/platform/objcblock"}},
	{"internal/woff", nil},
	{"internal/mimeinfo", nil},
	{"internal/appstartup", nil},
	{"internal/cldr/", nil},
	{"internal/cldr", nil},
	{"internal/driver", []string{"internal/mimeinfo", "internal/shaderir"}},
	{"internal/dev/locales", []string{"core", "i18n"}},
	// DevTools hand the automation's agent each surface's batches and events.
	{"internal/devtools", []string{"internal/driver"}},
	{"internal/shaderir/", []string{"internal/shaderir"}},
	{"internal/shader", []string{"internal/shaderir"}},
	{"internal/shader/wgsl", []string{"internal/shaderir"}},
	{"core", []string{"internal/crashreport", "internal/devtools", "internal/driver", "internal/layout", "internal/shader", "internal/shader/wgsl", "internal/shaderir"}},
	{"automation", []string{"core"}},
	{"internal/engine", []string{"core", "internal/driver", "internal/testhost"}},
	{"internal/testhost", []string{"core", "internal/driver"}},
	{"internal/platform/android/runtime", nil},
	{"internal/platform/applelog", nil},
	{"internal/platform/authscheme", []string{"internal/driver"}},
	{"internal/platform/apple", append([]string{"internal/driver"}, platformShared...)},
	{"internal/platform/canvas2d", []string{"internal/driver", "internal/platform/canvasmesh", "internal/platform/webstyle"}},
	{"internal/platform/canvasmesh", []string{"internal/driver"}},
	{"internal/platform/canvasshade", []string{"internal/driver"}},
	{"internal/platform/canvastest", []string{"internal/driver"}},
	{"internal/platform/codescan", []string{"internal/driver"}},
	{"internal/platform/field", []string{"internal/driver"}},
	{"internal/platform/finger", []string{"internal/driver"}},
	{"internal/platform/fontfile", nil},
	{"internal/platform/hover", []string{"internal/driver"}},
	{"internal/platform/imagedecode", []string{"internal/driver"}},
	{"internal/platform/imageload", []string{"internal/driver"}},
	{"internal/platform/knownfolder", nil},
	{"internal/platform/loopback", []string{"internal/driver"}},
	{"internal/platform/menuspeech", nil},
	{"internal/platform/menutext", []string{"internal/driver", "desktop/locales"}},
	{"internal/platform/objcblock", nil},
	{"internal/platform/pcmoutput", []string{"internal/driver"}},
	{"internal/platform/perfhud", []string{"internal/driver", "internal/engine", "internal/platform/apple"}},
	{"internal/platform/picked", []string{"internal/driver"}},
	{"internal/platform/privacy", []string{"internal/driver", "internal/platform/objcblock"}},
	{"internal/platform/queue", []string{"internal/driver"}},
	{"internal/platform/scrollbar", []string{"internal/driver"}},
	{"internal/platform/shareinbox", []string{"internal/driver", "internal/platform/objcblock"}},
	{"internal/platform/speechd", []string{"internal/driver"}},
	{"internal/platform/textselect", []string{"internal/driver"}},
	{"internal/platform/updates", []string{"internal/driver", "internal/platform/imageload"}},
	{"internal/platform/appimage", nil},
	{"internal/platform/videoalpha", nil},
	{"internal/platform/videoplay", []string{"internal/driver"}},
	{"internal/platform/videoalpha/clips", nil},
	{"internal/platform/viewtree", []string{"internal/driver"}},
	{"internal/platform/webstyle", []string{"internal/driver"}},
	{"internal/platform/", append([]string{"internal/driver", "internal/engine", "internal/testhost", "desktop/locales", "mobile/locales"}, platformShared...)},
	// Headless browser tests reuse the browser asset reader. Windowless rendering and text measurement reuse Apple's native player without starting a platform host; core still imports no backend.
	{"nsgotest", []string{"core", "gamepad", "internal/automation", "internal/devtools", "internal/driver", "internal/platform/apple", "internal/platform/imageload", "internal/shaderir/", "internal/testhost", "widget"}},
	{"", []string{"core", "internal/backend", "internal/dev", "internal/engine"}},
	// The backend runs the app on the platform build tags pick, with the launch flags, the dev client, restarts and, where the platform's updater keeps no schedule, update checks.
	{"internal/backend", []string{"core", "internal/crashreport", "internal/dev", "internal/engine", "internal/platform/", "internal/restart", "updater"}},
	// The dev client nsgo.Run starts in nsgo dev's builds; release builds link its no-op Start and Wrap alone.
	{"internal/dev", []string{"core", "internal/engine"}},
	{"internal/restart", []string{"internal/platform/appimage"}},
	// The module-wide tests, which have no code of their own.
	{"internal/moduletest", nil},
	// The code nsgo writes from catalogs runs on i18n.
	{"i18n", []string{"core", "internal/cldr"}},
	{"base/", []string{"core", "base/", "i18n"}},
	{"mobile/locales", []string{"core", "i18n"}},
	{"mobile", []string{"core"}},
	{"web", []string{"core"}},
	// Audio holds iOS's audio session for a library's own output beside the backends' players.
	{"audio", []string{"core", "internal/audiosession"}},
	// Gamepads reach GameController on Apple (with objcblock), XInput on Windows, evdev on Linux, navigator.getGamepads on the web, and the driver's bridge on Android and nsgotest.
	{"gamepad", []string{"core", "internal/driver", "internal/platform/objcblock"}},
	{"gpu", []string{"core", "internal/driver", "internal/shader", "internal/shader/wgsl", "internal/shaderir/"}},
	{"desktop/locales", []string{"core", "i18n"}},
	{"desktop", []string{"core"}},
	{"updater/locales", []string{"core", "i18n"}},
	{"updater", []string{"core", "updater/locales"}},
	{"icons/", []string{"core"}},
	{"ui/chat/locales", []string{"core", "i18n"}},
	{"ui/", []string{"core", "base/", "ui/", "mobile", "mobile/locales", "desktop", "icons/"}},
	// The TSX test runner drives apps through nsgotest, and runs its VM's work on the test goroutine through nsgotest's hook in internal/testhost (PostToTest), as App.Post runs on the engine's on a target.
	{"tsx/tsxtest", []string{"core", "internal/driver", "internal/testhost", "nsgotest", "tsx", "tsx/"}},
	// The nsgo tool links the bundler, so it reaches no runtime.
	{"tsx/bundler", []string{"core", "internal/driver", "internal/woff"}},
	// The JS engines' API reaches goja or the page, nothing of NSGo's.
	{"tsx/internal/js", []string{"tsx/websrc"}},
	// Packages are signed and verified with the standard library alone,
	// so the nsgo tool links them.
	{"tsx/pkg", nil},
	// nsgo/ui gives TSX ui's components, for the apps that import it.
	{"tsx/ui", []string{"core", "base/", "ui/", "tsx", "tsx/"}},
	{"tsx/ui/", []string{"core", "base/", "ui/", "tsx", "tsx/"}},
	// The scripts the page ships, which the tool writes as the page's files.
	{"tsx/websrc", nil},
	{"tsx/", []string{"core", "base/", "mobile", "desktop", "gamepad", "web", "updater", "internal/devtools", "tsx", "tsx/bundler", "tsx/internal/js", "tsx/pkg", "tsx/websrc"}},
	// Apps use the public packages only, and the hosts that embed an
	// example app its package.
	{"examples/", []string{"", "audio", "cloud", "automation", "core", "base/", "ui/", "gpu", "mobile", "desktop", "desktop/locales", "web", "gamepad", "gameservices", "i18n", "icons/", "nsgotest", "tsx/", "updater", "widget", "examples/"}},
	{"project", []string{"targets", "tsx/pkg"}},
	{"targets", nil},
	// The nsgo tool links no component or runtime, so changing them does
	// not build it again; it writes TSX declarations by running the app.
	{"cmd/nsgo/shareextension", []string{"internal/platform/objcblock", "internal/platform/shareinbox"}},
	{"cmd/", []string{"core", "cmd/", "i18n", "project", "targets", "internal/automation", "internal/appstartup", "internal/mimeinfo", "internal/platform/android/runtime", "tsx/bundler", "tsx/pkg", "tsx/websrc"}},
	{"tools/", []string{"core"}},
}

// platformShared is the code several backends share, with the shader
// writers but not the compiler, which only the core links.
var platformShared = []string{
	"internal/appstartup",
	"internal/platform/authscheme",
	"internal/audiosession", "internal/platform/apple", "internal/platform/canvas2d", "internal/platform/canvasmesh", "internal/platform/canvasshade", "internal/platform/canvastest", "internal/platform/codescan", "internal/platform/field", "internal/platform/finger",
	"internal/platform/appimage", "internal/platform/fontfile", "internal/platform/hover", "internal/platform/imagedecode", "internal/platform/imageload", "internal/platform/knownfolder", "internal/platform/loopback", "internal/platform/menuspeech", "internal/platform/menutext", "internal/platform/objcblock", "internal/platform/perfhud", "internal/platform/picked", "internal/platform/privacy", "internal/platform/queue", "internal/platform/scrollbar", "internal/platform/shareinbox", "internal/platform/speechd",
	"internal/platform/ops", "internal/platform/pcmoutput", "internal/platform/textruns", "internal/platform/textselect", "internal/platform/updates", "internal/platform/videoalpha", "internal/platform/videoplay", "internal/platform/viewtree", "internal/platform/webstyle", "internal/shaderir/",
}

// owned lists modules outside NSGo that only some packages may import, by module path: what is large enough that an app links it only by asking for what needs it (docs/architecture/layers.md, What an app links).
var owned = map[string][]string{
	"github.com/gdamore/tcell/v3": {"internal/platform/tui"},
	// Untie and the font it lays out in, for the formulas apps ask for.
	"github.com/caelyreth/untie": {"base/formula"},
	// limu, which reads and lays out Mermaid, for the diagrams apps ask for.
	"github.com/firede/limu": {"base/diagram"},
}

// portable lists the packages that build unchanged for every target: no
// purego, syscall or unsafe.
var portable = []string{"core", "internal/automation", "internal/appstartup", "internal/dev/locales", "internal/driver", "internal/mimeinfo", "internal/layout", "internal/shader", "internal/shader/wgsl", "internal/shaderir/", "internal/engine",
	"internal/testhost", "internal/cldr", "project", "targets", "base/", "ui/", "i18n", "mobile", "web", "audio", "gpu", "desktop", "desktop/locales", "icons/", "nsgotest", "tsx", "tsx/websrc", "updater", "updater/locales"}

// unportable are the packages among portable's that reach the system: the
// terminal loads libghostty-vt and runs programs, on macOS, Linux and
// Windows, and builds elsewhere to report that it cannot; formula reaches
// browser Web Workers on the web so parsing its math font stays off the page's
// main thread.
var unportable = []string{"base/terminal/", "base/formula/"}

// layerTargets are the GOOS/GOARCH pairs the nsgo tool builds for.
var layerTargets = [][2]string{
	{"darwin", "arm64"},
	{"ios", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
	{"linux", "amd64"},
	{"android", "arm64"},
	{"js", "wasm"},
}

const modulePath = "github.com/nsgo-dev/nsgo"

// devImports are what a package's nsgo dev builds (the nsgodev tag) import besides its layer's: the dev client reads what DevTools show from internal/devtools, the commit traces and their causes from the driver, and its failure overlay's words from its catalog.
var devImports = map[string][]string{"internal/dev": {"internal/dev/locales", "internal/devtools", "internal/driver"}}

// automationImports are what a package's nsgo dev -automation builds (the nsgoautomation tag, with nsgodev) import besides its dev builds': the dev client links nsgotest's agent, which it serves over DevTools' channel.
var automationImports = map[string][]string{"internal/dev": {"nsgotest"}}

// variantLayerTargets are the builds the nsgo tool makes with a variant's tag: nsgo watchos's, nsgo embed's iOS framework and nsgo miniprogram's.
var variantLayerTargets = [][3]string{{"darwin", "arm64", "nsgotui"}, {"linux", "amd64", "nsgotui"}, {"windows", "amd64", "nsgotui"}, {"ios", "arm64", "nsgowatch"}, {"ios", "arm64", "nsgoembed"}, {"js", "wasm", "nsgominiprogram,nsgo_noshader"}}

// devLayerTargets are the targets whose nsgo dev builds (the nsgodev tag) are checked too: the dev client's files differ only between the web and the rest.
var devLayerTargets = [][2]string{{"darwin", "arm64"}, {"js", "wasm"}}

func TestLayering(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	check := func(goos, goarch, tags string) {
		name := goos + "_" + goarch
		if tags != "" {
			name += "_" + strings.ReplaceAll(tags, ",", "_")
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, p := range listPackages(t, goos, goarch, tags) {
				checkLayer(t, p, tags)
			}
		})
	}
	for _, target := range layerTargets {
		check(target[0], target[1], "")
		check(target[0], target[1], "nsgotest")
	}
	// The watch app, an embedded iOS app and a mini-program build other files than the plain iOS and web builds, as apps and as test hosts.
	for _, variant := range variantLayerTargets {
		check(variant[0], variant[1], variant[2])
		check(variant[0], variant[1], "nsgotest,"+variant[2])
	}
	for _, target := range devLayerTargets {
		check(target[0], target[1], "nsgodev")
	}
	check("js", "wasm", "nsgodev,nsgominiprogram,nsgo_noshader")
	// nsgo dev -automation links the agent and the platform surfaces it drives.
	check("darwin", "arm64", "nsgodev,nsgoautomation")
	check("android", "arm64", "nsgodev,nsgoautomation")
	check("windows", "arm64", "nsgodev,nsgoautomation")
	check("ios", "arm64", "nsgodev,nsgoautomation")
	check("ios", "arm64", "nsgowatch,nsgodev,nsgoautomation")
	check("linux", "amd64", "nsgodev,nsgoautomation")
	check("js", "wasm", "nsgodev,nsgoautomation")
	check("js", "wasm", "nsgominiprogram,nsgo_noshader,nsgodev,nsgoautomation")
}

type listedPackage struct {
	ImportPath string
	Imports    []string
	CgoFiles   []string
	Error      *struct{ Err string }
}

func listPackages(t *testing.T, goos, goarch, tags string) []listedPackage {
	t.Helper()
	cmd := exec.Command("go", "list", "-e", "-tags="+tags, "-json=ImportPath,Imports,CgoFiles,Error", "./...")
	cmd.Dir = moduleRoot
	// CGO_ENABLED=1 keeps cgo files in the listing, so a C import shows.
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=1", "GOFLAGS=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.Bytes())
	}
	var pkgs []listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listedPackage
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if p.Error != nil {
			if !strings.Contains(p.Error.Err, "build constraints exclude all Go files") {
				t.Errorf("%s: %s", p.ImportPath, p.Error.Err)
			}
			continue
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

func checkLayer(t *testing.T, p listedPackage, tags string) {
	t.Helper()
	pkg, ok := inModule(p.ImportPath)
	if !ok {
		return
	}
	if len(p.CgoFiles) > 0 {
		t.Errorf("%s: cgo files %v; reach system APIs through purego or syscall", p.ImportPath, p.CgoFiles)
	}
	allowed, ruled := layerImports(pkg, tags)
	for _, tag := range strings.Split(tags, ",") {
		switch tag {
		case "nsgodev":
			allowed = append(slices.Clip(allowed), devImports[pkg]...)
		case "nsgoautomation":
			allowed = append(slices.Clip(allowed), automationImports[pkg]...)
		}
	}
	isPortable := matchesAny(pkg, portable) && !matchesAny(pkg, unportable)
	for _, imp := range p.Imports {
		// Android arm64 terminal binding uses NSGo’s cgo-free purego subset.
		if pkg == "base/terminal/internal/vt" && imp == modulePath+"/internal/purego" {
			continue
		}
		if dep, ok := inModule(imp); ok {
			if ruled && !matchesAny(dep, allowed) {
				t.Errorf("%s imports %s across layers", p.ImportPath, imp)
			}
		} else if isPortable && platformImport(imp) {
			t.Errorf("%s imports %s; it must build for every target unchanged", p.ImportPath, imp)
		}
		for module, owners := range owned {
			if (imp == module || strings.HasPrefix(imp, module+"/")) && !matchesAny(pkg, owners) {
				t.Errorf("%s imports %s, which only %v may", p.ImportPath, imp, owners)
			}
		}
	}
}

// inModule returns path inside the module, "" for its root.
func inModule(importPath string) (string, bool) {
	if importPath == modulePath {
		return "", true
	}
	rest, ok := strings.CutPrefix(importPath, modulePath+"/")
	return rest, ok
}

func layerImports(pkg, tags string) ([]string, bool) {
	for _, l := range layers {
		if matches(pkg, l.pkg) {
			allowed := l.imports
			if pkg == "core" && hasTag(tags, "nsgoautomation", "nsgotest", "nsgocheck") {
				allowed = append(slices.Clip(allowed), "internal/automation")
			}
			if matches(pkg, "internal/platform/") && hasTag(tags, "nsgotest", "nsgoautomation") {
				allowed = append(slices.Clip(allowed), "core")
			}
			return allowed, true
		}
	}
	return nil, false
}

func hasTag(tags string, wanted ...string) bool {
	for _, tag := range strings.Split(tags, ",") {
		for _, want := range wanted {
			if tag == want {
				return true
			}
		}
	}
	return false
}

func matches(pkg, pattern string) bool {
	if dir, ok := strings.CutSuffix(pattern, "/"); ok {
		return pkg == dir || strings.HasPrefix(pkg, pattern)
	}
	return pkg == pattern
}

func matchesAny(pkg string, patterns []string) bool {
	for _, pattern := range patterns {
		if matches(pkg, pattern) {
			return true
		}
	}
	return false
}

func platformImport(imp string) bool {
	switch imp {
	case "C", "unsafe", "syscall", "syscall/js":
		return true
	}
	return imp == "github.com/ebitengine/purego" || strings.HasPrefix(imp, "github.com/ebitengine/purego/")
}
