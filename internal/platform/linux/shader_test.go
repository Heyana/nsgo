//go:build linux && !android

package linux

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestShadersDrawWithGL opens a window with a shader view whose program
// mixes a uniform color, and reads its pixels.
func TestShadersDrawWithGL(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	prog := core.MustCompileShader(`package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	if pos.x < rectSize().x/2 {
		return Tint
	}
	return vec4(0, 0, 1, 1)
}
`)
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100), Background: driver.RGB(0xffffff)},
			Children: []core.Node{core.Shader{Program: prog, Uniforms: core.Uniforms{"Tint": driver.RGB(0x00ff00)},
				Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 200, Height: 100})
	waitForTitle(t, w, title)
	waitPixels(t, w, "the shader's green left half and blue right half", func(p pixels) bool {
		left, right := p.at(40, 50), p.at(160, 50)
		return left[1] > 0xf0 && left[2] < 0x10 && right[0] > 0xf0 && right[1] < 0x10
	})
}

// TestShadersReadImages draws an image stretched over a shader view:
// red on its left half, blue on its right.
func TestShadersReadImages(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for x := range 4 {
		for y := range 2 {
			c := color.RGBA{255, 0, 0, 255}
			if x >= 2 {
				c = color.RGBA{0, 0, 255, 255}
			}
			src.SetRGBA(x, y, c)
		}
	}
	var encoded bytes.Buffer
	png.Encode(&encoded, src)
	prog := core.MustCompileShader(`package main

func Fragment(dst vec4, pos vec2) vec4 {
	return image0At(pos * image0Size() / rectSize())
}
`)
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		return core.View{Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100), Background: driver.RGB(0xffffff)},
			Children: []core.Node{core.Shader{Program: prog, Images: []core.ImageSource{{Data: encoded.Bytes()}},
				Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100)}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 200, Height: 100})
	waitForTitle(t, w, title)
	waitPixels(t, w, "the image's red left half and blue right half", func(p pixels) bool {
		left, right := p.at(40, 50), p.at(160, 50)
		return left[2] > 0xf0 && left[0] < 0x10 && right[0] > 0xf0 && right[2] < 0x10
	})
}

// TestShaderViewsKeepOnlyTheirImages draws a shader view with a red
// image, then a blue one, and checks the view keeps only the blue one's
// texture.
func TestShaderViewsKeepOnlyTheirImages(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	var images []driver.ImageSource
	for _, c := range []color.RGBA{{255, 0, 0, 255}, {0, 0, 255, 255}} {
		src := image.NewRGBA(image.Rect(0, 0, 2, 2))
		for x := range 2 {
			for y := range 2 {
				src.SetRGBA(x, y, c)
			}
		}
		var encoded bytes.Buffer
		png.Encode(&encoded, src)
		images = append(images, driver.ImageSource{Data: encoded.Bytes()})
	}
	prog := core.MustCompileShader(`package main

func Fragment(dst vec4, pos vec2) vec4 {
	return image0At(pos * image0Size() / rectSize())
}
`)
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		return core.View{Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(100)},
			Children: []core.Node{core.Shader{Program: prog, Images: []core.ImageSource{images[0]},
				Style: core.Style{Width: driver.Pt(100), Height: driver.Pt(100)}}}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 100, Height: 100})
	waitForTitle(t, w, title)
	waitPixels(t, w, "the red image", func(p pixels) bool { px := p.at(50, 50); return px[2] > 0xf0 && px[0] < 0x10 })
	var g *glShaderView
	w.onLoop(t, func() {
		for _, v := range w.views {
			if v != nil && v.shader != nil {
				g = v.shader
				w.setShader(v, &driver.Shader{Program: g.shader.Program, Images: images[1:]})
			}
		}
	})
	if g == nil {
		t.Fatal("no shader view")
	}
	waitPixels(t, w, "the blue image", func(p pixels) bool { px := p.at(50, 50); return px[0] > 0xf0 && px[2] < 0x10 })
	w.onLoop(t, func() {
		if _, ok := g.textures[images[1].Key()]; len(g.textures) != 1 || !ok {
			t.Errorf("the view keeps %d textures, want only its image's", len(g.textures))
		}
	})
}

// TestShaderEffectReadsBackdrop opens a window with a red left half and blue right half behind a ShaderEffect whose WGSL shader swaps the red and blue channels of sample_backdrop(uv), and verifies the rendered pixels show blue on the left and red on the right (reading what is behind the view rather than transparency or the host's own background).
func TestShaderEffectReadsBackdrop(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	prog := core.MustCompileWGSL(`
fn shade(uv: vec2f) -> vec4f {
	let b = sample_backdrop(uv);
	return vec4f(b.b, b.g, b.r, b.a);
}
`)
	title := "nsgo-native-" + t.Name()
	app := func(cx *core.Scope) core.Node {
		cx.SetTitle(title)
		return core.View{
			Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(100), Background: driver.RGB(0xffffff)},
			Children: []core.Node{
				core.View{Style: core.Style{Position: core.Absolute, Left: driver.Pt(0), Top: driver.Pt(0),
					Width: driver.Pt(100), Height: driver.Pt(100), Background: driver.RGB(0xff0000)}},
				core.View{Style: core.Style{Position: core.Absolute, Left: driver.Pt(100), Top: driver.Pt(0),
					Width: driver.Pt(100), Height: driver.Pt(100), Background: driver.RGB(0x0000ff)}},
				core.ShaderEffect{
					Program: prog,
					Style: core.Style{Position: core.Absolute, Left: driver.Pt(20), Top: driver.Pt(10),
						Width: driver.Pt(160), Height: driver.Pt(80), Background: driver.RGB(0x00ff00)},
				},
			},
		}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 200, Height: 100})
	waitForTitle(t, w, title)
	waitPixels(t, w, "the backdrop-reading effect's swapped blue left half and red right half", func(p pixels) bool {
		// Inside effect left (red behind -> swapped blue) and right (blue behind -> swapped red).
		left, right := p.at(60, 50), p.at(140, 50)
		// Outside effect on the left remains original red.
		outside := p.at(10, 50)
		return left[0] > 0xf0 && left[1] < 0x10 && left[2] < 0x10 &&
			right[2] > 0xf0 && right[1] < 0x10 && right[0] < 0x10 &&
			outside[2] > 0xf0 && outside[0] < 0x10
	})
}
