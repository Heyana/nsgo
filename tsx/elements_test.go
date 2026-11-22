package tsx_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/dlclark/regexp2/v2"
	"golang.org/x/image/font/gofont/gomedium"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/compiler"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

func TestOverlayFloatsAboveAndReportsOutsidePresses(t *testing.T) {
	app := run(t, `import { mount, signal, Overlay, Text, View } from "nsgo";
function App() {
	const open = signal(true), presses = signal(0);
	let anchor;
	return <View testID="root" style={{ flex: 1 }}>
		<View testID="anchor" ref={anchor} style={{ width: 40, height: 20, marginTop: 100 }} />
		<If when={open()}>
			<Overlay testID="pop" anchor={anchor} placement="bottom" offset={4}
				onPressOutside={() => { presses.set(presses() + 1); open.set(false); }}>
				<Text>Menu</Text>
			</Overlay>
		</If>
		<Text testID="count" onPress={() => {}}>{() => "outside " + presses()}</Text>
	</View>;
}
mount(App);`)
	if !app.Exists("text:Menu") {
		t.Fatalf("no overlay: %q\n%s", app.Texts(), app.Dump())
	}
	pop, anchor := app.Find("id:pop"), app.Find("id:anchor")
	if got, want := pop.Frame.Origin.Y, anchor.Frame.Origin.Y+anchor.Frame.Size.Height+4; got != want {
		t.Errorf("the overlay is at y %v, want %v", got, want)
	}
	app.Press("id:count")
	if app.Exists("text:Menu") || !app.Exists("text:outside 1") {
		t.Errorf("after a press outside: %q", app.Texts())
	}
}

func TestTabBarSVGFromTSX(t *testing.T) {
	for _, os := range []core.OS{core.OSAndroid, core.OSIOS} {
		t.Run(string(os), func(t *testing.T) {
			app := run(t, `import { mount, signal, View, Text } from "nsgo";
import { TabBar } from "nsgo/mobile";
function App() {
    const selected = signal(0);
    return <View>
        <TabBar testID="bar" style={{ height: 80 }} selectedIndex={selected()}
            onChange={(e) => selected.set(e.index)} items={[
                { title: "Home", badge: selected() === 0 ? { text: "5" } : { dot: true }, svg: '<svg viewBox="0 0 24 24"><rect width="24" height="24"/></svg>' },
                { title: "Search", icon: "android:ic_menu_search" }
            ]} />
        <Text>{() => "selected " + selected()}</Text>
    </View>;
}
mount(App);`, nsgotest.Environment(core.Environment{OS: os}))
			bar := app.Find("id:bar")
			if got := bar.TabBadge(0); got != (core.TabBadge{Text: "5"}) {
				t.Fatalf("%s: TSX tab badge = %v, want 5", os, got)
			}
			items := bar.NativeProps["items"].([]any)
			if d, ok := items[0].(map[string]any)["drawing"].(*driver.Drawing); !ok || len(d.Ops) == 0 {
				t.Fatalf("%s: TSX SVG did not reach the native bar", os)
			}
			app.Emit("id:bar", "change", float64(1))
			app.Find("text:selected 1")
			if got := app.Find("id:bar").TabBadge(0); got != (core.TabBadge{Dot: true}) {
				t.Fatalf("%s: updated TSX tab badge = %v, want dot", os, got)
			}
		})
	}
}

func TestTabBarEdgeFromTSX(t *testing.T) {
	app := run(t, `import { mount, signal, View } from "nsgo";
import { TabBar } from "nsgo/mobile";
function App() {
    const selected = signal(0);
    return <View>
        <TabBar testID="bar" edge="automatic" style={{ width: 72, height: 49 }} selectedIndex={selected()}
            onChange={(e) => selected.set(e.index)} items={[
                { title: "Home", icon: "house" },
                { title: "Search", icon: "magnifyingglass" }
            ]} />
    </View>;
}
mount(App);`, nsgotest.Environment(core.Environment{OS: core.OSIOS}), nsgotest.VerticalBarEdge(core.VerticalBarTrailing))
	if got := app.Find("id:bar").NativeProps["edge"]; got != "trailing" {
		t.Fatalf("TSX TabBar edge = %v, want trailing", got)
	}
	app.SetVerticalBarEdge(core.VerticalBarLeading)
	if got := app.Find("id:bar").NativeProps["edge"]; got != "leading" {
		t.Fatalf("TSX TabBar edge after update = %v, want leading", got)
	}
}

func TestListRendersRowsWithAccessorsAndSlots(t *testing.T) {
	app := run(t, `import { mount, signal, List, Text } from "nsgo";
function App() {
	const items = signal(["a", "b", "c"]);
	return <List testID="list" each={items()} style={{ height: 400 }}
		header={<Text>Header</Text>} empty={() => <Text>Nothing</Text>}>
		{(item, index) => <Text onPress={() => items.set(items().filter((x) => x !== item()))}>{() => index() + ":" + item()}</Text>}
	</List>;
}
mount(App);`)
	for _, want := range []string{"Header", "0:a", "1:b", "2:c"} {
		if !app.Exists("text:" + want) {
			t.Fatalf("no %q among %q", want, app.Texts())
		}
	}
	app.Press("text:0:a")
	app.Press("text:0:b")
	app.Press("text:0:c")
	if !app.Exists("text:Nothing") {
		t.Errorf("the empty list shows %q", app.Texts())
	}
}

func TestNodeMethodsReachTheirViews(t *testing.T) {
	app := run(t, `import { mount, signal, Scroll, Text, View, WebView, Video } from "nsgo";
import { Input } from "nsgo/base";
function App() {
	let scroll, input, web, video;
	const where = signal("");
	return <View style={{ flex: 1 }}>
		<Scroll testID="scroll" ref={scroll} style={{ height: 100 }}>
			<View style={{ height: 1000 }} />
		</Scroll>
		<Input testID="field" ref={input} />
		<WebView testID="web" ref={web} src="https://example.com" style={{ height: 50 }} />
		<Video testID="video" ref={video} src="clip.mp4" paused style={{ height: 50 }} />
		<Text testID="go" onPress={async () => {
			scroll.scrollTo({ y: 300, animated: false });
			input.focus();
			web.postMessage("hi");
			video.seek(2.5);
			const m = await scroll.measure();
			where.set(m.bounds.size.height + "@" + m.pagePosition.y);
		}}>Go</Text>
		<Text testID="where">{where}</Text>
	</View>;
}
mount(App);`)
	app.Press("id:go")
	app.Settle()
	if v := app.Find("id:scroll"); v.Offset.Y != 300 {
		t.Errorf("the scroll view is at %v", v.Offset)
	}
	if !app.Find("id:field").Focused {
		t.Error("the input is not focused")
	}
	if v := app.Find("id:web"); len(v.WebCommands) == 0 {
		t.Error("the web view got no message")
	}
	if v := app.Find("id:video"); !slices.Contains(v.Seeks, 2500*time.Millisecond) || !v.Video.Paused {
		t.Errorf("the video: seeks %v, %+v", v.Seeks, v.Video)
	}
	app.WaitFor("text:100@0", time.Second)
}

