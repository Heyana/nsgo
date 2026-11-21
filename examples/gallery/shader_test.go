package main

import (
	"image/color"
	"os"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestShaderEffects draws the Shader page's WGSL demos: the wave's card
// still takes presses, the dissolve runs between its panels and a page
// opens over its stack through a circle and closes through it.
func TestShaderEffects(t *testing.T) {
	app := startDesktop(t, nsgotest.Size(1200, 900))
	if os.Getenv("NSGO_GALLERY_OPEN") != "Shader" {
		open(app, "Shader")
	}
	app.WaitFor("id:wave-effect", 10*time.Second)
	if fx := app.Find("id:wave-effect").Shader; fx == nil || fx.Role != nsgotest.ShaderEffect || !fx.WGSL {
		t.Fatalf("the wave is %+v", fx)
	}
	app.Screenshot("shader-effect-before")
	app.Press("id:wave-button")
	app.WaitFor("text:Start pressed 1 time", time.Second)
	app.Screenshot("shader-effect")
	app.Press("id:dissolve-next")
	app.Find("text:Second panel")
	if tr := app.Find("id:dissolve"); tr.ShaderStarts != 1 {
		t.Errorf("the dissolve started %d times", tr.ShaderStarts)
	}
	app.Screenshot("shader-transition")
	app.Press("id:page-home")
	app.WaitFor("id:page-page", time.Second)
	app.Screenshot("shader-page")
	app.Press("id:page-page")
	app.WaitFor("id:page-home", time.Second)
}

// TestShaderEffectsPhone does the same on a phone, where the page slides
// in its own way otherwise.
func TestShaderEffectsPhone(t *testing.T) {
	var app *nsgotest.App
	if target := nsgotest.Target(); target == "ios_simulator" || target == "android" {
		// Open the demo in the phone's own environment.
		t.Setenv("NSGO_GALLERY_OPEN", "Shader")
		app = nsgotest.Start(t, Root, nsgotest.Environment(core.Environment{Locale: "en-US"}))
	} else {
		app = startPhone(t)
		openPhone(app, "Shader")
	}
	app.WaitFor("id:wave-effect", 10*time.Second)
	app.Press("id:wave-button")
	app.WaitFor("text:Start pressed 1 time", time.Second)
	app.Press("id:dissolve-next")
	app.Find("text:Second panel")
	app.Press("id:page-home")
	app.WaitFor("id:page-page", time.Second)
	app.Press("id:page-page")
	app.WaitFor("id:page-home", time.Second)
}

// TestShaderPictures displays a canvas through each WGSL picture role. The
// colors distinguish the presented shader frame from either unprocessed view.
func TestShaderPictures(t *testing.T) {
	switch nsgotest.Target() {
	case "android":
	default:
		t.Skip("requires a native shader renderer")
	}
	effect := core.MustCompileWGSL(`
fn shade(uv: vec2f) -> vec4f {
    let c = sample_content(uv);
    return vec4f(c.bgr, c.a);
}
`)
	backdrop := core.MustCompileWGSL(`
fn shade(uv: vec2f) -> vec4f {
	let c = sample_backdrop(uv);
	return vec4f(c.bgr, c.a);
}
`)
	transition := core.MustCompileWGSL(`
fn shade(uv: vec2f) -> vec4f {
    let before = sample_from(uv);
    let after = sample_to(uv);
    return vec4f(before.r, after.b, 0.0, 1.0);
}
`)

	for _, test := range []struct {
		name                    string
		screen, changes, behind bool
	}{
		{name: "view effect"},
		{name: "screen effect", screen: true},
		{name: "backdrop", behind: true},
		{name: "view transition", changes: true},
		{name: "screen transition", screen: true, changes: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			const duration = 7 * time.Second
			var startTransition func()
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				changed := core.NewSignal(cx, false)
				startTransition = func() { cx.Post(func() { changed.Set(true) }) }
				child := core.Canvas{TestID: "picture", Style: core.Style{Width: core.Pt(180), Height: core.Pt(180)},
					OnPress: func(core.PressEvent) { changed.Set(true) },
					Draw: func(d *core.Drawing, size core.Size) {
						c := core.RGB(0xff0000)
						if changed.Get() {
							c = core.RGB(0x0000ff)
						}
						width, height := float32(size.Width), float32(size.Height)
						d.SetFillColor(c).FillRect(0, 0, width, height/2)
						d.SetFillColor(core.RGB(0x00ff00)).FillRect(0, height/2, width, height/2)
					},
				}
				var children []core.Node
				style := core.Style{FlexGrow: 1}
				switch {
				case test.behind:
					style.Background = core.RGB(0xffff00)
					children = []core.Node{core.ShaderEffect{Program: backdrop, Children: []core.Node{child}}}
				case test.screen && test.changes:
					children = []core.Node{child, core.ScreenTransition{Program: transition, Duration: duration,
						Value: func() any { return changed.Get() }}}
				case test.screen:
					children = []core.Node{child, core.ScreenEffect{Program: effect}}
				case test.changes:
					children = []core.Node{core.ShaderTransition{Program: transition, Duration: duration,
						Value: func() any { return changed.Get() }, Children: []core.Node{child}}}
				default:
					children = []core.Node{core.ShaderEffect{Program: effect, MaxSampleOffset: 8,
						Children: []core.Node{child}}}
				}
				return core.View{Style: style, Children: children}
			}, nsgotest.Size(320, 320))

			bounds := app.Find("id:picture").Bounds()
			root := app.Find("id:picture")
			for root.Parent != nil {
				root = root.Parent
			}
			wantPixel := func(want color.RGBA, timeout time.Duration) {
				t.Helper()
				until := time.Now().Add(timeout)
				for {
					img := app.Capture()
					x := img.Bounds().Min.X + int((bounds.Origin.X+bounds.Size.Width/2)*core.Points(img.Bounds().Dx())/root.Frame.Size.Width)
					y := img.Bounds().Min.Y + int((bounds.Origin.Y+bounds.Size.Height/4)*core.Points(img.Bounds().Dy())/root.Frame.Size.Height)
					got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
					if absByte(got.R, want.R) < 12 && absByte(got.G, want.G) < 12 && absByte(got.B, want.B) < 12 {
						return
					}
					if time.Now().After(until) {
						t.Fatalf("window pixel at %d,%d = %v, want %v", x, y, got, want)
					}
				}
			}
			red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
			switch {
			case test.behind:
				wantPixel(color.RGBA{G: 255, B: 255, A: 255}, time.Second)
			case test.changes:
				wantPixel(red, time.Second)
				startTransition()
				wantPixel(color.RGBA{R: 255, G: 255, A: 255}, time.Second)
				wantPixel(blue, duration+time.Second)
			default:
				wantPixel(blue, time.Second)
				app.Press("id:picture")
				wantPixel(red, time.Second)
			}
		})
	}
}

