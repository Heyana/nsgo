package tsx_test

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	baselocales "github.com/nsgo-dev/nsgo/base/locales"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/pkg"

	// NSGo's packages that register into its modules, for the sandbox's surface.
	_ "github.com/nsgo-dev/nsgo/tsx/diagram"
	_ "github.com/nsgo-dev/nsgo/tsx/diff"
	_ "github.com/nsgo-dev/nsgo/tsx/editor"
	_ "github.com/nsgo-dev/nsgo/tsx/formula"
	_ "github.com/nsgo-dev/nsgo/tsx/terminal"
	_ "github.com/nsgo-dev/nsgo/tsx/ui"
	_ "github.com/nsgo-dev/nsgo/tsx/ui/chat"
	_ "github.com/nsgo-dev/nsgo/tsx/ui/richeditor"
)

func TestParseLimitsMemory(t *testing.T) {
	limits, err := tsx.ParseLimits("memory=123456")
	if err != nil {
		t.Fatal(err)
	}
	if limits.Memory != 123456 {
		t.Fatalf("memory limit = %d, want 123456", limits.Memory)
	}
}

// verified compiles source and signs it with caps as com.example.plugin,
// returning it verified.
func verified(t *testing.T, source string, caps pkg.Capabilities) *pkg.Package {
	t.Helper()
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(source)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	root, _ := pkg.GenerateKey()
	release, _ := pkg.GenerateKey()
	cert, err := pkg.Certify(root, release.Public().(ed25519.PublicKey), []string{"com.example.*"}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	data, err := pkg.Build(pkg.Contents{App: pkg.App{ID: "com.example.plugin", Build: 1}, Source: code, Capabilities: caps}, release, cert)
	if err != nil {
		t.Fatal(err)
	}
	trust := pkg.NewTrust()
	trust.AddRoot(root.Public().(ed25519.PublicKey))
	p, err := trust.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// network answers every request with its URL, and redirects /away to
// evil.io.
func network(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/away" {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"https://evil.io/"}},
			Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
	if req.URL.Path == "/big" {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 100))), Request: req}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(req.URL.String())), Request: req}, nil
}

// tries is a plugin that runs each named attempt and shows what each
// gave, in order, as "name: result".
const tries = `import { mount, signal, Text, AuthSession, Clipboard, openUrl, Storage, Secrets, host, SignInWithApple, Purchases } from "nsgo";
const log = signal([]);
const say = (name, v) => log.set([...log(), name + ": " + v]);
const how = (e) => e.code ? e.code + " " + (e.permission ?? "") : e.name + " " + e.message;
const attempt = async (name, fn) => { try { say(name, await fn()); } catch (e) { say(name, how(e)); } };
(async () => {
	for (const [name, fn] of Object.entries(attempts)) await attempt(name, fn);
	say("done", 1);
})();
mount(() => <Text testID="log">{() => log().join("\n")}</Text>);
`

// results runs the attempts in a View of caps and returns what each gave.
func results(t *testing.T, attempts string, caps pkg.Capabilities, s tsx.Sandbox) map[string]string {
	t.Helper()
	p := verified(t, "const attempts = {"+attempts+"};\n"+tries, caps)
	// What Go holds the package to, which holds on the web too.
	s.WebUnisolated = true
	s.Limits = &tsx.Limits{Load: 10 * time.Second, Call: 2 * time.Second, Body: 50, Requests: 2, Timers: 3, Storage: 64}
	app := nsgotest.Start(t, tsx.WithFetcher(func(cx *core.Scope) core.Node {
		return tsx.View{Package: p, Sandbox: s}
	}, network))
	app.WaitUntil(10*time.Second, func() bool { return strings.Contains(app.Find("id:log").Text, "done: 1") })
	out := map[string]string{}
	for _, line := range strings.Split(app.Find("id:log").Text, "\n") {
		k, v, _ := strings.Cut(line, ": ")
		out[k] = v
	}
	return out
}

func TestSandboxHoldsAPackageToItsCapabilities(t *testing.T) {
	got := results(t, `
	allowed: async () => (await fetch("https://api.example.com/a")).text(),
	other: () => fetch("https://evil.io/"),
	redirect: () => fetch("https://api.example.com/away"),
	file: () => fetch("file:///etc/hosts"),
	big: async () => (await fetch("https://api.example.com/big")).text(),
	data: async () => (await fetch("data:,hi")).text(),
	clipboard: () => Clipboard.readText(),
	openUrl: () => openUrl("https://example.com"),
	auth: () => AuthSession.start((redirect) => "https://provider.example/?redirect_uri=" + redirect).result,
	signInWithApple: () => SignInWithApple.signIn(),
	purchases: () => Purchases.products(["coins"]),
	granted: () => host.call("greet", "Ada"),
	refused: () => host.call("secret"),
	timers: () => { for (let i = 0; i < 5; i++) setTimeout(() => {}, 100000); },
	storage: async () => { await Storage.setItem("k", "v"); return (await Storage.keys()).join(","); },
	full: () => Storage.setItem("big", "x".repeat(100)),
	secrets: () => Secrets.get("token"),
	`, pkg.Capabilities{Network: &pkg.Network{Hosts: []string{"api.example.com"}}, Host: []string{"greet"}}, tsx.Sandbox{Host: greeter()})
	want := map[string]string{
		"allowed":         "https://api.example.com/a",
		"other":           "ERR_ACCESS_DENIED network",
		"redirect":        "ERR_ACCESS_DENIED network",
		"file":            "ERR_ACCESS_DENIED network",
		"big":             "ERR_NSH_QUOTA_EXCEEDED ",
		"data":            "hi",
		"clipboard":       "ERR_ACCESS_DENIED clipboard",
		"openUrl":         "ERR_ACCESS_DENIED openUrl",
		"auth":            "ERR_ACCESS_DENIED authSession",
		"signInWithApple": "ERR_ACCESS_DENIED signInWithApple",
		"purchases":       "ERR_ACCESS_DENIED purchases",
		"granted":         "hi Ada",
		"refused":         "ERR_ACCESS_DENIED host",
		"timers":          "ERR_NSH_QUOTA_EXCEEDED ",
		"storage":         "k",
		"full":            "ERR_NSH_QUOTA_EXCEEDED ",
		"secrets":         "ERR_ACCESS_DENIED secrets",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s gave %q, want %q", k, got[k], w)
		}
	}
}

// TestSandboxKeepsAPackagesSecretsApart grants a package the secrets capability: it keeps and reads its own, under a service of its partition, and never sees the app's.
func TestSandboxKeepsAPackagesSecretsApart(t *testing.T) {
	p := verified(t, `const attempts = {
	missing: async () => String(await Secrets.get("token")),
	set: () => Secrets.set("token", "the package's"),
	get: () => Secrets.get("token"),
	deleted: async () => { await Secrets.delete("token"); await Secrets.set("kept", "1"); return String(await Secrets.get("token")); },
};
`+tries, pkg.Capabilities{Secrets: true})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}
	}, nsgotest.Secrets(map[string]string{"token": "the app's"}))
	app.WaitUntil(10*time.Second, func() bool { return strings.Contains(app.Find("id:log").Text, "done: 1") })
	got := map[string]string{}
	for _, line := range strings.Split(app.Find("id:log").Text, "\n") {
		k, v, _ := strings.Cut(line, ": ")
		got[k] = v
	}
	for k, w := range map[string]string{"missing": "null", "set": "undefined", "get": "the package's", "deleted": "null"} {
		if got[k] != w {
			t.Errorf("%s gave %q, want %q", k, got[k], w)
		}
	}
	items := app.Secrets.Items()
	var apps, packages int
	for item, v := range items {
		switch {
		case item.Account == "token" && v == "the app's" && !strings.Contains(item.Service, "#"):
			apps++
		case item.Account == "kept" && strings.HasSuffix(item.Service, "#com.example.plugin"):
			packages++
		}
	}
	if len(items) != 2 || apps != 1 || packages != 1 {
		t.Fatalf("the store holds %v", items)
	}
}

