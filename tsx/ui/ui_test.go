package ui

import (
	"bytes"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx/compiler"
)

func TestReadIcon(t *testing.T) {
	Icons(map[string]func() *core.SVG{"plus": lucide.Plus})
	if readIcon("plus") != lucide.Plus() {
		t.Error("a name reads as the icon Icons gave")
	}
	if readIcon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><rect width="4" height="4"/></svg>`) == nil {
		t.Error("an SVG document reads as an icon")
	}
	if readIcon("unknown") != nil || readIcon(nil) != nil {
		t.Error("what names no icon reads as none")
	}
}

// A malformed SVG string in an icon prop logs one diagnostic naming the
// parse error, and only once for the same string (#585).
func TestReadIconMalformedSVGLogsOnce(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	bad := "<svg"
	if readIcon(bad) != nil {
		t.Error("a malformed SVG reads as an icon")
	}
	if readIcon(bad) != nil {
		t.Error("a malformed SVG reads as an icon on a second read")
	}
	got := buf.String()
	if strings.Count(got, bad) != 1 || !strings.Contains(got, "not a valid SVG") || !strings.Contains(got, "unexpected EOF") {
		t.Errorf("malformed icon logged %q, want one line naming the string and the parse error", got)
	}
	// A valid SVG and a registered name stay silent and keep working.
	buf.Reset()
	if readIcon(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><rect width="4" height="4"/></svg>`) == nil {
		t.Error("a valid SVG no longer reads as an icon")
	}
	if readIcon("plus") != lucide.Plus() {
		t.Error("a registered name no longer reads as its icon")
	}
	if buf.Len() != 0 {
		t.Errorf("valid icons logged %q, want nothing", buf.String())
	}
}

// The reported-malformed set stays bounded: after more than two
// generations of distinct strings, the earliest has aged out while a
// recent one is still remembered.
func TestBadIconSetStaysBounded(t *testing.T) {
	var set badIconSet
	for i := 0; i <= 2*badIconSetSize; i++ {
		set.add("<svg bad-" + strconv.Itoa(i))
	}
	if got := len(set.now) + len(set.old); got > 2*badIconSetSize {
		t.Errorf("badIconSet holds %d entries, want at most %d", got, 2*badIconSetSize)
	}
	if set.seen("<svg bad-0") {
		t.Error("the earliest string is still remembered after aging out")
	}
	if !set.seen("<svg bad-" + strconv.Itoa(2*badIconSetSize)) {
		t.Error("the latest string is not remembered")
	}
}

// A malformed string that aged out of the bounded set logs again on its
// next read, as the review of #594 allows.
func TestReadIconMalformedSVGAgedOutLogsAgain(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	first := "<svg bad-0"
	for i := 0; i <= 2*badIconSetSize; i++ {
		if readIcon("<svg bad-"+strconv.Itoa(i)) != nil {
			t.Fatalf("malformed string %d reads as an icon", i)
		}
	}
	if readIcon(first) != nil {
		t.Error("an aged-out malformed SVG reads as an icon")
	}
	if got := strings.Count(buf.String(), first); got != 2 {
		t.Errorf("aged-out string logged %d times, want 2 (once, then again after aging out)", got)
	}
}

// TestTouchControlSizingFromTSX sizes a Button through its style, filling the column or at a percentage width and a height of the app's, with the face filling it, and an OtpInput's cells through size, on phones of both platforms.
func TestTouchControlSizingFromTSX(t *testing.T) {
	files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, View } from "nsgo";
