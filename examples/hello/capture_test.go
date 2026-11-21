package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestRuntimeCapture checks the runtime service on displayed pixels, including
// a Canvas, shader and native input, and reuses the result in an Image.
func TestRuntimeCapture(t *testing.T) {
	target := nsgotest.Target()
	switch target {
	case "macos", "ios", "ios_simulator", "windows", "linux", "android", "web":
	default:
		t.Skip("requires a runtime capture target")
	}
	const w, row = 120, 28
	style := core.Style{Width: core.Pt(w), Height: core.Pt(row)}
	red, green := core.RGB(0xff0000), core.RGB(0x00ff00)
	var scope *core.Scope
	var subtree, copyRef core.Ref
	var result *core.Signal[[]byte]
	loaded := make(chan struct{}, 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		result = core.NewSignalFunc(cx, nil, bytes.Equal)
		var shader core.Node = core.ShaderEffect{
			Style: style,
			Program: core.MustCompileWGSL(`
fn shade(uv: vec2f) -> vec4f {
	let color = sample_content(uv);
	return vec4f(color.bgr, color.a);
}
`),
			Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(w), Height: core.Pt(row), Background: red}}},
		}
		if target == "android" {
			// A shader fill is available before view effects land in the Android runtime.
			shader = core.Shader{Style: style, Program: core.MustCompileWGSL(`
fn shade(uv: vec2f) -> vec4f {
	return vec4f(0.0, 0.0, 1.0, 1.0);
}
`)}
		}
		return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(16), Gap: 4}, Children: []core.Node{
			core.View{Ref: &subtree, TestID: "capture-source", Style: core.Style{Width: core.Pt(w), Height: core.Pt(3 * row)},
				Children: []core.Node{
					core.Canvas{Style: style, Draw: func(d *core.Drawing, size core.Size) {
						d.SetFillColor(red).FillRect(0, 0, float32(size.Width), float32(size.Height))
					}},
					shader,
					core.Input{Style: core.Style{Width: core.Pt(w), Height: core.Pt(row), Background: green}},
				}},
			core.Image{Ref: &copyRef, TestID: "capture-result", Style: core.Style{Width: core.Pt(w), Height: core.Pt(3 * row)},
				SourceFunc:   func() core.ImageSource { return core.ImageSource{Data: result.Get()} },
				FadeDuration: time.Nanosecond,
				OnLoad: func(core.Size) {
					select {
					case loaded <- struct{}{}:
					default:
					}
				}},
		}}
	}, nsgotest.Size(280, 260))
	app.Find("id:capture-source")

	type capture struct {
		png []byte
		err error
	}
	read := func(ref *core.Ref, show bool) image.Image {
		t.Helper()
		reply := make(chan capture, 1)
		scope.Post(func() {
			scope.Capture(ref, func(data []byte, err error) {
				if show && err == nil {
					// The callback can update the reactive graph as an ordinary handler.
					result.Set(data)
				}
				reply <- capture{data, err}
			})
		})
		select {
		case got := <-reply:
			if got.err != nil {
				t.Fatalf("capture: %v", got.err)
			}
			img, err := png.Decode(bytes.NewReader(got.png))
			if err != nil {
				t.Fatal(err)
			}
			return img
		case <-time.After(10 * time.Second):
			t.Fatal("no capture result")
			return nil
		}
	}
	want := []color.RGBA{{R: 255, A: 255}, {B: 255, A: 255}, {G: 255, A: 255}}
	check := func(img image.Image) {
		t.Helper()
		s := img.Bounds().Size()
		if s.X < w || s.Y < 3*row || absCapture(s.X*3*row-s.Y*w) > w*2 {
			t.Errorf("capture size %v does not match the view", s)
		}
		for i, expect := range want {
			x, y := img.Bounds().Min.X+s.X*3/4, img.Bounds().Min.Y+s.Y*(2*i+1)/6
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if !closeCaptureColor(got, expect) {
				t.Errorf("view pixel %d at %d,%d is %v, want %v", i, x, y, got, expect)
			}
		}
	}
	// Apple compiles the effect asynchronously. Allow its first displayed
	// frame to arrive before using its pixels for the comparison.
	if target == "macos" || target == "ios" || target == "ios_simulator" {
		deadline := time.Now().Add(4 * time.Second)
		for {
			img := read(&subtree, false)
			s := img.Bounds().Size()
			if closeCaptureColor(color.RGBAModel.Convert(img.At(s.X*3/4, s.Y/2)).(color.RGBA), want[1]) || time.Now().After(deadline) {
				break
			}
			time.Sleep(60 * time.Millisecond)
		}
	}
	check(read(&subtree, true))
	select {
	case <-loaded:
	case <-time.After(10 * time.Second):
		t.Fatal("the capture did not load in an Image")
	}
	app.Find("id:capture-result")
	app.Settle()
	check(read(&copyRef, false))
	screen := read(nil, false)
	for _, expect := range want {
		found := false
		for y := screen.Bounds().Min.Y; y < screen.Bounds().Max.Y && !found; y += 4 {
			for x := screen.Bounds().Min.X; x < screen.Bounds().Max.X; x += 4 {
				if closeCaptureColor(color.RGBAModel.Convert(screen.At(x, y)).(color.RGBA), expect) {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("surface capture did not include %v", expect)
		}
	}
}

func absCapture(i int) int { return max(i, -i) }

func closeCaptureColor(a, b color.RGBA) bool {
	return absCapture(int(a.R)-int(b.R)) < 15 && absCapture(int(a.G)-int(b.G)) < 15 &&
		absCapture(int(a.B)-int(b.B)) < 15 && absCapture(int(a.A)-int(b.A)) < 15
}

func TestRuntimeCaptureShare(t *testing.T) {
	switch nsgotest.Target() {
	case "ios", "ios_simulator", "android":
	default:
		t.Skip("requires a mobile share sheet")
	}
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{TestID: "capture-share", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80), Background: core.RGB(0x123456)}}
	})
	app.Find("id:capture-share")
	app.Settle()
	result := make(chan error, 1)
	scope.Post(func() {
		scope.Capture(nil, func(data []byte, err error) {
			if err != nil {
				result <- err
				return
			}
			file, err := os.CreateTemp(scope.CacheDir(), "capture-*.png")
			if err != nil {
				result <- err
				return
			}
			path := file.Name()
			t.Cleanup(func() { os.Remove(path) })
			_, err = file.Write(data)
			if err := errors.Join(err, file.Close()); err != nil {
				result <- err
				return
			}
			scope.Share(core.ShareOptions{Files: []core.SharedFile{{Path: path}}}, func(shown bool, err error) {
				if err == nil && !shown {
					err = errors.New("the system did not show a share sheet")
				}
				result <- err
			})
		})
	})
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the capture and share request did not complete")
	}
}