func TestSandboxHoldsWhatWebPagesCapture(t *testing.T) {
	p := verified(t, `import { mount, View, WebView } from "nsgo";
mount(() => <View>
	<WebView testID="web" src="https://call.example.com/" media={["microphone", "camera"]} style={{ height: 50 }} />
</View>);`, pkg.Capabilities{WebView: &pkg.WebView{Hosts: []string{"call.example.com"}, Media: []string{"microphone"}}})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}} })
	app.WaitFor("id:web", 10*time.Second)
	if v := app.Find("id:web"); v.Web.Source.Src != "https://call.example.com/" || v.Web.Media != core.WebMicrophone {
		t.Fatalf("the web view loads %q capturing %v", v.Web.Source.Src, v.Web.Media)
	}
}

// TestTheWebRefusesWhatItCannotIsolate runs a package with capabilities: the page's engine cannot isolate it, so on the web View and Load refuse it unless the host opts in.
func TestTheWebRefusesWhatItCannotIsolate(t *testing.T) {
	p := verified(t, `import { mount, Text } from "nsgo";
mount(() => <Text testID="ran">ran</Text>);`, pkg.Capabilities{Clipboard: true})
	var viewErr, loadErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		_, loadErr = tsx.Load(cx, p, tsx.Sandbox{})
		return core.View{Children: []core.Node{
			core.Catch{Try: tsx.View{Package: p}.Render, Fallback: func(cx *core.Scope, err error, _ func()) core.Node {
				viewErr = err
				return core.Text{Content: "refused", TestID: "refused"}
			}},
			tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}},
		}}
	})
	if runtime.GOOS == "js" {
		if !errors.Is(viewErr, tsx.ErrUnisolated) || !errors.Is(loadErr, tsx.ErrUnisolated) || app.Find("id:refused").Text != "refused" {
			t.Fatalf("the view failed with %v and the plugin with %v\n%s", viewErr, loadErr, app.Dump())
		}
		if !strings.Contains(viewErr.Error(), "cannot isolate") {
			t.Fatalf("the refusal says %q", viewErr)
		}
	} else if viewErr != nil || loadErr != nil {
		t.Fatalf("the view failed with %v and the plugin with %v", viewErr, loadErr)
	}
	if !app.Exists("id:ran") {
		t.Fatalf("the package the host opted in for did not run\n%s", app.Dump())
	}
}

func greeter() *core.Host {
	h := core.NewHost()
	h.Handle("greet", func(c *core.HostCall) { c.Resolve("hi " + c.Data.(string)) })
	h.Handle("secret", func(c *core.HostCall) { c.Resolve("leaked") })
	return h
}

func TestGrantNarrowsCapabilities(t *testing.T) {
	got := results(t, `
	a: async () => (await fetch("https://a.com/")).text(),
	b: () => fetch("https://b.com/"),
	`, pkg.Capabilities{Network: &pkg.Network{Hosts: []string{"a.com", "b.com"}}},
		tsx.Sandbox{Grant: &pkg.Capabilities{Network: &pkg.Network{Hosts: []string{"a.com"}}}})
	if got["a"] != "https://a.com/" || got["b"] != "ERR_ACCESS_DENIED network" {
		t.Fatalf("got %v", got)
	}
}

type sandboxClass struct{}

func init() {
	tsx.Register("sandboxtest", map[string]core.Node{"Marker": core.Text{}}, tsx.SandboxSafe())
	tsx.RegisterFunctions("nsgo/base", map[string]any{"sandboxSecret": func() string { return "secret" }})
	// What NSGo's modules hold without SandboxSafe: a component, a class and a static of a component sandboxes import.
	tsx.Register("nsgo/base", map[string]core.Node{"SandboxHidden": core.Text{}})
	tsx.RegisterClass("nsgo/base", "SandboxHiddenClass", tsx.Class[sandboxClass]{New: func() *sandboxClass { return &sandboxClass{} }})
	tsx.RegisterStatics("nsgo/base", "Button", map[string]any{"sandboxHidden": func() string { return "secret" }})
}

func TestSandboxImportsOnlyWhatItIsGiven(t *testing.T) {
	attempts := `
	base: async () => typeof (await import("nsgo/base")).Button,
	fn: async () => typeof (await import("nsgo/base")).sandboxSecret,
	component: async () => typeof (await import("nsgo/base")).SandboxHidden,
	class: async () => typeof (await import("nsgo/base")).SandboxHiddenClass,
	static: async () => typeof (await import("nsgo/base")).Button.sandboxHidden,
	theme: async () => typeof (await import("nsgo/base")).Scrollbar.setTheme,
	scheme: async () => typeof (await import("nsgo")).setColorScheme,
	exit: async () => typeof (await import("nsgo/mobile")).BackHandler.exitApp,
	app: async () => typeof (await import("sandboxtest")).Marker,
	`
	got := results(t, attempts, pkg.Capabilities{}, tsx.Sandbox{})
	for _, k := range []string{"fn", "component", "class", "static", "theme", "scheme", "exit"} {
		if got[k] != "undefined" {
			t.Errorf("by default, %s is %s", k, got[k])
		}
	}
	if got["base"] != "function" || !strings.Contains(got["app"], "not given to this sandbox") {
		t.Fatalf("by default: %v", got)
	}
	got = results(t, attempts, pkg.Capabilities{}, tsx.Sandbox{Modules: []string{"nsgo/base.sandboxSecret", "nsgo/base.SandboxHidden",
		"nsgo/base.SandboxHiddenClass", "nsgo/base.Button.sandboxHidden", "nsgo.setColorScheme", "nsgo/mobile.BackHandler.exitApp", "sandboxtest"}})
	for _, k := range []string{"fn", "component", "class", "static", "scheme", "exit", "app"} {
		if got[k] != "function" {
			t.Errorf("given, %s is %s", k, got[k])
		}
	}
	if got["theme"] != "undefined" {
		t.Errorf("given others, setTheme is %s", got["theme"])
	}
}

// outsideNSGo is what registering into nsgo/base panicked with when the caller was not NSGo's: reflect's call, before any app started.
var outsideNSGo = func() (msg string) {
	defer func() { msg = fmt.Sprint(recover()) }()
	reflect.ValueOf(tsx.Register).Call([]reflect.Value{reflect.ValueOf("nsgo/base"),
		reflect.ValueOf(map[string]core.Node{"Outsider": core.Text{}}), reflect.ValueOf(tsx.SandboxSafe())})
	return ""
}()

