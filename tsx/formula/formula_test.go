package formula_test

import (
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/compiler"
	_ "github.com/nsgo-dev/nsgo/tsx/formula"
	_ "github.com/nsgo-dev/nsgo/tsx/textview"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func start(t *testing.T, source string) *nsgotest.App {
	t.Helper()
	view := compiler.App(fstest.MapFS{"App.tsx": {Data: []byte(source)}}, "App.tsx")
	return nsgotest.Start(t, view, nsgotest.Environment(core.Environment{Locale: "en-US"}))
}

func waitForFormula(t *testing.T, app *nsgotest.App, selector string) *nsgotest.View {
	t.Helper()
	app.WaitUntil(10*time.Second, func() bool {
		views := app.QueryMounted(selector)
		return len(views) == 1 && views[0].Kind == nsgotest.KindCanvas && len(views[0].Ops) > 0
	})
	return app.FindMounted(selector)
}

func TestFormulaSourceFollowsProps(t *testing.T) {
	app := start(t, `import { mount, signal, Text, View } from "nsgo";
import { Formula } from "nsgo/base";
function App() {
  const source = signal("x");
  return <View class="p-4 gap-4">
    <View testID="change" class="w-32 h-10" onPress={() => source.set("x^2")}><Text>Change</Text></View>
    <Formula testID="formula" source={source()} accessibilityLabel="Equation" style={{ fontSize: 24 }} />
    <Formula testID="typst" source="frac(1, 2)" dialect="typst" accessibilityLabel="Typst fraction" />
  </View>;
}
mount(App);`)
	before := waitForFormula(t, app, "id:formula").Frame.Size
	if label := app.Find("id:formula").Props.Label; label != "Equation" {
		t.Fatalf("formula label = %q", label)
	}
	if size := waitForFormula(t, app, "id:typst").Frame.Size; size.Width <= 0 || size.Height <= 0 {
		t.Fatalf("Typst fraction has no size: %+v", size)
	}
	app.Press("id:change")
	after := waitForFormula(t, app, "id:formula").Frame.Size
	if after.Width <= before.Width || after.Height <= before.Height {
		t.Fatalf("the source did not grow into a superscript: before %+v, after %+v", before, after)
	}
	app.Screenshot("tsx-formula-source")
}

func TestTextViewRenderMathUsesInlineAndDisplay(t *testing.T) {
	const markdown = "Energy $E=mc^2$.\n\n$$\n\\frac{1}{2}\n$$"
	app := start(t, `import { mount } from "nsgo";
import { Formula, TextView } from "nsgo/base";
mount(() => <TextView markdown={`+strconv.Quote(markdown)+`}
 renderMath={(math) => <Formula testID={math.display ? "display" : "inline"} source={math.tex} display={math.display} />} />);`)
	inline := waitForFormula(t, app, "id:inline")
	display := waitForFormula(t, app, "id:display")
	if inline.Props.Label != "E equals m c squared" || display.Props.Label != "1 over 2" {
		t.Fatalf("math renderer missed its data: inline %q, display %q", inline.Props.Label, display.Props.Label)
	}
	text := inline.Parent
	for text != nil && text.Kind != nsgotest.KindText {
		text = text.Parent
	}
	if text == nil || len(text.Spans) < 2 || text.Spans[1].Copy != "$E=mc^2$" {
		t.Fatal("the inline formula lost its TeX copy value")
	}
	text = display.Parent
	for text != nil && text.Kind != nsgotest.KindText {
		text = text.Parent
	}
	if text == nil || len(text.Spans) != 1 || text.Spans[0].Copy != "$$\n\\frac{1}{2}\n$$" {
		t.Fatal("the display formula lost its TeX copy value")
	}
	app.Screenshot("tsx-textview-math")
}

func TestFormulaDeclarations(t *testing.T) {
	d := tsx.Declarations()["types/nsgo/base.d.ts"]
	for _, want := range []string{"export declare const Formula:", "export type FormulaDialect =", `"tex"`, `"typst"`, "dialect?: FormulaDialect", "source?: string", "display?: boolean"} {
		if !strings.Contains(d, want) {
			t.Errorf("formula declarations lack %q", want)
		}
	}
	if strings.Contains(d, "renderer?:") || strings.Contains(d, "FormulaRenderer") {
		t.Error("formula renderer configuration leaked into TSX props")
	}
}

func TestSandboxCanTypesetAFormula(t *testing.T) {
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`import { mount } from "nsgo";
import { Formula } from "nsgo/base";
mount(() => <Formula testID="sandbox-formula" source="x^2" />);`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, tsx.Sandboxed("formula", code, tsx.Sandbox{WebUnisolated: true}),
		nsgotest.Environment(core.Environment{Locale: "en-US"}))
	if label := waitForFormula(t, app, "id:sandbox-formula").Props.Label; label != "x squared" {
		t.Fatalf("sandbox formula label = %q", label)
	}
}
