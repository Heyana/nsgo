package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math"
	"net/url"
	"path"
	"strings"

	"github.com/oneo-me/nsgo/core"
)

// components are the gallery's native components, registered in main:
// each platform's file adds its own.
var components = map[string]core.NativeFactory{}

// sliderComponent is the name of the platform's own slider, which each
// platform's file registers where it has one.
const sliderComponent = "gallery.slider"

func nativePage(cx *core.Scope, t *theme) core.Node {
	value := core.NewSignal(cx, 0.4)
	events := core.NewSignal(cx, "none yet")
	var slider core.Ref
	_, ok := components[sliderComponent]
	note := "This platform registers no slider, so the box stays empty."
	if ok {
		note = "Dragging the platform's slider reports change events; the buttons set its value prop and send it a command."
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Native component", "nsgo.Component registers a platform view by name; a Native element shows it, sized by the style or by what the component asks for. "+note,
			core.Native{Component: sliderComponent, Ref: &slider, TestID: "native-slider", Label: "Native slider",
				PropsFunc: func() map[string]any { return map[string]any{"value": value.Get()} },
				OnEvent: func(name string, data any) {
					if v, ok := data.(float64); ok && name == "change" {
						value.Set(v)
					}
					events.Set(fmt.Sprintf("%s %v", name, data))
				},
				Style: core.Style{AlignSelf: core.AlignStart, MinWidth: core.Pt(200), MinHeight: core.Pt(28)}},
			core.Text{TestID: "native-value", Style: t.caption(), ContentFunc: func() string {
				return fmt.Sprintf("value %.2f · last event: %s", value.Get(), events.Get())
			}},
			flow(
				t.primary("Set 0.8", func() { value.Set(0.8) }),
				t.plain("Reset command", func() { slider.Command("reset", nil) }),
			),
		),
	}}
}

func filesPage(cx *core.Scope, t *theme) core.Node {
	over := core.NewSignal(cx, false)
	dropped := core.NewSignalFunc[[]string](cx, nil, nil)
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("File drop", "OnFileDrop receives files dragged from another app as file: URLs; on the web, as blob: URLs named by their fragment. OnFileDragEnter and OnFileDragLeave follow the drag.",
			core.View{TestID: "drop-zone", Label: "Drop files here",
				OnFileDragEnter: func() { over.Set(true) }, OnFileDragLeave: func() { over.Set(false) },
				OnFileDrop: func(uris []string) { over.Set(false); dropped.Set(uris) },
				StyleFunc: func() core.Style {
					s := core.Style{Height: core.Pt(140), CornerRadius: cardRadius, BorderWidth: 2, BorderColor: t.line,
						Background: t.background, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
					if over.Get() {
						s.BorderColor, s.Background = t.accent, t.sunken
					}
					return s
				},
				Children: []core.Node{core.Text{Style: t.caption(), ContentFunc: func() string {
					if over.Get() {
						return "Release to drop"
					}
					return "Drop files here"
				}}}},
			core.Text{TestID: "dropped", Style: t.caption(), ContentFunc: func() string {
				uris := dropped.Get()
				if len(uris) == 0 {
					return "Nothing dropped yet."
				}
				var names []string
				for _, u := range uris {
					names = append(names, fileName(u))
				}
				return fmt.Sprintf("%d dropped: %s", len(uris), strings.Join(names, ", "))
			}},
		),
	}}
}

// fileName is the name a dropped file's URL gives it.
func fileName(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	if u.Fragment != "" {
		return u.Fragment
	}
	return path.Base(u.Path)
}

// accessoryDemo is a field with a bar above the software keyboard.
func accessoryDemo(cx *core.Scope, t *theme) core.Node {
	text := core.NewSignal(cx, "")
	return t.demo("Keyboard accessory", "An InputAccessoryView rides above the software keyboard while a field naming it has focus, on iOS and Android.",
		core.Input{Placeholder: "Write a note", InputAccessoryViewID: "gallery.note", Value: text.Get, OnChange: text.Set,
			TestID: "accessory-field", Style: t.field()},
		core.InputAccessoryView{ID: "gallery.note", TestID: "accessory-bar", Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(6, 12),
				Background: t.surface, BorderColor: t.line, Borders: core.Edges{Top: 1}}, Children: []core.Node{
				core.Text{Style: core.Style{FlexGrow: 1, Color: t.muted, FontSize: 13},
					ContentFunc: func() string { return fmt.Sprintf("%d characters", len([]rune(text.Get()))) }},
				t.plain("Clear", func() { text.Set("") }),
				t.primary("Done", cx.DismissKeyboard),
			}},
		}},
	)
}

// animatedGIF draws a spinner of eight frames as an animated GIF.
var animatedGIF = func() []byte {
	const size = 64
	pal := color.Palette{color.Transparent, color.RGBA{0x2b, 0x6c, 0xf0, 0xff}, color.RGBA{0xc8, 0xd6, 0xf8, 0xff}}
	g := &gif.GIF{LoopCount: 0}
	for f := range 8 {
		img := image.NewPaletted(image.Rect(0, 0, size, size), pal)
		for i := range 8 {
			a := float64(i) * math.Pi / 4
			c := uint8(2)
			if i == f {
				c = 1
			}
			cx, cy := size/2+math.Cos(a)*22, size/2+math.Sin(a)*22
			for y := range size {
				for x := range size {
					if math.Hypot(float64(x)-cx, float64(y)-cy) < 6 {
						img.SetColorIndex(x, y, c)
					}
				}
			}
		}
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
		g.Disposal = append(g.Disposal, gif.DisposalBackground)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		panic(err)
	}
	return b.Bytes()
}()

// badgeSVG is an SVG file with a gradient, a transform and opacity.
const badgeSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 96 96">
  <defs>
    <linearGradient id="sky" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#ffb347"/>
      <stop offset="1" stop-color="#7b2ff7"/>
    </linearGradient>
  </defs>
  <rect width="96" height="96" rx="22" fill="url(#sky)"/>
  <g transform="translate(48 48) rotate(-12)">
    <path d="M0 -26 L7 -8 L26 -8 L11 4 L16 23 L0 12 L-16 23 L-11 4 L-26 -8 L-7 -8 Z" fill="white" fill-opacity="0.9"/>
  </g>
  <circle cx="76" cy="20" r="8" fill="rgb(255,255,255)" opacity="0.5"/>
</svg>`

// starSVG is a shape on a transparent ground, which a tint recolors.
const starSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-30 -30 60 60">
  <path d="M0 -26 L7 -8 L26 -8 L11 4 L16 23 L0 12 L-16 23 L-11 4 L-26 -8 L-7 -8 Z" fill="black"/>
</svg>`
