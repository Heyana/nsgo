//go:build windows

package windows

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/shader"
	"github.com/nsgo-dev/nsgo/internal/shader/wgsl"
)

// TestShadersReadImages draws a shader that reads an image with Direct3D
// into a view's texture, which the painter draws, and reads the pixels
// back: the image's left half red, its right half blue, stretched over
// the view at half alpha.
func TestShadersReadImages(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
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
	pic, err := decode(context.Background(), encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img := driver.ImageSource{Data: encoded.Bytes()}
	w := &window{scale: 1, animator: driver.NewAnimator(nil)}
	w.canvasImages = map[string]*imageSlot{img.Key(): {src: img, pic: pic}}
	ir, err := shader.CompileFragment([]byte(`package main

var Alpha float

func Fragment(dst vec4, pos vec2) vec4 {
	return image0At(pos*image0Size()/rectSize()) * Alpha
}
`))
	if err != nil {
		t.Fatal(err)
	}
	prog := &driver.ShaderProgram{IR: ir}
	v := &view{id: 1}
	w.setShader(v, &driver.Shader{Program: prog, Lanes: []float64{0.5}, Images: []driver.ImageSource{img}})

	const pw, ph = 8, 4
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96, dpiY: 96,
		options: d2dBitmapTarget}
	var target, readable *comObject
	if failed(g.dc.callSize(dcCreateBitmap, pw, ph, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target)))) {
		t.Fatal("no target bitmap")
	}
	defer target.release()
	props.options = d2dBitmapCPURead | d2dBitmapCannotDraw
	if failed(g.dc.callSize(dcCreateBitmap, pw, ph, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&readable)))) {
		t.Fatal("no readable bitmap")
	}
	defer readable.release()
	g.dc.call(dcSetTarget, target.ptr())
	g.dc.callF(rtSetDpi, 0b11, f32(96), f32(96))
	g.dc.call(rtBeginDraw)
	clear := colorF{}
	g.dc.call(rtClear, uintptr(unsafe.Pointer(&clear)))
	p := newPainter(w, g)
	p.setTransform(identity)
	p.drawShader(v, pw, ph)
	p.release()
	if hr := g.dc.call(rtEndDraw, 0, 0); failed(hr) {
		t.Fatal(hresultError("EndDraw", hr))
	}
	g.dc.call(dcSetTarget, 0)
	v.shaderView.release()
	readable.call(bitmapCopyFromBitmap, 0, target.ptr(), 0)
	var mapped struct {
		pitch uint32
		bits  *byte
	}
	if failed(readable.call(bitmapMap, d2dMapRead, uintptr(unsafe.Pointer(&mapped)))) {
		t.Fatal("cannot map the pixels")
	}
	pix := unsafe.Slice(mapped.bits, int(mapped.pitch)*ph)
	left, right := pix[1*int(mapped.pitch)+1*4:], pix[1*int(mapped.pitch)+6*4:]
	readable.call(bitmapUnmap)
	if left[2] < 120 || left[0] > 10 || left[3] < 120 || left[3] > 135 {
		t.Errorf("left pixel %v, want half red", left[:4])
	}
	if right[0] < 120 || right[2] > 10 {
		t.Errorf("right pixel %v, want half blue", right[:4])
	}
}