func TestRegistryIsNSGosAndSetBeforeApps(t *testing.T) {
	if !strings.Contains(outsideNSGo, "the nsgo modules are NSGo's own") {
		t.Errorf("a caller outside NSGo registered into nsgo/base: %q", outsideNSGo)
	}
	nsgotest.Start(t, func(cx *core.Scope) core.Node { return tsx.App("late.js", `require("nsgo");`)(cx) })
	for name, register := range map[string]func(){
		"Register":          func() { tsx.Register("app/late", map[string]core.Node{"Late": core.Text{}}) },
		"RegisterClass":     func() { tsx.RegisterClass("app/late", "Late", tsx.Class[sandboxClass]{}) },
		"RegisterStatics":   func() { tsx.RegisterStatics("app/late", "Late", map[string]any{"f": func() {}}) },
		"RegisterFunctions": func() { tsx.RegisterFunctions("app/late", map[string]any{"f": func() {}}) },
		"RegisterMessages":  func() { tsx.RegisterMessages("app/late", baselocales.In, baselocales.Parameters) },
	} {
		func() {
			defer func() {
				if msg := fmt.Sprint(recover()); !strings.Contains(msg, "after an app started") {
					t.Errorf("%s after an app started: %q", name, msg)
				}
			}()
			register()
		}()
	}
}

// What a default Sandbox imports, every export of SandboxModules and the members of those that are functions (statics) or objects (services), with all of NSGo's packages that register into them linked, is testdata/sandbox-surface.txt, so whatever widens it is a reviewed change; NSGO_UPDATE_SANDBOX_SURFACE writes it anew.
func TestSandboxSurface(t *testing.T) {
	modules, _ := json.Marshal(tsx.SandboxModules)
	p := verified(t, `const modules = `+string(modules)+`;
export function surface() {
	const out = [];
	for (const name of modules) {
		const m = require(name);
		for (const k of Object.keys(m).sort()) {
			out.push(name + "." + k);
			const v = m[k];
			if (typeof v === "function" || typeof v === "object" && v !== null) for (const s of Object.keys(v).sort()) out.push(name + "." + k + "." + s);
		}
	}
	return out.join("\n") + "\n";
}`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var loadErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	var got any
	var callErr error
	app.Post(func() { plugin.Call("surface", nil, func(v any, err error) { got, callErr = v, err }) })
	app.WaitUntil(5*time.Second, func() bool { return got != nil || callErr != nil })
	if callErr != nil {
		t.Fatal(callErr)
	}
	const name = "testdata/sandbox-surface.txt"
	if os.Getenv("NSGO_UPDATE_SANDBOX_SURFACE") != "" {
		if err := os.WriteFile(name, []byte(got.(string)), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := testdata.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("a default Sandbox imports other than %s; review the change and write it anew with NSGO_UPDATE_SANDBOX_SURFACE=1:\n%s", name, got)
	}
}

// A plugin's key bindings, when: "always" included, and back handlers fire only while the focus is in what it shows: the host's field keeps the host's shortcut and back.
func TestSandboxKeysAndBackStayInItsView(t *testing.T) {
	p := verified(t, `import { mount, signal, keymap, View, Text } from "nsgo";
import { Input } from "nsgo/base";
import { BackHandler } from "nsgo/mobile";
const log = signal("");
keymap.bind("mod+s", () => log.set(log() + "save;"), { when: "always" });
BackHandler.onBackPress(() => { log.set(log() + "back;"); return true; });
mount(() => <View><Input testID="plugin-field" /><Text testID="plugin-log">{() => "plugin " + log()}</Text></View>);`, pkg.Capabilities{})
	var host []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.OnKey("mod+s", func() { host = append(host, "save") })
		cx.OnBackPress(func() bool { host = append(host, "back"); return true })
		return core.View{Children: []core.Node{core.Input{TestID: "host-field"}, tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}}}
	})
	app.Focus("id:host-field")
	app.Key("mod+s")
	app.Back()
	if got := app.Find("id:plugin-log").Text; got != "plugin " || !slices.Equal(host, []string{"save", "back"}) {
		t.Fatalf("focused in the host's field: the plugin's log %q, the host's %q", got, host)
	}
	app.Focus("id:plugin-field")
	app.Key("mod+s")
	app.Back()
	if got := app.Find("id:plugin-log").Text; got != "plugin save;back;" || len(host) != 2 {
		t.Fatalf("focused in the plugin's field: the plugin's log %q, the host's %q", got, host)
	}
}

// A plugin takes the focus on its own only while it has it or as the user's press on it opens what asks for it, so a timer cannot move the focus, and the host's keys with it, from the host's field; an app not sandboxed moves it as it likes.
func TestSandboxTakesTheFocusOnlyFromTheUser(t *testing.T) {
	const source = `import { mount, signal, keymap, View, Text } from "nsgo";
import { Input, Dialog } from "nsgo/base";
const log = signal(""), open = signal(false);
keymap.bind("mod+s", () => log.set(log() + "save;"), { when: "always" });
let field, other;
setTimeout(() => field.focus(), 100);
setTimeout(() => other.focus(), 300);
mount(() => <View>
	<Input testID="plugin-field" ref={field} />
	<Input testID="plugin-other" ref={other} />
	<Text testID="open" onPress={() => open.set(true)}>Open</Text>
	<Dialog open={open()}><Input testID="dialog-field" autoFocus /></Dialog>
	<Text testID="plugin-log">{() => "plugin " + log()}</Text>
</View>);`
	for _, sandboxed := range []bool{true, false} {
		name := map[bool]string{true: "sandboxed", false: "not sandboxed"}[sandboxed]
		var plugin func(cx *core.Scope) core.Node
		if sandboxed {
			p := verified(t, source, pkg.Capabilities{})
			plugin = tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}.Render
		} else {
			code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(source)}}, "App.tsx")
			if err != nil {
				t.Fatal(err)
			}
			plugin = tsx.App("App.js", string(code))
		}
		var host []string
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			cx.OnKey("mod+s", func() { host = append(host, "save") })
			return core.View{Children: []core.Node{core.Input{TestID: "host-field"}, core.Dynamic(plugin)}}
		})
		focused := func(what, want string) {
			t.Helper()
			for _, id := range []string{"host-field", "plugin-field", "plugin-other", "dialog-field"} {
				if app.Exists("id:"+id) && app.Find("id:"+id).Focused != (id == want) {
					t.Errorf("%s, %s: %s focused %v", name, what, id, !(id == want))
				}
			}
		}
		app.Focus("id:host-field")
		app.Advance(150 * time.Millisecond)
		if sandboxed {
			focused("after a timer's focus", "host-field")
			app.Key("mod+s")
			if got := app.Find("id:plugin-log").Text; got != "plugin " || !slices.Equal(host, []string{"save"}) {
				t.Errorf("after a timer's focus: the plugin's log %q, the host's %q", got, host)
			}
		} else {
			focused("after a timer's focus", "plugin-field")
		}
		app.Focus("id:plugin-field")
		app.Advance(200 * time.Millisecond)
		focused("after a timer's focus while it has the focus", "plugin-other")
		app.Focus("id:host-field")
		app.Press("id:open")
		app.WaitFor("id:dialog-field", time.Second)
		focused("after a press on it opened a dialog", "dialog-field")
	}
}

