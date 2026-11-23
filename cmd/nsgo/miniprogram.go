package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/andybalholm/brotli"
)

// The miniprogram_<platform> targets write a mini program project for
// WeChat, QQ, Alipay or Douyin: the runtime (miniprogram/, embedded
// here), the app's module brotli compressed in a subpackage of its own, as
// WXWebAssembly and its likes load only files of the code package and
// each package holds at most 2 MB, and the files the platform's developer
// tool reads. The module and the runtime's logic (miniprogram/common) are
// the same on every platform; the render layer is WeChat's
// (miniprogram/wechat), its markup rewritten for QQ and Douyin, or
// Alipay's own (miniprogram/alipay).

//go:embed miniprogram
var miniprogramRuntime embed.FS

// packageLimit is the most a mini program's package may hold, the module's
// subpackage included.
const packageLimit = 2 << 20

// miniProgramPlatform is a platform mini programs run on: the files it
// reads and the developer tool that opens them.
type miniProgramPlatform struct {
	// name follows miniprogram_ in the target's name and names the
	// platform in nsgo.yaml's miniprogram.appid and the runtime's
	// config.js; title in messages.
	name, title string
	// markup, style and script are the extensions of its templates,
	// styles and template scripts. prefix begins its directives (wx:if)
	// and tag is its template scripts' element, which WeChat's markup is
	// rewritten with; Alipay's markup is its own.
	markup, style, script, prefix, tag string
	// project is the file its tool reads the project from, tourist the
	// AppID it previews a project without one of its own with.
	project, tourist string
	// tool is its developer tool on macOS, download where to get it.
	tool, download string
}

var miniProgramPlatforms = []miniProgramPlatform{
	{name: "wechat", title: "WeChat", markup: "wxml", style: "wxss", script: "wxs", prefix: "wx:", tag: "wxs",
		project: "project.config.json", tourist: "touristappid", tool: "/Applications/wechatwebdevtools.app",
		download: "https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html"},
	{name: "qq", title: "QQ", markup: "qml", style: "qss", script: "qs", prefix: "qq:", tag: "qs",
		project: "project.config.json", tool: "/Applications/QQ小程序开发者工具.app",
		download: "https://q.qq.com/wiki/tools/devtool/"},
	{name: "alipay", title: "Alipay", markup: "axml", style: "acss", script: "sjs",
		project: "mini.project.json", tool: "/Applications/小程序开发者工具.app",
		download: "https://opendocs.alipay.com/mini/ide/download"},
	{name: "douyin", title: "Douyin", markup: "ttml", style: "ttss", script: "sjs", prefix: "tt:", tag: "sjs",
		project: "project.config.json", tool: "/Applications/抖音开发者工具.app",
		download: "https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download"},
}

// miniProgramTargets are the mini program targets' names.
func miniProgramTargets() []string {
	out := make([]string, len(miniProgramPlatforms))
	for i, mp := range miniProgramPlatforms {
		out[i] = "miniprogram_" + mp.name
	}
	return out
}

// miniProgramPlatformOf returns the platform of a mini program target.
func miniProgramPlatformOf(target string) (miniProgramPlatform, bool) {
	name, ok := strings.CutPrefix(target, "miniprogram_")
	for _, mp := range miniProgramPlatforms {
		if ok && mp.name == name {
			return mp, true
		}
	}
	return miniProgramPlatform{}, false
}

// miniprogramSubpackage is the subpackage the module is in.
const miniprogramSubpackage = "nsgo-app"

// miniprogramTags are the build's tags: the backend, and no shader
// compiler, as the render layer draws no shader.
var miniprogramTags = []string{"nsgominiprogram", "nsgo_noshader"}

