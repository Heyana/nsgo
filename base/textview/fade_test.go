package textview_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// alphas gives each span of the text the alpha of its color.
func alphas(v *nsgotest.View) map[string]uint32 {
	out := map[string]uint32{}
	for _, sp := range v.Spans {
		out[v.Text[sp.Start:sp.End]] = uint32(sp.Style.Color & 0xff)
	}
	return out
}

func TestTextViewFadesInWhatArrives(t *testing.T) {
	var md *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		md = core.NewSignal(cx, "Hello")
		return textview.TextView{MarkdownFunc: md.Get, FadeIn: 300 * time.Millisecond,
			Styles: textview.Styles{textview.PartParagraph: {Color: core.RGB(0x112233)}}}
	})
	app.Advance(50 * time.Millisecond)
	if v := app.Find("text:Hello"); len(v.Spans) > 1 || v.Spans[0].Style.Color != core.RGB(0x112233) {
		t.Fatalf("the first text faded: %v", alphas(v))
	}
	md.Set("Hello world")
	app.Advance(20 * time.Millisecond)
	v := app.Find("text:Hello world")
	a := alphas(v)
	if a["Hello"] != 0xff {
		t.Errorf("what showed faded again: %v", a)
	}
	if len(v.Spans) < 3 {
		t.Errorf("the new characters fade as one: %v", a)
	}
	last := v.Spans[len(v.Spans)-1]
	if got := v.Text[last.Start:last.End]; last.Style.Color&0xff != 0 {
		t.Errorf("the last character %q shows at %d before its turn", got, last.Style.Color&0xff)
	}
	app.Advance(time.Second)
	if v := app.Find("text:Hello world"); len(v.Spans) != 1 || v.Spans[0].Style.Color != core.RGB(0x112233) {
		t.Errorf("the fade did not end: %v", alphas(v))
	}
}

func TestTextViewFadeKeepsItsPlaceAsBlocksGrow(t *testing.T) {
	var md *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		md = core.NewSignal(cx, "")
		return textview.TextView{MarkdownFunc: md.Get, FadeIn: 300 * time.Millisecond, Options: textview.Options{Streaming: true},
			Styles: textview.Styles{textview.PartParagraph: {Color: core.RGB(0x112233)}}}
	})
	app.Advance(50 * time.Millisecond)
	md.Set("- one")
	app.Advance(time.Second)
	// A second item makes the list again, and the first keeps showing.
	md.Set("- one\n- two")
	app.Advance(20 * time.Millisecond)
	if v := app.Find("text:one"); len(v.Spans) != 1 || v.Spans[0].Style.Color&0xff != 0xff {
		t.Errorf("the first item faded again: %v", alphas(v))
	}
	if v := app.Find("text:two"); len(v.Spans) < 2 {
		t.Errorf("the second item does not fade in: %v", alphas(v))
	}
}