// A plugin shows in a view of its own that clips it: its views and overlays stay in it, sized to it, and a dialog it opens on a timer is refused while one its press opens shows; an app not sandboxed shows them over the host's views.
func TestSandboxShowsOnlyWithinItsView(t *testing.T) {
	const source = `import { mount, signal, View, Text, Overlay } from "nsgo";
import { Dialog } from "nsgo/base";
const open = signal(false);
setTimeout(() => open.set(true), 100);
mount(() => <View testID="plugin-root">
	<View testID="plugin-cover" style={{ position: "absolute", top: -40, left: 0, width: 100, height: 100 }} />
	<Overlay testID="plugin-overlay" pointerEvents="none" style={{ width: "100%", height: "100%" }} />
	<Text testID="close" onPress={() => open.set(false)}>Close</Text>
	<Text testID="open" onPress={() => open.set(true)}>Open</Text>
	<Dialog open={open()}><Text testID="dialog-text">Hi</Text></Dialog>
</View>);`
	for _, sandboxed := range []bool{true, false} {
		name := map[bool]string{true: "sandboxed", false: "not sandboxed"}[sandboxed]
		var plugin func(cx *core.Scope) core.Node
		if sandboxed {
			plugin = tsx.View{Package: verified(t, source, pkg.Capabilities{}), Sandbox: tsx.Sandbox{WebUnisolated: true}}.Render
		} else {
			code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(source)}}, "App.tsx")
			if err != nil {
				t.Fatal(err)
			}
			plugin = tsx.App("App.js", string(code))
		}
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(100), Height: core.Pt(200)}, Children: []core.Node{
				core.View{TestID: "host-button", OnPress: func(core.PressEvent) {}, Style: core.Style{Height: core.Pt(40)}},
				core.View{TestID: "host-slot", Style: core.Style{Height: core.Pt(60)}, Children: []core.Node{core.Dynamic(plugin)}},
			}}
		})
		frame, overlay := app.Find("id:plugin-root").Parent, app.Find("id:plugin-overlay")
		if sandboxed {
			if frame == app.Find("id:host-slot") || !frame.Style.Clip || frame.Frame.Size != (core.Size{Width: 100, Height: 60}) {
				t.Errorf("%s: the plugin's view is the host's %v, clips %v, at %+v", name, frame == app.Find("id:host-slot"), frame.Style.Clip, frame.Frame.Size)
			}
			if overlay.Parent != frame || overlay.Frame.Size != frame.Frame.Size {
				t.Errorf("%s: the overlay is in its view %v at %+v", name, overlay.Parent == frame, overlay.Frame.Size)
			}
		} else if overlay.Frame.Size.Height <= 60 || app.Find("id:host-slot").Style.Clip {
			t.Errorf("%s: the overlay is %+v and the host's slot clips %v", name, overlay.Frame.Size, app.Find("id:host-slot").Style.Clip)
		}
		app.Advance(150 * time.Millisecond)
		if shown := app.Exists("id:dialog-text"); shown == sandboxed {
			t.Errorf("%s: a dialog a timer opened shows %v", name, shown)
		}
		app.Press("id:close")
		app.Press("id:open")
		if !app.Exists("id:dialog-text") {
			t.Errorf("%s: a dialog its press opened does not show", name)
		}
	}
}

// A sandboxed package's Dialog and NavPage without a statusBar prop ask nothing of the statusBar capability it lacks, so they log no refusal and leave the one logged refusal to a status bar the package does set (#218).
func TestSandboxLogsOnlyTheStatusBarItSets(t *testing.T) {
	var logged strings.Builder
	prev := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(prev) })
	const source = `import { mount, signal, Text } from "nsgo";
import { Dialog, createNavStack, NavStack, NavPage } from "nsgo/base";
const nav = createNavStack(["home"]);
const open = signal(false);
const hide = signal(false);
mount(() => <NavStack state={nav}>{(route) => <NavPage title={route}>
	<Text testID="open" onPress={() => open.set(true)}>Open</Text>
	<Dialog open={open()} statusBar={hide() ? { hidden: true } : undefined}><Text testID="hide" onPress={() => hide.set(true)}>Hide</Text></Dialog>
</NavPage>}</NavStack>);`
	app := nsgotest.Start(t, tsx.View{Package: verified(t, source, pkg.Capabilities{}), Sandbox: tsx.Sandbox{WebUnisolated: true}}.Render)
	app.Press("id:open")
	app.Settle()
	refusals := func() int { return strings.Count(logged.String(), `"statusBar" capability`) }
	if !app.Exists("id:hide") || refusals() != 0 {
		t.Fatalf("dialog shown %v; without a status bar setting the package logged %q", app.Exists("id:hide"), logged.String())
	}
	app.Press("id:hide")
	app.Settle()
	if got := refusals(); got != 1 {
		t.Errorf("the package's own status bar logged %d refusals, want one: %q", got, logged.String())
	}
}

// A plugin taller than the view its host gives it is held to that view, so it cannot grow over the host's views below; an app not sandboxed overflows it.
func TestSandboxStaysInTheRoomItIsGiven(t *testing.T) {
	const source = `import { mount, View } from "nsgo";
mount(() => <View testID="plugin-root" onPress={() => {}} style={{ height: 300 }} />);`
	for _, sandboxed := range []bool{true, false} {
		name := map[bool]string{true: "sandboxed", false: "not sandboxed"}[sandboxed]
		var plugin func(cx *core.Scope) core.Node
		if sandboxed {
			plugin = tsx.View{Package: verified(t, source, pkg.Capabilities{}), Sandbox: tsx.Sandbox{WebUnisolated: true}}.Render
		} else {
			code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(source)}}, "App.tsx")
			if err != nil {
				t.Fatal(err)
			}
			plugin = tsx.App("App.js", string(code))
		}
		pressed := false
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(100), Height: core.Pt(200)}, Children: []core.Node{
				// Below the plugin's view, and under it in the order views stack in.
				core.View{TestID: "host-button", OnPress: func(core.PressEvent) { pressed = true },
					Style: core.Style{Position: core.Absolute, Top: core.Pt(60), Width: core.Pt(100), Height: core.Pt(40)}},
				core.View{TestID: "host-slot", Style: core.Style{Height: core.Pt(60)}, Children: []core.Node{core.Dynamic(plugin)}},
			}}
		})
		frame := app.Find("id:plugin-root").Parent
		app.PressAt("id:host-button", driver.Point{X: 50, Y: 20})
		if sandboxed && (frame.Frame.Size.Height != 60 || !pressed) {
			t.Errorf("%s: the plugin's view is %v tall and a press on the host's button reached it %v", name, frame.Frame.Size.Height, pressed)
		}
		if !sandboxed && pressed {
			t.Errorf("%s: a press on the host's button reached it, not the app's view over it", name)
		}
	}
}

func TestStopsAtTheNodeQuota(t *testing.T) {
	p := verified(t, `import { mount, signal, View, Text } from "nsgo";
const n = signal(1);
mount(() => <View>
	<Text testID="more" onPress={() => n.set(100)}>more</Text>
	{() => Array.from({ length: n() }, () => <View />)}
</View>);`, pkg.Capabilities{})
	var failed error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Catch{Try: tsx.View{Package: p, Sandbox: tsx.Sandbox{Limits: &tsx.Limits{Nodes: 50}, WebUnisolated: true}}.Render,
			Fallback: func(cx *core.Scope, err error, _ func()) core.Node {
				failed = err
				return core.Text{Content: "stopped", TestID: "stopped"}
			}}
	})
	app.Press("id:more")
	app.WaitFor("id:stopped", 5*time.Second)
	if core.ErrorCode(failed) != core.CodeQuotaExceeded {
		t.Fatalf("stopped with %v", failed)
	}
}