func absByte(x, y uint8) uint8 {
	return max(x, y) - min(x, y)
}

// TestPerfShaderCanvas measures an effect sampling a canvas drawn on every frame.
func TestPerfShaderCanvas(t *testing.T) {
	perfScenarios(t)
	program := core.MustCompileWGSL(`
fn shade(uv: vec2f) -> vec4f {
    return sample_content(uv);
}
`)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		x := core.NewSignal(cx, float32(0))
		var frame func(time.Time)
		frame = func(time.Time) {
			x.Set(float32(int(x.GetUntracked()+2) % 120))
			cx.RequestAnimationFrame(frame)
		}
		cx.RequestAnimationFrame(frame)
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.ShaderEffect{Program: program, Children: []core.Node{
				core.Canvas{Style: core.Style{Width: core.Pt(320), Height: core.Pt(320)},
					Draw: func(d *core.Drawing, size core.Size) {
						d.SetFillColor(core.RGB(0x222244)).FillRect(0, 0, float32(size.Width), float32(size.Height))
						d.SetFillColor(core.RGB(0xffaa33)).FillRect(x.Get(), 20, 200, 280)
					}},
			}},
		}}
	}, perfOptions()...)
	app.Advance(time.Second)
	app.Mark("Shader effect over a moving canvas")
	app.Advance(perfSeconds(8))
	app.Mark("-")
}
