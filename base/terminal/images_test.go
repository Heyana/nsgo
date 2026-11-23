package terminal

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"image"
	stdcolor "image/color"
	"image/png"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func kittyImage(id uint32, placement string, pixels ...byte) string {
	return fmt.Sprintf("\x1b_Ga=T,f=32,s=2,v=2,i=%d,C=1,q=2%s;%s\x1b\\", id, placement, base64.StdEncoding.EncodeToString(pixels))
}

var redImage = []byte{255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255}
var greenImage = []byte{0, 255, 0, 255, 0, 255, 0, 255, 0, 255, 0, 255, 0, 255, 0, 255}

func graphicsView(t *testing.T, opts Options, theme *Theme) (*Terminal, *Ref, *nsgotest.App) {
	t.Helper()
	term := protocolTerminal(t, opts)
	ref := &Ref{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return View{Terminal: term, Ref: ref, Theme: theme, DarkTheme: theme, TestID: "term", NoKeyBar: true, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(480, 240))
	term.Feed([]byte("\x1b[?25l"))
	app.Settle()
	return term, ref, app
}

func canvasOps(t *testing.T, app *nsgotest.App) []driver.DrawOp {
	t.Helper()
	var find func(*nsgotest.View) []driver.DrawOp
	find = func(v *nsgotest.View) []driver.DrawOp {
		if v.Drawing != nil {
			return v.Ops
		}
		for _, c := range v.Children {
			if ops := find(c); ops != nil {
				return ops
			}
		}
		return nil
	}
	ops := find(app.Find("id:term"))
	if ops == nil {
		t.Fatal("terminal canvas missing")
	}
	return ops
}

func imageOps(t *testing.T, app *nsgotest.App) []driver.DrawOp {
	var out []driver.DrawOp
	for _, op := range canvasOps(t, app) {
		if op.Kind == driver.DrawImage {
			out = append(out, op)
		}
	}
	return out
}

func TestTerminalGraphicsLifecycle(t *testing.T) {
	term, ref, app := graphicsView(t, Options{}, nil)
	term.Feed([]byte(kittyImage(7, ",c=4,r=2", redImage...)))
	app.Settle()
	ops := imageOps(t, app)
	if len(ops) != 1 {
		t.Fatalf("image ops %d, want 1", len(ops))
	}
	first := ops[0].Image.Pixels
	if got := first.RGBAAt(0, 0); got != (stdcolor.RGBA{255, 0, 0, 255}) {
		t.Fatalf("pixels %v", got)
	}
	if ops[0].W != 4*ref.v.cellW || ops[0].H != 2*ref.v.cellH {
		t.Fatalf("image size %v×%v", ops[0].W, ops[0].H)
	}
	// A synchronized update holds the exact image snapshot, then releases new pixels.
	term.Feed([]byte("\x1b[?2026h" + kittyImage(7, ",c=4,r=2", greenImage...)))
	app.Settle()
	if got := imageOps(t, app)[0].Image.Pixels; got != first {
		t.Fatal("image changed during synchronized update")
	}
	term.Feed([]byte("\x1b[?2026l"))
	app.Settle()
	second := imageOps(t, app)[0].Image.Pixels
	if second == first || second.RGBAAt(0, 0) != (stdcolor.RGBA{0, 255, 0, 255}) {
		t.Fatal("replacement did not show")
	}
	// Unchanged generations reuse the same decoded source.
	term.Feed([]byte("text"))
	app.Settle()
	if imageOps(t, app)[0].Image.Pixels != second {
		t.Fatal("recopied unchanged image")
	}
	term.Feed([]byte("\x1b[?1049h"))
	app.Settle()
	if len(imageOps(t, app)) != 0 {
		t.Fatal("main-screen image leaked onto alternate screen")
	}
	term.Feed([]byte("\x1b[?1049l"))
	app.Settle()
	if len(imageOps(t, app)) != 1 {
		t.Fatal("main-screen image missing after alternate screen")
	}
	term.Feed([]byte("\x1b_Ga=d,d=I,i=7,q=2\x1b\\"))
	app.Settle()
	if len(imageOps(t, app)) != 0 || len(ref.v.imageCache) != 0 {
		t.Fatal("deleted image retained")
	}
}

func TestTerminalGraphicsPNGAndLimit(t *testing.T) {
	for _, limit := range []int64{0, -1, 8} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			term, _, app := graphicsView(t, Options{ImageLimit: limit}, nil)
			img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
			img.SetNRGBA(0, 0, stdcolor.NRGBA{200, 100, 50, 128})
			var b bytes.Buffer
			png.Encode(&b, img)
			term.Feed([]byte("\x1b_Ga=T,f=100,i=8,c=2,r=2,q=2;" + base64.StdEncoding.EncodeToString(b.Bytes()) + "\x1b\\"))
			app.Settle()
			ops := imageOps(t, app)
			if limit != 0 {
				if len(ops) != 0 {
					t.Fatal("image bypassed limit")
				}
				return
			}
			if len(ops) != 1 {
				t.Fatalf("PNG image ops %d", len(ops))
			}
			if got := ops[0].Image.Pixels.RGBAAt(0, 0); got.R != 100 || got.G != 50 || got.B != 25 || got.A != 128 {
				t.Fatalf("premultiplied PNG %v", got)
			}
		})
	}
}

