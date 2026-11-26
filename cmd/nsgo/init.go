package main

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"text/template"
)

// templates are what nsgo init writes: a Go app, a TSX app, or a Go app
// with a TSX entry importing Go of its own (NSH's ts, rust and mixed).
var templates = []string{"go", "tsx", "mixed"}

type initData struct{ Name, ID, Module, Slug string }

// stdModules is the standard library's import paths, as the go tool lists them.
var stdModules = sync.OnceValue(func() []string {
	out, err := exec.Command("go", "list", "std").Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
})

// moduleCollidesWithStd reports whether path is a standard library package or a prefix of one, which an app's module path cannot be.
func moduleCollidesWithStd(path string) bool {
	for _, std := range stdModules() {
		if std == path || strings.HasPrefix(std, path+"/") {
			return true
		}
	}
	return false
}

// initApp writes a new app of template kind into dir: nsgo.yaml, a main
// package and tests; a TSX app's entry, its tests, tsconfig.json and
// .gitignore. Outside any module, or with -module, it writes the app's go.mod too, and completeModule then makes the module build.
func initApp(ctx context.Context, dir, name, module, kind string) error {
	if name == "" {
		var err error
		if name, err = dirAppName(dir); err != nil {
			return err
		}
	}
	if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _-]*$`).MatchString(name) {
		return fmt.Errorf("init: %q is not an app name; use letters, digits, spaces, - and _", name)
	}
	if _, err := os.Stat(filepath.Join(dir, "nsgo.yaml")); err == nil {
		return fmt.Errorf("init: %s already has an nsgo.yaml", rel(dir))
	}
	id := "dev.nsgo." + strings.ToLower(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(name, ""))
	slug := strings.Trim(strings.ToLower(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(name, "-")), "-")
	if module != "" && moduleCollidesWithStd(module) {
		return fmt.Errorf("init: module path %q is a standard library package; pass -module with a path of your own, such as example.com/%s", module, slug)
	}
	data := initData{name, id, module, slug}
	// The go.mod of the module dir is in, found before init writes one of its own.
	enclosing, err := enclosingGoMod(dir)
	if err != nil {
		return err
	}
	if !slices.Contains(templates, kind) {
		return initModuleTemplate(ctx, dir, kind, data, enclosing)
	}
	newModule := module != "" || enclosing == ""
	if newModule {
		data.Module = cmp.Or(module, defaultModule(slug))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string]*template.Template{"nsgo.yaml": initYAML, "app.go": initApp_, "app_test.go": initTest}
	if kind != "go" {
		files = map[string]*template.Template{"nsgo.yaml": initYAML, "main.go": initTSXMain, "App.tsx": initTSX,
			"app.test.tsx": initTSXTest, "tsconfig.json": initTSConfig, ".gitignore": initIgnore}
	}
	if kind == "mixed" {
		files["App.tsx"], files["native.go"] = initMixedTSX, initNative
	}
	if newModule {
		files["go.mod"] = initMod
	}
	names := slices.Sorted(maps.Keys(files))
	for _, file := range names {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			return fmt.Errorf("init: %s exists", rel(filepath.Join(dir, file)))
		}
	}
	for _, file := range names {
		t := files[file]
		f, err := os.OpenFile(filepath.Join(dir, file), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		err = t.Execute(f, data)
		f.Close()
		if err != nil {
			return err
		}
		done("create %s", rel(filepath.Join(dir, file)))
	}
	if err := completeModule(ctx, dir, enclosing, newModule); err != nil {
		return err
	}
	if kind != "go" {
		detail("nsgo dev and nsgo check write the TSX's types to .nsgo/types for editors; check also type-checks it")
	}
	return nil
}

// defaultModule is the module path of an app init names after its slug, which takes a domain of its own when it names a standard library package.
func defaultModule(slug string) string {
	if moduleCollidesWithStd(slug) {
		return "example.com/" + slug
	}
	return slug
}

// enclosingGoMod is the go.mod of the module dir belongs to, the first found walking up from it, or empty outside any module.
func enclosingGoMod(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		gomod := filepath.Join(dir, "go.mod")
		if info, err := os.Stat(gomod); err == nil && info.Mode().IsRegular() {
			return gomod, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// initNSGoVersion is the version of NSGo a module init completes requires, this nsgo's; tests replace it.
var initNSGoVersion = toolVersion

// requirableVersion reports whether version is one a go.mod can require: a release or a pseudo-version, which go install of a tag or commit builds, but not what a checkout's build records, nothing or a version marked +dirty.
func requirableVersion(version string) bool {
	_, meta, _ := strings.Cut(version, "+")
	return version != "" && (meta == "" || meta == "incompatible")
}

// completeModule makes the module of the app init wrote into dir build, so nsgo dev runs next: a module init wrote, at dir's go.mod, requires this nsgo, declares it as a tool and is tidied, which fills go.sum; an enclosing module gets the require and tool lines it lacks and nothing else. A development build, with no version to require, leaves go.mod as it is and prints the go get commands that add them.
func completeModule(ctx context.Context, dir, enclosing string, newModule bool) error {
	gomod := enclosing
	if newModule {
		gomod = filepath.Join(dir, "go.mod")
	}
	version := initNSGoVersion()
	if !requirableVersion(version) {
		edits, err := nsgoModuleEdits(ctx, gomod, "")
		if err != nil {
			return err
		}
		if len(edits) > 0 {
			detail("next: cd %s && go get %s && go get -tool %s && %s dev", rel(dir), nsgoModule, nsgoTool, nsgoCommand())
			return nil
		}
		detail("next: %s dev %s", nsgoCommand(), rel(dir))
		return nil
	}
	edits, err := nsgoModuleEdits(ctx, gomod, version)
	if err != nil {
		return err
	}
	if len(edits) > 0 {
		if _, err := execute(ctx, processCommand{name: "go", args: append(append([]string{"mod", "edit"}, edits...), gomod)}); err != nil {
			return fmt.Errorf("init: add NSGo to %s: %w", rel(gomod), err)
		}
		if !newModule {
			done("add %s %s and its tool to %s", nsgoModule, version, rel(gomod))
		}
	}
	if newModule {
		stage(ctx, "running go mod tidy")
		cmd := exec.CommandContext(ctx, "go", "mod", "tidy")
		// The new module is in no workspace, whatever go.work encloses it.
		cmd.Dir, cmd.Env = dir, append(os.Environ(), "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("init: wrote the app, but go mod tidy in %s failed; run it there again\n%s", rel(dir), strings.TrimSpace(string(out)))
		}
		done("require %s %s in %s and run go mod tidy", nsgoModule, version, rel(gomod))
	}
	detail("next: %s dev %s", nsgoCommand(), rel(dir))
	return nil
}

// nsgoModuleEdits are the go mod edit flags that give gomod the lines it lacks of a require of NSGo at version and NSGo's tool. NSGo's own module needs neither.
func nsgoModuleEdits(ctx context.Context, gomod, version string) ([]string, error) {
	mod, err := readGoMod(ctx, gomod)
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	if mod.Module.Path == nsgoModule {
		return nil, nil
	}
	var edits []string
	if !slices.ContainsFunc(mod.Require, func(r struct{ Path, Version string }) bool { return r.Path == nsgoModule }) {
		edits = append(edits, "-require="+nsgoModule+"@"+version)
	}
	if !slices.ContainsFunc(mod.Tool, func(t struct{ Path string }) bool { return t.Path == nsgoTool }) {
		edits = append(edits, "-tool="+nsgoTool)
	}
	return edits, nil
}

// executable is os.Executable, which tests replace.
var executable = os.Executable

// nsgoCommand is the command that ran this nsgo, as the commands it suggests name it: go tool nsgo for a build the go command runs, and nsgo for an installed one, including the module's nsgo an installed one delegated to.
func nsgoCommand() string {
	if os.Getenv("NSGO_DELEGATED") == "1" {
		return "nsgo"
	}
	if exe, err := executable(); err == nil && builtByGo(exe) {
		return "go tool nsgo"
	}
	return "nsgo"
}

// goCacheEntry is the directory of an executable in Go's build cache, the output's SHA-256 with -d (cmd/go/internal/cache's DiskCache.fileName).
var goCacheEntry = regexp.MustCompile(`^[0-9a-f]{64}-d$`)

// builtByGo reports whether exe is a binary the go command built to run, as go tool does and sets nothing in the environment to say: one in its build cache, at <GOCACHE>/<first byte>/<hash>-d/<name>, or one it has just linked in its work directory, at go-build<N>/b<N>/exe/<name> (cmd/go/internal/tool's buildAndRunModtool).
func builtByGo(exe string) bool {
	dir := filepath.Dir(exe)
	parent, grandparent := filepath.Dir(dir), filepath.Dir(filepath.Dir(dir))
	if base := filepath.Base(dir); goCacheEntry.MatchString(base) {
		return filepath.Base(parent) == base[:2]
	}
	return filepath.Base(dir) == "exe" && regexp.MustCompile(`^b[0-9]+$`).MatchString(filepath.Base(parent)) && strings.HasPrefix(filepath.Base(grandparent), "go-build")
}

// dirAppName names an app after its directory, made absolute so that .
// names the current one, without what an app's name cannot hold.
func dirAppName(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	base := filepath.Base(abs)
	name := strings.Trim(regexp.MustCompile(`[^A-Za-z0-9 _-]+`).ReplaceAllString(base, "-"), " _-")
	name = strings.TrimLeft(name, "0123456789 _-")
	if name == "" {
		return "", fmt.Errorf("init: cannot name an app after %q; give one with -name", base)
	}
	return name, nil
}

var initYAML = template.Must(template.New("yaml").Parse(`name: {{.Name}}
id: {{.ID}}
version: 0.1.0