// A change of injectedJavaScriptBeforeContentLoaded loads the source again with the new script in the same web view, as core's InjectedJavaScriptBeforeContentFunc does, rather than making a new one.
func TestWebViewBeforeContentScriptFollowsItsProp(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View, WebView } from "nsgo";
function App() {
	const script = signal("one");
	return <View>
		<WebView testID="web" src="https://example.com" style={{ height: 50 }} injectedJavaScriptBeforeContentLoaded={script()} />
		<Text testID="change" onPress={() => script.set("two")}>Change</Text>
	</View>;
}
mount(App);`)
	web := app.Find("id:web")
	if got := web.Web.InjectedJavaScriptBeforeContent; got != "one" {
		t.Fatalf("initial script %q", got)
	}
	app.Press("id:change")
	again := app.Find("id:web")
	if got := again.Web.InjectedJavaScriptBeforeContent; got != "two" {
		t.Fatalf("updated script %q", got)
	}
	if again.ID != web.ID {
		t.Fatalf("the script's change made a new web view, %v in place of %v", again.ID, web.ID)
	}
}

func TestWebViewMutedFollowsItsProp(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View, WebView } from "nsgo";
function App() {
	const muted = signal(false);
	return <View>
		<WebView testID="web" html="<p>Hello</p>" muted={muted()} style={{ height: 50 }} />
		<Text testID="toggle" onPress={() => muted.set(!muted())}>Toggle</Text>
	</View>;
}
mount(App);`)
	web := app.Find("id:web")
	for _, muted := range []bool{true, false} {
		app.Press("id:toggle")
		again := app.Find("id:web")
		if again.ID != web.ID || again.Web.Muted != muted {
			t.Fatalf("mute %v produced view %v with mute %v, want view %v", muted, again.ID, again.Web.Muted, web.ID)
		}
	}
}

func TestVideoReportsWhatThePlatformPausedAndPlayed(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View, Video } from "nsgo";
function App() {
	const heard = signal("");
	return <View style={{ flex: 1 }}>
		<Video testID="video" src="talk.mp4" style={{ height: 50 }}
			onPause={(e) => heard.set(heard() + "pause " + e.reason + ";")}
			onPlay={() => heard.set(heard() + "play;")} />
		<Text testID="heard">{heard}</Text>
	</View>;
}
mount(App);`)
	app.PauseVideo("id:video", core.VideoPauseRouteLost)
	app.PlayVideo("id:video")
	app.PauseVideo("id:video", core.VideoPauseControls)
	app.WaitFor("text:pause route-lost;play;pause controls;", time.Second)
}

func TestVideoPassesItsAlphaThreshold(t *testing.T) {
	app := run(t, `import { mount, Video } from "nsgo";
mount(() => <Video testID="video" src="pet.mov" hitTestAlpha={0.5} style={{ height: 50 }} />);`)
	if got := app.Find("id:video").Video.HitTestAlpha; got != 0.5 {
		t.Fatalf("video alpha threshold = %g, want 0.5", got)
	}
}

func TestVideoAudioSessionFollowsItsProp(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View, Video } from "nsgo";
function App() {
	const session = signal("playback");
	return <View style={{ flex: 1 }}>
		<Video testID="video" src="talk.mp4" audioSession={session()} style={{ height: 50 }} />
		<Text testID="mix" onPress={() => session.set("mixWithOthers")}>Mix</Text>
	</View>;
}
mount(App);`)
	v := app.Find("id:video")
	if v.Video.AudioSession != core.AudioSessionPlayback {
		t.Fatalf("mounted with %+v, want AudioSessionPlayback", v.Video)
	}
	id := v.ID
	app.Press("id:mix")
	app.Settle()
	if v := app.Find("id:video"); v.ID != id || v.Video.AudioSession != core.AudioSessionMixWithOthers || v.Video.Source != "talk.mp4" {
		t.Errorf("after the change: view %v (was %v), %+v", v.ID, id, v.Video)
	}
}

func TestWebViewMobileFollowsItsProp(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View, WebView } from "nsgo";
function App() {
	const mobile = signal(true);
	return <View style={{ flex: 1 }}>
		<WebView testID="web" src="https://m.example/" mobile={mobile()} style={{ height: 50 }} />
		<WebView testID="desktop" src="https://example.com/" style={{ height: 50 }} />
		<Text testID="off" onPress={() => mobile.set(false)}>Off</Text>
	</View>;
}
mount(App);`)
	if v := app.Find("id:web"); !v.Web.Mobile || v.Web.UserAgent != "" {
		t.Fatalf("mounted with %+v, want Mobile", v.Web)
	}
	if v := app.Find("id:desktop"); v.Web.Mobile {
		t.Errorf("a web view without mobile is Mobile: %+v", v.Web)
	}
	app.Press("id:off")
	app.Settle()
	if v := app.Find("id:web"); v.Web.Mobile || v.Web.Source.Src != "https://m.example/" {
		t.Errorf("after the change: %+v", v.Web)
	}
}

// userAgentSuffix reaches the web view's config, and a change makes a new web view with the new suffix, as Apple reads it only as the view is first made.
func TestWebViewUserAgentSuffixFollowsItsProp(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View, WebView } from "nsgo";
function App() {
	const suffix = signal("Version/26.0 Safari/605.1.15");
	return <View style={{ flex: 1 }}>
		<WebView testID="web" src="https://example.com/" userAgentSuffix={suffix()} style={{ height: 50 }} />
		<Text testID="change" onPress={() => suffix.set("Test/2")}>Change</Text>
	</View>;
}
mount(App);`)
	v := app.Find("id:web")
	if v.Web.UserAgentSuffix != "Version/26.0 Safari/605.1.15" || v.Web.UserAgent != "" {
		t.Fatalf("mounted with %+v, want the suffix alone", v.Web)
	}
	id := v.ID
	app.Press("id:change")
	app.Settle()
	if v := app.Find("id:web"); v.Web.UserAgentSuffix != "Test/2" || v.ID == id {
		t.Errorf("after the change: view %v (was %v), %+v", v.ID, id, v.Web)
	}
}