import { Button, OtpInput } from "nsgo/ui";
mount(() => <View style={{width: 320, alignItems: "flex-start"}}>
    <Button testID="action" size="large" style={{width: "100%"}}>Continue</Button>
    <Button testID="custom" style={{width: "50%", height: 50}}>Custom</Button>
    <OtpInput testID="code" length={4} size={50} cellHeight={52} cornerRadius={10} label="Code" />
</View>);`)}}
	for _, os := range []core.OS{core.OSIOS, core.OSAndroid} {
		t.Run(string(os), func(t *testing.T) {
			app := nsgotest.Start(t, compiler.App(files, "App.tsx"), nsgotest.Size(390, 844),
				nsgotest.Environment(core.Environment{OS: os, Idiom: core.IdiomPhone}))
			for _, c := range []struct {
				id            string
				width, height core.Points
			}{{"action", 320, 48}, {"custom", 160, 50}} {
				button := app.Find(c.id)
				if got := button.Frame.Size; got.Width != c.width || got.Height != c.height || button.Children[0].Frame.Size != got {
					t.Fatalf("%s button and face = %v, %v; want %v x %v", c.id, got, button.Children[0].Frame.Size, c.width, c.height)
				}
			}
			cell := app.Find("code").Children[0].Children[0]
			if got := cell.Frame.Size; got.Width != 50 || got.Height != 52 || cell.Style.CornerRadius != 10 {
				t.Fatalf("OTP cell = %v with radius %v, want 50 x 52 with radius 10", got, cell.Style.CornerRadius)
			}
		})
	}
}

// TestTabsUnderlineIndicatorFromTSX sets an underline's color and content fit by name.
func TestTabsUnderlineIndicatorFromTSX(t *testing.T) {
	files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal, View } from "nsgo";
import { Tabs } from "nsgo/ui";
function App() {
    const value = signal("a");
    const color = signal("#123456");
    return <View style={{width: 400}}>
        <Tabs testID="tabs" variant="underline" equal indicatorFit="content" indicatorColor={color()} value={value()} onChange={(v) => { value.set(v); color.set("#abcdef"); }}
            options={[{value: "a", label: "One", testID: "one"}, {value: "b", label: "A longer tab", testID: "two"}]} />
    </View>;
}
mount(App);`)}}
	app := nsgotest.Start(t, compiler.App(files, "App.tsx"), nsgotest.Size(480, 320))
	indicator := app.Find("tabs").Children[0]
	if got := indicator.Style.Background; got != core.RGB(0x123456) {
		t.Fatalf("TSX underline color = %x, want 123456", got)
	}
	if content, tab := app.Find("one").Children[0].Frame, app.Find("one").Frame; indicator.Frame.Size.Width != content.Size.Width || content.Size.Width >= tab.Size.Width-12 {
		t.Fatalf("TSX underline is %v wide, want the content's %v inside a %v tab", indicator.Frame.Size.Width, content.Size.Width, tab.Size.Width)
	}
	app.Press("two")
	app.Settle()
	if got := app.Find("tabs").Children[0].Style.Background; got != core.RGB(0xabcdef) {
		t.Fatalf("TSX underline color = %x after a change, want abcdef", got)
	}
}

func TestToastSurfaceStylesFromTSX(t *testing.T) {
	files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal, View } from "nsgo";
