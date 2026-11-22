package terminal

import (
	"fmt"
	imagecolor "image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMinimumContrast(t *testing.T) {
	if got := contrastRatio(core.RGB(0x777777), core.RGB(0xffffff)); math.Abs(got-4.4780894536) > 1e-8 {
		t.Fatalf("WCAG sRGB reference ratio=%g", got)
	}
	for _, tc := range []struct {
		fg, bg uint32
		ratio  float64
	}{
		{0x303030, 0x202020, 4.5}, {0xdddddd, 0xffffff, 7},
		{0x804060, 0x604050, 3}, {0x000000, 0x000000, 1.1},
		{0xffffff, 0xffffff, 4.5}, {0xffffff, 0, 21},
	} {
		t.Run(fmt.Sprintf("%06x/%06x/%g", tc.fg, tc.bg, tc.ratio), func(t *testing.T) {
			fg, bg := core.RGB(tc.fg), core.RGB(tc.bg)
			adjusted := minimumContrast(fg, bg, tc.ratio)
			if got := contrastRatio(adjusted, bg); got+1e-6 < tc.ratio {
				t.Fatalf("contrast %.4f, want at least %g", got, tc.ratio)
			}
			if tc.ratio <= 1 || contrastRatio(fg, bg) >= tc.ratio {
				if adjusted != fg {
					t.Fatal("already readable color changed")
				}
			}
		})
	}
	for _, ratio := range []float64{0, 1, math.NaN()} {
		fg := core.RGBA(0xaaaaaa, 0.5)
		if minimumContrast(fg, core.RGB(0), ratio) != fg {
			t.Fatal("disabled contrast changed text")
		}
	}
	fg, bg := core.RGB(0xffffff).Faded(0.1), core.RGB(0)
	if contrastRatio(minimumContrast(fg, bg, 4.5), bg) < 4.5 {
		t.Fatal("faint text was checked before compositing")
	}
}

func TestGraphicsRecording(t *testing.T) {
	for _, ratio := range []float32{1, 1.25, 2, 3} {
		for _, interval := range [][2]rune{{0x2500, 0x259f}, {0xe0b0, 0xe0b7}} {
			for r := interval[0]; r <= interval[1]; r++ {
				var d core.Drawing
				if !drawGraphics(&d, r, 3.25, 4.75, 8.5, 17.5, ratio, core.RGB(0xffffff)) {
					t.Fatalf("%U was left to the font", r)
				}
				painted := false
				for _, op := range d.Ops {
					if op.Kind == driver.DrawFillText {
						t.Fatalf("%U drew text", r)
					}
					painted = painted || op.Kind == driver.DrawFill || op.Kind == driver.DrawStroke
				}
				if !painted {
					t.Fatalf("%U recorded no painting at density %g", r, ratio)
				}
			}
		}
	}
	for _, r := range "A中😀" {
		var d core.Drawing
		if drawGraphics(&d, r, 0, 0, 8, 16, 1, core.RGB(0)) || len(d.Ops) != 0 {
			t.Fatalf("ordinary character %U became graphics", r)
		}
	}
}

func terminalCanvas(t *testing.T, app *nsgotest.App) *nsgotest.View {
	t.Helper()
	var find func(*nsgotest.View) *nsgotest.View
	find = func(v *nsgotest.View) *nsgotest.View {
		if v.Drawing != nil {
			return v
		}
		for _, child := range v.Children {
			if got := find(child); got != nil {
				return got
			}
		}
		return nil
	}
	if got := find(app.Find("id:term")); got != nil {
		return got
	}
	t.Fatal("terminal canvas missing")
	return nil
}

func TestTerminalGraphicsAndContrast(t *testing.T) {
	term := newTerminal(t, Options{Conn: newRecordingConn()})
	term.Feed([]byte("\x1b[?25l\x1b[38;2;32;32;32mTEXT \x1b[38;2;255;0;0m──█\ue0b0\x1b[0m\r\ne\u0301 👍🏽"))
	th := *DarkTheme()
	th.Background = core.RGB(0x202020)
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return View{Terminal: term, TestID: "term", MinimumContrast: 4.5, Theme: &th, DarkTheme: &th, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(320, 160))
	ink := terminalCanvas(t, app)
	current := core.Color(0)
	found := false
	graphemes := map[string]bool{"e\u0301": false, "👍🏽": false}
	for _, op := range ink.Drawing.Ops {
		if op.Kind == driver.DrawFillColor {
			current = op.Color
		}
		if op.Kind != driver.DrawFillText {
			continue
		}
		if strings.ContainsAny(op.Text, "─█\ue0b0") {
			t.Fatal("terminal graphics reached font drawing")
		}
		if _, ok := graphemes[op.Text]; ok {
			graphemes[op.Text] = true
		}
		if strings.Contains(op.Text, "TEXT") {
			found = true
			if contrastRatio(current, th.Background) < 4.5 {
				t.Fatal("minimum contrast was not applied to the frame")
			}
		}
	}
	if !found {
		t.Fatal("ordinary text missing")
	}
	for text, found := range graphemes {
		if !found {
			t.Fatalf("grapheme %q was duplicated or lost", text)
		}
	}
	app.Screenshot("terminal/frame-contrast")
}