func TestWebViewMessagesCarryTheirOrigin(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View, WebView } from "nsgo";
function App() {
	const heard = signal("");
	const add = (e) => heard.set(heard() + e.data + "@" + e.origin + ";");
	return <View style={{ flex: 1 }}>
		<WebView testID="own" src="https://app.example/" style={{ height: 50 }} onMessage={add} />
		<WebView testID="listed" src="https://app.example/" messageOrigins={["https://other.example"]} style={{ height: 50 }} onMessage={add} />
		<Text testID="heard">{heard}</Text>
	</View>;
}
mount(App);`)
	app.PostMessage("id:own", "a")
	app.PostMessageFrom("id:own", "https://other.example/", true, "b")
	app.PostMessageFrom("id:listed", "https://app.example/", true, "c")
	app.PostMessageFrom("id:listed", "https://other.example/", false, "d")
	app.PostMessageFrom("id:listed", "https://other.example/", true, "e")
	app.WaitFor("text:a@https://app.example;e@https://other.example;", time.Second)
}

func TestPlatformPropsApplyOnTheirPlatform(t *testing.T) {
	src := `import { mount, View } from "nsgo";
mount(() => <View testID="v" style={{ width: 10, height: 10 }} ios={{ testID: "ios" }} desktop={{ accessibilityLabel: "desk" }} />);`
	app := run(t, src)
	v := app.Find("id:v")
	if v.Props.Label != "desk" {
		t.Errorf("on the desktop the label is %q", v.Props.Label)
	}
}

func TestStyleKeysDecode(t *testing.T) {
	app := run(t, `import { mount, View, Text } from "nsgo";
mount(() => <View>
	<View testID="v" style={{ width: "50%", height: 20, boxShadow: "0 2px 4px rgba(0,0,0,0.5)",
		backgroundImage: "linear-gradient(to right, red, blue)", filter: "blur(2px) grayscale(50%)",
		transform: [{ translateX: 10 }, { rotate: "45deg" }, { scale: 2 }], borderTopLeftRadius: 6,
		paddingTop: "calc(env(safe-area-inset-top) + 8px)", cursor: "pointer", gridColumn: "1 / span 2" }} />
	<Text testID="t" style={{ textDecorationLine: "underline line-through", textTransform: "uppercase",
		fontVariant: ["tabular-nums"], letterSpacing: 1 }}>hi</Text>
</View>);`)
	v := app.Find("id:v")
	fx := v.Style.Effects
	if fx == nil {
		t.Fatalf("no effects: %+v", v.Style)
	}
	if len(fx.Shadows) != 1 || fx.Shadows[0].Blur != 4 || fx.Shadows[0].Offset.Y != 2 {
		t.Errorf("shadows %+v", fx.Shadows)
	}
	if len(fx.Gradients) != 1 {
		t.Errorf("gradients %+v", fx.Gradients)
	}
	if len(fx.Filters) != 2 {
		t.Errorf("filters %+v", fx.Filters)
	}
	if tr := v.Style.Transform; tr.TranslateX != 10 || tr.Rotate != 45 || tr.ScaleX != 2 || tr.ScaleY != 2 {
		t.Errorf("transform %+v", tr)
	}
	if v.Style.Corners.TopLeft != 6 {
		t.Errorf("corners %+v", v.Style.Corners)
	}
	if got := app.Find("text:HI"); got == nil || got.TextStyle.Decoration != core.TextDecorationUnderline|core.TextDecorationLineThrough {
		t.Errorf("the text %+v", got)
	}
}

func TestAnimatedValuesBindAndAnimate(t *testing.T) {
	app := run(t, `import { mount, signal, createAnimatedValue, Animated, Easing, Text, View } from "nsgo";
function App() {
	const x = createAnimatedValue(0);
	const half = x.interpolate({ inputRange: [0, 100], outputRange: [0, 50] });
	const done = signal("no");
	return <View>
		<View testID="box" style={{ width: 10, height: 10, opacity: x.interpolate({ inputRange: [0, 100], outputRange: [1, 0] }), transform: [{ translateX: half }] }} />
		<Text testID="go" onPress={() => Animated.timing(x, { toValue: 100, duration: 200, easing: Easing.out(Easing.quad) }).start(({ finished }) => done.set(String(finished)))}>Go</Text>
		<Text>{() => "done " + done()}</Text>
	</View>;
}
mount(App);`)
	if box := app.Find("id:box"); box.Animated.Set == 0 {
		t.Fatalf("the box binds no animated values: %+v", box.Animated)
	}
	app.Press("id:go")
	app.FastForward(time.Second)
	app.WaitFor("text:done true", time.Second)
}

func TestPanResponderFollowsADrag(t *testing.T) {
	app := run(t, `import { mount, signal, PanResponder, Text, View } from "nsgo";
function App() {
	const log = signal([]);
	const push = (s) => log.set([...log(), s]);
	const pan = PanResponder.create({
		onStartShouldSetPanResponder: () => true,
		onPanResponderGrant: () => push("grant"),
		onPanResponderRelease: (e, g) => push("release " + Math.round(g.dx)),
	});
	return <View>
		<View testID="pad" {...pan.panHandlers} style={{ width: 200, height: 200 }} />
		<Text>{() => log().join(",")}</Text>
	</View>;
}
mount(App);`)
	app.Drag("id:pad", driver.Point{X: 10, Y: 10}, driver.Point{X: 40, Y: 10}, driver.Point{X: 70, Y: 10})
	if !app.Exists("text:grant,release 70") {
		t.Errorf("the pan logged %q", app.Texts())
	}
}

func TestWebAPIs(t *testing.T) {
	app := run(t, `import { mount, signal, Text } from "nsgo";
