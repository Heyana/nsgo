//go:build js && wasm && !nsgominiprogram

package web

import (
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type shaderOps struct {
	create          func(*surface, *view)
	setShader       func(*surface, *view, *driver.Shader)
	setEffect       func(*surface, *view, *driver.Shader)
	startTransition func(*surface, *view)
	shadeLater      func(*surface, *view)
	changedShaders  func(*surface)
	drawShaders     func(*surface)
	staleEffects    func(*surface)
	release         func(*surface, *view)
}

type canvasOps struct {
	setDrawing       func(*surface, *view, *driver.Drawing)
	paint            func(*surface, *view)
	changedCanvases  func(*surface)
	refreshScale     func(*surface, js.Value)
	offscreenCanvas  func(*surface, *driver.OffscreenCanvas, *view) (js.Value, float64, bool)
	releaseOffscreen func(*surface, uint64)
}

type videoOps struct {
	create   func(*surface, *view)
	setVideo func(*surface, *view, *driver.VideoConfig)
	seek     func(*view, time.Duration)
	release  func(*view)
}

type webViewOps struct {
	create  func(*surface, *view)
	setWeb  func(*surface, *view, *driver.WebConfig)
	command func(*surface, *view, driver.WebCommand, string, any)
	close   func(*view)
}

type nativeOps struct {
	configure      func(*surface, *view, *driver.NativeConfig)
	command        func(*view, string, any)
	changedNatives func(*surface)
	close          func(*view)
}

var (
	shaders  atomic.Pointer[shaderOps]
	canvases atomic.Pointer[canvasOps]
	videos   atomic.Pointer[videoOps]
	webViews atomic.Pointer[webViewOps]
	natives  atomic.Pointer[nativeOps]
)

func (*surface) EnableShaders() {
	if shaders.Load() != nil {
		return
	}
	shaders.Store(&shaderOps{
		create:          (*surface).createShader,
		setShader:       (*surface).setShader,
		setEffect:       (*surface).setEffect,
		startTransition: (*surface).startTransition,
		shadeLater:      (*surface).shadeLater,
		changedShaders:  (*surface).changedShaders,
		drawShaders:     (*surface).drawShaders,
		staleEffects:    (*surface).staleEffects,
		release: func(s *surface, v *view) {
			s.endEffect(v)
			if v.gl != nil {
				v.gl.release()
			}
		},
	})
}

func (*surface) EnableCanvas() {
	if canvases.Load() != nil {
		return
	}
	canvases.Store(&canvasOps{
		setDrawing:       (*surface).setDrawing,
		paint:            (*surface).paint,
		changedCanvases:  (*surface).changedCanvases,
		refreshScale:     (*surface).refreshCanvasScale,
		offscreenCanvas:  (*surface).offscreenCanvas,
		releaseOffscreen: (*surface).releaseOffscreen,
	})
}

func (*surface) EnableVideo() {
	if videos.Load() != nil {
		return
	}
	videos.Store(&videoOps{
		create:   (*surface).createVideo,
		setVideo: (*surface).setVideo,
		seek: func(v *view, d time.Duration) {
			if v.video != nil {
				v.video.seek(d)
			}
		},
		release: func(v *view) {
			if v.video != nil {
				v.video.release()
			}
		},
	})
}

func (*surface) EnableWebView() {
	if webViews.Load() != nil {
		return
	}
	webViews.Store(&webViewOps{
		create:  (*surface).createWeb,
		setWeb:  (*surface).setWeb,
		command: (*surface).webCommand,
		close: func(v *view) {
			if v.web != nil {
				v.web.closePort()
			}
		},
	})
}

func (*surface) EnableNative() {
	if natives.Load() != nil {
		return
	}
	natives.Store(&nativeOps{
		configure: (*surface).configureNative,
		command: func(v *view, name string, data any) {
			if v.component != nil {
				v.component.Command(name, data)
			}
		},
		changedNatives: (*surface).changedNatives,
		close: func(v *view) {
			if v.component != nil {
				v.component.Close()
			}
		},
	})
}
