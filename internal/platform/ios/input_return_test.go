//go:build ios && nsgotest

package ios

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Exercise the software keyboard, rather than delivering EventKey directly:
// neither UIKit delegate may submit, blur or edit a Return a binding takes.
func TestSoftwareReturnReachesTheKeymapBeforeEditing(t *testing.T) {
	if onTV() {
		t.Skip("the iPhone and iPad software keyboard")
	}
	checkReturnBinding(t, false, core.SubmitBehaviorDefault)
}

func TestSoftwareReturnNewlineBinding(t *testing.T) {
	checkReturnBinding(t, true, core.SubmitBehaviorDefault)
}

func TestSoftwareReturnSubmitBinding(t *testing.T) {
	checkReturnBinding(t, true, core.SubmitBehaviorBlurAndSubmit)
}

func checkReturnBinding(t *testing.T, multiline bool, submit core.SubmitBehavior) {
	t.Helper()
	if onTV() {
		t.Skip("the iPhone and iPad software keyboard")
	}

	var bound, submitted, blurred, keys int
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways, Repeat: true}, func() { bound++ })
		// A newer automatic binding must leave Return to the older always binding.
		cx.OnKey("enter", func() { t.Error("automatic binding took an editing key") })
		return core.Input{
			TestID: "return", Multiline: multiline, SubmitBehavior: submit,
			Style:    core.Style{Width: core.Pt(240), Height: core.Pt(100)},
			OnSubmit: func(string) { submitted++ }, OnBlur: func() { blurred++ },
			OnKeyPress: func(string) { keys++ },
		}
	})
	app.Type("id:return", "draft")
	keys = 0
	for want := 1; want <= 2; want++ {
		app.Key("enter")
		if bound != want || submitted != 0 || blurred != 0 || keys != 0 {
			t.Fatalf("Return: binding=%d submit=%d blur=%d field keys=%d; want %d,0,0,0", bound, submitted, blurred, keys, want)
		}
		var text string
		var focused bool
		onMainSync(func() {
			for _, v := range runner.views {
				if v != nil && v.input != nil && v.props.TestID == "return" {
					text = v.input.text()
					focused = objc.Send[bool](v.input.field, sel("isFirstResponder"))
				}
			}
		})
		if text != "draft" || !focused {
			t.Fatalf("Return changed native editor: text=%q focused=%v", text, focused)
		}
	}
}