function App() {
	const out = signal("");
	(async () => {
		const u = new URL("/a/b?x=1#h", "https://example.com:443");
		u.searchParams.set("y", "2");
		const bytes = new TextEncoder().encode("héllo");
		const blob = new Blob([bytes, " ", "world"], { type: "text/plain" });
		const res = new Response(JSON.stringify({ ok: true }), { headers: { "Content-Type": "application/json" } });
		const fd = new FormData();
		fd.append("k", "v");
		const data = await fetch("data:text/plain,hi%20there").then((r) => r.text());
		out.set([u.href, u.origin, bytes.length, await blob.text(), (await res.json()).ok, res.headers.get("content-type"),
			btoa("ab"), atob("YWI="), fd.get("k"), data].join("|"));
	})();
	return <Text testID="out">{out}</Text>;
}
mount(App);`)
	want := "https://example.com/a/b?x=1&y=2#h|https://example.com|6|héllo world|true|application/json|YWI=|ab|v|hi there"
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:" + want) })
	if !app.Exists("text:" + want) {
		t.Errorf("got %q, want %q", app.Texts(), want)
	}
	_ = fmt.Sprint
}

func TestCanvasRecordsWhatDrawDraws(t *testing.T) {
	app := run(t, `import { mount, signal, Canvas, Path2D, Text, View } from "nsgo";
function App() {
	const n = signal(1);
	return <View>
		<Canvas testID="c" style={{ width: 100, height: 50 }} draw={(cx, size) => {
			cx.fillStyle = "red";
			for (let i = 0; i < n(); i++) cx.fillRect(i * 10, 0, 8, size.height);
			const p = new Path2D("M0 0 L10 10");
			cx.strokeStyle = cx.createLinearGradient(0, 0, 10, 0);
			cx.lineWidth = 2;
			cx.stroke(p);
			cx.font = "bold 12px serif";
			cx.fillText(String(cx.measureText("ab").width > 0), 0, 40);
		}} />
		<Text testID="more" onPress={() => n.set(3)}>More</Text>
	</View>;
}
mount(App);`)
	count := func() int {
		n := 0
		for _, op := range app.Find("id:c").Ops {
			if op.Kind == driver.DrawRect {
				n++
			}
		}
		return n
	}
	if got := count(); got != 1 {
		t.Fatalf("%d rects, want 1: %+v", got, app.Find("id:c").Ops)
	}
	app.Press("id:more")
	if got := count(); got != 3 {
		t.Errorf("%d rects after the press, want 3", got)
	}
}

func TestCanvasImageSmoothing(t *testing.T) {
	app := run(t, `import { mount, Canvas } from "nsgo";
mount(() => <Canvas testID="c" style={{ width: 100, height: 50 }} draw={(ctx) => {
	const src = "asset:checker.png";
	if (!ctx.imageSmoothingEnabled) throw new Error("smoothing starts enabled");
	ctx.drawImage(src, 0, 0);
	ctx.save();
	ctx.imageSmoothingEnabled = false;
	ctx.drawImage(src, 0, 0, 40, 40);
	ctx.save();
	ctx.imageSmoothingEnabled = true;
	ctx.restore();
	if (ctx.imageSmoothingEnabled) throw new Error("inner restore should disable smoothing");
	ctx.drawImage(src, 0, 0, 16, 16, 0, 0, 40, 40);
	ctx.restore();
	if (!ctx.imageSmoothingEnabled) throw new Error("outer restore should enable smoothing");
	ctx.drawImage(src, 0, 0);
}} />);`)
	var got []core.ImageSampling
	for _, op := range app.Find("id:c").Ops {
		if op.Kind == driver.DrawImage {
			got = append(got, op.Sampling)
		}
	}
	want := []core.ImageSampling{core.ImageSamplingSmooth, core.ImageSamplingNearest, core.ImageSamplingNearest, core.ImageSamplingSmooth}
	if !slices.Equal(got, want) {
		t.Errorf("image sampling = %v, want %v", got, want)
	}
}

func TestCanvasRecordsTintedImage(t *testing.T) {
	app := run(t, `import { mount, Canvas } from "nsgo";
mount(() => <Canvas testID="canvas" style={{ width: 100, height: 50 }} draw={cx => {
	cx.drawTintedImage("asset:/sprite.png", "#4080c080", 4, 3, 2, 1, 10, 20, 30, 40);
	cx.drawImage("asset:/sprite.png", 0, 0);
}} />);`)
	ops := app.Find("id:canvas").Ops
	if len(ops) != 2 {
		t.Fatalf("recorded %d ops, want two images", len(ops))
	}
	tint := ops[0]
	if tint.Kind != driver.DrawImage || tint.Image == nil || tint.Image.URI != "asset:/sprite.png" ||
		!tint.Tinted || tint.Color != 0x4080c080 || tint.X1 != 4 || tint.Y1 != 3 || tint.X2 != 2 || tint.Y2 != 1 ||
		tint.X != 10 || tint.Y != 20 || tint.W != 30 || tint.H != 40 {
		t.Errorf("tinted image recorded as %+v", tint)
	}
	if ops[1].Kind != driver.DrawImage || ops[1].Tinted {
		t.Errorf("next image recorded as %+v", ops[1])
	}
}

func TestDialogOpensAndReportsItsClose(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View } from "nsgo";
import { Dialog } from "nsgo/base";
function App() {
	const open = signal(false), why = signal("");
	return <View style={{ flex: 1 }}>
		<Text testID="open" onPress={() => open.set(true)}>Open</Text>
		<Dialog testID="dialog" open={open()} backdropClass="bg-black/50" class="p-4"
			onOpenChange={(e) => { open.set(e.open); why.set(e.reason); }}>
			<Text>Inside</Text>
		</Dialog>
		<Text>{() => "why " + why()}</Text>
	</View>;
}
mount(App);`)
	if app.Exists("text:Inside") {
		t.Fatal("the dialog shows before it opens")
	}
	app.Press("id:open")
	app.WaitFor("text:Inside", time.Second)
	app.Back()
	app.WaitForGone("text:Inside", time.Second)
	if !app.Exists("text:why cancel") {
		t.Errorf("after back: %q", app.Texts())
	}
}