// TestSandboxGuardsRecursiveBuiltins gives the built-ins that go down data in Go, in a sandbox as a plugin gets one, data a million levels deep, which before #875 hung the call past its budget or overflowed the goroutine's stack and ended the process; each must throw a RangeError the plugin catches. It runs in a process of its own so that a crash fails the test rather than ending its binary.
func TestSandboxGuardsRecursiveBuiltins(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the web keeps the browser's built-ins, and starts no process")
	}
	if os.Getenv("NSGO_TEST_SANDBOX_NESTING") == "" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSandboxGuardsRecursiveBuiltins$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_SANDBOX_NESTING=1")
		cmd.WaitDelay = time.Second
		timer := time.AfterFunc(2*time.Minute, func() { _ = cmd.Process.Kill() })
		defer timer.Stop()
		if out, err := cmd.CombinedOutput(); err != nil {
			text := string(out)
			if i := strings.Index(text, "\ngoroutine "); i >= 0 {
				text = text[:i]
			}
			t.Fatalf("%v\n%s", err, text)
		}
		return
	}
	p := verified(t, `import { mount, Text } from "nsgo";
let value = [];
for (let i = 0; i < 1000000; i++) value = [value];
const result = [];
for (const [name, fn] of [["json", () => JSON.stringify(value)], ["string", () => String(value)], ["locale", () => value.toLocaleString()], ["flat", () => value.flat(Infinity)], ["revive", () => JSON.parse("[1, 2]", function (k, v) { if (k === "0") this[1] = value; return v; })]]) {
	try { fn(); result.push(name + ":ok"); } catch (e) { result.push(name + ":" + e.name); }
}
mount(() => <Text testID="out">{result.join("|")}</Text>);`, pkg.Capabilities{})
	app := nsgotest.Start(t, tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}.Render)
	app.WaitFor("id:out", 30*time.Second)
	if got := app.Find("id:out").Text; got != "json:RangeError|string:RangeError|locale:RangeError|flat:RangeError|revive:RangeError" {
		t.Fatalf("recursive built-ins returned %q", got)
	}
}

func TestLoadCallsWhatAPluginExports(t *testing.T) {
	p := verified(t, `export function add(a, b) { return a + b; }
export async function search(q) { const r = await fetch("https://api.example.com/" + q.term); return { url: await r.text() }; }
export function syncFails() { const e = new Error("no such word"); e.code = "notFound"; throw e; }
export async function fails() { throw new Error("no luck"); }
export const notAFunction = 1;`, pkg.Capabilities{Network: &pkg.Network{Hosts: []string{"api.example.com"}}})
	var plugin *tsx.Plugin
	var loadErr error
	app := nsgotest.Start(t, tsx.WithFetcher(func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true})
		return core.View{}
	}, network))
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	exports := plugin.Exports()
	slices.Sort(exports)
	if !slices.Equal(exports, []string{"add", "fails", "search", "syncFails"}) {
		t.Fatalf("exports %v", exports)
	}
	got := map[string]any{}
	call := func(name string, args ...any) {
		app.Post(func() {
			plugin.Call(name, args, func(v any, err error) {
				if err != nil {
					got[name] = err
				} else {
					got[name] = v
				}
			})
		})
	}
	call("add", 1, 2)
	call("search", map[string]any{"term": "cats"})
	call("syncFails")
	call("fails")
	call("missing")
	app.WaitUntil(5*time.Second, func() bool { return len(got) == 5 })
	if got["add"] != int64(3) && got["add"] != float64(3) {
		t.Errorf("add gave %#v", got["add"])
	}
	if m, _ := got["search"].(map[string]any); m["url"] != "https://api.example.com/cats" {
		t.Errorf("search gave %#v", got["search"])
	}
	if err, _ := got["syncFails"].(error); core.ErrorCode(err) != "notFound" || err.Error() != "no such word" {
		t.Errorf("syncFails gave %v", got["syncFails"])
	}
	if err, _ := got["fails"].(error); err == nil || err.Error() != "no luck" {
		t.Errorf("fails gave %v", got["fails"])
	}
	if err, _ := got["missing"].(error); !errors.Is(err, tsx.ErrNoExport) {
		t.Errorf("missing gave %v", got["missing"])
	}
	if plugin.Stopped() || plugin.Err() != nil {
		t.Errorf("call errors stopped the plugin: %v", plugin.Err())
	}
	if runtime.GOOS == "js" {
		return
	}
	app.Post(plugin.Close)
	var closed error
	app.Post(func() { plugin.Call("add", []any{1, 2}, func(_ any, err error) { closed = err }) })
	app.Settle()
	if closed == nil {
		t.Fatal("a closed plugin was called")
	}
}

// advanceUntil advances app's clock a millisecond at a time, so the plugin's timers fire, until cond holds, failing t after timeout of real time.
func advanceUntil(t *testing.T, app *nsgotest.App, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("the condition did not hold after %v", timeout)
		}
		app.Advance(time.Millisecond)
	}
}

func TestPluginWatchdogKeepsIndependentPromiseContinuation(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the page's engine cannot interrupt a running slice")
	}
	p := verified(t, `let release;
const shared = new Promise(resolve => { release = resolve; });
export async function victim() { await shared; return "ready"; }
export function trigger() {
	Promise.resolve().then(() => { for (;;) {} });
	release();
	return "sent";
}`, pkg.Capabilities{})
	limits := tsx.Limits{Load: time.Second, Call: 100 * time.Millisecond}
	var plugin *tsx.Plugin
	var loadErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{Limits: &limits})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	var victimCalls, triggerCalls int
	var victimResult any
	var victimErr, triggerErr error
	app.Post(func() {
		plugin.Call("victim", nil, func(result any, err error) {
			victimCalls++
			victimResult, victimErr = result, err
		})
		plugin.Call("trigger", nil, func(_ any, err error) {
			triggerCalls++
			triggerErr = err
		})
	})
	app.WaitUntil(5*time.Second, func() bool { return victimCalls > 0 && triggerCalls > 0 })
	app.Post(plugin.Close)
	app.Settle()
	if triggerCalls != 1 || !errors.Is(triggerErr, tsx.ErrBudget) {
		t.Fatalf("trigger called back %d times with %v, want ErrBudget once", triggerCalls, triggerErr)
	}
	if victimCalls != 1 || victimErr != nil || victimResult != "ready" {
		t.Fatalf("victim called back %d times with %v, %v, want ready once", victimCalls, victimResult, victimErr)
	}
}

func TestPluginCallSliceTimeout(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the page's engine cannot interrupt a running slice")
	}
	p := verified(t, `const spin = () => { for (;;) {} };
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
let later = 0;
const interval = () => setInterval(() => { later++; }, 25);
export async function first() { interval(); spin(); }
export function timer() {
	return new Promise(resolve => {
		setTimeout(spin, 0);
		setTimeout(() => { later++; resolve(1); }, 25);
	});
}
export async function awaited() { interval(); await wait(0); spin(); }
let resume;
const shared = new Promise(resolve => { resume = resolve; });
export async function sharedPromise() { interval(); await shared; spin(); }
export function release() { setTimeout(resume, 0); }
export async function peer() { await wait(10); return "ok"; }
export function hits() { return later; }`, pkg.Capabilities{})
	for _, tc := range []struct {
		name, export string
		total        time.Duration
	}{
		{"first-slice", "first", 0},
		{"timer", "timer", 0},
		{"after-await", "awaited", 0},
		{"with-total-budget", "awaited", 10 * time.Second},
		{"shared-promise", "sharedPromise", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := tsx.Limits{Load: time.Second, Call: 100 * time.Millisecond, CallTotal: tc.total}
			var plugin *tsx.Plugin
			var loadErr error
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{Limits: &limits})
				return core.View{}
			})
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			var callErr, peerErr error
			var peer any
			done := 0
			app.Post(func() {
				plugin.Call("peer", nil, func(v any, err error) { peer, peerErr = v, err })
				plugin.Call(tc.export, nil, func(_ any, err error) { done++; callErr = err })
				if tc.export == "sharedPromise" {
					plugin.Call("release", nil, func(_ any, _ error) {})
				}
			})
			app.Settle()
			app.Advance(100 * time.Millisecond)
			if done != 1 || !errors.Is(callErr, tsx.ErrBudget) {
				t.Fatalf("done called %d times with %v, want once with ErrBudget", done, callErr)
			}
			if peer != "ok" || peerErr != nil {
				t.Fatalf("independent call = %v, %v; want ok, nil", peer, peerErr)
			}
			if v, err := callPlugin(t, app, plugin, "hits"); err != nil || number(v) != 0 {
				t.Fatalf("timers left by the cut call ran %v times, error %v", v, err)
			}
			app.Post(plugin.Close)
			app.Settle()
			if done != 1 {
				t.Fatalf("done called %d times after Close, want once", done)
			}
		})
	}
}

