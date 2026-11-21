package main

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

//go:embed resources/fonts/Pacifico-Regular.ttf
var pacifico []byte

// TestARegisteredFontMeasures registers a font as the app runs, under nsgo test -target, and lays out a text in it with its own glyphs, not the system font's that a family it cannot find falls back to (#313).
func TestARegisteredFontMeasures(t *testing.T) {
	if nsgotest.Target() == "" {
		t.Skip("a headless test's platform registers no fonts")
	}
	const words = "Registered fonts"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		if err := cx.RegisterFont(pacifico); err != nil {
			t.Errorf("RegisterFont: %v", err)
		}
		return core.View{Style: core.Style{AlignItems: core.AlignStart}, Children: []core.Node{
			core.Text{TestID: "registered", Content: words, Style: core.Style{FontFamily: "Pacifico", FontSize: 20}},
			core.Text{TestID: "missing", Content: words, Style: core.Style{FontFamily: "NSGo Missing Family", FontSize: 20}},
		}}
	})
	size := func(id string) driver.Size { return app.Find("id:" + id).Frame.Size }
	// The web loads a face after RegisterFont returns.
	deadline := time.Now().Add(5 * time.Second)
	for app.Settle(); size("registered") == size("missing"); app.Settle() {
		if time.Now().After(deadline) {
			t.Fatalf("the registered font's text measures %v, as the fallback's does", size("registered"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSpanBackgroundBand compares a wrapped background before and after adding insets through the platform's text view.
func TestSpanBackgroundBand(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator", "android", "windows", "linux", "web", "tvos", "tvos_simulator":
	default:
		t.Skip("needs a target that paints span backgrounds")
	}

	var inset *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		inset = core.NewSignal(cx, false)
		return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(24), Background: core.RGB(0xffffff)}, Children: []core.Node{
			core.Text{TestID: "highlight", Selectable: core.SelectableOn, Style: core.Style{
				Width: core.Pt(190), FontSize: 32, LineHeight: 48, Color: core.RGB(0), SelectionColor: core.RGB(0x0000ff)},
				SpansFunc: func() []core.Span {
					style := core.Style{Background: core.RGB(0xff0000), CornerRadius: 5}
					if inset.Get() {
						style.BackgroundInsetTop, style.BackgroundInsetBottom = core.Pct(60), core.Pt(2)
					}
					return []core.Span{
						{Content: "a "},
						{Content: "marks marks marks marks", Style: style},
						{Content: " z"},
					}
				}},
		}}
	}, nsgotest.Size(300, 340))

	type band struct{ left, top, bottom, firstWidth, maxWidth int }
	paint := func() []band {
		img := app.Capture()
		bounds := img.Bounds()
		var bands []band
		start := true
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			left, right := bounds.Max.X, bounds.Min.X
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r > 0xd000 && g < 0x6000 && b < 0x6000 {
					left, right = min(left, x), max(right, x)
				}
			}
			if left >= right {
				start = true
				continue
			}
			if start {
				bands = append(bands, band{left: left, top: y, firstWidth: right - left})
				start = false
			}
			b := &bands[len(bands)-1]
			b.left = min(b.left, left)
			b.bottom = y
			b.maxWidth = max(b.maxWidth, right-left)
		}
		return bands
	}

	size := app.Find("id:highlight").Bounds()
	full := paint()
	app.Post(func() { inset.Set(true) })
	marked := paint()
	if got := app.Find("id:highlight").Bounds(); got != size {
		t.Errorf("insets moved or resized the text from %v to %v", size, got)
	}
	if len(full) < 2 || len(marked) != len(full) {
		t.Fatalf("full backgrounds = %+v, inset backgrounds = %+v, want a band on each wrapped line", full, marked)
	}
	if marked[0].left <= marked[1].left {
		t.Errorf("the first band starts at %d, want it after the first word (later rows start at %d)", marked[0].left, marked[1].left)
	}
	for i, b := range marked {
		f := full[i]
		if b.top <= (f.top+f.bottom)/2 || b.bottom >= f.bottom {
			t.Errorf("line %d: inset rows %d..%d, want them in the lower part of %d..%d with bottom space",
				i, b.top, b.bottom, f.top, f.bottom)
		}
		if b.firstWidth >= b.maxWidth {
			t.Errorf("line %d: the top of the band is as wide as its middle (%d pixels)", i, b.maxWidth)
		}
	}

	switch nsgotest.Target() {
	case "macos", "windows", "linux", "web":
		app.DragAt("id:highlight", core.Point{X: 4, Y: 24}, core.Point{X: 60}, core.Point{X: 110})
		app.Key("mod+a")
		for _, b := range paint() {
			if b.maxWidth > marked[0].maxWidth/2 {
				t.Errorf("a selected line still shows %d pixels of the band under the opaque selection", b.maxWidth)
			}
		}
	}
}