import { Button, Toasts } from "nsgo/ui";
function App() {
    const toasts = signal(null);
    const fill = signal("#334466"), radius = signal(20);
    return <View>
        <Button testID="show" onPress={() => {
            toasts()?.show({id: "default", title: "Default", duration: -1});
            toasts()?.show({id: "custom", title: "Custom", duration: -1,
                style: {backgroundColor: "#121522"}, styleFunc: () => ({borderRadius: radius()})});
        }}>Show</Button>
        <Button testID="update" onPress={() => { fill.set("#416631"); radius.set(30); }}>Update</Button>
        <Toasts placement="bottom" ref={toasts.set} style={{backgroundColor: fill(), borderWidth: 3, borderRadius: 8}} />
    </View>;
}
mount(App);`)}}
	app := nsgotest.Start(t, compiler.App(files, "App.tsx"))
	app.Press("id:show")
	app.Advance(time.Second)
	if s := app.Find("id:default").Style; s.Background != core.RGB(0x334466) || s.BorderWidth != 3 || s.CornerRadius != 8 {
		t.Fatalf("stack style did not reach the toast: background %x, border %v, radius %v", s.Background, s.BorderWidth, s.CornerRadius)
	}
	if s := app.Find("id:custom").Style; s.Background != core.RGB(0x121522) || s.BorderWidth != 3 || s.CornerRadius != 20 {
		t.Fatalf("the toast's style did not override the stack: background %x, border %v, radius %v", s.Background, s.BorderWidth, s.CornerRadius)
	}
	app.Press("id:update")
	if got := app.Find("id:default").Style.Background; got != core.RGB(0x416631) {
		t.Fatalf("stack style did not update: %x", got)
	}
	if got := app.Find("id:custom").Style.CornerRadius; got != 30 {
		t.Fatalf("per-toast style did not update: %v", got)
	}
}

func TestTabShellSVGFromTSX(t *testing.T) {
	files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal, Text } from "nsgo";
import { TabShell } from "nsgo/ui";
function App() {
    const selected = signal(0);
    return <TabShell selected={selected()} onChange={(e) => selected.set(e.index)} tabs={[
        { text: "Home", icon: '<svg viewBox="0 0 24 24"><rect width="24" height="24"/></svg>', badge: () => selected() === 0 ? { text: "3" } : { dot: true }, content: () => <Text>Home page</Text> },
        { text: "Search", icon: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/></svg>', content: () => <Text>Search page</Text> }
    ]} />;
}
mount(App);`)}}
	app := nsgotest.Start(t, compiler.App(files, "App.tsx"), nsgotest.Environment(core.Environment{OS: core.OSAndroid}))
	bar := app.FindMounted("id:tab-bar")
	if got := bar.TabBadge(0); got != (core.TabBadge{Text: "3"}) {
		t.Fatalf("TSX shell badge = %v, want 3", got)
	}
	items := bar.NativeProps["items"].([]any)
	for _, item := range items {
		if d, ok := item.(map[string]any)["drawing"].(*driver.Drawing); !ok || len(d.Ops) == 0 {
			t.Fatal("TSX shell icon did not reach Android's native bar")
		}
	}
	app.Emit("id:tab-bar", "change", float64(1))
	app.Find("text:Search page")
	if got := app.FindMounted("id:tab-bar").TabBadge(0); got != (core.TabBadge{Dot: true}) {
		t.Fatalf("updated TSX shell badge = %v, want dot", got)
	}
}

func TestAlertDialogActionRolesFromTSX(t *testing.T) {
	for _, c := range []struct{ key, want string }{{"Escape", "cancel"}, {"Return", "delete"}} {
		t.Run(c.key, func(t *testing.T) {
			files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal, Text, View } from "nsgo";
import { AlertDialog } from "nsgo/ui";
function App() {
    const open = signal(true), result = signal("");
    return <View>
        <AlertDialog open={open()} title="Delete?" onOpenChange={(e) => open.set(e.open)} actions={[
            { text: "Cancel", role: "cancel", onPress: () => result.set("cancel") },
            { text: "Delete", role: "destructive", default: true, testID: "default", onPress: () => result.set("delete") },
        ]} />
        <Text>{() => "result " + result()}</Text>
    </View>;
}
mount(App);`)}}
			// On a desktop, as nsgotest presses the focused default there on Return.
			app := nsgotest.Start(t, compiler.App(files, "App.tsx"), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
			app.Find("role:alertdialog")
			if !app.Find("id:default").Focused {
				t.Fatal("TSX default action was not focused")
			}
			app.Key(c.key)
			app.Advance(time.Second)
			app.WaitFor("text:result "+c.want, time.Second)
		})
	}
}

func TestSliderBoundsFromTSXStayReactive(t *testing.T) {
	files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal, View } from "nsgo";
import { Button, Slider } from "nsgo/ui";
function App() {
    const minimum = signal(0), maximum = signal(0), step = signal(0), value = signal(60);
    return <View style={{width: 240}}>
        <Button testID="load" onPress={() => setTimeout(() => {
            minimum.set(10); maximum.set(300); step.set(5);
        }, 1000)}>Load duration</Button>
        <Slider testID="position" label="Position" min={minimum()} max={maximum()} step={step()}
            value={value()} onChange={(event) => value.set(event.value)} />
    </View>;
}
mount(App);`)}}
	app := nsgotest.Start(t, compiler.App(files, "App.tsx"), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	thumb := app.Find("position-thumb").ID
	if v := app.Find("position-thumb").Props.Value; v.Min != 0 || v.Max != 0 || v.Now != 0 {
		t.Fatalf("unknown duration: %+v", v)
	}
	app.Press("load")
	app.Focus("position-thumb")
	app.Advance(time.Second)
	if v := app.Find("position-thumb"); v.ID != thumb || !v.Focused || v.Props.Value.Min != 10 || v.Props.Value.Max != 300 || v.Props.Value.Now != 60 {
		t.Fatalf("TSX bounds lost the thumb, focus or value: %+v, focused %v", v.Props.Value, v.Focused)
	}
	app.Key("right")
	if v := app.Find("position-thumb").Props.Value; v.Now != 65 {
		t.Fatalf("TSX step did not change: %+v", v)
	}
}

