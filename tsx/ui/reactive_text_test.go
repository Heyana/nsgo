package ui

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx/compiler"
)

func TestRemainingTextPropsFollowTSXSignalsWithoutRemounting(t *testing.T) {
	files := fstest.MapFS{"App.tsx": {Data: []byte(`import { mount, signal, View } from "nsgo";
import { Button, NumberInput, OtpInput, Spinner, Select } from "nsgo/ui";
function App() {
 const label = signal("Words");
 return <View>
  <NumberInput accessibilityLabel={label() + " number"} />
  <OtpInput length={4} accessibilityLabel={label() + " code"} />
  <Spinner testID="spinner" accessibilityLabel={label()} />
  <Select testID="select" searchable={true} searchPlaceholder={label() + " search"} empty={label() + " empty"} />
  <Button testID="translate" onPress={() => setTimeout(() => label.set("Mots"), 1000)}>Translate</Button>
 </View>;
}
mount(App);`)}}
	app := nsgotest.Start(t, compiler.App(files, "App.tsx"), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Press("translate")
	app.Focus("label:Words number")
	number := app.Find("label:Words number")
	code := app.Find("label:Words code")
	spinner := app.Find("spinner")
	app.Advance(time.Second)
	if after := app.Find("label:Mots number"); after.ID != number.ID || !after.Focused {
		t.Fatal("NumberInput signal remounted or lost the focused field")
	}
	if app.Find("label:Mots code").ID != code.ID || app.Find("spinner").ID != spinner.ID || app.Find("spinner").Props.Label != "Mots" {
		t.Fatal("OTP or spinner signal did not update its original view")
	}
	app.Press("select")
	app.Type("label:Mots search", "zzz")
	if field := app.Find("label:Mots search"); field.Input.Placeholder != "Mots search" || !app.Exists("text:Mots empty") {
		t.Fatal("select did not bind search and empty expressions")
	}
}
