package tsx_test

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// signedPackage compiles source and signs it as build of com.example.plugin,
// returning it and a Trust of its root.
func signedPackage(t *testing.T, source string, build uint64) ([]byte, *pkg.Trust) {
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
	data, err := pkg.Build(pkg.Contents{App: pkg.App{ID: "com.example.plugin", Build: build}, Source: code}, release, cert)
	if err != nil {
		t.Fatal(err)
	}
	trust := pkg.NewTrust()
	trust.AddRoot(root.Public().(ed25519.PublicKey))
	return data, trust
}

func TestViewRunsAVerifiedPackage(t *testing.T) {
	data, trust := signedPackage(t, `import { mount, signal, Text } from "nsgo";
const n = signal(0);
let made = "no";
try { made = String(new Function("return 1")()); } catch (e) { made = "refused"; }
mount(() => <Text testID="count" onPress={() => n.set(n() + 1)}>{made + " " + n()}</Text>);`, 1)
	p, err := trust.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{core.Text{Content: "host", TestID: "host"}, tsx.View{Package: p}}}
	})
	if got := app.Find("id:count").Text; got != "refused 0" {
		t.Fatalf("the package shows %q\n%s", got, app.Dump())
	}
	app.Press("id:count")
	if got := app.Find("id:count").Text; got != "refused 1" {
		t.Fatalf("after a press it shows %q", got)
	}
	if app.Find("id:host").Text != "host" {
		t.Fatal("the host's own view is gone")
	}
	b := []byte(strings.Clone(string(data)))
	b[len(b)-100] ^= 1
	if _, err := trust.Verify(b); !errors.Is(err, pkg.ErrSignature) {
		t.Fatalf("a changed package: got %v", err)
	}
}

func TestViewThrowsUnderCatch(t *testing.T) {
	data, trust := signedPackage(t, `throw new Error("broken plugin");`, 1)
	p, err := trust.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	var failed error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Catch{Try: tsx.View{Package: p}.Render, Fallback: func(cx *core.Scope, err error, _ func()) core.Node {
			failed = err
			return core.Text{Content: "failed", TestID: "failed"}
		}}
	})
	if app.Find("id:failed").Text != "failed" || failed == nil || !strings.Contains(failed.Error(), "broken plugin") {
		t.Fatalf("got %v\n%s", failed, app.Dump())
	}
}

func TestViewRendersNamedExportsWithIndependentState(t *testing.T) {
	data, trust := signedPackage(t, `import { signal, Text } from "nsgo";
export function First() { const n = signal(0); return <Text testID="first" onPress={() => n.set(n() + 1)}>{"first " + n()}</Text>; }
export function Second() { const n = signal(0); return <Text testID="second" onPress={() => n.set(n() + 1)}>{"second " + n()}</Text>; }`, 1)
	p, err := trust.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			tsx.View{Package: p, Export: "First"},
			tsx.View{Package: p, Export: "Second"},
		}}
	})
	if app.Find("id:first").Text != "first 0" || app.Find("id:second").Text != "second 0" {
		t.Fatalf("initial named views: %s", app.Dump())
	}
	app.Press("id:first")
	if app.Find("id:first").Text != "first 1" || app.Find("id:second").Text != "second 0" {
		t.Fatalf("named views share state: %s", app.Dump())
	}
}

// TestViewExportDisposesEntryMount verifies that what an entry module mounts never runs in a view of an export: neither its nodes nor the timers and animation frames it sets, however late it sets them.
func TestViewExportDisposesEntryMount(t *testing.T) {
	data, trust := signedPackage(t, `import { mount, signal, effect, Text } from "nsgo";
const ticks = signal(0);
const frames = signal(0);
const late = signal(0);
function Mounted() {
  effect(() => { setInterval(() => ticks.set(ticks() + 1), 10); });
  const loop = () => { frames.set(frames() + 1); requestAnimationFrame(loop); };
  requestAnimationFrame(loop);
  queueMicrotask(() => setInterval(() => late.set(late() + 1), 10));
  return <Text testID="mounted">mounted</Text>;
}
mount(Mounted);
export function Panel() { return <Text testID="export">{() => "export " + ticks() + " " + frames() + " " + late()}</Text>; }`, 1)
	p, err := trust.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return tsx.View{Package: p, Export: "Panel"}
	})
	if app.Exists("id:mounted") || app.Find("id:export").Text != "export 0 0 0" {
		t.Fatalf("entry mount was left in the root\n%s", app.Dump())
	}
	app.FastForward(100 * time.Millisecond)
	if got := app.Find("id:export").Text; got != "export 0 0 0" {
		t.Fatalf("entry mount kept running: %q", got)
	}
}

