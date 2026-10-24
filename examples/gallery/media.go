package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/oneo-me/nsgo/core"
)

const riverURL = "https://picsum.photos/id/1015/600/300"

func imagesPage(cx *core.Scope, t *theme) core.Node {
	status := core.NewSignal(cx, "loading…")
	modes := []struct {
		name string
		mode core.ResizeMode
	}{{"Cover", core.Cover}, {"Contain", core.Contain}, {"Stretch", core.Stretch}, {"Center", core.Center},
		{"ScaleDown", core.ScaleDown}}
	var resized []core.Node
	for _, m := range modes {
		resized = append(resized, core.View{Style: core.Style{Gap: 4, AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Image{Source: core.Src(riverURL), ResizeMode: m.mode, Label: m.name, Placeholder: core.ImageSource{Data: logo},
				Style: core.Style{Width: core.Pt(120), Height: core.Pt(90), CornerRadius: 6, Background: t.sunken, Overflow: core.Hidden}},
			t.note(m.name),
		}})
	}
	frame := core.Style{Width: core.Pt(120), Height: core.Pt(90), CornerRadius: 6, Background: t.sunken, Overflow: core.Hidden}
	positions := []struct {
		name string
		mode core.ResizeMode
		at   core.ObjectPosition
	}{
		{"Contain, top", core.Contain, core.ObjectPosition{Y: core.PositionStart}},
		{"Contain, 8pt from the bottom", core.Contain, core.ObjectPosition{Y: core.PositionAxis{Offset: core.Pt(8), FromEnd: true}}},
		{"Cover, left", core.Cover, core.ObjectPosition{X: core.PositionStart}},
		{"Cover, right", core.Cover, core.ObjectPosition{X: core.PositionEnd}},
	}
	var placed []core.Node
	for _, p := range positions {
		style := frame
		style.ObjectPosition = p.at
		placed = append(placed, core.View{Style: core.Style{Gap: 4, AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Image{Source: core.Src(riverURL), ResizeMode: p.mode, Label: p.name, Style: style},
			t.note(p.name),
		}})
	}
	size := core.NewSignal(cx, "not asked")
	getSize := func() {
		size.Set("asking…")
		cx.ImageSize(core.Src(riverURL), func(px core.Size, err error) {
			if err != nil {
				size.Set("error: " + err.Error())
				return
			}
			size.Set(fmt.Sprintf("%g×%g px", px.Width, px.Height))
		})
	}
	prefetch := func() {
		size.Set("prefetching…")
		cx.PrefetchImage(core.Src(riverURL), func(ok bool) {
			if ok {
				size.Set("prefetched: views show it without loading again")
			} else {
				size.Set("prefetch failed")
			}
		})
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Embedded", "Bytes from go:embed, sized by the source's declared size; TintColor recolors opaque pixels.",
			hstack(
				core.Image{Source: core.ImageSource{Data: logo, Size: core.Size{Width: 96, Height: 96}}, Label: "Logo", TestID: "logo"},
				core.Image{Source: core.ImageSource{Data: logo}, ResizeMode: core.Contain, Label: "Tinted",
					Style: core.Style{Width: core.Pt(48), Height: core.Pt(48), TintColor: t.danger}},
				core.Image{Source: core.ImageSource{Data: logo}, ResizeMode: core.Contain, Label: "Rounded",
					Style: core.Style{Width: core.Pt(64), Height: core.Pt(64), CornerRadius: 32, Overflow: core.Hidden, Background: t.sunken}},
			),
		),
		t.demo("Remote", "A URL the platform fetches and caches, with a placeholder and progress until it loads.",
			core.Image{Source: core.Src(riverURL), ResizeMode: core.Cover, Label: "River",
				Style:       core.Style{Height: core.Pt(180), CornerRadius: cardRadius, Overflow: core.Hidden, Background: t.sunken},
				OnProgress:  func(loaded, total int64) { status.Set(fmt.Sprintf("loading… %d of %d bytes", loaded, total)) },
				OnLoad:      func(px core.Size) { status.Set(fmt.Sprintf("loaded %g×%g px", px.Width, px.Height)) },
				OnError:     func(err error) { status.Set("error: " + err.Error()) },
				Placeholder: core.ImageSource{Data: logo}},
			t.live(status.Get),
		),
		t.demo("Resize modes", "ResizeMode fits a 2:1 picture into a 4:3 view: Center keeps its size, cropped, and ScaleDown shrinks it.",
			flow(resized...)),
		t.demo("Object position", "The style's ObjectPosition places the fitted picture; it is centered by default.", flow(placed...)),
		t.demo("Blur and fade", "BlurRadius blurs the picture in points; FadeDuration fades it in over its placeholder.",
			hstack(
				core.Image{Source: core.Src(riverURL), BlurRadius: 6, Label: "Blurred", TestID: "blurred", Style: frame},
				core.Image{Source: core.Src(riverURL), FadeDuration: 800 * time.Millisecond, Label: "Fades in",
					Placeholder: core.ImageSource{Data: logo}, Style: frame},
			),
		),
		t.demo("Size and prefetch", "Scope.ImageSize loads an image and reports its pixels; PrefetchImage loads one ahead.",
			hstack(t.plain("Get size", getSize), t.plain("Prefetch", prefetch)),
			core.Text{ContentFunc: size.Get, Style: t.caption(), TestID: "image-size"},
		),
		t.demo("Animated and vector", "Animated GIF and WebP files play on the platform; an SVG file draws sharp at the view's size.",
			hstack(
				core.Image{Source: core.ImageSource{Data: animatedGIF}, Label: "Animated GIF", TestID: "gif",
					Style: core.Style{Width: core.Pt(64), Height: core.Pt(64)}},
				core.Image{Source: core.ImageSource{Data: []byte(badgeSVG)}, Label: "SVG badge", TestID: "svg",
					Style: core.Style{Width: core.Pt(96), Height: core.Pt(96)}},
				core.Image{Source: core.ImageSource{Data: []byte(starSVG)}, Label: "Tinted SVG", ResizeMode: core.Contain,
					Style: core.Style{Width: core.Pt(48), Height: core.Pt(48), TintColor: t.accent}},
			),
		),
	}}
}

