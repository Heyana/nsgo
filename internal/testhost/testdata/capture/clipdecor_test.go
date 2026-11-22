package capture_test

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A clip keeps the node's decoration outside it, while cutting off content at its actual corners. Hard shadows put the samples far from soft edges.
func TestClipKeepsOuterDecoration(t *testing.T) {
	if nsgotest.Target() != "android" {
		t.Skip("checks Android's native view drawing")
	}
	type sample struct{ kind, corners string }
	var selected *core.Signal[sample]
	var ratio float32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ratio = cx.Env().PixelRatio()
		selected = core.NewSignal(cx, sample{"view", "round"})
		var children []core.Node
		for _, kind := range []string{"view", "view-ripple", "view-foreground-ripple", "canvas", "image", "text", "input", "web", "scroll"} {
			children = append(children, clipDecorationNode(kind, func() core.Style {
				current := selected.Get()
				style := core.Style{Position: core.Absolute, Left: core.Pt(80), Top: core.Pt(80),
					Width: core.Pt(120), Height: core.Pt(80), Overflow: core.Hidden,
					Background: core.RGB(0x0000ff), CornerRadius: 24,
					Shadows:      []core.BoxShadow{{Spread: 12, Color: core.RGB(0xff0000)}},
					OutlineWidth: 4, OutlineOffset: 2, OutlineColor: core.RGB(0x00ff00)}
				if current.kind != kind {
					style.Display = core.DisplayNone
				}
				switch current.corners {
				case "shaped":
					style.CornerRadii = core.Corners{TopLeft: 24, TopRight: 12, BottomRight: 32, BottomLeft: 8}
				case "continuous":
					style.BorderCurve = core.BorderCurveContinuous
				}
				return style
			}))
		}
		return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xffffff)}, Children: children}
	})
	for _, kind := range []string{"view", "view-ripple", "view-foreground-ripple", "canvas", "image", "text", "input", "web", "scroll"} {
		for _, corners := range []string{"round", "shaped", "continuous"} {
			t.Run(kind+"/"+corners, func(t *testing.T) {
				app.Post(func() { selected.Set(sample{kind, corners}) })
				app.Settle()
				frame := app.Find("id:clipped-" + kind).Bounds()
				at := func(img image.Image, x, y core.Points) uint32 {
					px := img.Bounds().Min.X + int(float32(frame.Origin.X+x)*ratio)
					py := img.Bounds().Min.Y + int(float32(frame.Origin.Y+y)*ratio)
					r, g, b, _ := img.At(px, py).RGBA()
					return r>>8<<16 | g>>8<<8 | b>>8
				}
				img := app.Capture()
				if kind == "image" {
					// Pixel uploads and the native image's initial fade are asynchronous.
					deadline := time.Now().Add(5 * time.Second)
					for at(img, 60, 40) != 0xffff00 && time.Now().Before(deadline) {
						time.Sleep(20 * time.Millisecond)
						img = app.Capture()
					}
				}
				check := func(label string, x, y core.Points, want uint32) {
					t.Helper()
					if got := at(img, x, y); got != want {
						t.Errorf("%s at (%g,%g): #%06x, want #%06x", label, x, y, got, want)
					}
				}
				check("outer shadow", -9, 40, 0xff0000)
				check("outline", -4, 40, 0x00ff00)
				check("clipped overflow", 129, 40, 0xff0000)
				check("past shadow", 137, 40, 0xffffff)
				// Inside the bounds but outside the body and outline: only the outer shadow remains, with no rectangular content.
				check("rounded content", 1, 1, 0xff0000)
				if strings.HasPrefix(kind, "view") || kind == "canvas" || kind == "image" || kind == "scroll" {
					check("content", 60, 40, 0xffff00)
					if corners == "shaped" {
						check("smaller top-right corner", 116, 5, 0xffff00)
					}
				}
			})
		}
	}
}

func clipDecorationNode(kind string, style func() core.Style) core.Node {
	child := core.View{Style: core.Style{Width: core.Pt(180), Height: core.Pt(140), Background: core.RGB(0xffff00)}}
	switch kind {
	case "canvas":
		return core.Canvas{TestID: "clipped-" + kind, StyleFunc: style, Draw: func(d *core.Drawing, _ core.Size) {
			d.SetFillColor(core.RGB(0xffff00)).FillRect(-40, -40, 200, 160)
		}}
	case "image":
		pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
		pixels.Pix = []byte{255, 255, 0, 255}
		return core.Image{TestID: "clipped-" + kind, StyleFunc: style, Source: core.ImageSource{Pixels: pixels}, ResizeMode: core.Stretch}
	case "text":
		return core.Text{TestID: "clipped-" + kind, StyleFunc: style, Content: "Clipped text"}
	case "input":
		return core.Input{TestID: "clipped-" + kind, StyleFunc: style, DefaultValue: "Clipped input"}
	case "web":
		return core.WebView{TestID: "clipped-" + kind, StyleFunc: style, Source: core.WebHTML("<body style='background:yellow'>Clipped web</body>", "")}
	case "scroll":
		return core.Scroll{TestID: "clipped-" + kind, StyleFunc: style, Children: []core.Node{child}, HideIndicator: true}
	default:
		view := core.View{TestID: "clipped-" + kind, StyleFunc: style, Children: []core.Node{child}}
		if kind != "view" {
			view.Android.Ripple = &core.Ripple{Color: core.RGB(0x808080), Foreground: kind == "view-foreground-ripple"}
			view.OnPress = func(core.PressEvent) {}
		}
		return view
	}
}