// TestRequestAnimationFrameNeverPassesNow runs a frame the display timed a frame ahead of the clock, as macOS's and iOS's display links do: the callback's timestamp is held to performance.now(), as a browser's is, so the time since it is never negative.
func TestRequestAnimationFrameNeverPassesNow(t *testing.T) {
	data, trust := signedPackage(t, `import { signal, Text } from "nsgo";
export function Panel() {
  const lag = signal("waiting");
  requestAnimationFrame((ts) => lag.set("lag " + (performance.now() - ts)));
  return <Text testID="lag">{() => lag()}</Text>;
}`, 1)
	p, err := trust.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	var root *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		root = cx
		return tsx.View{Package: p, Export: "Panel"}
	})
	root.RunFrameAt(app.Clock.Now().Add(16 * time.Millisecond))
	app.Settle()
	if got := app.Find("id:lag").Text; got != "lag 0" {
		t.Fatalf("performance.now() - ts in a frame timed ahead of now = %q, want %q", got, "lag 0")
	}
}

func TestViewNamedExportErrors(t *testing.T) {
	data, trust := signedPackage(t, `export const NotAComponent = 1;`, 1)
	p, err := trust.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		want error
	}{
		{name: "Missing", want: tsx.ErrNoViewExport},
		{name: "NotAComponent", want: tsx.ErrViewExportNotComponent},
	} {
		t.Run(test.name, func(t *testing.T) {
			var failed error
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				return core.Catch{Try: tsx.View{Package: p, Export: test.name}.Render, Fallback: func(cx *core.Scope, err error, _ func()) core.Node {
					failed = err
					return core.Text{Content: "failed", TestID: "failed"}
				}}
			})
			if app.Find("id:failed").Text != "failed" || !errors.Is(failed, test.want) {
				t.Fatalf("got %v\n%s", failed, app.Dump())
			}
		})
	}
}

// TestViewHoldsANamedExportToThePackage renders a named export of a package declaring the clipboard and the host call "greet" under a Grant of the host call alone: the export is held to the same capabilities, narrowed by Grant, as the entry is, and its props are a copy of the host's.
func TestViewHoldsANamedExportToThePackage(t *testing.T) {
	p := verified(t, `import { signal, Text, Clipboard, host } from "nsgo";
export function Panel(props) {
	const log = signal([]);
	const say = (v) => log.set([...log(), v]);
	props.title = "changed";
	(async () => {
		try { await Clipboard.readText(); say("clipboard ok"); } catch (e) { say("clipboard " + e.code); }
		try { await host.call("greet", "Ada"); say("host ok"); } catch (e) { say("host " + e.code); }
		say("done");
	})();
	return <Text testID="log">{() => props.title + ": " + log().join(", ")}</Text>;
}`, pkg.Capabilities{Clipboard: true, Host: []string{"greet"}})
	props := map[string]any{"title": "panel"}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return tsx.View{Package: p, Export: "Panel", Props: props, Sandbox: tsx.Sandbox{Host: greeter(), Grant: &pkg.Capabilities{Host: []string{"greet"}}}}
	})
	app.WaitUntil(10*time.Second, func() bool { return strings.Contains(app.Find("id:log").Text, "done") })
	if got := app.Find("id:log").Text; got != "changed: clipboard ERR_ACCESS_DENIED, host ok, done" {
		t.Fatalf("the export shows %q", got)
	}
	if props["title"] != "panel" {
		t.Fatalf("the export wrote the host's props: %v", props)
	}
}

func TestLoadNeedsTree(t *testing.T) {
	data, trust := signedPackage(t, `export function greet() { return "hello"; }`, 1)
	p, err := trust.Verify(data)
	if err != nil {
		t.Fatal(err)
	}
	root := core.NewRoot(core.RootOptions{})
	defer root.Dispose()
	if _, err := tsx.Load(root, p, tsx.Sandbox{}); !errors.Is(err, tsx.ErrNoTree) {
		t.Fatalf("Load without a tree = %v, want ErrNoTree", err)
	}
}
