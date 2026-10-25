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
	row := func(children ...core.Node) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 12}, Children: children}
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Drawing", "A Canvas records paths, fills, strokes and text; it draws again when a signal it read changes.",
			row(core.Canvas{Draw: chart, Style: box, Label: "Bar chart", TestID: "chart"},
				core.Canvas{Draw: scene, Style: box, Label: "Pie chart"}),
			t.primary("Shuffle", shuffle),
		),
		t.demo("Strokes and paints", "Dashes, joins and miter limits; linear, radial and conic gradients; shadows.",
			row(core.Canvas{Draw: strokes(t), Style: box, Label: "Strokes", TestID: "strokes"},
				core.Canvas{Draw: paints(t), Style: box, Label: "Gradients and shadows", TestID: "paints"}),
		),
		t.demo("Shapes, text and images", "Transforms, arcTo, rotated ellipses, per-corner rectangles and SVG paths; aligned, stroked text; images by URI or bytes.",
			row(core.Canvas{Draw: shapes(t), Style: box, Label: "Shapes", TestID: "shapes"},
				core.Canvas{Draw: texts(t), Style: box, Label: "Text", TestID: "canvas-text"},
				core.Canvas{Draw: pictures(t), Style: box, Label: "Images", TestID: "canvas-images"}),
		),
		t.demo("Composite modes", "How each operator mixes a blue circle drawn over a red square, as the web's globalCompositeOperation.",
			core.Canvas{Draw: composites(t), Style: core.Style{Height: core.Pt(300), Background: t.background, CornerRadius: radius},
				Label: "Composite modes", TestID: "composites"},
		),
		hitTests(cx, t, box),
		animatedDrawing(cx, t, box),
	}}
}

// strokes draws lines with dashes, caps, joins and miter limits.
func strokes(t *theme) func(*core.Drawing, core.Size) {
	return func(d *core.Drawing, size core.Size) {
		w := float32(size.Width)
		d.SetStrokeColor(t.ink).SetLineWidth(3)
		for i, dash := range [][]float32{nil, {12, 6}, {2, 6}, {16, 4, 2, 4}} {
			y := 24 + float32(i)*22
			d.SetLineDash(dash...).SetLineDashOffset(0).BeginPath().MoveTo(16, y).LineTo(w-16, y).Stroke()
		}
		d.SetLineDash()
		d.SetLineWidth(10).SetStrokeColor(t.accent)
		for i, j := range []core.LineJoin{core.JoinMiter, core.JoinRound, core.JoinBevel} {
			x := 30 + float32(i)*(w-60)/3
			d.SetLineJoin(j).BeginPath().MoveTo(x, 170).LineTo(x+20, 120).LineTo(x+40, 170).Stroke()
		}
		d.SetLineJoin(core.JoinMiter).SetMiterLimit(1).SetStrokeColor(t.success).
			BeginPath().MoveTo(w-60, 175).LineTo(w-45, 125).LineTo(w-30, 175).Stroke()
		d.SetMiterLimit(10)
	}
}

// paints fills with gradients and casts shadows.
func paints(t *theme) func(*core.Drawing, core.Size) {
	return func(d *core.Drawing, size core.Size) {
		w, h := float32(size.Width), float32(size.Height)
		cw := (w - 64) / 3
		stops := []core.GradientStop{{Offset: 0, Color: t.accent}, {Offset: 0.5, Color: t.warning}, {Offset: 1, Color: t.danger}}
		d.SetFillGradient(core.CanvasLinear(16, 0, 16+cw, 0, stops...)).
			BeginPath().RoundRect(16, 16, cw, 80, 10).Fill()
		x := 32 + cw
		d.SetFillGradient(core.CanvasRadial(x+cw/2-10, 46, 4, x+cw/2, 56, cw/2, stops...)).
			BeginPath().RoundRect(x, 16, cw, 80, 10).Fill()
		x += 16 + cw
		d.SetFillGradient(core.CanvasConic(0, x+cw/2, 56, append(stops, core.GradientStop{Offset: 1, Color: t.accent})...)).
			BeginPath().Ellipse(x+cw/2, 56, cw/2, 40).Fill()
		d.Save().SetShadow(core.RGBA(0x000000, 0.35), 4, 6, 10).SetFillColor(t.surface).
			BeginPath().RoundRect(16, 116, w/2-24, h-132, 12).Fill().Restore()
		d.Save().SetShadow(core.RGBA(0x0969da, 0.6), 0, 0, 16).
			SetStrokeGradient(core.CanvasLinear(w/2+8, 0, w-16, 0, stops...)).SetLineWidth(6).
			BeginPath().Arc(w*0.75, 116+(h-132)/2, min(w/4-16, (h-132)/2-6), 0, 2*math.Pi, false).Stroke().Restore()
	}
}