# macOS and iOS use assets/AppIcon.icon, or assets/AppIcon.icns on macOS; the
# other targets, and Apple ones without those, use assets/Icon.png. An Apple
# TV's icon is a wide layered card and an Apple Watch's is drawn in circles,
# so each takes files of its own; name them here:
# icon:
#   apple: assets/AppIcon.icon  # an Icon Composer .icon; macOS also takes .icns
#   png: assets/Icon.png        # a square PNG, 1024 pixels wide
#   tvos: assets/AppIconTV.brandassets  # layered brand assets, or a wide PNG
#   watchos: assets/AppIconWatch.icon   # an .icon drawn in circles, or a square PNG
# nsgo dev uses the same names ending in -dev where they exist, or the files
# dev.icon names.

dev:
  # native: this machine's backend; web: the browser; ios: a connected iPhone; ios_simulator; android: a connected device or emulator.
  target: native
  # true, the default, rebuilds and restarts on save; false runs once and exits with the app.
  hot_reload: true

build:
  output: dist
  # true opens the output directory after building.
  open: false
  targets: [macos_arm64, windows_x64, ios_simulator, android_arm64, web]
`))

var initMod = template.Must(template.New("mod").Parse(`module {{.Module}}

go 1.27.3
`))

var initApp_ = template.Must(template.New("app").Parse(`// Command {{.Name}} is an NSGo app.
package main