// TestPluginCallTotalStopsYieldingWork holds CallTotal to failing a call whose every slice stays within Call, as it yields to a timer between them, and to dropping its timers, so the app runs on.
func TestPluginCallTotalStopsYieldingWork(t *testing.T) {
	p := verified(t, `export async function loop() {
	for (;;) {
		for (let i = 0; i < 100000; i++) {}
		await new Promise(resolve => setTimeout(resolve, 0));
	}
}`, pkg.Capabilities{})
	limits := tsx.Limits{Load: time.Second, Call: 10 * time.Second, CallTotal: 150 * time.Millisecond}
	var plugin *tsx.Plugin
	var loadErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{Limits: &limits, WebUnisolated: true})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	var callErr error
	start := time.Now()
	app.Post(func() { plugin.Call("loop", nil, func(_ any, err error) { callErr = err }) })
	advanceUntil(t, app, 5*time.Second, func() bool { return callErr != nil })
	if !errors.Is(callErr, tsx.ErrBudget) {
		t.Fatalf("call error = %v, want ErrBudget", callErr)
	}
	if took := time.Since(start); took < limits.CallTotal {
		t.Fatalf("call failed after %v, before its total of %v", took, limits.CallTotal)
	}
	app.Post(plugin.Close)
}

// TestPluginCallTotalCutsATimerQuietly holds CallTotal, cutting JS a timer runs for the call, to failing the call alone, not the app as a panic past the call.
func TestPluginCallTotalCutsATimerQuietly(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the page's engine cannot interrupt a running timer callback")
	}
	p := verified(t, `export function spin() {
	return new Promise(() => setTimeout(() => { for (;;) {} }, 0));
}`, pkg.Capabilities{})
	limits := tsx.Limits{Load: time.Second, Call: 10 * time.Second, CallTotal: 150 * time.Millisecond}
	var plugin *tsx.Plugin
	var loadErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{Limits: &limits, WebUnisolated: true})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	var callErr error
	app.Post(func() { plugin.Call("spin", nil, func(_ any, err error) { callErr = err }) })
	advanceUntil(t, app, 5*time.Second, func() bool { return callErr != nil })
	if !errors.Is(callErr, tsx.ErrBudget) {
		t.Fatalf("call error = %v, want ErrBudget", callErr)
	}
	app.Post(plugin.Close)
}

// deadlinePlugin loads source as a plugin held to a CallTotal of total and a load of load, with a Call budget no test reaches.
func deadlinePlugin(t *testing.T, source string, load, total time.Duration) (*nsgotest.App, *tsx.Plugin) {
	t.Helper()
	p := verified(t, source, pkg.Capabilities{})
	limits := tsx.Limits{Load: load, Call: 10 * time.Second, CallTotal: total}
	var plugin *tsx.Plugin
	var loadErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{Limits: &limits, WebUnisolated: true})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	t.Cleanup(func() { app.Post(plugin.Close) })
	return app, plugin
}

// callPlugin calls name and runs the app's clock until the call settles.
func callPlugin(t *testing.T, app *nsgotest.App, plugin *tsx.Plugin, name string) (any, error) {
	t.Helper()
	var result any
	var err error
	settled := false
	app.Post(func() {
		plugin.Call(name, nil, func(v any, e error) { result, err, settled = v, e, true })
	})
	advanceUntil(t, app, 5*time.Second, func() bool { return settled })
	return result, err
}

// counts calls the plugin's counts export, an array of numbers.
func counts(t *testing.T, app *nsgotest.App, plugin *tsx.Plugin) []float64 {
	t.Helper()
	v, err := callPlugin(t, app, plugin, "counts")
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	var out []float64
	for _, n := range v.([]any) {
		out = append(out, number(n))
	}
	return out
}

// number is a JSON number as Call gives one, an int64 or a float64.
func number(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	}
	return -1
}

// runUntil runs the app's clock, so the plugin's timers fire, until since is at least d ago in real time.
func runUntil(app *nsgotest.App, since time.Time, d time.Duration) {
	for time.Since(since) < d {
		app.Advance(time.Millisecond)
	}
}

// TestPluginCallTotalStopsAnIntervalTheCallLeft holds an interval a call sets going, which runs while the call's deadline lasts, to stopping once it passed, though the call settled long before, and the plugin to answering calls after.
func TestPluginCallTotalStopsAnIntervalTheCallLeft(t *testing.T) {
	app, plugin := deadlinePlugin(t, `let n = 0;
export function start() { setInterval(() => { n++; }, 1); return 1; }
export function counts() { return [n]; }`, time.Second, 300*time.Millisecond)
	start := time.Now()
	if v, err := callPlugin(t, app, plugin, "start"); err != nil || number(v) != 1 {
		t.Fatalf("start = %v, %v", v, err)
	}
	runUntil(app, start, 150*time.Millisecond)
	if c := counts(t, app, plugin); c[0] == 0 {
		t.Fatal("the interval did not run within its call's deadline")
	}
	runUntil(app, start, 400*time.Millisecond)
	before := counts(t, app, plugin)
	runUntil(app, start, 600*time.Millisecond)
	if after := counts(t, app, plugin); after[0] != before[0] {
		t.Fatalf("the interval ran %v times after its call's deadline", after[0]-before[0])
	}
}

// TestPluginLoadDeadlineStopsAModuleTimer holds an interval the entry module sets as it loads to the load's deadline.
func TestPluginLoadDeadlineStopsAModuleTimer(t *testing.T) {
	app, plugin := deadlinePlugin(t, `let n = 0;
setInterval(() => { n++; }, 1);
export function counts() { return [n]; }`, 300*time.Millisecond, time.Second)

	start := time.Now()
	app.Advance(time.Millisecond)
	if c := counts(t, app, plugin); c[0] == 0 {
		t.Fatal("the module's interval did not run within the load's deadline")
	}
	runUntil(app, start, 400*time.Millisecond)
	before := counts(t, app, plugin)
	runUntil(app, start, 600*time.Millisecond)
	if after := counts(t, app, plugin); after[0] != before[0] {
		t.Fatalf("the module's interval ran %v times after the load's deadline", after[0]-before[0])
	}
}

