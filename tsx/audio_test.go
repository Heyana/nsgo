package tsx_test

import (
	"fmt"
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

func TestAudioDeclarationsIncluded(t *testing.T) {
	decl := tsx.Declarations()["types/nsgo/audio.d.ts"]
	for _, want := range []string{
		"export interface Sound",
		"export interface Playback",
		"export declare function supported(): boolean;",
		"export declare function loadSound(uri: string): Promise<Sound>;",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("types/nsgo/audio.d.ts missing %q", want)
		}
	}
}

// soundDurations has the fake decode one 20-second sound and 200 short ones, s0.wav to s199.wav.
func soundDurations(a *nsgotest.App) {
	a.AudioDurations = map[string]time.Duration{"asset:long.wav": 20 * time.Second}
	for i := range 200 {
		a.AudioDurations[fmt.Sprintf("asset:s%d.wav", i)] = 10 * time.Millisecond
	}
}

// raised is a Go app that raises the engine's sound limits before showing node.
func raised(node func(cx *core.Scope) core.Node) func(cx *core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		if err := cx.SetSoundLimits(core.SoundLimits{MaxSounds: 250, MaxDuration: 30 * time.Second, MaxDecodedBytes: 64 << 20}); err != nil {
			panic(err)
		}
		return node(cx)
	}
}

// loadsAll loads a 20-second sound and then n short ones, and shows what each load came to.
func loadsAll(n int) string {
	return fmt.Sprintf(`
import { mount, signal, View, Text } from "nsgo";
import { loadSound } from "nsgo/audio";

const long = signal("loading");
const short = signal("loading");

function failure(e: any) { return "rejected:" + e.code + ":" + e.reason; }

// The short sounds load once the long one settled, so a refused long sound has returned its slot.
loadSound("asset:long.wav").then((s) => long.set("loaded:" + s.duration), (e) => long.set(failure(e)))
  .then(() => Promise.allSettled(Array.from({ length: %d }, (_, i) => loadSound("asset:s" + i + ".wav")))).then((results) => {
  const ok = results.filter((r) => r.status === "fulfilled").length;
  const failed = results.filter((r): r is PromiseRejectedResult => r.status === "rejected").map((r) => failure(r.reason));
  short.set(ok + "|" + Array.from(new Set(failed)).join(","));
});

function App() {
  return <View>
    <Text testID="long">{long}</Text>
    <Text testID="short">{short}</Text>
  </View>;
}
mount(App);
`, n)
}

// The app's own TSX is the Go app's code, so it gets the limits the Go app raised.
func TestAudioTSXAppTakesTheGoAppsLimits(t *testing.T) {
	headlessAudio(t)
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(loadsAll(200))}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, raised(tsx.App("App.tsx", code)), playsAudio, soundDurations)
	app.Find("text:loaded:20000")
	app.Find("text:200|")
}

// A package keeps NSH's default limits whatever the Go app hosting it raised.
func TestAudioPackageKeepsTheDefaultQuotas(t *testing.T) {
	headlessAudio(t)
	p := verified(t, loadsAll(65), pkg.Capabilities{Audio: true})
	app := nsgotest.Start(t, raised(func(*core.Scope) core.Node {
		return tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}
	}), playsAudio, soundDurations)
	app.Find("text:64|rejected:ERR_NSH_QUOTA_EXCEEDED:quota-exceeded")
	if got := app.Find("id:long").Text; got != "rejected:ERR_NSH_QUOTA_EXCEEDED:quota-exceeded" {
		t.Fatalf("a 20-second sound in a package: %s", got)
	}
}

