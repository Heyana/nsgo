package capture_test

import (
	"image"
	"testing"

	_ "github.com/nsgo-dev/nsgo"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestMacOSTestSurfaceKeepsRequestedSize(t *testing.T) {
	if nsgotest.Target() != "macos" {
		t.Skip("checks the macOS test host's geometry")
	}
	const width, height = 1304, 1400
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{TestID: "root", Style: core.Style{FlexGrow: 1, Background: core.RGB(0x326699)}}
	}, nsgotest.Size(width, height))
	checkSize := func(width, height int) {
		t.Helper()
		if got := app.Find("id:root").Frame.Size; got != (core.Size{Width: core.Points(width), Height: core.Points(height)}) {
			t.Fatalf("requested %d × %d points, laid out at %g × %g", width, height, got.Width, got.Height)
		}
		pixels := app.Capture().Bounds().Size()
		scale := pixels.X / width
		if scale < 1 || pixels.X != width*scale || pixels.Y != height*scale {
			t.Fatalf("requested %d × %d points, captured %d × %d pixels", width, height, pixels.X, pixels.Y)
		}
	}
	checkSize(width, height)
	app.Resize(width, 1200)
	checkSize(width, 1200)
}

// TestCaptureShowsCompositedEffects checks that a macOS capture is what WindowServer composites: a filter and a box shadow are Core Animation's, and an AppKit redraw of the view tree leaves both out.
func TestCaptureShowsCompositedEffects(t *testing.T) {
	if nsgotest.Target() != "macos" {
		t.Skip("checks the macOS test host's capture")
	}
	const width, height = 300, 200
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Background: core.RGB(0xffffff), Padding: core.All(40), Gap: 40, Direction: core.Row}, Children: []core.Node{
			core.View{TestID: "inverted", Style: core.Style{Width: core.Pt(60), Height: core.Pt(60), Background: core.RGB(0xff0000), Filters: []core.Filter{core.Invert(1)}}},
			core.View{TestID: "shadowed", Style: core.Style{Width: core.Pt(40), Height: core.Pt(40), Background: core.RGB(0x0000ff),
				Shadows: []core.BoxShadow{{Offset: core.Point{Y: 30}, Color: core.RGB(0x000000)}}}},
		}}
	}, nsgotest.Size(width, height))
	img := app.Capture()
	at := func(x, y core.Points) (r, g, b uint32) {
		p := image.Pt(img.Bounds().Min.X+int(float64(x)*float64(img.Bounds().Dx())/width), img.Bounds().Min.Y+int(float64(y)*float64(img.Bounds().Dy())/height))
		r, g, b, _ = img.At(p.X, p.Y).RGBA()
		return r >> 8, g >> 8, b >> 8
	}
	// Antialiasing and the shadow's edge blend into neighbours, so each check leaves room around the colour it wants.
	if r, g, b := at(70, 70); r > 160 || g < 200 || b < 200 {
		t.Errorf("the inverted red box shows %d,%d,%d, want cyan", r, g, b)
	}
	if r, g, b := at(160, 60); r > 40 || g > 40 || b < 200 {
		t.Errorf("the shadowed box shows %d,%d,%d, want blue", r, g, b)
	}
	if r, g, b := at(160, 95); r > 40 || g > 40 || b > 40 {
		t.Errorf("below the shadowed box shows %d,%d,%d, want its black shadow", r, g, b)
	}
}

// TestCaptureRingAndShadowIsDeterministic checks that a scene with a core.Ring
// and a box shadow consistently captures the WindowServer composite (including
// both the ring and shadow) rather than intermittently falling back (#2614).
func TestCaptureRingAndShadowIsDeterministic(t *testing.T) {
	if nsgotest.Target() != "macos" {
		t.Skip("checks the macOS test host's composited capture")
	}
	const width, height = 240, 180
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{
			Style: core.Style{
				Background: core.RGB(0xffffff),
				Padding:    core.All(40),
			},
			Children: []core.Node{
				core.View{
					TestID: "card",
					Style: core.Style{
						Width:      core.Pt(120),
						Height:     core.Pt(60),
						Background: core.RGB(0x1e293b),
						Shadows: []core.BoxShadow{
							core.Ring(4, core.RGBA(0xf59e0b, 0.9)),
							{Offset: core.Point{Y: 24}, Color: core.RGB(0x000000)},
						},
					},
				},
			},
		}
	}, nsgotest.Size(width, height))
	app.Screenshot("ring-shadow")
	img := app.Capture()
	at := func(x, y core.Points) (r, g, b uint32) {
		p := image.Pt(img.Bounds().Min.X+int(float64(x)*float64(img.Bounds().Dx())/width), img.Bounds().Min.Y+int(float64(y)*float64(img.Bounds().Dy())/height))
		r, g, b, _ = img.At(p.X, p.Y).RGBA()
		return r >> 8, g >> 8, b >> 8
	}
	// Card sits at (40,40)-(160,100); the 4pt amber ring extends to y=36..40.
	if r, g, b := at(100, 38); r < 180 || g < 110 || b > 100 {
		t.Errorf("above the card shows %d,%d,%d, want the amber core.Ring", r, g, b)
	}
	// The black shadow offset by +24pt sits at (40,64)-(160,124), visible at y=112.
	if r, g, b := at(100, 112); r > 40 || g > 40 || b > 40 {
		t.Errorf("below the card shows %d,%d,%d, want its black shadow", r, g, b)
	}
}