import (
	"fmt"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Root is the app's first screen.
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(24), PaddingEnv: core.EnvAll(core.EnvSafeArea), Gap: 16}, Children: []core.Node{
		core.Text{Content: "{{.Name}}", Style: core.Style{FontSize: 28, FontWeight: core.FontWeightBold}},
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		base.Button{OnPress: func(core.PressEvent) { count.Update(func(n int) int { return n + 1 }) }, Label: "Press me",
			Style: core.Style{Background: core.RGB(0x0969da), CornerRadius: 8, Padding: core.Symmetric(10, 16),
				AlignSelf: core.AlignStart, Active: &core.Style{Opacity: core.Alpha(0.8)}},
			Children: []core.Node{core.Text{Content: "Press me", Style: core.Style{Color: core.RGB(0xffffff)}}}},
	}}
}

func main() { nsgo.Run(Root) }
`))

var initTest = template.Must(template.New("test").Parse(`package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestMain gives the platform the main thread under nsgo test -target.
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestPress(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("role:button Press me")
	app.Find("text:Pressed 1 times")
}
`))

var initTSXMain = template.Must(template.New("main").Parse(`// Command {{.Name}} is an NSGo app written in TSX: App.tsx is its entry.
// nsgo dev and nsgo build compile it ahead; go run and go test compile it
// as it starts.
package main

