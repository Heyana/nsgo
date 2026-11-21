package nsgotest

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func cursorPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.SetRGBA(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestActiveCustomCursorAndHiDPISelection(t *testing.T) {
	png1x := cursorPNG(t, 24, 24, color.RGBA{R: 255, A: 255})
	png2x := cursorPNG(t, 48, 48, color.RGBA{G: 255, A: 255})

	brush := &core.CustomCursor{
		Image:    core.ImageSource{Data: png1x},
		Image2x:  core.ImageSource{Data: png2x},
		Hotspot:  core.Point{X: 30, Y: -4}, // clamps to (23, 0)
		Fallback: core.CursorCrosshair,
	}

	root := func(cx *core.Scope) core.Node {
		return core.View{
			TestID: "canvas",
			Style: core.Style{
				Width:       core.Pt(200),
				Height:      core.Pt(200),
				CursorImage: brush,
			},
			Children: []core.Node{
				core.View{
					TestID: "inherited",
					Style:  core.Style{Width: core.Pt(80), Height: core.Pt(80)},
				},
				core.View{
					TestID: "named",
					Style:  core.Style{Width: core.Pt(80), Height: core.Pt(80), Cursor: core.CursorGrab},
				},
				core.View{
					TestID: "empty-custom",
					Style: core.Style{
						Width:       core.Pt(40),
						Height:      core.Pt(40),
						CursorImage: &core.CustomCursor{Fallback: core.CursorHelp},
					},
				},
			},
		}
	}

	// 1x display selects Image (1x).
	app1x := Start(t, root, Environment(driver.Environment{OS: driver.OSMacOS, Idiom: driver.IdiomDesktop, PixelRatio: 1}))
	if got := app1x.ActiveCursor(); got.Cursor != driver.CursorAuto || got.Custom != nil {
		t.Fatalf("before hover: got %+v, want auto", got)
	}

	app1x.Hover("id:inherited")
	ac := app1x.ActiveCursor()
	if ac.Custom == nil || ac.Is2x || !bytes.Equal(ac.Source.Data, png1x) {
		t.Fatalf("1x hover inherited: got %+v, want 1x brush", ac)
	}
	if ac.Hotspot != (driver.Point{X: 23, Y: 0}) {
		t.Fatalf("1x clamped hotspot = %v, want (23, 0)", ac.Hotspot)
	}
	if ac.Fallback != driver.CursorCrosshair {
		t.Fatalf("1x fallback = %v, want crosshair", ac.Fallback)
	}

	app1x.Hover("id:named")
	if ac := app1x.ActiveCursor(); ac.Custom != nil || ac.Cursor != driver.CursorGrab {
		t.Fatalf("named child override: got %+v, want grab", ac)
	}

	if ac := app1x.CursorAt("id:empty-custom"); ac.Cursor != driver.CursorHelp || !ac.Source.Empty() {
		t.Fatalf("empty custom cursor: got %+v, want help fallback", ac)
	}

	app1x.HoverOut()
	if ac := app1x.ActiveCursor(); ac.Cursor != driver.CursorAuto || ac.Custom != nil {
		t.Fatalf("after HoverOut: got %+v, want auto", ac)
	}

	// 2x Retina display selects Image2x.
	app2x := Start(t, root, Environment(driver.Environment{OS: driver.OSMacOS, Idiom: driver.IdiomDesktop, PixelRatio: 2}))
	app2x.Hover("id:canvas")
	ac2 := app2x.ActiveCursor()
	if ac2.Custom == nil || !ac2.Is2x || !bytes.Equal(ac2.Source.Data, png2x) {
		t.Fatalf("2x hover canvas: got Is2x=%v len=%d, want 2x", ac2.Is2x, len(ac2.Source.Data))
	}
	if ac2.Hotspot != (driver.Point{X: 23, Y: 0}) {
		t.Fatalf("2x clamped hotspot = %v, want (23, 0)", ac2.Hotspot)
	}
}

func TestEnvSupportsCustomCursorAcrossPlatforms(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  driver.Environment
		want bool
	}{
		{"macos", driver.Environment{OS: driver.OSMacOS, Idiom: driver.IdiomDesktop}, true},
		{"windows", driver.Environment{OS: driver.OSWindows, Idiom: driver.IdiomDesktop}, true},
		{"linux", driver.Environment{OS: driver.OSLinux, Idiom: driver.IdiomDesktop}, true},
		{"web", driver.Environment{OS: driver.OSWeb, Idiom: driver.IdiomDesktop}, true},
		{"ios", driver.Environment{OS: driver.OSIOS, Idiom: driver.IdiomPhone}, false},
		{"android", driver.Environment{OS: driver.OSAndroid, Idiom: driver.IdiomPhone}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var supported bool
			Start(t, func(cx *core.Scope) core.Node {
				supported = cx.Env().SupportsCustomCursor()
				return core.View{}
			}, Environment(tc.env))
			if supported != tc.want {
				t.Errorf("SupportsCustomCursor() = %v, want %v", supported, tc.want)
			}
		})
	}
}