func TestTerminalGraphicsPlaceholders(t *testing.T) {
	term, ref, app := graphicsView(t, Options{}, nil)
	// 0x01000009 exercises the high-byte diacritic; the second cell inherits its row, next column and high byte. Underline color names placement 3.
	id := uint32(0x01000009)
	text := kittyImage(id, ",U=1,p=3,c=2,r=1", redImage...) + "\x1b[38;2;0;0;9;58;2;0;0;3m" + string([]rune{kittyPlaceholder, kittyDiacritics[0], kittyDiacritics[0], kittyDiacritics[1], kittyPlaceholder}) + "\x1b[0m"
	term.Feed([]byte(text))
	app.Settle()
	ops := imageOps(t, app)
	if len(ops) != 2 {
		t.Fatalf("virtual image ops %d, want 2", len(ops))
	}
	for _, op := range ops {
		if op.W > ref.v.cellW || op.H > ref.v.cellH {
			t.Fatalf("placeholder escaped its cell %+v", op)
		}
	}
	if ops[1].X <= ops[0].X {
		t.Fatal("inherited placeholder did not advance")
	}
	for _, op := range canvasOps(t, app) {
		if op.Kind == driver.DrawFillText && strings.ContainsRune(op.Text, kittyPlaceholder) {
			t.Fatal("placeholder was drawn as text")
		}
	}
	app.Screenshot("terminal-graphics-placeholder")
}