import (
	"embed"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/tsx/compiler"
)

//go:embed *.tsx nsgo.yaml
var src embed.FS

// Root is the app: what App.tsx mounts.
var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
`))

var initTSX = template.Must(template.New("tsx").Parse(`import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="flex-1 gap-4 p-6">
      <Text class="text-3xl font-bold">{{.Name}}</Text>
      <Text>Pressed {count()} times</Text>
      <View
        role="button"
        accessibilityLabel="Press me"
        onPress={() => count.set(count() + 1)}
        class="self-start rounded-lg bg-blue-600 px-4 py-2 active:opacity-80"
      >
        <Text class="text-white">Press me</Text>
      </View>
    </View>
  );
}

mount(App);
`))

var initMixedTSX = template.Must(template.New("mixed").Parse(`import { mount, signal, Text, View } from "nsgo";
// What native.go registers: a component, a function and a class in Go.
import { Badge, greet, Tally } from "{{.Slug}}/native";

function App() {
  const tally = new Tally();
  const count = signal(tally.count);
  return (
    <View class="flex-1 gap-4 p-6">
      <Text class="text-3xl font-bold">{greet("{{.Name}}")}</Text>
      <Badge text={` + "`" + `Pressed ${count()} times` + "`" + `} />
      <View
        role="button"
        accessibilityLabel="Press me"
        onPress={() => count.set(tally.add(1))}
        class="self-start rounded-lg bg-blue-600 px-4 py-2 active:opacity-80"
      >
        <Text class="text-white">Press me</Text>
      </View>
    </View>
  );
}

mount(App);
`))

var initNative = template.Must(template.New("native").Parse(`package main

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

// Badge is a Go component App.tsx renders: its fields are its props, in
// lower camel case (Text is text).
type Badge struct {
	Text string
}

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text, Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
}

// Tally is a Go class App.tsx makes with new: its methods are the
// class's, and Count, a getter, its count property.
type Tally struct{ count int }

func (t *Tally) Add(by int) int { t.count += by; return t.count }
func (t *Tally) Count() int     { return t.count }

func init() {
	tsx.Register("{{.Slug}}/native", map[string]core.Node{"Badge": Badge{}})
	tsx.RegisterFunctions("{{.Slug}}/native", map[string]any{
		"greet": func(name string) string { return "Hello from Go, " + name },
	})
	tsx.RegisterClass("{{.Slug}}/native", "Tally", tsx.Class[Tally]{
		New:     func() *Tally { return &Tally{} },
		Getters: []string{"Count"},
	})
}
`))

var initTSXTest = template.Must(template.New("tsxtest").Parse(`// The app's TSX tests: nsgo test runs them headless, or on a platform
// with -target.
import { expect, open, test } from "nsgo/test";

test("counts presses", async () => {
  const app = await open("App.tsx");
  await app.press("role:button[Press me]");
  expect(app.shows("Pressed 1 times")).toBe(true);
});
`))

var initTSConfig = template.Must(template.New("tsconfig").Parse(`{
  "extends": "./.nsgo/tsconfig.json",
  "include": ["**/*.ts", "**/*.tsx"]
}
`))

var initIgnore = template.Must(template.New("ignore").Parse(`.nsgo/
dist/
`))