func TestServices(t *testing.T) {
	app := run(t, `import { mount, signal, Clipboard, Storage, keymap, resource, Text } from "nsgo";
import { isAvailable as mobile, BackHandler } from "nsgo/mobile";
import { isAvailable as desktop, Window, titleBar } from "nsgo/desktop";
function App() {
	const out = signal(""), keys = signal("");
	keymap.bind("mod+k", (e) => keys.set(e.keystroke));
	const data = resource(() => new Promise((ok) => setTimeout(() => ok("loaded"), 10)));
	(async () => {
		await Storage.setItem("a", "1");
		await Clipboard.writeText("copied");
		out.set([await Storage.getItem("a"), await Storage.getItem("missing"), (await Storage.keys()).join(),
			await Clipboard.readText(), mobile, desktop, typeof Window.setTitle, typeof titleBar().height].join("|"));
	})();
	return <>
		<Text testID="out">{out}</Text>
		<Text>{() => "keys " + keys()}</Text>
		<Text>{() => "data " + (data.loading() ? "…" : data())}</Text>
	</>;
}
mount(App);`)
	app.WaitUntil(time.Second, func() bool { return app.Find("id:out").Text != "" })
	if got, want := app.Find("id:out").Text, "1||a|copied|false|false|function|number"; got != want {
		t.Errorf("services gave %q, want %q", got, want)
	}
	app.Key("mod+k")
	app.FastForward(100 * time.Millisecond)
	app.WaitFor("text:data loaded", time.Second)
	if !app.Exists("text:keys mod+k") {
		t.Errorf("the binding did not report mod+k: %q", app.Texts())
	}
}

func TestScheduledNotifications(t *testing.T) {
	app := run(t, `import { mount, signal, Notifications, Text } from "nsgo";
function App() {
	const out = signal("");
	(async () => {
		await Notifications.requestPermission();
		const can = await Notifications.canSchedule();
		const later = await Notifications.show({ id: "review", title: "Review", at: new Date(Date.now() + 60000) });
		const daily = await Notifications.show({ id: "daily", title: "Daily", at: Date.now(), repeat: "daily" });
		let bad = "";
		try { await Notifications.show({ id: "x", title: "X", at: Date.now(), repeat: "hourly" }); } catch (e) { bad = e.name; }
		out.set([can, later, daily, bad].join("|"));
	})();
	return <Text testID="out">{out}</Text>;
}
mount(App);`, nsgotest.Environment(core.Environment{ScheduledNotifications: true}))
	app.WaitUntil(time.Second, func() bool { return app.Find("id:out").Text != "" })
	if got, want := app.Find("id:out").Text, "true|true|true|RangeError"; got != want {
		t.Errorf("scheduling gave %q, want %q", got, want)
	}
	if len(app.PendingNotifications) != 2 {
		t.Fatalf("pending %+v", app.PendingNotifications)
	}
	app.FastForward(time.Minute)
	if len(app.Notifications) != 1 || app.Notifications[0].ID != "review" {
		t.Errorf("a minute later: %+v", app.Notifications)
	}
}

// A dialog shadows the page's keymap bindings, but not one bound with overModals.
func TestKeymapOverModals(t *testing.T) {
	app := run(t, `import { mount, signal, keymap, View, Text } from "nsgo";
import { Dialog } from "nsgo/base";
function App() {
	const log = signal("");
	keymap.bind("space", () => log.set(log() + "space;"));
	keymap.bind("f1", () => log.set(log() + "f1;"), { overModals: true });
	return <View>
		<Text testID="log">{() => "log " + log()}</Text>
		<Dialog open={true}><Text testID="dialog">In the dialog</Text></Dialog>
	</View>;
}
mount(App);`, nsgotest.Environment(core.Environment{OS: core.OSMacOS}))
	app.Find("id:dialog")
	app.Key("space")
	app.Key("f1")
	if got := app.Find("id:log").Text; got != "log f1;" {
		t.Errorf("under the dialog: %q, want %q", got, "log f1;")
	}
}