// A package past the default decoded size is refused even where the Go app raised it: nine sounds of 4.9 seconds decode to more than 16 MiB in nsgotest's fake.
func TestAudioPackageKeepsTheDecodedQuota(t *testing.T) {
	headlessAudio(t)
	p := verified(t, `
import { mount, signal, Text } from "nsgo";
import { loadSound } from "nsgo/audio";

const result = signal("loading");
(async () => {
  const got: string[] = [];
  for (let i = 0; i < 9; i++) {
    try { await loadSound("asset:m" + i + ".wav"); got.push("ok"); } catch (e: any) { got.push(e.code + ":" + e.reason); }
  }
  result.set(got.join(","));
})();

mount(() => <Text testID="result">{result}</Text>);
`, pkg.Capabilities{Audio: true})
	app := nsgotest.Start(t, raised(func(*core.Scope) core.Node {
		return tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}
	}), playsAudio, func(a *nsgotest.App) {
		a.AudioDurations = map[string]time.Duration{}
		for i := range 9 {
			a.AudioDurations[fmt.Sprintf("asset:m%d.wav", i)] = 4900 * time.Millisecond
		}
	})
	app.Find("text:" + strings.Repeat("ok,", 8) + "ERR_NSH_QUOTA_EXCEEDED:quota-exceeded")
}

// A package's playbacks reach the platform restricted, which holds them to the restricted share of the output's voices.
func TestAudioPackagePlaysRestricted(t *testing.T) {
	headlessAudio(t)
	p := verified(t, `
import { mount, signal, Text } from "nsgo";
import { loadSound } from "nsgo/audio";

const result = signal("loading");
loadSound("asset:s0.wav").then((s) => { s.play(); result.set("played"); });

mount(() => <Text testID="result">{result}</Text>);
`, pkg.Capabilities{Audio: true})
	app := nsgotest.Start(t, raised(func(*core.Scope) core.Node {
		return tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}
	}), playsAudio, soundDurations)
	app.Find("text:played")
	if pb := app.Sounds[0].Playbacks; len(pb) != 1 || !pb[0].Restricted {
		t.Fatalf("a package's playbacks: %+v", pb)
	}
}

// Every load holds Go state until it fails or its sound closes, so a sandbox may have at most Limits.Sounds pending or open, of one source or many.
func TestAudioPackageSoundsLimit(t *testing.T) {
	headlessAudio(t)
	p := verified(t, `
import { mount, signal, Text } from "nsgo";
import { loadSound, type Sound } from "nsgo/audio";

const result = signal("loading");
(async () => {
  const got: string[] = [];
  const held: Sound[] = [];
  for (let i = 0; i < 4; i++) {
    try { held.push(await loadSound("asset:s0.wav")); got.push("ok"); } catch (e: any) { got.push(e.code + ":" + e.reason); }
  }
  held[0].close();
  held[0].close();
  try { await loadSound("asset:s1.wav"); got.push("ok"); } catch (e: any) { got.push(e.code); }
  try { await loadSound("asset:s2.wav"); got.push("ok"); } catch (e: any) { got.push(e.code); }
  result.set(got.join(","));
})();

mount(() => <Text testID="result">{result}</Text>);
`, pkg.Capabilities{Audio: true})
	limits := tsx.DefaultLimits
	limits.Sounds = 3
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return tsx.View{Package: p, Sandbox: tsx.Sandbox{Limits: &limits, WebUnisolated: true}}
	}, playsAudio, soundDurations)
	app.Find("text:ok,ok,ok,ERR_NSH_QUOTA_EXCEEDED:quota-exceeded,ok,ERR_NSH_QUOTA_EXCEEDED")
}

func TestAudioPackageWithoutTheCapability(t *testing.T) {
	src := `
import { mount, signal, Text } from "nsgo";
import { supported, loadSound } from "nsgo/audio";

const result = signal("loading");
loadSound("asset:s0.wav").then(() => result.set("loaded"), (e: any) => result.set(e.code + ":" + e.permission + ":" + e.reason));

mount(() => <Text testID="result">{() => "supported:" + supported() + " " + result()}</Text>);
`
	for _, c := range []struct {
		name     string
		declared pkg.Capabilities
		grant    *pkg.Capabilities
	}{
		{"undeclared", pkg.Capabilities{}, nil},
		{"denied_by_host_grant", pkg.Capabilities{Audio: true}, &pkg.Capabilities{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			headlessAudio(t)
			p := verified(t, src, c.declared)
			app := nsgotest.Start(t, func(*core.Scope) core.Node {
				return tsx.View{Package: p, Sandbox: tsx.Sandbox{Grant: c.grant, WebUnisolated: true}}
			}, playsAudio, soundDurations)
			app.Find("text:supported:false ERR_ACCESS_DENIED:audio:not-allowed")
			if len(app.Sounds) != 0 {
				t.Fatalf("%d sounds reached the platform", len(app.Sounds))
			}
		})
	}
}

