package diagram_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/compiler"
	_ "github.com/nsgo-dev/nsgo/tsx/diagram"
	_ "github.com/nsgo-dev/nsgo/tsx/textview"
)

func start(t *testing.T, source string) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, compiler.App(fstest.MapFS{"App.tsx": {Data: []byte(source)}}, "App.tsx"))
}

func drawnText(v *nsgotest.View) []string {
	var texts []string
	for _, op := range v.Ops {
		if op.Kind == driver.DrawFillText {
			texts = append(texts, op.Text)
		}
	}
	return texts
}

func TestDiagramSourceFollowsProps(t *testing.T) {
	app := start(t, `import { mount, signal, View } from "nsgo";
import { Diagram } from "nsgo/base";
function App() {
  const source = signal("flowchart LR\n  A[Start] --> B[Done]");
  return <View>
    <View testID="change" class="w-24 h-10" onPress={() => source.set("flowchart LR\n  A[Changed] --> B[Done]")} />
    <View testID="fail" class="w-24 h-10" onPress={() => source.set("not a diagram")} />
    <Diagram testID="diagram" source={source()} accessibilityLabel="Process" theme={{ surface: "#663399" }} errorClass="font-mono" />
  </View>;
}
mount(App);`)
	check := func(want string) {
		t.Helper()
		v := app.Find("id:diagram")
		if v.Kind != nsgotest.KindCanvas || v.Frame.Size.Width <= 0 || v.Frame.Size.Height <= 0 || !slices.Contains(drawnText(v), want) {
			t.Fatalf("diagram is %v at %+v, drawing %q; want %q", v.Kind, v.Frame.Size, drawnText(v), want)
		}
		if v.Props.Label != "Process" {
			t.Errorf("diagram label = %q", v.Props.Label)
		}
		if !slices.ContainsFunc(v.Ops, func(op driver.DrawOp) bool { return op.Kind == driver.DrawFillColor && op.Color == core.RGB(0x663399) }) {
			t.Error("diagram does not use the partial theme's surface color")
		}
	}
	check("Start")
	app.Press("id:change")
	check("Changed")
	app.Press("id:fail")
	if v := app.Find("id:diagram"); v.Kind != nsgotest.KindText || v.Text != "not a diagram" {
		t.Fatalf("invalid source shows %+v", v)
	}
	app.Press("id:change")
	check("Changed")
}

func TestTextViewRenderDiagramUsesTheCodeBlock(t *testing.T) {
	const markdown = "Before.\n\n```mermaid\nflowchart LR\n  A[Start] --> B[Done]\n```\n\n```go\nplain code\n```"
	app := start(t, `import { mount } from "nsgo";
import { Diagram, TextView } from "nsgo/base";
mount(() => <TextView markdown={`+strconv.Quote(markdown)+`}
  renderDiagram={(code) => <Diagram testID="attachment" source={code.code} />} />);`)
	v := app.Find("id:attachment")
	if v.Kind != nsgotest.KindCanvas || !slices.Contains(drawnText(v), "Start") || !slices.Contains(drawnText(v), "Done") {
		t.Fatalf("mermaid attachment shows %v with %q", v.Kind, drawnText(v))
	}
	if !app.Exists("text:Before.") || !app.Exists("text:plain code") {
		t.Fatalf("ordinary content changed: %q", app.Texts())
	}
}

func TestDiagramDeclarations(t *testing.T) {
	d := tsx.Declarations()["types/nsgo/base.d.ts"]
	for _, want := range []string{"export declare const Diagram:", "export interface DiagramTheme", "theme?: DiagramTheme", "radius?: number", "strokeWidth?: number"} {
		if !strings.Contains(d, want) {
			t.Errorf("diagram declarations lack %q", want)
		}
	}
}

func TestSandboxCanDrawADiagram(t *testing.T) {
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`import { mount } from "nsgo";
import { Diagram } from "nsgo/base";
mount(() => <Diagram testID="sandbox-diagram" source={"flowchart LR\n  A[Safe] --> B[Drawing]"} />);`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, tsx.Sandboxed("diagram", code, tsx.Sandbox{WebUnisolated: true}))
	if v := app.Find("id:sandbox-diagram"); v.Kind != nsgotest.KindCanvas || !slices.Contains(drawnText(v), "Safe") {
		t.Fatalf("sandbox diagram shows %v with %q", v.Kind, drawnText(v))
	}
}