func TestTerminalCursorBlink(t *testing.T) {
	term := newTerminal(t, Options{Conn: newRecordingConn(), CursorBlink: true})
	var ref Ref
	th := DarkTheme()
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return View{Terminal: term, TestID: "term", Label: "Terminal", AutoFocus: true, Ref: &ref, Theme: th, DarkTheme: th, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(320, 160))
	alpha := func() float32 {
		t.Helper()
		ink := terminalCanvas(t, app)
		for _, slot := range ink.Drawing.Slots {
			if slot.Arg == driver.ArgA {
				return ink.Ops[slot.Op].A
			}
		}
		t.Fatal("cursor has no native animated alpha")
		return 0
	}
	if nsgotest.Target() == "" {
		recorded := terminalCanvas(t, app).Drawing
		if got := alpha(); got != 1 {
			t.Fatalf("initial alpha=%g", got)
		}
		app.Advance(600 * time.Millisecond)
		if got := alpha(); got != 0 {
			t.Fatalf("blink alpha=%g, want zero", got)
		}
		app.Advance(500 * time.Millisecond)
		if got := alpha(); got != 1 {
			t.Fatalf("second phase alpha=%g, want one", got)
		}
		if terminalCanvas(t, app).Drawing != recorded {
			t.Fatal("blinking re-recorded the terminal on the engine")
		}
	} else {
		// The model's clock is separate on a native target. Inspect the
		// actual canvas pixels while its platform animation keeps running.
		for _, want := range []bool{false, true} {
			app.WaitUntil(2*time.Second, func() bool {
				img := app.Capture()
				s := float32(img.Bounds().Dx()) / float32(app.Find("id:term").Bounds().Size.Width)
				x, y := ref.v.padX+ref.v.cellW/2, ref.v.padY+ref.v.cellH/2
				c := imagecolor.NRGBAModel.Convert(img.At(int(x*s), int(y*s))).(imagecolor.NRGBA)
				return (c.R > 100) == want
			})
			app.Screenshot(fmt.Sprintf("terminal/blink-%t", want))
		}
	}
	term.Feed([]byte("\x1b[2 q"))
	app.WaitUntil(time.Second, func() bool { return len(terminalCanvas(t, app).Drawing.Slots) == 0 })
	term.Feed([]byte("\x1b[5 q"))
	app.WaitUntil(time.Second, func() bool { return len(terminalCanvas(t, app).Drawing.Slots) > 0 })
	term.Feed([]byte("\x1b[?12l"))
	app.WaitUntil(time.Second, func() bool { return len(terminalCanvas(t, app).Drawing.Slots) == 0 })
	term.Feed([]byte("\x1b[?12h"))
	app.WaitUntil(time.Second, func() bool { return len(terminalCanvas(t, app).Drawing.Slots) > 0 })
	app.Post(func() { ref.v.focused.Set(false) })
	app.WaitUntil(time.Second, func() bool { return len(terminalCanvas(t, app).Drawing.Slots) == 0 })
}

func TestGraphicsPixels(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Canvas{TestID: "graphics", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}, Draw: func(d *core.Drawing, _ core.Size) {
			d.SetFillColor(core.RGB(0)).FillRect(0, 0, 240, 220)
			for row, text := range []string{"───", "│││", "███", "▛▀▜", "\ue0b0\ue0b2"} {
				for col, r := range []rune(text) {
					drawGraphics(d, r, 20.25+float32(col)*20.5, 20.25+float32(row)*32.5, 20.5, 32.5, float32(cx.Env().PixelRatio()), core.RGB(0xffffff))
				}
			}
		}}
	}, nsgotest.Size(240, 220))
	img := app.Capture()
	scale := float64(img.Bounds().Dx()) / float64(app.Find("id:graphics").Bounds().Size.Width)
	white := func(x, y float64) {
		t.Helper()
		c := imagecolor.NRGBAModel.Convert(img.At(int(math.Round(x*scale)), int(math.Round(y*scale)))).(imagecolor.NRGBA)
		if c.R < 240 || c.G < 240 || c.B < 240 {
			t.Fatalf("gap at %.2f,%.2f: %#v", x, y, c)
		}
	}
	// Joins must stay covered where two cells meet, across both axes.
	for _, edge := range []float64{40.75, 61.25} {
		for _, offset := range []float64{-0.5, 0, 0.5} {
			white(edge+offset, 36.5)
			white(edge+offset, 100)
		}
	}
	white(30.5, 53)
	white(30.5, 84.75)
	app.Screenshot("terminal/graphics-joins")
}