// TestPartialBorderShowsOverABackdrop checks that a border on only some sides of a frosted, material or Liquid Glass view draws over its backdrop, the right way up, as a uniform border does (#935). Glass needs macOS 26 and falls back to the blur before it.
func TestPartialBorderShowsOverABackdrop(t *testing.T) {
	if nsgotest.Target() != "macos" {
		t.Skip("checks the macOS test host's capture")
	}
	for _, kind := range []string{"blur", "material", "glass"} {
		t.Run(kind, func(t *testing.T) {
			const width, height = 300, 300
			app := nsgotest.Start(t, func(*core.Scope) core.Node {
				v := core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(100), Background: core.RGBA(0xffffff, 0.3), BackdropFilters: []core.Filter{core.Blur(10)},
					Borders: core.Edges{Top: 6}, BorderColors: core.EdgeColors{Top: core.RGB(0xff0000)}}}
				switch kind {
				case "material":
					v.IOS.BackdropMaterial = core.BackdropRegular
				case "glass":
					v.Style.GlassEffect = core.GlassRegular
				}
				return core.View{Style: core.Style{Background: core.RGB(0x2050c0), Padding: core.All(50)}, Children: []core.Node{v}}
			}, nsgotest.Size(width, height))
			img := app.Capture()
			at := func(x, y core.Points) (r, g, b uint32) {
				p := image.Pt(img.Bounds().Min.X+int(float64(x)*float64(img.Bounds().Dx())/width), img.Bounds().Min.Y+int(float64(y)*float64(img.Bounds().Dy())/height))
				r, g, b, _ = img.At(p.X, p.Y).RGBA()
				return r >> 8, g >> 8, b >> 8
			}
			if r, g, b := at(150, 53); r < 180 || g > 110 || b > 110 {
				t.Errorf("the top border shows %d,%d,%d, want red over the backdrop", r, g, b)
			}
			if r, g, b := at(150, 147); r > 180 && g < 110 && b < 110 {
				t.Errorf("the bottom edge shows the top border's red %d,%d,%d: the ring is upside down", r, g, b)
			}
		})
	}
}

// TestCaptureIsSRGB checks that a macOS capture holds the sRGB values the app set rather than the display's, which on a Display P3 Mac read #2B7766 back as about #417567 (#311).
func TestCaptureIsSRGB(t *testing.T) {
	if nsgotest.Target() != "macos" {
		t.Skip("checks the macOS test host's capture")
	}
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(100), Height: core.Pt(100), Background: core.RGB(0x2b7766)}}
	}, nsgotest.Size(100, 100))
	img := app.Capture()
	b := img.Bounds()
	r, g, bl, _ := img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA()
	near := func(got uint32, want uint32) bool { return got>>8+3 >= want && got>>8 <= want+3 }
	if !near(r, 0x2b) || !near(g, 0x77) || !near(bl, 0x66) {
		t.Errorf("the capture shows #%02x%02x%02x, want #2b7766", r>>8, g>>8, bl>>8)
	}
}

func TestIOSCaptureIsSRGB(t *testing.T) {
	switch nsgotest.Target() {
	case "ios", "ios_simulator", "tvos_simulator":
	default:
		t.Skip("checks the UIKit test host's capture")
	}
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xff0000)}}
	})
	img := app.Capture()
	b := img.Bounds()
	r, g, bl, a := img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA()
	if r != 0xffff || g != 0 || bl != 0 || a != 0xffff {
		t.Errorf("the capture shows #%02x%02x%02x%02x, want #ff0000ff", r>>8, g>>8, bl>>8, a>>8)
	}
}