// TestSelectingInAScrollViewKeepsItsPlace selects inside a scroll view under nsgo test -target and leaves it where it was: only a drag past its edge scrolls it, never where the last drag ended before this one's first move.
func TestSelectingInAScrollViewKeepsItsPlace(t *testing.T) {
	if nsgotest.Target() == "" {
		t.Skip("a headless drag is a finger's, which scrolls")
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var texts []core.Node
		for i := range 40 {
			texts = append(texts, core.Text{TestID: fmt.Sprint("line-", i), Content: fmt.Sprint(i, " ", strings.Repeat("Selectable words ", 8))})
		}
		// A bar under the scroll view, as a chat's composer, puts the window's corner past its edge, and its spinner keeps the frame clock running.
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.Scroll{TestID: "scroll", Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)}, Children: []core.Node{
				core.View{Selectable: true, Style: core.Style{Padding: core.All(16), Gap: 12}, Children: texts},
			}},
			core.View{Style: core.Style{Height: core.Pt(40), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{ui.Spinner{Size: 16}}},
		}}
	}, nsgotest.Size(400, 500))
	app.Scroll("id:scroll", 600)
	offset := func() core.Points { return app.Find("id:scroll").Offset.Y }
	at := offset()
	line := ""
	for i := range 40 {
		if y := app.Find(fmt.Sprint("id:line-", i)).Bounds().Origin.Y; y > 150 && y < 250 {
			line = fmt.Sprint("id:line-", i)
			break
		}
	}
	for range 2 {
		app.Drag(line, core.Point{X: 10, Y: 10}, core.Point{X: 20, Y: -20}, core.Point{X: 30, Y: -40})
		if got := offset(); got != at {
			t.Fatalf("selecting scrolled the view from %v to %v", at, got)
		}
	}
}

// TestSelectingReportsTheText drags across a text under nsgo test -target and has the area hear the text selected as it grows within the text, with the box of the text, which stays the same (the range's box on the web), and nothing once Escape clears it.
func TestSelectingReportsTheText(t *testing.T) {
	if nsgotest.Target() == "" {
		t.Skip("a headless drag is a finger's, which scrolls")
	}
	var mu sync.Mutex
	var heard []core.TextSelection
	var line core.Ref
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(16)}, Children: []core.Node{
			core.View{TestID: "area", Selectable: true, OnTextSelection: func(s core.TextSelection) {
				mu.Lock()
				heard = append(heard, s)
				mu.Unlock()
			}, Children: []core.Node{
				core.Text{TestID: "line", Ref: &line, Content: "serendipity happens every day", Style: core.Style{FontSize: 20}},
			}},
		}}
	}, nsgotest.Size(400, 200))
	box := app.Find("id:line").Bounds().Size
	app.DragAt("id:line", core.Point{X: 2, Y: box.Height / 2}, core.Point{X: 60}, core.Point{X: 120}, core.Point{X: box.Width - 4})
	app.Settle()
	mu.Lock()
	got := slices.Clone(heard)
	mu.Unlock()
	var texts []string
	for _, s := range got {
		texts = append(texts, s.Text)
		if web := nsgotest.Target() == "web"; !web && s.Bounds.Size != box || web && (s.Bounds.Size.Width > box.Width || s.Bounds.Size.Height > box.Height) {
			t.Errorf("heard %+v, want the text's box %v", s, box)
		}
	}
	if len(texts) < 2 || texts[len(texts)-1] != "serendipity happens every day" {
		t.Fatalf("heard %q as the drag grew within one text, want it to end with the whole text", texts)
	}
	// The platforms that report each text's range give the whole line's, by its Ref.
	switch last := got[len(got)-1]; nsgotest.Target() {
	case "macos", "linux", "windows":
		if want := []core.SelectedText{{Ref: &line, Start: 0, End: len("serendipity happens every day")}}; !slices.Equal(last.Ranges, want) {
			t.Errorf("heard ranges %+v, want %+v", last.Ranges, want)
		}
	}
	app.Key("Escape")
	app.Settle()
	mu.Lock()
	last := heard[len(heard)-1]
	mu.Unlock()
	if last.Text != "" || last.Bounds.Size != (core.Size{}) || last.Ranges != nil {
		t.Errorf("after Escape heard %+v, want no selection", last)
	}
}
