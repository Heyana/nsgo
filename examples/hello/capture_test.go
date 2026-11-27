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
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestRuntimeCapture checks the runtime service on displayed pixels, including
// a Canvas, shader effect, native input and GPU view, and reuses the result in an Image.
func TestRuntimeCapture(t *testing.T) {
	target := nsgotest.Target()
	switch target {
	case "macos", "ios", "ios_simulator", "windows", "linux", "android", "web":
	default:
		t.Skip("requires a runtime capture target")
	}
	const w, row = 120, 28
	style := core.Style{Width: core.Pt(w), Height: core.Pt(row)}
	red, green, yellow := core.RGB(0xff0000), core.RGB(0x00ff00), core.RGB(0xffff00)
	var scope *core.Scope
	var subtree, copyRef, gpuRef core.Ref
	var result *core.Signal[[]byte]
	var hasGPU bool
	loaded := make(chan struct{}, 1)
	gpuRendered := make(chan struct{}, 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		hasGPU = cx.Env().SupportsGPU()
		result = core.NewSignalFunc(cx, nil, bytes.Equal)
		shader := core.ShaderEffect{
			Style: style,
			Program: core.MustCompileWGSL(`
fn shade(uv: vec2f) -> vec4f {
	let color = sample_content(uv);
	return vec4f(color.bgr, color.a);
}
`),
			Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(w), Height: core.Pt(row), Background: red}}},
		}
		rows := []core.Node{
			core.Canvas{Style: style, Draw: func(d *core.Drawing, size core.Size) {
				d.SetFillColor(red).FillRect(0, 0, float32(size.Width), float32(size.Height))
			}},
			shader,
			core.Input{Style: core.Style{Width: core.Pt(w), Height: core.Pt(row), Background: green}},
		}
		if hasGPU {
			var drawn bool
			rows = append(rows, gpu.View(cx, gpu.ViewProps{
				Ref:    &gpuRef,
				TestID: "capture-gpu",
				Style:  style,
				OnError: func(err gpu.Error) {
					t.Errorf("gpu.View OnError: %v", err)
				},
				OnFrame: func(frame *gpu.Frame) {
					if drawn {
						return
					}
					drawn = true
					pass := frame.BeginRenderPass(gpu.RenderPassDesc{})
					pass.ClearColor(yellow)
					pass.End()
					frame.Submit()
					select {
					case gpuRendered <- struct{}{}:
					default:
					}
				},
			}))
		}
		boxHeight := core.Pt(core.Points(len(rows) * row))
		return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(16), Gap: 4}, Children: []core.Node{
			core.View{Ref: &subtree, TestID: "capture-source", Style: core.Style{Width: core.Pt(w), Height: boxHeight},
				Children: rows},
			core.Image{Ref: &copyRef, TestID: "capture-result", Style: core.Style{Width: core.Pt(w), Height: boxHeight},
				SourceFunc:   func() core.ImageSource { return core.ImageSource{Data: result.Get()} },
				FadeDuration: time.Nanosecond,
				OnLoad: func(core.Size) {
					select {
					case loaded <- struct{}{}:
					default:
					}
				}},
		}}
	}, nsgotest.Size(280, 300))
	app.Find("id:capture-source")
	if target == "macos" {
		// On a locked-screen macOS host AppKit reports the window occluded
		// (LifecycleBackground), which pauses Metal shader effects and gpu.View.
		var bg bool
		deadline := time.Now().Add(500 * time.Millisecond)
		for {
			done := make(chan bool, 1)
			scope.Post(func() { done <- scope.Env().Lifecycle() == core.LifecycleBackground })
			bg = <-done
			if !bg || time.Now().After(deadline) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if bg {
			t.Skip("macOS window is occluded (locked screen); Metal shader effects and GPU views are suspended")
		}
	}
	if hasGPU {
		select {
		case <-gpuRendered:
		case <-time.After(10 * time.Second):
			t.Fatal("gpu.View did not render its initial frame")
		}
	}

	type capture struct {
		png []byte
		err error
	}
	read := func(ref *core.Ref, show bool) image.Image {
		t.Helper()
		reply := make(chan capture, 1)
		scope.Post(func() {
			syncCalled := false
			scope.Capture(ref, func(data []byte, err error) {
				syncCalled = true
				if show && err == nil {
					// The callback can update the reactive graph as an ordinary handler.
					result.Set(data)
				}
				reply <- capture{data, err}
			})
			if syncCalled {
				t.Errorf("Capture invoked its callback synchronously")
			}
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
	if hasGPU {
		want = append(want, color.RGBA{R: 255, G: 255, A: 255})
	}
	numRows := len(want)
	check := func(img image.Image) {
		t.Helper()
		s := img.Bounds().Size()
		if s.X < w || s.Y < numRows*row || absCapture(s.X*numRows*row-s.Y*w) > w*2 {
			t.Errorf("capture size %v does not match the view (%dx%d)", s, w, numRows*row)
		}
		for i, expect := range want {
			x, y := img.Bounds().Min.X+s.X*3/4, img.Bounds().Min.Y+s.Y*(2*i+1)/(2*numRows)
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if !closeCaptureColor(got, expect) {
				t.Errorf("view pixel %d at %d,%d is %v, want %v", i, x, y, got, expect)
			}
		}
	}
	// Shader effects and GPU layers present asynchronously on Apple/Android/Web.
	// Poll briefly until both the effect row and GPU row have landed on screen.
	deadline := time.Now().Add(4 * time.Second)
	for {
		img := read(&subtree, false)
		s := img.Bounds().Size()
		ready := true
		for i, expect := range want {
			x, y := img.Bounds().Min.X+s.X*3/4, img.Bounds().Min.Y+s.Y*(2*i+1)/(2*numRows)
			if !closeCaptureColor(color.RGBAModel.Convert(img.At(x, y)).(color.RGBA), expect) {
				ready = false
				break
			}
		}
		if ready || time.Now().After(deadline) {
			break
		}
		time.Sleep(60 * time.Millisecond)
	}
	check(read(&subtree, true))
	if hasGPU {
		gpuImg := read(&gpuRef, false)
		s := gpuImg.Bounds().Size()
		got := color.RGBAModel.Convert(gpuImg.At(gpuImg.Bounds().Min.X+s.X/2, gpuImg.Bounds().Min.Y+s.Y/2)).(color.RGBA)
		if !closeCaptureColor(got, color.RGBA{R: 255, G: 255, A: 255}) {
			t.Errorf("direct gpu.View capture center pixel is %v, want yellow", got)
		}
	}
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