// shapes draws with transforms, arcTo, ellipse arcs, per-corner
// rectangles and an SVG path.
func shapes(t *theme) func(*core.Drawing, core.Size) {
	heart := core.MustParsePath("M 12 21 C 12 21 3 14.5 3 8.5 A 4.5 4.5 0 0 1 12 6 A 4.5 4.5 0 0 1 21 8.5 C 21 14.5 12 21 12 21 Z")
	return func(d *core.Drawing, size core.Size) {
		w := float32(size.Width)
		d.SetFillColor(t.accent).BeginPath().RoundRectCorners(16, 16, 80, 60, 24, 4, 24, 4).Fill()
		d.SetStrokeColor(t.ink).SetLineWidth(3).BeginPath().MoveTo(112, 76).ArcTo(112, 16, 172, 16, 24).
			ArcTo(172, 16, 172, 76, 12).Stroke()
		d.SetFillColor(t.warning).BeginPath().EllipseArc(w-50, 46, 34, 16, math.Pi/6, 0, 2*math.Pi, false).Fill()
		d.SetStrokeColor(t.danger).BeginPath().EllipseArc(w-50, 46, 34, 16, -math.Pi/6, 0, math.Pi, false).Stroke()
		for i := range 5 {
			// A skewed and rotated heart each: the transform maps the path.
			a := float32(i) * 0.3
			d.Save().Translate(28+float32(i)*48, 112).Transform(1, 0, 0.2*float32(i-2), 1, 0, 0).Rotate(a-0.6).
				Scale(1.6, 1.6).Translate(-12, -12)
			d.SetFillColor(colorsAt(t, i)).FillPath(heart, core.NonZero).Restore()
		}
		d.SetTransform(1, 0, 0, 1, 0, 0).SetFillColor(t.muted).FillText("SetTransform resets", 16, 186, core.Font{Size: 12})
	}
}

func colorsAt(t *theme, i int) core.Color {
	c := []core.Color{t.accent, t.success, t.warning, t.danger}
	return c[i%len(c)]
}

// texts draws text by its alignments and baselines, filled with a
// gradient and stroked.
func texts(t *theme) func(*core.Drawing, core.Size) {
	return func(d *core.Drawing, size core.Size) {
		w := float32(size.Width)
		mid := w / 2
		d.SetStrokeColor(t.line).SetLineWidth(1).BeginPath().MoveTo(mid, 8).LineTo(mid, 112).Stroke()
		font := core.Font{Size: 14}
		for i, a := range []core.TextAlign{core.TextAlignStart, core.TextAlignCenter, core.TextAlignEnd} {
			d.SetTextAlign(a).SetFillColor(t.ink).FillText([]string{"start", "center", "end"}[i], mid, 28+float32(i)*20, font)
		}
		d.SetTextAlign(core.TextAlignStart)
		y := float32(130)
		d.BeginPath().MoveTo(8, y).LineTo(w-8, y).Stroke()
		x := float32(10)
		for i, b := range []core.TextBaseline{core.BaselineTop, core.BaselineMiddle, core.BaselineAlphabetic, core.BaselineBottom} {
			name := []string{"top", "middle", "alpha", "bottom"}[i]
			d.SetTextBaseline(b).FillText(name, x, y, core.Font{Size: 12})
			x += d.MeasureText(name, core.Font{Size: 12}) + 10
		}
		d.SetTextBaseline(core.BaselineAlphabetic)
		big := core.Font{Size: 30, Weight: core.FontWeightBold}
		d.SetFillGradient(core.CanvasLinear(10, 0, 10+d.MeasureText("Gradient", big), 0,
			core.GradientStop{Offset: 0, Color: t.accent}, core.GradientStop{Offset: 1, Color: t.danger})).
			FillText("Gradient", 10, 180, big)
		d.SetStrokeColor(t.ink).SetLineWidth(1.5).StrokeText("Stroke", w-10-d.MeasureText("Stroke", big), 180, big)
	}
}

// pictures draws the logo at its size, stretched and cut.
func pictures(t *theme) func(*core.Drawing, core.Size) {
	src := core.ImageSource{Data: logo}
	return func(d *core.Drawing, size core.Size) {
		d.DrawImageIn(src, 16, 16, 72, 72)
		d.Save().Translate(150, 52).Rotate(0.3).DrawImageIn(src, -36, -36, 72, 72).Restore()
		d.DrawImageFrom(src, 0, 0, 0.5, 0.5, 16, 104, 72, 72)
		d.Save().SetAlpha(0.5).DrawImageIn(src, 112, 104, 120, 72).Restore()
	}
}