func TestSliderUndefinedBoundsUseDefaults(t *testing.T) {
	files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal, View } from "nsgo";
import { Button, Slider } from "nsgo/ui";
function Forwarded(props) { return <Slider testID="position" value={50} min={undefined} max={props.max} />; }
function App() {
    const maximum = signal(undefined);
    return <View style={{width:240}}>
        <Slider testID="omitted" value={50}/>
        <Slider testID="undefined" value={50} max={undefined}/>
        <Forwarded max={maximum()}/>
        <Button testID="load" onPress={() => setTimeout(() => maximum.set(200), 1000)}>Load</Button>
        <Button testID="zero" onPress={() => setTimeout(() => maximum.set(0), 1000)}>Zero</Button>
        <Button testID="reset" onPress={() => setTimeout(() => maximum.set(undefined), 1000)}>Reset</Button>
    </View>;
}
mount(App);`)}}
	app := nsgotest.Start(t, compiler.App(files, "App.tsx"), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	for _, name := range []string{"omitted-thumb", "undefined-thumb", "position-thumb"} {
		v := app.Find(name).Props.Value
		if v.Min != 0 || v.Max != 100 || v.Now != 50 {
			t.Fatalf("%s: %+v, want default 0..100 at 50", name, v)
		}
	}
	id := app.Find("position-thumb").ID
	for _, step := range []struct {
		button string
		max    float32
	}{{"load", 200}, {"zero", 0}, {"reset", 100}} {
		app.Press(step.button)
		app.Focus("position-thumb")
		app.Advance(time.Second)
		v := app.Find("position-thumb")
		if v.ID != id || !v.Focused || v.Props.Value.Max != step.max {
			t.Fatalf("%s changed identity, focus or bounds: ID %v (was %v), focused %v, %+v", step.button, v.ID, id, v.Focused, v.Props.Value)
		}
	}
}

// A TSX Select's options take a group, as a Go one's do.
func TestSelectOptionGroupsFromTSX(t *testing.T) {
	files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal } from "nsgo";
import { Select } from "nsgo/ui";
function App() {
    const value = signal("");
    return <Select testID="service" label="Service" value={value()} onChange={(v) => value.set(v)}
        options={[{ value: "one", label: "One", group: "Built in" }, { value: "two", label: "Two", group: "Plugins" }]} />;
}
mount(App);`)}}
	app := nsgotest.Start(t, compiler.App(files, "App.tsx"), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Press("service")
	if !app.Exists("role:header Built in") || !app.Exists("role:header Plugins") {
		t.Fatal("the options' groups show no headers")
	}
}

func TestSelectLabelFollowsTSXSignal(t *testing.T) {
	files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal, View } from "nsgo";
import { Button, Select } from "nsgo/ui";
function App() {
    const label = signal("Language");
    return <View>
        <Select testID="language" accessibilityLabel={label()} />
        <Button testID="translate" onPress={() => setTimeout(() => label.set("Langue"), 1000)}>Translate</Button>
    </View>;
}
mount(App);`)}}
	app := nsgotest.Start(t, compiler.App(files, "App.tsx"), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Press("translate")
	app.Focus("language")
	before := app.Find("language")
	if before.Props.Label != "Language" {
		t.Fatalf("initial label = %q, want Language", before.Props.Label)
	}
	app.Advance(time.Second)
	after := app.Find("language")
	if after.Props.Label != "Langue" || after.ID != before.ID || !after.Focused {
		t.Fatalf("label = %q, same view = %t, focused = %t", after.Props.Label, after.ID == before.ID, after.Focused)
	}
}