// A binding bound with yieldsToMenu leaves its key to an enabled item of the application menu with the same shortcut, and takes it back once the item is disabled.
func TestKeymapYieldsToMenu(t *testing.T) {
	app := run(t, `import { mount, signal, effect, keymap, View, Text } from "nsgo";
import { Menu } from "nsgo/desktop";
function App() {
	const log = signal(""), disabled = signal(false);
	keymap.bind("mod+r", () => log.set(log() + "key;"), { yieldsToMenu: true });
	keymap.bind("f2", () => disabled.set(true));
	effect(() => Menu.setApplicationMenu([{ label: "File", submenu: [{ label: "Run", shortcut: "mod+r", disabled: disabled(), onSelect: () => log.set(log() + "menu;") }] }]));
	return <View>
		<Text testID="log">{() => "log " + log()}</Text>
	</View>;
}
mount(App);`, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Key("mod+r")
	if got := app.Find("id:log").Text; got != "log menu;" {
		t.Errorf("with the menu's item: %q, want %q", got, "log menu;")
	}
	app.Key("f2")
	app.Key("mod+r")
	if got := app.Find("id:log").Text; got != "log menu;key;" {
		t.Errorf("with the item disabled: %q, want %q", got, "log menu;key;")
	}
}

func TestHostCallsPostsAndEvents(t *testing.T) {
	h := core.NewHost()
	_ = h.SetParams(map[string]any{"user": "ada"})
	var posted []any
	h.Handle("greet", func(c *core.HostCall) {
		if c.IsPost() {
			posted = append(posted, c.Data)
			return
		}
		c.Resolve("hello " + c.Data.(map[string]any)["name"].(string))
	})
	app := run(t, `import { mount, signal, host, Text } from "nsgo";
function App() {
	const out = signal(""), events = signal([]);
	host.on("tick", (n) => events.set([...events(), n]));
	(async () => {
		host.post("greet", "posted");
		const reply = await host.call("greet", { name: host.params.user });
		let failed = "";
		try { await host.call("missing"); } catch (e) { failed = "rejected"; }
		out.set([host.available(), reply, failed].join("|"));
	})();
	return <><Text testID="out">{out}</Text><Text>{() => "events " + events().join(",")}</Text></>;
}
mount(App);`, nsgotest.Host(h))
	app.WaitUntil(time.Second, func() bool { return app.Find("id:out").Text != "" })
	if got := app.Find("id:out").Text; got != "true|hello ada|rejected" {
		t.Errorf("host gave %q", got)
	}
	_ = h.Send("tick", 1)
	_ = h.Send("tick", 2)
	app.Settle()
	if !app.Exists("text:events 1,2") || len(posted) != 1 {
		t.Errorf("events %q, posted %v", app.Texts(), posted)
	}
}

func TestIntlFormats(t *testing.T) {
	app := run(t, `import { mount, Text } from "nsgo";
const d = new Date(Date.UTC(2026, 10, 1, 15, 4, 5));
const out = [
	(1234567.891).toLocaleString("en-US"),
	(1234567.891).toLocaleString("de-DE"),
	new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(-42.5),
	new Intl.NumberFormat("en-US", { style: "percent", maximumFractionDigits: 1 }).format(0.1234),
	new Intl.NumberFormat("en", { notation: "compact" }).format(15300),
	new Intl.PluralRules("en").select(1) + "," + new Intl.PluralRules("en").select(2),
	new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", timeZone: "UTC" }).format(d),
	new Intl.RelativeTimeFormat("en", { numeric: "auto" }).format(-1, "day"),
	["b", "a", "C"].sort(new Intl.Collator("en").compare).join(""),
].join(" | ");
mount(() => <Text testID="out">{out}</Text>);`)
	want := "1,234,567.891 | 1.234.567,891 | -$42.50 | 12.3% | 15K | one,other | Nov 1 | yesterday | abC"
	if got := app.Find("id:out").Text; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestNsgoYamlThemesTheApp(t *testing.T) {
	fsys := fstest.MapFS{
		"nsgo.yaml": {Data: []byte("name: T\ntheme: theme.json\n")},
		"theme.json": {Data: []byte(`{"cssVars": {"theme": {"radius-card": "16px"},
			"light": {"brand": "#4f6e0c", "muted": "#f0f0ec"}, "dark": {"brand": "#c8f26d"}}}`)},
		"App.tsx": {Data: []byte(`import { mount, token, View, Text } from "nsgo";
mount(() => <View testID="box" class="bg-muted border-brand border-2">
	<Text testID="t">{[token("brand"), token("radius-card"), token("missing")].join(" ")}</Text>
</View>);`)},
	}
	app := nsgotest.Start(t, compiler.App(fsys, "App.tsx"))
	if v := app.Find("id:box"); v.Style.Background != core.RGB(0xf0f0ec) || v.Style.BorderColor != core.RGB(0x4f6e0c) {
		t.Errorf("the box is %#x, bordered %#x", v.Style.Background, v.Style.BorderColor)
	}
	if got := app.Find("id:t").Text; got != "#4f6e0cff 16 " {
		t.Errorf("tokens %q", got)
	}
}

func TestNsgoYamlExtendsTailwind(t *testing.T) {
	fsys := fstest.MapFS{
		"nsgo.yaml": {Data: []byte("name: T\ntailwind:\n  colors: { brand: { 500: \"#7c3aed\" } }\n  spacing: 5\n")},
		"App.tsx": {Data: []byte(`import { mount, View } from "nsgo";
mount(() => <View testID="box" class="bg-brand-500 w-2 h-2" />);`)},
	}
	app := nsgotest.Start(t, compiler.App(fsys, "App.tsx"))
	if v := app.Find("id:box"); v.Style.Background != core.RGB(0x7c3aed) || v.Frame.Size.Width != 10 {
		t.Errorf("the box is %#x, %v wide", v.Style.Background, v.Frame.Size.Width)
	}
	fsys["nsgo.yaml"] = &fstest.MapFile{Data: []byte("name: T\ntailwind:\n  colours: {}\n")}
	if _, err := bundler.Compile(fsys, "App.tsx"); err == nil || !strings.Contains(err.Error(), "colours") {
		t.Errorf("an unknown key compiles: %v", err)
	}
}

func TestSandboxHardensTypedArrays(t *testing.T) {
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, Text } from "nsgo";
const arrays = ["Uint8Array", "Uint8ClampedArray", "Int8Array", "Uint16Array", "Int16Array", "Uint32Array",
    "Int32Array", "Float16Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array", "DataView"];
const typed = Object.getPrototypeOf(Uint8Array);
const frozen = [typed, ...arrays.map(name => globalThis[name]).filter(Boolean)].every(C =>
    Object.isFrozen(C) && Object.isFrozen(C.prototype));
const fixedMethods = ["set", "subarray"].every(name => !Object.getOwnPropertyDescriptor(typed.prototype, name).writable);
const bytes = new Uint8Array(3);
bytes.set([110, 115, 103]);
const text = new TextDecoder().decode(bytes.subarray(0, 2));
const generator = Object.getPrototypeOf((function* () {}).prototype);
const fixedFunctionPrototypes = Object.isFrozen(Function.prototype) && Object.isFrozen(generator);
mount(() => <Text testID="result">{frozen && fixedMethods && fixedFunctionPrototypes ? text : "unfrozen"}</Text>);`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, tsx.Sandboxed("App.tsx", code, tsx.Sandbox{}))
	if got := app.Find("id:result").Text; got != "ns" {
		t.Fatalf("hardened typed-array check = %q, want ns", got)
	}
}

func TestSandboxLimitsCutRunawayScripts(t *testing.T) {
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal, Text } from "nsgo";
let made = "no";
try { made = String(new Function("return 1")()); } catch (e) { made = "refused"; }
let frozen = "no";
try { Array.prototype.evil = 1; } catch (e) {}
if ([].evil === undefined) frozen = "frozen";
mount(() => <Text testID="spin" onPress={() => { for (;;) {} }}>{made + " " + frozen + " " + typeof eval}</Text>);`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	limits := tsx.DefaultLimits
	limits.Call = 50 * time.Millisecond
	var failed error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Catch{Try: tsx.Sandboxed("App.tsx", code, tsx.Sandbox{Limits: &limits}), Fallback: func(cx *core.Scope, err error, _ func()) core.Node {
			failed = err
			return core.Text{Content: "cut"}
		}}
	})
	if got := app.Find("id:spin").Text; got != "refused frozen undefined" {
		t.Errorf("hardening: %q", got)
	}
	if runtime.GOOS == "js" {
		t.Skip("the page's engine cannot be interrupted: a web app's time budgets are the browser's")
	}
	app.Press("id:spin")
	if failed == nil || !strings.Contains(failed.Error(), "time budget") {
		t.Errorf("the runaway handler ended with %v", failed)
	}
}