// TestShaderViewsKeepOnlyTheirImages draws a shader view with one image,
// then another, and checks the view keeps only the second's texture.
func TestShaderViewsKeepOnlyTheirImages(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	w := &window{scale: 1, animator: driver.NewAnimator(nil), canvasImages: map[string]*imageSlot{}}
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
		pic, err := decode(context.Background(), encoded.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		img := driver.ImageSource{Data: encoded.Bytes()}
		w.canvasImages[img.Key()] = &imageSlot{src: img, pic: pic}
		images = append(images, img)
	}
	ir, err := shader.CompileFragment([]byte(`package main

func Fragment(dst vec4, pos vec2) vec4 {
	return image0At(pos)
}
`))
	if err != nil {
		t.Fatal(err)
	}
	prog := &driver.ShaderProgram{IR: ir}
	v := &view{id: 1}
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96, dpiY: 96,
		options: d2dBitmapTarget}
	var target *comObject
	if failed(g.dc.callSize(dcCreateBitmap, 4, 4, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target)))) {
		t.Fatal("no target bitmap")
	}
	defer target.release()
	defer func() { v.shaderView.release() }()
	for _, img := range images {
		w.setShader(v, &driver.Shader{Program: prog, Images: []driver.ImageSource{img}})
		g.dc.call(dcSetTarget, target.ptr())
		g.dc.call(rtBeginDraw)
		p := newPainter(w, g)
		p.setTransform(identity)
		p.drawShader(v, 4, 4)
		p.release()
		if hr := g.dc.call(rtEndDraw, 0, 0); failed(hr) {
			t.Fatal(hresultError("EndDraw", hr))
		}
		g.dc.call(dcSetTarget, 0)
		if v.shaderView.images.len() != 1 || v.shaderView.images.entries[img.Key()] == nil {
			t.Errorf("the view keeps %d textures, want only its image's", v.shaderView.images.len())
		}
	}
}