// TestPluginCallTotalStopsTimeoutChains holds a self-renewing timeout
// chain to the call's deadline. Goja also tracks which call owns a
// chain started after await.
func TestPluginCallTotalStopsTimeoutChains(t *testing.T) {
	app, plugin := deadlinePlugin(t, `let a = 0, b = 0, late = 0;
export function chain() { setTimeout(function tick() { a++; setTimeout(tick, 1); }, 1); return 1; }
export async function after() {
	await new Promise(resolve => setTimeout(resolve, 5));
	setTimeout(function tick() { b++; setTimeout(tick, 1); }, 1);
	setTimeout(() => { late = 1; }, 20);
	return 2;
}
export function counts() { return [a, b, late]; }`, time.Second, 300*time.Millisecond)
	start := time.Now()
	if _, err := callPlugin(t, app, plugin, "chain"); err != nil {
		t.Fatal(err)
	}
	tracksContinuation := runtime.GOOS != "js"
	if tracksContinuation {
		if v, err := callPlugin(t, app, plugin, "after"); err != nil || number(v) != 2 {
			t.Fatalf("after = %v, %v, want 2 after its short delay", v, err)
		}
	}
	app.Advance(25 * time.Millisecond)
	if c := counts(t, app, plugin); c[0] == 0 || tracksContinuation && (c[1] == 0 || c[2] != 1) {
		t.Fatalf("counts within the deadline = %v, promise contexts tracked = %v", c, tracksContinuation)
	}
	runUntil(app, start, 450*time.Millisecond)
	before := counts(t, app, plugin)
	runUntil(app, start, 650*time.Millisecond)
	if after := counts(t, app, plugin); after[0] != before[0] || after[1] != before[1] {
		t.Fatalf("the chains ran on after their calls' deadlines: %v, then %v", before, after)
	}
}

// TestPluginCallTotalStopsPromiseChains holds a chain of microtasks, which never yields the call into JS, and a chain of promises a timer settles, both left running by calls that returned, to their calls' deadlines, and the plugin to answering after.
func TestPluginCallTotalStopsPromiseChains(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the page's engine cannot interrupt a running microtask chain")
	}
	app, plugin := deadlinePlugin(t, `let m = 0, n = 0;
export function micro() { (async () => { for (;;) { m++; await null; } })(); return 1; }
export function timed() { (async () => { for (;;) { n++; await new Promise(resolve => setTimeout(resolve, 1)); } })(); return 1; }
export function counts() { return [m, n]; }`, time.Second, 300*time.Millisecond)
	start := time.Now()
	if _, err := callPlugin(t, app, plugin, "micro"); !errors.Is(err, tsx.ErrBudget) {
		t.Fatalf("micro = %v, want ErrBudget, as its microtasks ran to its deadline", err)
	}
	if time.Since(start) < 300*time.Millisecond {
		t.Fatalf("micro failed after %v, before its deadline", time.Since(start))
	}
	timedAt := time.Now()
	if _, err := callPlugin(t, app, plugin, "timed"); err != nil {
		t.Fatal(err)
	}
	runUntil(app, timedAt, 100*time.Millisecond)
	before := counts(t, app, plugin)
	if before[0] == 0 || before[1] == 0 {
		t.Fatalf("counts = %v, want both chains to have run", before)
	}
	runUntil(app, timedAt, 400*time.Millisecond)
	before = counts(t, app, plugin)
	runUntil(app, timedAt, 600*time.Millisecond)
	if after := counts(t, app, plugin); after[0] != before[0] || after[1] != before[1] {
		t.Fatalf("the promise chains ran on after their calls' deadlines: %v, then %v", before, after)
	}
}

// TestPluginCallTotalKeepsCallsApart holds two calls' work, started at different times, each to its own call's deadline: the earlier's stops while the later's runs on, which neither extends nor cuts the earlier's.
func TestPluginCallTotalKeepsCallsApart(t *testing.T) {
	app, plugin := deadlinePlugin(t, `let a = 0, b = 0;
export function first() { setInterval(() => { a++; }, 1); return 1; }
export function second() { setInterval(() => { b++; }, 1); return 1; }
export function counts() { return [a, b]; }`, time.Second, 600*time.Millisecond)
	start := time.Now()
	if _, err := callPlugin(t, app, plugin, "first"); err != nil {
		t.Fatal(err)
	}
	runUntil(app, start, 300*time.Millisecond)
	if _, err := callPlugin(t, app, plugin, "second"); err != nil {
		t.Fatal(err)
	}
	runUntil(app, start, 700*time.Millisecond)
	mid := counts(t, app, plugin)
	runUntil(app, start, 820*time.Millisecond)
	late := counts(t, app, plugin)
	if late[0] != mid[0] {
		t.Fatalf("the first call's interval ran past its deadline: %v, then %v", mid, late)
	}
	if late[1] <= mid[1] {
		t.Fatalf("the second call's interval stopped before its deadline: %v, then %v", mid, late)
	}
	runUntil(app, start, 1050*time.Millisecond)
	before := counts(t, app, plugin)
	runUntil(app, start, 1200*time.Millisecond)
	if after := counts(t, app, plugin); after[1] != before[1] {
		t.Fatalf("the second call's interval ran past its deadline: %v, then %v", before, after)
	}
}

// TestPluginInterruptFailsAWaitingCall holds Plugin.Interrupt to failing a call that waits on a timer, which no JS settles once the plugin stopped.
func TestPluginInterruptFailsAWaitingCall(t *testing.T) {
	p := verified(t, `export async function wait() {
	await new Promise(resolve => setTimeout(resolve, 60000));
	return 1;
}`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var loadErr error
	var notifications []error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true, OnStop: func(err error) {
			notifications = append(notifications, err)
		}})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	called := false
	var callErr error
	app.Post(func() {
		plugin.Call("wait", nil, func(_ any, err error) { called, callErr = true, err })
	})
	app.Settle()
	stop := errors.New("watchdog")
	interrupted := make(chan struct{})
	go func() {
		plugin.Interrupt(stop)
		close(interrupted)
	}()
	<-interrupted
	if !plugin.Stopped() || plugin.Err() != stop {
		t.Fatalf("stop state = %v, %v, want stopped with the interrupt's error", plugin.Stopped(), plugin.Err())
	}
	app.WaitUntil(time.Second, func() bool { return called && len(notifications) != 0 })
	if callErr != stop || len(notifications) != 1 || notifications[0] != stop {
		t.Fatalf("call error = %v, stop notifications = %v, want the interrupt's error", callErr, notifications)
	}
	app.Post(func() {
		plugin.Call("wait", nil, func(_ any, err error) { callErr = err })
	})
	app.Settle()
	if callErr != stop {
		t.Fatalf("a call after the interrupt failed with %v, want the interrupt's", callErr)
	}
	app.Post(plugin.Close)
	app.Settle()
	if len(notifications) != 1 || plugin.Err() != stop {
		t.Fatalf("closing after interrupt changed the stop state: %v, %v", notifications, plugin.Err())
	}
}