func TestTerminalSearchColorsAndUpdates(t *testing.T) {
	th := DarkTheme()
	th.Search = core.RGB(0x123456)
	th.SearchText = core.RGB(0xabcdef)
	th.SearchSelected = core.RGB(0x654321)
	th.SearchSelectedText = core.RGB(0xfedcba)
	var statuses []SearchStatus
	term, ref, app := graphicsView(t, Options{OnSearch: func(s SearchStatus) { statuses = append(statuses, s) }}, th)
	term.Feed([]byte("foo BAR foo\r\nfoo"))
	term.Search("FOO")
	app.WaitUntil(time.Second, func() bool { return term.SearchStatus().Total == 3 && !term.SearchStatus().Searching })
	app.Settle()
	if st := term.SearchStatus(); st.Selected != 1 {
		t.Fatalf("search status %+v", st)
	}
	// Configured foregrounds must be set for glyphs, and each opaque highlight must appear before its glyph run in the draw stream.
	fill := core.Color(0)
	highlights := 0
	texts := 0
	for _, op := range canvasOps(t, app) {
		if op.Kind == driver.DrawFillColor {
			fill = op.Color
		}
		if op.Kind == driver.DrawRect && (fill == th.Search || fill == th.SearchSelected) {
			highlights++
		}
		if op.Kind == driver.DrawFillText && strings.Contains(strings.ToLower(op.Text), "foo") {
			texts++
			if highlights == 0 || fill != th.SearchText && fill != th.SearchSelectedText {
				t.Fatalf("search glyph %q color %x before %d highlights", op.Text, fill, highlights)
			}
		}
	}
	if highlights != 3 || texts != 3 {
		t.Fatalf("highlights %d, glyph runs %d", highlights, texts)
	}
	term.SearchNext()
	app.Settle()
	if term.SearchStatus().Selected != 2 {
		t.Fatal("next match did not select older result")
	}
	term.SearchPrevious()
	app.Settle()
	if term.SearchStatus().Selected != 1 {
		t.Fatal("previous match did not return")
	}
	term.Feed([]byte(" foo"))
	app.WaitUntil(time.Second, func() bool { return term.SearchStatus().Total == 4 })
	term.EndSearch()
	app.Settle()
	if len(ref.v.matches) != 0 || term.SearchStatus() != (SearchStatus{}) {
		t.Fatal("ended search left matches")
	}
	if len(statuses) < 4 || statuses[len(statuses)-1] != (SearchStatus{}) {
		t.Fatalf("OnSearch statuses %+v", statuses)
	}
	app.Screenshot("terminal-search-ended")
}

func TestTerminalGraphicsCropStackAndScroll(t *testing.T) {
	term, ref, app := graphicsView(t, Options{}, DarkTheme())
	// The same source is placed in all three layers over a nondefault cell background. Cropping and physical offsets must survive the canvas snapshot.
	term.Feed([]byte("\x1b[44mXX\x1b[0m\x1b[H" +
		kittyImage(11, ",c=2,r=2,z=-1073741825", redImage...) +
		kittyImage(12, ",c=2,r=2,z=-1,X=1,Y=2,x=1,w=1", greenImage...) +
		kittyImage(13, ",c=2,r=2,z=1", redImage...)))
	app.Settle()
	ops := imageOps(t, app)
	// z=-1 is split between rows, so later row backgrounds cannot erase it.
	if len(ops) != 4 {
		t.Fatalf("image draws %d, want 4", len(ops))
	}
	if ops[0].Image.Pixels.RGBAAt(0, 0) != (stdcolor.RGBA{255, 0, 0, 255}) || ops[1].X1 != 1 || ops[1].X2 != 1 {
		t.Fatal("source crop changed")
	}
	if ops[1].X != ref.v.padX+1/ref.v.ratio || ops[1].Y != ref.v.padY+2/ref.v.ratio {
		t.Fatalf("physical offset %+v", ops[1])
	}
	bg, text, front, back := -1, -1, -1, -1
	fill := core.Color(0)
	for i, op := range canvasOps(t, app) {
		if op.Kind == driver.DrawFillColor {
			fill = op.Color
		}
		if op.Kind == driver.DrawImage && op.Image.Pixels == ops[0].Image.Pixels {
			back = i
		}
		if op.Kind == driver.DrawImage && op.Image.Pixels == ops[len(ops)-1].Image.Pixels {
			front = i
		}
		if op.Kind == driver.DrawRect && fill == core.RGB(0x2472c8) {
			bg = i
		}
		if op.Kind == driver.DrawFillText && strings.Contains(op.Text, "XX") {
			text = i
		}
	}
	if !(back >= 0 && back < bg && bg < text && text < front) {
		t.Fatalf("stack back=%d bg=%d text=%d front=%d", back, bg, text, front)
	}
	term.Feed([]byte(fmt.Sprintf("\x1b[%d;1H\n", ref.v.rows)))
	app.Settle()
	for _, g := range ref.v.graphics {
		if g.y >= ref.v.padY {
			t.Fatal("scroll did not move image origin above viewport")
		}
	}
	for _, op := range imageOps(t, app) {
		if op.Y < ref.v.padY || op.H > ref.v.cellH+0.01 {
			t.Fatalf("scrolled image not clipped %+v", op)
		}
	}
}

