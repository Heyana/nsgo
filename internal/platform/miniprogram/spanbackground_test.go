package miniprogram

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func sentBackgrounds(t *testing.T, at *driver.AttributedText, width driver.Points) [][]any {
	t.Helper()
	m, h := mounted()
	l := NewTextEngine(&approx{}).Prepare(at).Layout(driver.Size{Width: width, Height: 100})
	m.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemText, ID: 3,
		Text: &driver.TextContent{Text: at.Text, Style: at.Style, Spans: at.Spans, Layout: l}}}})
	ops := h.ops("t")
	if len(ops) != 1 {
		t.Fatalf("text operations = %v", ops)
	}
	var op []json.RawMessage
	if err := json.Unmarshal([]byte(ops[0]), &op); err != nil {
		t.Fatal(err)
	}
	var content struct{ B [][]any }
	if err := json.Unmarshal(op[1], &content); err != nil {
		t.Fatal(err)
	}
	return content.B
}

func TestSpanBackgroundsWrapWithInsetsAndRoundedCorners(t *testing.T) {
	at := plain("aa bb cc")
	at.Style.Align = driver.TextAlignCenter
	at.Spans = []driver.TextSpan{{Start: 0, End: len(at.Text), Style: at.Style,
		Background: driver.TextBackground{Color: driver.RGB(0xff0000), Radius: 20, Top: driver.Pct(50)}}}
	got := sentBackgrounds(t, at, 25)
	want := [][]any{
		{float64(0), float64(6), float64(25), float64(5), "rgba(255,0,0,1)", float64(2.5)},
		{float64(7.5), float64(18), float64(10), float64(5), "rgba(255,0,0,1)", float64(2.5)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("background boxes = %v, want %v", got, want)
	}
}

func TestSpanBackgroundsKeepAdjacentSpansSeparate(t *testing.T) {
	at := plain("aabb")
	bg := driver.TextBackground{Color: driver.RGB(0x00ff00), Radius: 2}
	at.Spans = []driver.TextSpan{
		{Start: 0, End: 2, Style: at.Style, Background: bg},
		{Start: 2, End: 4, Style: at.Style, Background: bg},
	}
	got := sentBackgrounds(t, at, 20)
	if len(got) != 2 || got[0][2] != float64(10) || got[1][0] != float64(10) || got[1][2] != float64(10) {
		t.Fatalf("adjacent spans' boxes = %v, want two separate 10-point boxes", got)
	}
}

func TestSpanBackgroundsOmitHiddenAndEmptyBands(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		at := plain("aa")
		bg := driver.TextBackground{Color: driver.RGB(0xff0000)}
		if !hidden {
			bg.Top, bg.Bottom = driver.Pct(60), driver.Pt(5)
		}
		at.Spans = []driver.TextSpan{{Start: 0, End: 2, Style: at.Style, Background: bg, Hidden: hidden}}
		if got := sentBackgrounds(t, at, 20); len(got) != 0 {
			t.Fatalf("hidden=%v background = %v, want none", hidden, got)
		}
	}
}

type kernedMeasurer struct{}

func (kernedMeasurer) Width(_ string, text string) float64 {
	return float64(utf8.RuneCountInString(text)*5 - strings.Count(text, "AV")*2)
}

func TestSpanBackgroundsPreserveKerningAndLineBreaks(t *testing.T) {
	e := NewTextEngine(kernedMeasurer{})
	at := plain("AV AV")
	before := e.Prepare(at).Measure(driver.Auto)
	wrapped := e.Prepare(at).Measure(12)
	at.Spans = []driver.TextSpan{{Start: 0, End: 1, Style: at.Style,
		Background: driver.TextBackground{Color: driver.RGB(0xffcc00)}}}
	if after := e.Prepare(at).Measure(driver.Auto); after != before {
		t.Fatalf("background changed size from %v to %v", before, after)
	}
	if after := e.Prepare(at).Measure(12); after != wrapped {
		t.Fatalf("background changed wrapped size from %v to %v", wrapped, after)
	}
}

func TestSpanBackgroundsStopAtTheVisibleEllipsis(t *testing.T) {
	at := plain("abcdefgh")
	at.MaxLines = 1
	at.Spans = []driver.TextSpan{{Start: 0, End: len(at.Text), Style: at.Style,
		Background: driver.TextBackground{Color: driver.RGB(0xffcc00)}}}
	got := sentBackgrounds(t, at, 15)
	if len(got) != 1 || got[0][2] != float64(15) {
		t.Fatalf("ellipsized background = %v, want one 15-point box", got)
	}
}