// buildMiniProgram writes the project to dir. The module is built with
// encoding/json's first implementation, which is smaller, and when it is
// still too large for its package, again without inlining.
func buildMiniProgram(ctx context.Context, p *project, mp miniProgramPlatform, dir string) error {
	// Release builds (nsgo build, or nsgo dev -release) omit the dev client; dev builds keep it so DevTools, -highlight, -layout and -trace reach mini programs, still stripped with -s -w to fit the 2 MB package cap.
	if p.Release && p.Dev.Client {
		p.Dev.Client = false
		defer func() { p.Dev.Client = true }()
	}
	tmp, err := os.MkdirTemp("", "nsgo-miniprogram")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	wasm := filepath.Join(tmp, "app.wasm")
	env := []string{"GOOS=js", "GOARCH=wasm"}
	cur, err := goExperiment(ctx, p.Dir, env)
	if err != nil {
		return err
	}
	env = append(env, "GOEXPERIMENT="+experiments(cur, "nojsonv2"))
	// A mini program runs no code it did not ship, which the page's
	// engine would, and goja does not fit in its package.
	tsx, err := linkedDir(ctx, p, env, "github.com/nsgo-dev/nsgo/tsx", miniprogramTags...)
	if err != nil {
		return err
	}
	if tsx != "" {
		return fmt.Errorf("mini programs run Go apps alone: this app links package tsx")
	}
	module, err := buildMiniProgramModule(ctx, p, wasm, env, nil)
	if err != nil {
		return err
	}
	if len(module) > packageLimit {
		stage(ctx, "compiling Go without inlining")
		if module, err = buildMiniProgramModule(ctx, p, wasm, env, []string{"-gcflags=all=-l"}); err != nil {
			return err
		}
	}
	if len(module) > packageLimit {
		// nsgo dev -automation links its agent, which the developer tool's simulator may still load past the limit; builds without it leave it out.
		if !p.Dev.Automation || p.Release {
			return fmt.Errorf("the module is %s compressed, past the %s a mini program's package holds; "+
				"leave out what the app does not show on phones, such as the editor, the terminal or TSX",
				formatSize(int64(len(module))), formatSize(packageLimit))
		}
		warnIn(ctx, "the module with -automation's agent is %s compressed, past the %s %s's package holds; builds without -automation leave the agent out",
			formatSize(int64(len(module))), formatSize(packageLimit), mp.title)
	}
	stage(ctx, "writing the project")
	if err := writeMiniProgram(p, mp, dir, module); err != nil {
		return err
	}
	if main := dirSize(dir) - dirSize(filepath.Join(dir, miniprogramSubpackage)); main > packageLimit {
		warnIn(ctx, "the main package is %s, past %s's %s: move resources the app does not need out", formatSize(main), mp.title, formatSize(packageLimit))
	}
	return nil
}

// miniProgramLdflags returns the extra linker flags for a dev mini program module: stripped with -s -w so dev builds fit the 2 MB package cap while keeping full file paths for DevTools and core.App().Development true. Release builds are stripped already.
func miniProgramLdflags(p *project) []string {
	if p.Release {
		return nil
	}
	return []string{"-s -w"}
}

