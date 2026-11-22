//go:build darwin && !ios

package nsgotest

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"sync"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
	"golang.org/x/image/font/sfnt"
)

func TestWindowlessCanvasSamples(t *testing.T) {
	for _, sample := range canvastest.Samples(PlatformTextEngine(), canvastest.Logo("..")) {
		t.Run(sample.Name, func(t *testing.T) {
			d := &core.Drawing{}
			d.SetFillColor(core.RGB(0xffffff)).FillRect(0, 0, float32(sample.Width), float32(sample.Height))
			d.Ops = append(d.Ops, sample.Drawing.Ops...)
			img, err := Render(d, core.Size{Width: core.Points(sample.Width), Height: core.Points(sample.Height)}, canvastest.Scale)
			if err != nil {
				t.Fatal(err)
			}
			for _, failure := range sample.Check(img) {
				t.Error(failure)
			}
		})
	}
}

func TestWindowlessScaleAndTransparency(t *testing.T) {
	draw := func(s float32) *core.Drawing {
		d := &core.Drawing{}
		d.SetFillColor(core.RGBA(0xff0000, 0.5)).FillRect(2*s, 3*s, 4*s, 5*s)
		return d
	}
	a, err := Render(draw(1), core.Size{Width: 12, Height: 12}, 3)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Render(draw(3), core.Size{Width: 36, Height: 36}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("scale differs from recording the same drawing three times as large")
	}
	if a.RGBAAt(0, 0) != (color.RGBA{}) || a.RGBAAt(9, 12) != (color.RGBA{128, 0, 0, 128}) {
		t.Fatalf("transparent/premultiplied pixels = %v / %v", a.RGBAAt(0, 0), a.RGBAAt(9, 12))
	}
}

func TestWindowlessImagesAndFailures(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(4, 5, 6, 7))
	for y := 5; y < 7; y++ {
		for x := 4; x < 6; x++ {
			pixels.SetRGBA(x, y, color.RGBA{0, 255, 0, 255})
		}
	}
	d := &core.Drawing{}
	d.DrawImageIn(core.ImageSource{Pixels: pixels}, 0, 0, 4, 4)
	off := &driver.OffscreenCanvas{ID: 1, Version: 1, Size: core.Size{Width: 4, Height: 4}, Scale: 1, Drawing: d}
	outer := &core.Drawing{}
	outer.DrawImageIn(core.ImageSource{Offscreen: off}, 0, 0, 4, 4)
	img, err := Render(outer, core.Size{Width: 4, Height: 4}, 1)
	if err != nil || img.RGBAAt(2, 2) != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("nested pixels = %v, error = %v", img, err)
	}
	cycle := &core.Drawing{}
	cycle.DrawImage(core.ImageSource{Offscreen: off}, 0, 0)
	off.Drawing = cycle
	if _, err := Render(cycle, core.Size{Width: 4, Height: 4}, 1); err == nil {
		t.Fatal("offscreen cycle did not fail")
	}
	bad := &core.Drawing{}
	bad.DrawImage(core.ImageSource{Data: []byte("not an image")}, 0, 0)
	if _, err := Render(bad, core.Size{Width: 4, Height: 4}, 1); err == nil {
		t.Fatal("invalid image did not fail")
	}
	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1), 1e10} {
		if _, err := Render(d, core.Size{Width: 4, Height: 4}, scale); err == nil {
			t.Errorf("invalid scale %v did not fail", scale)
		}
	}
}

func TestWindowlessConcurrentImageRenders(t *testing.T) {
	var jobs sync.WaitGroup
	errors := make(chan error, 4)
	for i := range 4 {
		jobs.Add(1)
		go func() {
			defer jobs.Done()
			want := color.RGBA{R: uint8(40 + i*30), G: 90, B: 120, A: 255}
			pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
			pixels.SetRGBA(0, 0, want)
			var encoded bytes.Buffer
			if err := png.Encode(&encoded, pixels); err != nil {
				errors <- err
				return
			}
			d := &core.Drawing{}
			d.DrawImageIn(core.ImageSource{Data: encoded.Bytes()}, 0, 0, 8, 8)
			for range 4 {
				img, err := Render(d, core.Size{Width: 8, Height: 8}, 1)
				if err != nil {
					errors <- err
					return
				}
				if got := img.RGBAAt(4, 4); got != want {
					errors <- fmt.Errorf("render %d: pixel = %v, want %v", i, got, want)
					return
				}
			}
		}()
	}
	jobs.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func TestWindowlessRegisteredFontAndMetrics(t *testing.T) {
	data, err := os.ReadFile("../base/formula/fonts/NewCMMath-Book.otf")
	if err != nil {
		t.Fatal(err)
	}
	font, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	family, err := font.Name(&buf, sfnt.NameIDPostScript)
	if err != nil {
		t.Fatal(err)
	}
	var glyph rune
	for code := rune(0xe000); code <= 0xf8ff; code++ {
		index, err := font.GlyphIndex(&buf, code)
		if err == nil && index != 0 {
			glyph = code
			break
		}
	}
	if glyph == 0 {
		t.Fatal("the fixture has no Private Use Area glyph")
	}
	f := core.Font{Family: family, Size: 32}
	fallback := &core.Drawing{}
	fallback.SetFillColor(core.RGB(0)).FillText(string(glyph), 10, 40, core.Font{Family: "NSGo Missing PUA Font", Size: 32})
	before, err := Render(fallback, core.Size{Width: 100, Height: 60}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterFont(data); err != nil {
		t.Fatal(err)
	}
	native := PlatformTextEngine()
	var drawing *core.Drawing
	var metrics core.TextMetrics
	app := Start(t, func(cx *core.Scope) core.Node {
		canvas := core.Canvas{TestID: "page", Style: core.Style{Width: core.Pt(100), Height: core.Pt(60)}, Draw: func(d *core.Drawing, _ core.Size) {
			drawing = d
			metrics = d.TextMetrics(string(glyph), f)
			d.SetFillColor(core.RGB(0)).FillText(string(glyph), 10, 40, f)
		}}
		return core.View{Style: core.Style{AlignItems: core.AlignStart}, Children: []core.Node{
			core.Text{Content: string(glyph), TestID: "native-text", Style: core.Style{FontFamily: family, FontSize: 32}}, canvas,
		}}
	}, TextEngine(native))
	if app.Query("id:page") == nil || drawing == nil || metrics.Width <= 0 {
		t.Fatalf("native canvas metrics = %+v", metrics)
	}
	direct := driver.NewDrawing(native).TextMetrics(string(glyph), core.Font{Family: family, Size: 32})
	if metrics != direct {
		t.Fatalf("headless native metrics = %+v, direct native metrics = %+v", metrics, direct)
	}
	if width := app.Find("id:native-text").Frame.Size.Width; math.Abs(float64(width)-math.Ceil(float64(direct.Width))) > 0.01 {
		t.Fatalf("headless text width = %v, native width = %v", width, direct.Width)
	}
	img, err := Render(drawing, core.Size{Width: 100, Height: 60}, 1)
	if err != nil {
		t.Fatal(err)
	}
	ink := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			ink++
		}
	}
	if ink == 0 {
		t.Fatal("registered PUA glyph drew no pixels")
	}
	if bytes.Equal(before.Pix, img.Pix) {
		t.Fatalf("font %q glyph %U rendered the same pixels as a missing family", family, glyph)
	}
}