func TestPluginCloseFailsAWaitingCall(t *testing.T) {
	p := verified(t, `export async function wait() {
	await new Promise(() => {});
}`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var loadErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	called := 0
	var callErr error
	app.Post(func() {
		plugin.Call("wait", nil, func(_ any, err error) { called++; callErr = err })
	})
	app.Settle()
	app.Post(plugin.Close)
	app.WaitUntil(time.Second, func() bool { return called == 1 })
	if callErr != tsx.ErrPluginClosed {
		t.Fatalf("call error = %v, want plugin closed", callErr)
	}
	if called != 1 {
		t.Fatalf("done called %d times, want once", called)
	}
}

// Closing a plugin runs its cleanups before it fails the calls still waiting.
func TestPluginCloseRunsCleanups(t *testing.T) {
	p := verified(t, `import { cleanup, host } from "nsgo";
cleanup(() => { host.call("bye"); });
export async function wait() {
	await new Promise(() => {});
}`, pkg.Capabilities{Host: []string{"bye"}})
	h := core.NewHost()
	byes := 0
	h.Handle("bye", func(c *core.HostCall) { byes++; c.Resolve(nil) })
	var plugin *tsx.Plugin
	var loadErr, callErr error
	var notifications []error
	var cleanupsAtStop int
	var callErrorAtStop error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true, Host: h, OnStop: func(err error) {
			notifications = append(notifications, err)
			cleanupsAtStop, callErrorAtStop = byes, callErr
			plugin.Close()
		}})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	app.Post(func() {
		plugin.Call("wait", nil, func(_ any, err error) { callErr = err })
	})
	app.Settle()
	app.Post(plugin.Close)
	app.Settle()
	if byes != 1 {
		t.Fatalf("the cleanup ran %d times, want once", byes)
	}
	if cleanupsAtStop != 1 || callErrorAtStop != tsx.ErrPluginClosed {
		t.Fatalf("stop observed %d cleanups and call error %v", cleanupsAtStop, callErrorAtStop)
	}
	if len(notifications) != 1 || notifications[0] != tsx.ErrPluginClosed || plugin.Err() != tsx.ErrPluginClosed {
		t.Fatalf("close state = %v, %v, want one normal close", notifications, plugin.Err())
	}
}

func TestPluginOwnerDisposalNotifiesOnce(t *testing.T) {
	p := verified(t, `export function wait() { return new Promise(() => {}); }`, pkg.Capabilities{})
	var owner *core.Scope
	var plugin *tsx.Plugin
	var loadErr, callErr error
	var notifications []error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		owner = cx.Child()
		owner.Run(func() {
			plugin, loadErr = tsx.Load(owner, p, tsx.Sandbox{WebUnisolated: true, OnStop: func(err error) {
				notifications = append(notifications, err)
			}})
		})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	app.Post(func() {
		plugin.Call("wait", nil, func(_ any, err error) { callErr = err })
		owner.Dispose()
		plugin.Close()
	})
	app.Settle()
	if !plugin.Stopped() || plugin.Err() != tsx.ErrPluginClosed || callErr != tsx.ErrPluginClosed {
		t.Fatalf("scope disposal left plugin state %v, %v and call error %v", plugin.Stopped(), plugin.Err(), callErr)
	}
	if len(notifications) != 1 || notifications[0] != tsx.ErrPluginClosed {
		t.Fatalf("stop notifications = %v, want one normal close", notifications)
	}
}

func TestPluginReportsQuotaStop(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the page's engine owns its memory")
	}
	p := verified(t, `export function schedule() {
  setTimeout(() => new ArrayBuffer(65536), 20);
}`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var loadErr, runtimeErr error
	var notifications []error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{Limits: &tsx.Limits{Memory: 1024}, OnStop: func(err error) {
			notifications = append(notifications, err)
		}})
		return core.View{}
	}, nsgotest.Errors(func(err error) { runtimeErr = err }))
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if _, err := callPlugin(t, app, plugin, "schedule"); err != nil || plugin.Stopped() {
		t.Fatalf("scheduling the timer failed: %v, stop reason %v", err, plugin.Err())
	}
	app.Advance(20 * time.Millisecond)
	if !plugin.Stopped() || core.ErrorCode(plugin.Err()) != core.CodeQuotaExceeded {
		t.Fatalf("quota stop state = %v, %v", plugin.Stopped(), plugin.Err())
	}
	if !errors.Is(runtimeErr, plugin.Err()) || len(notifications) != 1 || notifications[0] != plugin.Err() {
		t.Fatalf("quota stop, runtime error and notifications differ: %v, %v, %v", plugin.Err(), runtimeErr, notifications)
	}
}

// A plugin closed before its interrupt fails the waiting calls still fails them, with the interrupt's error.
func TestPluginCloseAfterInterrupt(t *testing.T) {
	p := verified(t, `export async function wait() {
	await new Promise(() => {});
}`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var loadErr error
	var notifications []error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true, OnStop: func(err error) {
			notifications = append(notifications, err)
		}})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	called := 0
	var callErr error
	app.Post(func() {
		plugin.Call("wait", nil, func(_ any, err error) { called++; callErr = err })
	})
	app.Settle()
	stop := errors.New("watchdog")
	app.Post(func() {
		plugin.Interrupt(stop)
		plugin.Close()
	})
	app.Settle()
	if called != 1 || callErr != stop {
		t.Fatalf("done called %d times with %v, want once with the interrupt's error", called, callErr)
	}
	if len(notifications) != 1 || notifications[0] != stop {
		t.Fatalf("stop notifications = %v, want the interrupt's error once", notifications)
	}
}

func TestPluginInterruptStopsCall(t *testing.T) {
	p := verified(t, `export async function loop() {
	for (;;) await new Promise(resolve => setTimeout(resolve, 0));
}`, pkg.Capabilities{})
	var plugin *tsx.Plugin
	var loadErr error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true})
		return core.View{}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	var callErr error
	app.Post(func() { plugin.Call("loop", nil, func(_ any, err error) { callErr = err }) })
	time.Sleep(20 * time.Millisecond)
	plugin.Interrupt(nil)
	app.WaitUntil(time.Second, func() bool { return callErr != nil })
	if !errors.Is(callErr, tsx.ErrInterrupted) {
		t.Fatalf("call error = %v, want ErrInterrupted", callErr)
	}
	app.Post(plugin.Close)
}

func TestParseLimitsCallTotal(t *testing.T) {
	got, err := tsx.ParseLimits("call-total=3s")
	if err != nil {
		t.Fatal(err)
	}
	if got.CallTotal != 3*time.Second {
		t.Fatalf("call total = %v, want 3s", got.CallTotal)
	}
}

// TestLoadReturnsEntryModuleError holds Load to returning what a package's entry module throws as it loads, to its caller alone: no Catch above the scope and no app report sees it, and nothing the module set up before it threw runs after.
func TestLoadReturnsEntryModuleError(t *testing.T) {
	for _, c := range []struct{ name, source, want string }{
		{"an error", `setTimeout(() => { throw new Error("a timer of a failed plugin ran"); }, 10);
export function add(a, b) { return a + b; }
throw new Error("entry failed");`, "entry failed"},
		{"a value", `throw "entry threw a string";`, "entry threw a string"},
	} {
		p := verified(t, c.source, pkg.Capabilities{})
		var plugin *tsx.Plugin
		var loadErr error
		var caught []error
		app := nsgotest.Start(t, tsx.WithFetcher(func(cx *core.Scope) core.Node {
			return core.Catch{Try: func(cx *core.Scope) core.Node {
				plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{WebUnisolated: true})
				return core.View{}
			}, Fallback: func(cx *core.Scope, err error, _ func()) core.Node {
				caught = append(caught, err)
				return core.View{}
			}}
		}, network))
		app.Advance(time.Second)
		if plugin != nil {
			t.Errorf("%s: Load returned a plugin for a throwing entry module", c.name)
		}
		var panicked *core.PanicError
		if loadErr == nil || !strings.Contains(loadErr.Error(), c.want) || errors.As(loadErr, &panicked) {
			t.Errorf("%s: Load returned %v, want what the module threw, %q", c.name, loadErr, c.want)
		}
		if len(caught) != 0 {
			t.Errorf("%s: the Catch above the plugin caught %v", c.name, caught)
		}
	}
}