func TestAudioTSXPlaybacks(t *testing.T) {
	headlessAudio(t)
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`
import { mount, signal, View, Text } from "nsgo";
import { supported, loadSound, type Sound, type Playback } from "nsgo/audio";

let click: Sound | undefined;
const loaded = signal("loading");
const played = signal<Playback | null>(null);
loadSound("asset:s0.wav").then((s) => { click = s; loaded.set("duration:" + s.duration); });

function App() {
  return <View>
    <Text testID="supported">{() => "supported:" + supported()}</Text>
    <Text testID="loaded">{loaded}</Text>
    <Text testID="ended">{() => { const p = played(); return p ? "ended:" + p.ended : "none"; }}</Text>
    <Text testID="play" onPress={() => {
      const p = click!.play({ volume: 0.5, rate: 1.5, pan: -1, loop: true });
      p.setVolume(0.25);
      p.setRate(2);
      played.set(p);
      click!.play({ volume: 0 });
    }}>Play</Text>
    <Text testID="stop" onPress={() => played()!.stop()}>Stop</Text>
    <Text testID="close" onPress={() => click!.close()}>Close</Text>
  </View>;
}
mount(App);
`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, tsx.App("App.tsx", code), playsAudio, soundDurations)
	app.Find("text:supported:true")
	app.Find("text:duration:10")
	app.Press("id:play")
	app.Find("text:ended:false")
	s := app.Sounds[0]
	if len(s.Plays) != 2 || s.Plays[0] != (core.SoundVoice{Volume: 0.5, Rate: 1.5, Pan: -1, Loop: true}) || s.Plays[1].Volume != 0 {
		t.Fatalf("plays %+v", s.Plays)
	}
	if pb := s.Playbacks[0]; pb.Volume != 0.25 || pb.Rate != 2 || pb.Ended || pb.Restricted {
		t.Fatalf("first playback %+v", pb)
	}
	app.Press("id:stop")
	app.Find("text:ended:true")
	if !s.Playbacks[0].Ended || s.Playbacks[1].Ended {
		t.Fatalf("stopping one playback: %+v, %+v", s.Playbacks[0], s.Playbacks[1])
	}
	app.Press("id:close")
	if !s.Unloaded {
		t.Fatal("a closed sound stays loaded")
	}
}

func TestAudioTSXWhereNothingPlays(t *testing.T) {
	headlessAudio(t)
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`
import { mount, signal, Text } from "nsgo";
import { supported, loadSound } from "nsgo/audio";

const result = signal("loading");
loadSound("asset:s0.wav").then(() => result.set("loaded"), (e: any) => result.set(e.code + ":" + e.reason));
// A source that is no string is refused at once, before the platform's answer.
let bad = "";
loadSound(42 as any).catch((e: any) => { bad = e.name; });

mount(() => <Text testID="result">{() => "supported:" + supported() + " " + result() + " " + bad}</Text>);
`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, tsx.App("App.tsx", code), soundDurations)
	app.Find("text:supported:false ERR_FEATURE_UNAVAILABLE_ON_PLATFORM:unavailable TypeError")
}

var playsAudio = nsgotest.Environment(core.Environment{Audio: true})

// headlessAudio skips a test that decodes through nsgotest's audio fake on a real target.
func headlessAudio(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the audio fake runs headless only")
	}
}
