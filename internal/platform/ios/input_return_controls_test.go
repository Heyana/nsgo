//go:build ios && nsgotest

package ios

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

type returnEvents struct{ bound, submits, blurs atomic.Int32 }

func startReturnControl(t *testing.T, multiline bool, when core.KeyWhen) (*nsgotest.App, *returnEvents) {
	t.Helper()
	if onTV() {
		t.Skip("the iPhone and iPad software keyboard")
	}
	e := new(returnEvents)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.OnKeyWith("enter", core.KeyOptions{When: when, Repeat: true}, func() { e.bound.Add(1) })
		return core.Input{TestID: "return", Multiline: multiline,
			Style:    core.Style{Width: core.Pt(240), Height: core.Pt(100)},
			OnSubmit: func(string) { e.submits.Add(1) }, OnBlur: func() { e.blurs.Add(1) }}
	})
	app.Type("id:return", "draft")
	return app, e
}

// Native state is read on UIKit's main thread, independently of the test model.
func returnInput() *input {
	for _, v := range runner.views {
		if v != nil && v.input != nil && v.props.TestID == "return" {
			return v.input
		}
	}
	return nil
}

func TestSoftwareReturnAutoKeepsSubmit(t *testing.T) {
	app, e := startReturnControl(t, false, core.KeyAuto)
	app.Key("enter")
	if e.bound.Load() != 0 || e.submits.Load() != 1 || e.blurs.Load() != 1 {
		t.Fatalf("bound=%d submits=%d blurs=%d; want 0,1,1", e.bound.Load(), e.submits.Load(), e.blurs.Load())
	}
}

func TestSoftwareReturnNotEditingKeepsNewline(t *testing.T) {
	app, e := startReturnControl(t, true, core.KeyNotEditing)
	app.Key("enter")
	var text string
	onMainSync(func() { text = returnInput().text() })
	if text != "draft\n" || e.bound.Load() != 0 || e.submits.Load() != 0 || e.blurs.Load() != 0 {
		t.Fatalf("text=%q bound=%d submits=%d blurs=%d; want newline,0,0,0", text, e.bound.Load(), e.submits.Load(), e.blurs.Load())
	}
}

func TestSoftwareReturnCompositionKeepsItsCandidates(t *testing.T) {
	app, e := startReturnControl(t, false, core.KeyAlways)
	app.Compose("id:return", "拼")
	var composing bool
	onMainSync(func() {
		in := returnInput()
		_, composing = in.markedText()
		views[in.field].native.Send(sel("textFieldShouldReturn:"), in.field)
	})
	app.Settle()
	if !composing || e.bound.Load() != 0 || e.submits.Load() != 0 || e.blurs.Load() != 0 {
		t.Fatalf("composing=%v bound=%d submits=%d blurs=%d", composing, e.bound.Load(), e.submits.Load(), e.blurs.Load())
	}
	app.Commit("id:return", "拼")
	app.Key("enter")
	if e.bound.Load() != 1 || e.submits.Load() != 0 || e.blurs.Load() != 0 {
		t.Fatal("Return after composition did not reach only the binding")
	}
}

func TestSoftwareReturnTextViewKeepsCompositionAndProgrammaticEdits(t *testing.T) {
	app, e := startReturnControl(t, true, core.KeyAlways)
	app.Compose("id:return", "拼")
	var composing, allowed bool
	onMainSync(func() {
		in := returnInput()
		_, composing = in.markedText()
		allowed = in.shouldChange(apple.NSRange{}, "\n")
	})
	app.Settle()
	if !composing || !allowed || e.bound.Load() != 0 {
		t.Fatal("composition newline was intercepted")
	}
	app.Commit("id:return", "拼")
	onMainSync(func() {
		in := returnInput()
		in.applying = true
		in.field.Send(sel("insertText:"), apple.NSString("\n"))
		in.applying = false
	})
	app.Settle()
	if e.bound.Load() != 0 {
		t.Fatal("programmatic newline fired a shortcut")
	}
}

func TestHardwareReturnKeepsModifiersAndMatchesOnce(t *testing.T) {
	app, e := startReturnControl(t, false, core.KeyAlways)
	app.KeyDown("enter")
	app.KeyUp("enter")
	if e.bound.Load() != 1 || e.submits.Load() != 0 || e.blurs.Load() != 0 {
		t.Fatalf("hardware Return: bound=%d submits=%d blurs=%d", e.bound.Load(), e.submits.Load(), e.blurs.Load())
	}
	app.Key("ctrl+enter")
	if e.bound.Load() != 1 {
		t.Fatalf("Control+Return matched plain Return: %d", e.bound.Load())
	}
}

func TestSoftwareReturnDeviceFallbackDoesNotLeaveAKeyHeld(t *testing.T) {
	app, e := startReturnControl(t, false, core.KeyAlways)
	var held bool

	for range 2 {
		if err := (&testSurface{s: runner}).keyDevice(runner.window, "enter"); err != nil {
			t.Fatal(err)
		}
		app.Settle()
	}
	app.Advance(650 * time.Millisecond)
	onMainSync(func() { held = runner.held != 0 || runner.heldStop != nil || len(runner.keysHeld) != 0 })
	if e.bound.Load() != 2 || e.submits.Load() != 0 || e.blurs.Load() != 0 || held {
		t.Fatalf("bound=%d submits=%d blurs=%d held=%v", e.bound.Load(), e.submits.Load(), e.blurs.Load(), held)
	}
	var focused bool
	onMainSync(func() { focused = objc.Send[bool](returnInput().field, sel("isFirstResponder")) })
	if !focused {
		t.Fatal("software Return blurred the native editor")
	}
}