func TestSandboxLimitsCutBacktrackingRegExp(t *testing.T) {
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, Text } from "nsgo";
mount(() => <Text testID="regex" onPress={() => /(a+)+\1$/.test("a".repeat(28) + "!")}>regex</Text>);`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	limits := tsx.DefaultLimits
	limits.Load = 0
	limits.Call = 100 * time.Millisecond
	var failed error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Catch{Try: tsx.Sandboxed("App.tsx", code, tsx.Sandbox{Limits: &limits}), Fallback: func(cx *core.Scope, err error, _ func()) core.Node {
			failed = err
			return core.Text{Content: "cut"}
		}}
	})
	if runtime.GOOS == "js" {
		t.Skip("the page's engine cannot be interrupted: a web app's time budgets are the browser's")
	}
	started := time.Now()
	app.Press("id:regex")
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("backtracking match took %v", elapsed)
	}
	if failed == nil || !strings.Contains(failed.Error(), "time budget") {
		t.Fatalf("backtracking handler ended with %v", failed)
	}
}

// nestedPlugin is what host/nested's call calls, from a sandboxed bundle, and nestedResult what that call heard.
var (
	nestedPlugin *tsx.Plugin
	nestedResult string
)

func init() {
	tsx.RegisterFunctions("host/nested", map[string]any{"call": func() {
		nestedPlugin.Call("f", nil, func(v any, err error) { nestedResult = fmt.Sprint(v, " ", err) })
	}})
}

// A sandboxed call that runs a plugin's on the same goroutine, through a host function, neither waits on itself nor loses its own match timeout when the plugin's call returns, and the process-wide default comes back once both end.
func TestSandboxRegExpTimeoutsNest(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the page's engine runs its own regular expressions")
	}
	p := verified(t, `export function f() { return "b"; }`, pkg.Capabilities{})
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, Text } from "nsgo";
import { call } from "host/nested";
mount(() => <Text testID="a" onPress={() => { call(); new RegExp("(a+)+\\1$").test("a".repeat(28) + "!"); }}>a</Text>);`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	limits := tsx.DefaultLimits
	limits.Call = 100 * time.Millisecond
	var loadErr, failed error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		nestedPlugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{})
		return core.Catch{Try: tsx.Sandboxed("App.tsx", code, tsx.Sandbox{Limits: &limits, Modules: []string{"host/nested.call"}}), Fallback: func(cx *core.Scope, err error, _ func()) core.Node {
			failed = err
			return core.Text{Content: "cut"}
		}}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	before := regexp2.DefaultMatchTimeout
	started := time.Now()
	app.Press("id:a")
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("the nested call took %v", elapsed)
	}
	if nestedResult != "b <nil>" {
		t.Errorf("the plugin's call gave %q", nestedResult)
	}
	if failed == nil || !strings.Contains(failed.Error(), "time budget") {
		t.Errorf("the handler ended with %v", failed)
	}
	if regexp2.DefaultMatchTimeout != before || before != time.Duration(math.MaxInt64) {
		t.Errorf("regexp2's default is %v after the calls, %v before", regexp2.DefaultMatchTimeout, before)
	}
}

func TestTouchesCaptureThenBubbleUntilStopped(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View } from "nsgo";
const log = signal("");
const add = (s, stop) => (e) => { log.set(log() + s + " "); if (stop) e.stopPropagation(); };
mount(() => <View>
	<View testID="outer" onTouchStartCapture={add("outer-capture")} onTouchStart={add("outer")}>
		<View testID="inner" style={{ height: 40 }} onTouchStartCapture={add("inner-capture")} onTouchStart={add("inner", true)} />
	</View>
	<Text testID="log">{log}</Text>
</View>);`)
	app.Drag("id:inner", driver.Point{Y: 4})
	if got := app.Find("id:log").Text; got != "outer-capture inner-capture inner " {
		t.Errorf("log %q: capture in, bubble out, stopped at inner", got)
	}
}

type quote struct {
	Symbol string
	Price  float64
}

func init() {
	tsx.RegisterFunctions("host/trading", map[string]any{
		"quote": func(symbol string) (quote, error) {
			if symbol == "" {
				return quote{}, errors.New("no symbol")
			}
			return quote{Symbol: symbol, Price: 42.5}, nil
		},
	})
}

func TestNativeModuleGivesRegisteredFunctions(t *testing.T) {
	app := run(t, `import { mount, nativeModule, Text } from "nsgo";
import { quote } from "host/trading";
const trading = nativeModule("host/trading");
const q = trading.quote("AAPL");
let failed = "", missing = "";
try { trading.quote(""); } catch (e) { failed = String(e.message ?? e); }
try { nativeModule("host/nope"); } catch (e) { missing = e instanceof ReferenceError ? "ReferenceError" : String(e); }
mount(() => <Text testID="out">{[q.symbol, q.price, quote === trading.quote, failed, missing].join(" ")}</Text>);`)
	if got := app.Find("id:out").Text; got != "AAPL 42.5 true no symbol ReferenceError" {
		t.Errorf("got %q", got)
	}
	if d := tsx.Declarations()["types/host/trading.d.ts"]; !strings.Contains(d, "export declare function quote(arg0: string): ") {
		t.Errorf("declarations:\n%s", d)
	}
}

func TestScrollContentStyleAndOverlayRoom(t *testing.T) {
	app := run(t, `import { mount, Overlay, Scroll, signal, Text, View } from "nsgo";
const room = signal("");
let anchor;
mount(() => <View>
	<Scroll testID="scroll" contentStyle={{ padding: 12 }} contentClass="gap-2">
		<Text testID="row">Row</Text>
	</Scroll>
	<View ref={(r) => (anchor = r)} style={{ height: 20 }} />
	<Overlay anchor={anchor} placement="bottom" margin={8}
		onPosition={(p) => room.set(p.placement + " " + p.available.width + "x" + Math.round(p.available.height))}>
		<Text>Pop</Text>
	</Overlay>
	<Text testID="room">{room}</Text>
</View>);`)
	if f := app.Find("id:row").Frame; f.Origin.X != 12 || f.Origin.Y != 12 {
		t.Errorf("the row at %v, want inside the content's padding", f.Origin)
	}
	if got := app.Find("id:room").Text; !strings.HasPrefix(got, "bottom 374x") {
		t.Errorf("onPosition %q: the room below, inside the margins", got)
	}
}

func TestFontImportsAreTheirFamiliesAndRegister(t *testing.T) {
	fsys := fstest.MapFS{
		"go.ttf": {Data: gomedium.TTF},
		"App.tsx": {Data: []byte(`import { mount, Text } from "nsgo";