func canvasPage(cx *core.Scope, t *theme) core.Node {
	bars := core.NewSignalFunc(cx, []float32{0.4, 0.75, 0.55, 0.9, 0.3, 0.65}, nil)
	shuffle := func() {
		next := make([]float32, len(bars.GetUntracked()))
		for i := range next {
			next[i] = 0.15 + 0.85*rand.Float32()
		}
		bars.Set(next)
	}
	colors := []core.Color{t.accent, t.success, t.warning, t.danger}

	chart := func(d *core.Drawing, size core.Size) {
		values := bars.Get()
		w, h := float32(size.Width), float32(size.Height)
		d.SetStrokeColor(t.line).SetLineWidth(1)
		for i := range 5 {
			y := h - 20 - float32(i)*(h-30)/4
			d.BeginPath().MoveTo(0, y).LineTo(w, y).Stroke()
		}
		slot := w / float32(len(values))
		for i, v := range values {
			bh := v * (h - 30)
			d.SetFillColor(colors[i%len(colors)]).BeginPath().
				RoundRect(float32(i)*slot+slot*0.2, h-20-bh, slot*0.6, bh, 4).Fill()
			d.SetFillColor(t.muted).FillText(fmt.Sprintf("%.0f", v*100), float32(i)*slot+slot*0.32, h-4,
				core.Font{Size: 11})
		}
	}
	scene := func(d *core.Drawing, size core.Size) {
		w, h := float32(size.Width), float32(size.Height)
		mx, my, r := w/2, h/2, min(w, h)/2-12
		// A pie of the same values.
		values := bars.Get()
		var total float32
		for _, v := range values {
			total += v
		}
		a := float32(-math.Pi / 2)
		for i, v := range values {
			sweep := v / total * 2 * math.Pi
			d.SetFillColor(colors[i%len(colors)]).BeginPath().MoveTo(mx, my).Arc(mx, my, r, a, a+sweep, false).ClosePath().Fill()
			a += sweep
		}
		d.SetFillColor(t.surface).BeginPath().Ellipse(mx, my, r*0.55, r*0.55).Fill()
		d.SetStrokeColor(t.ink).SetLineWidth(3).SetLineCap(core.CapRound).
			BeginPath().MoveTo(mx-r*0.25, my).QuadTo(mx, my-r*0.4, mx+r*0.25, my).Stroke()
	}
	box := core.Style{Height: core.Pt(200), FlexGrow: 1, FlexBasis: core.Pt(240), Background: t.background, CornerRadius: radius}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Drawing", "A Canvas records paths, fills, strokes and text; it draws again when a signal it read changes.",
			core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 12}, Children: []core.Node{
				core.Canvas{Draw: chart, Style: box, Label: "Bar chart", TestID: "chart"},
				core.Canvas{Draw: scene, Style: box, Label: "Pie chart"},
			}},
			t.primary("Shuffle", shuffle),
		),
	}}
}