// composites draws each operator over the same backdrop, a layer of its
// own each so the operators that clear stay in their cell.
func composites(t *theme) func(*core.Drawing, core.Size) {
	ops := []core.CompositeOp{core.CompositeSourceOver, core.CompositeSourceIn, core.CompositeSourceOut,
		core.CompositeSourceAtop, core.CompositeDestinationOver, core.CompositeDestinationIn,
		core.CompositeDestinationOut, core.CompositeDestinationAtop, core.CompositeLighter, core.CompositeCopy,
		core.CompositeXor, core.CompositeMultiply, core.CompositeScreen, core.CompositeOverlay, core.CompositeDarken,
		core.CompositeLighten, core.CompositeColorDodge, core.CompositeColorBurn, core.CompositeHardLight,
		core.CompositeSoftLight, core.CompositeDifference, core.CompositeExclusion, core.CompositeHue,
		core.CompositeSaturation, core.CompositeColor, core.CompositeLuminosity}
	return func(d *core.Drawing, size core.Size) {
		cols := max(int(float32(size.Width)/90), 1)
		cw := float32(size.Width) / float32(cols)
		for i, op := range ops {
			x, y := float32(i%cols)*cw, float32(i/cols)*58
			d.Save().BeginPath().Rect(x, y, cw, 58).Clip()
			d.SetFillColor(core.RGB(0xe5534b)).FillRect(x+8, y+6, 30, 30)
			d.SetComposite(op).SetFillColor(core.RGBA(0x2f81f7, 0.85)).BeginPath().Arc(x+38, y+30, 15, 0, 2*math.Pi, false).Fill()
			d.SetComposite(core.CompositeSourceOver).SetFillColor(t.muted).FillText(op.Name(), x+4, y+54, core.Font{Size: 9})
			d.Restore()
		}
	}
}

// hitTests names each bar of a chart for assistive technology and tests,
// and selects the bar a press lands in.
func hitTests(cx *core.Scope, t *theme, box core.Style) core.Node {
	values := []float32{0.5, 0.8, 0.35, 0.65}
	selected := core.NewSignal(cx, -1)
	var last *core.Drawing
	paths := make([]*core.Path, len(values))
	draw := func(d *core.Drawing, size core.Size) {
		w, h := float32(size.Width), float32(size.Height)
		slot := w / float32(len(values))
		for i, v := range values {
			x, bh := float32(i)*slot+slot*0.15, v*(h-24)
			paths[i] = new(core.Path).RoundRect(x, h-12-bh, slot*0.7, bh, 6)
			fill := t.line
			if selected.Get() == i {
				fill = t.accent
			}
			d.SetFillColor(fill).FillPath(paths[i], core.NonZero)
			d.Node(core.CanvasNode{Bounds: core.Bounds{Origin: core.Point{X: core.Points(x), Y: core.Points(h - 12 - bh)},
				Size: core.Size{Width: core.Points(slot * 0.7), Height: core.Points(bh)}},
				Role: core.RoleButton, Label: fmt.Sprintf("Bar %d, %.0f%%", i+1, v*100), TestID: fmt.Sprintf("bar-%d", i+1),
				Selected: selected.Get() == i})
		}
		last = d
	}
	press := func(e core.PressEvent) {
		for i, p := range paths {
			if last != nil && p != nil && last.PathContains(p, float32(e.Position.X), float32(e.Position.Y), core.NonZero) {
				selected.Set(i)
				return
			}
		}
		selected.Set(-1)
	}
	return t.demo("Nodes and hit tests", "Each bar is a node assistive technology and tests see; a press selects the bar PathContains finds.",
		core.Canvas{Draw: draw, OnPress: press, Style: box, Label: "Selectable bars", TestID: "bars"},
		t.live(func() string {
			if i := selected.Get(); i >= 0 {
				return fmt.Sprintf("Bar %d selected", i+1)
			}
			return "No bar selected"
		}),
	)
}

// animatedDrawing binds drawing arguments to animated values, which the
// platform moves on its frame clock without a commit.
func animatedDrawing(cx *core.Scope, t *theme, box core.Style) core.Node {
	phase := core.NewAnimatedValue(cx, 0)
	cx.StartAnimation(core.Loop(core.Timing(phase, 1, core.TimingConfig{Duration: 2 * time.Second, Easing: core.BezierEasing(core.EaseInOut)}), 0), nil)
	radius := core.Interpolate(phase, core.Interpolation{Input: []float64{0, 0.5, 1}, Output: []float64{12, 60, 12}})
	color := core.Interpolate(phase, core.Interpolation{Input: []float64{0, 0.5, 1}, Colors: []core.Color{t.accent, t.danger, t.accent}})
	sweep := core.Interpolate(phase, core.Interpolation{Input: []float64{0, 1}, Output: []float64{-math.Pi / 2, 3 * math.Pi / 2}})
	draw := func(d *core.Drawing, size core.Size) {
		w, h := float32(size.Width), float32(size.Height)
		d.SetFillColor(t.accent).Bind(core.ArgColor, color).
			BeginPath().Arc(w/4, h/2, 12, 0, 2*math.Pi, false).Bind(core.ArgR, radius).Fill()
		d.SetStrokeColor(t.success).SetLineWidth(10).SetLineCap(core.CapRound).
			BeginPath().Arc(w*0.72, h/2, min(w/5, h/2-16), -math.Pi/2, 0, false).Bind(core.ArgA1, sweep).Stroke()
	}
	return t.demo("Animated drawing", "Drawing.Bind ties an argument to an animated value: the platform redraws as it moves, with no commit.",
		core.Canvas{Draw: draw, Style: box, Label: "Animated drawing", TestID: "animated-drawing"},
	)
}
