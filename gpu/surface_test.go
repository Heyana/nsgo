package gpu_test

import (
	"errors"
	"image/color"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/gpu"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

type windowGPUState struct {
	device      *gpu.Device
	generations []uint32
	lost        []gpu.DeviceLostEvent
	errors      []gpu.Error
	pixels      []byte
	readErr     error
	reads       int
}

func (s *windowGPUState) read(pixels []byte, err error) {
	s.pixels, s.readErr = slices.Clone(pixels), err
	s.reads++
}

func windowGPU(state *windowGPUState, fill core.Color) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		return gpu.View(cx, gpu.ViewProps{
			TestID: "viewport", Style: core.Style{FlexGrow: 1},
			OnReady: func(dev *gpu.Device) {
				state.device = dev
				state.generations = append(state.generations, dev.Generation())
				encoder := dev.BeginEncoder()
				pass := encoder.BeginRenderPass(gpu.RenderPassDesc{})
				pass.ClearColor(fill)
				pass.End()
				dev.Submit(encoder.Finish())
				dev.ReadSurface(state.read)
			},
			OnDeviceLost: func(ev gpu.DeviceLostEvent) { state.lost = append(state.lost, ev) },
			OnError:      func(err gpu.Error) { state.errors = append(state.errors, err) },
		})
	}
}

func checkWindowGPU(t *testing.T, surface *nsgotest.Surface, state *windowGPUState, width, height int, want color.RGBA) {
	t.Helper()
	img, err := surface.GPUReadPixels("id:viewport")
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != width || img.Bounds().Dy() != height || img.RGBAAt(0, 0) != want {
		t.Fatalf("surface %q pixels = %v, %v; want %dx%d, %v", surface.Config().Title, img.Bounds(), img.RGBAAt(0, 0), width, height, want)
	}
	if state.readErr != nil || len(state.pixels) != width*height*4 {
		t.Fatalf("surface %q readback bytes=%d error=%v", surface.Config().Title, len(state.pixels), state.readErr)
	}
	if got := (color.RGBA{R: state.pixels[0], G: state.pixels[1], B: state.pixels[2], A: state.pixels[3]}); got != want {
		t.Fatalf("surface %q readback color=%v, want %v", surface.Config().Title, got, want)
	}
	if len(state.errors) != 0 {
		t.Fatalf("surface %q errors = %v", surface.Config().Title, state.errors)
	}
}

func TestHeadlessGPUWindowsKeepDevicesAndReadbacksSeparate(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("checks the headless software GPU")
	}
	for _, platform := range []struct {
		name string
		os   core.OS
	}{{"macos", core.OSMacOS}, {"windows", core.OSWindows}, {"linux", core.OSLinux}} {
		t.Run(platform.name, func(t *testing.T) { checkHeadlessGPUWindows(t, platform.os) })
	}
}

func checkHeadlessGPUWindows(t *testing.T, os core.OS) {
	t.Helper()
	mainState, inspectorState := &windowGPUState{}, &windowGPUState{}
	app := nsgotest.StartApp(t, func(cx *core.Scope) {
		cx.SetMainView(windowGPU(mainState, core.RGB(0xff0000)))
		desktop.OpenWindow(cx, desktop.WindowOptions{Title: "Inspector", Width: 48, Height: 32}, windowGPU(inspectorState, core.RGB(0x0000ff)))
	}, nsgotest.Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop, PixelRatio: 1}), nsgotest.Size(64, 48))
	main, inspector := app.Surface("index:0"), app.Surface("title:Inspector")
	if main.Find("id:viewport").ID != inspector.Find("id:viewport").ID {
		t.Fatal("fixture must exercise equal view IDs in different trees")
	}
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	checkWindowGPU(t, main, mainState, 64, 48, red)
	checkWindowGPU(t, inspector, inspectorState, 48, 32, blue)
	frames := app.GPUFrames()
	for _, surface := range []*nsgotest.Surface{main, inspector} {
		if !slices.ContainsFunc(frames, func(frame nsgotest.GPUFrameRecord) bool { return frame.SurfaceID == surface.Config().ID }) {
			t.Fatalf("app-wide frames lost surface %d: %+v", surface.Config().ID, frames)
		}
	}
	inspector.TriggerGPUDeviceLost("id:viewport", nsgotest.GPULostReset, "inspector reset")
	if len(inspectorState.lost) != 1 || inspectorState.lost[0].Message != "inspector reset" || len(mainState.lost) != 0 {
		t.Fatalf("device loss crossed windows: main=%v inspector=%v", mainState.lost, inspectorState.lost)
	}
	// Recovery takes the selected window's scale, leaving the main device alone.
	app.SetWindowEnvironment("Inspector", func(env *core.Environment) { env.PixelRatio = 2 })
	inspector.RestoreGPUDevice("id:viewport")
	if !slices.Equal(mainState.generations, []uint32{1}) || !slices.Equal(inspectorState.generations, []uint32{1, 2}) {
		t.Fatalf("device recovery crossed windows: main=%v inspector=%v", mainState.generations, inspectorState.generations)
	}
	checkWindowGPU(t, main, mainState, 64, 48, red)
	checkWindowGPU(t, inspector, inspectorState, 96, 64, blue)
	if !inspector.Close() {
		t.Fatal("inspector did not close")
	}
	var closedErr error
	app.Post(func() {
		inspectorState.device.ReadSurface(func(_ []byte, err error) { closedErr = err })
		mainState.device.ReadSurface(mainState.read)
	})
	app.Settle()
	if !errors.Is(closedErr, gpu.ErrDeviceLost) || mainState.reads != 2 || inspectorState.reads != 2 {
		t.Fatalf("closed window readback error=%v, read counts main=%d inspector=%d", closedErr, mainState.reads, inspectorState.reads)
	}
	checkWindowGPU(t, main, mainState, 64, 48, red)
}