func TestGraphicsFractionalGridAndPixels(t *testing.T) {
	p := vt.Placement{XOffset: 2, YOffset: 3, Width: 40, Height: 20, SrcW: 40, SrcH: 20}
	x, y, w, h := placementGeometry(p, 7.8, 16.5, 1.25, 9, 20)
	if x != 1.6 || y != 2.4 || w != 32 || h != 16 {
		t.Fatalf("natural geometry %v %v %v %v", x, y, w, h)
	}
	p.ParamCols = 4
	_, _, w, h = placementGeometry(p, 7.8, 16.5, 1.25, 9, 20)
	if math.Abs(float64(w-29.6)) > 0.001 || math.Abs(float64(h-14.8)) > 0.001 {
		t.Fatalf("one-axis grid %v %v", w, h)
	}
	p.ParamRows = 2
	_, _, w, h = placementGeometry(p, 7.8, 16.5, 1.25, 9, 20)
	if math.Abs(float64(w-29.6)) > 0.001 || math.Abs(float64(h-30.6)) > 0.001 {
		t.Fatalf("two-axis grid %v %v", w, h)
	}
}

// TestTerminalGraphicsPixels checks real platform output in addition to draw records. Capture skips on headless runs, where there are no native pixels.
func TestTerminalGraphicsPixels(t *testing.T) {
	th := DarkTheme()
	th.Search = core.RGB(0xffe082)
	th.SearchSelected = core.RGB(0xf2a57e)
	term, ref, app := graphicsView(t, Options{}, th)
	term.Feed([]byte("foo and foo\x1b[3;3H" + kittyImage(1, ",c=6,r=3", redImage...)))
	term.Search("foo")
	app.WaitUntil(time.Second, func() bool { return term.SearchStatus().Total == 2 })
	app.Settle()
	app.Screenshot("terminal-search-and-images")
	pixels := app.Capture()
	sx, sy := float32(pixels.Bounds().Dx())/480, float32(pixels.Bounds().Dy())/240
	at := func(x, y float32) stdcolor.RGBA {
		return stdcolor.RGBAModel.Convert(pixels.At(int(x*sx), int(y*sy))).(stdcolor.RGBA)
	}
	v := ref.v
	if got := at(v.padX+4*v.cellW, v.padY+3*v.cellH); got.R < 240 || got.G > 20 || got.B > 20 {
		t.Fatalf("native image pixel %v", got)
	}
	// Sample low in each match cell, outside the text glyphs.
	for _, x := range []float32{0.25, 8.25} {
		got := at(v.padX+x*v.cellW, v.padY+0.9*v.cellH)
		if got.R < 200 || got.G < 100 || got.B > 160 {
			t.Fatalf("native search pixel at %.2f: %v", x, got)
		}
	}
}

func TestSearchRevealsScrollbackInSameFrame(t *testing.T) {
	term, ref, app := graphicsView(t, Options{}, nil)
	term.Feed([]byte("anchor\r\n" + strings.Repeat("later\r\n", 50)))
	term.Search("anchor")
	app.WaitUntil(time.Second, func() bool { return term.SearchStatus().Total == 1 && !term.SearchStatus().Searching })
	app.Settle()
	if len(ref.v.matches) != 1 {
		t.Fatal("search did not bring its result into view")
	}
	found := false
	for _, op := range canvasOps(t, app) {
		if op.Kind == driver.DrawFillText && strings.Contains(op.Text, "anchor") {
			found = true
		}
	}
	if !found {
		t.Fatal("search highlight used a newer viewport than its text snapshot")
	}
}