// buildMiniProgramModule builds the module to wasm and returns it
// compressed.
func buildMiniProgramModule(ctx context.Context, p *project, wasm string, env, flags []string) ([]byte, error) {
	if err := goBuildWith(ctx, p, wasm, env, buildExtra{tags: miniprogramTags, flags: flags}, miniProgramLdflags(p)...); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(wasm)
	if err != nil {
		return nil, err
	}
	stage(ctx, "compressing the module")
	var out bytes.Buffer
	w := brotli.NewWriterLevel(&out, brotli.BestCompression)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// miniprogramConfig is nsgo/config.js: what the runtime starts the app
// with.
type miniprogramConfig struct {
	Platform         string            `json:"platform"`
	Wasm             string            `json:"wasm"`
	Subpackage       string            `json:"subpackage"`
	CustomNavigation bool              `json:"customNavigation"`
	Env              map[string]string `json:"env"`
}

// writeMiniProgram writes the runtime, module and project files for mp to
// dir. A project written before keeps its AppID, which a developer tool
// writes when it imports one with a test account.
func writeMiniProgram(p *project, mp miniProgramPlatform, dir string, module []byte) error {
	layer := "wechat"
	if mp.name == "alipay" {
		layer = "alipay"
	}
	for _, root := range []string{"common", layer} {
		err := fs.WalkDir(miniprogramRuntime, "miniprogram/"+root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := miniprogramRuntime.ReadFile(path)
			if err != nil {
				return err
			}
			name, data := mp.rewrite(strings.TrimPrefix(path, "miniprogram/"+root+"/"), data)
			return writeFile(filepath.Join(dir, name), data)
		})
		if err != nil {
			return err
		}
	}
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return err
	}
	exec, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(goroot)), "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "nsgo", "wasm_exec.js"), miniprogramExec(exec)); err != nil {
		return err
	}
	cfg := miniprogramConfig{Platform: mp.name, Wasm: "/" + miniprogramSubpackage + "/app.wasm.br", Subpackage: miniprogramSubpackage,
		CustomNavigation: p.MiniProgram.CustomNavigation, Env: map[string]string{}}
	if p.Updates.Channel != "" {
		cfg.Env["NSGO_UPDATE_CHANNEL"] = p.Updates.Channel
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "nsgo", "config.js"), fmt.Appendf(nil, "module.exports = %s\n", data)); err != nil {
		return err
	}
	sub := filepath.Join(dir, miniprogramSubpackage)
	if err := writeFile(filepath.Join(sub, "app.wasm.br"), module); err != nil {
		return err
	}
	// A subpackage holds at least one page.
	for name, text := range map[string]string{
		"index.js": "Page({})\n", "index.json": "{}\n", "index." + mp.markup: "<view />\n", "index." + mp.style: "",
	} {
		if err := writeFile(filepath.Join(sub, name), []byte(text)); err != nil {
			return err
		}
	}
	if err := copyResources(p, filepath.Join(dir, "resources")); err != nil {
		return err
	}
	files := map[string]any{"app.json": mp.app(p)}
	if project := mp.projectFile(p, dir); project != nil {
		files[mp.project] = project
	}
	if mp.name == "wechat" {
		files["sitemap.json"] = map[string]any{"rules": []any{map[string]any{"action": "allow", "page": "*"}}}
	}
	for name, v := range files {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(dir, name), append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// rewrite returns a file of WeChat's render layer, its name and contents,
// as mp reads it: with mp's extensions, directives and template scripts.
// Other files stay as they are.
func (mp miniProgramPlatform) rewrite(name string, data []byte) (string, []byte) {
	if mp.name == "wechat" || mp.name == "alipay" {
		return name, data
	}
	base, ext, _ := strings.Cut(filepath.Base(name), ".")
	switch ext {
	case "wxml":
		text := strings.NewReplacer(`wx:`, mp.prefix, "<wxs ", "<"+mp.tag+" ", `src="./index.wxs"`, `src="./index.`+mp.script+`"`).
			Replace(string(data))
		return filepath.Join(filepath.Dir(name), base+"."+mp.markup), []byte(text)
	case "wxss":
		return filepath.Join(filepath.Dir(name), base+"."+mp.style), data
	case "wxs":
		return filepath.Join(filepath.Dir(name), base+"."+mp.script), data
	}
	return name, data
}

// app is mp's app.json: the page, the module's subpackage, loaded with
// it, and the window, colored for the app's appearance.
func (mp miniProgramPlatform) app(p *project) map[string]any {
	dark := p.Appearance == "dark"
	back := map[bool]string{false: "#ffffff", true: "#000000"}[dark]
	app := map[string]any{
		"pages":       []string{"pages/index/index"},
		"subPackages": []any{map[string]any{"root": miniprogramSubpackage, "name": miniprogramSubpackage, "pages": []string{"index"}}},
		"preloadRule": map[string]any{"pages/index/index": map[string]any{"network": "all", "packages": []string{miniprogramSubpackage}}},
	}
	if mp.name == "alipay" {
		window := map[string]any{"defaultTitle": p.Name, "titleBarColor": back, "backgroundColor": back}
		if p.MiniProgram.CustomNavigation {
			window["transparentTitle"] = "always"
		}
		app["window"] = window
		return app
	}
	window := map[string]any{"navigationBarTitleText": p.Name, "navigationBarBackgroundColor": back,
		"navigationBarTextStyle": map[bool]string{false: "black", true: "white"}[dark], "backgroundColor": back}
	if p.MiniProgram.CustomNavigation {
		window["navigationStyle"] = "custom"
	}
	app["window"] = window
	if mp.name == "wechat" {
		app["style"] = "v2"
		app["sitemapLocation"] = "sitemap.json"
		app["lazyCodeLoading"] = "requiredComponents"
		if p.Appearance == "" {
			app["darkmode"] = false
		}
	}
	return app
}

// projectFile is the project file mp's developer tool reads, with
// nsgo.yaml's AppID, else the one the file holds, else mp's tourist one.
// QQ and Douyin's tools ask for an AppID when there is none.
func (mp miniProgramPlatform) projectFile(p *project, dir string) map[string]any {
	if mp.name == "alipay" {
		return map[string]any{"format": 2, "compileType": "mini", "compileOptions": map[string]any{"component2": true}}
	}
	appid := p.MiniProgram.AppID[mp.name]
	if appid == "" {
		var kept struct {
			AppID string `json:"appid"`
		}
		if data, err := os.ReadFile(filepath.Join(dir, mp.project)); err == nil && json.Unmarshal(data, &kept) == nil &&
			kept.AppID != mp.tourist {
			appid = kept.AppID
		}
	}
	if appid == "" {
		appid = mp.tourist
	}
	project := map[string]any{
		"description": p.Name + " (" + p.ID + "), an NSGo app",
		"projectname": p.Name,
		"compileType": "miniprogram",
		// urlCheck is off: an app's images reach any host while it is
		// developed; a released mini program lists its domains.
		"setting":     map[string]any{"es6": true, "enhance": true, "minified": true, "urlCheck": false},
		"packOptions": map[string]any{"ignore": []any{}, "include": []any{}},
	}
	if appid != "" {
		project["appid"] = appid
	}
	if mp.name == "wechat" {
		project["libVersion"] = "3.0.0"
	}
	return project
}

// miniprogramExec is Go's wasm_exec.js as a module the runtime calls with
// the global object Go runs in and what the mini program lacks: browsers'
// TextEncoder, performance and crypto, and WebAssembly, whose Instance it
// checks the module against while WXWebAssembly makes its own.
func miniprogramExec(src []byte) []byte {
	var b bytes.Buffer
	b.WriteString("// Go's wasm_exec.js (" + runtime.Version() + "), as a function of the globals it reaches.\n")
	b.WriteString("module.exports = function (globalThis, TextEncoder, TextDecoder, performance, crypto, WebAssembly) {\n")
	b.WriteString("const setTimeout = globalThis.setTimeout, clearTimeout = globalThis.clearTimeout;\n")
	b.WriteString("const fs = { writeSync: (fd, buf) => globalThis.fs.writeSync(fd, buf) };\n")
	b.Write(src)
	b.WriteString("\n};\n")
	return b.Bytes()
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// configureMiniProgramDev sets p's dev server address and run token for a mini program dev build, or clears them for -release builds so they match nsgo build.
func configureMiniProgramDev(p *project, opts devOptions) {
	if opts.server != nil && !opts.release {
		p.devURL, p.devToken = opts.server.url(), opts.server.token
	} else {
		p.devURL, p.devToken = "", ""
	}
	if opts.release {
		p.Dev.Client = false
	}
}

// miniprogramSession writes the project into the dev cache and opens it in mp's developer tool: WeChat DevTools after each successful build, as it loads a rebuild only when the project is opened again, and the other tools once, as opening again would only bring their window forward on every rebuild.
func miniprogramSession(mp miniProgramPlatform, opts devOptions) session {
	dir := func(p *project) string { return p.cache("dev", "miniprogram_"+mp.name) }
	opened := false
	return session{
		build: func(ctx context.Context, p *project) error {
			configureMiniProgramDev(p, opts)
			return buildMiniProgram(ctx, p, mp, dir(p))
		},
		start: func(ctx context.Context, p *project) (app, error) {
			running := app{stop: func() {}}
			if opened && mp.name != "wechat" {
				return running, nil
			}
			note("%s", rel(dir(p)))
			if err := openMiniProgramProject(ctx, mp, dir(p)); err != nil {
				return running, err
			}
			opened = true
			return running, nil
		},
	}
}

// devToolsCLI is WeChat DevTools' command line on this machine, "" when
// it is not installed.
func devToolsCLI() string {
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		paths = []string{"/Applications/wechatwebdevtools.app/Contents/MacOS/cli"}
	case "windows":
		paths = []string{`C:\Program Files (x86)\Tencent\微信web开发者工具\cli.bat`, `C:\Program Files\Tencent\微信web开发者工具\cli.bat`}
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// canOpenMiniProgram reports whether this machine opens mp's projects:
// WeChat's with DevTools' command line, the others' tools on macOS.
func canOpenMiniProgram(mp miniProgramPlatform) bool {
	if mp.name == "wechat" {
		return devToolsCLI() != ""
	}
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := os.Stat(mp.tool)
	return err == nil
}

// openMiniProgram opens the project in mp's developer tool: WeChat
// DevTools through its command line, whose service port must be on
// (Settings, Security), and the others as macOS opens a folder in an app.
func openMiniProgram(ctx context.Context, mp miniProgramPlatform, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if !canOpenMiniProgram(mp) {
		return fmt.Errorf("%s's developer tool is not installed here: get it at %s, then open %s", mp.title, mp.download, rel(dir))
	}
	return openMiniProgramAt(ctx, mp, abs, devToolsCLI())
}

// openMiniProgramProject and runMiniProgramCommand are openMiniProgram and run, replaced by tests.
var (
	openMiniProgramProject = openMiniProgram
	runMiniProgramCommand  = run
)

func openMiniProgramAt(ctx context.Context, mp miniProgramPlatform, abs, cli string) error {
	if mp.name == "wechat" {
		// WeChat DevTools' open command neither compiles a project nor loads a rebuild into one already open. Closing it first makes the following auto command compile and load the fresh files on every -run and dev rebuild. Closing a project that is not open fails, which is safe to ignore.
		if err := runMiniProgramCommand(ctx, cli, "close", "--project", abs); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		return runMiniProgramCommand(ctx, cli, "auto", "--project", abs)
	}
	return runMiniProgramCommand(ctx, "open", "-a", mp.tool, abs)
}