import family from "./go.ttf";
mount(() => <Text testID="t" style={{ fontFamily: family }}>{family}</Text>);`)},
	}
	code, err := bundler.Compile(fsys, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(code, "registerFont(") {
		t.Error("the bundle does not register the font")
	}
	app := nsgotest.Start(t, compiler.App(fsys, "App.tsx"))
	if got := app.Find("id:t").Text; got != "Go Medium" {
		t.Errorf("the import is %q, want the font's family", got)
	}
}

func TestScrollEndsReportRest(t *testing.T) {
	app := run(t, `import { List, mount, Scroll, signal, Text, View } from "nsgo";
const ends = signal(0), listEnds = signal(0);
const rows = Array.from({ length: 50 }, (_, i) => i);
mount(() => <View style={{ flex: 1 }}>
	<Scroll testID="scroll" style={{ height: 200 }} onScrollEnd={() => ends.set(ends() + 1)}>
		{rows.map((i) => <Text style={{ height: 40 }}>{"Row " + i}</Text>)}
	</Scroll>
	<List testID="list" each={rows} estimatedItemSize={40} style={{ height: 200 }} onScrollEnd={() => listEnds.set(listEnds() + 1)}>
		{(i) => <Text style={{ height: 40 }}>{"Item " + i()}</Text>}
	</List>
	<Text testID="ends">{() => ends() + " " + listEnds()}</Text>
</View>);`)
	app.Scroll("id:scroll", 300)
	app.Scroll("id:list", 300)
	if got := app.Find("id:ends").Text; got != "1 1" {
		t.Errorf("scroll ends %q, want one each", got)
	}
}

func TestFailuresCarryStableCodes(t *testing.T) {
	app := run(t, `import { mount, signal, Text, Storage, host } from "nsgo";
function App() {
	const out = signal("");
	const code = (p) => p.then(() => "kept", (e) => e.name + " " + e.code);
	(async () => {
		out.set([await code(Storage.setItem("big", "x".repeat(6 * 1024 * 1024))), await code(host.call("x"))].join("|"));
	})();
	return <Text testID="out">{out}</Text>;
}
mount(App);`)
	want := "Error ERR_NSH_QUOTA_EXCEEDED|Error ERR_FEATURE_UNAVAILABLE_ON_PLATFORM"
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:" + want) })
	if !app.Exists("text:" + want) {
		t.Errorf("got %q, want %q", app.Texts(), want)
	}
}

func TestShippedImagesAreAssets(t *testing.T) {
	var pixel bytes.Buffer
	if err := png.Encode(&pixel, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{"a.png": {Data: pixel.Bytes()}, "App.tsx": {Data: []byte(`import { mount, signal, Text } from "nsgo";
import a from "./a.png";
function App() {
	const out = signal("");
	(async () => {
		const res = await fetch(a.url);
		const bytes = await res.bytes();
		const missing = await fetch("asset:/b.png").catch((e) => e.name);
		out.set([a.url, res.headers.get("content-type"), res.headers.get("content-length") == bytes.length, missing].join("|"));
	})();
	return <Text testID="out">{out}</Text>;
}
mount(App);`)}}
	app := nsgotest.Start(t, compiler.App(fsys, "App.tsx"))
	want := "asset:/a.png|image/png|true|TypeError"
	app.WaitUntil(2*time.Second, func() bool { return app.Exists("text:" + want) })
	if !app.Exists("text:" + want) {
		t.Errorf("got %q, want %q", app.Texts(), want)
	}
}

// TestWindowWorkspaces puts the window on every workspace and floats it, each its own request, so the level keeps the workspaces.
func TestWindowWorkspaces(t *testing.T) {
	app := run(t, `import { mount, Text } from "nsgo";
import { Window } from "nsgo/desktop";
function App() {
	Window.setWorkspaces("all");
	Window.setLevel("floating");
	return <Text>Pet</Text>;
}
mount(App);`)
	want := []nsgotest.WindowRequest{{Action: nsgotest.WindowSetWorkspaces, Workspaces: core.WindowWorkspacesAll},
		{Action: nsgotest.WindowSetLevel, Level: core.WindowLevelFloating}}
	if !slices.Equal(app.Window, want) {
		t.Errorf("the window was sent %+v; want %+v", app.Window, want)
	}
}

func TestVideoUndefinedVolumeUsesItsDefault(t *testing.T) {
	app := run(t, `import { mount, signal, Text, View, Video } from "nsgo";
function App() {
    const volume = signal(undefined);
    return <View>
        <Video testID="video" src="talk.mp4" volume={volume()} paused style={{height:50}}/>
        <Video testID="omitted" src="talk.mp4" paused style={{height:50}}/>
        <Video testID="undefined" src="talk.mp4" volume={undefined} paused style={{height:50}}/>
        <Text testID="quiet" onPress={() => volume.set(0.25)}>Quiet</Text>
        <Text testID="full" onPress={() => volume.set(0)}>Full</Text>
        <Text testID="reset" onPress={() => volume.set(undefined)}>Reset</Text>
    </View>;
}
mount(App);`)
	id := app.Find("video").ID
	for _, name := range []string{"video", "omitted", "undefined"} {
		if v := app.Find(name).Video.Volume; v != 1 {
			t.Fatalf("%s volume %v, want 1", name, v)
		}
	}
	for _, step := range []struct {
		button string
		volume float32
	}{{"quiet", 0.25}, {"full", 1}, {"reset", 1}} {
		app.Press(step.button)
		app.Settle()
		if v := app.Find("video"); v.ID != id || v.Video.Volume != step.volume {
			t.Fatalf("%s: ID %v (was %v), volume %v, want %v", step.button, v.ID, id, v.Video.Volume, step.volume)
		}
	}
}