func TestTerminalPaddingPixels(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   PaddingColor
		prefix string
		top    bool
	}{
		{"background", PaddingBackground, "", false},
		{"extend", PaddingExtend, "", true},
		{"prompt", PaddingExtend, "\x1b]133;A\x07", false},
		{"prompt-always", PaddingExtendAlways, "\x1b]133;A\x07", true},
		{"alternate", PaddingExtend, "\x1b[?1049h", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term := newTerminal(t, Options{Conn: newRecordingConn()})
			th := *DarkTheme()
			th.Background = core.RGB(0)
			app := nsgotest.Start(t, func(*core.Scope) core.Node {
				return View{Terminal: term, TestID: "term", Padding: Padding{20, 20}, PaddingColor: tc.mode, Theme: &th, DarkTheme: &th, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
			}, nsgotest.Size(320, 160))
			sequence := "\x1b[?25l\x1b[48;2;196;24;24m\x1b[2J"
			if strings.Contains(tc.prefix, "1049") {
				sequence = tc.prefix + sequence
			} else {
				sequence += tc.prefix
			}
			term.Feed([]byte(sequence))
			app.WaitUntil(time.Second, func() bool {
				term.mu.Lock()
				defer term.mu.Unlock()
				return term.rs.Colors().Background == rgb(th.Background)
			})
			app.Advance(30 * time.Millisecond)
			img := app.Capture()
			width := float64(app.Find("id:term").Bounds().Size.Width)
			scale := float64(img.Bounds().Dx()) / width
			check := func(x, y float64, red bool) {
				t.Helper()
				c := imagecolor.NRGBAModel.Convert(img.At(int(x*scale), int(y*scale))).(imagecolor.NRGBA)
				if red {
					if c.R < 185 || c.G > 35 || c.B > 35 {
						t.Fatalf("padding %.0f,%.0f=%#v, want red", x, y, c)
					}
				} else if c.R > 8 || c.G > 8 || c.B > 8 {
					t.Fatalf("padding %.0f,%.0f=%#v, want background", x, y, c)
				}
			}
			check(2, 2, tc.top)
			check(2, 80, tc.mode != PaddingBackground)
			check(width-2, 80, tc.mode != PaddingBackground)
			if tc.top {
				// A single-colored edge must have no dark seams between
				// fractional cell widths, including the outermost corners.
				for x := 2; x < int(width)-2; x++ {
					check(float64(x), 2, true)
				}
			}
			app.Screenshot("terminal/padding-" + tc.name)
		})
	}
}

func TestGraphicsAtlasPixels(t *testing.T) {
	runes := make([]rune, 0, 168)
	for r := rune(0x2500); r <= 0x259f; r++ {
		runes = append(runes, r)
	}
	for r := rune(0xe0b0); r <= 0xe0b7; r++ {
		runes = append(runes, r)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Canvas{TestID: "atlas", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}, Draw: func(d *core.Drawing, size core.Size) {
			d.SetFillColor(core.RGB(0)).FillRect(0, 0, float32(size.Width), float32(size.Height))
			for i, r := range runes {
				drawGraphics(d, r, 12+float32(i%16)*20.5, 12+float32(i/16)*30.5, 20.5, 30.5, float32(cx.Env().PixelRatio()), core.RGB(0xffffff))
			}
		}}
	}, nsgotest.Size(360, 370))
	img := app.Capture()
	scale := float64(img.Bounds().Dx()) / float64(app.Find("id:atlas").Bounds().Size.Width)
	for _, point := range []struct {
		x, y  float64
		white bool
	}{{177, 257, true}, {195, 285, true}, {138, 290, false}, {138, 311, true}, {153, 311, false}} {
		c := imagecolor.NRGBAModel.Convert(img.At(int(point.x*scale), int(point.y*scale))).(imagecolor.NRGBA)
		if (c.R > 230) != point.white {
			t.Fatalf("block pixel %g,%g=%#v", point.x, point.y, c)
		}
	}
	for _, r := range []rune{'╔', '╗', '╚', '╝'} {
		i := int(r - 0x2500)
		left, top := int((12+float64(i%16)*20.5)*scale)+1, int((12+float64(i/16)*30.5)*scale)+1
		right, bottom := int((12+float64(i%16+1)*20.5)*scale)-1, int((12+float64(i/16+1)*30.5)*scale)-1
		seen := make(map[[2]int]bool)
		components := 0
		white := func(x, y int) bool {
			c := imagecolor.NRGBAModel.Convert(img.At(x, y)).(imagecolor.NRGBA)
			return c.R > 200
		}
		for y := top; y < bottom; y++ {
			for x := left; x < right; x++ {
				start := [2]int{x, y}
				if seen[start] || !white(x, y) {
					continue
				}
				components++
				pending := [][2]int{start}
				seen[start] = true
				for len(pending) > 0 {
					at := pending[len(pending)-1]
					pending = pending[:len(pending)-1]
					for _, step := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
						p := [2]int{at[0] + step[0], at[1] + step[1]}
						if p[0] >= left && p[0] < right && p[1] >= top && p[1] < bottom && !seen[p] && white(p[0], p[1]) {
							seen[p] = true
							pending = append(pending, p)
						}
					}
				}
			}
		}
		if components != 2 {
			t.Fatalf("double corner %c has %d disconnected strokes, want two joined lanes", r, components)
		}
	}
	app.Screenshot("terminal/graphics-atlas")
}