// TestShaderEffectReadsBackdrop draws a WGSL ShaderEffect whose shader swaps the red and blue channels of sample_backdrop(uv) over a red left half and blue right half, verifying that sample_backdrop reads what was drawn behind the host (not transparency, and not the host's own background), that overflow reaches past the host's box, and that a ScreenEffect on the root reads transparent.
func TestShaderEffectReadsBackdrop(t *testing.T) {
	g, err := devices()
	if err != nil {
		t.Skip(err)
	}
	compiled, err := wgsl.Compile(`
fn shade(uv: vec2f) -> vec4f {
	let b = sample_backdrop(uv);
	return vec4f(b.b, b.g, b.r, b.a);
}
`)
	if err != nil {
		t.Fatal(err)
	}
	prog := &driver.ShaderProgram{IR: compiled.IR, WGSL: true}

	renderTree := func(pw, ph int, build func(w *window) *view) ([]byte, int) {
		w := &window{scale: 1, animator: driver.NewAnimator(nil)}
		root := build(w)
		props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96, dpiY: 96,
			options: d2dBitmapTarget}
		var target, readable *comObject
		if failed(g.dc.callSize(dcCreateBitmap, uint32(pw), uint32(ph), 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target)))) {
			t.Fatal("no target bitmap")
		}
		defer target.release()
		props.options = d2dBitmapCPURead | d2dBitmapCannotDraw
		if failed(g.dc.callSize(dcCreateBitmap, uint32(pw), uint32(ph), 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&readable)))) {
			t.Fatal("no readable bitmap")
		}
		defer readable.release()
		g.dc.call(dcSetTarget, target.ptr())
		g.dc.callF(rtSetDpi, 0b11, f32(96), f32(96))
		g.dc.call(rtBeginDraw)
		clear := colorF{}
		g.dc.call(rtClear, uintptr(unsafe.Pointer(&clear)))
		p := newPainter(w, g)
		p.targetWidth, p.targetHeight, p.targetAlpha = uint32(pw), uint32(ph), true
		p.draw(root, identity, 0, 0)
		p.release()
		if hr := g.dc.call(rtEndDraw, 0, 0); failed(hr) {
			t.Fatal(hresultError("EndDraw", hr))
		}
		g.dc.call(dcSetTarget, 0)
		for v := range w.effects {
			w.endEffect(v)
		}
		readable.call(bitmapCopyFromBitmap, 0, target.ptr(), 0)
		var mapped struct {
			pitch uint32
			bits  *byte
		}
		if failed(readable.call(bitmapMap, d2dMapRead, uintptr(unsafe.Pointer(&mapped)))) {
			t.Fatal("cannot map the pixels")
		}
		defer readable.call(bitmapUnmap)
		src := unsafe.Slice(mapped.bits, int(mapped.pitch)*ph)
		out := make([]byte, len(src))
		copy(out, src)
		return out, int(mapped.pitch)
	}

	// 1. ShaderEffect over red left half and blue right half with overflow and its own green background: sample_backdrop reads red/blue behind it (not green), so swapping R<->B outputs blue on the left and red on the right, including inside the overflow band.
	pix, pitch := renderTree(40, 20, func(w *window) *view {
		root := &view{id: 1, kind: driver.KindView, frame: driver.Bounds{Size: driver.Size{Width: 40, Height: 20}},
			style: driver.ViewStyle{Opacity: 1}}
		leftBG := &view{id: 2, kind: driver.KindView, parent: root,
			frame: driver.Bounds{Size: driver.Size{Width: 20, Height: 20}},
			style: driver.ViewStyle{Opacity: 1, Background: driver.RGB(0xff0000)}}
		rightBG := &view{id: 3, kind: driver.KindView, parent: root,
			frame: driver.Bounds{Origin: driver.Point{X: 20}, Size: driver.Size{Width: 20, Height: 20}},
			style: driver.ViewStyle{Opacity: 1, Background: driver.RGB(0x0000ff)}}
		fxView := &view{id: 4, kind: driver.KindView, parent: root,
			frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 4}, Size: driver.Size{Width: 20, Height: 12}},
			style: driver.ViewStyle{Opacity: 1, Background: driver.RGB(0x00ff00)}}
		// Later sibling after fxView must not leak into fxView's backdrop.
		after := &view{id: 5, kind: driver.KindView, parent: root,
			frame: driver.Bounds{Origin: driver.Point{X: 36, Y: 0}, Size: driver.Size{Width: 4, Height: 20}},
			style: driver.ViewStyle{Opacity: 1, Background: driver.RGB(0x00ff00)}}
		root.children = []*view{leftBG, rightBG, fxView, after}
		w.root = root.id
		w.views = []*view{nil, root, leftBG, rightBG, fxView, after}
		const ov = 4.0
		w.setEffect(fxView, &driver.Shader{
			Program:  prog,
			Lanes:    []float64{0, 0, ov},
			Role:     driver.ShaderEffect,
			Pictures: driver.PictureBackdrop,
			Overflow: ov,
		})
		return root
	})
	at := func(x, y int) []byte { return pix[y*pitch+x*4 : y*pitch+x*4+4] }
	// Inside fxView left half (x=14, y=10): red behind -> swapped to blue (BGRA: B>240, G<10, R<10).
	if p := at(14, 10); p[0] < 240 || p[1] > 10 || p[2] > 10 {
		t.Errorf("left effect pixel = %v, want swapped blue", p)
	}
	// Inside fxView right half (x=25, y=10): blue behind -> swapped to red (BGRA: B<10, G<10, R>240).
	if p := at(25, 10); p[2] < 240 || p[1] > 10 || p[0] > 10 {
		t.Errorf("right effect pixel = %v, want swapped red", p)
	}
	// Inside left overflow band (x=8, y=10): still over red leftBG -> swapped to blue.
	if p := at(8, 10); p[0] < 240 || p[1] > 10 || p[2] > 10 {
		t.Errorf("left overflow pixel = %v, want swapped blue", p)
	}
	// Outside overflow band (x=2, y=10): untouched red leftBG.
	if p := at(2, 10); p[2] < 240 || p[1] > 10 || p[0] > 10 {
		t.Errorf("outside left pixel = %v, want original red", p)
	}

	// 2. ScreenEffect on root reading sample_backdrop reads transparent (nothing behind root).
	rootPix, rootPitch := renderTree(16, 16, func(w *window) *view {
		root := &view{id: 1, kind: driver.KindView, frame: driver.Bounds{Size: driver.Size{Width: 16, Height: 16}},
			style: driver.ViewStyle{Opacity: 1}}
		screen := &view{id: 2, kind: driver.KindView, parent: root,
			style: driver.ViewStyle{Opacity: 1}}
		root.children = []*view{screen}
		w.root = root.id
		w.views = []*view{nil, root, screen}
		w.setEffect(screen, &driver.Shader{
			Program:  prog,
			Lanes:    []float64{0, 0, 0},
			Role:     driver.ShaderScreen,
			Pictures: driver.PictureBackdrop,
		})
		return root
	})
	if p := rootPix[8*rootPitch+8*4 : 8*rootPitch+8*4+4]; p[3] != 0 {
		t.Errorf("root screen-effect backdrop pixel = %v, want transparent", p)
	}
}
